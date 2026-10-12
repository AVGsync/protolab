package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/AVGsync/protolab/internal/jobs"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

func (p *project) dtoLocked() model.Project {
	pr := p.st.Project
	pr.Settings.TargetPorts = slices.Clone(pr.Settings.TargetPorts)
	sessions := 0
	for _, rec := range p.st.Captures {
		sessions += p.captureDTO(rec).SessionCount
	}
	pr.Stats = model.ProjectStats{
		Captures:        len(p.st.Captures),
		Sessions:        sessions,
		Actions:         len(p.st.Actions),
		Observations:    len(p.st.Observations),
		Hypotheses:      len(p.st.Hypotheses),
		Interpretations: len(p.st.Interpretations),
		Runs:            len(p.st.Runs),
	}
	return pr
}

func (p *project) dto() model.Project {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dtoLocked()
}

func (a *App) ListProjects() []model.Project {
	list := a.projectList()
	out := make([]model.Project, len(list))
	for i, p := range list {
		out[i] = p.dto()
	}
	return out
}

func (a *App) GetProject(id string) (model.Project, error) {
	p, err := a.project(id)
	if err != nil {
		return model.Project{}, err
	}
	return p.dto(), nil
}

func validPorts(ports []int) error {
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return invalid("Порт %d вне диапазона 1–65535", port)
		}
	}
	return nil
}

func (a *App) CreateProject(in model.CreateProjectInput) (model.Project, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return model.Project{}, invalid("Укажите название проекта")
	}
	if err := validPorts(in.TargetPorts); err != nil {
		return model.Project{}, err
	}
	now := model.Now()
	st := &store.State{
		Project: model.Project{
			ID:          newID("prj"),
			Name:        name,
			Description: strings.TrimSpace(in.Description),
			CreatedAt:   now,
			UpdatedAt:   now,
			Settings: model.ProjectSettings{
				OverlapPolicy:  "first",
				ChecksumPolicy: "warn",
				TargetPorts:    uniqueSorted(in.TargetPorts),
			},
		},
	}
	p := newProject(a, st)
	if err := a.store.Save(st); err != nil {
		return model.Project{}, err
	}
	a.register(p)
	a.log.Info("project created", "project", p.id, "name", name)
	return p.dto(), nil
}

func uniqueSorted(ports []int) []int {
	out := slices.Clone(ports)
	slices.Sort(out)
	out = slices.Compact(out)
	if out == nil {
		out = []int{}
	}
	return out
}

// SettingsPatch changes some project settings; nil fields stay as they are.
type SettingsPatch struct {
	OverlapPolicy  *string `json:"overlapPolicy"`
	ChecksumPolicy *string `json:"checksumPolicy"`
	TargetPorts    []int   `json:"targetPorts"`
}

type ProjectPatch struct {
	Name        *string        `json:"name"`
	Description *string        `json:"description"`
	Settings    *SettingsPatch `json:"settings"`
}

// UpdateProject changes metadata or reassembly settings. Settings changes
// invalidate cached analyses: sessions are rebuilt and older runs become stale.
func (a *App) UpdateProject(id string, patch ProjectPatch) (model.Project, error) {
	p, err := a.project(id)
	if err != nil {
		return model.Project{}, err
	}
	reanalyze := false
	err = p.update(func(st *store.State) error {
		if patch.Name != nil {
			name := strings.TrimSpace(*patch.Name)
			if name == "" {
				return invalid("Название не может быть пустым")
			}
			st.Project.Name = name
		}
		if patch.Description != nil {
			st.Project.Description = strings.TrimSpace(*patch.Description)
		}
		if s := patch.Settings; s != nil {
			cur := &st.Project.Settings
			if s.OverlapPolicy != nil {
				if *s.OverlapPolicy != "first" && *s.OverlapPolicy != "last" {
					return invalid("Политика перекрытий: first или last")
				}
				reanalyze = reanalyze || *s.OverlapPolicy != cur.OverlapPolicy
				cur.OverlapPolicy = *s.OverlapPolicy
			}
			if s.ChecksumPolicy != nil {
				if *s.ChecksumPolicy != "warn" && *s.ChecksumPolicy != "ignore" {
					return invalid("Политика контрольных сумм: warn или ignore")
				}
				reanalyze = reanalyze || *s.ChecksumPolicy != cur.ChecksumPolicy
				cur.ChecksumPolicy = *s.ChecksumPolicy
			}
			if s.TargetPorts != nil {
				if err := validPorts(s.TargetPorts); err != nil {
					return err
				}
				ports := uniqueSorted(s.TargetPorts)
				reanalyze = reanalyze || !slices.Equal(ports, cur.TargetPorts)
				cur.TargetPorts = ports
			}
		}
		if reanalyze {
			p.invalidate()
		}
		return nil
	})
	if err != nil {
		return model.Project{}, err
	}
	if reanalyze {
		a.log.Info("project settings changed, captures will be re-analysed", "project", id)
		go func() {
			for _, c := range p.captureIDs() {
				_, _ = p.analysis(c)
			}
		}()
	}
	return p.dto(), nil
}

// ExportProject writes the project as a zip archive and returns its file name.
func (a *App) ExportProject(id string, w io.Writer) error {
	p, err := a.project(id)
	if err != nil {
		return err
	}
	// Hold the lock so project.json is not replaced while it is being read.
	p.mu.Lock()
	defer p.mu.Unlock()
	return a.store.ExportZip(p.id, w)
}

