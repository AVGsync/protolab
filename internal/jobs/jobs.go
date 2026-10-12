// Package jobs runs long operations (capture import, verification, reports)
// in the background with progress reporting and cancellation.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/AVGsync/protolab/internal/model"
)

// Job statuses.
const (
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// keep is how many finished jobs stay visible.
const keep = 50

// ErrNotCancellable is returned by Cancel for a finished job.
var ErrNotCancellable = errors.New("job already finished")

// ErrNotFound is returned for an unknown job id.
var ErrNotFound = errors.New("job not found")

// Progress lets a running job report how far it got. fraction is clamped to [0, 1].
type Progress func(fraction float64, stage string)

// Func is the body of a job. A non-nil result is stored on success.
type Func func(ctx context.Context, progress Progress) (*model.JobResult, error)

type entry struct {
	job    model.Job
	cancel context.CancelFunc
	done   chan struct{}
}

// Manager owns all jobs of the process.
type Manager struct {
	mu    sync.Mutex
	jobs  []*entry // newest first
	base  context.Context
	wg    sync.WaitGroup
	onEnd func(model.Job)
}

// NewManager creates a manager; jobs are cancelled when base is cancelled.
// onEnd (optional) is called after a job finished, outside of the lock.
func NewManager(base context.Context, onEnd func(model.Job)) *Manager {
	return &Manager{base: base, onEnd: onEnd}
}

// Start launches fn in a goroutine and returns the job snapshot.
func (m *Manager) Start(projectID, kind, title string, fn Func) model.Job {
	ctx, cancel := context.WithCancel(m.base)
	e := &entry{
		job: model.Job{
			ID:          newID(),
			ProjectID:   projectID,
			Kind:        kind,
			Title:       title,
			Status:      StatusRunning,
			StartedAt:   model.Now(),
			Cancellable: true,
		},
		cancel: cancel,
		done:   make(chan struct{}),
	}

	m.mu.Lock()
	m.jobs = append([]*entry{e}, m.jobs...)
	m.trimLocked()
	snapshot := e.job
	m.mu.Unlock()

	m.wg.Add(1)
	go m.run(ctx, e, fn)
	return snapshot
}

func (m *Manager) run(ctx context.Context, e *entry, fn Func) {
	defer m.wg.Done()
	defer close(e.done)
	defer e.cancel()

	progress := func(fraction float64, stage string) {
		fraction = min(max(fraction, 0), 1)
		m.mu.Lock()
		if e.job.Status == StatusRunning {
			e.job.Progress = fraction
			if stage != "" {
				e.job.Stage = stage
			}
		}
		m.mu.Unlock()
	}

	result, err := safeCall(ctx, fn, progress)

	m.mu.Lock()
	switch {
	case err == nil:
		// The body finished its work (and committed it) even if a cancellation
		// arrived after its last check: reporting "cancelled" would contradict
		// the stored result (a ready capture, a succeeded run).
		e.job.Status = StatusSucceeded
		e.job.Progress = 1
		e.job.Result = result
	case ctx.Err() != nil && errors.Is(err, context.Canceled):
		e.job.Status = StatusCancelled
	default:
		e.job.Status = StatusFailed
		e.job.Error = err.Error()
	}
	e.job.FinishedAt = model.Now()
	e.job.Cancellable = false
	finished := e.job
	m.mu.Unlock()

	if m.onEnd != nil {
		m.onEnd(finished)
	}
}

// safeCall turns a panic inside a job into an error so one broken job cannot crash the server.
func safeCall(ctx context.Context, fn Func, p Progress) (res *model.JobResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicError{r}
		}
	}()
	return fn(ctx, p)
}

type panicError struct{ v any }

func (p panicError) Error() string {
	if e, ok := p.v.(error); ok {
		return "internal error: " + e.Error()
	}
	if s, ok := p.v.(string); ok {
		return "internal error: " + s
	}
	return "internal error"
}

// trimLocked drops the oldest finished jobs beyond the retention limit.
func (m *Manager) trimLocked() {
	if len(m.jobs) <= keep {
		return
	}
	kept := m.jobs[:0]
	finished := 0
	for _, e := range m.jobs {
		if e.job.Status != StatusRunning {
			finished++
			if finished > keep {
				continue
			}
		}
		kept = append(kept, e)
	}
	m.jobs = kept
}

// List returns up to limit jobs, newest first.
func (m *Manager) List(limit int) []model.Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > len(m.jobs) {
		limit = len(m.jobs)
	}
	out := make([]model.Job, limit)
	for i := range out {
		out[i] = m.jobs[i].job
	}
	return out
}

// Get returns a job snapshot.
func (m *Manager) Get(id string) (model.Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.jobs {
		if e.job.ID == id {
			return e.job, nil
		}
	}
	return model.Job{}, ErrNotFound
}

// Cancel requests cancellation and waits until the job body has returned,
// so the caller observes the final state. A job that completed before it
// noticed the request is reported with ErrNotCancellable and its final state.
func (m *Manager) Cancel(id string) (model.Job, error) {
	m.mu.Lock()
	var target *entry
	for _, e := range m.jobs {
		if e.job.ID == id {
			target = e
			break
		}
	}
	if target == nil {
		m.mu.Unlock()
		return model.Job{}, ErrNotFound
	}
	if target.job.Status != StatusRunning {
		snapshot := target.job
		m.mu.Unlock()
		return snapshot, ErrNotCancellable
	}
	m.mu.Unlock()

	target.cancel()
	select {
	case <-target.done:
	case <-time.After(10 * time.Second):
		// The job ignores its context; report the current state.
		return m.Get(id)
	}
	j, err := m.Get(id)
	if err == nil && j.Status != StatusCancelled {
		return j, ErrNotCancellable
	}
	return j, err
}

// Wait blocks until all running jobs finished (used on shutdown and in tests).
func (m *Manager) Wait() { m.wg.Wait() }

// WaitJob blocks until the given job finished or ctx is done.
func (m *Manager) WaitJob(ctx context.Context, id string) (model.Job, error) {
	m.mu.Lock()
	var done chan struct{}
	for _, e := range m.jobs {
		if e.job.ID == id {
			done = e.done
		}
	}
	m.mu.Unlock()
	if done == nil {
		return model.Job{}, ErrNotFound
	}
	select {
	case <-done:
		return m.Get(id)
	case <-ctx.Done():
		return model.Job{}, ctx.Err()
	}
}

func newID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return "job-" + hex.EncodeToString(b[:])
}
