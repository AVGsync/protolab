package app

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/AVGsync/protolab/internal/demo"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
	"github.com/AVGsync/protolab/internal/store"
)

// CreateDemoProject builds the "ThermoLink" demo through the same code paths a
// user goes through: capture import, action log, interpretation versions,
// hypotheses, observations, corpora and verification runs.
func (a *App) CreateDemoProject(ctx context.Context) (model.Project, error) {
	hypIDs := map[string]string{}
	hypID := func(seed string) string {
		if id, ok := hypIDs[seed]; ok {
			return id
		}
		id := newID("hyp")
		hypIDs[seed] = id
		return id
	}
	ds := demo.Build(demo.Options{HypothesisID: hypID})

	pr, err := a.CreateProject(model.CreateProjectInput{Name: ds.ProjectName, Description: ds.ProjectDescription, TargetPorts: ds.TargetPorts})
	if err != nil {
		return model.Project{}, err
	}
	p, err := a.project(pr.ID)
	if err != nil {
		return model.Project{}, err
	}
	fail := func(step string, err error) (model.Project, error) {
		a.log.Error("demo project incomplete", "step", step, "err", err)
		return p.dto(), fmt.Errorf("демо-проект создан не полностью (%s): %w", step, err)
	}

	captureIDs := map[string]string{} // file name -> capture id
	for _, c := range ds.Captures {
		cp, err := a.importCaptureSync(ctx, p, c.FileName, bytes.NewReader(c.Data))
		if err != nil {
			return fail("импорт "+c.FileName, err)
		}
		captureIDs[c.FileName] = cp.ID
	}

	sessions := p.allSessions()
	var actions []model.ActionEntry
	for _, in := range ds.Actions {
		act, err := a.newAction(p, in, "import", sessions)
		if err != nil {
			return fail("журнал действий", err)
		}
		actions = append(actions, act)
	}
	if err := p.update(func(st *store.State) error {
		st.Actions = append(st.Actions, actions...)
		p.rev++
		return nil
	}); err != nil {
		return fail("журнал действий", err)
	}
	for _, act := range actions {
		a.index(p.id, act.ID)
	}

	var versions []model.InterpretationVersion
	for i, vs := range ds.Versions {
		s := vs.Spec
		if i == 0 {
			interp, err := a.addInterpretation(p, ds.InterpretationName, ds.InterpretationDescription, &s, vs.Comment, vs.CreatedAt)
			if err != nil {
				return fail("интерпретация", err)
			}
			v, _ := p.version(interp.LatestVersionID)
			versions = append(versions, v)
			continue
		}
		v, err := a.addVersion(p, versions[0].InterpretationID, &s, vs.Comment, vs.CreatedAt)
		if err != nil {
			return fail("версия интерпретации", err)
		}
		versions = append(versions, v)
	}

	for _, h := range ds.Hypotheses {
		_, err := a.addHypothesis(p, hypID(h.SeedID), model.CreateHypothesisInput{
			Title:        h.Title,
			Statement:    h.Statement,
			Alternatives: h.Alternatives,
			RuleRefs:     h.RuleRefs,
		}, h.Status, h.History, h.CreatedAt)
		if err != nil {
			return fail("гипотезы", err)
		}
	}

	latest := versions[len(versions)-1]
	for _, o := range ds.Observations {
		src, err := a.locateField(p, captureIDs, o.Ref, &latest.Spec, latest.Hash)
		if err != nil {
			return fail("наблюдения", err)
		}
		ids := make([]string, len(o.HypothesisIDs))
		for i, seed := range o.HypothesisIDs {
			ids[i] = hypID(seed)
		}
		if _, err := a.addObservation(p, model.CreateObservationInput{Note: o.Note, Source: src, HypothesisIDs: ids}, o.CreatedAt); err != nil {
			return fail("наблюдения", err)
		}
	}

	var corpora []model.Corpus
	for _, cs := range ds.Corpora {
		var ids []string
		for _, sd := range p.allSessions() {
			if !slices.ContainsFunc(cs.CaptureFiles, func(f string) bool { return captureIDs[f] == sd.dto.CaptureID }) {
				continue
			}
			if cs.OnlyTarget && !sd.dto.IsTarget {
				continue
			}
			ids = append(ids, sd.dto.ID)
		}
		filter := model.CorpusFilter{OnlyTarget: cs.OnlyTarget}
		for _, f := range cs.CaptureFiles {
			filter.CaptureIDs = append(filter.CaptureIDs, captureIDs[f])
		}
		c, err := a.addCorpus(p, model.CreateCorpusInput{Name: cs.Name, Description: cs.Description, SessionIDs: ids, Filter: filter}, "")
		if err != nil {
			return fail("корпус "+cs.Name, err)
		}
		corpora = append(corpora, c)
	}

	var runIDs []string
	for _, rs := range ds.Runs {
		rec, err := a.newRun(p, versions[rs.Version].ID, corpora[rs.Corpus].ID, model.Now())
		if err != nil {
			return fail("проверка", err)
		}
		if err := a.executeRun(ctx, p, rec.Run.ID, nil); err != nil {
			return fail("проверка", err)
		}
		runIDs = append(runIDs, rec.Run.ID)
	}

	for _, ce := range ds.Counterexamples {
		src, err := a.locateField(p, captureIDs, ce.Ref, &latest.Spec, latest.Hash)
		if err != nil {
			return fail("контрпримеры", err)
		}
		runID := ""
		if ce.Run >= 0 && ce.Run < len(runIDs) {
			runID = runIDs[ce.Run]
		}
		id := hypID(ce.HypothesisID)
		if err := p.update(func(st *store.State) error {
			i := findIndex(st.Hypotheses, func(h *model.Hypothesis) bool { return h.ID == id })
			if i < 0 {
				return notFound("Гипотеза")
			}
			st.Hypotheses[i].Counterexamples = append(st.Hypotheses[i].Counterexamples, model.Counterexample{Source: src, Note: ce.Note, RunID: runID})
			return nil
		}); err != nil {
			return fail("контрпримеры", err)
		}
	}

	a.log.Info("demo project created", "project", p.id)
	return p.dto(), nil
}

// locateField resolves a demo field reference to the exact source bytes.
func (a *App) locateField(p *project, captureIDs map[string]string, ref demo.FieldRef, sp *spec.Spec, hash string) (model.SourceRef, error) {
	capID := captureIDs[ref.CaptureFile]
	for _, sd := range p.allSessions() {
		if sd.dto.CaptureID != capID || sd.dto.Client.Port != ref.ClientPort {
			continue
		}
		for _, m := range p.parse(sd, sp, hash).Messages {
			if m.RuleID == nil || *m.RuleID != ref.Rule {
				continue
			}
			for _, f := range m.Fields {
				if f.Name != ref.Field {
					continue
				}
				off := m.Offset + f.Offset
				return model.SourceRef{
					CaptureID:     sd.dto.CaptureID,
					CaptureSHA256: sd.dto.Capture.SHA256,
					SessionID:     sd.dto.ID,
					Direction:     m.Direction,
					Offset:        off,
					Length:        f.Length,
					PacketNumbers: packetsInRange(sd.stream(m.Direction).Segments, off, f.Length),
				}, nil
			}
		}
	}
	return model.SourceRef{}, fmt.Errorf("поле %s.%s в %s (порт %d) не найдено", ref.Rule, ref.Field, ref.CaptureFile, ref.ClientPort)
}
