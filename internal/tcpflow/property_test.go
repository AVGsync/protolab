package tcpflow

import (
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/pcap"
)

// wireSeg is one TCP segment of a generated capture.
type wireSeg struct {
	fromServer bool
	flags      netpkt.TCPFlags
	seq, ack   uint32
	payload    []byte
}

// copies cuts a stream of n bytes into random segments and sends them with
// identical duplicates, partial overlaps and local reordering. When lose is
// set, one segment is never sent. It returns the payload transmissions as
// stream ranges and the lost range.
func copies(rnd *rand.Rand, n, minSize int, lose bool) (out [][2]int, lost [2]int) {
	var cuts [][2]int
	for pos := 0; pos < n; {
		size := min(minSize+rnd.IntN(1460-minSize+1), n-pos)
		if rnd.IntN(4) == 0 {
			size = min(minSize+rnd.IntN(8), n-pos) // plenty of tiny segments
		}
		if n-pos-size < minSize && n-pos-size > 0 {
			size = n - pos
		}
		cuts = append(cuts, [2]int{pos, pos + size})
		pos += size
	}
	if lose && len(cuts) > 0 {
		lost = cuts[rnd.IntN(len(cuts))]
	}
	clear := func(r [2]int) bool { return r[1] <= lost[0] || r[0] >= lost[1] }
	for _, c := range cuts {
		if !clear(c) {
			continue // the lost segment
		}
		out = append(out, c)
		if rnd.Float64() < 0.15 {
			out = append(out, c) // identical retransmission
		}
		if rnd.Float64() < 0.2 {
			// A retransmission straddling neighbouring segments.
			from := max(0, c[0]-rnd.IntN(64))
			to := min(n, c[1]+rnd.IntN(64))
			if r := [2]int{from, to}; clear(r) && to-from >= minSize {
				out = append(out, r)
			}
		}
	}
	// Local reordering: every copy may move up to 6 places.
	for i := range out {
		j := min(len(out)-1, i+rnd.IntN(7))
		out[i], out[j] = out[j], out[i]
	}
	return out, lost
}

// direction generates the segments of one direction, acknowledged by the peer.
func direction(rnd *rand.Rand, fromServer bool, isn, peerISN uint32, stream []byte, ranges [][2]int) (data, acks []wireSeg) {
	received := make([]bool, len(stream))
	prefix := 0
	for _, r := range ranges {
		data = append(data, wireSeg{fromServer, PA, isn + 1 + uint32(r[0]), peerISN + 1, stream[r[0]:r[1]]})
		for i := r[0]; i < r[1]; i++ {
			received[i] = true
		}
		for prefix < len(stream) && received[prefix] {
			prefix++
		}
		if rnd.IntN(2) == 0 {
			acks = append(acks, wireSeg{!fromServer, A, peerISN + 1, isn + 1 + uint32(prefix), nil})
		}
	}
	return data, acks
}

// interleave merges lists keeping the order within each list.
func interleave(rnd *rand.Rand, lists ...[]wireSeg) []wireSeg {
	var out []wireSeg
	for {
		var live []int
		for i, l := range lists {
			if len(l) > 0 {
				live = append(live, i)
			}
		}
		if len(live) == 0 {
			return out
		}
		i := live[rnd.IntN(len(live))]
		out = append(out, lists[i][0])
		lists[i] = lists[i][1:]
	}
}

// pipeline writes segs into a pcapng file, reads it back, decodes and
// reassembles it, as the application does.
func pipeline(t *testing.T, opt Options, segs []wireSeg) []*Session {
	t.Helper()
	var file bytes.Buffer
	w, err := pcap.NewNgWriter(&file, pcap.LinkTypeEthernet, 65535)
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range segs {
		src, dst := client, server
		if s.fromServer {
			src, dst = server, client
		}
		frame := netpkt.BuildEthernetTCP(src, dst, s.seq, s.ack, s.flags, 8192, s.payload, netpkt.BuildOptions{})
		if len(frame) < 60 {
			frame = append(frame, make([]byte, 60-len(frame))...) // Ethernet padding
		}
		if err := w.WritePacket(t0.Add(time.Duration(i)*time.Microsecond), frame, len(frame)); err != nil {
			t.Fatal(err)
		}
	}
	r, err := pcap.NewReader(&file)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAssembler(opt)
	for {
		p, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		seg, err := netpkt.Decode(p.LinkType, p.Data, p.OrigLen)
		if err != nil {
			t.Fatalf("packet %d: %v", p.Number, err)
		}
		if seg.Checksum != netpkt.ChecksumOK {
			t.Fatalf("packet %d: checksum %d", p.Number, seg.Checksum)
		}
		a.Add(Input{Number: p.Number, Timestamp: p.Timestamp, Seg: seg})
	}
	return a.Finish()
}

