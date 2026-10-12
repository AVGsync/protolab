package tcpflow

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

// transmission is one client segment of a randomized capture.
type transmission struct {
	off      int
	data     []byte
	captured int
}

// byteModel is a deliberately naive per-byte reference implementation of the
// placement rules: every byte remembers its value, owner and rivals.
type byteModel struct {
	last      bool
	received  []bool
	value     []byte
	owner     []int
	rivals    [][]int
	announced []bool // listed by some truncated tail
	next      int    // furthest logical end so far
	high      int    // furthest received byte + 1
}

type expectedPacket struct {
	notes  []string
	offset int
}

func newByteModel(n int, last bool) *byteModel {
	return &byteModel{
		last: last, received: make([]bool, n), value: make([]byte, n),
		owner: make([]int, n), rivals: make([][]int, n), announced: make([]bool, n),
	}
}

func (m *byteModel) apply(pkt int, tx transmission) expectedPacket {
	fresh, firstFresh, newTail := 0, -1, 0
	conflict, won := false, false
	highBefore := m.high
	for i, v := range tx.data[:tx.captured] {
		pos := tx.off + i
		switch {
		case !m.received[pos]:
			m.received[pos], m.value[pos], m.owner[pos] = true, v, pkt
			if fresh == 0 {
				firstFresh = pos
			}
			fresh++
			m.high = max(m.high, pos+1)
		case m.value[pos] != v:
			conflict = true
			if m.last {
				m.rivals[pos] = append(slices.Clone(m.rivals[pos]), m.owner[pos])
				m.owner[pos], m.value[pos], won = pkt, v, true
			} else {
				m.rivals[pos] = append(slices.Clone(m.rivals[pos]), pkt)
			}
		}
	}
	for pos := tx.off + tx.captured; pos < tx.off+len(tx.data); pos++ {
		if !m.received[pos] && !m.announced[pos] {
			newTail++
		}
		if !m.received[pos] {
			m.announced[pos] = true
		}
	}
	end := tx.off + len(tx.data)
	var notes []string
	if fresh == 0 && newTail == 0 && end <= m.next {
		notes = append(notes, model.FlagRetransmission)
	}
	if fresh > 0 && firstFresh < highBefore {
		notes = append(notes, model.FlagOutOfOrder)
	}
	if conflict {
		notes = append(notes, model.FlagOverlapConflict)
	}
	if tx.captured < len(tx.data) {
		notes = append(notes, model.FlagTruncated)
	}
	m.next = max(m.next, end)
	offset := -1
	if fresh > 0 || won || newTail > 0 {
		offset = tx.off
	}
	return expectedPacket{notes: notes, offset: offset}
}

func (m *byteModel) gaps() []Gap {
	gaps := []Gap{}
	for i := 0; i < len(m.received); {
		if m.received[i] {
			i++
			continue
		}
		j := i
		for j < len(m.received) && !m.received[j] && m.announced[j] == m.announced[i] {
			j++
		}
		reason := model.GapLost
		if m.announced[i] {
			reason = model.GapTruncated
		}
		gaps = append(gaps, Gap{Offset: i, Length: j - i, Reason: reason})
		i = j
	}
	return gaps
}

func (m *byteModel) conflicts() []Conflict {
	out := []Conflict{}
	for i := range m.received {
		if len(m.rivals[i]) == 0 {
			continue
		}
		pkts := append(slices.Clone(m.rivals[i]), m.owner[i])
		slices.Sort(pkts)
		if n := len(out); n > 0 && out[n-1].Offset+out[n-1].Length == i && out[n-1].Chosen == m.owner[i] && slices.Equal(out[n-1].Packets, pkts) {
			out[n-1].Length++
			continue
		}
		out = append(out, Conflict{Offset: i, Length: 1, Packets: pkts, Chosen: m.owner[i]})
	}
	return out
}

