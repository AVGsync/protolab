package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"slices"
	"testing"

	"github.com/AVGsync/protolab/internal/demo"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

// The interpretation file a user downloads (with "$meta") is accepted back by
// the API, as `protolab apply --spec` accepts it.
func TestExportedSpecFileIsAccepted(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ints, _ := a.ListInterpretations(p.ID)
	v, err := a.GetVersion(ints[0].LatestVersionID)
	if err != nil {
		t.Fatal(err)
	}
	file, _, err := a.ExportVersion(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res := a.ValidateSpec(file); !res.Valid {
		t.Fatalf("exported file does not validate: %+v", res.Issues)
	}
	created, err := a.CreateInterpretation(p.ID, CreateInterpretationInput{Name: "copy", Spec: file})
	if err != nil {
		t.Fatalf("exported file rejected: %v", err)
	}
	cv, _ := a.GetVersion(created.LatestVersionID)
	if cv.Hash != v.Hash {
		t.Errorf("hash of the re-imported spec %s, want %s", cv.Hash, v.Hash)
	}
	if _, err := a.CreateVersion(created.ID, CreateVersionInput{Spec: file}); err == nil {
		t.Error("the same file saved again must be no_changes")
	} else if e, ok := AsError(err); !ok || e.Code != "no_changes" {
		t.Errorf("same file again: %v", err)
	}
}

// Attaching observations with PATCH keeps both sides of the link, and a
// rejected PATCH changes nothing.
func TestUpdateHypothesisLinksObservations(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	obs, _ := a.ListObservations(p.ID, ObservationFilter{})
	h, err := a.CreateHypothesis(p.ID, model.CreateHypothesisInput{Title: "H"})
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{obs[0].ID, obs[1].ID, obs[0].ID}
	got, err := a.UpdateHypothesis(h.ID, model.UpdateHypothesisInput{ObservationIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.ObservationIDs, ids[:2]) {
		t.Errorf("observationIds = %v, want %v", got.ObservationIDs, ids[:2])
	}
	linked, _ := a.ListObservations(p.ID, ObservationFilter{HypothesisID: h.ID})
	if len(linked) != 2 {
		t.Fatalf("observations?hypothesisId: %d, want 2", len(linked))
	}
	// Unlinking removes the hypothesis from the observation.
	if _, err := a.UpdateHypothesis(h.ID, model.UpdateHypothesisInput{ObservationIDs: ids[1:2]}); err != nil {
		t.Fatal(err)
	}
	if linked, _ := a.ListObservations(p.ID, ObservationFilter{HypothesisID: h.ID}); len(linked) != 1 || linked[0].ID != obs[1].ID {
		t.Fatalf("after unlink: %+v", linked)
	}

	// Status change + unknown observation: rejected as a whole.
	status, empty := "testing", ""
	_, err = a.UpdateHypothesis(h.ID, model.UpdateHypothesisInput{Status: &status, Note: "n", ObservationIDs: []string{"obs-missing"}})
	if e, ok := AsError(err); !ok || e.Status != 422 {
		t.Fatalf("unknown observation: %v", err)
	}
	_, err = a.UpdateHypothesis(h.ID, model.UpdateHypothesisInput{Status: &status, Note: "n", Title: &empty})
	if e, ok := AsError(err); !ok || e.Status != 422 {
		t.Fatalf("empty title: %v", err)
	}
	hs, _ := a.ListHypotheses(p.ID)
	for _, x := range hs {
		if x.ID == h.ID && (x.Status != "proposed" || len(x.History) != 1) {
			t.Errorf("rejected PATCH was partly applied: %+v", x)
		}
	}
}

// A rejected settings PATCH leaves the project as it was.
func TestUpdateProjectIsAtomic(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateProject(model.CreateProjectInput{Name: "P", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	var patch ProjectPatch
	if err := json.Unmarshal([]byte(`{"name":"renamed","settings":{"overlapPolicy":"last","checksumPolicy":"bogus"}}`), &patch); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UpdateProject(p.ID, patch); err == nil {
		t.Fatal("invalid checksum policy accepted")
	}
	got, _ := a.GetProject(p.ID)
	if got.Name != "P" || got.Settings.OverlapPolicy != "first" {
		t.Errorf("rejected PATCH was partly applied: %+v", got)
	}
}

// An action log imported before the capture of the experiment is linked when
// the capture has been indexed.
func TestActionsLinkedAfterCaptureImport(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Порядок импорта", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	ds := demo.Build(demo.Options{})
	log, err := json.Marshal(ds.Actions)
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.ImportActions(p.ID, "log.json", bytes.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	if res.Linked != 0 || len(res.Warnings) != res.Imported {
		t.Fatalf("no capture yet, nothing can be linked: %+v", res)
	}
	up, err := a.ImportCapture(p.ID, ds.Captures[0].FileName, bytes.NewReader(ds.Captures[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	if j := waitJob(t, a, up.JobID); j.Status != "succeeded" {
		t.Fatalf("import: %+v", j)
	}
	actions, _ := a.ListActions(p.ID)
	sessions, _ := a.ListSessions(p.ID, SessionFilter{})
	linked := 0
	for _, act := range actions {
		if len(act.Links) == 1 && act.Links[0].Method == "auto_time" {
			linked++
		}
	}
	withActions := 0
	for _, s := range sessions {
		withActions += len(s.ActionIDs)
	}
	if linked == 0 || linked != withActions {
		t.Fatalf("linked actions %d, session action ids %d", linked, withActions)
	}
}

// A tampered archive whose project.json points a capture outside of the
// project is rejected; nothing is read or removed by that path.
func TestImportRejectsCapturePathTraversal(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var exported bytes.Buffer
	if err := a.ExportProject(p.ID, &exported); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(exported.Bytes()), int64(exported.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var tampered bytes.Buffer
	zw := zip.NewWriter(&tampered)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "project.json" {
			var st map[string]any
			if err := json.Unmarshal(data, &st); err != nil {
				t.Fatal(err)
			}
			st["captures"].([]any)[0].(map[string]any)["file"] = "../../../../outside.pcapng"
			data, _ = json.Marshal(st)
		}
		w, _ := zw.Create(f.Name)
		w.Write(data)
	}
	zw.Close()
	_, _, err = a.ImportProject("tampered.protolab.zip", bytes.NewReader(tampered.Bytes()))
	if e, ok := AsError(err); !ok || e.Code != "invalid_archive" {
		t.Fatalf("tampered archive: %v", err)
	}
	if list := a.ListProjects(); len(list) != 1 {
		t.Fatalf("projects after a rejected import: %d", len(list))
	}
}

// A run and a report whose process was killed mid-job are not shown as
// running forever after a restart: they fail with an explicit reason.
func TestInterruptedRunsAndReportsFailOnStart(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	// Simulate a process killed while a run and a report were in progress.
	st, err := store.LoadDir(filepath.Join(dir, "projects", p.ID))
	if err != nil {
		t.Fatal(err)
	}
	run := st.Runs[len(st.Runs)-1].Run.ID
	st.Runs[len(st.Runs)-1].Run.Status = "running"
	st.Reports = append(st.Reports, model.Report{ID: "rep-000000000001", ProjectID: p.ID, Status: "pending", RunIDs: []string{}, Artifacts: []model.ReportArtifact{}})
	data, _ := json.Marshal(st)
	if err := store.WriteFileAtomic(filepath.Join(dir, "projects", p.ID, "project.json"), data); err != nil {
		t.Fatal(err)
	}

	b := newTestApp(t, dir)
	got, err := b.GetRun(run)
	if err != nil || got.Status != "failed" || got.Error == "" || got.FinishedAt == "" {
		t.Fatalf("interrupted run after restart: %+v %v", got, err)
	}
	reports, _ := b.ListReports(p.ID)
	if len(reports) != 1 || reports[0].Status != "failed" {
		t.Fatalf("interrupted report after restart: %+v", reports)
	}
	// The change is stored, not only shown.
	st, _ = store.LoadDir(filepath.Join(dir, "projects", p.ID))
	if st.Reports[0].Status != "failed" {
		t.Fatalf("stored report status %q", st.Reports[0].Status)
	}
}
