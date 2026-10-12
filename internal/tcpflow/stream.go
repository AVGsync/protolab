package tcpflow

import (
	"bytes"
	"slices"
	"sort"

	"github.com/AVGsync/protolab/internal/model"
)

// span is a range of received bytes. Spans of a direction are sorted and never
// overlap; together they are the known part of the stream.
type span struct {
	start, end int64
	owner      int   // packet whose bytes are in the stream data
	rivals     []int // packets that carried different bytes for this range
	weak       bool  // the owner failed the checksum check (builder.suspect)
}

// rng is a half-open range [start, end).
type rng struct{ start, end int64 }

// rangeSet is a sorted set of disjoint, non-touching ranges.
type rangeSet []rng

// add inserts [start, end) and returns how many of its bytes were not in the set.
func (s *rangeSet) add(start, end int64) int64 {
	if start >= end {
		return 0
	}
	set := *s
	i := sort.Search(len(set), func(k int) bool { return set[k].end >= start })
	j := i
	covered := int64(0)
	merged := rng{start, end}
	for ; j < len(set) && set[j].start <= end; j++ {
		covered += min(set[j].end, end) - max(set[j].start, start)
		merged.start = min(merged.start, set[j].start)
		merged.end = max(merged.end, set[j].end)
	}
	*s = slices.Replace(set, i, j, merged)
	return end - start - covered
}

// first returns the first range ending after pos.
func (s rangeSet) first(pos int64) (rng, bool) {
	i := sort.Search(len(s), func(k int) bool { return s[k].end > pos })
	if i == len(s) {
		return rng{}, false
	}
	return s[i], true
}

// tailRef remembers which packet left a truncated tail, for diagnostics.
type tailRef struct {
	start, end int64
	packet     int
}

// half is one direction of a connection: its sequence space and the stream
// reassembled from it.
type half struct {
	dir   model.Direction
	base  uint32 // sequence number of stream offset 0
	known bool   // base comes from the handshake (StartKnown)
	seq   seqUnwrapper

	// Extent of the stream, measured before placement.
	dataEnd  int64 // end of the furthest payload, truncated tails included
	finSeen  bool
	finOff   int64 // offset of the FIN's sequence number
	ackHigh  int64 // highest offset acknowledged by the peer
	length   int64
	tooLarge bool // cut by a limit
	overall  bool // the limit was the capture-wide one

	// Placement state.
	data      []byte
	spans     []span
	truncated rangeSet // bytes announced by truncated packets and not captured
	tails     []tailRef
	next      int64 // next expected offset: furthest payload end or FIN+1 so far
	synDone   bool
	finDone   bool
	finAt     int64
	segments  []Segment
}

func newHalf(dir model.Direction) *half { return &half{dir: dir} }

func (h *half) setBase(base uint32, known bool) {
	h.base, h.known = base, known
	h.seq = seqUnwrapper{origin: base}
}

// ref is the sequence number that relative numbers count from: the ISN, or
// the number just before the first byte when the ISN was not captured.
func (h *half) ref() uint32 { return h.base - 1 }

// noteAck records an acknowledgment of this direction's data by the peer.
func (h *half) noteAck(ack uint32) {
	h.ackHigh = max(h.ackHigh, h.seq.at(ack))
}

// finishExtent fixes the stream length from data ends, the FIN and the peer's
// acknowledgments, applying the per-stream limit and what is left of the
// capture-wide one.
func (h *half) finishExtent(limit int64, left *int64) {
	ackLen := h.ackHigh
	if h.finSeen && ackLen > h.finOff {
		ackLen-- // the FIN itself occupies one sequence number
	}
	n := max(h.dataEnd, ackLen, 0)
	if h.finSeen {
		n = max(n, h.finOff)
	}
	if n > limit {
		n, h.tooLarge = limit, true
	}
	if n > *left {
		n, h.tooLarge, h.overall = max(*left, 0), true, true
	}
	*left -= n
	h.length = n
	h.data = make([]byte, n)
}

// capturedEnd is the end of the furthest bytes received so far.
func (h *half) capturedEnd() int64 {
	if len(h.spans) == 0 {
		return 0
	}
	return h.spans[len(h.spans)-1].end
}

