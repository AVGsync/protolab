package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/jobs"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

// corpusHash covers the session ids and the hashes of their captures:
// the same ids over different capture bytes give a different hash.
func corpusHash(sessionIDs []string, shaOf func(string) string) string {
	ids := slices.Clone(sessionIDs)
	slices.Sort(ids)
	h := sha256.New()
	for _, id := range ids {
		fmt.Fprintf(h, "%s:%s\n", id, shaOf(id))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (p *project) captureSHA(sessionID string) string {
	capID, _ := captureOfSession(sessionID)
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == capID }); i >= 0 {
		return p.st.Captures[i].SHA256
	}
	return ""
}

func (a *App) ListCorpora(projectID string) ([]model.Corpus, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return nonNil(slices.Clone(p.st.Corpora)), nil
}

func (a *App) CreateCorpus(projectID string, in model.CreateCorpusInput) (model.Corpus, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.Corpus{}, err
	}
	return a.addCorpus(p, in, "")
}

func (a *App) addCorpus(p *project, in model.CreateCorpusInput, createdAt string) (model.Corpus, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return model.Corpus{}, invalid("Укажите название корпуса")
	}
	ids := slices.Compact(slices.Sorted(slices.Values(in.SessionIDs)))
	if len(ids) == 0 {
		return model.Corpus{}, invalid("Корпус должен содержать хотя бы одну сессию")
	}
	for _, id := range ids {
		if _, err := p.session(id); err != nil {
			return model.Corpus{}, invalid("Неизвестная сессия %s", id)
		}
	}
	if createdAt == "" {
		createdAt = model.Now()
	}
	c := model.Corpus{
		ID:          newID("cor"),
		ProjectID:   p.id,
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		SessionIDs:  ids,
		Filter:      in.Filter,
		Hash:        corpusHash(ids, p.captureSHA),
		CreatedAt:   createdAt,
	}
	if err := p.update(func(st *store.State) error { st.Corpora = append(st.Corpora, c); return nil }); err != nil {
		return model.Corpus{}, err
	}
	a.index(p.id, c.ID)
	return c, nil
}

// runDTO adds staleness: a result is stale when it no longer reflects the
// current interpretation, corpus or reassembly settings. The caller holds p.mu.
func (p *project) runDTO(rec store.RunRecord) model.VerificationRun {
	r := rec.Run
	reasons := []string{}
	if i := findIndex(p.st.Interpretations, func(x *model.Interpretation) bool { return x.ID == r.InterpretationID }); i >= 0 {
		if latest := p.st.Interpretations[i].LatestVersion; latest > r.Version {
			reasons = append(reasons, fmt.Sprintf("Проверка выполнена на v%d, актуальная версия интерпретации — v%d", r.Version, latest))
		}
	}
	if i := findIndex(p.st.Corpora, func(x *model.Corpus) bool { return x.ID == r.CorpusID }); i >= 0 && p.st.Corpora[i].Hash != r.CorpusHash {
		reasons = append(reasons, "Состав корпуса изменился после проверки")
	}
	s := p.st.Project.Settings
	if s.OverlapPolicy != r.Settings.OverlapPolicy || s.ChecksumPolicy != r.Settings.ChecksumPolicy {
		reasons = append(reasons, "Изменены настройки восстановления потоков проекта")
	}
	r.Stale = r.Status == "succeeded" && len(reasons) > 0
	r.StaleReasons = reasons
	return r
}

func (a *App) ListRuns(projectID string) ([]model.VerificationRun, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// Newest first; runs started in the same millisecond, latest created first.
	out := make([]model.VerificationRun, 0, len(p.st.Runs))
	for _, rec := range slices.Backward(p.st.Runs) {
		out = append(out, p.runDTO(rec))
	}
	slices.SortStableFunc(out, func(x, y model.VerificationRun) int { return strings.Compare(y.StartedAt, x.StartedAt) })
	return out, nil
}

func (a *App) GetRun(id string) (model.VerificationRun, error) {
	p, err := a.projectOf(id, "Проверка")
	if err != nil {
		return model.VerificationRun{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	i := findIndex(p.st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == id })
	if i < 0 {
		return model.VerificationRun{}, notFound("Проверка")
	}
	return p.runDTO(p.st.Runs[i]), nil
}

func (a *App) ListFindings(runID, kind string) ([]model.Finding, error) {
	p, err := a.projectOf(runID, "Проверка")
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	i := findIndex(p.st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == runID })
	if i < 0 {
		return nil, notFound("Проверка")
	}
	out := []model.Finding{}
	for _, f := range p.st.Runs[i].Findings {
		if kind == "" || f.Kind == kind {
			out = append(out, f)
		}
	}
	return out, nil
}

