package app

import (
	"sort"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
)

// SessionFilter narrows ListSessions. Target nil means any.
type SessionFilter struct {
	CaptureID string
	Status    string
	Target    *bool
	Query     string
}

// allSessions loads the analyses of every ready capture of the project.
// Captures that failed to analyse are skipped: one broken file must not hide the rest.
func (p *project) allSessions() []*sessionData {
	var out []*sessionData
	for _, id := range p.captureIDs() {
		res, err := p.analysis(id)
		if err != nil {
			continue
		}
		out = append(out, res.sessions...)
	}
	return out
}

// session finds a session by id.
func (p *project) session(id string) (*sessionData, error) {
	capID, ok := captureOfSession(id)
	if !ok {
		return nil, notFound("Сессия")
	}
	res, err := p.analysis(capID)
	if err != nil {
		if e, ok := AsError(err); ok {
			return nil, e
		}
		return nil, &Error{Status: 500, Code: "analysis_failed", Message: err.Error()}
	}
	sd := res.byID[id]
	if sd == nil {
		return nil, notFound("Сессия")
	}
	return sd, nil
}

// actionIndex maps session ids to the ids of actions linked to them.
func (p *project) actionIndex() map[string][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := map[string][]string{}
	for _, act := range p.st.Actions {
		for _, l := range act.Links {
			idx[l.SessionID] = append(idx[l.SessionID], act.ID)
		}
	}
	return idx
}

// linkedActions returns the actions linked to a session.
func (p *project) linkedActions(sessionID string) []model.ActionEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []model.ActionEntry
	for _, act := range p.st.Actions {
		for _, l := range act.Links {
			if l.SessionID == sessionID {
				out = append(out, act)
				break
			}
		}
	}
	return out
}

func withActions(s model.TcpSession, idx map[string][]string) model.TcpSession {
	s.ActionIDs = append([]string{}, idx[s.ID]...)
	return s
}

func (a *App) ListSessions(projectID string, f SessionFilter) ([]model.TcpSession, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	idx := p.actionIndex()
	q := strings.ToLower(strings.TrimSpace(f.Query))
	var out []model.TcpSession
	for _, sd := range p.allSessions() {
		s := sd.dto
		if f.CaptureID != "" && s.CaptureID != f.CaptureID {
			continue
		}
		if f.Status != "" && s.Status != f.Status {
			continue
		}
		if f.Target != nil && s.IsTarget != *f.Target {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(sessionHaystack(s)), q) {
			continue
		}
		out = append(out, withActions(s, idx))
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out, nil
}

func sessionHaystack(s model.TcpSession) string {
	return strings.Join([]string{
		s.ID,
		s.Client.IP + ":" + itoa(s.Client.Port),
		s.Server.IP + ":" + itoa(s.Server.Port),
		s.Capture.FileName,
	}, " ")
}

func (a *App) GetSession(id string) (model.TcpSession, error) {
	p, err := a.projectOfSession(id)
	if err != nil {
		return model.TcpSession{}, err
	}
	sd, err := p.session(id)
	if err != nil {
		return model.TcpSession{}, err
	}
	return withActions(sd.dto, p.actionIndex()), nil
}

func (a *App) Stream(id string, dir model.Direction) (model.DirectedStream, error) {
	if !dir.Valid() {
		return model.DirectedStream{}, badRequest("Направление: c2s или s2c")
	}
	p, err := a.projectOfSession(id)
	if err != nil {
		return model.DirectedStream{}, err
	}
	sd, err := p.session(id)
	if err != nil {
		return model.DirectedStream{}, err
	}
	return *sd.stream(dir), nil
}

// StreamBytes returns a range of the reassembled bytes (whole stream when length < 0).
// Gap bytes are zero in transport only; DirectedStream.Gaps marks them.
func (a *App) StreamBytes(id string, dir model.Direction, offset, length int) ([]byte, int, error) {
	if !dir.Valid() {
		return nil, 0, badRequest("Направление: c2s или s2c")
	}
	p, err := a.projectOfSession(id)
	if err != nil {
		return nil, 0, err
	}
	sd, err := p.session(id)
	if err != nil {
		return nil, 0, err
	}
	data := sd.flow.Stream(dir).Data
	total := len(data)
	if offset < 0 || offset > total {
		return nil, total, badRequest("offset вне потока (0–%d)", total)
	}
	end := total
	if length >= 0 && length < total-offset { // not offset+length: it may overflow
		end = offset + length
	}
	return data[offset:end], total, nil
}

func (a *App) Packets(id string) ([]model.Packet, error) {
	p, err := a.projectOfSession(id)
	if err != nil {
		return nil, err
	}
	sd, err := p.session(id)
	if err != nil {
		return nil, err
	}
	return sd.packets, nil
}
