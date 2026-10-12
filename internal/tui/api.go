// Package tui is the terminal interface shown while the server runs.
package tui

import (
	"errors"
	"time"

	"github.com/AVGsync/protolab/internal/model"
)

type ProjectSummary struct {
	ID       string
	Name     string
	Captures int
	Sessions int
	Runs     int
}

type LogLine struct {
	Time    time.Time
	Level   string // "DEBUG" | "INFO" | "WARN" | "ERROR"
	Message string
}

// Snapshot is the state rendered by the TUI; it is polled a few times per second.
type Snapshot struct {
	Version   string
	URL       string // http://127.0.0.1:port
	DataDir   string
	Projects  []ProjectSummary
	Jobs      []model.Job // newest first
	Logs      []LogLine   // oldest first
	MemAlloc  uint64
	StartedAt time.Time
}

type Source interface {
	Snapshot() Snapshot
}

type Options struct {
	Source      Source
	OpenBrowser func(url string) error
	// CreateDemo creates the demo project (hotkey "d"); nil hides the action.
	CreateDemo func() error
}

// ErrNotTerminal is returned by Run (before touching the terminal) when stdin or
// stdout is not an interactive terminal; the caller falls back to plain output.
var ErrNotTerminal = errors.New("tui: stdin/stdout is not a terminal")
