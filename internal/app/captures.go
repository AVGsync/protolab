package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/AVGsync/protolab/internal/jobs"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/store"
)

// captureMagic recognises the first four bytes of pcap and pcapng files.
func captureMagic(b []byte) (format, ext string, ok bool) {
	if len(b) < 4 {
		return "", "", false
	}
	switch {
	case bytes.Equal(b[:4], []byte{0x0a, 0x0d, 0x0d, 0x0a}):
		return "pcapng", "pcapng", true
	case bytes.Equal(b[:4], []byte{0xd4, 0xc3, 0xb2, 0xa1}), bytes.Equal(b[:4], []byte{0xa1, 0xb2, 0xc3, 0xd4}),
		bytes.Equal(b[:4], []byte{0x4d, 0x3c, 0xb2, 0xa1}), bytes.Equal(b[:4], []byte{0xa1, 0xb2, 0x3c, 0x4d}):
		return "pcap", "pcap", true
	}
	return "", "", false
}

func cleanFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	for len(name) > 200 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" || name == "." || name == "/" {
		return "capture"
	}
	return name
}

// ImportCapture stores an uploaded capture unchanged and starts indexing it.
func (a *App) ImportCapture(projectID, fileName string, body io.Reader) (model.UploadCaptureResult, error) {
	p, err := a.project(projectID)
	if err != nil {
		return model.UploadCaptureResult{}, err
	}
	rec, err := a.storeUpload(p, fileName, body)
	if err != nil {
		return model.UploadCaptureResult{}, err
	}
	captureID := rec.ID
	job := a.jobs.Start(p.id, "import_capture", "Импорт "+rec.FileName, func(ctx context.Context, progress jobs.Progress) (*model.JobResult, error) {
		return a.indexCapture(ctx, p, captureID, progress)
	})
	p.mu.Lock()
	if i := findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == captureID }); i >= 0 {
		p.st.Captures[i].JobID = job.ID
		rec.JobID = job.ID
	}
	p.mu.Unlock()
	a.log.Info("capture uploaded", "project", p.id, "file", rec.FileName, "bytes", rec.SizeBytes)
	return model.UploadCaptureResult{Capture: rec.Capture, JobID: job.ID}, nil
}

// resumeIndexing restarts indexing of captures left in the "indexing" state
// by a previous run that stopped before the import job finished.
func (a *App) resumeIndexing(p *project) {
	p.mu.Lock()
	var pending []store.CaptureRecord
	for _, c := range p.st.Captures {
		if c.Status == "indexing" {
			pending = append(pending, c)
		}
	}
	p.mu.Unlock()
	for _, rec := range pending {
		captureID := rec.ID
		job := a.jobs.Start(p.id, "import_capture", "Импорт "+rec.FileName, func(ctx context.Context, progress jobs.Progress) (*model.JobResult, error) {
			return a.indexCapture(ctx, p, captureID, progress)
		})
		p.mu.Lock()
		if i := findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == captureID }); i >= 0 {
			p.st.Captures[i].JobID = job.ID
		}
		p.mu.Unlock()
		a.log.Info("capture indexing resumed", "project", p.id, "file", rec.FileName)
	}
}

// importCaptureSync stores and indexes a capture in the calling goroutine (demo project, tests).
func (a *App) importCaptureSync(ctx context.Context, p *project, fileName string, body io.Reader) (model.Capture, error) {
	rec, err := a.storeUpload(p, fileName, body)
	if err != nil {
		return model.Capture{}, err
	}
	if _, err := a.indexCapture(ctx, p, rec.ID, func(float64, string) {}); err != nil {
		return model.Capture{}, err
	}
	return a.GetCapture(rec.ID)
}

// storeUpload streams the body into the project's captures directory while
// hashing it, checks size and format, rejects duplicates and registers the
// capture with status "indexing".
func (a *App) storeUpload(p *project, fileName string, body io.Reader) (store.CaptureRecord, error) {
	dir := filepath.Join(p.dir(), "captures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return store.CaptureRecord{}, err
	}
	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return store.CaptureRecord{}, err
	}
	tmpName := tmp.Name()
	keep := false
	defer func() {
		if !keep {
			os.Remove(tmpName)
		}
	}()

	limit := a.cfg.Limits.MaxCaptureBytes
	hasher := sha256.New()
	head := &headWriter{max: 4}
	n, err := io.Copy(io.MultiWriter(tmp, hasher, head), io.LimitReader(body, limit+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return store.CaptureRecord{}, badRequest("Не удалось принять файл: %v", err)
	}
	if n > limit {
		return store.CaptureRecord{}, &Error{
			Status:  http.StatusRequestEntityTooLarge,
			Code:    "capture_too_large",
			Message: fmt.Sprintf("Файл больше заявленного предела %d МиБ", limit>>20),
		}
	}
	format, ext, ok := captureMagic(head.buf)
	if !ok {
		return store.CaptureRecord{}, &Error{
			Status:  http.StatusUnprocessableEntity,
			Code:    "unsupported_format",
			Message: "Файл не является PCAP/PCAPNG (неизвестная сигнатура)",
			Details: map[string]string{"fileName": fileName},
		}
	}
	sum := hex.EncodeToString(hasher.Sum(nil))
	rel := "captures/" + sum + "." + ext

	var rec store.CaptureRecord
	err = p.update(func(st *store.State) error {
		if i := findIndex(st.Captures, func(c *storeCapture) bool { return c.SHA256 == sum }); i >= 0 {
			return &Error{
				Status:  http.StatusConflict,
				Code:    "duplicate_capture",
				Message: fmt.Sprintf("Эта запись уже импортирована как %s", st.Captures[i].FileName),
				Details: map[string]string{"captureId": st.Captures[i].ID},
			}
		}
		if err := os.Rename(tmpName, filepath.Join(p.dir(), filepath.FromSlash(rel))); err != nil {
			return err
		}
		keep = true
		rec = store.CaptureRecord{
			Capture: model.Capture{
				ID:          newID("cap"),
				ProjectID:   p.id,
				FileName:    cleanFileName(fileName),
				Format:      format,
				SizeBytes:   n,
				SHA256:      sum,
				Integrity:   "ok",
				ImportedAt:  model.Now(),
				Status:      "indexing",
				Diagnostics: []model.Diagnostic{},
			},
			File: rel,
		}
		st.Captures = append(st.Captures, rec)
		return nil
	})
	if err != nil {
		return store.CaptureRecord{}, err
	}
	a.index(p.id, rec.ID)
	return rec, nil
}