// TestPropertyReassemblyOfIdenticalCopies: whatever the segmentation,
// duplication, partial overlap (with identical bytes) and reordering, the
// stream is the original byte for byte, with no gaps or conflicts; when one
// segment is lost, exactly its range is a gap and everything else is intact.
func TestPropertyReassemblyOfIdenticalCopies(t *testing.T) {
	runs := 300
	if testing.Short() {
		runs = 60
	}
	for run := range runs {
		rnd := rand.New(rand.NewPCG(uint64(run), 2026))
		midstream := run%4 == 3
		lose := !midstream && run%3 == 1

		var streams [2][]byte
		var lost [2][2]int // per direction, empty when nothing is lost
		var lists [][]wireSeg
		isn := [2]uint32{rnd.Uint32(), rnd.Uint32()}
		if run%5 == 0 {
			isn[0] = 0xffffffff - uint32(rnd.IntN(5000)) // both streams cross 2^32
			isn[1] = 0xffffffff - uint32(rnd.IntN(5000))
		}
		for d := range 2 {
			n := 1 + rnd.IntN(8000)
			if d == 1 && rnd.IntN(4) == 0 {
				n = 0 // a direction without data
			}
			streams[d] = make([]byte, n)
			for i := range streams[d] {
				streams[d][i] = byte(rnd.IntN(256))
			}
			minSize := 1
			if midstream {
				// Without a handshake a lone byte below what was already sent
				// is indistinguishable from a keep-alive probe.
				minSize = 2
			}
			var ranges [][2]int
			ranges, lost[d] = copies(rnd, n, minSize, lose && rnd.IntN(2) == 0)
			data, acks := direction(rnd, d == 1, isn[d], isn[1-d], streams[d], ranges)
			lists = append(lists, data, acks)
		}
		segs := interleave(rnd, lists...)
		cEnd, sEnd := isn[0]+1+uint32(len(streams[0])), isn[1]+1+uint32(len(streams[1]))
		closing := []wireSeg{
			{false, FA, cEnd, sEnd, nil},
			{true, FA, sEnd, cEnd + 1, nil},
			{false, A, cEnd + 1, sEnd + 1, nil},
		}
		opt := Options{}
		if midstream {
			opt.TargetPorts = []int{int(server.Port())}
		} else {
			hs := []wireSeg{{false, S, isn[0], 0, nil}, {true, SA, isn[1], isn[0] + 1, nil}, {false, A, isn[0] + 1, isn[1] + 1, nil}}
			segs = append(hs, segs...)
		}
		segs = append(segs, closing...)

		ss := pipeline(t, opt, segs)
		if len(ss) != 1 {
			t.Fatalf("run %d: %d sessions", run, len(ss))
		}
		s := ss[0]
		for d, dir := range model.Directions {
			st := s.Stream(dir)
			want := streams[d]
			var wantGaps []Gap
			if r := lost[d]; r[1] > r[0] {
				want = slices.Clone(want)
				clear(want[r[0]:r[1]]) // gap bytes are zero; Gaps marks them
				wantGaps = []Gap{{Offset: r[0], Length: r[1] - r[0], Reason: model.GapLost}}
			}
			if !bytes.Equal(st.Data, want) {
				i := 0
				for i < len(want) && i < len(st.Data) && st.Data[i] == want[i] {
					i++
				}
				t.Logf("lost %v gaps %+v conflicts %+v", lost[d], st.Gaps, st.Conflicts)
				for _, p := range s.Packets {
					if p.Dir == dir && p.PayloadLen > 0 {
						t.Logf("#%d seq=%d len=%d off=%d %v", p.Number, p.Seq, p.PayloadLen, p.StreamOffset, p.Annotations)
					}
				}
				t.Fatalf("run %d %s: data differs at %d (len %d, want %d)", run, dir, i, len(st.Data), len(want))
			}
			if wantGaps == nil {
				wantGaps = []Gap{}
			}
			if !slices.Equal(st.Gaps, wantGaps) {
				t.Fatalf("run %d %s: gaps %+v, want %+v", run, dir, st.Gaps, wantGaps)
			}
			if len(st.Conflicts) != 0 {
				t.Fatalf("run %d %s: conflicts %+v", run, dir, st.Conflicts)
			}
			if st.StartKnown == midstream {
				t.Fatalf("run %d %s: startKnown=%v", run, dir, st.StartKnown)
			}
			// Every stream byte is attributed to a packet: provenance holds.
			covered := make([]bool, len(st.Data))
			for _, sg := range st.Segments {
				for i := sg.Offset; i < sg.Offset+sg.Length; i++ {
					covered[i] = true
				}
			}
			for _, g := range st.Gaps {
				for i := g.Offset; i < g.Offset+g.Length; i++ {
					covered[i] = true
				}
			}
			if i := slices.Index(covered, false); i >= 0 {
				t.Fatalf("run %d %s: byte %d belongs to no segment", run, dir, i)
			}
		}
		wantStatus := statusComplete
		if lost[0][1] > lost[0][0] || lost[1][1] > lost[1][0] || midstream {
			wantStatus = statusPartial
		}
		if s.Status != wantStatus {
			t.Fatalf("run %d: status %s, want %s (%v)", run, s.Status, wantStatus, codes(s))
		}
		if !midstream && (s.Roles != RolesHandshake || !s.Closed) {
			t.Fatalf("run %d: roles=%s closed=%v", run, s.Roles, s.Closed)
		}
	}
}
