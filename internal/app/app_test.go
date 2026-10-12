package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/demo"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/report"
	"github.com/AVGsync/protolab/internal/store"
)

func newTestApp(t *testing.T, dir string) *App {
	t.Helper()
	a, err := New(Config{DataDir: dir, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

func waitJob(t *testing.T, a *App, id string) model.Job {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	j, err := a.Jobs().WaitJob(ctx, id)
	if err != nil {
		t.Fatalf("job %s: %v", id, err)
	}
	return j
}

func runBy(t *testing.T, runs []model.VerificationRun, version int, corpus string) model.VerificationRun {
	t.Helper()
	for _, r := range runs {
		if r.Version == version && strings.HasPrefix(r.CorpusName, corpus) {
			return r
		}
	}
	t.Fatalf("run v%d on %q not found", version, corpus)
	return model.VerificationRun{}
}

func TestDemoProjectEndToEnd(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	ctx := context.Background()
	p, err := a.CreateDemoProject(ctx)
	if err != nil {
		t.Fatal(err)
	}

	captures, err := a.ListCaptures(p.ID)
	if err != nil || len(captures) != 3 {
		t.Fatalf("captures: %d %v", len(captures), err)
	}
	for _, c := range captures {
		if c.Status != "ready" || c.Integrity != "ok" || c.PacketCount == 0 {
			t.Fatalf("capture %s: status %s integrity %s packets %d", c.FileName, c.Status, c.Integrity, c.PacketCount)
		}
		if c.Diagnostics == nil {
			t.Errorf("capture %s: diagnostics must be a list, not null", c.FileName)
		}
	}
	// Background frames that are not TCP/IPv4 are skipped with a diagnostic.
	skipped := map[string]int{}
	for _, d := range captures[0].Diagnostics {
		skipped[d.Code] = d.Count
	}
	if skipped["non_ip"] != 2 || skipped["non_tcp"] != 3 || skipped["ipv6_unsupported"] != 1 {
		t.Errorf("non-TCP diagnostics of %s: %v", captures[0].FileName, captures[0].Diagnostics)
	}

	// A reused client port starts a new session; the capture defects of day 3
	// are reported, not hidden.
	sessions, err := a.ListSessions(p.ID, SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	reused := 0
	defects := map[string]bool{}
	for _, s := range sessions {
		if s.CaptureID == captures[0].ID && s.Client.Port == 51003 {
			reused++
		}
		if s.CaptureID != captures[2].ID {
			continue
		}
		for _, d := range s.Diagnostics {
			defects[d.Code] = true
		}
		for _, dir := range model.Directions {
			st, err := a.Stream(s.ID, dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, g := range st.Gaps {
				defects["gap_"+g.Reason] = true
			}
			if !st.StartKnown {
				defects["start_unknown"] = true
			}
		}
	}
	if reused != 2 {
		t.Errorf("sessions from client port 51003: %d, want 2", reused)
	}
	for _, code := range []string{"tcp_gap", "tcp_truncated", "overlap_conflict", "missing_handshake", "checksum_offload", "gap_lost", "gap_truncated", "start_unknown"} {
		if !defects[code] {
			t.Errorf("defect %s not reported for %s: %v", code, captures[2].FileName, defects)
		}
	}

	runs, err := a.ListRuns(p.ID)
	if err != nil || len(runs) != 5 {
		t.Fatalf("runs: %d %v", len(runs), err)
	}
	for _, r := range runs {
		if r.Status != "succeeded" || r.Summary == nil {
			t.Fatalf("run %s: %s", r.ID, r.Status)
		}
	}
	if s := runBy(t, runs, 1, "Основной").Summary; s.Violations != 0 {
		t.Errorf("v1 on the main corpus: %d violations, want 0", s.Violations)
	}
	if s := runBy(t, runs, 1, "Расширенный").Summary; s.Violations != 3 {
		t.Errorf("v1 on the extended corpus: %d violations, want 3", s.Violations)
	}
	for _, v := range []int{2, 3} {
		s := runBy(t, runs, v, "Расширенный").Summary
		if s.Violations != 0 || s.UnknownType != 0 || s.UnknownBytes != 0 {
			t.Errorf("v%d on the extended corpus: %+v", v, s)
		}
	}
	if s := runBy(t, runs, 2, "Дефекты").Summary; s.Partial == 0 {
		t.Errorf("defects corpus must have partial messages: %+v", s)
	}
	v3 := runBy(t, runs, 3, "Расширенный")
	if v3.Stale || !runBy(t, runs, 1, "Расширенный").Stale {
		t.Errorf("staleness: v3 stale=%v, v1 stale=%v", v3.Stale, runBy(t, runs, 1, "Расширенный").Stale)
	}
	if v3.PreviousRunID != runBy(t, runs, 2, "Расширенный").ID {
		t.Errorf("previous run of v3: %q", v3.PreviousRunID)
	}

	obs, _ := a.ListObservations(p.ID, ObservationFilter{})
	hyps, _ := a.ListHypotheses(p.ID)
	if len(obs) == 0 || len(hyps) == 0 {
		t.Fatalf("observations %d, hypotheses %d", len(obs), len(hyps))
	}
	for _, o := range obs {
		if o.Integrity != "ok" || strings.Contains(o.SnapshotHex, "??") {
			t.Errorf("observation %q: integrity %s snapshot %s", o.Note, o.Integrity, o.SnapshotHex)
		}
	}
	// A counterexample cites a run and points at bytes that run flagged.
	counterexamples := 0
	for _, h := range hyps {
		for _, ce := range h.Counterexamples {
			counterexamples++
			findings, err := a.ListFindings(ce.RunID, model.FindingViolation)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, f := range findings {
				for _, ex := range f.Examples {
					found = found || (ex.SessionID == ce.Source.SessionID && ex.Direction == ce.Source.Direction &&
						ex.Offset == ce.Source.Offset && ex.Length == ce.Source.Length)
				}
			}
			if !found {
				t.Errorf("counterexample %q of %q is not a finding of run %s", ce.Note, h.Title, ce.RunID)
			}
		}
	}
	if counterexamples == 0 {
		t.Error("the refuted hypothesis has no counterexamples")
	}

	// The examples of a report replay its runs with the report's spec.
	if _, _, err := a.CreateReport(p.ID, model.CreateReportInput{VersionID: v3.VersionID, RunIDs: []string{runBy(t, runs, 1, "Расширенный").ID}}); err == nil {
		t.Error("a report must not include runs of another version")
	}

	// Report with examples that reference the original captures.
	rep, jobID, err := a.CreateReport(p.ID, model.CreateReportInput{VersionID: v3.VersionID, RunIDs: []string{v3.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if j := waitJob(t, a, jobID); j.Status != "succeeded" {
		t.Fatalf("report job: %+v", j)
	}
	reports, _ := a.ListReports(p.ID)
	if len(reports) != 1 || reports[0].Status != "ready" || len(reports[0].Artifacts) != 4 {
		t.Fatalf("report: %+v", reports)
	}
	_, _, _, examplesName := report.Names(rep.InterpretationName, rep.Version)
	path, _, err := a.ReportFile(rep.ID, examplesName)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ex report.Examples
	if err := json.Unmarshal(raw, &ex); err != nil || len(ex.Corpora) != 1 || len(ex.Corpora[0].Sessions) == 0 {
		t.Fatalf("examples: %v %+v", err, ex)
	}
	if _, _, err := a.ReportFile(rep.ID, "../../project.json"); err == nil {
		t.Fatal("report file lookup must reject foreign names")
	}

	// Changing the overlap policy re-analyses captures and makes runs stale.
	policy := "last"
	if _, err := a.UpdateProject(p.ID, ProjectPatch{Settings: &SettingsPatch{OverlapPolicy: &policy}}); err != nil {
		t.Fatal(err)
	}
	runs, _ = a.ListRuns(p.ID)
	if !runBy(t, runs, 3, "Расширенный").Stale {
		t.Error("runs must become stale after a settings change")
	}

	// Export and import the same project: every id is regenerated.
	var buf bytes.Buffer
	if err := a.ExportProject(p.ID, &buf); err != nil {
		t.Fatal(err)
	}
	imported, jobID, err := a.ImportProject("tc200.protolab.zip", &buf)
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, a, jobID)
	if imported.ID == p.ID || imported.Stats.Runs != 5 || imported.Stats.Captures != 3 {
		t.Fatalf("imported project: %+v", imported)
	}
	importedSessions, err := a.ListSessions(imported.ID, SessionFilter{})
	if err != nil || len(importedSessions) == 0 {
		t.Fatalf("imported sessions: %d %v", len(importedSessions), err)
	}
	for _, s := range importedSessions {
		if strings.HasPrefix(s.ID, captures[0].ID) {
			t.Fatalf("imported session kept an old id: %s", s.ID)
		}
	}
	importedReports, _ := a.ListReports(imported.ID)
	if len(importedReports) != 1 || importedReports[0].ID == rep.ID {
		t.Fatalf("imported reports: %+v", importedReports)
	}
	for _, art := range importedReports[0].Artifacts {
		if !strings.Contains(art.URL, importedReports[0].ID) {
			t.Errorf("artifact URL points to another report: %s", art.URL)
		}
		if _, _, err := a.ReportFile(importedReports[0].ID, art.FileName); err != nil {
			t.Errorf("imported artifact %s: %v", art.FileName, err)
		}
	}
	importedRuns, _ := a.ListRuns(imported.ID)
	for _, r := range importedRuns {
		findings, _ := a.ListFindings(r.ID, "")
		for _, f := range findings {
			if !strings.HasPrefix(f.ID, r.ID+"-f") || f.RunID != r.ID {
				t.Fatalf("finding %s of run %s", f.ID, r.ID)
			}
		}
	}
}

func TestImportCaptureJobAndDuplicates(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Импорт", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	ds := demo.Build(demo.Options{})
	c := ds.Captures[0]
	res, err := a.ImportCapture(p.ID, c.FileName, bytes.NewReader(c.Data))
	if err != nil {
		t.Fatal(err)
	}
	if res.Capture.Status != "indexing" || res.JobID == "" {
		t.Fatalf("upload result: %+v", res)
	}
	if j := waitJob(t, a, res.JobID); j.Status != "succeeded" {
		t.Fatalf("import job: %+v", j)
	}
	got, err := a.GetCapture(res.Capture.ID)
	if err != nil || got.Status != "ready" || got.SessionCount == 0 || got.Integrity != "ok" {
		t.Fatalf("capture after import: %+v %v", got, err)
	}
	if _, err := a.ImportCapture(p.ID, "copy.pcapng", bytes.NewReader(c.Data)); err == nil {
		t.Fatal("duplicate capture must be rejected")
	} else if e, ok := AsError(err); !ok || e.Code != "duplicate_capture" {
		t.Fatalf("duplicate error: %v", err)
	}

	sessions, _ := a.ListSessions(p.ID, SessionFilter{})
	sd := sessions[0]
	data, total, err := a.StreamBytes(sd.ID, model.C2S, 0, -1)
	if err != nil || len(data) != total || total != sd.Bytes.C2S {
		t.Fatalf("stream bytes: %d/%d %v", len(data), total, err)
	}
	if _, _, err := a.StreamBytes(sd.ID, "up", 0, -1); err == nil {
		t.Fatal("invalid direction must fail")
	}
}

func TestReopenKeepsProjects(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	b := newTestApp(t, dir)
	got, err := b.GetProject(p.ID)
	if err != nil || got.Stats.Runs != 5 {
		t.Fatalf("reopened project: %+v %v", got, err)
	}
	caps, _ := b.ListCaptures(p.ID)
	if _, err := b.ListSessions(p.ID, SessionFilter{}); err != nil {
		t.Fatal(err)
	}
	caps, _ = b.ListCaptures(p.ID)
	for _, c := range caps {
		if c.Integrity != "ok" {
			t.Errorf("capture %s integrity after reopen: %s", c.FileName, c.Integrity)
		}
	}
}

func TestInterruptedIndexingResumesOnStart(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Прерванный импорт", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	c := demo.Build(demo.Options{}).Captures[0]
	res, err := a.ImportCapture(p.ID, c.FileName, bytes.NewReader(c.Data))
	if err != nil {
		t.Fatal(err)
	}
	waitJob(t, a, res.JobID)
	a.Close()

	// Simulate a process that stopped while the import job was running.
	st, err := store.LoadDir(filepath.Join(dir, "projects", p.ID))
	if err != nil {
		t.Fatal(err)
	}
	st.Captures[0].Status = "indexing"
	st.Captures[0].SessionCount = 0
	data, _ := json.Marshal(st)
	if err := store.WriteFileAtomic(filepath.Join(dir, "projects", p.ID, "project.json"), data); err != nil {
		t.Fatal(err)
	}

	b := newTestApp(t, dir)
	jobs := b.Jobs().List(0)
	if len(jobs) != 1 || jobs[0].Kind != "import_capture" {
		t.Fatalf("resumed jobs: %+v", jobs)
	}
	if j := waitJob(t, b, jobs[0].ID); j.Status != "succeeded" {
		t.Fatalf("resumed job: %+v", j)
	}
	got, err := b.GetCapture(res.Capture.ID)
	if err != nil || got.Status != "ready" || got.SessionCount == 0 {
		t.Fatalf("capture after resume: %+v %v", got, err)
	}
}
