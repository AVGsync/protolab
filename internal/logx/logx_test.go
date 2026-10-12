package logx

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRingKeepsTheLatestLinesOldestFirst(t *testing.T) {
	r := NewRing(3)
	for _, m := range []string{"a", "b", "c", "d", "e"} {
		r.add(Line{Message: m})
	}
	var got []string
	for _, l := range r.Lines() {
		got = append(got, l.Message)
	}
	if strings.Join(got, "") != "cde" {
		t.Fatalf("lines = %v, want c d e", got)
	}
}

func TestHandlerQualifiesAttrsByTheGroupsOpenWhenAdded(t *testing.T) {
	r := NewRing(10)
	log := slog.New(NewHandler(r, slog.LevelInfo, nil))
	log.With("a", 1).WithGroup("g").With("b", 2).Info("msg", "c", 3)
	log.Debug("hidden")

	lines := r.Lines()
	if len(lines) != 1 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines[0].Message != "msg a=1 g.b=2 g.c=3" || lines[0].Level != "INFO" {
		t.Fatalf("line = %+v", lines[0])
	}
}

func TestSetMirrorAppliesToDerivedHandlers(t *testing.T) {
	r := NewRing(10)
	h := NewHandler(r, slog.LevelInfo, nil)
	derived := slog.New(h).With("k", "v")

	derived.Info("before")
	var buf bytes.Buffer
	h.SetMirror(&buf)
	derived.Warn("after")

	if strings.Contains(buf.String(), "before") || !strings.Contains(buf.String(), "WARN  after k=v") {
		t.Fatalf("mirror output: %q", buf.String())
	}
	if len(r.Lines()) != 2 {
		t.Fatalf("ring must keep every record: %+v", r.Lines())
	}
}
