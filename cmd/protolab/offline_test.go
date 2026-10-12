package main

import (
	"testing"

	"github.com/AVGsync/protolab/internal/report"
)

func TestExampleSettings(t *testing.T) {
	ex := report.Examples{Settings: report.ExampleSettings{OverlapPolicy: "last", ChecksumPolicy: "ignore", TargetPorts: []int{5020}}}
	if s := exampleSettings(ex, report.CorpusExample{}); s.OverlapPolicy != "last" || s.ChecksumPolicy != "ignore" || s.TargetPorts[0] != 5020 {
		t.Errorf("file settings: %+v", s)
	}
	own := report.CorpusExample{Settings: &report.ExampleSettings{OverlapPolicy: "first", ChecksumPolicy: "warn"}}
	if s := exampleSettings(ex, own); s.OverlapPolicy != "first" || s.ChecksumPolicy != "warn" || s.TargetPorts[0] != 5020 {
		t.Errorf("corpus settings: %+v", s)
	}
	// Files written before the checksum policy was recorded.
	old := report.Examples{Settings: report.ExampleSettings{OverlapPolicy: "first"}}
	if s := exampleSettings(old, report.CorpusExample{}); s.ChecksumPolicy != "warn" || s.TargetPorts == nil {
		t.Errorf("old file: %+v", s)
	}
}
