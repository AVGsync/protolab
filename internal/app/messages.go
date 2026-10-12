package app

import (
	"container/list"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"unsafe"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

func itoa(n int) string { return strconv.Itoa(n) }

// messageCacheBytes is the memory budget of a project's message cache.
const messageCacheBytes = 256 << 20

// messageCache is an LRU of parsed sessions keyed by session, spec hash,
// the analysis that produced the session and the project revision (actions
// and settings change the result). Its budget is an estimate of the memory
// the messages hold, not a number of entries or messages: one session of a
// long capture can hold more messages than hundreds of short ones, and a
// message with a 255-entry array outweighs a hundred short commands.
type messageCache struct {
	mu     sync.Mutex
	budget int // bytes
	used   int
	order  *list.List
	items  map[string]*list.Element
}

type cacheEntry struct {
	key    string
	parsed engine.ParsedSession
	weight int
}

func newMessageCache(budget int) *messageCache {
	return &messageCache{budget: budget, order: list.New(), items: map[string]*list.Element{}}
}

// parsedSize estimates the memory a parsed session holds.
func parsedSize(ps engine.ParsedSession) int {
	n := int(unsafe.Sizeof(ps)) + len(ps.Unframed)*int(unsafe.Sizeof(engine.Unframed{}))
	for i := range ps.Messages {
		m := &ps.Messages[i]
		n += int(unsafe.Sizeof(*m)) + len(m.Data) + 8*len(m.PacketNumbers) +
			len(m.Unknown)*int(unsafe.Sizeof(model.ByteRange{})) + fieldsSize(m.Fields)
		for _, s := range m.Issues {
			n += len(s) + 16
		}
	}
	return n
}

func fieldsSize(fields []model.ParsedField) int {
	n := 0
	for i := range fields {
		f := &fields[i]
		// Name, Type and the meaning strings are shared with the spec.
		n += int(unsafe.Sizeof(*f)) + len(f.Path) + len(f.Display) + len(f.Issue) + fieldsSize(f.Children)
		if s, ok := f.Value.(string); ok {
			n += len(s) + 16
		} else if f.Value != nil {
			n += 8
		}
	}
	return n
}

func (c *messageCache) get(key string) (engine.ParsedSession, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).parsed, true
	}
	return engine.ParsedSession{}, false
}

func (c *messageCache) put(key string, parsed engine.ParsedSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := &cacheEntry{key: key, parsed: parsed, weight: parsedSize(parsed)}
	if entry.weight > c.budget {
		return // larger than the whole cache: parse again on demand
	}
	if el, ok := c.items[key]; ok {
		c.used -= el.Value.(*cacheEntry).weight
		c.order.Remove(el)
	}
	c.items[key] = c.order.PushFront(entry)
	c.used += entry.weight
	for c.used > c.budget {
		last := c.order.Back()
		old := last.Value.(*cacheEntry)
		c.order.Remove(last)
		delete(c.items, old.key)
		c.used -= old.weight
	}
}

func (c *messageCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.items = map[string]*list.Element{}
	c.used = 0
}

// engineInput builds the engine input of a session.
func engineInput(sd *sessionData, actions []model.ActionEntry, actionIDs []string) engine.SessionInput {
	in := engine.SessionInput{Session: sd.dto, Actions: actions}
	in.Session.ActionIDs = actionIDs
	for i, dir := range model.Directions {
		ds := sd.stream(dir)
		in.Streams[i] = engine.StreamInput{
			Bytes:        sd.flow.Stream(dir).Data,
			Gaps:         ds.Gaps,
			Segments:     ds.Segments,
			UnknownStart: !ds.StartKnown,
		}
	}
	return in
}

