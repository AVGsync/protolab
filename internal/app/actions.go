package app

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

var actionKinds = []string{"read_params", "set_param", "get_measurement", "read_log", "other"}

// Automatic linking window: a session belongs to an action when it starts
// between 0.5 s before and 5 s after the action was recorded.
const (
	linkBefore = 500 * time.Millisecond
	linkAfter  = 5 * time.Second
)

func (a *App) ListActions(projectID string) ([]model.ActionEntry, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	out := slices.Clone(p.st.Actions)
	p.mu.Unlock()
	slices.SortStableFunc(out, func(x, y model.ActionEntry) int { return strings.Compare(x.Timestamp, y.Timestamp) })
	return nonNil(out), nil
}

func parseActionTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999Z07:00", "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05.999999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("некорректное время %q (ожидается RFC 3339, например 2026-09-14T10:01:10Z)", s)
}

// autoLink finds the target session that started closest after the action.
func autoLink(sessions []*sessionData, at time.Time) []model.ActionLink {
	var best *sessionData
	var bestDelta time.Duration
	for _, sd := range sessions {
		if !sd.dto.IsTarget {
			continue
		}
		delta := sd.flow.Start.Sub(at)
		if delta < -linkBefore || delta > linkAfter {
			continue
		}
		if best == nil || absDur(delta) < absDur(bestDelta) {
			best, bestDelta = sd, delta
		}
	}
	if best == nil {
		return []model.ActionLink{}
	}
	ms := bestDelta.Milliseconds()
	return []model.ActionLink{{SessionID: best.dto.ID, Method: "auto_time", DeltaMs: &ms}}
}

// linkActions links actions that have no session yet to the sessions of a
// newly indexed capture, by the same time window: an action log is often
// imported before the capture of the same experiment.
func linkActions(actions []model.ActionEntry, sessions []*sessionData) {
	for i := range actions {
		act := &actions[i]
		if len(act.Links) > 0 {
			continue
		}
		at, err := parseActionTime(act.Timestamp)
		if err != nil {
			continue
		}
		act.Links = autoLink(sessions, at)
	}
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func normalizeKind(k string) string {
	k = strings.TrimSpace(k)
	if slices.Contains(actionKinds, k) {
		return k
	}
	return "other"
}

// sanitizeValues keeps only JSON numbers and strings.
func sanitizeValues(in map[string]any) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range in {
		switch x := v.(type) {
		case float64:
			if math.IsNaN(x) || math.IsInf(x, 0) {
				return nil, fmt.Errorf("параметр %q: некорректное число", k)
			}
			out[k] = x
		case string:
			out[k] = x
		case json.Number:
			f, err := x.Float64()
			if err != nil {
				return nil, fmt.Errorf("параметр %q: %v", k, err)
			}
			out[k] = f
		case int:
			out[k] = float64(x)
		default:
			return nil, fmt.Errorf("параметр %q: допускаются числа и строки", k)
		}
	}
	return out, nil
}

func (a *App) newAction(p *project, in model.CreateActionInput, source string, sessions []*sessionData) (model.ActionEntry, error) {
	label := strings.TrimSpace(in.Label)
	if label == "" {
		return model.ActionEntry{}, invalid("Укажите описание действия")
	}
	at, err := parseActionTime(in.Timestamp)
	if err != nil {
		return model.ActionEntry{}, invalid("%v", err)
	}
	params, err := sanitizeValues(in.Params)
	if err != nil {
		return model.ActionEntry{}, invalid("%v", err)
	}
	var observed map[string]any
	if len(in.Observed) > 0 {
		if observed, err = sanitizeValues(in.Observed); err != nil {
			return model.ActionEntry{}, invalid("%v", err)
		}
	}
	act := model.ActionEntry{
		ID:        newID("act"),
		ProjectID: p.id,
		Timestamp: model.PacketTime(at),
		Kind:      normalizeKind(in.Kind),
		Label:     label,
		Params:    params,
		Observed:  observed,
		Result:    strings.TrimSpace(in.Result),
		Source:    source,
	}
	if len(in.SessionIDs) > 0 {
		known := map[string]bool{}
		for _, sd := range sessions {
			known[sd.dto.ID] = true
		}
		for _, id := range in.SessionIDs {
			if !known[id] {
				return model.ActionEntry{}, invalid("Неизвестная сессия %s", id)
			}
			act.Links = append(act.Links, model.ActionLink{SessionID: id, Method: "manual"})
		}
	} else {
		act.Links = autoLink(sessions, at)
	}
	return act, nil
}

