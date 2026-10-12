package tcpflow

import (
	"fmt"
	"net/netip"
	"slices"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
)

// options are the normalised Options.
type options struct {
	last        bool // PolicyLast
	noChecksums bool // ChecksumIgnore
	targetPorts map[uint16]bool
	maxBytes    int64
}

func normalize(opt Options) *options {
	o := &options{
		last:        opt.OverlapPolicy == PolicyLast,
		noChecksums: opt.ChecksumPolicy == ChecksumIgnore,
		targetPorts: make(map[uint16]bool, len(opt.TargetPorts)),
		maxBytes:    int64(opt.MaxStreamBytes),
	}
	if o.maxBytes <= 0 {
		o.maxBytes = DefaultMaxStreamBytes
	}
	for _, p := range opt.TargetPorts {
		if p > 0 && p <= 0xffff {
			o.targetPorts[uint16(p)] = true
		}
	}
	return o
}

type assembler struct {
	opt     *options
	current map[connKey]*conn // latest instance of every endpoint pair
	conns   []*conn           // all instances, in order of creation
	arena   arena
}

func newAssembler(opt Options) *assembler {
	return &assembler{opt: normalize(opt), current: make(map[connKey]*conn)}
}

// connKey is the unordered pair of endpoints.
type connKey struct{ lo, hi netip.AddrPort }

func keyOf(a, b netip.AddrPort) connKey {
	if b.Compare(a) < 0 {
		a, b = b, a
	}
	return connKey{lo: a, hi: b}
}

func (a *assembler) add(in Input) {
	seg := &in.Seg
	seg.PayloadLen = max(seg.PayloadLen, len(seg.Payload))
	if a.opt.noChecksums && seg.Checksum == netpkt.ChecksumBad {
		seg.Checksum = netpkt.ChecksumOK
	}
	// Keep a private copy: the caller may reuse its frame buffers, and copying only
	// the payload lets the frames themselves be freed.
	seg.Payload = a.arena.copy(seg.Payload)

	key := keyOf(seg.Src, seg.Dst)
	c := a.current[key]
	if c == nil || c.opensNewInstance(seg) {
		c = &conn{key: key}
		a.current[key] = c
		a.conns = append(a.conns, c)
	}
	c.add(in)
}

func (a *assembler) finish() []*Session {
	conns := a.conns
	a.conns, a.current, a.arena = nil, make(map[connKey]*conn), arena{}

	slices.SortStableFunc(conns, func(x, y *conn) int { return x.inputs[0].Number - y.inputs[0].Number })
	sessions := make([]*Session, len(conns))
	for i, c := range conns {
		sessions[i] = safeBuild(c, a.opt)
		sessions[i].Index = i
		conns[i] = nil // sessions keep no reference to the inputs or payload copies
	}
	return sessions
}

// build is buildSession; tests replace it to simulate a defect.
var build = buildSession

// safeBuild isolates the sessions from each other: a failure while
// reassembling one connection yields that session with status "error" and
// its packets, the other sessions are not affected.
func safeBuild(c *conn, opt *options) (s *Session) {
	defer func() {
		if v := recover(); v != nil {
			s = failedSession(c, v)
		}
	}()
	return build(c, opt)
}

func failedSession(c *conn, cause any) *Session {
	s := &Session{
		Client: c.key.lo, Server: c.key.hi, Roles: RolesUnknown, Status: statusError,
		Packets: make([]Packet, len(c.inputs)),
		Diagnostics: []model.Diagnostic{{
			Level:   model.LevelError,
			Code:    codeReassemblyFailed,
			Message: fmt.Sprintf("Внутренняя ошибка сборки соединения (%v): байты потоков не восстановлены, пакеты сохранены", cause),
		}},
	}
	if c.hasSyn {
		s.Client = c.synFrom
		if s.Client == c.key.hi {
			s.Server = c.key.lo
		}
	}
	s.Start, s.End = timeSpan(c.inputs)
	for i, in := range c.inputs {
		dir := model.S2C
		if in.Seg.Src == s.Client {
			dir = model.C2S
		}
		s.Packets[i] = Packet{
			Number: in.Number, Timestamp: in.Timestamp, Dir: dir, Flags: in.Seg.Flags,
			Seq: in.Seg.Seq, Ack: in.Seg.Ack, PayloadLen: in.Seg.PayloadLen, StreamOffset: -1, Annotations: []string{},
		}
	}
	for i, dir := range model.Directions {
		s.Streams[i] = &Stream{Dir: dir, Data: []byte{}, Segments: []Segment{}, Gaps: []Gap{}, Conflicts: []Conflict{}}
	}
	return s
}

