package tcpflow

import (
	"encoding/binary"
	"net/netip"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
)

// FuzzAssembler reads the input as a list of 8-byte segment descriptions over
// a small sequence space, so overlaps, gaps, wraps and flag mixes are frequent.
func FuzzAssembler(f *testing.F) {
	f.Add([]byte{0x02, 0, 0, 0, 0, 0, 0, 0, 0x12, 0, 0, 0, 0, 1, 0, 0, 0x18, 1, 0, 0, 1, 1, 5, 3})
	f.Add([]byte{0x18, 0xf0, 0xff, 0xff, 0xff, 0, 40, 0x18, 0x18, 0x10, 0, 0, 0, 0, 40, 9})
	f.Fuzz(func(t *testing.T, raw []byte) {
		a := NewAssembler(Options{OverlapPolicy: []string{PolicyFirst, PolicyLast}[len(raw)%2], TargetPorts: []int{502}, MaxStreamBytes: 300})
		cli := netip.MustParseAddrPort("10.0.0.1:40000")
		srv := netip.MustParseAddrPort("10.0.0.2:502")
		for i := 0; i+8 <= len(raw) && i < 8*400; i += 8 {
			d := raw[i : i+8]
			src, dst := cli, srv
			if d[0]&0x80 != 0 {
				src, dst = srv, cli
			}
			// Sequence numbers near 0 and near 2^32 so wraparound is exercised.
			seq := binary.BigEndian.Uint32(d[1:5])
			seq = seq&0x1ff - 0x100
			n := int(d[5] % 64)
			payload := make([]byte, n)
			for k := range payload {
				payload[k] = d[6] + byte(k%3)
			}
			captured := n
			if d[7]&0x80 != 0 {
				captured = int(d[7]&0x7f) % (n + 1)
			}
			seg := netpkt.Segment{
				Src: src, Dst: dst, Seq: seq, Ack: seq ^ uint32(d[7]) - 0x80,
				Flags: netpkt.TCPFlags(d[0] & 0x3f), Payload: payload[:captured], PayloadLen: n,
			}
			a.Add(Input{Number: i/8 + 1, Seg: seg})
		}
		for _, s := range a.Finish() {
			for _, st := range s.Streams {
				checkStreamInvariants(t, st)
			}
		}
	})
}

func checkStreamInvariants(t *testing.T, st *Stream) {
	t.Helper()
	n := len(st.Data)
	if n > 300 {
		t.Fatalf("stream longer than the limit: %d", n)
	}
	known := make([]bool, n)
	pos := 0
	for _, g := range st.Gaps {
		if g.Offset < pos || g.Length <= 0 || g.Offset+g.Length > n {
			t.Fatalf("bad gap %+v (pos %d, len %d)", g, pos, n)
		}
		if g.Reason != model.GapLost && g.Reason != model.GapTruncated {
			t.Fatalf("gap reason %q", g.Reason)
		}
		for k := g.Offset; k < g.Offset+g.Length; k++ {
			known[k] = true
		}
		pos = g.Offset + g.Length
	}
	for _, c := range st.Conflicts {
		if c.Offset < 0 || c.Length <= 0 || c.Offset+c.Length > n || len(c.Packets) < 2 {
			t.Fatalf("bad conflict %+v", c)
		}
	}
	for i, sg := range st.Segments {
		if sg.Offset < 0 || sg.Length < 0 || sg.Offset+sg.Length > n || (i > 0 && st.Segments[i-1].Offset > sg.Offset) {
			t.Fatalf("bad segment %+v", sg)
		}
	}
}
