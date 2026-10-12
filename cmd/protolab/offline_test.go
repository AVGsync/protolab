package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AVGsync/protolab/internal/app"
	"github.com/AVGsync/protolab/internal/model"
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

// R6: the report bundle is self-contained. Unpacked anywhere, the commands the
// report prints succeed from its directory and reproduce the expected results.
func TestReportBundleReproduces(t *testing.T) {
	a, err := app.New(app.Config{DataDir: t.TempDir(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx := context.Background()
	p, err := a.CreateDemoProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ints, _ := a.ListInterpretations(p.ID)
	runs, _ := a.ListRuns(p.ID)
	var runIDs []string
	for _, r := range runs {
		if r.VersionID == ints[0].LatestVersionID && r.Status == "succeeded" {
			runIDs = append(runIDs, r.ID)
		}
	}
	rep, jobID, err := a.CreateReport(p.ID, model.CreateReportInput{VersionID: ints[0].LatestVersionID, RunIDs: runIDs})
	if err != nil || len(runIDs) == 0 {
		t.Fatalf("report: %v (%d runs)", err, len(runIDs))
	}
	if j, err := a.Jobs().WaitJob(ctx, jobID); err != nil || j.Status != "succeeded" {
		t.Fatalf("report job: %+v %v", j, err)
	}
	name, write, err := a.ReportBundle(rep.ID)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := write(&buf); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	var examples, specFile, capture string
	for _, f := range zr.File {
		dst := filepath.Join(dir, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatal(err)
		}
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		rc.Close()
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.HasPrefix(f.Name, "examples."):
			examples = f.Name
		case strings.HasPrefix(f.Name, "interpretation."):
			specFile = f.Name
		case strings.HasPrefix(f.Name, "captures/"):
			capture = f.Name
		}
	}
	if examples == "" || specFile == "" || capture == "" {
		t.Fatalf("bundle %s lacks examples, interpretation or captures: %v", name, zr.File)
	}
	t.Chdir(dir)
	if code := runVerify([]string{"--examples", examples}); code != 0 {
		t.Fatalf("verify from the bundle directory: exit %d", code)
	}
	if code := runApply([]string{"--spec", specFile, "--input", capture, "--out", filepath.Join(t.TempDir(), "out.json")}); code != 0 {
		t.Fatalf("apply: exit %d", code)
	}
}
