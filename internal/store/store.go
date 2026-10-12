// Package store keeps projects on disk.
//
// Layout of the data directory:
//
//	<data>/projects/<projectId>/project.json      all metadata of the project
//	<data>/projects/<projectId>/captures/<sha256>.pcap[ng]   original capture files, never modified
//	<data>/projects/<projectId>/reports/<reportId>/...       generated report artifacts
//
// Every path inside a project directory is relative, so a project directory
// can be copied or zipped and opened on another machine.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/AVGsync/protolab/internal/model"
)

// SchemaVersion of project.json.
const SchemaVersion = 1

const stateFile = "project.json"

// CaptureRecord is a capture plus the relative path of its original file.
type CaptureRecord struct {
	model.Capture
	File string `json:"file"` // relative to the project directory
}

// RunRecord is a verification run with its findings.
type RunRecord struct {
	Run      model.VerificationRun `json:"run"`
	Findings []model.Finding       `json:"findings"`
}

// State is everything stored in project.json.
type State struct {
	SchemaVersion   int                           `json:"schemaVersion"`
	Project         model.Project                 `json:"project"`
	Captures        []CaptureRecord               `json:"captures"`
	Actions         []model.ActionEntry           `json:"actions"`
	Interpretations []model.Interpretation        `json:"interpretations"`
	Versions        []model.InterpretationVersion `json:"versions"`
	Hypotheses      []model.Hypothesis            `json:"hypotheses"`
	Observations    []model.Observation           `json:"observations"`
	Corpora         []model.Corpus                `json:"corpora"`
	Runs            []RunRecord                   `json:"runs"`
	Reports         []model.Report                `json:"reports"`
}

// Store is the root data directory.
type Store struct {
	root string
}

// Open creates the directory structure if needed.
func Open(root string) (*Store, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "projects"), 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	return &Store{root: abs}, nil
}

// Root is the absolute data directory.
func (s *Store) Root() string { return s.root }

// ProjectsDir holds one directory per project.
func (s *Store) ProjectsDir() string { return filepath.Join(s.root, "projects") }

// ProjectDir is the directory of a project.
func (s *Store) ProjectDir(id string) string { return filepath.Join(s.ProjectsDir(), id) }

// List loads every project in the data directory. Broken projects are
// reported through the errs map instead of failing the whole listing.
func (s *Store) List() (states []*State, errs map[string]error) {
	errs = map[string]error{}
	entries, err := os.ReadDir(s.ProjectsDir())
	if err != nil {
		errs[""] = err
		return nil, errs
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		st, err := s.Load(e.Name())
		if err != nil {
			errs[e.Name()] = err
			continue
		}
		states = append(states, st)
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Project.CreatedAt > states[j].Project.CreatedAt })
	return states, errs
}

// Load reads project.json of a project directory.
func (s *Store) Load(id string) (*State, error) {
	return LoadDir(s.ProjectDir(id))
}

// LoadDir reads project.json from any directory (used for imports).
func LoadDir(dir string) (*State, error) {
	b, err := os.ReadFile(filepath.Join(dir, stateFile))
	if err != nil {
		return nil, err
	}
	var st State
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("%s: %w", stateFile, err)
	}
	if st.SchemaVersion > SchemaVersion {
		return nil, fmt.Errorf("project format v%d is newer than this program supports (v%d)", st.SchemaVersion, SchemaVersion)
	}
	if st.Project.ID == "" {
		return nil, errors.New("project.json has no project id")
	}
	return &st, nil
}

// Save writes project.json atomically: a crash leaves either the old or the new file.
func (s *Store) Save(st *State) error {
	st.SchemaVersion = SchemaVersion
	dir := s.ProjectDir(st.Project.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(dir, stateFile), b)
}

// WriteFileAtomic writes data to a temporary file in the same directory,
// syncs it and renames it over path.
func WriteFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// DirSize returns the total size of regular files under dir.
func DirSize(dir string) int64 {
	var total int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
