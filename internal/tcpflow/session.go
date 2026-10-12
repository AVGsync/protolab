package tcpflow

import (
	"net/netip"
	"slices"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
)

// roleSource tells how the client and the server were told apart.
type roleSource int

const (
	byHandshake   roleSource = iota // the client's SYN was captured
	bySynAck                        // the server's SYN-ACK was captured
	byPort                          // exactly one side uses a target port
	byFirstSender                   // nothing to go on: the first sender is the client
)

func (r roleSource) roles() string {
	switch r {
	case byHandshake:
		return RolesHandshake
	case bySynAck, byPort:
		return RolesInferred
	}
	return RolesUnknown
}

// builder turns a collected connection instance into a Session.
type builder struct {
	c      *conn
	opt    *options
	client netip.AddrPort
	source roleSource
	halves [2]*half // [0] client->server, [1] server->client
	offset []int64  // stream offset of each input's first payload byte
}

func buildSession(c *conn, opt *options) *Session {
	b := &builder{c: c, opt: opt, halves: [2]*half{newHalf(model.C2S), newHalf(model.S2C)}}
	b.assignRoles()
	b.findBases()
	b.measure()
	packets, badChecksums := b.place()
	return b.session(packets, badChecksums)
}

func (b *builder) server() netip.AddrPort {
	if b.client == b.c.key.lo {
		return b.c.key.hi
	}
	return b.c.key.lo
}

func (b *builder) peerOf(ep netip.AddrPort) netip.AddrPort {
	if ep == b.c.key.lo {
		return b.c.key.hi
	}
	return b.c.key.lo
}

func (b *builder) assignRoles() {
	c := b.c
	switch {
	case c.hasSyn:
		b.client, b.source = c.synFrom, byHandshake
	case c.hasSynAck:
		b.client, b.source = b.peerOf(c.synAckFrom), bySynAck
	default:
		first := c.inputs[0].Seg
		srcTarget, dstTarget := b.opt.targetPorts[first.Src.Port()], b.opt.targetPorts[first.Dst.Port()]
		switch {
		case srcTarget && !dstTarget:
			b.client, b.source = first.Dst, byPort
		case dstTarget && !srcTarget:
			b.client, b.source = first.Src, byPort
		default:
			b.client, b.source = first.Src, byFirstSender
		}
	}
}

// dir returns the index into halves of the direction s was sent in.
func (b *builder) dir(s *netpkt.Segment) int {
	if s.Src == b.client {
		return 0
	}
	return 1
}

// dataSeq is the sequence number of the first payload byte: a SYN occupies one.
func dataSeq(s *netpkt.Segment) uint32 {
	if s.Flags.Has(netpkt.SYN) {
		return s.Seq + 1
	}
	return s.Seq
}

// findBases sets the sequence number of offset 0 for both directions.
func (b *builder) findBases() {
	c, c2s, s2c := b.c, b.halves[0], b.halves[1]
	server := b.server()
	switch {
	case c.hasSyn && c.synFrom == b.client:
		c2s.setBase(c.synSeq+1, true)
	case c.hasSynAck && c.synAckFrom == server:
		// The SYN-ACK acknowledges the client's ISN+1, the client's first data byte.
		c2s.setBase(c.synAckAck, true)
	}
	if c.hasSynAck && c.synAckFrom == server {
		s2c.setBase(c.synAckSeq+1, true)
	}
	if !c2s.known || !s2c.known {
		b.estimateBases()
	}
}

// estimate collects what a direction without a captured ISN reveals about where
// its stream starts. Positions are relative to the first number seen.
type estimate struct {
	u        seqUnwrapper
	started  bool
	minData  int64 // lowest payload or FIN position
	hasData  bool
	maxPure  int64 // highest position of segments without payload (their SND.NXT)
	hasPure  bool
	minAck   int64 // lowest position acknowledged by the peer
	ackHigh  int64 // highest position acknowledged by the peer so far
	hasAck   bool
	sentHigh int64 // highest sequence position the sender has used so far
	hasSent  bool
}

