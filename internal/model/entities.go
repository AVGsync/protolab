package model

import "github.com/AVGsync/protolab/internal/spec"

// ---------------------------------------------------------------- project

type ProjectSettings struct {
	OverlapPolicy  string `json:"overlapPolicy"`  // "first" | "last"
	ChecksumPolicy string `json:"checksumPolicy"` // "warn" | "ignore"
	TargetPorts    []int  `json:"targetPorts"`
}

type ProjectStats struct {
	Captures        int `json:"captures"`
	Sessions        int `json:"sessions"`
	Actions         int `json:"actions"`
	Observations    int `json:"observations"`
	Hypotheses      int `json:"hypotheses"`
	Interpretations int `json:"interpretations"`
	Runs            int `json:"runs"`
}

type Project struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	CreatedAt   string          `json:"createdAt"`
	UpdatedAt   string          `json:"updatedAt"`
	Settings    ProjectSettings `json:"settings"`
	Stats       ProjectStats    `json:"stats"`
}

type CreateProjectInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	TargetPorts []int  `json:"targetPorts"`
}

// ---------------------------------------------------------------- capture

type Capture struct {
	ID                 string       `json:"id"`
	ProjectID          string       `json:"projectId"`
	FileName           string       `json:"fileName"`
	Format             string       `json:"format"` // "pcap" | "pcapng"
	SizeBytes          int64        `json:"sizeBytes"`
	SHA256             string       `json:"sha256"`
	Integrity          string       `json:"integrity"` // "ok" | "mismatch" | "unchecked"
	LinkType           string       `json:"linkType"`
	SnapLen            int          `json:"snapLen"`
	Interfaces         int          `json:"interfaces"`
	PacketCount        int          `json:"packetCount"`
	SessionCount       int          `json:"sessionCount"`
	TargetSessionCount int          `json:"targetSessionCount"`
	FirstPacketAt      *string      `json:"firstPacketAt"`
	LastPacketAt       *string      `json:"lastPacketAt"`
	ImportedAt         string       `json:"importedAt"`
	Status             string       `json:"status"` // "indexing" | "ready" | "error"
	JobID              string       `json:"jobId,omitempty"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
}

type UploadCaptureResult struct {
	Capture Capture `json:"capture"`
	JobID   string  `json:"jobId"`
}

// ---------------------------------------------------------------- session & stream

type SessionFlags struct {
	Handshake        bool `json:"handshake"`
	Closed           bool `json:"closed"`
	Gaps             int  `json:"gaps"`
	Retransmissions  int  `json:"retransmissions"`
	OutOfOrder       int  `json:"outOfOrder"`
	OverlapConflicts int  `json:"overlapConflicts"`
	ChecksumWarnings int  `json:"checksumWarnings"`
}

type CaptureRef struct {
	FileName string `json:"fileName"`
	SHA256   string `json:"sha256"`
}

type ByteCounts struct {
	C2S int `json:"c2s"`
	S2C int `json:"s2c"`
}

// TcpSession is one TCP connection instance. Reused address/port tuples give distinct sessions.
type TcpSession struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"projectId"`
	CaptureID   string       `json:"captureId"`
	Capture     CaptureRef   `json:"capture"`
	Index       int          `json:"index"`
	Client      Endpoint     `json:"client"`
	Server      Endpoint     `json:"server"`
	Roles       string       `json:"roles"` // "handshake" | "inferred" | "unknown"
	StartedAt   string       `json:"startedAt"`
	EndedAt     string       `json:"endedAt"`
	PacketCount int          `json:"packetCount"`
	Bytes       ByteCounts   `json:"bytes"`
	Status      string       `json:"status"` // "complete" | "partial" | "error"
	Flags       SessionFlags `json:"flags"`
	IsTarget    bool         `json:"isTarget"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	ActionIDs   []string     `json:"actionIds"`
}

// Segment/packet annotations.
const (
	FlagRetransmission  = "retransmission"
	FlagOutOfOrder      = "out_of_order"
	FlagOverlapConflict = "overlap_conflict"
	FlagChecksumWarning = "checksum_warning"
	FlagTruncated       = "truncated"
	FlagKeepAlive       = "keep_alive"
)

// Gap reasons.
const (
	GapLost        = "lost"
	GapTruncated   = "truncated"
	GapNotCaptured = "not_captured"
)

type StreamSegment struct {
	Offset       int      `json:"offset"`
	Length       int      `json:"length"`
	PacketNumber int      `json:"packetNumber"`
	Timestamp    string   `json:"timestamp"`
	Flags        []string `json:"flags"`
}

type StreamGap struct {
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	Reason string `json:"reason"`
}

type OverlapConflict struct {
	Offset        int   `json:"offset"`
	Length        int   `json:"length"`
	PacketNumbers []int `json:"packetNumbers"`
	ChosenPacket  int   `json:"chosenPacket"`
}

type DirectedStream struct {
	SessionID  string            `json:"sessionId"`
	Direction  Direction         `json:"direction"`
	Length     int               `json:"length"`
	StartKnown bool              `json:"startKnown"`
	Segments   []StreamSegment   `json:"segments"`
	Gaps       []StreamGap       `json:"gaps"`
	Conflicts  []OverlapConflict `json:"conflicts"`
}

type Packet struct {
	Number        int       `json:"number"`
	Timestamp     string    `json:"timestamp"`
	Direction     Direction `json:"direction"`
	TCPFlags      []string  `json:"tcpFlags"`
	Seq           uint32    `json:"seq"`
	Ack           uint32    `json:"ack"`
	PayloadLength int       `json:"payloadLength"`
	StreamOffset  *int      `json:"streamOffset"`
	Flags         []string  `json:"flags"`
}

// ---------------------------------------------------------------- messages

// Message statuses.
const (
	MsgOK          = "ok"
	MsgViolation   = "violation"
	MsgPartial     = "partial"
	MsgUnknownType = "unknown_type"
	MsgAmbiguous   = "ambiguous"
)

// Field statuses.
const (
	FieldOK        = "ok"
	FieldViolation = "violation"
	FieldUnknown   = "unknown"
)

type ParsedField struct {
	Path          string        `json:"path"`
	Name          string        `json:"name"`
	Offset        int           `json:"offset"`
	Length        int           `json:"length"`
	Type          string        `json:"type"`
	Value         any           `json:"value"` // number | string | null
	Display       string        `json:"display"`
	Status        string        `json:"status"`
	Issue         string        `json:"issue,omitempty"`
	Meaning       string        `json:"meaning,omitempty"`
	MeaningStatus string        `json:"meaningStatus,omitempty"`
	HypothesisID  string        `json:"hypothesisId,omitempty"`
	Children      []ParsedField `json:"children,omitempty"`
}

type Message struct {
	SessionID        string        `json:"sessionId"`
	Direction        Direction     `json:"direction"`
	Index            int           `json:"index"`
	Offset           int           `json:"offset"`
	Length           int           `json:"length"`
	Timestamp        string        `json:"timestamp"`
	PacketNumbers    []int         `json:"packetNumbers"`
	RuleID           *string       `json:"ruleId"`
	RuleTitle        string        `json:"ruleTitle,omitempty"`
	CandidateRuleIDs []string      `json:"candidateRuleIds,omitempty"`
	Status           string        `json:"status"`
	Issues           []string      `json:"issues"`
	Fields           []ParsedField `json:"fields"`
	Unknown          []ByteRange   `json:"unknown"`
	Data             []byte        `json:"data"` // base64 in JSON
}

// ---------------------------------------------------------------- action log

type ActionLink struct {
	SessionID string `json:"sessionId"`
	Method    string `json:"method"` // "auto_time" | "manual"
	DeltaMs   *int64 `json:"deltaMs,omitempty"`
}

// ActionEntry is a known action performed in the vendor client (an experiment condition).
// Param values are JSON numbers or strings.
type ActionEntry struct {
	ID        string         `json:"id"`
	ProjectID string         `json:"projectId"`
	Timestamp string         `json:"timestamp"`
	Kind      string         `json:"kind"`
	Label     string         `json:"label"`
	Params    map[string]any `json:"params"`
	Observed  map[string]any `json:"observed,omitempty"`
	Result    string         `json:"result,omitempty"`
	Links     []ActionLink   `json:"links"`
	Source    string         `json:"source"` // "import" | "manual"
}

type CreateActionInput struct {
	Timestamp  string         `json:"timestamp"`
	Kind       string         `json:"kind"`
	Label      string         `json:"label"`
	Params     map[string]any `json:"params"`
	Observed   map[string]any `json:"observed,omitempty"`
	Result     string         `json:"result,omitempty"`
	SessionIDs []string       `json:"sessionIds,omitempty"`
}

type ImportActionsResult struct {
	Imported int      `json:"imported"`
	Linked   int      `json:"linked"`
	Warnings []string `json:"warnings"`
}

// ---------------------------------------------------------------- interpretations

type Interpretation struct {
	ID              string `json:"id"`
	ProjectID       string `json:"projectId"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	LatestVersion   int    `json:"latestVersion"`
	LatestVersionID string `json:"latestVersionId"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

// InterpretationVersion is immutable; every change creates a new version with its own hash.
type InterpretationVersion struct {
	ID               string    `json:"id"`
	InterpretationID string    `json:"interpretationId"`
	Version          int       `json:"version"`
	Hash             string    `json:"hash"`
	Comment          string    `json:"comment"`
	CreatedAt        string    `json:"createdAt"`
	ParentVersionID  string    `json:"parentVersionId,omitempty"`
	Spec             spec.Spec `json:"spec"`
}

// ---------------------------------------------------------------- hypotheses & observations

type HypothesisEvent struct {
	At     string `json:"at"`
	Status string `json:"status"`
	Note   string `json:"note"`
}

type Counterexample struct {
	Source SourceRef `json:"source"`
	Note   string    `json:"note"`
	RunID  string    `json:"runId,omitempty"`
}

type Hypothesis struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"projectId"`
	Title           string            `json:"title"`
	Statement       string            `json:"statement"`
	Status          string            `json:"status"` // proposed | testing | supported | refuted
	ObservationIDs  []string          `json:"observationIds"`
	Counterexamples []Counterexample  `json:"counterexamples"`
	Alternatives    []string          `json:"alternatives"`
	RuleRefs        []string          `json:"ruleRefs"`
	CreatedAt       string            `json:"createdAt"`
	UpdatedAt       string            `json:"updatedAt"`
	History         []HypothesisEvent `json:"history"`
}

