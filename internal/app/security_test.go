package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/demo"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/store"
)

func exportArchive(t *testing.T, a *App, projectID string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := a.ExportProject(projectID, &buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rewriteArchive copies an archive, passing project.json through edit and
// appending extra entries (name -> writer of the body).
func rewriteArchive(t *testing.T, archive []byte, edit func(st map[string]any), extra map[string]func(io.Writer)) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "project.json" && edit != nil {
			var st map[string]any
			if err := json.Unmarshal(data, &st); err != nil {
				t.Fatal(err)
			}
			edit(st)
			data, _ = json.Marshal(st)
		}
		w, _ := zw.Create(f.Name)
		w.Write(data)
	}
	for name, write := range extra {
		w, _ := zw.Create(name)
		write(w)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func writeRepeated(b byte, n int) func(io.Writer) {
	return func(w io.Writer) {
		chunk := bytes.Repeat([]byte{b}, 1<<20)
		for ; n > 0; n -= len(chunk) {
			w.Write(chunk[:min(n, len(chunk))])
		}
	}
}

func invalidArchive(t *testing.T, err error) {
	t.Helper()
	if e, ok := AsError(err); !ok || e.Code != "invalid_archive" {
		t.Fatalf("expected invalid_archive, got %v", err)
	}
}

// The same archive imported twice at once (a double click, two tabs): both
// imports must find the id taken by the other, not fail on the directory.
func TestConcurrentImportsOfOneArchive(t *testing.T) {
	src := newTestApp(t, t.TempDir())
	p, err := src.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	archive := exportArchive(t, src, p.ID)

	a := newTestApp(t, t.TempDir())
	const n = 6
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, _, err := a.ImportProject("x.protolab.zip", bytes.NewReader(archive))
			ids[i], errs[i] = got.ID, err
		}()
	}
	wg.Wait()
	seen := map[string]bool{}
	for i := range n {
		if errs[i] != nil {
			t.Fatalf("import %d: %v", i, errs[i])
		}
		if seen[ids[i]] {
			t.Fatalf("two imports got the project id %s", ids[i])
		}
		seen[ids[i]] = true
	}
	// Every copy owns its own captures.
	for id := range seen {
		caps, err := a.ListCaptures(id)
		if err != nil || len(caps) != 3 {
			t.Fatalf("project %s: %d captures, %v", id, len(caps), err)
		}
		for _, c := range caps {
			if got, err := a.GetCapture(c.ID); err != nil || got.ProjectID != id {
				t.Fatalf("capture %s resolves to %q, want %s (%v)", c.ID, got.ProjectID, id, err)
			}
		}
	}
}

// Ids of an archive become directory names and URL segments; remapIDs replaces
// them as JSON tokens. Only the form ProtoLab generates is accepted.
func TestImportRefusesForeignIDs(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	archive := exportArchive(t, a, p.ID)
	for name, edit := range map[string]func(st map[string]any){
		"project id with a newline": func(st map[string]any) { st["project"].(map[string]any)["id"] = "prj-a\nb" },
		"project id with ?#%":       func(st map[string]any) { st["project"].(map[string]any)["id"] = "prj-a?b#c%2F" },
		"long project id":           func(st map[string]any) { st["project"].(map[string]any)["id"] = strings.Repeat("x", 300) },
		"capture id with <>": func(st map[string]any) {
			st["captures"].([]any)[0].(map[string]any)["id"] = "cap-<1>"
		},
	} {
		_, _, err := a.ImportProject("x.zip", bytes.NewReader(rewriteArchive(t, archive, edit, nil)))
		if e, ok := AsError(err); !ok || e.Code != "invalid_archive" {
			t.Errorf("%s: expected invalid_archive, got %v", name, err)
		}
	}
	if n := len(a.ListProjects()); n != 1 {
		t.Fatalf("projects after rejected imports: %d", n)
	}
}

