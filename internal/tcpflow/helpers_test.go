package tcpflow

import (
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
)

const (
	S   = netpkt.SYN
	A   = netpkt.ACK
	F   = netpkt.FIN
	R   = netpkt.RST
	P   = netpkt.PSH
	SA  = S | A
	FA  = F | A
	PA  = P | A
	RA  = R | A
	ci  = 1000 // client ISN
	si  = 9000 // server ISN
	c1  = ci + 1
	s1  = si + 1
	dt  = time.Millisecond
	cli = "10.0.0.1:40000"
	srv = "10.0.0.2:502"
)

var (
	client = netip.MustParseAddrPort(cli)
	server = netip.MustParseAddrPort(srv)
	t0     = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
)

// pk describes one segment of a test capture.
type pk struct {
	fromServer bool
	flags      netpkt.TCPFlags
	seq, ack   uint32
	data       string
	captured   int  // when > 0 (or snap is set), only this many payload bytes were captured
	snap       bool // captured is meaningful even when 0
	badSum     bool
	src, dst   netip.AddrPort // override the default endpoints
}

func C(flags netpkt.TCPFlags, seq, ack uint32, data string) pk {
	return pk{flags: flags, seq: seq, ack: ack, data: data}
}

func Sv(flags netpkt.TCPFlags, seq, ack uint32, data string) pk {
	return pk{fromServer: true, flags: flags, seq: seq, ack: ack, data: data}
}

func (p pk) cut(n int) pk                     { p.captured, p.snap = n, true; return p }
func (p pk) bad() pk                          { p.badSum = true; return p }
func (p pk) ports(src, dst netip.AddrPort) pk { p.src, p.dst = src, dst; return p }

func (p pk) segment() netpkt.Segment {
	src, dst := client, server
	if p.fromServer {
		src, dst = server, client
	}
	if p.src.IsValid() {
		src, dst = p.src, p.dst
	}
	payload := []byte(p.data)
	if p.snap {
		payload = payload[:p.captured]
	}
	s := netpkt.Segment{
		Src: src, Dst: dst, Seq: p.seq, Ack: p.ack, Flags: p.flags, Window: 8192,
		Payload: payload, PayloadLen: len(p.data), Checksum: netpkt.ChecksumOK,
	}
	if p.snap {
		s.Checksum = netpkt.ChecksumUnknown
	}
	if p.badSum {
		s.Checksum = netpkt.ChecksumBad
	}
	return s
}

// handshake is SYN, SYN-ACK, ACK with the default ISNs.
func handshake() []pk {
	return []pk{C(S, ci, 0, ""), Sv(SA, si, c1, ""), C(A, c1, s1, "")}
}

// closing is a FIN exchange once the client sent cn and the server sn bytes.
func closing(cn, sn uint32) []pk {
	return []pk{
		C(FA, c1+cn, s1+sn, ""),
		Sv(FA, s1+sn, c1+cn+1, ""),
		C(A, c1+cn+1, s1+sn+1, ""),
	}
}

func seq(parts ...[]pk) []pk { return slices.Concat(parts...) }

func assemble(t *testing.T, opt Options, pkts []pk) []*Session {
	t.Helper()
	a := NewAssembler(opt)
	for i, p := range pkts {
		a.Add(Input{Number: i + 1, Timestamp: t0.Add(time.Duration(i) * dt), Seg: p.segment()})
	}
	return a.Finish()
}

func one(t *testing.T, opt Options, pkts []pk) *Session {
	t.Helper()
	ss := assemble(t, opt, pkts)
	if len(ss) != 1 {
		t.Fatalf("got %d sessions, want 1", len(ss))
	}
	return ss[0]
}

func wantData(t *testing.T, s *Session, dir model.Direction, want string) {
	t.Helper()
	if got := string(s.Stream(dir).Data); got != want {
		t.Errorf("%s data = %q, want %q", dir, got, want)
	}
}

func wantGaps(t *testing.T, s *Session, dir model.Direction, want ...Gap) {
	t.Helper()
	got := s.Stream(dir).Gaps
	if want == nil {
		want = []Gap{}
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s gaps = %+v, want %+v", dir, got, want)
	}
}

func wantNotes(t *testing.T, s *Session, number int, want ...string) {
	t.Helper()
	p := s.Packets[slices.IndexFunc(s.Packets, func(p Packet) bool { return p.Number == number })]
	if want == nil {
		want = []string{}
	}
	if !slices.Equal(p.Annotations, want) {
		t.Errorf("packet #%d annotations = %v, want %v", number, p.Annotations, want)
	}
}

func packet(s *Session, number int) Packet {
	return s.Packets[slices.IndexFunc(s.Packets, func(p Packet) bool { return p.Number == number })]
}

func codes(s *Session) []string {
	var out []string
	for _, d := range s.Diagnostics {
		out = append(out, d.Code)
	}
	return out
}

func diag(t *testing.T, s *Session, code string) model.Diagnostic {
	t.Helper()
	i := slices.IndexFunc(s.Diagnostics, func(d model.Diagnostic) bool { return d.Code == code })
	if i < 0 {
		t.Fatalf("no %s diagnostic in %v", code, codes(s))
	}
	return s.Diagnostics[i]
}
