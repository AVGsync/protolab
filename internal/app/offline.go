package app

import (
	"context"
	"path/filepath"

	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
)

// OfflineCapture is a capture analysed outside of any project (CLI apply/verify).
type OfflineCapture struct {
	File        string
	SHA256      string
	Format      string
	Packets     int
	Diagnostics []model.Diagnostic
	Sessions    []engine.SessionInput
}

// AnalyzeFile reads and reassembles a capture file with the given settings
// using exactly the same pipeline as project imports.
func AnalyzeFile(ctx context.Context, path string, settings model.ProjectSettings, limits Limits) (*OfflineCapture, error) {
	if limits == (Limits{}) {
		limits = DefaultLimits()
	}
	if settings.OverlapPolicy == "" {
		settings.OverlapPolicy = "first"
	}
	if settings.ChecksumPolicy == "" {
		settings.ChecksumPolicy = "warn"
	}
	base := filepath.Base(path)
	res, err := analyzeFile(ctx, analyzeInput{
		path:      path,
		projectID: "offline",
		capture:   model.Capture{ID: "cap-offline", FileName: base},
		settings:  settings,
		limits:    limits,
	}, nil)
	if err != nil {
		return nil, err
	}
	out := &OfflineCapture{File: base, SHA256: res.sha256, Format: res.format, Packets: res.packetCount, Diagnostics: res.diagnostics}
	for _, sd := range res.sessions {
		in := engineInput(sd, nil, nil)
		in.Session.Capture.SHA256 = res.sha256
		out.Sessions = append(out.Sessions, in)
	}
	return out, nil
}
