package app

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/AVGsync/protolab/internal/jobs"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/report"
	"github.com/AVGsync/protolab/internal/store"
)

func reportURL(reportID, file string) string {
	return "/api/v1/reports/" + reportID + "/files/" + file
}

func (a *App) ListReports(projectID string) ([]model.Report, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	out := slices.Clone(p.st.Reports)
	p.mu.Unlock()
	slices.SortStableFunc(out, func(x, y model.Report) int { return strings.Compare(y.CreatedAt, x.CreatedAt) })
	for i := range out {
		out[i].Artifacts = artifactsDTO(out[i])
	}
	return nonNil(out), nil
}

// artifactsDTO derives download URLs from the report ID on every read, so they
// stay correct after an import regenerated the ID.
func artifactsDTO(r model.Report) []model.ReportArtifact {
	out := make([]model.ReportArtifact, len(r.Artifacts))
	for i, a := range r.Artifacts {
		a.URL = reportURL(r.ID, a.FileName)
		out[i] = a
	}
	return out
}

// CreateReport renders the report artifacts in a background job. Artifacts are
// written once and never modified, so a report is a snapshot of the research.
func (a *App) CreateReport(projectID string, in model.CreateReportInput) (model.Report, string, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.Report{}, "", err
	}
	var rep model.Report
	err = p.update(func(st *store.State) error {
		vi := findIndex(st.Versions, func(v *model.InterpretationVersion) bool { return v.ID == in.VersionID })
		if vi < 0 {
			return notFound("Версия интерпретации")
		}
		v := st.Versions[vi]
		ii := findIndex(st.Interpretations, func(x *model.Interpretation) bool { return x.ID == v.InterpretationID })
		if ii < 0 {
			return notFound("Интерпретация")
		}
		for _, rid := range in.RunIDs {
			ri := findIndex(st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == rid })
			if ri < 0 {
				return invalid("Проверка %s не найдена", rid)
			}
			if st.Runs[ri].Run.Status != "succeeded" {
				return invalid("Проверка %s не завершена", rid)
			}
			// The examples file replays every run with the report's spec.
			if r := st.Runs[ri].Run; r.VersionID != v.ID {
				return invalid("Проверка %s выполнена на v%d, а отчёт формируется по v%d", rid, r.Version, v.Version)
			}
		}
		title := strings.TrimSpace(in.Title)
		if title == "" {
			title = fmt.Sprintf("%s v%d", st.Interpretations[ii].Name, v.Version)
		}
		rep = model.Report{
			ID:                 newID("rep"),
			ProjectID:          p.id,
			Title:              title,
			CreatedAt:          model.Now(),
			VersionID:          v.ID,
			Version:            v.Version,
			InterpretationName: st.Interpretations[ii].Name,
			RunIDs:             nonNil(slices.Clone(in.RunIDs)),
			Status:             "pending",
			Artifacts:          []model.ReportArtifact{},
			ReproduceCommand:   report.ReproduceCommand(st.Interpretations[ii].Name, v.Version),
		}
		st.Reports = append(st.Reports, rep)
		return nil
	})
	if err != nil {
		return model.Report{}, "", err
	}
	a.index(p.id, rep.ID)

	reportID := rep.ID
	job := a.jobs.Start(p.id, "report", "Отчёт «"+rep.Title+"»", func(ctx context.Context, progress jobs.Progress) (*model.JobResult, error) {
		if err := a.renderReport(ctx, p, reportID, progress); err != nil {
			_ = p.update(func(st *store.State) error {
				if i := findIndex(st.Reports, func(r *model.Report) bool { return r.ID == reportID }); i >= 0 {
					st.Reports[i].Status = "failed"
				}
				return nil
			})
			return nil, err
		}
		return &model.JobResult{Type: "report", ID: reportID}, nil
	})
	return rep, job.ID, nil
}

