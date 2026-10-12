package app

import (
	"slices"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

var hypothesisStatuses = []string{"proposed", "testing", "supported", "refuted"}

func (a *App) ListHypotheses(projectID string) ([]model.Hypothesis, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return nonNil(slices.Clone(p.st.Hypotheses)), nil
}

func (a *App) CreateHypothesis(projectID string, in model.CreateHypothesisInput) (model.Hypothesis, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.Hypothesis{}, err
	}
	return a.addHypothesis(p, newID("hyp"), in, "proposed", nil, "")
}

func (a *App) addHypothesis(p *project, id string, in model.CreateHypothesisInput, status string, history []model.HypothesisEvent, createdAt string) (model.Hypothesis, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return model.Hypothesis{}, invalid("Сформулируйте гипотезу")
	}
	if createdAt == "" {
		createdAt = model.Now()
	}
	if history == nil {
		history = []model.HypothesisEvent{{At: createdAt, Status: status, Note: "Создана"}}
	}
	h := model.Hypothesis{
		ID:              id,
		ProjectID:       p.id,
		Title:           title,
		Statement:       strings.TrimSpace(in.Statement),
		Status:          status,
		ObservationIDs:  nonNil(in.ObservationIDs),
		Counterexamples: []model.Counterexample{},
		Alternatives:    nonNil(in.Alternatives),
		RuleRefs:        nonNil(in.RuleRefs),
		CreatedAt:       createdAt,
		UpdatedAt:       history[len(history)-1].At,
		History:         history,
	}
	err := p.update(func(st *store.State) error {
		for _, oid := range h.ObservationIDs {
			i := findIndex(st.Observations, func(o *model.Observation) bool { return o.ID == oid })
			if i < 0 {
				return invalid("Наблюдение %s не найдено", oid)
			}
			if !slices.Contains(st.Observations[i].HypothesisIDs, h.ID) {
				st.Observations[i].HypothesisIDs = append(st.Observations[i].HypothesisIDs, h.ID)
			}
		}
		st.Hypotheses = append(st.Hypotheses, h)
		return nil
	})
	if err != nil {
		return model.Hypothesis{}, err
	}
	a.index(p.id, h.ID)
	return h, nil
}

// UpdateHypothesis changes fields; a status change requires a note and is kept in the history.
// Everything is validated before the first change, so a rejected request changes nothing.
func (a *App) UpdateHypothesis(id string, in model.UpdateHypothesisInput) (model.Hypothesis, error) {
	p, err := a.projectOf(id, "Гипотеза")
	if err != nil {
		return model.Hypothesis{}, err
	}
	var out model.Hypothesis
	err = p.update(func(st *store.State) error {
		i := findIndex(st.Hypotheses, func(h *model.Hypothesis) bool { return h.ID == id })
		if i < 0 {
			return notFound("Гипотеза")
		}
		h := &st.Hypotheses[i]
		statusChange := in.Status != nil && *in.Status != h.Status
		note := strings.TrimSpace(in.Note)
		if statusChange {
			if !slices.Contains(hypothesisStatuses, *in.Status) {
				return invalid("Неизвестный статус %q", *in.Status)
			}
			if note == "" {
				return invalid("Укажите основание смены статуса")
			}
		}
		if in.Title != nil && strings.TrimSpace(*in.Title) == "" {
			return invalid("Формулировка не может быть пустой")
		}
		var obsIDs []string
		if in.ObservationIDs != nil {
			obsIDs = []string{}
			for _, oid := range in.ObservationIDs {
				if slices.Contains(obsIDs, oid) {
					continue
				}
				if findIndex(st.Observations, func(o *model.Observation) bool { return o.ID == oid }) < 0 {
					return invalid("Наблюдение %s не найдено", oid)
				}
				obsIDs = append(obsIDs, oid)
			}
		}

		now := model.Now()
		if statusChange {
			h.History = append(h.History, model.HypothesisEvent{At: now, Status: *in.Status, Note: note})
			h.Status = *in.Status
		}
		if in.Title != nil {
			h.Title = strings.TrimSpace(*in.Title)
		}
		if in.Statement != nil {
			h.Statement = strings.TrimSpace(*in.Statement)
		}
		if in.Alternatives != nil {
			h.Alternatives = in.Alternatives
		}
		if obsIDs != nil {
			// The link is kept on both sides: observations?hypothesisId= reads the observation side.
			for j := range st.Observations {
				o := &st.Observations[j]
				linked, has := slices.Contains(obsIDs, o.ID), slices.Contains(o.HypothesisIDs, h.ID)
				switch {
				case linked && !has:
					o.HypothesisIDs = append(o.HypothesisIDs, h.ID)
				case !linked && has:
					o.HypothesisIDs = slices.DeleteFunc(o.HypothesisIDs, func(x string) bool { return x == h.ID })
				}
			}
			h.ObservationIDs = obsIDs
		}
		if in.Counterexamples != nil {
			h.Counterexamples = in.Counterexamples
		}
		h.UpdatedAt = now
		out = *h
		return nil
	})
	return out, err
}

// ObservationFilter narrows ListObservations.
type ObservationFilter struct {
	HypothesisID string
	SessionID    string
}