type CreateHypothesisInput struct {
	Title          string   `json:"title"`
	Statement      string   `json:"statement"`
	ObservationIDs []string `json:"observationIds"`
	Alternatives   []string `json:"alternatives"`
	RuleRefs       []string `json:"ruleRefs"`
}

type UpdateHypothesisInput struct {
	Title           *string          `json:"title"`
	Statement       *string          `json:"statement"`
	Status          *string          `json:"status"`
	Note            string           `json:"note"`
	Alternatives    []string         `json:"alternatives"`
	ObservationIDs  []string         `json:"observationIds"`
	Counterexamples []Counterexample `json:"counterexamples"`
}

// Observation is a note bound to exact source bytes; SnapshotHex keeps the bytes seen at creation.
type Observation struct {
	ID            string    `json:"id"`
	ProjectID     string    `json:"projectId"`
	Note          string    `json:"note"`
	Source        SourceRef `json:"source"`
	SnapshotHex   string    `json:"snapshotHex"`
	Integrity     string    `json:"integrity"` // "ok" | "mismatch"
	HypothesisIDs []string  `json:"hypothesisIds"`
	Tags          []string  `json:"tags"`
	CreatedAt     string    `json:"createdAt"`
}

type CreateObservationInput struct {
	Note          string    `json:"note"`
	Source        SourceRef `json:"source"`
	HypothesisIDs []string  `json:"hypothesisIds"`
	Tags          []string  `json:"tags"`
}