func (a *App) renderReport(ctx context.Context, p *project, reportID string, progress jobs.Progress) error {
	progress(0.1, "Сбор оснований")
	in, err := a.reportInput(p, reportID)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	progress(0.5, "Рендер отчёта")
	files, err := report.Render(in)
	if err != nil {
		return err
	}
	progress(0.8, "Запись артефактов")
	dir := filepath.Join(p.dir(), "reports", reportID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	artifacts := make([]model.ReportArtifact, 0, len(files))
	for _, f := range files {
		if err := store.WriteFileAtomic(filepath.Join(dir, f.Name), f.Data); err != nil {
			return err
		}
		sum := sha256.Sum256(f.Data)
		artifacts = append(artifacts, model.ReportArtifact{
			Kind:      f.Kind,
			FileName:  f.Name,
			URL:       reportURL(reportID, f.Name),
			SizeBytes: int64(len(f.Data)),
			SHA256:    hex.EncodeToString(sum[:]),
		})
	}
	return p.update(func(st *store.State) error {
		i := findIndex(st.Reports, func(r *model.Report) bool { return r.ID == reportID })
		if i < 0 {
			return notFound("Отчёт")
		}
		st.Reports[i].Status = "ready"
		st.Reports[i].Artifacts = artifacts
		return nil
	})
}

// reportInput gathers everything the report needs; sessions are referenced by
// capture path + SHA-256 + tcp.stream so the examples file is portable.
func (a *App) reportInput(p *project, reportID string) (report.Input, error) {
	p.mu.Lock()
	ri := findIndex(p.st.Reports, func(r *model.Report) bool { return r.ID == reportID })
	if ri < 0 {
		p.mu.Unlock()
		return report.Input{}, notFound("Отчёт")
	}
	rep := p.st.Reports[ri]
	in := report.Input{
		Report:       rep,
		Project:      p.dtoLocked(),
		Hypotheses:   slices.Clone(p.st.Hypotheses),
		Observations: slices.Clone(p.st.Observations),
	}
	vi := findIndex(p.st.Versions, func(v *model.InterpretationVersion) bool { return v.ID == rep.VersionID })
	if vi < 0 {
		p.mu.Unlock()
		return report.Input{}, notFound("Версия интерпретации")
	}
	in.Version = p.st.Versions[vi]
	if ii := findIndex(p.st.Interpretations, func(x *model.Interpretation) bool { return x.ID == in.Version.InterpretationID }); ii >= 0 {
		in.Interpretation = p.st.Interpretations[ii]
	}
	type pending struct {
		run    store.RunRecord
		corpus model.Corpus
	}
	var runs []pending
	for _, rid := range rep.RunIDs {
		i := findIndex(p.st.Runs, func(r *store.RunRecord) bool { return r.Run.ID == rid })
		if i < 0 {
			continue
		}
		rec := p.st.Runs[i]
		rec.Run = p.runDTO(rec)
		var corpus model.Corpus
		if ci := findIndex(p.st.Corpora, func(c *model.Corpus) bool { return c.ID == rec.Run.CorpusID }); ci >= 0 {
			corpus = p.st.Corpora[ci]
		}
		runs = append(runs, pending{rec, corpus})
	}
	captureFile := map[string]store.CaptureRecord{}
	for _, c := range p.st.Captures {
		captureFile[c.ID] = c
	}
	p.mu.Unlock()

	for _, r := range runs {
		rd := report.RunData{Run: r.run.Run, Findings: r.run.Findings, Corpus: r.corpus}
		for _, sid := range r.corpus.SessionIDs {
			sd, err := p.session(sid)
			if err != nil {
				continue
			}
			c := captureFile[sd.dto.CaptureID]
			rd.Sessions = append(rd.Sessions, report.SessionRef{
				Capture:   c.File,
				SHA256:    c.SHA256,
				TCPStream: sd.dto.Index,
				Client:    fmt.Sprintf("%s:%d", sd.dto.Client.IP, sd.dto.Client.Port),
				Server:    fmt.Sprintf("%s:%d", sd.dto.Server.IP, sd.dto.Server.Port),
				Actions:   p.linkedActions(sid),
			})
		}
		in.Runs = append(in.Runs, rd)
	}
	return in, nil
}

// ReportFile returns the path and content type of a report artifact.
func (a *App) ReportFile(reportID, name string) (string, string, error) {
	p, err := a.projectOf(reportID, "Отчёт")
	if err != nil {
		return "", "", err
	}
	p.mu.Lock()
	i := findIndex(p.st.Reports, func(r *model.Report) bool { return r.ID == reportID })
	var ok bool
	if i >= 0 {
		for _, art := range p.st.Reports[i].Artifacts {
			if art.FileName == name {
				ok = true
			}
		}
	}
	p.mu.Unlock()
	if !ok {
		return "", "", notFound("Файл отчёта")
	}
	ctype := "application/json"
	if strings.HasSuffix(name, ".md") {
		ctype = "text/markdown; charset=utf-8"
	}
	return filepath.Join(p.dir(), "reports", reportID, name), ctype, nil
}

// ReportBundle returns the file name and the writer of a ready report as one
// zip: its artifacts and the captures its examples file refers to, laid out so
// that `protolab verify --examples examples.<x>.json` works in the unpacked
// directory. It is the hand-over package: readable report, machine-readable
// interpretation, example inputs and expected results.
func (a *App) ReportBundle(reportID string) (string, func(io.Writer) error, error) {
	p, err := a.projectOf(reportID, "Отчёт")
	if err != nil {
		return "", nil, err
	}
	p.mu.Lock()
	i := findIndex(p.st.Reports, func(r *model.Report) bool { return r.ID == reportID })
	if i < 0 {
		p.mu.Unlock()
		return "", nil, notFound("Отчёт")
	}
	rep := p.st.Reports[i]
	captures := map[string]bool{}
	for _, c := range p.st.Captures {
		captures[c.File] = true // checked paths inside the project (store.LoadDir)
	}
	p.mu.Unlock()
	if rep.Status != "ready" {
		return "", nil, conflict("not_ready", "Отчёт ещё не сформирован")
	}

	type entry struct {
		name, path string
		stored     bool
	}
	dir := filepath.Join(p.dir(), "reports", rep.ID)
	var files []entry
	name := rep.ID + ".zip"
	seen := map[string]bool{}
	for _, art := range rep.Artifacts {
		files = append(files, entry{name: art.FileName, path: filepath.Join(dir, art.FileName)})
		switch art.Kind {
		case "report_md":
			name = strings.TrimSuffix(art.FileName, ".md") + ".zip"
		case "examples":
			raw, err := os.ReadFile(filepath.Join(dir, art.FileName))
			if err != nil {
				return "", nil, err
			}
			var ex report.Examples
			if err := json.Unmarshal(raw, &ex); err != nil {
				return "", nil, err
			}
			for _, c := range ex.Corpora {
				for _, s := range c.Sessions {
					if captures[s.Capture] && !seen[s.Capture] {
						seen[s.Capture] = true
						files = append(files, entry{name: s.Capture, path: filepath.Join(p.dir(), filepath.FromSlash(s.Capture)), stored: true})
					}
				}
			}
		}
	}
	write := func(w io.Writer) error {
		zw := zip.NewWriter(w)
		for _, f := range files {
			hdr := &zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: time.Now()}
			if f.stored {
				hdr.Method = zip.Store // captures compress poorly
			}
			fw, err := zw.CreateHeader(hdr)
			if err != nil {
				return err
			}
			src, err := os.Open(f.path)
			if err != nil {
				return err
			}
			_, err = io.Copy(fw, src)
			src.Close()
			if err != nil {
				return err
			}
		}
		return zw.Close()
	}
	return name, write, nil
}