// ExportFileName is the suggested name of the export archive.
func (a *App) ExportFileName(id string) string { return id + ".protolab.zip" }

// ImportProject unpacks a project archive. If the project (or any of its
// objects) already exists, every identifier is regenerated so both copies
// can live side by side.
func (a *App) ImportProject(fileName string, r io.Reader) (model.Project, string, error) {
	tmp, err := os.CreateTemp(a.store.ProjectsDir(), ".upload-*.zip")
	if err != nil {
		return model.Project{}, "", err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	maxArchive := a.cfg.Limits.MaxCaptureBytes * 8
	n, err := io.Copy(tmp, io.LimitReader(r, maxArchive+1))
	if err != nil {
		return model.Project{}, "", badRequest("Не удалось принять архив: %v", err)
	}
	if n > maxArchive {
		return model.Project{}, "", &Error{Status: http.StatusRequestEntityTooLarge, Code: "archive_too_large", Message: "Архив больше допустимого размера"}
	}

	dir, err := a.store.ExtractZip(tmp, n, store.ImportLimits{MaxFiles: 10_000, MaxTotalBytes: maxArchive * 2})
	if err != nil {
		if errors.Is(err, store.ErrBadArchive) {
			return model.Project{}, "", &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_archive", Message: err.Error()}
		}
		return model.Project{}, "", err
	}
	defer os.RemoveAll(dir) // no-op once moved

	st, err := store.LoadDir(dir)
	if err != nil {
		return model.Project{}, "", &Error{Status: http.StatusUnprocessableEntity, Code: "invalid_archive", Message: err.Error()}
	}
	if a.collides(st) {
		if st, err = remapIDs(st, dir); err != nil {
			return model.Project{}, "", err
		}
	}
	normalizeState(st)
	for i := range st.Captures {
		st.Captures[i].ProjectID = st.Project.ID
	}
	st.Project.Description = strings.TrimSpace(st.Project.Description)

	dst := a.store.ProjectDir(st.Project.ID)
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return model.Project{}, "", err
	}
	if err := store.WriteFileAtomic(filepath.Join(dir, "project.json"), data); err != nil {
		return model.Project{}, "", err
	}
	if err := os.Rename(dir, dst); err != nil {
		return model.Project{}, "", err
	}
	p := newProject(a, st)
	a.register(p)
	a.log.Info("project imported", "project", p.id, "file", fileName)
	a.resumeIndexing(p) // the archive was made in the middle of a capture import

	job := a.jobs.Start(p.id, "import_project", "Импорт проекта "+cleanFileName(fileName), func(ctx context.Context, progress jobs.Progress) (*model.JobResult, error) {
		ids := p.captureIDs()
		for i, c := range ids {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			progress(float64(i)/float64(max(len(ids), 1)), "Проверка SHA-256 и восстановление потоков")
			if _, err := p.analysis(c); err != nil {
				if e, ok := AsError(err); ok && e.Code == "not_ready" {
					continue // indexed by its own resumed import job
				}
				a.log.Warn("imported capture failed", "capture", c, "err", err)
			}
		}
		return &model.JobResult{Type: "project", ID: p.id}, nil
	})
	return p.dto(), job.ID, nil
}

func (a *App) collides(st *store.State) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, ok := a.projects[st.Project.ID]; ok {
		return true
	}
	tmp := &project{st: st}
	for _, id := range tmp.entityIDs() {
		if _, ok := a.owner[id]; ok {
			return true
		}
	}
	return false
}

var sessionIDPattern = regexp.MustCompile(`"(cap-[0-9a-f]+)-(\d+)"`)

// remapIDs gives every object of an imported project a fresh identifier.
// IDs are random tokens, so replacing their quoted JSON spelling is exact.
func remapIDs(st *store.State, dir string) (*store.State, error) {
	tmp := &project{st: st}
	mapping := map[string]string{st.Project.ID: newID("prj")}
	for _, id := range tmp.entityIDs() {
		prefix, _, ok := strings.Cut(id, "-")
		if !ok {
			prefix = "obj"
		}
		mapping[id] = newID(prefix)
	}
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	text := sessionIDPattern.ReplaceAllStringFunc(string(raw), func(m string) string {
		parts := sessionIDPattern.FindStringSubmatch(m)
		if newCap, ok := mapping[parts[1]]; ok {
			return fmt.Sprintf(`"%s-%s"`, newCap, parts[2])
		}
		return m
	})
	pairs := make([]string, 0, len(mapping)*2)
	for old, nw := range mapping {
		pairs = append(pairs, `"`+old+`"`, `"`+nw+`"`)
	}
	text = strings.NewReplacer(pairs...).Replace(text)

	var out store.State
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, err
	}
	// Finding IDs are derived from the run ID.
	for i := range out.Runs {
		for k := range out.Runs[i].Findings {
			out.Runs[i].Findings[k].ID = fmt.Sprintf("%s-f%d", out.Runs[i].Run.ID, k+1)
		}
	}
	for _, r := range st.Reports {
		oldDir := filepath.Join(dir, "reports", r.ID)
		if _, err := os.Stat(oldDir); err == nil {
			if err := os.Rename(oldDir, filepath.Join(dir, "reports", mapping[r.ID])); err != nil {
				return nil, err
			}
		}
	}
	return &out, nil
}