func (a *App) ListObservations(projectID string, f ObservationFilter) ([]model.Observation, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	list := slices.Clone(p.st.Observations)
	p.mu.Unlock()

	out := make([]model.Observation, 0, len(list))
	for _, o := range list {
		if f.HypothesisID != "" && !slices.Contains(o.HypothesisIDs, f.HypothesisID) {
			continue
		}
		if f.SessionID != "" && o.Source.SessionID != f.SessionID {
			continue
		}
		o.Integrity = "ok"
		if snap, err := p.snapshot(o.Source); err != nil || snap != o.SnapshotHex {
			o.Integrity = "mismatch"
		}
		out = append(out, o)
	}
	slices.SortStableFunc(out, func(x, y model.Observation) int { return strings.Compare(y.CreatedAt, x.CreatedAt) })
	return out, nil
}

// snapshot renders the referenced bytes as hex; unknown (gap) bytes are "??".
func (p *project) snapshot(src model.SourceRef) (string, error) {
	sd, err := p.session(src.SessionID)
	if err != nil {
		return "", err
	}
	if !src.Direction.Valid() {
		return "", badRequest("Направление: c2s или s2c")
	}
	data := sd.flow.Stream(src.Direction).Data
	if src.Offset < 0 || src.Length <= 0 || src.Offset > len(data)-src.Length { // not Offset+Length: it may overflow
		return "", invalid("Диапазон %d+%d вне потока (%d Б)", src.Offset, src.Length, len(data))
	}
	gaps := sd.stream(src.Direction).Gaps
	var b strings.Builder
	for i := src.Offset; i < src.Offset+src.Length; i++ {
		if i > src.Offset {
			b.WriteByte(' ')
		}
		if inGaps(gaps, i) {
			b.WriteString("??")
			continue
		}
		const digits = "0123456789ABCDEF"
		b.WriteByte(digits[data[i]>>4])
		b.WriteByte(digits[data[i]&0x0f])
	}
	return b.String(), nil
}

func inGaps(gaps []model.StreamGap, off int) bool {
	for _, g := range gaps {
		if off >= g.Offset && off < g.Offset+g.Length {
			return true
		}
	}
	return false
}

// maxObservationBytes bounds the byte snapshot stored with an observation.
const maxObservationBytes = 64 << 10

func (a *App) CreateObservation(projectID string, in model.CreateObservationInput) (model.Observation, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.Observation{}, err
	}
	return a.addObservation(p, in, "")
}

func (a *App) addObservation(p *project, in model.CreateObservationInput, createdAt string) (model.Observation, error) {
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return model.Observation{}, invalid("Опишите наблюдение")
	}
	if in.Source.Length > maxObservationBytes {
		return model.Observation{}, invalid("Наблюдение охватывает не больше %d байт: снимок байтов хранится в проекте", maxObservationBytes)
	}
	sd, err := p.session(in.Source.SessionID)
	if err != nil {
		return model.Observation{}, err
	}
	if sd.dto.Capture.SHA256 != in.Source.CaptureSHA256 || sd.dto.CaptureID != in.Source.CaptureID {
		return model.Observation{}, conflict("source_mismatch", "Запись источника не совпадает с записью сессии (SHA-256)")
	}
	snap, err := p.snapshot(in.Source)
	if err != nil {
		return model.Observation{}, err
	}
	if createdAt == "" {
		createdAt = model.Now()
	}
	src := in.Source
	src.PacketNumbers = packetsInRange(sd.stream(src.Direction).Segments, src.Offset, src.Length)
	o := model.Observation{
		ID:            newID("obs"),
		ProjectID:     p.id,
		Note:          note,
		Source:        src,
		SnapshotHex:   snap,
		Integrity:     "ok",
		HypothesisIDs: nonNil(in.HypothesisIDs),
		Tags:          nonNil(in.Tags),
		CreatedAt:     createdAt,
	}
	err = p.update(func(st *store.State) error {
		for _, hid := range o.HypothesisIDs {
			i := findIndex(st.Hypotheses, func(h *model.Hypothesis) bool { return h.ID == hid })
			if i < 0 {
				return invalid("Гипотеза %s не найдена", hid)
			}
			if !slices.Contains(st.Hypotheses[i].ObservationIDs, o.ID) {
				st.Hypotheses[i].ObservationIDs = append(st.Hypotheses[i].ObservationIDs, o.ID)
			}
		}
		st.Observations = append(st.Observations, o)
		return nil
	})
	if err != nil {
		return model.Observation{}, err
	}
	a.index(p.id, o.ID)
	return o, nil
}

func packetsInRange(segments []model.StreamSegment, offset, length int) []int {
	out := []int{}
	for _, s := range segments {
		if s.Offset < offset+length && offset < s.Offset+s.Length {
			out = append(out, s.PacketNumber)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func (a *App) DeleteObservation(id string) error {
	p, err := a.projectOf(id, "Наблюдение")
	if err != nil {
		return err
	}
	return p.update(func(st *store.State) error {
		i := findIndex(st.Observations, func(o *model.Observation) bool { return o.ID == id })
		if i < 0 {
			return notFound("Наблюдение")
		}
		st.Observations = slices.Delete(st.Observations, i, i+1)
		for j := range st.Hypotheses {
			st.Hypotheses[j].ObservationIDs = slices.DeleteFunc(st.Hypotheses[j].ObservationIDs, func(x string) bool { return x == id })
		}
		return nil
	})
}
