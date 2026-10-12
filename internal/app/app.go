// Package app is the service layer: it owns projects, runs background jobs
// and turns captures into sessions, streams, messages and verification results.
// The HTTP server and the TUI are thin adapters on top of it.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/jobs"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
	"github.com/AVGsync/protolab/internal/tcpflow"
)

// Limits are the declared protective limits. Exceeding one is reported
// explicitly, data is never cut silently.
type Limits struct {
	MaxCaptureBytes     int64
	MaxPackets          int
	MaxSessions         int
	MaxMessages         int   // per message query (one session or the comparison view)
	MaxMessageBytes     int   // framing upper bound when a spec does not set maxLength
	MaxStreamBytes      int   // per reassembled direction
	MaxTotalStreamBytes int64 // all reassembled directions of one capture together
}

// DefaultLimits cover the reference load of the task (100 MiB, 250k packets,
// 1000 connections, 100k messages) with a safety margin; docs/performance.md
// has the measurements.
func DefaultLimits() Limits {
	return Limits{
		MaxCaptureBytes:     512 << 20,
		MaxPackets:          2_000_000,
		MaxSessions:         20_000,
		MaxMessages:         200_000,
		MaxMessageBytes:     engine.DefaultMaxLength,
		MaxStreamBytes:      tcpflow.DefaultMaxStreamBytes,
		MaxTotalStreamBytes: tcpflow.DefaultMaxTotalBytes,
	}
}

// BuildInfo is injected by the linker.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

type Config struct {
	DataDir string
	Limits  Limits
	Build   BuildInfo
	Logger  *slog.Logger
}

// App is safe for concurrent use.
type App struct {
	cfg     Config
	store   *store.Store
	jobs    *jobs.Manager
	log     *slog.Logger
	started time.Time
	ctx     context.Context
	stop    context.CancelFunc

	mu       sync.RWMutex
	projects map[string]*project
	owner    map[string]string // entity id -> project id

	importMu sync.Mutex // serialises installImport
}

// staleTempAge: temporary files of an upload or an import older than this
// were left by a process that was killed (a running import keeps writing).
const staleTempAge = time.Hour

// New opens the data directory and loads every project in it.
func New(cfg Config) (*App, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Limits == (Limits{}) {
		cfg.Limits = DefaultLimits()
	}
	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	for _, p := range st.RemoveStaleTemp(staleTempAge) {
		cfg.Logger.Info("stale temporary file removed", "path", p)
	}
	ctx, stop := context.WithCancel(context.Background())
	a := &App{
		cfg:      cfg,
		store:    st,
		log:      cfg.Logger,
		started:  time.Now(),
		ctx:      ctx,
		stop:     stop,
		projects: map[string]*project{},
		owner:    map[string]string{},
	}
	a.jobs = jobs.NewManager(ctx, a.onJobEnd)

	states, errs := st.List()
	for id, err := range errs {
		a.log.Error("project skipped", "project", id, "err", err)
	}
	for _, s := range states {
		p := newProject(a, s)
		a.register(p)
		a.failInterrupted(p)
		a.resumeIndexing(p)
	}
	a.log.Info("data directory opened", "path", st.Root(), "projects", len(states))
	return a, nil
}

// Preload analyses every capture in the background so the first page load is fast.
func (a *App) Preload() {
	for _, p := range a.projectList() {
		go func(p *project) {
			for _, c := range p.captureIDs() {
				if a.ctx.Err() != nil {
					return
				}
				if _, err := p.analysis(c); err != nil {
					a.log.Warn("capture analysis failed", "project", p.id, "capture", c, "err", err)
				}
			}
		}(p)
	}
}

// Close cancels running jobs and waits for them.
func (a *App) Close() {
	a.stop()
	a.jobs.Wait()
}

// Jobs exposes the job manager (read-only use by the HTTP layer).
func (a *App) Jobs() *jobs.Manager { return a.jobs }

func (a *App) Limits() Limits { return a.cfg.Limits }

func (a *App) DataDir() string { return a.store.Root() }

func (a *App) Build() BuildInfo { return a.cfg.Build }

func (a *App) onJobEnd(j model.Job) {
	switch j.Status {
	case jobs.StatusFailed:
		a.log.Error("job failed", "job", j.Title, "err", j.Error)
	case jobs.StatusCancelled:
		a.log.Info("job cancelled", "job", j.Title)
	default:
		a.log.Info("job finished", "job", j.Title)
	}
}

// --------------------------------------------------------------- registry

func (a *App) register(p *project) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.projects[p.id] = p
	for _, id := range p.entityIDs() {
		a.owner[id] = p.id
	}
}

func (a *App) unregister(p *project) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.projects, p.id)
	for id, owner := range a.owner {
		if owner == p.id {
			delete(a.owner, id)
		}
	}
}

func (a *App) index(projectID string, ids ...string) {
	a.mu.Lock()
	for _, id := range ids {
		a.owner[id] = projectID
	}
	a.mu.Unlock()
}

func (a *App) project(id string) (*project, error) {
	a.mu.RLock()
	p := a.projects[id]
	a.mu.RUnlock()
	if p == nil {
		return nil, notFound("Проект")
	}
	return p, nil
}

// projectOf finds the project owning an entity id.
func (a *App) projectOf(entityID, what string) (*project, error) {
	a.mu.RLock()
	pid, ok := a.owner[entityID]
	p := a.projects[pid]
	a.mu.RUnlock()
	if !ok || p == nil {
		return nil, notFound(what)
	}
	return p, nil
}

// projectOfSession resolves "<captureId>-<index>" session ids through the capture.
func (a *App) projectOfSession(sessionID string) (*project, error) {
	capID, ok := captureOfSession(sessionID)
	if !ok {
		return nil, notFound("Сессия")
	}
	return a.projectOf(capID, "Сессия")
}

func (a *App) projectList() []*project {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*project, 0, len(a.projects))
	for _, p := range a.projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].createdAt() > out[j].createdAt() })
	return out
}

// --------------------------------------------------------------- ids

func newID(prefix string) string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func sessionID(captureID string, index int) string {
	return fmt.Sprintf("%s-%03d", captureID, index)
}

// captureOfSession splits "<captureId>-<index>".
func captureOfSession(sessionID string) (string, bool) {
	for i := len(sessionID) - 1; i > 0; i-- {
		c := sessionID[i]
		if c == '-' {
			return sessionID[:i], i < len(sessionID)-1
		}
		if c < '0' || c > '9' {
			return "", false
		}
	}
	return "", false
}
