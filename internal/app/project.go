package app

import (
	"sync"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

type storeCapture = store.CaptureRecord

// project is the in-memory state of one project directory.
//
// mu guards st and the analysis slots. Long work (parsing captures, running
// the engine) happens outside of mu on immutable snapshots.
type project struct {
	app *App
	id  string

	mu    sync.Mutex
	st    *store.State
	slots map[string]*analysisSlot // captureID -> analysis
	rev   int                      // bumped on every change that affects parsed messages
	cache *messageCache
}

func newProject(a *App, st *store.State) *project {
	normalizeState(st)
	return &project{
		app:   a,
		id:    st.Project.ID,
		st:    st,
		slots: map[string]*analysisSlot{},
		cache: newMessageCache(256),
	}
}

// normalizeState replaces nil slices so the JSON API never returns null lists.
func normalizeState(st *store.State) {
	if st.Project.Settings.TargetPorts == nil {
		st.Project.Settings.TargetPorts = []int{}
	}
	if st.Project.Settings.OverlapPolicy == "" {
		st.Project.Settings.OverlapPolicy = "first"
	}
	if st.Project.Settings.ChecksumPolicy == "" {
		st.Project.Settings.ChecksumPolicy = "warn"
	}
	for i := range st.Actions {
		if st.Actions[i].Params == nil {
			st.Actions[i].Params = map[string]any{}
		}
		if st.Actions[i].Links == nil {
			st.Actions[i].Links = []model.ActionLink{}
		}
	}
	for i := range st.Hypotheses {
		h := &st.Hypotheses[i]
		h.ObservationIDs = nonNil(h.ObservationIDs)
		h.Alternatives = nonNil(h.Alternatives)
		h.RuleRefs = nonNil(h.RuleRefs)
		if h.Counterexamples == nil {
			h.Counterexamples = []model.Counterexample{}
		}
		if h.History == nil {
			h.History = []model.HypothesisEvent{}
		}
	}
	for i := range st.Observations {
		st.Observations[i].HypothesisIDs = nonNil(st.Observations[i].HypothesisIDs)
		st.Observations[i].Tags = nonNil(st.Observations[i].Tags)
	}
	for i := range st.Reports {
		st.Reports[i].RunIDs = nonNil(st.Reports[i].RunIDs)
	}
	for i := range st.Captures {
		if st.Captures[i].Diagnostics == nil {
			st.Captures[i].Diagnostics = []model.Diagnostic{}
		}
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// save persists the state; the caller holds p.mu.
func (p *project) save() error {
	p.st.Project.UpdatedAt = model.Now()
	return p.app.store.Save(p.st)
}

// update runs fn under the lock and saves when it succeeds.
func (p *project) update(fn func(st *store.State) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := fn(p.st); err != nil {
		return err
	}
	return p.save()
}

// read runs fn under the lock.
func (p *project) read(fn func(st *store.State)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn(p.st)
}

func (p *project) dir() string { return p.app.store.ProjectDir(p.id) }

func (p *project) createdAt() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st.Project.CreatedAt
}

func (p *project) captureIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]string, 0, len(p.st.Captures))
	for _, c := range p.st.Captures {
		if c.Status != "indexing" {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func (p *project) entityIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.st
	var ids []string
	for _, c := range st.Captures {
		ids = append(ids, c.ID)
	}
	for _, x := range st.Actions {
		ids = append(ids, x.ID)
	}
	for _, x := range st.Interpretations {
		ids = append(ids, x.ID)
	}
	for _, x := range st.Versions {
		ids = append(ids, x.ID)
	}
	for _, x := range st.Hypotheses {
		ids = append(ids, x.ID)
	}
	for _, x := range st.Observations {
		ids = append(ids, x.ID)
	}
	for _, x := range st.Corpora {
		ids = append(ids, x.ID)
	}
	for _, x := range st.Runs {
		ids = append(ids, x.Run.ID)
	}
	for _, x := range st.Reports {
		ids = append(ids, x.ID)
	}
	return ids
}

// invalidate drops cached analyses and messages (settings changed).
func (p *project) invalidate() {
	p.slots = map[string]*analysisSlot{}
	p.rev++
	p.cache.clear()
}

func findIndex[T any](items []T, match func(*T) bool) int {
	for i := range items {
		if match(&items[i]) {
			return i
		}
	}
	return -1
}