func (a *App) CreateAction(projectID string, in model.CreateActionInput) (model.ActionEntry, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.ActionEntry{}, err
	}
	act, err := a.newAction(p, in, "manual", p.allSessions())
	if err != nil {
		return model.ActionEntry{}, err
	}
	err = p.update(func(st *store.State) error {
		st.Actions = append(st.Actions, act)
		p.rev++
		return nil
	})
	if err != nil {
		return model.ActionEntry{}, err
	}
	a.index(p.id, act.ID)
	return act, nil
}

// maxActionLogBytes bounds an imported action log.
const maxActionLogBytes = 16 << 20

// ImportActions reads a CSV (timestamp,kind,label,params,result[,observed]) or
// JSON (array of CreateActionInput) action log and links it to sessions by time.
func (a *App) ImportActions(projectID, fileName string, r io.Reader) (model.ImportActionsResult, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.ImportActionsResult{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, maxActionLogBytes+1))
	if err != nil {
		return model.ImportActionsResult{}, badRequest("Не удалось прочитать файл: %v", err)
	}
	if len(raw) > maxActionLogBytes {
		// Cut at the limit, the last record would be imported truncated.
		return model.ImportActionsResult{}, &Error{Status: http.StatusRequestEntityTooLarge, Code: "action_log_too_large",
			Message: fmt.Sprintf("Журнал действий больше %d МиБ", maxActionLogBytes>>20)}
	}
	var inputs []model.CreateActionInput
	trimmed := bytes.TrimSpace(raw)
	if strings.HasSuffix(strings.ToLower(fileName), ".json") || bytes.HasPrefix(trimmed, []byte("[")) {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		if err := dec.Decode(&inputs); err != nil {
			return model.ImportActionsResult{}, &Error{Status: 422, Code: "invalid_action_log", Message: "JSON журнала: " + err.Error()}
		}
	} else if inputs, err = parseActionCSV(trimmed); err != nil {
		return model.ImportActionsResult{}, &Error{Status: 422, Code: "invalid_action_log", Message: err.Error()}
	}

	sessions := p.allSessions()
	res := model.ImportActionsResult{Warnings: []string{}}
	var created []model.ActionEntry
	for i, in := range inputs {
		in.SessionIDs = nil // imported logs are always linked by time
		act, err := a.newAction(p, in, "import", sessions)
		if err != nil {
			e, _ := AsError(err)
			msg := err.Error()
			if e != nil {
				msg = e.Message
			}
			return model.ImportActionsResult{}, &Error{Status: 422, Code: "invalid_action_log", Message: fmt.Sprintf("Запись %d: %s", i+1, msg)}
		}
		if len(act.Links) > 0 {
			res.Linked++
		} else {
			res.Warnings = append(res.Warnings, fmt.Sprintf("«%s» (%s): целевая сессия в окне −0,5…+5 с не найдена", act.Label, act.Timestamp))
		}
		created = append(created, act)
	}
	err = p.update(func(st *store.State) error {
		st.Actions = append(st.Actions, created...)
		p.rev++
		return nil
	})
	if err != nil {
		return model.ImportActionsResult{}, err
	}
	for _, act := range created {
		a.index(p.id, act.ID)
	}
	res.Imported = len(created)
	return res, nil
}

// parseActionCSV: header row optional; params/observed as "k=v;k=v" or a JSON object.
func parseActionCSV(data []byte) ([]model.CreateActionInput, error) {
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV журнала: %v", err)
	}
	var out []model.CreateActionInput
	for i, row := range rows {
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		if i == 0 && strings.EqualFold(strings.TrimSpace(row[0]), "timestamp") {
			continue
		}
		if len(row) < 3 {
			return nil, fmt.Errorf("строка %d: нужны как минимум timestamp,kind,label", i+1)
		}
		cell := func(n int) string {
			if n < len(row) {
				return strings.TrimSpace(row[n])
			}
			return ""
		}
		params, err := parseKV(cell(3))
		if err != nil {
			return nil, fmt.Errorf("строка %d: params: %v", i+1, err)
		}
		observed, err := parseKV(cell(5))
		if err != nil {
			return nil, fmt.Errorf("строка %d: observed: %v", i+1, err)
		}
		label := cell(2)
		if label == "" {
			label = cell(1)
		}
		out = append(out, model.CreateActionInput{
			Timestamp: cell(0),
			Kind:      cell(1),
			Label:     label,
			Params:    params,
			Result:    cell(4),
			Observed:  observed,
		})
	}
	return out, nil
}

func parseKV(s string) (map[string]any, error) {
	out := map[string]any{}
	if s == "" {
		return out, nil
	}
	if strings.HasPrefix(s, "{") {
		if err := json.Unmarshal([]byte(s), &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	for _, pair := range strings.Split(s, ";") {
		k, v, _ := strings.Cut(pair, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" {
			continue
		}
		if f, err := strconv.ParseFloat(v, 64); err == nil && v != "" {
			out[k] = f
		} else {
			out[k] = v
		}
	}
	return out, nil
}
