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
	mirror *mirrorWriter
	attrs  []slog.Attr
	groups []string
}

func NewHandler(ring *Ring, level slog.Leveler, mirror io.Writer) *Handler {
	return &Handler{ring: ring, level: level, mirror: &mirrorWriter{w: mirror}}
}

// mirrorWriter is shared by a handler and its WithAttrs/WithGroup copies, so
// SetMirror affects all of them.
type mirrorWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// SetMirror starts (or, with nil, stops) copying records to w, for example
// when the TUI stops and the log has to go to stderr instead.
func (h *Handler) SetMirror(w io.Writer) {
	h.mirror.mu.Lock()
	h.mirror.w = w
	h.mirror.mu.Unlock()
}

func (h *Handler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level.Level() }

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	for _, a := range h.attrs { // already qualified by WithAttrs
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value.Resolve())
	}
	r.Attrs(func(a slog.Attr) bool {
		if !a.Equal(slog.Attr{}) {
			fmt.Fprintf(&b, " %s=%v", h.qualify(a.Key), a.Value.Resolve())
		}
		return true
	})

	line := Line{Time: r.Time, Level: r.Level.String(), Message: b.String()}
	h.ring.add(line)
	h.mirror.mu.Lock()
	if h.mirror.w != nil {
		fmt.Fprintf(h.mirror.w, "%s %-5s %s\n", line.Time.Format("15:04:05"), line.Level, line.Message)
	}
	h.mirror.mu.Unlock()
	return nil
}

// qualify prefixes a key with the groups opened so far.
func (h *Handler) qualify(key string) string {
	if len(h.groups) == 0 {
		return key
	}
	return strings.Join(h.groups, ".") + "." + key
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append([]slog.Attr(nil), h.attrs...)
	for _, a := range attrs {
		if !a.Equal(slog.Attr{}) {
			c.attrs = append(c.attrs, slog.Attr{Key: h.qualify(a.Key), Value: a.Value})
		}
	}
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