// A zip bomb is refused from the declared sizes before anything is written.
func TestImportRefusesZipBombs(t *testing.T) {
	t.Run("ratio", func(t *testing.T) {
		limits := DefaultLimits()
		limits.MaxCaptureBytes = 1 << 20 // archive limit 8 MiB
		a, err := New(Config{DataDir: t.TempDir(), Limits: limits, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(a.Close)
		p, err := a.CreateProject(model.CreateProjectInput{Name: "bomb"})
		if err != nil {
			t.Fatal(err)
		}
		// ~12 KiB that unpack to 12 MiB, within twice the archive limit.
		bomb := rewriteArchive(t, exportArchive(t, a, p.ID), func(st map[string]any) {
			st["project"].(map[string]any)["id"] = "prj-0123456789ab"
		}, map[string]func(io.Writer){"captures/zeros": writeRepeated(0, 12<<20)})
		_, _, err = a.ImportProject("bomb.zip", bytes.NewReader(bomb))
		invalidArchive(t, err)
	})
	t.Run("project.json", func(t *testing.T) {
		a := newTestApp(t, t.TempDir())
		p, err := a.CreateProject(model.CreateProjectInput{Name: "big state"})
		if err != nil {
			t.Fatal(err)
		}
		archive := exportArchive(t, a, p.ID)
		zr, _ := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		rc, _ := zr.File[0].Open()
		state, _ := io.ReadAll(rc)
		rc.Close()
		var st map[string]any
		json.Unmarshal(state, &st)
		st["project"].(map[string]any)["id"] = "prj-0123456789ab"
		state, _ = json.Marshal(st)
		// Valid JSON, read into memory whole by LoadDir. Stored, not
		// compressed: the size alone is refused, whatever the ratio.
		out, err := os.Create(filepath.Join(t.TempDir(), "big.zip"))
		if err != nil {
			t.Fatal(err)
		}
		defer out.Close()
		zw := zip.NewWriter(out)
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: "project.json", Method: zip.Store})
		w.Write(state)
		writeRepeated(' ', store.DefaultMaxStateBytes+1)(w)
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		out.Seek(0, io.SeekStart)
		_, _, err = a.ImportProject("big.zip", out)
		invalidArchive(t, err)
	})
}

// blockingWriter stalls the first write until released, like a download that
// is not being read.
type blockingWriter struct {
	started, release chan struct{}
	once             sync.Once
	buf              bytes.Buffer
}

func (w *blockingWriter) Write(b []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.buf.Write(b)
}

