package report

import (
	"encoding/json"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// The examples file records the reassembly settings the expected summaries were
// computed with (those of the runs), not the project settings of the moment.
func TestExamplesRecordRunSettings(t *testing.T) {
	sp, res := spec.Parse([]byte(`{"format":"protolab/v1","protocol":"P","framing":{"kind":"fixed","size":2},"messages":[{"id":"m","match":[],"fields":[]}]}`))
	if !res.Valid {
		t.Fatal(res.Issues)
	}
	run := func(id, overlap, checksum string) RunData {
		return RunData{
			Run:    model.VerificationRun{ID: id, Summary: &model.RunSummary{}, Settings: model.RunSettings{OverlapPolicy: overlap, ChecksumPolicy: checksum}, StaleReasons: []string{}},
			Corpus: model.Corpus{Name: id},
		}
	}
	in := Input{
		Project:        model.Project{Settings: model.ProjectSettings{OverlapPolicy: "last", ChecksumPolicy: "warn", TargetPorts: []int{5020}}},
		Interpretation: model.Interpretation{Name: "P"},
		Version:        model.InterpretationVersion{Version: 1, Spec: *sp, Hash: spec.Hash(sp)},
		Runs:           []RunData{run("r1", "first", "ignore"), run("r2", "last", "warn")},
	}
	files, err := Render(in)
	if err != nil {
		t.Fatal(err)
	}
	var ex Examples
	for _, f := range files {
		if f.Kind == "examples" {
			if err := json.Unmarshal(f.Data, &ex); err != nil {
				t.Fatal(err)
			}
		}
	}
	if ex.Settings.OverlapPolicy != "first" || ex.Settings.ChecksumPolicy != "ignore" || len(ex.Settings.TargetPorts) != 1 {
		t.Errorf("file settings = %+v, want those of the first run", ex.Settings)
	}
	if len(ex.Corpora) != 2 || ex.Corpora[0].Settings != nil {
		t.Fatalf("corpora = %+v", ex.Corpora)
	}
	if s := ex.Corpora[1].Settings; s == nil || s.OverlapPolicy != "last" || s.ChecksumPolicy != "warn" {
		t.Errorf("second run settings = %+v", s)
	}
}
