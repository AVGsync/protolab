package tcpflow

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/pcap"
)

// TestCaptureFileEndToEnd writes frames into a pcapng file with a small
// snaplen, reads them back and reassembles them, as the application does.
func TestCaptureFileEndToEnd(t *testing.T) {
	type frame struct {
		fromServer bool
		flags      netpkt.TCPFlags
		seq, ack   uint32
		data       string
		badSum     bool
	}
	long := strings.Repeat("L", 150) // longer than the snaplen allows
	frames := []frame{
		{false, S, ci, 0, "", false},
		{true, SA, si, c1, "", false},
		{false, A, c1, s1, "", false},
		{false, PA, c1, s1, "READ 0001\n", false},         // 4
		{true, PA, s1, c1 + 10, "part-one;", true},        // 5: damaged checksum, the other server frames verify
		{true, PA, s1 + 18, c1 + 10, "part-three", false}, // 6: part two is late
		{true, PA, s1 + 9, c1 + 10, "part-two;", false},   // 7
		{true, PA, s1 + 9, c1 + 10, "part-two;", false},   // 8: duplicate
		{false, PA, c1 + 10, s1 + 28, long, false},        // 9: truncated by snaplen
		{false, FA, c1 + 160, s1 + 28, "", false},         // 10
		{true, FA, s1 + 28, c1 + 161, "", false},          // 11
		{false, A, c1 + 161, s1 + 29, "", false},          // 12
	}

	const snapLen = 120
	var file bytes.Buffer
	w, err := pcap.NewNgWriter(&file, pcap.LinkTypeEthernet, snapLen)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range frames {
		src, dst := client, server
		if f.fromServer {
			src, dst = server, client
		}
		b := netpkt.BuildEthernetTCP(src, dst, f.seq, f.ack, f.flags, 8192, []byte(f.data), netpkt.BuildOptions{BadChecksum: f.badSum})
		if len(b) < 60 {
			b = append(b, make([]byte, 60-len(b))...) // minimum Ethernet frame, as seen on the receiver
		}
		if err := w.WritePacket(t0.Add(time.Duration(i)*time.Millisecond), b, len(b)); err != nil {
			t.Fatal(err)
		}
	}

	r, err := pcap.NewReader(&file)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAssembler(Options{TargetPorts: []int{502}})
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
		a.Add(Input{Number: p.Number, Timestamp: p.Timestamp, Seg: seg})
	}
	sessions := a.Finish()
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions", len(sessions))
	}
	s := sessions[0]

	wantData(t, s, model.S2C, "part-one;part-two;part-three")
	wantNotes(t, s, 5, model.FlagChecksumWarning)
	wantNotes(t, s, 7, model.FlagOutOfOrder)
	wantNotes(t, s, 8, model.FlagRetransmission)
	wantNotes(t, s, 9, model.FlagTruncated)

	c2s := s.Stream(model.C2S)
	captured := snapLen - 54 // Ethernet + IPv4 + TCP headers
	if got := string(c2s.Data[10 : 10+captured]); got != long[:captured] {
		t.Errorf("captured part of the long message = %q", got)
	}
	wantGaps(t, s, model.C2S, Gap{Offset: 10 + captured, Length: 150 - captured, Reason: model.GapTruncated})
	if len(c2s.Data) != 160 || !s.Closed || !s.Handshake || s.Status != statusPartial {
		t.Errorf("length=%d closed=%v handshake=%v status=%s", len(c2s.Data), s.Closed, s.Handshake, s.Status)
	}
	got := codes(s)
	if want := []string{codeTruncated, codeChecksumBad}; !slices.Equal(got, want) {
		t.Errorf("diagnostics = %v, want %v", got, want)
	}
}