// A stalled export download must not lock the project: every other request
// to it (and the project list) would hang until the browser reads again.
func TestExportDoesNotBlockTheProject(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	w := &blockingWriter{started: make(chan struct{}), release: make(chan struct{})}
	exported := make(chan error, 1)
	go func() { exported <- a.ExportProject(p.ID, w) }()
	<-w.started

	renamed := "Переименован во время экспорта"
	updated := make(chan error, 1)
	go func() {
		_, err := a.UpdateProject(p.ID, ProjectPatch{Name: &renamed})
		updated <- err
	}()
	select {
	case err := <-updated:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		close(w.release)
		<-exported
		<-updated
		t.Fatal("the project stayed locked while the export download was stalled")
	}
	close(w.release)
	if err := <-exported; err != nil {
		t.Fatal(err)
	}

	// The archive is complete: the state as of the start of the export.
	b := newTestApp(t, t.TempDir())
	got, _, err := b.ImportProject("x.zip", bytes.NewReader(w.buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != p.Name || got.Stats.Captures != 3 || got.Stats.Runs != 5 {
		t.Fatalf("imported export: %+v", got)
	}
}

// An action log over the limit is refused, not imported cut at the limit with
// a truncated last record.
func TestImportActionsRefusesOversizedLog(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateProject(model.CreateProjectInput{Name: "log"})
	if err != nil {
		t.Fatal(err)
	}
	log := "2026-09-14T10:00:00Z,other,Запись,," + strings.Repeat("r", maxActionLogBytes)
	_, err = a.ImportActions(p.ID, "log.csv", strings.NewReader(log))
	if e, ok := AsError(err); !ok || e.Code != "action_log_too_large" || e.Status != 413 {
		t.Fatalf("oversized log: %v", err)
	}
	if acts, _ := a.ListActions(p.ID); len(acts) != 0 {
		t.Fatalf("actions imported from an oversized log: %d", len(acts))
	}
}

// offset+length must not wrap around: an observation outside of the stream is
// refused instead of being stored with an empty snapshot.
func TestObservationRangeDoesNotOverflow(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sessions, _ := a.ListSessions(p.ID, SessionFilter{})
	s := sessions[0]
	_, err = a.CreateObservation(p.ID, model.CreateObservationInput{
		Note: "overflow",
		Source: model.SourceRef{
			CaptureID: s.CaptureID, CaptureSHA256: s.Capture.SHA256, SessionID: s.ID,
			Direction: model.C2S, Offset: math.MaxInt - 10, Length: 100,
		},
	})
	if e, ok := AsError(err); !ok || e.Code != "validation_error" {
		t.Fatalf("observation past the end of the stream: %v", err)
	}
}

// Settings changed while a capture was being indexed: its sessions are built
// with the new settings. The re-analysis after a settings change skips
// captures that are still indexing, so a stale result would stay.
func TestSettingsChangedDuringIndexing(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateProject(model.CreateProjectInput{Name: "settings", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	pr, _ := a.project(p.ID)
	c := demo.Build(demo.Options{}).Captures[0]
	rec, err := a.storeUpload(pr, c.FileName, bytes.NewReader(c.Data))
	if err != nil {
		t.Fatal(err)
	}
	ports := []int{1}
	var once sync.Once
	progress := func(float64, string) {
		// Called while the file is read, after the settings were taken.
		once.Do(func() {
			if _, err := a.UpdateProject(p.ID, ProjectPatch{Settings: &SettingsPatch{TargetPorts: ports}}); err != nil {
				t.Error(err)
			}
		})
	}
	if _, err := a.indexCapture(context.Background(), pr, rec.ID, progress); err != nil {
		t.Fatal(err)
	}
	sessions, _ := a.ListSessions(p.ID, SessionFilter{})
	if len(sessions) == 0 {
		t.Fatal("no sessions")
	}
	for _, s := range sessions {
		if s.IsTarget {
			t.Fatalf("session %s (server port %d) is a target with target ports %v", s.ID, s.Server.Port, ports)
		}
	}
	if got, _ := a.GetCapture(rec.ID); got.TargetSessionCount != 0 {
		t.Fatalf("target sessions of the capture: %d", got.TargetSessionCount)
	}
}

// Temporary files of an import or upload killed midway are removed on start;
// fresh ones (another process at work) are kept.
func TestStaleTemporaryFilesRemovedOnStart(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	p, err := a.CreateProject(model.CreateProjectInput{Name: "temp"})
	if err != nil {
		t.Fatal(err)
	}
	a.Close()

	projects := filepath.Join(dir, "projects")
	old := time.Now().Add(-2 * time.Hour)
	stale := []string{
		filepath.Join(projects, ".upload-1.zip"),
		filepath.Join(projects, ".import-1", "captures", "x.pcap"),
		filepath.Join(projects, p.ID, ".tmp-1"),
		filepath.Join(projects, p.ID, "captures", ".upload-1"),
		filepath.Join(projects, p.ID, "reports", "rep-1", ".tmp-2"),
	}
	fresh := filepath.Join(projects, ".upload-2.zip")
	for _, f := range append(stale, fresh) {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range append(stale, filepath.Join(projects, ".import-1")) {
		os.Chtimes(f, old, old)
	}

	b := newTestApp(t, dir)
	for _, f := range append(stale, filepath.Join(projects, ".import-1")) {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("stale temporary file kept: %s", f)
		}
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh temporary file removed: %v", err)
	}
	if _, err := b.GetProject(p.ID); err != nil {
		t.Fatal(err)
	}
}

// A copy of a project folder next to it (a backup under another name) keeps
// the id inside project.json. It must not replace the project: its state
// would be shown and then saved over the original's project.json.
func TestProjectFolderCopyDoesNotShadowTheProject(t *testing.T) {
	dir := t.TempDir()
	a := newTestApp(t, dir)
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Резервная копия"})
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	projects := filepath.Join(dir, "projects")
	backup := filepath.Join(projects, "zzz-backup")
	if err := os.MkdirAll(backup, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(projects, p.ID, "project.json"))
	if err := os.WriteFile(filepath.Join(backup, "project.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// The project goes on after the backup was made.
	b := newTestApp(t, dir)
	current := "Текущее состояние"
	if _, err := b.UpdateProject(p.ID, ProjectPatch{Name: &current}); err != nil {
		t.Fatal(err)
	}
	b.Close()

	c := newTestApp(t, dir)
	got, err := c.GetProject(p.ID)
	if err != nil || got.Name != current {
		t.Fatalf("project after restart: %q %v (the backup copy shadowed it)", got.Name, err)
	}
	if n := len(c.ListProjects()); n != 1 {
		t.Fatalf("projects: %d", n)
	}
	// A change saves over the project's own state, not over the backup's.
	desc := "ещё изменение"
	if _, err := c.UpdateProject(p.ID, ProjectPatch{Description: &desc}); err != nil {
		t.Fatal(err)
	}
	st, err := store.LoadDir(filepath.Join(projects, p.ID))
	if err != nil || st.Project.Name != current || st.Project.Description != desc {
		t.Fatalf("stored project: %+v %v", st.Project, err)
	}
}

// A framing that cuts a long stream into tiny messages hits the message limit
// before the messages are built: with 1-byte messages a 16 MiB stream needed
// 8 GiB of memory before the limit was checked, and preview had no limit.
func TestMessageLimitIsCheckedBeforeParsing(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxMessages = 1000
	a, err := New(Config{DataDir: t.TempDir(), Limits: limits, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	p, err := a.CreateProject(model.CreateProjectInput{Name: "limit", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	// One connection with 1 MiB from the client.
	b := newCaptureBuilder(t)
	const cli, srv = "10.0.0.1:40000", "10.0.0.2:5020"
	b.tcp(cli, srv, 100, 0, netpkt.SYN, "")
	b.tcp(srv, cli, 500, 101, netpkt.SYN|netpkt.ACK, "")
	chunk := strings.Repeat("x", 1456)
	seq := uint32(101)
	for range 720 {
		b.tcp(cli, srv, seq, 501, netpkt.PSH|netpkt.ACK, chunk)
		seq += uint32(len(chunk))
	}
	importSync(t, a, p, "bulk.pcapng", b.bytes())
	sessions, _ := a.ListSessions(p.ID, SessionFilter{})
	if len(sessions) != 1 || sessions[0].Bytes.C2S != 720*1456 {
		t.Fatalf("sessions: %+v", sessions)
	}
	bytesSpec := json.RawMessage(`{"format":"protolab/v1","framing":{"kind":"fixed","size":1},"messages":[{"id":"b","match":[],"fields":[{"name":"b","type":"u8","offset":0}]}]}`)
	interp, err := a.CreateInterpretation(p.ID, CreateInterpretationInput{Name: "bytes", Spec: bytesSpec})
	if err != nil {
		t.Fatal(err)
	}

	for name, call := range map[string]func() error{
		"session messages": func() error { _, err := a.SessionMessages(sessions[0].ID, interp.LatestVersionID); return err },
		"preview":          func() error { _, err := a.PreviewSpec(bytesSpec, sessions[0].ID); return err },
		"project messages": func() error {
			_, err := a.ProjectMessages(p.ID, MessagesQuery{VersionID: interp.LatestVersionID})
			return err
		},
	} {
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		err := call()
		runtime.ReadMemStats(&after)
		if e, ok := AsError(err); !ok || e.Code != "message_limit" {
			t.Errorf("%s: expected message_limit, got %v", name, err)
		}
		// 1M parsed messages take about 650 MiB.
		if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 64<<20 {
			t.Errorf("%s: %d MiB allocated before the limit was applied", name, alloc>>20)
		}
	}
}

// A remapped id keeps the archive's prefix only when it is short: the new id
// must itself be importable when the project is exported again.
func TestRemappedIDsStayImportable(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 60) + "-1"
	archive := rewriteArchive(t, exportArchive(t, a, p.ID), func(st map[string]any) {
		st["actions"].([]any)[0].(map[string]any)["id"] = long
	}, nil)
	first, _, err := a.ImportProject("x.zip", bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	// Exported and imported again: every id of the copy was remapped once.
	if _, _, err := a.ImportProject("y.zip", bytes.NewReader(exportArchive(t, a, first.ID))); err != nil {
		t.Fatalf("re-import of an imported project: %v", err)
	}
}