// conn is one connection instance while segments are being collected. It keeps
// just enough handshake state to tell a new instance from a retransmission.
type conn struct {
	key    connKey
	inputs []Input

	hasSyn  bool // client SYN (SYN without ACK)
	synFrom netip.AddrPort
	synSeq  uint32
	synData uint32 // payload carried by the SYN (TCP Fast Open)

	hasSynAck  bool
	synAckFrom netip.AddrPort
	synAckSeq  uint32
	synAckAck  uint32

	finLo, finHi bool // FIN sent by key.lo / key.hi
	rst          bool

	// Sequence number of the first segment sent by key.lo / key.hi.
	firstLo, firstHi uint32
	seenLo, seenHi   bool
}

func (c *conn) closed() bool { return c.rst || (c.finLo && c.finHi) }

// opensNewInstance reports whether s belongs to a new connection that reuses
// the endpoints of c.
func (c *conn) opensNewInstance(s *netpkt.Segment) bool {
	switch {
	case s.Flags.Has(netpkt.SYN) && !s.Flags.Has(netpkt.ACK):
		return !c.acceptsSyn(s)
	case s.Flags.Has(netpkt.SYN | netpkt.ACK):
		// Normally the SYN opened the instance already; this catches port reuse
		// when the new connection's SYN was not captured, whether or not the
		// old instance was seen closing.
		return !c.acceptsSynAck(s)
	}
	return false
}

func (c *conn) acceptsSyn(s *netpkt.Segment) bool {
	if c.closed() {
		return false
	}
	if c.hasSyn {
		return s.Src == c.synFrom && s.Seq == c.synSeq // retransmitted SYN
	}
	// A SYN captured after the SYN-ACK that answers it.
	return c.hasSynAck && s.Src != c.synAckFrom && s.Seq+1 == c.synAckAck
}

func (c *conn) acceptsSynAck(s *netpkt.Segment) bool {
	switch {
	case c.hasSynAck:
		return s.Src == c.synAckFrom && s.Seq == c.synAckSeq // retransmitted SYN-ACK
	case c.hasSyn:
		// The SYN-ACK acknowledges the SYN and, with TCP Fast Open, maybe its data.
		return s.Src != c.synFrom && s.Ack-(c.synSeq+1) <= c.synData
	case c.closed():
		return false
	}
	// The instance was picked up without a handshake. A SYN-ACK belongs to it
	// only when it was retransmitted right after the handshake, before the
	// segments already seen: they then continue its sequence numbers.
	const slack = 1 << 20
	if first, ok := c.firstSeq(s.Dst); ok && first-s.Ack < slack {
		return true
	}
	first, ok := c.firstSeq(s.Src)
	return ok && first-(s.Seq+1) < slack
}

// firstSeq returns the sequence number of the first segment sent by ep.
func (c *conn) firstSeq(ep netip.AddrPort) (uint32, bool) {
	if ep == c.key.lo {
		return c.firstLo, c.seenLo
	}
	return c.firstHi, c.seenHi
}

func (c *conn) add(in Input) {
	c.inputs = append(c.inputs, in)
	s := &in.Seg
	switch {
	case s.Flags.Has(netpkt.SYN) && !s.Flags.Has(netpkt.ACK) && !c.hasSyn:
		c.hasSyn, c.synFrom, c.synSeq, c.synData = true, s.Src, s.Seq, uint32(s.PayloadLen)
	case s.Flags.Has(netpkt.SYN|netpkt.ACK) && !c.hasSynAck:
		c.hasSynAck, c.synAckFrom, c.synAckSeq, c.synAckAck = true, s.Src, s.Seq, s.Ack
	}
	if s.Src == c.key.lo {
		if !c.seenLo {
			c.firstLo, c.seenLo = s.Seq, true
		}
		c.finLo = c.finLo || s.Flags.Has(netpkt.FIN)
	} else {
		if !c.seenHi {
			c.firstHi, c.seenHi = s.Seq, true
		}
		c.finHi = c.finHi || s.Flags.Has(netpkt.FIN)
	}
	if s.Flags.Has(netpkt.RST) {
		c.rst = true
	}
}

// arena stores payload copies in large shared chunks instead of one allocation
// per packet.
type arena struct{ chunk []byte }

const arenaChunk = 1 << 20

func (a *arena) copy(p []byte) []byte {
	if len(p) == 0 {
		return nil
	}
	if len(p) > arenaChunk/4 {
		return slices.Clone(p)
	}
	if cap(a.chunk)-len(a.chunk) < len(p) {
		a.chunk = make([]byte, 0, arenaChunk)
	}
	n := len(a.chunk)
	a.chunk = append(a.chunk, p...)
	return a.chunk[n:len(a.chunk):len(a.chunk)]
}