// headWriter keeps the first max bytes written to it.
type headWriter struct {
	buf []byte
	max int
}

func (h *headWriter) Write(b []byte) (int, error) {
	if room := h.max - len(h.buf); room > 0 {
		h.buf = append(h.buf, b[:min(room, len(b))]...)
	}
	return len(b), nil
}

// indexCapture is the body of the import job.
func (a *App) indexCapture(ctx context.Context, p *project, captureID string, progress jobs.Progress) (*model.JobResult, error) {
	p.mu.Lock()
	i := findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == captureID })
	if i < 0 {
		p.mu.Unlock()
		return nil, errors.New("запись удалена")
	}
	rec := p.st.Captures[i]
	in := analyzeInput{
		path:      filepath.Join(p.dir(), filepath.FromSlash(rec.File)),
		projectID: p.id,
		capture:   rec.Capture,
		settings:  p.st.Project.Settings,
		limits:    a.cfg.Limits,
	}
	p.mu.Unlock()

	res, err := analyzeFile(ctx, in, progress)

	p.mu.Lock()
	defer p.mu.Unlock()
	i = findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == captureID })
	if i < 0 {
		return nil, errors.New("запись удалена")
	}
	c := &p.st.Captures[i]
	if ctx.Err() != nil && a.ctx.Err() != nil {
		// The program is shutting down: keep the file and the "indexing" record,
		// indexing resumes on the next start (resumeIndexing).
		return nil, ctx.Err()
	}
	if ctx.Err() != nil {
		// Cancelled by the user: nothing of the capture is kept, so it can be imported again.
		os.Remove(in.path)
		p.st.Captures = append(p.st.Captures[:i], p.st.Captures[i+1:]...)
		if serr := p.save(); serr != nil {
			a.log.Error("save after cancel", "err", serr)
		}
		return nil, ctx.Err()
	}
	if err != nil {
		c.Status = "error"
		c.Diagnostics = []model.Diagnostic{{Level: model.LevelError, Code: "analysis_failed", Message: "Не удалось разобрать запись: " + err.Error()}}
		if serr := p.save(); serr != nil {
			a.log.Error("save capture", "err", serr)
		}
		return nil, err
	}
	applyAnalysis(c, res)
	c.Status = "ready"
	p.setAnalysis(captureID, res)
	p.rev++
	if err := p.save(); err != nil {
		return nil, err
	}
	return &model.JobResult{Type: "capture", ID: captureID}, nil
}

// applyAnalysis copies capture-level results into the stored record.
func applyAnalysis(c *store.CaptureRecord, res *captureAnalysis) {
	c.LinkType = res.linkType
	c.SnapLen = res.snapLen
	c.Interfaces = res.interfaces
	c.PacketCount = res.packetCount
	c.SessionCount = len(res.sessions)
	c.TargetSessionCount = res.targets
	c.FirstPacketAt, c.LastPacketAt = nil, nil
	if !res.first.IsZero() {
		first, last := model.PacketTime(res.first), model.PacketTime(res.last)
		c.FirstPacketAt, c.LastPacketAt = &first, &last
	}
	c.Diagnostics = res.diagnostics
	if c.Diagnostics == nil {
		c.Diagnostics = []model.Diagnostic{}
	}
}

// captureDTO merges the stored record with the live analysis state.
func (p *project) captureDTO(rec store.CaptureRecord) model.Capture {
	c := rec.Capture
	c.Integrity = "unchecked"
	if c.Status != "ready" {
		return c
	}
	slot := p.slots[c.ID]
	if slot == nil {
		return c
	}
	res, err, ready := slot.peek()
	if !ready {
		return c
	}
	if err != nil {
		c.Status = "error"
		c.Diagnostics = append([]model.Diagnostic{{Level: model.LevelError, Code: "analysis_failed", Message: err.Error()}}, c.Diagnostics...)
		return c
	}
	if res.sha256 == c.SHA256 {
		c.Integrity = "ok"
	} else {
		c.Integrity = "mismatch"
	}
	c.SessionCount = len(res.sessions)
	c.TargetSessionCount = res.targets
	c.Diagnostics = res.diagnostics
	return c
}

func (a *App) ListCaptures(projectID string) ([]model.Capture, error) {
	p, err := a.project(projectID)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]model.Capture, 0, len(p.st.Captures))
	for _, rec := range p.st.Captures {
		out = append(out, p.captureDTO(rec))
	}
	return out, nil
}

func (a *App) GetCapture(captureID string) (model.Capture, error) {
	p, err := a.projectOf(captureID, "Запись")
	if err != nil {
		return model.Capture{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	i := findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == captureID })
	if i < 0 {
		return model.Capture{}, notFound("Запись")
	}
	return p.captureDTO(p.st.Captures[i]), nil
}
