package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AVGsync/protolab/internal/report"
)

func TestExampleSettings(t *testing.T) {
	ex := report.Examples{Settings: report.ExampleSettings{OverlapPolicy: "last", ChecksumPolicy: "ignore", TargetPorts: []int{5020}}}
	if s := exampleSettings(ex, report.CorpusExample{}); s.OverlapPolicy != "last" || s.ChecksumPolicy != "ignore" || s.TargetPorts[0] != 5020 {
		t.Errorf("file settings: %+v", s)
	}
	own := report.CorpusExample{Settings: &report.ExampleSettings{OverlapPolicy: "first", ChecksumPolicy: "warn"}}
	if s := exampleSettings(ex, own); s.OverlapPolicy != "first" || s.ChecksumPolicy != "warn" || s.TargetPorts[0] != 5020 {
		t.Errorf("corpus settings: %+v", s)
	}
	// Files written before the checksum policy was recorded.
	old := report.Examples{Settings: report.ExampleSettings{OverlapPolicy: "first"}}
	if s := exampleSettings(old, report.CorpusExample{}); s.ChecksumPolicy != "warn" || s.TargetPorts == nil {
		t.Errorf("old file: %+v", s)
	}
}

// `protolab verify --examples examples.x.json` run from the report folder of a
// project (reports/<id>/) finds the captures two levels up.
func TestCaptureRootsFromRelativeExamplesPath(t *testing.T) {
	root := t.TempDir()
	reportDir := filepath.Join(root, "reports", "rep-1")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "captures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "captures", "a.pcapng"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(reportDir)
	path, ok := findCapture(captureRoots("", filepath.Dir("examples.x.json")), "captures/a.pcapng")
	if !ok {
		t.Fatal("capture not found from a relative examples path")
	}
	got, err1 := os.Stat(path)
	want, err2 := os.Stat(filepath.Join(root, "captures", "a.pcapng"))
	if err1 != nil || err2 != nil || !os.SameFile(got, want) {
		t.Fatalf("found %q", path)
	}
}
