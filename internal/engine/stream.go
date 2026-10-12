package engine

import (
	"bytes"
	"slices"
	"sort"

	"github.com/AVGsync/protolab/internal/model"
)

// stream is one direction of a session prepared for framing and parsing.
type stream struct {
	dir  model.Direction
	data []byte
	// unknown has a 1 for every byte inside a gap. It is nil when the stream
	// has no gaps, which keeps the common case free of mask lookups.
	unknown []byte
	gaps    []model.StreamGap
	segs    segmentIndex

	frames   []frame
	unframed []model.ByteRange
}

func newStream(dir model.Direction, in StreamInput, own bool) *stream {
	data := in.Bytes
	if own {
		data = bytes.Clone(in.Bytes)
	}
	return &stream{
		dir:     dir,
		data:    data,
		unknown: unknownMask(len(data), in.Gaps),
		gaps:    in.Gaps,
		segs:    newSegmentIndex(in.Segments),
	}
}

func unknownMask(n int, gaps []model.StreamGap) []byte {
	var mask []byte
	for _, g := range gaps {
		start, end := max(0, g.Offset), min(n, g.Offset+g.Length)
		if end <= start {
			continue
		}
		if mask == nil {
			mask = make([]byte, n)
		}
		for i := start; i < end; i++ {
			mask[i] = 1
		}
	}
	return mask
}

// anyUnknown reports whether [from, to) contains a gap byte.
func (st *stream) anyUnknown(from, to int) bool {
	return st.unknown != nil && bytes.IndexByte(st.unknown[from:to], 1) >= 0
}

func (st *stream) countUnknown(from, to int) int {
	if st.unknown == nil {
		return 0
	}
	return bytes.Count(st.unknown[from:to], []byte{1})
}

// skipUnknown returns the first known offset at or after pos.
func (st *stream) skipUnknown(pos int) int {
	if st.unknown == nil || pos >= len(st.unknown) {
		return pos
	}
	i := bytes.IndexByte(st.unknown[pos:], 0)
	if i < 0 {
		return len(st.unknown)
	}
	return pos + i
}

// unknownSlice returns the mask of [from, to), or nil when nothing is unknown.
func (st *stream) unknownSlice(from, to int) []byte {
	if st.unknown == nil {
		return nil
	}
	return st.unknown[from:to:to]
}

// segmentIndex answers "which segments overlap this range" in logarithmic
// time. Reassembled segments are normally sorted and disjoint, but overlapping
// copies (partial retransmissions) are handled too: maxEnd lets the search skip
// every segment that ends before the range regardless of overlaps.
type segmentIndex struct {
	segs   []model.StreamSegment
	order  []int // segment indices sorted by offset
	maxEnd []int // maxEnd[k] = max end of segs[order[0..k]]
}

func newSegmentIndex(segs []model.StreamSegment) segmentIndex {
	order := make([]int, len(segs))
	for i := range order {
		order[i] = i
	}
	sorted := slices.IsSortedFunc(segs, func(a, b model.StreamSegment) int { return a.Offset - b.Offset })
	if !sorted {
		sort.SliceStable(order, func(a, b int) bool { return segs[order[a]].Offset < segs[order[b]].Offset })
	}
	maxEnd := make([]int, len(segs))
	end := 0
	for k, i := range order {
		end = max(end, segs[i].Offset+segs[i].Length)
		maxEnd[k] = end
	}
	return segmentIndex{segs: segs, order: order, maxEnd: maxEnd}
}

// overlapping returns the positions in x.order of segments that may overlap
// [from, to): the caller still filters segments that end before from.
func (x *segmentIndex) overlapping(from, to int) (lo, hi int) {
	lo = sort.Search(len(x.order), func(k int) bool { return x.maxEnd[k] > from })
	hi = lo
	for hi < len(x.order) && x.segs[x.order[hi]].Offset < to {
		hi++
	}
	return lo, hi
}

// timestampAt is the timestamp of the segment holding byte offset; when the
// byte was never captured it falls back to the last segment, like the
// reference implementation. Among overlapping copies the earliest listed wins.
func (x *segmentIndex) timestampAt(offset int) string {
	if len(x.segs) == 0 {
		return ""
	}
	best := -1
	lo, hi := x.overlapping(offset, offset+1)
	for _, i := range x.order[lo:hi] {
		if s := x.segs[i]; s.Offset+s.Length > offset && (best < 0 || i < best) {
			best = i
		}
	}
	if best < 0 {
		best = len(x.segs) - 1
	}
	return x.segs[best].Timestamp
}

// packets returns the sorted numbers of the packets overlapping [offset, offset+length).
func (x *segmentIndex) packets(offset, length int) []int {
	lo, hi := x.overlapping(offset, offset+length)
	out := make([]int, 0, hi-lo)
	for _, i := range x.order[lo:hi] {
		if s := x.segs[i]; s.Offset+s.Length > offset {
			out = append(out, s.PacketNumber)
		}
	}
	slices.Sort(out)
	return out
}