// placement is what happened to the captured part of a payload.
type placement struct {
	fresh      int64 // bytes received for the first time
	firstFresh int64 // offset of the first of them
	conflict   bool  // some already received bytes differed
	won        bool  // and this packet's copy replaced them (PolicyLast)
}

// copyTo is the copy of s narrowed to [start, end).
func (s span) copyTo(start, end int64) span {
	s.start, s.end = start, end
	return s
}

// place merges payload, which starts at stream offset off, into the stream.
// weak marks a copy that failed the checksum check (builder.suspect).
func (h *half) place(off int64, payload []byte, pkt int, last, weak bool) placement {
	from, to := max(off, 0), min(off+int64(len(payload)), h.length)
	if from >= to {
		return placement{}
	}
	if from >= h.capturedEnd() {
		// In-order data, the common case: nothing to compare against.
		copy(h.data[from:to], payload[from-off:to-off])
		h.spans = appendSpan(h.spans, span{start: from, end: to, owner: pkt, weak: weak})
		return placement{fresh: to - from, firstFresh: from}
	}

	var p placement
	i := sort.Search(len(h.spans), func(k int) bool { return h.spans[k].end > from })
	j := i
	var out []span // replaces h.spans[i:j]
	cur := from
	for ; j < len(h.spans) && h.spans[j].start < to; j++ {
		s := h.spans[j]
		if s.start < cur {
			out = appendSpan(out, s.copyTo(s.start, cur))
		} else if s.start > cur {
			out = h.fill(out, cur, s.start, off, payload, pkt, weak, &p)
		}
		ovFrom, ovTo := max(s.start, cur), min(s.end, to)
		out = h.compare(out, s, ovFrom, ovTo, off, payload, pkt, last, weak, &p)
		if s.end > to {
			out = appendSpan(out, s.copyTo(to, s.end))
		}
		cur = ovTo
	}
	if cur < to {
		out = h.fill(out, cur, to, off, payload, pkt, weak, &p)
	}
	h.spans = slices.Replace(h.spans, i, j, out...)
	return p
}

// fill writes bytes nobody delivered before.
func (h *half) fill(out []span, from, to, off int64, payload []byte, pkt int, weak bool, p *placement) []span {
	copy(h.data[from:to], payload[from-off:to-off])
	if p.fresh == 0 {
		p.firstFresh = from
	}
	p.fresh += to - from
	return appendSpan(out, span{start: from, end: to, owner: pkt, weak: weak})
}

// compare checks bytes of span s in [from, to) against the new copy. Equal runs
// are duplicates; differing runs are conflicts resolved by the overlap policy,
// except that a copy with a valid checksum beats a damaged one.
func (h *half) compare(out []span, s span, from, to, off int64, payload []byte, pkt int, last, weak bool, p *placement) []span {
	old := h.data[from:to]
	cand := payload[from-off : to-off]
	confirmed := s
	confirmed.weak = s.weak && weak // an intact copy vouches for the same bytes
	if bytes.Equal(old, cand) {
		return appendSpan(out, confirmed.copyTo(from, to))
	}
	p.conflict = true
	replace := last
	if weak != s.weak {
		replace = !weak
	}
	for k := 0; k < len(old); {
		same := old[k] == cand[k]
		n := k + 1
		for n < len(old) && (old[n] == cand[n]) == same {
			n++
		}
		a, b := from+int64(k), from+int64(n)
		switch {
		case same:
			out = appendSpan(out, confirmed.copyTo(a, b))
		case replace:
			copy(old[k:n], cand[k:n])
			p.won = true
			out = appendSpan(out, span{start: a, end: b, owner: pkt, rivals: withRival(s.rivals, s.owner), weak: weak})
		default:
			out = appendSpan(out, span{start: a, end: b, owner: s.owner, rivals: withRival(s.rivals, pkt), weak: s.weak})
		}
		k = n
	}
	return out
}

// hasWeakBytes reports stream bytes that only a damaged copy delivered.
func (h *half) hasWeakBytes() bool {
	return slices.ContainsFunc(h.spans, func(s span) bool { return s.weak })
}

// appendSpan appends s, merging it into the previous span when they touch and
// have the same owner, rivals and verification.
func appendSpan(spans []span, s span) []span {
	if n := len(spans); n > 0 {
		prev := &spans[n-1]
		if prev.end == s.start && prev.owner == s.owner && prev.weak == s.weak && slices.Equal(prev.rivals, s.rivals) {
			prev.end = s.end
			return spans
		}
	}
	return append(spans, s)
}

