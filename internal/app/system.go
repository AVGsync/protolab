package app

import (
	"runtime"
	"sync"
	"time"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

var diskUsage struct {
	mu    sync.Mutex
	at    time.Time
	bytes int64
}

// usedBytes walks the data directory at most every 30 seconds.
func (a *App) usedBytes() int64 {
	diskUsage.mu.Lock()
	defer diskUsage.mu.Unlock()
	if time.Since(diskUsage.at) > 30*time.Second {
		diskUsage.bytes = store.DirSize(a.store.Root())
		diskUsage.at = time.Now()
	}
	return diskUsage.bytes
}

// SystemInfo reports build info, the declared limits and current resource usage.
func (a *App) SystemInfo() model.SystemInfo {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	l := a.cfg.Limits
	b := a.cfg.Build
	return model.SystemInfo{
		Version:       b.Version,
		Commit:        b.Commit,
		BuildDate:     b.Date,
		EngineVersion: engine.Version,
		Limits: model.SystemLimits{
			MaxCaptureBytes: l.MaxCaptureBytes,
			MaxPackets:      l.MaxPackets,
			MaxSessions:     l.MaxSessions,
			MaxMessages:     l.MaxMessages,
			MaxMessageBytes: l.MaxMessageBytes,
		},
		Runtime: model.SystemRuntime{
			OS:            runtime.GOOS,
			Arch:          runtime.GOARCH,
			CPUs:          runtime.NumCPU(),
			GoVersion:     runtime.Version(),
			MemAllocBytes: ms.HeapAlloc,
			MemSysBytes:   ms.Sys,
			UptimeSec:     int64(time.Since(a.started).Seconds()),
		},
		Storage: model.SystemStorage{DataDir: a.store.Root(), UsedBytes: a.usedBytes()},
	}
}

// Summary is a compact view of a project for the TUI.
type Summary struct {
	ID       string
	Name     string
	Captures int
	Sessions int
	Runs     int
}

func (a *App) Summaries() []Summary {
	list := a.projectList()
	out := make([]Summary, len(list))
	for i, p := range list {
		d := p.dto()
		out[i] = Summary{ID: d.ID, Name: d.Name, Captures: d.Stats.Captures, Sessions: d.Stats.Sessions, Runs: d.Stats.Runs}
	}
	return out
}

// Uptime since the application started.
func (a *App) Uptime() time.Duration { return time.Since(a.started) }

// StartedAt is when the application started.
func (a *App) StartedAt() time.Time { return a.started }
