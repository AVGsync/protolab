// Package demo generates a complete demo dataset for the synthetic "ThermoLink"
// protocol: real pcapng files (with capture defects), an action log, three
// interpretation versions, hypotheses and observations.
package demo

import (
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// Capture is a generated capture file.
type Capture struct {
	FileName string
	Data     []byte // pcap or pcapng bytes
}

// FieldRef locates a parsed field: the first message matching Rule in the
// session of CaptureFile whose client port is ClientPort.
type FieldRef struct {
	CaptureFile string
	ClientPort  int
	Rule        string
	Field       string
}

type VersionSeed struct {
	Spec      spec.Spec
	Comment   string
	CreatedAt string // model.EntityTime
}

type HypothesisSeed struct {
	SeedID       string // e.g. "h-03"; mapped through Options.HypothesisID
	Title        string
	Statement    string
	Status       string
	Alternatives []string
	RuleRefs     []string
	History      []model.HypothesisEvent
	CreatedAt    string
	UpdatedAt    string
}

type ObservationSeed struct {
	Ref           FieldRef
	Note          string
	HypothesisIDs []string // seed ids
	CreatedAt     string
}

type CounterexampleSeed struct {
	HypothesisID string // seed id
	Ref          FieldRef
	Note         string
	Run          int // index into Dataset.Runs
}

type CorpusSeed struct {
	Name         string
	Description  string
	CaptureFiles []string
	OnlyTarget   bool
}

// RunSeed: verify Versions[Version] on Corpora[Corpus].
type RunSeed struct {
	Version int
	Corpus  int
}

type Dataset struct {
	ProjectName        string
	ProjectDescription string
	TargetPorts        []int

	Captures []Capture
	Actions  []model.CreateActionInput

	InterpretationName        string
	InterpretationDescription string
	Versions                  []VersionSeed // v1, v2, v3

	Hypotheses      []HypothesisSeed
	Observations    []ObservationSeed
	Counterexamples []CounterexampleSeed
	Corpora         []CorpusSeed
	Runs            []RunSeed
}

// Options customise generated identifiers.
type Options struct {
	// HypothesisID maps a seed id ("h-03") to the id used in the project; specs
	// reference hypotheses through it. Identity mapping when nil.
	HypothesisID func(seed string) string
}

// Build generates the dataset deterministically.
func Build(opt Options) Dataset { panic("not implemented") }