func withRival(rivals []int, pkt int) []int {
	if slices.Contains(rivals, pkt) {
		return rivals
	}
	out := make([]int, len(rivals), len(rivals)+1)
	copy(out, rivals)
	return append(out, pkt)
}

// addTail records [from, to) as announced by a truncated packet but not
// captured, and returns how many of those bytes were neither received nor
// already known to be truncated.
func (h *half) addTail(from, to int64, pkt int) int64 {
	from, to = max(from, 0), min(to, h.length)
	if from >= to {
		return 0
	}
	h.tails = append(h.tails, tailRef{from, to, pkt})
	added := int64(0)
	i := sort.Search(len(h.spans), func(k int) bool { return h.spans[k].end > from })
	for ; i < len(h.spans) && from < to; i++ {
		s := h.spans[i]
		if s.start >= to {
			break
		}
		added += h.truncated.add(from, s.start)
		from = max(from, s.end)
	}
	return added + h.truncated.add(from, to)
}

// gaps lists the unknown ranges of [0, length): lost unless a truncated packet
// announced them.
func (h *half) gaps() []Gap {
	gaps := []Gap{}
	pos := int64(0)
	for _, s := range h.spans {
		if s.start > pos {
			gaps = h.appendHole(gaps, pos, s.start)
		}
		pos = max(pos, s.end)
	}
	if pos < h.length {
		gaps = h.appendHole(gaps, pos, h.length)
	}
	return gaps
}

func (h *half) appendHole(gaps []Gap, from, to int64) []Gap {
	for from < to {
		r, ok := h.truncated.first(from)
		if !ok || r.start >= to {
			return append(gaps, Gap{Offset: int(from), Length: int(to - from), Reason: model.GapLost})
		}
		if r.start > from {
			gaps = append(gaps, Gap{Offset: int(from), Length: int(r.start - from), Reason: model.GapLost})
			from = r.start
		}
		end := min(r.end, to)
		gaps = append(gaps, Gap{Offset: int(from), Length: int(end - from), Reason: model.GapTruncated})
		from = end
	}
	return gaps
}

// conflicts lists the ranges for which different copies were received.
func (h *half) conflicts() []Conflict {
	out := []Conflict{}
	for _, s := range h.spans {
		if len(s.rivals) == 0 {
			continue
		}
		pkts := append(slices.Clone(s.rivals), s.owner)
		slices.Sort(pkts)
		if n := len(out); n > 0 {
			prev := &out[n-1]
			if int64(prev.Offset+prev.Length) == s.start && prev.Chosen == s.owner && slices.Equal(prev.Packets, pkts) {
				prev.Length += int(s.end - s.start)
				continue
			}
		}
		out = append(out, Conflict{Offset: int(s.start), Length: int(s.end - s.start), Packets: pkts, Chosen: s.owner})
	}
	return out
}

// neighbours returns the packets whose bytes end right before from and start
// right at to: the context of a gap.
func (h *half) neighbours(from, to int64) []int {
	var pkts []int
	i := sort.Search(len(h.spans), func(k int) bool { return h.spans[k].end >= from })
	if i < len(h.spans) && h.spans[i].end == from {
		pkts = append(pkts, h.spans[i].owner)
		i++
	}
	if i < len(h.spans) && h.spans[i].start == to {
		pkts = append(pkts, h.spans[i].owner)
	}
	return pkts
}

// truncatedBy returns up to limit packets whose truncated tails overlap [from, to).
func (h *half) truncatedBy(from, to int64, limit int) []int {
	var pkts []int
	for _, t := range h.tails {
		if t.start < to && t.end > from && !slices.Contains(pkts, t.packet) {
			pkts = append(pkts, t.packet)
			if len(pkts) == limit {
				break
			}
		}
	}
	slices.Sort(pkts)
	return pkts
}

func (h *half) stream() *Stream {
	slices.SortStableFunc(h.segments, func(a, b Segment) int { return a.Offset - b.Offset })
	if h.segments == nil {
		h.segments = []Segment{}
	}
	return &Stream{
		Dir:        h.dir,
		Data:       h.data,
		StartKnown: h.known,
		Segments:   h.segments,
		Gaps:       h.gaps(),
		Conflicts:  h.conflicts(),
	}
}