// randomCapture cuts a random stream into segments, then loses, duplicates,
// corrupts, truncates and reorders some of them.
func randomCapture(rnd *rand.Rand, n int) []transmission {
	stream := make([]byte, n)
	for i := range stream {
		stream[i] = byte(rnd.IntN(256))
	}
	piece := func(from, to int) transmission {
		return transmission{off: from, data: slices.Clone(stream[from:to]), captured: to - from}
	}
	var txs []transmission
	for pos := 0; pos < n; {
		size := min(2+rnd.IntN(120), n-pos)
		if n-pos-size == 1 {
			size++ // no 1-byte segments: they could look like keep-alive probes
		}
		if rnd.Float64() >= 0.1 {
			txs = append(txs, piece(pos, pos+size))
		}
		if rnd.Float64() < 0.2 {
			from := max(0, pos-rnd.IntN(16))
			to := min(n, pos+size+rnd.IntN(16))
			tx := piece(from, to)
			if rnd.Float64() < 0.4 {
				for range 1 + rnd.IntN(4) {
					tx.data[rnd.IntN(len(tx.data))] ^= byte(1 + rnd.IntN(255))
				}
			}
			txs = append(txs, tx)
		}
		pos += size
	}
	for i := range txs {
		if rnd.Float64() < 0.07 {
			txs[i].captured = rnd.IntN(len(txs[i].data))
		}
	}
	for i := 0; i+1 < len(txs); i++ {
		if rnd.Float64() < 0.2 {
			txs[i], txs[i+1] = txs[i+1], txs[i]
		}
	}
	return txs
}

func TestReassemblyMatchesByteModel(t *testing.T) {
	for run := range 400 {
		rnd := rand.New(rand.NewPCG(uint64(run), 42))
		n := 2 + rnd.IntN(4000)
		last := run%2 == 1
		isn := rnd.Uint32()
		if run%5 == 0 {
			isn = 0xffffffff - uint32(rnd.IntN(3000)) // stream crosses 2^32
		}
		txs := randomCapture(rnd, n)

		pkts := []pk{C(S, isn, 0, ""), Sv(SA, si, isn+1, ""), C(A, isn+1, s1, "")}
		ref := newByteModel(n, last)
		want := map[int]expectedPacket{}
		for _, tx := range txs {
			p := C(PA, isn+1+uint32(tx.off), s1, string(tx.data))
			if tx.captured < len(tx.data) {
				p = p.cut(tx.captured)
			}
			pkts = append(pkts, p)
			want[len(pkts)] = ref.apply(len(pkts), tx)
		}
		end := isn + 1 + uint32(n)
		pkts = append(pkts, C(FA, end, s1, ""), Sv(FA, s1, end+1, ""), C(A, end+1, s1+1, ""))

		policy := PolicyFirst
		if last {
			policy = PolicyLast
		}
		s := one(t, Options{OverlapPolicy: policy}, pkts)
		st := s.Stream(model.C2S)

		if !slices.Equal(st.Data, ref.value) {
			t.Fatalf("run %d: data differs from the model", run)
		}
		if got, exp := st.Gaps, ref.gaps(); !slices.Equal(got, exp) {
			t.Fatalf("run %d: gaps\n got %+v\nwant %+v", run, got, exp)
		}
		gotC, expC := st.Conflicts, ref.conflicts()
		if !slices.EqualFunc(gotC, expC, func(a, b Conflict) bool {
			return a.Offset == b.Offset && a.Length == b.Length && a.Chosen == b.Chosen && slices.Equal(a.Packets, b.Packets)
		}) {
			t.Fatalf("run %d: conflicts\n got %+v\nwant %+v", run, gotC, expC)
		}
		var wantSegs [][3]int
		for _, p := range s.Packets {
			exp, ok := want[p.Number]
			if !ok {
				continue
			}
			if exp.notes == nil {
				exp.notes = []string{}
			}
			if !slices.Equal(p.Annotations, exp.notes) || p.StreamOffset != exp.offset {
				t.Fatalf("run %d packet #%d: notes=%v offset=%d, want %v %d", run, p.Number, p.Annotations, p.StreamOffset, exp.notes, exp.offset)
			}
			if exp.offset >= 0 {
				wantSegs = append(wantSegs, [3]int{exp.offset, p.PayloadLen, p.Number})
			}
		}
		slices.SortStableFunc(wantSegs, func(a, b [3]int) int { return a[0] - b[0] })
		var gotSegs [][3]int
		for _, sg := range st.Segments {
			gotSegs = append(gotSegs, [3]int{sg.Offset, sg.Length, sg.Packet})
		}
		if !slices.Equal(gotSegs, wantSegs) {
			t.Fatalf("run %d: segments\n got %v\nwant %v", run, gotSegs, wantSegs)
		}
		wantStatus := statusComplete
		if len(st.Gaps) > 0 || len(st.Conflicts) > 0 {
			wantStatus = statusPartial
		}
		if s.Status != wantStatus {
			t.Fatalf("run %d: status %s, want %s", run, s.Status, wantStatus)
		}
	}
}
