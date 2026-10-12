package engine

import (
	"bytes"
	"slices"
	"sort"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

func parseSession(s *spec.Spec, in SessionInput) ParsedSession {
	c := compile(s)
	sess := c.newSession(&in, true)
	b := c.newBuilder(&in, false)
	out := ParsedSession{Messages: make([]model.Message, len(sess.order)), Unframed: sess.unframed}
	for i, ref := range sess.order {
		st, f := sess.frame(ref)
		b.build(&out.Messages[i], st, f)
		out.Messages[i].Index = i
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
	a, b := streams[0], streams[1]
	out := make([]frameRef, 0, len(a.frames)+len(b.frames))
	before := func(x, y frameRef) bool {
		sx, sy := streams[x.dir], streams[y.dir]
		fx, fy := &sx.frames[x.idx], &sy.frames[y.idx]
		if x.dir != y.dir || fx.seg != fy.seg {
			if tx, ty := sx.timestampOf(fx), sy.timestampOf(fy); tx != ty {
				return tx < ty
			}
		}
		return fx.offset < fy.offset
	}

	if a.framesInTimeOrder() && b.framesInTimeOrder() {
		i, j := 0, 0
		for i < len(a.frames) && j < len(b.frames) {
			ra, rb := frameRef{dir: 0, idx: int32(i)}, frameRef{dir: 1, idx: int32(j)}
			if before(rb, ra) {
				out = append(out, rb)
				j++
			} else {
				out = append(out, ra)
				i++
			}
		}
		for ; i < len(a.frames); i++ {
			out = append(out, frameRef{dir: 0, idx: int32(i)})
		}
		for ; j < len(b.frames); j++ {
			out = append(out, frameRef{dir: 1, idx: int32(j)})
		}
		return out
	}

	for i := range a.frames {
		out = append(out, frameRef{dir: 0, idx: int32(i)})
	}
	for j := range b.frames {
		out = append(out, frameRef{dir: 1, idx: int32(j)})
	}
	sort.SliceStable(out, func(x, y int) bool { return before(out[x], out[y]) })
	return out
}

// framesInTimeOrder reports whether the frames of a stream are already sorted
// by time. Offsets grow along a stream, so only timestamps need checking;
// segments captured out of order can break it.
func (st *stream) framesInTimeOrder() bool {
	for i := 1; i < len(st.frames); i++ {
		prev, cur := &st.frames[i-1], &st.frames[i]
		if prev.seg != cur.seg && st.timestampOf(cur) < st.timestampOf(prev) {
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

	// lean is set when messages are only tallied and then dropped: display
	// texts and packet lists are skipped and buffers are reused across
	// messages, so a message is valid only until the next one is built.
	lean       bool
	unknownBuf []model.ByteRange

	ruleIDs []*string // Message.RuleID of each rule, shared by its messages
	ints    []int     // chunk that packet number lists are carved from
}

func (c *compiled) newBuilder(in *SessionInput, lean bool) *builder {
	b := &builder{
		c:         c,
		sessionID: in.Session.ID,
		bound:     c.bindAssertions(in.Actions),
		parser:    fieldParser{lean: lean},
		lean:      lean,
		ruleIDs:   make([]*string, len(c.rules)),
	}
	for i := range c.rules {
		id := c.rules[i].id
		b.ruleIDs[i] = &id
	}
	return b
}

// build fills m, which must be zero, with the message cut out by f.
// Messages are large, so they are written in place rather than returned.
func (b *builder) build(m *model.Message, st *stream, f *frame) {
	end := f.offset + f.length
	data := st.data[f.offset:end:end]
	unknown := st.unknownSlice(f.offset, end)

	m.SessionID = b.sessionID
	m.Direction = st.dir
	m.Offset, m.Length = f.offset, f.length
	m.Timestamp = st.timestampOf(f)
	m.Issues = []string{}
	m.Fields = []model.ParsedField{}
	m.Data = data
	if issues := st.issuesOf(f); len(issues) > 0 {
		m.Issues = append(make([]string, 0, len(issues)+1), issues...)
	}
	if !b.lean {
		m.PacketNumbers = b.packets(st, f)
	}

	b.parser.reset(data, unknown)
	b.matched = b.c.matchRules(b.matched[:0], data, unknown, st.dir)
	var r *rule
	if len(b.matched) > 0 {
		r = &b.c.rules[b.matched[0]]
		var bound []boundAssertion
		if b.bound != nil {
			bound = b.bound[b.matched[0]]
		}
		m.RuleID, m.RuleTitle = b.ruleIDs[b.matched[0]], r.title
		b.parser.keepValues = !b.lean || len(bound) > 0
		m.Fields = b.parser.parse(r.fields, 0, "")
		applyAssertions(m, r, bound)
	}
	b.unknownBuf = appendUncovered(b.unknownBuf[:0], b.parser.covered)
	if b.lean {
		m.Unknown = b.unknownBuf
	} else {
		m.Unknown = append([]model.ByteRange{}, b.unknownBuf...)
	}

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
}

// packets lists the packets of a message. Lists are carved out of shared
// chunks: most messages span one or two packets, and allocating each list
// separately would dominate the cost of large sessions.
func (b *builder) packets(st *stream, f *frame) []int {
	lo, hi := st.segs.overlapping(f.offset, f.offset+f.length)
	if lo == hi {
		return []int{}
	}
	if len(b.ints)+hi-lo > cap(b.ints) {
		b.ints = make([]int, 0, max(hi-lo, 4096))
	}
	start := len(b.ints)
	for _, i := range st.segs.order[lo:hi] {
		if s := st.segs.segs[i]; s.Offset+s.Length > f.offset {
			b.ints = append(b.ints, s.PacketNumber)
		}
	}
	out := b.ints[start:len(b.ints):len(b.ints)]
	slices.Sort(out)
	return out
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

// appendUncovered appends the ranges of zero bytes in covered to out.
func appendUncovered(out []model.ByteRange, covered []byte) []model.ByteRange {
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
