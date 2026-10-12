package store

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

func newState(id string) *State {
	return &State{Project: model.Project{ID: id, Name: "Тест", CreatedAt: model.Now(), Settings: model.ProjectSettings{OverlapPolicy: "first", TargetPorts: []int{5020}}}}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st := newState("prj-1")
	st.Actions = []model.ActionEntry{{ID: "act-1", Label: "x", Params: map[string]any{"value": 21.0}}}
	if err := s.Save(st); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("prj-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != SchemaVersion || got.Project.Name != "Тест" || got.Actions[0].Params["value"] != 21.0 {
		t.Fatalf("round trip: %+v", got)
	}
	entries, _ := os.ReadDir(s.ProjectDir("prj-1"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp") {
			t.Fatalf("temporary file left behind: %s", e.Name())
		}
	}
	list, errs := s.List()
	if len(list) != 1 || len(errs) != 0 {
		t.Fatalf("list: %d projects, errors %v", len(list), errs)
	}
}

func TestLoadRejectsNewerSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(`{"schemaVersion": 99, "project": {"id": "p"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDir(dir); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("expected newer-schema error, got %v", err)
	}
}

func zipOf(t *testing.T, files map[string]string) *bytes.Reader {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	zw.Close()
	return bytes.NewReader(buf.Bytes())
}

var lim = ImportLimits{MaxFiles: 100, MaxTotalBytes: 1 << 20}

func TestExportExtractRoundTrip(t *testing.T) {
	s, _ := Open(t.TempDir())
	st := newState("prj-2")
	s.Save(st)
	os.MkdirAll(filepath.Join(s.ProjectDir("prj-2"), "captures"), 0o755)
	os.WriteFile(filepath.Join(s.ProjectDir("prj-2"), "captures", "abc.pcapng"), []byte("pcap-bytes"), 0o644)

	var buf bytes.Buffer
	if err := s.ExportZip("prj-2", &buf); err != nil {
		t.Fatal(err)
	}
	dir, err := s.ExtractZip(bytes.NewReader(buf.Bytes()), int64(buf.Len()), lim)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	b, err := os.ReadFile(filepath.Join(dir, "captures", "abc.pcapng"))
	if err != nil || string(b) != "pcap-bytes" {
		t.Fatalf("capture not restored: %q %v", b, err)
	}
	if got, err := LoadDir(dir); err != nil || got.Project.ID != "prj-2" {
		t.Fatalf("state: %+v %v", got, err)
	}
}

func TestExtractRejectsBadArchives(t *testing.T) {
	s, _ := Open(t.TempDir())
	cases := map[string]map[string]string{
		"zip slip":        {"project.json": "{}", "../evil.txt": "x"},
		"absolute path":   {"project.json": "{}", "/etc/evil": "x"},
		"no project.json": {"captures/a.pcap": "x"},
	}
	for name, files := range cases {
		r := zipOf(t, files)
		if _, err := s.ExtractZip(r, r.Size(), lim); !errors.Is(err, ErrBadArchive) {
			t.Errorf("%s: expected ErrBadArchive, got %v", name, err)
		}
	}
	big := zipOf(t, map[string]string{"project.json": "{}", "captures/big": strings.Repeat("A", 2<<20)})
	if _, err := s.ExtractZip(big, big.Size(), lim); !errors.Is(err, ErrBadArchive) {
		t.Errorf("oversized archive: expected ErrBadArchive, got %v", err)
	}
	if entries, _ := os.ReadDir(s.ProjectsDir()); len(entries) != 0 {
		t.Errorf("failed imports left directories: %v", entries)
	}
}

// Values of project.json that become paths are checked when it is loaded: an
// imported archive must not point outside of its project directory.
func TestLoadRejectsPathsOutsideTheProject(t *testing.T) {
	sha := strings.Repeat("ab", 32)
	good := `{"project":{"id":"prj-1"},"captures":[{"id":"cap-1","sha256":"` + sha + `","format":"pcapng","file":"captures/` + sha + `.pcapng"}],` +
		`"reports":[{"id":"rep-1","artifacts":[{"fileName":"report.x.v1.md"}]}]}`
	bad := map[string]string{
		"project id":    strings.Replace(good, `"id":"prj-1"`, `"id":"../prj-1"`, 1),
		"capture file":  strings.Replace(good, `"file":"captures/`+sha+`.pcapng"`, `"file":"../../../etc/passwd"`, 1),
		"capture sha":   strings.ReplaceAll(good, sha, "../x"),
		"report id":     strings.Replace(good, `"id":"rep-1"`, `"id":".."`, 1),
		"artifact name": strings.Replace(good, `"fileName":"report.x.v1.md"`, `"fileName":"../../project.json"`, 1),
	}
	load := func(body string) error {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "project.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := LoadDir(dir)
		return err
	}
	if err := load(good); err != nil {
		t.Fatalf("valid project rejected: %v", err)
	}
	for name, body := range bad {
		if err := load(body); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
