package app

import (
	"container/list"
	"slices"
	"strconv"
	"sync"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

func itoa(n int) string { return strconv.Itoa(n) }

// messageCache is a small LRU of parsed sessions keyed by session, spec hash,
// the analysis that produced the session and the project revision (actions
// and settings change the result).
type messageCache struct {
	mu    sync.Mutex
	cap   int
	order *list.List
	items map[string]*list.Element
}

type cacheEntry struct {
	key    string
	parsed engine.ParsedSession
}

func newMessageCache(capacity int) *messageCache {
	return &messageCache{cap: capacity, order: list.New(), items: map[string]*list.Element{}}
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
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).parsed = parsed
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&cacheEntry{key: key, parsed: parsed})
	for c.order.Len() > c.cap {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*cacheEntry).key)
	}
}

func (c *messageCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.order.Init()
	c.items = map[string]*list.Element{}
}

// engineInput builds the engine input of a session.
func engineInput(sd *sessionData, actions []model.ActionEntry, actionIDs []string) engine.SessionInput {
	in := engine.SessionInput{Session: sd.dto, Actions: actions}
	in.Session.ActionIDs = actionIDs
	for i, dir := range model.Directions {
		ds := sd.stream(dir)
		in.Streams[i] = engine.StreamInput{
			Bytes:    sd.flow.Stream(dir).Data,
			Gaps:     ds.Gaps,
			Segments: ds.Segments,
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
	parsed := engine.ParseSession(sp, engineInput(sd, actions, ids))
	p.cache.put(key, parsed)
	return parsed
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
	return nonNil(p.parse(sd, &v.Spec, v.Hash).Messages), nil
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
		for _, m := range p.parse(sd, &v.Spec, v.Hash).Messages {
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
