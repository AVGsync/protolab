// Package logx keeps recent log records in memory for the TUI log panel and
// optionally mirrors them to a writer (stderr in --no-tui mode).
package logx

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Line is one formatted log record.
type Line struct {
	Time    time.Time
	Level   string
	Message string
}

// Ring is a fixed-size buffer of the latest lines.
type Ring struct {
	mu    sync.Mutex
	lines []Line
	next  int
	full  bool
}

func NewRing(size int) *Ring { return &Ring{lines: make([]Line, size)} }

func (r *Ring) add(l Line) {
	r.mu.Lock()
	r.lines[r.next] = l
	r.next = (r.next + 1) % len(r.lines)
	if r.next == 0 {
		r.full = true
	}
	r.mu.Unlock()
}

// Lines returns the buffered lines, oldest first.
func (r *Ring) Lines() []Line {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]Line(nil), r.lines[:r.next]...)
	}
	out := make([]Line, 0, len(r.lines))
	out = append(out, r.lines[r.next:]...)
	return append(out, r.lines[:r.next]...)
}

// Handler is a slog.Handler writing "msg key=value ..." lines into a Ring and,
// when mirror is set, also to that writer.
type Handler struct {
	ring   *Ring
	level  slog.Leveler
	mirror io.Writer
	mu     *sync.Mutex // guards mirror
	attrs  []slog.Attr
	groups []string
}

func NewHandler(ring *Ring, level slog.Leveler, mirror io.Writer) *Handler {
	return &Handler{ring: ring, level: level, mirror: mirror, mu: &sync.Mutex{}}
}

func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	write := func(a slog.Attr) bool {
		if a.Equal(slog.Attr{}) {
			return true
		}
		key := a.Key
		if len(h.groups) > 0 {
			key = strings.Join(h.groups, ".") + "." + key
		}
		fmt.Fprintf(&b, " %s=%v", key, a.Value.Resolve())
		return true
	}
	for _, a := range h.attrs {
		write(a)
	}
	r.Attrs(write)

	line := Line{Time: r.Time, Level: r.Level.String(), Message: b.String()}
	h.ring.add(line)
	if h.mirror != nil {
		h.mu.Lock()
		fmt.Fprintf(h.mirror, "%s %-5s %s\n", line.Time.Format("15:04:05"), line.Level, line.Message)
		h.mu.Unlock()
	}
	return nil
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &c
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.groups = append(append([]string(nil), h.groups...), name)
	return &c
}