// ---------------------------------------------------------------- corpora & verification

type CorpusFilter struct {
	CaptureIDs   []string `json:"captureIds,omitempty"`
	OnlyTarget   bool     `json:"onlyTarget,omitempty"`
	OnlyComplete bool     `json:"onlyComplete,omitempty"`
	ActionKinds  []string `json:"actionKinds,omitempty"`
}

type Corpus struct {
	ID          string       `json:"id"`
	ProjectID   string       `json:"projectId"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	SessionIDs  []string     `json:"sessionIds"`
	Filter      CorpusFilter `json:"filter"`
	Hash        string       `json:"hash"`
	CreatedAt   string       `json:"createdAt"`
}

type CreateCorpusInput struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	SessionIDs  []string     `json:"sessionIds"`
	Filter      CorpusFilter `json:"filter"`
}

type RuleStat struct {
	RuleID     string `json:"ruleId"`
	Matched    int    `json:"matched"`
	Violations int    `json:"violations"`
}

// RunSummary covers the whole corpus, failures and unknown data included.
type RunSummary struct {
	Sessions           int        `json:"sessions"`
	SessionsWithErrors int        `json:"sessionsWithErrors"`
	Messages           int        `json:"messages"`
	OK                 int        `json:"ok"`
	Violations         int        `json:"violations"`
	Partial            int        `json:"partial"`
	Ambiguous          int        `json:"ambiguous"`
	UnknownType        int        `json:"unknownType"`
	UnknownBytes       int        `json:"unknownBytes"`
	TotalBytes         int        `json:"totalBytes"`
	InapplicableRules  []string   `json:"inapplicableRules"`
	ByRule             []RuleStat `json:"byRule"`
}

type RunSettings struct {
	OverlapPolicy  string `json:"overlapPolicy"`
	ChecksumPolicy string `json:"checksumPolicy"`
}

type VerificationRun struct {
	ID                 string      `json:"id"`
	ProjectID          string      `json:"projectId"`
	InterpretationID   string      `json:"interpretationId"`
	InterpretationName string      `json:"interpretationName"`
	VersionID          string      `json:"versionId"`
	Version            int         `json:"version"`
	VersionHash        string      `json:"versionHash"`
	CorpusID           string      `json:"corpusId"`
	CorpusName         string      `json:"corpusName"`
	CorpusHash         string      `json:"corpusHash"`
	Status             string      `json:"status"` // queued | running | succeeded | failed | cancelled
	JobID              string      `json:"jobId,omitempty"`
	StartedAt          string      `json:"startedAt"`
	FinishedAt         string      `json:"finishedAt,omitempty"`
	DurationMs         *int64      `json:"durationMs,omitempty"`
	Summary            *RunSummary `json:"summary,omitempty"`
	Stale              bool        `json:"stale"`
	StaleReasons       []string    `json:"staleReasons"`
	PreviousRunID      string      `json:"previousRunId,omitempty"`
	Settings           RunSettings `json:"settings"`
	EngineVersion      string      `json:"engineVersion"`
	Error              string      `json:"error,omitempty"`
}

// Finding kinds.
const (
	FindingViolation        = "violation"
	FindingPartial          = "partial"
	FindingAmbiguous        = "ambiguous"
	FindingUnknownType      = "unknown_type"
	FindingUnknownRegion    = "unknown_region"
	FindingInapplicableRule = "inapplicable_rule"
	FindingCaptureDefect    = "capture_defect"
)

type Finding struct {
	ID          string      `json:"id"`
	RunID       string      `json:"runId"`
	Kind        string      `json:"kind"`
	Severity    string      `json:"severity"` // error | warning | info
	RuleID      string      `json:"ruleId,omitempty"`
	FieldPath   string      `json:"fieldPath,omitempty"`
	Message     string      `json:"message"`
	Occurrences int         `json:"occurrences"`
	Examples    []SourceRef `json:"examples"`
}

type StartRunInput struct {
	VersionID string `json:"versionId"`
	CorpusID  string `json:"corpusId"`
}

type StartRunResult struct {
	Run   VerificationRun `json:"run"`
	JobID string          `json:"jobId"`
}

// ---------------------------------------------------------------- jobs, reports, system

type JobResult struct {
	Type string `json:"type"` // capture | run | report | project
	ID   string `json:"id"`
}

type Job struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"projectId"`
	Kind        string     `json:"kind"`
	Title       string     `json:"title"`
	Status      string     `json:"status"` // queued | running | succeeded | failed | cancelled
	Progress    float64    `json:"progress"`
	Stage       string     `json:"stage,omitempty"`
	StartedAt   string     `json:"startedAt"`
	FinishedAt  string     `json:"finishedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	Result      *JobResult `json:"result,omitempty"`
	Cancellable bool       `json:"cancellable"`
}

type ReportArtifact struct {
	Kind      string `json:"kind"` // report_md | interpretation | results | examples
	FileName  string `json:"fileName"`
	URL       string `json:"url"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

type Report struct {
	ID                 string           `json:"id"`
	ProjectID          string           `json:"projectId"`
	Title              string           `json:"title"`
	CreatedAt          string           `json:"createdAt"`
	VersionID          string           `json:"versionId"`
	Version            int              `json:"version"`
	InterpretationName string           `json:"interpretationName"`
	RunIDs             []string         `json:"runIds"`
	Status             string           `json:"status"` // pending | ready | failed
	Artifacts          []ReportArtifact `json:"artifacts"`
	ReproduceCommand   string           `json:"reproduceCommand"`
}

type CreateReportInput struct {
	Title     string   `json:"title"`
	VersionID string   `json:"versionId"`
	RunIDs    []string `json:"runIds"`
}

type SystemLimits struct {
	MaxCaptureBytes     int64 `json:"maxCaptureBytes"`
	MaxPackets          int   `json:"maxPackets"`
	MaxSessions         int   `json:"maxSessions"`
	MaxMessages         int   `json:"maxMessages"`
	MaxMessageBytes     int   `json:"maxMessageBytes"`
	MaxStreamBytes      int   `json:"maxStreamBytes"`      // one reassembled direction
	MaxTotalStreamBytes int64 `json:"maxTotalStreamBytes"` // all directions of a capture together
}

type SystemRuntime struct {
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	CPUs          int    `json:"cpus"`
	GoVersion     string `json:"goVersion"`
	MemAllocBytes uint64 `json:"memAllocBytes"`
	MemSysBytes   uint64 `json:"memSysBytes"`
	UptimeSec     int64  `json:"uptimeSec"`
}

type SystemStorage struct {
	DataDir   string `json:"dataDir"`
	UsedBytes int64  `json:"usedBytes"`
}

type SystemInfo struct {
	Version       string        `json:"version"`
	Commit        string        `json:"commit"`
	BuildDate     string        `json:"buildDate"`
	EngineVersion string        `json:"engineVersion"`
	Limits        SystemLimits  `json:"limits"`
	Runtime       SystemRuntime `json:"runtime"`
	Storage       SystemStorage `json:"storage"`
}