// StartRun verifies a version on a corpus as a background job.
func (a *App) StartRun(projectID string, in model.StartRunInput) (model.StartRunResult, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.StartRunResult{}, err
	}
	rec, err := a.newRun(p, in.VersionID, in.CorpusID, model.Now())
	if err != nil {
		return model.StartRunResult{}, err
	}
	runID := rec.Run.ID
	title := fmt.Sprintf("Проверка %s v%d на «%s»", rec.Run.InterpretationName, rec.Run.Version, rec.Run.CorpusName)
	job := a.jobs.Start(p.id, "verification", title, func(ctx context.Context, progress jobs.Progress) (*model.JobResult, error) {
		if err := a.executeRun(ctx, p, runID, progress); err != nil {
			return nil, err
		}
		return &model.JobResult{Type: "run", ID: runID}, nil
	})
	var out model.VerificationRun
	_ = p.update(func(st *store.State) error {
		if i := findIndex(st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == runID }); i >= 0 {
			if st.Runs[i].Run.JobID == "" {
				st.Runs[i].Run.JobID = job.ID
			}
			out = p.runDTO(st.Runs[i])
		}
		return nil
	})
	return model.StartRunResult{Run: out, JobID: job.ID}, nil
}

// newRun registers a queued run.
func (a *App) newRun(p *project, versionID, corpusID, startedAt string) (store.RunRecord, error) {
	var rec store.RunRecord
	err := p.update(func(st *store.State) error {
		vi := findIndex(st.Versions, func(v *model.InterpretationVersion) bool { return v.ID == versionID })
		if vi < 0 {
			return notFound("Версия интерпретации")
		}
		ci := findIndex(st.Corpora, func(c *model.Corpus) bool { return c.ID == corpusID })
		if ci < 0 {
			return notFound("Корпус")
		}
		v, c := st.Versions[vi], st.Corpora[ci]
		ii := findIndex(st.Interpretations, func(x *model.Interpretation) bool { return x.ID == v.InterpretationID })
		if ii < 0 {
			return notFound("Интерпретация")
		}
		rec = store.RunRecord{
			Run: model.VerificationRun{
				ID:                 newID("run"),
				ProjectID:          p.id,
				InterpretationID:   v.InterpretationID,
				InterpretationName: st.Interpretations[ii].Name,
				VersionID:          v.ID,
				Version:            v.Version,
				VersionHash:        v.Hash,
				CorpusID:           c.ID,
				CorpusName:         c.Name,
				CorpusHash:         c.Hash,
				Status:             "running",
				StartedAt:          startedAt,
				StaleReasons:       []string{},
				Settings: model.RunSettings{
					OverlapPolicy:  st.Project.Settings.OverlapPolicy,
					ChecksumPolicy: st.Project.Settings.ChecksumPolicy,
				},
				EngineVersion: engine.Version,
			},
			Findings: []model.Finding{},
		}
		st.Runs = append(st.Runs, rec)
		return nil
	})
	if err == nil {
		a.index(p.id, rec.Run.ID)
	}
	return rec, err
}

