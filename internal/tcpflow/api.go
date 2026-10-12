// Package tcpflow splits decoded TCP segments into connection instances
// (sessions) and reassembles each direction into a byte stream ordered by
// sequence numbers, keeping track of retransmissions, reordering, conflicting
// overlaps, gaps and truncation.
package tcpflow

import (
	"net/netip"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
)

// Overlap policies: which copy wins when a retransmission carries different bytes.
const (
	PolicyFirst = "first"
	PolicyLast  = "last"
)

// Checksum policies (model.ProjectSettings.ChecksumPolicy).
const (
	// ChecksumWarn: bad TCP checksums are annotated and diagnosed. Failures
	// of a sender whose segments mostly fail the check are taken for checksum
	// offloading on the capturing host and change nothing else. A failing
	// segment of a sender whose segments mostly pass is taken for damaged:
	// its sequence and acknowledgment numbers do not extend the stream, and a
	// copy with a valid checksum wins an overlap conflict against it
	// regardless of the overlap policy.
	ChecksumWarn = "warn"
	// ChecksumIgnore: checksums are not looked at.
	ChecksumIgnore = "ignore"
)

// Role certainty values (model.TcpSession.Roles).
const (
	RolesHandshake = "handshake" // client sent the SYN
	RolesInferred  = "inferred"  // derived from the SYN-ACK or a target port
	RolesUnknown   = "unknown"   // first sender assumed to be the client
)

// Protective limits. Gap bytes are held as zeros, so a stream's size follows
// its sequence numbers, not the captured bytes.
const (
	// DefaultMaxStreamBytes bounds a reassembled direction.
	DefaultMaxStreamBytes = 64 << 20
	// DefaultMaxTotalBytes bounds all reassembled directions of a capture together.
	DefaultMaxTotalBytes = 1 << 30
)

type Options struct {
	OverlapPolicy  string // PolicyFirst (default) or PolicyLast
	ChecksumPolicy string // ChecksumWarn (default) or ChecksumIgnore
	TargetPorts    []int  // server ports of the device; used to infer roles without a handshake
	MaxStreamBytes int    // 0 means DefaultMaxStreamBytes
	// MaxTotalBytes: 0 means DefaultMaxTotalBytes. Sessions are reassembled in
	// the order of their first packet; a direction that does not fit into what
	// is left is cut there, with a stream_too_large error like the per-stream limit.
	MaxTotalBytes int64
}

// Input is one decoded segment with its capture metadata.
type Input struct {
	Number    int // 1-based packet number in the capture
	Timestamp time.Time
	Seg       netpkt.Segment
}

// Packet is a packet of a session as shown to the user.
type Packet struct {
	Number    int
	Timestamp time.Time
	Dir       model.Direction
	Flags     netpkt.TCPFlags
	// Seq and Ack are relative: Seq to the sender's ISN, Ack to the peer's ISN
	// (Wireshark style). When the ISN was not captured the first byte seen has Seq 1.
	Seq, Ack   uint32
	PayloadLen int
	// StreamOffset is the offset of the payload in the directed stream, or -1 when
	// the packet carries no payload or contributed no bytes (pure duplicate).
	StreamOffset int
	Annotations  []string // model.Flag* values
}

// Segment is a packet that contributed bytes to a stream.
type Segment struct {
	Offset      int
	Length      int // logical payload length (includes a truncated tail)
	Packet      int // packet number
	Timestamp   time.Time
	Annotations []string
}

// Gap is a range of the stream whose bytes are unknown.
type Gap struct {
	Offset, Length int
	Reason         string // model.GapLost | model.GapTruncated
}

// Conflict is a range received twice with different bytes.
type Conflict struct {
	Offset, Length int
	Packets        []int // packet numbers of the competing copies, in arrival order
	Chosen         int   // packet number whose bytes are in Data
}

// Stream is one reassembled direction.
type Stream struct {
	Dir        model.Direction
	Data       []byte // gap bytes are zero; consult Gaps
	StartKnown bool   // false when the beginning of the stream was not captured
	Segments   []Segment
	Gaps       []Gap
	Conflicts  []Conflict
}

// Session is one TCP connection instance.
type Session struct {
	Index          int // 0-based, in order of the first packet
	Client, Server netip.AddrPort
	Roles          string
	Start, End     time.Time
	Packets        []Packet   // capture order
	Streams        [2]*Stream // [0] client->server, [1] server->client
	Handshake      bool       // SYN from the client was captured
	Closed         bool       // FIN seen in both directions, or RST
	Status         string     // "complete" | "partial" | "error"
	Diagnostics    []model.Diagnostic
}

// Stream returns the stream of a direction.
func (s *Session) Stream(d model.Direction) *Stream {
	if d == model.C2S {
		return s.Streams[0]
	}
	return s.Streams[1]
}

// Assembler collects segments; call Finish once all of them were added.
type Assembler struct{ impl *assembler }

func NewAssembler(opt Options) *Assembler { return &Assembler{impl: newAssembler(opt)} }

// Add takes segments in capture order.
func (a *Assembler) Add(in Input) { a.impl.add(in) }

// Finish reassembles all connections and returns sessions ordered by their first packet.
func (a *Assembler) Finish() []*Session { return a.impl.finish() }
