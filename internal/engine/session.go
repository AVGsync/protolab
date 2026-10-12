package engine

import (
	"bytes"
	"sort"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

func parseSession(s *spec.Spec, in SessionInput) ParsedSession {
	c := compile(s)
	sess := c.newSession(&in, true)
	b := c.newBuilder(&in)
	out := ParsedSession{Messages: make([]model.Message, len(sess.order)), Unframed: sess.unframed}
	for i, ref := range sess.order {
		st, f := sess.frame(ref)
		m := b.message(st, f)
		m.Index = i
		out.Messages[i] = m
	}
	return out
}

// session is a framed session: both streams with their frames, the time order
// of all frames and the unframed ranges. Messages are built from it one at a
// time, so verification never holds a whole session of parsed messages.
type session struct {
	in       *SessionInput
	streams  [2]*stream
	order    []frameRef
	unframed []Unframed
}

type frameRef struct {
	dir uint8 // index into session.streams
	idx int32 // index into stream.frames
}

// newSession frames both directions. With own set the stream bytes are
// copied, because the messages built from them escape to the caller.
func (c *compiled) newSession(in *SessionInput, own bool) *session {
	s := &session{in: in, unframed: []Unframed{}}
	for d, dir := range model.Directions {
		st := newStream(dir, in.Streams[d], own)
		st.frameStream(&c.framing)
		for _, u := range st.unframed {
			s.unframed = append(s.unframed, Unframed{Direction: dir, Offset: u.Offset, Length: u.Length})
		}
		s.streams[d] = st
	}
	s.order = timeOrder(s.streams)
	return s
}

func (s *session) frame(ref frameRef) (*stream, *frame) {
	st := s.streams[ref.dir]
	return st, &st.frames[ref.idx]
}

// timeOrder sorts the frames of both directions by timestamp, then by stream
// offset, keeping client frames first on ties (a stable sort of c2s followed
// by s2c). Timestamps are fixed-width UTC strings, so they compare as text.
// Each direction is usually already in order, which allows a linear merge.
func timeOrder(streams [2]*stream) []frameRef {
	a, b := streams[0].frames, streams[1].frames
	out := make([]frameRef, 0, len(a)+len(b))
	before := func(x, y *frame) bool {
		if x.ts != y.ts {
			return x.ts < y.ts
		}
		return x.offset < y.offset
	}

	if framesSorted(a) && framesSorted(b) {
		i, j := 0, 0
		for i < len(a) && j < len(b) {
			if before(&b[j], &a[i]) {
				out = append(out, frameRef{dir: 1, idx: int32(j)})
				j++
			} else {
				out = append(out, frameRef{dir: 0, idx: int32(i)})
				i++
			}
		}
		for ; i < len(a); i++ {
			out = append(out, frameRef{dir: 0, idx: int32(i)})
		}
		for ; j < len(b); j++ {
			out = append(out, frameRef{dir: 1, idx: int32(j)})
		}
		return out
	}

	for i := range a {
		out = append(out, frameRef{dir: 0, idx: int32(i)})
	}
	for j := range b {
		out = append(out, frameRef{dir: 1, idx: int32(j)})
	}
	sort.SliceStable(out, func(x, y int) bool {
		fx := &streams[out[x].dir].frames[out[x].idx]
		fy := &streams[out[y].dir].frames[out[y].idx]
		return before(fx, fy)
	})
	return out
}

// framesSorted reports whether frames of one stream are in time order.
// Offsets grow along a stream, so only timestamps need checking; segments
// captured out of order can break it.
func framesSorted(fs []frame) bool {
	for i := 1; i < len(fs); i++ {
		if fs[i].ts < fs[i-1].ts {
			return false
		}
	}
	return true
}

// builder turns frames of one session into messages.
type builder struct {
	c         *compiled
	sessionID string
	bound     [][]boundAssertion
	parser    fieldParser
	matched   []int
}

func (c *compiled) newBuilder(in *SessionInput) *builder {
	return &builder{c: c, sessionID: in.Session.ID, bound: c.bindAssertions(in.Actions)}
}

func (b *builder) message(st *stream, f *frame) model.Message {
	end := f.offset + f.length
	data := st.data[f.offset:end:end]
	unknown := st.unknownSlice(f.offset, end)

	m := model.Message{
		SessionID:     b.sessionID,
		Direction:     st.dir,
		Offset:        f.offset,
		Length:        f.length,
		Timestamp:     f.ts,
		PacketNumbers: st.segs.packets(f.offset, f.length),
		Issues:        make([]string, len(f.issues), len(f.issues)+1),
		Fields:        []model.ParsedField{},
		Data:          data,
	}
	copy(m.Issues, f.issues)

	b.parser.reset(data, unknown)
	b.matched = b.c.matchRules(b.matched[:0], data, unknown, st.dir)
	var r *rule
	if len(b.matched) > 0 {
		r = &b.c.rules[b.matched[0]]
		id := r.id
		m.RuleID, m.RuleTitle = &id, r.title
		m.Fields = b.parser.parse(r.fields, 0, "")
		if b.bound != nil {
			applyAssertions(&m, r, b.bound[b.matched[0]])
		}
	}
	m.Unknown = uncovered(b.parser.covered)

	if len(b.matched) > 1 {
		m.CandidateRuleIDs = make([]string, len(b.matched))
		for i, ri := range b.matched {
			m.CandidateRuleIDs[i] = b.c.rules[ri].id
		}
		m.Issues = append(m.Issues, "Подходят правила: "+strings.Join(m.CandidateRuleIDs, ", "))
	}

	switch {
	case r == nil:
		m.Status = model.MsgUnknownType
	case f.partial:
		m.Status = model.MsgPartial
	case len(b.matched) > 1:
		m.Status = model.MsgAmbiguous
	case anyViolation(m.Fields):
		m.Status = model.MsgViolation
	default:
		m.Status = model.MsgOK
	}
	return m
}

// matchRules appends the indices of all rules matching a message. Gap bytes
// never satisfy a match condition, even when their zero fill would.
func (c *compiled) matchRules(dst []int, msg, unknown []byte, dir model.Direction) []int {
	for i := range c.rules {
		if c.rules[i].matches(msg, unknown, dir) {
			dst = append(dst, i)
		}
	}
	return dst
}

func (r *rule) matches(msg, unknown []byte, dir model.Direction) bool {
	if r.direction != "" && r.direction != "any" && r.direction != string(dir) {
		return false
	}
	for _, m := range r.match {
		end := m.offset + len(m.pattern)
		if m.pattern == nil || m.offset < 0 || end > len(msg) {
			return false
		}
		if unknown != nil && bytes.IndexByte(unknown[m.offset:end], 1) >= 0 {
			return false
		}
		if !bytes.Equal(msg[m.offset:end], m.pattern) {
			return false
		}
	}
	return true
}

// uncovered returns the ranges of zero bytes in covered.
func uncovered(covered []byte) []model.ByteRange {
	out := []model.ByteRange{}
	for pos := 0; pos < len(covered); {
		start := bytes.IndexByte(covered[pos:], 0)
		if start < 0 {
			break
		}
		start += pos
		end := bytes.IndexByte(covered[start:], 1)
		if end < 0 {
			end = len(covered)
		} else {
			end += start
		}
		out = append(out, model.ByteRange{Offset: start, Length: end - start})
		pos = end
	}
	return out
}

// anyViolation looks for a violation anywhere in a field tree.
func anyViolation(fields []model.ParsedField) bool {
	for i := range fields {
		if fields[i].Status == model.FieldViolation || anyViolation(fields[i].Children) {
			return true
		}
	}
	return false
}