// executeRun applies the version to every session of the corpus and stores the result.
func (a *App) executeRun(ctx context.Context, p *project, runID string, progress jobs.Progress) error {
	if progress == nil {
		progress = func(float64, string) {}
	}
	p.mu.Lock()
	i := findIndex(p.st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == runID })
	if i < 0 {
		p.mu.Unlock()
		return notFound("Проверка")
	}
	run := p.st.Runs[i].Run
	vi := findIndex(p.st.Versions, func(v *model.InterpretationVersion) bool { return v.ID == run.VersionID })
	ci := findIndex(p.st.Corpora, func(c *model.Corpus) bool { return c.ID == run.CorpusID })
	if vi < 0 || ci < 0 {
		p.mu.Unlock()
		return a.failRun(p, runID, "версия или корпус удалены")
	}
	version := p.st.Versions[vi]
	sessionIDs := slices.Clone(p.st.Corpora[ci].SessionIDs)
	p.mu.Unlock()

	started := time.Now()
	progress(0, "Загрузка корпуса")
	inputs := make([]engine.SessionInput, 0, len(sessionIDs))
	var missing []string
	for _, id := range sessionIDs {
		if ctx.Err() != nil {
			return a.cancelRun(p, runID)
		}
		sd, err := p.session(id)
		if err != nil {
			missing = append(missing, id)
			continue
		}
		actions := p.linkedActions(id)
		ids := make([]string, len(actions))
		for k, act := range actions {
			ids[k] = act.ID
		}
		inputs = append(inputs, engineInput(sd, actions, ids))
	}

	result, err := engine.Verify(ctx, &version.Spec, inputs, func(done, total int) {
		progress(0.1+0.85*float64(done)/float64(max(total, 1)), "Применение правил")
	})
	if ctx.Err() != nil {
		return a.cancelRun(p, runID)
	}
	if err != nil {
		return a.failRun(p, runID, err.Error())
	}
	for _, id := range missing {
		// Sessions that disappeared (a capture failed to load) are part of the result, not skipped silently.
		result.Findings = append(result.Findings, model.Finding{
			Kind:        model.FindingCaptureDefect,
			Severity:    "error",
			Message:     fmt.Sprintf("Сессия корпуса %s недоступна: исходная запись не загружена или изменена", id),
			Occurrences: 1,
			Examples:    []model.SourceRef{},
		})
		result.Summary.Sessions++
		result.Summary.SessionsWithErrors++
	}
	progress(0.97, "Сводка")

	elapsed := time.Since(started).Milliseconds()
	return p.update(func(st *store.State) error {
		i := findIndex(st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == runID })
		if i < 0 {
			return notFound("Проверка")
		}
		rec := &st.Runs[i]
		for k := range result.Findings {
			result.Findings[k].ID = fmt.Sprintf("%s-f%d", runID, k+1)
			result.Findings[k].RunID = runID
			if result.Findings[k].Examples == nil {
				result.Findings[k].Examples = []model.SourceRef{}
			}
		}
		summary := result.Summary
		rec.Findings = result.Findings
		rec.Run.Summary = &summary
		rec.Run.Status = "succeeded"
		rec.Run.FinishedAt = model.Now()
		rec.Run.DurationMs = &elapsed
		rec.Run.PreviousRunID = previousRun(st.Runs, rec.Run)
		return nil
	})
}

// previousRun is the latest earlier successful run of the same interpretation
// on the same corpus. Runs are ordered by start time (millisecond precision)
// and, between runs started in the same millisecond, by creation: runs are
// appended to the project in the order they are created.
func previousRun(runs []store.RunRecord, run model.VerificationRun) string {
	self := slices.IndexFunc(runs, func(r store.RunRecord) bool { return r.Run.ID == run.ID })
	if self < 0 { // not stored yet: it is the latest created
		runs = append(slices.Clip(runs), store.RunRecord{Run: run})
		self = len(runs) - 1
	}
	before := func(i, j int) bool { // run i started before run j
		if a, b := runs[i].Run.StartedAt, runs[j].Run.StartedAt; a != b {
			return a < b
		}
		return i < j
	}
	best := -1
	for i, r := range runs {
		x := r.Run
		if i == self || x.Status != "succeeded" || x.InterpretationID != run.InterpretationID || x.CorpusID != run.CorpusID {
			continue
		}
		if before(i, self) && (best < 0 || before(best, i)) {
			best = i
		}
	}
	if best < 0 {
		return ""
	}
	return runs[best].Run.ID
}

func (a *App) failRun(p *project, runID, msg string) error {
	_ = p.update(func(st *store.State) error {
		if i := findIndex(st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == runID }); i >= 0 {
			st.Runs[i].Run.Status = "failed"
			st.Runs[i].Run.Error = msg
			st.Runs[i].Run.FinishedAt = model.Now()
		}
		return nil
	})
	return fmt.Errorf("проверка не выполнена: %s", msg)
}

func (a *App) cancelRun(p *project, runID string) error {
	_ = p.update(func(st *store.State) error {
		if i := findIndex(st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == runID }); i >= 0 {
			st.Runs[i].Run.Status = "cancelled"
			st.Runs[i].Run.FinishedAt = model.Now()
		}
		return nil
	})
	return context.Canceled
}