func (e *estimate) pos(seq uint32, advance bool) int64 {
	if !e.started {
		e.u, e.started = seqUnwrapper{origin: seq}, true
	}
	if advance {
		return e.u.advance(seq)
	}
	return e.u.at(seq)
}

func (e *estimate) base() uint32 {
	switch {
	case e.hasData:
		return e.u.origin + uint32(e.minData)
	case e.hasPure:
		return e.u.origin + uint32(e.maxPure)
	case e.hasAck:
		return e.u.origin + uint32(e.minAck)
	}
	return 0
}

// estimateBases picks offset 0 of a direction without a captured ISN: the
// lowest sequence number carrying payload or a FIN (the first byte seen).
// Keep-alive probes are left out: their single byte is garbage below SND.NXT,
// recognisable when an earlier segment or acknowledgment already went past it.
func (b *builder) estimateBases() {
	var est [2]estimate
	for i := range b.c.inputs {
		s := &b.c.inputs[i].Seg
		d := b.dir(s)
		if s.Flags.Has(netpkt.ACK) && !b.halves[1-d].known {
			p := &est[1-d]
			a := p.pos(s.Ack, false)
			if !p.hasAck {
				p.minAck, p.ackHigh, p.hasAck = a, a, true
			}
			p.minAck, p.ackHigh = min(p.minAck, a), max(p.ackHigh, a)
		}
		if b.halves[d].known || s.Flags.Has(netpkt.RST) {
			continue
		}
		e := &est[d]
		carries := s.PayloadLen > 0 || s.Flags.Has(netpkt.FIN)
		start := e.pos(dataSeq(s), carries)
		end := start + int64(s.PayloadLen)
		switch {
		case carries && !e.isProbe(s, end):
			if !e.hasData || start < e.minData {
				e.minData = start
			}
			e.hasData = true
		case !carries:
			if !e.hasPure || start > e.maxPure {
				e.maxPure = start
			}
			e.hasPure = true
		}
		if !e.hasSent || end > e.sentHigh {
			e.sentHigh = end
		}
		e.hasSent = true
	}
	for d, h := range b.halves {
		if !h.known {
			h.setBase(est[d].base(), false)
		}
	}
}

// isProbe reports a keep-alive: at most one byte, entirely below what the
// sender already used or the peer already acknowledged.
func (e *estimate) isProbe(s *netpkt.Segment, end int64) bool {
	if s.PayloadLen > 1 || s.Flags&(netpkt.SYN|netpkt.FIN) != 0 {
		return false
	}
	return (e.hasSent && end <= e.sentHigh) || (e.hasAck && end <= e.ackHigh)
}

// measure computes every input's stream offset and the extent of both streams.
func (b *builder) measure() {
	b.offset = make([]int64, len(b.c.inputs))
	for i := range b.c.inputs {
		s := &b.c.inputs[i].Seg
		d := b.dir(s)
		h := b.halves[d]
		carries := s.PayloadLen > 0 || s.Flags.Has(netpkt.FIN)
		if carries && !s.Flags.Has(netpkt.RST) {
			off := h.seq.advance(dataSeq(s))
			b.offset[i] = off
			end := off + int64(s.PayloadLen)
			h.dataEnd = max(h.dataEnd, end)
			if s.Flags.Has(netpkt.FIN) {
				h.finOff = max(h.finOff, end)
				h.finSeen = true
			}
		} else {
			b.offset[i] = h.seq.at(dataSeq(s))
		}
		if s.Flags.Has(netpkt.ACK) {
			b.halves[1-d].noteAck(s.Ack)
		}
	}
	for _, h := range b.halves {
		h.finishExtent(b.opt.maxBytes)
	}
}

