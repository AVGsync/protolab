package engine

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/tcpflow"
)

// streamInput converts a reassembled direction the way the application does.
func streamInput(st *tcpflow.Stream) StreamInput {
	in := StreamInput{Bytes: st.Data}
	for _, g := range st.Gaps {
		in.Gaps = append(in.Gaps, model.StreamGap{Offset: g.Offset, Length: g.Length, Reason: g.Reason})
	}
	for _, s := range st.Segments {
		in.Segments = append(in.Segments, model.StreamSegment{Offset: s.Offset, Length: s.Length, PacketNumber: s.Packet, Timestamp: model.PacketTime(s.Timestamp), Flags: s.Annotations})
	}
	return in
}

// TestLostSegmentResynchronises: a lost segment carries the end of one message
// and the start of the next. The first is kept as partial with its unknown
// bytes marked; the framer then cannot trust the next header, skips to the
// sync pattern on known bytes, and the bytes in between belong to no message.
func TestLostSegmentResynchronises(t *testing.T) {
	m1 := tlFrame(0x01, 7)                // read params: 6 bytes, offsets 0-5
	m2 := tlFrame(0x02, 8, 0x01, 0xC8, 0) // set param: 9 bytes, offsets 6-14
	m3 := tlFrame(0x10, 9)                // heartbeat: 6 bytes, offsets 15-20
	m4 := tlFrame(0x03, 10, 0x01)         // measure: 7 bytes, offsets 21-27
	if m3[5] == 0xA5 || m3[3] == 0xA5 {
		t.Fatal("the tail of m3 must not look like a sync byte")
	}
	stream := concat(m1, m2, m3, m4)
	segA := stream[:11]   // m1 and the header of m2
	segB := stream[11:18] // lost: the rest of m2 and the start of m3
	segC := stream[18:]   // the rest of m3 and m4
	_ = segB

	cli := netip.MustParseAddrPort("192.168.10.20:50000")
	srv := netip.MustParseAddrPort("192.168.10.50:5020")
	a := tcpflow.NewAssembler(tcpflow.Options{TargetPorts: []int{5020}})
	add := func(n int, src, dst netip.AddrPort, seq, ack uint32, flags netpkt.TCPFlags, payload []byte) {
		a.Add(tcpflow.Input{Number: n, Timestamp: baseTime.Add(time.Duration(n) * time.Millisecond), Seg: netpkt.Segment{
			Src: src, Dst: dst, Seq: seq, Ack: ack, Flags: flags, Payload: payload, PayloadLen: len(payload), Checksum: netpkt.ChecksumOK,
		}})
	}
	const ci, si = 1000, 5000
	add(1, cli, srv, ci, 0, netpkt.SYN, nil)
	add(2, srv, cli, si, ci+1, netpkt.SYN|netpkt.ACK, nil)
	add(3, cli, srv, ci+1, si+1, netpkt.ACK, nil)
	add(4, cli, srv, ci+1, si+1, netpkt.PSH|netpkt.ACK, segA)
	add(5, cli, srv, ci+1+18, si+1, netpkt.PSH|netpkt.ACK, segC) // segB was not captured
	add(6, cli, srv, ci+1+uint32(len(stream)), si+1, netpkt.FIN|netpkt.ACK, nil)
	sessions := a.Finish()
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d", len(sessions))
	}
	c2s := sessions[0].Stream(model.C2S)
	if len(c2s.Gaps) != 1 || c2s.Gaps[0].Offset != 11 || c2s.Gaps[0].Length != 7 || c2s.Gaps[0].Reason != model.GapLost {
		t.Fatalf("gaps = %+v", c2s.Gaps)
	}

	got := ParseSession(thermoLink(t), SessionInput{
		Session: model.TcpSession{ID: "s"},
		Streams: [2]StreamInput{streamInput(c2s), streamInput(sessions[0].Stream(model.S2C))},
	})
	type want struct {
		offset, length int
		status         string
		packets        []int
	}
	exp := []want{
		{0, 6, model.MsgOK, []int{4}},
		{6, 9, model.MsgPartial, []int{4}},
		{21, 7, model.MsgOK, []int{5}},
	}
	if len(got.Messages) != len(exp) {
		t.Fatalf("messages = %d: %+v", len(got.Messages), got.Messages)
	}
	for i, w := range exp {
		m := got.Messages[i]
		if m.Offset != w.offset || m.Length != w.length || m.Status != w.status || !slices.Equal(m.PacketNumbers, w.packets) {
			t.Errorf("message %d: offset %d length %d status %s packets %v; want %+v", i, m.Offset, m.Length, m.Status, m.PacketNumbers, w)
		}
		if len(m.PacketNumbers) == 0 {
			t.Errorf("message %d has no packet numbers", i)
		}
	}
	// No field of the partial message reads a gap byte (message bytes 5-8) as data.
	partial := got.Messages[1]
	if !slices.Contains(partial.Issues, "4 Б неизвестны (пропуск в записи)") {
		t.Errorf("issues = %v", partial.Issues)
	}
	for _, f := range partial.Fields {
		overGap := f.Offset+f.Length > 5 && f.Offset < 9
		if overGap != (f.Status == model.FieldUnknown) || (overGap && f.Value != nil) {
			t.Errorf("field %s [%d,+%d): status %s value %v", f.Name, f.Offset, f.Length, f.Status, f.Value)
		}
	}
	if want := []Unframed{{Direction: model.C2S, Offset: 18, Length: 3}}; !slices.Equal(got.Unframed, want) {
		t.Errorf("unframed = %+v, want %+v", got.Unframed, want)
	}
}