// parse applies a spec to a session, using the cache.
func (p *project) parse(sd *sessionData, sp *spec.Spec, hash string) engine.ParsedSession {
	p.mu.Lock()
	key := sd.dto.ID + "|" + hash + "|" + strconv.FormatUint(sd.analysis, 10) + "|" + itoa(p.rev)
	p.mu.Unlock()
	if parsed, ok := p.cache.get(key); ok {
		return parsed
	}
	actions := p.linkedActions(sd.dto.ID)
	ids := make([]string, len(actions))
	for i, a := range actions {
		ids[i] = a.ID
	}
	in := engineInput(sd, actions, ids)
	// A framing that cuts a long stream into tiny messages would otherwise
	// build millions of them before any limit is checked.
	in.MaxMessages = p.app.cfg.Limits.MaxMessages
	parsed := engine.ParseSession(sp, in)
	p.cache.put(key, parsed)
	return parsed
}

// sessionMessageLimit is the error for a session with more messages than the limit.
func sessionMessageLimit(limit int) error {
	return &Error{Status: 422, Code: "message_limit", Message: fmt.Sprintf(
		"В сессии больше %d сообщений (предел). Проверьте кадрирование интерпретации: такие числа обычно означают неверную длину кадра", limit)}
}

// version returns an interpretation version of the project.
func (p *project) version(versionID string) (model.InterpretationVersion, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	i := findIndex(p.st.Versions, func(v *model.InterpretationVersion) bool { return v.ID == versionID })
	if i < 0 {
		return model.InterpretationVersion{}, badRequest("Версия интерпретации %q не найдена в проекте", versionID)
	}
	return p.st.Versions[i], nil
}

// SessionMessages frames and parses a session with an interpretation version.
func (a *App) SessionMessages(sessionID, versionID string) ([]model.Message, error) {
	if versionID == "" {
		return nil, badRequest("Укажите versionId интерпретации")
	}
	p, err := a.projectOfSession(sessionID)
	if err != nil {
		return nil, err
	}
	v, err := p.version(versionID)
	if err != nil {
		return nil, err
	}
	sd, err := p.session(sessionID)
	if err != nil {
		return nil, err
	}
	parsed := p.parse(sd, &v.Spec, v.Hash)
	if parsed.TooMany {
		return nil, sessionMessageLimit(a.cfg.Limits.MaxMessages)
	}
	return nonNil(parsed.Messages), nil
}

type MessagesQuery struct {
	VersionID  string
	RuleID     string
	Direction  string
	SessionIDs []string
}

// ProjectMessages collects messages across the project's target sessions (comparison view).
func (a *App) ProjectMessages(projectID string, q MessagesQuery) ([]model.Message, error) {
	if q.VersionID == "" {
		return nil, badRequest("Укажите versionId интерпретации")
	}
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	v, err := p.version(q.VersionID)
	if err != nil {
		return nil, err
	}
	sessions := p.allSessions()
	slices.SortStableFunc(sessions, func(x, y *sessionData) int {
		switch {
		case x.dto.StartedAt < y.dto.StartedAt:
			return -1
		case x.dto.StartedAt > y.dto.StartedAt:
			return 1
		}
		return 0
	})
	limit := a.cfg.Limits.MaxMessages
	out := []model.Message{}
	for _, sd := range sessions {
		if !sd.dto.IsTarget {
			continue
		}
		if len(q.SessionIDs) > 0 && !slices.Contains(q.SessionIDs, sd.dto.ID) {
			continue
		}
		parsed := p.parse(sd, &v.Spec, v.Hash)
		if parsed.TooMany {
			return nil, &Error{Status: 422, Code: "message_limit", Message: "Слишком много сообщений для сравнения: сузьте выборку (тип сообщения, направление или сессии)"}
		}
		for _, m := range parsed.Messages {
			if q.RuleID != "" && (m.RuleID == nil || *m.RuleID != q.RuleID) {
				continue
			}
			if q.Direction != "" && string(m.Direction) != q.Direction {
				continue
			}
			if len(out) >= limit {
				return nil, &Error{Status: 422, Code: "message_limit", Message: "Слишком много сообщений для сравнения: сузьте выборку (тип сообщения, направление или сессии)"}
			}
			out = append(out, m)
		}
	}
	return out, nil
}