// place reassembles both streams in capture order and annotates the packets.
func (b *builder) place() ([]Packet, int) {
	packets := make([]Packet, len(b.c.inputs))
	badChecksums := 0
	for i := range b.c.inputs {
		in := &b.c.inputs[i]
		s := &in.Seg
		d := b.dir(s)
		h, peer := b.halves[d], b.halves[1-d]

		p := Packet{
			Number:       in.Number,
			Timestamp:    in.Timestamp,
			Dir:          h.dir,
			Flags:        s.Flags,
			Seq:          s.Seq - h.ref(),
			PayloadLen:   s.PayloadLen,
			StreamOffset: -1,
		}
		if s.Flags.Has(netpkt.ACK) {
			p.Ack = s.Ack - peer.ref()
		}
		contributed, notes := h.accept(in, b.offset[i], b.opt.last)
		if s.Truncated() {
			notes = append(notes, model.FlagTruncated)
		}
		if s.Checksum == netpkt.ChecksumBad {
			notes = append(notes, model.FlagChecksumWarning)
			badChecksums++
		}
		p.Annotations = notes
		if contributed {
			off := max(b.offset[i], 0)
			end := min(b.offset[i]+int64(s.PayloadLen), h.length)
			p.StreamOffset = int(off)
			h.segments = append(h.segments, Segment{
				Offset:      int(off),
				Length:      int(end - off),
				Packet:      in.Number,
				Timestamp:   in.Timestamp,
				Annotations: slices.Clone(notes),
			})
		}
		packets[i] = p
	}
	return packets, badChecksums
}

// accept merges one segment into the stream. It reports whether the segment
// contributed to the stream (new bytes, a new truncated range, or a won
// conflict) and returns its annotations.
func (h *half) accept(in *Input, off int64, last bool) (bool, []string) {
	s := &in.Seg
	notes := []string{}
	if s.Flags.Has(netpkt.RST) {
		// Data in a RST is diagnostic text, never delivered to the application.
		return false, notes
	}
	if h.isKeepAlive(s, off) {
		// The probe's byte (if any) is garbage, so it is neither placed nor compared.
		return false, append(notes, model.FlagKeepAlive)
	}

	end := off + int64(s.PayloadLen)
	var pl placement
	var newTail int64
	outOfOrder := false
	if s.PayloadLen > 0 {
		received := h.capturedEnd()
		pl = h.place(off, s.Payload, in.Number, last)
		outOfOrder = pl.fresh > 0 && pl.firstFresh < received
		if s.Truncated() {
			newTail = h.addTail(off+int64(len(s.Payload)), end, in.Number)
		}
	}
	retransmission := h.isRetransmission(s, end, pl.fresh > 0 || newTail > 0)
	h.advance(s, end)

	if retransmission {
		notes = append(notes, model.FlagRetransmission)
	}
	if outOfOrder {
		notes = append(notes, model.FlagOutOfOrder)
	}
	if pl.conflict {
		notes = append(notes, model.FlagOverlapConflict)
	}
	return pl.fresh > 0 || pl.won || newTail > 0, notes
}

// isKeepAlive reports a probe at SND.NXT-1 carrying no data or one garbage byte.
func (h *half) isKeepAlive(s *netpkt.Segment, off int64) bool {
	return s.PayloadLen <= 1 && s.Flags&(netpkt.SYN|netpkt.FIN) == 0 && off == h.next-1
}

// isRetransmission reports a segment all of whose sequence numbers were seen
// before. brought tells whether its payload added any bytes or truncated range.
func (h *half) isRetransmission(s *netpkt.Segment, end int64, brought bool) bool {
	syn, fin := s.Flags.Has(netpkt.SYN), s.Flags.Has(netpkt.FIN)
	switch {
	case s.PayloadLen == 0 && !syn && !fin:
		return false // a pure ACK occupies no sequence space
	case s.PayloadLen > 0 && (brought || end > h.next):
		return false
	case syn && !h.synDone:
		return false
	case fin && !(h.finDone && end <= h.finAt):
		return false
	}
	return true
}

// advance moves the next expected offset past the segment.
func (h *half) advance(s *netpkt.Segment, end int64) {
	h.next = max(h.next, end)
	if s.Flags.Has(netpkt.SYN) {
		h.synDone = true
	}
	if s.Flags.Has(netpkt.FIN) {
		h.next = max(h.next, end+1)
		h.finAt = max(h.finAt, end)
		h.finDone = true
	}
}
