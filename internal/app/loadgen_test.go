package app

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"io"
	"math/rand/v2"
	"net/netip"
	"slices"
	"time"

	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/pcap"
)

// The control load of the task: a capture of about 100 MiB with 250,000
// packets, 1,000 TCP connections and 100,000 ThermoLink messages (short
// commands, responses up to 64 KiB), with background traffic around it.

// loadPlan sizes a synthetic capture.
type loadPlan struct {
	Seed            uint64
	TargetConns     int // ThermoLink connections to port 5020
	BackgroundConns int // TLS-like bulk transfers to port 443
	Exchanges       int // request/response pairs over all target connections
	DumpResponses   int // responses of DumpMin..DumpMax bytes of payload
	DumpMin         int
	DumpMax         int
	BackgroundBytes int // server bytes over all background connections
	NoiseFrames     int // UDP, ARP and IPv6 frames
	Duration        time.Duration
	// Every ReuseEvery-th target connection reuses the client port of the
	// previous one after it closed: a new instance on the same 4-tuple.
	ReuseEvery int
}

func controlLoadPlan() loadPlan {
	return loadPlan{
		Seed:            2026,
		TargetConns:     900,
		BackgroundConns: 100,
		Exchanges:       50_000,
		DumpResponses:   1_000,
		DumpMin:         16 << 10,
		DumpMax:         65_535,
		BackgroundBytes: 30 << 20,
		NoiseFrames:     12_000,
		Duration:        10 * time.Minute,
		ReuseEvery:      50,
	}
}

// loadStats describes what was generated: the expected results.
type loadStats struct {
	Bytes       int64
	Packets     int
	TCPConns    int // connection instances
	TargetConns int
	Messages    int // ThermoLink messages, both directions
	Retransmits int // identical retransmissions
	Reordered   int // swapped segment pairs
	MaxMessage  int // largest message, bytes
}

const (
	loadDevicePort = 5020
	loadMSS        = 1460
	typeDump       = 0x05 // dump request; the response type is 0x85
)

type genFrame struct {
	at    int64 // ns since the start of the capture
	frame []byte
}

type generator struct {
	rnd    *rand.Rand
	frames []genFrame
	stats  loadStats
}

func (g *generator) emit(at time.Duration, frame []byte) {
	g.frames = append(g.frames, genFrame{at: int64(at), frame: frame})
}

// tcpConn tracks the sequence numbers of a generated connection.
type tcpConn struct {
	g        *generator
	cli, srv netip.AddrPort
	next     [2]uint32 // next sequence number of client, server
	at       time.Duration
}

func (c *tcpConn) send(fromServer bool, flags netpkt.TCPFlags, payload []byte, delay time.Duration) {
	c.at += delay
	self, peer := 0, 1
	src, dst := c.cli, c.srv
	if fromServer {
		self, peer, src, dst = 1, 0, c.srv, c.cli
	}
	var ack uint32
	if flags.Has(netpkt.ACK) {
		ack = c.next[peer]
	}
	c.g.emit(c.at, netpkt.BuildEthernetTCP(src, dst, c.next[self], ack, flags, 64240, payload, netpkt.BuildOptions{}))
	c.next[self] += uint32(len(payload))
	if flags&(netpkt.SYN|netpkt.FIN) != 0 {
		c.next[self]++
	}
}

// bulk sends data in MSS-sized segments, acknowledged every two segments,
// with an occasional identical retransmission or swapped pair.
func (c *tcpConn) bulk(fromServer bool, data []byte, first time.Duration) {
	type seg struct {
		at   time.Duration
		seq  uint32
		data []byte
	}
	self := 0
	src, dst := c.cli, c.srv
	if fromServer {
		self, src, dst = 1, c.srv, c.cli
	}
	var segs []seg
	c.at += first
	for off := 0; off < len(data); off += loadMSS {
		piece := data[off:min(off+loadMSS, len(data))]
		segs = append(segs, seg{c.at, c.next[self], piece})
		c.next[self] += uint32(len(piece))
		c.at += 12 * time.Microsecond
		if len(segs)%2 == 0 || off+loadMSS >= len(data) {
			c.send(!fromServer, netpkt.ACK, nil, 30*time.Microsecond)
		}
	}
	if len(segs) > 2 && c.g.rnd.IntN(100) == 0 {
		i := c.g.rnd.IntN(len(segs) - 1)
		segs[i].at, segs[i+1].at = segs[i+1].at, segs[i].at
		c.g.stats.Reordered++
	}
	for _, s := range segs {
		frame := netpkt.BuildEthernetTCP(src, dst, s.seq, c.next[1-self], netpkt.PSH|netpkt.ACK, 64240, s.data, netpkt.BuildOptions{})
		c.g.emit(s.at, frame)
		if c.g.rnd.IntN(200) == 0 {
			c.g.emit(s.at+200*time.Millisecond, frame) // retransmission after an RTO
			c.g.stats.Retransmits++
		}
	}
}

func (c *tcpConn) open(at time.Duration) {
	c.at = at
	c.send(false, netpkt.SYN, nil, 0)
	c.send(true, netpkt.SYN|netpkt.ACK, nil, 300*time.Microsecond)
	c.send(false, netpkt.ACK, nil, 200*time.Microsecond)
}

func (c *tcpConn) close() {
	c.send(false, netpkt.FIN|netpkt.ACK, nil, 15*time.Millisecond)
	c.send(true, netpkt.FIN|netpkt.ACK, nil, 400*time.Microsecond)
	c.send(false, netpkt.ACK, nil, 150*time.Microsecond)
}

// tlFrame is a ThermoLink frame: A5 | type | seq | len u16 LE | payload | sum8.
func tlFrame(typ, seq byte, payload []byte) []byte {
	out := make([]byte, 0, len(payload)+6)
	out = append(out, 0xA5, typ, seq, byte(len(payload)), byte(len(payload)>>8))
	out = append(out, payload...)
	var sum byte
	for _, b := range out {
		sum += b
	}
	return append(out, sum)
}

func (g *generator) randomBytes(n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n; i += 8 {
		var w [8]byte
		binary.LittleEndian.PutUint64(w[:], g.rnd.Uint64())
		copy(b[i:], w[:])
	}
	return b
}

// exchange returns a request and its response.
func (g *generator) exchange(seq byte, dump bool) (req, resp []byte) {
	if dump {
		n := 16<<10 + g.rnd.IntN(65_535-16<<10+1)
		return tlFrame(typeDump, seq, []byte{byte(g.rnd.IntN(16)), 0}), tlFrame(typeDump|0x80, seq, g.randomBytes(n))
	}
	switch k := g.rnd.IntN(10); {
	case k < 3:
		params := 1 + g.rnd.IntN(6)
		p := []byte{byte(params)}
		for id := 1; id <= params; id++ {
			p = append(p, byte(id), byte(g.rnd.IntN(256)), byte(g.rnd.IntN(4)))
		}
		return tlFrame(0x01, seq, nil), tlFrame(0x81, seq, p)
	case k < 5:
		param := byte(1 + g.rnd.IntN(6))
		return tlFrame(0x02, seq, []byte{param, byte(g.rnd.IntN(256)), 0}), tlFrame(0x82, seq, []byte{0, param})
	case k < 8:
		ch := byte(1 + g.rnd.IntN(4))
		p := append([]byte{ch}, g.randomBytes(8)...)
		return tlFrame(0x03, seq, []byte{ch}), tlFrame(0x83, seq, p)
	case k < 9:
		n := 1 + g.rnd.IntN(255)
		return tlFrame(0x04, seq, []byte{byte(n)}), tlFrame(0x84, seq, append([]byte{byte(n)}, g.randomBytes(8*n)...))
	default:
		return tlFrame(0x10, seq, nil), tlFrame(0x90, seq, nil)
	}
}

// writeControlLoad generates the capture and writes it as pcapng.
func writeControlLoad(w io.Writer, plan loadPlan) (loadStats, error) {
	g := &generator{rnd: rand.New(rand.NewPCG(plan.Seed, 42))}
	span := plan.Duration * 8 / 10

	// Spread the exchanges and the large responses over the connections.
	exchanges := make([]int, plan.TargetConns)
	for range plan.Exchanges {
		exchanges[g.rnd.IntN(plan.TargetConns)]++
	}
	dumpAt := map[[2]int]bool{}
	for len(dumpAt) < plan.DumpResponses {
		c := g.rnd.IntN(plan.TargetConns)
		if exchanges[c] > 0 {
			dumpAt[[2]int{c, g.rnd.IntN(exchanges[c])}] = true
		}
	}

	var prevEnd time.Duration
	var prevCli netip.AddrPort
	for i := range plan.TargetConns {
		dev := netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 10, byte(50 + i%10)}), loadDevicePort)
		cli := netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 10, byte(100 + i%50)}), uint16(40000+i))
		start := time.Duration(g.rnd.Int64N(int64(span)))
		if plan.ReuseEvery > 0 && i > 0 && i%plan.ReuseEvery == 0 {
			// Same 4-tuple as the previous connection, after it closed.
			cli, start = prevCli, prevEnd+time.Second
			dev = netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 10, byte(50 + (i-1)%10)}), loadDevicePort)
		}
		c := &tcpConn{g: g, cli: cli, srv: dev, next: [2]uint32{g.rnd.Uint32(), g.rnd.Uint32()}}
		c.open(start)
		for k := range exchanges[i] {
			req, resp := g.exchange(byte(k), dumpAt[[2]int{i, k}])
			g.stats.Messages += 2
			g.stats.MaxMessage = max(g.stats.MaxMessage, len(resp))
			c.send(false, netpkt.PSH|netpkt.ACK, req, time.Duration(50+g.rnd.IntN(250))*time.Millisecond)
			if len(resp) <= loadMSS {
				c.send(true, netpkt.PSH|netpkt.ACK, resp, time.Duration(1+g.rnd.IntN(4))*time.Millisecond)
				c.send(false, netpkt.ACK, nil, 200*time.Microsecond)
			} else {
				c.bulk(true, resp, 3*time.Millisecond)
			}
		}
		c.close()
		prevEnd, prevCli = c.at, cli
		g.stats.TCPConns++
		g.stats.TargetConns++
	}

	for i := range plan.BackgroundConns {
		cli := netip.AddrPortFrom(netip.AddrFrom4([4]byte{192, 168, 10, byte(150 + i%20)}), uint16(50000+i))
		srv := netip.AddrPortFrom(netip.AddrFrom4([4]byte{93, 184, 216, byte(1 + i%30)}), 443)
		c := &tcpConn{g: g, cli: cli, srv: srv, next: [2]uint32{g.rnd.Uint32(), g.rnd.Uint32()}}
		c.open(time.Duration(g.rnd.Int64N(int64(span))))
		c.send(false, netpkt.PSH|netpkt.ACK, g.randomBytes(300+g.rnd.IntN(200)), time.Millisecond)
		n := plan.BackgroundBytes / plan.BackgroundConns
		c.bulk(true, g.randomBytes(n/2+g.rnd.IntN(n)), 20*time.Millisecond)
		c.close()
		g.stats.TCPConns++
	}

	for range plan.NoiseFrames {
		g.emit(time.Duration(g.rnd.Int64N(int64(plan.Duration))), g.noiseFrame())
	}

	slices.SortStableFunc(g.frames, func(a, b genFrame) int { return cmp.Compare(a.at, b.at) })
	bw := bufio.NewWriterSize(w, 1<<20)
	cw := &countWriter{w: bw}
	pw, err := pcap.NewNgWriter(cw, pcap.LinkTypeEthernet, 262144)
	if err != nil {
		return loadStats{}, err
	}
	base := time.Date(2026, 9, 14, 9, 0, 0, 0, time.UTC)
	for i, f := range g.frames {
		if err := pw.WritePacket(base.Add(time.Duration(f.at)), f.frame, len(f.frame)); err != nil {
			return loadStats{}, err
		}
		g.frames[i].frame = nil
	}
	if err := bw.Flush(); err != nil {
		return loadStats{}, err
	}
	g.stats.Packets = len(g.frames)
	g.stats.Bytes = cw.n
	return g.stats, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	return n, err
}

// noiseFrame is background traffic that is not TCP/IPv4: DNS over UDP, ARP, IPv6.
func (g *generator) noiseFrame() []byte {
	mac := []byte{0x02, 0, 0, 0, 0, 1, 0x02, 0, 0, 0, 0, 2}
	switch k := g.rnd.IntN(10); {
	case k < 7: // DNS query or answer
		payload := g.randomBytes(30 + g.rnd.IntN(120))
		ip := make([]byte, 20+8+len(payload))
		ip[0], ip[8], ip[9] = 0x45, 64, 17
		binary.BigEndian.PutUint16(ip[2:], uint16(len(ip)))
		copy(ip[12:], []byte{192, 168, 10, byte(100 + g.rnd.IntN(50)), 192, 168, 10, 1})
		binary.BigEndian.PutUint16(ip[20:], uint16(30000+g.rnd.IntN(30000)))
		binary.BigEndian.PutUint16(ip[22:], 53)
		binary.BigEndian.PutUint16(ip[24:], uint16(8+len(payload)))
		copy(ip[28:], payload)
		return slices.Concat(mac, []byte{0x08, 0x00}, ip)
	case k < 9: // ARP request, padded to the Ethernet minimum
		return slices.Concat([]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, mac[6:], []byte{0x08, 0x06}, make([]byte, 46))
	default: // IPv6 (unsupported, diagnosed)
		ip6 := make([]byte, 40+20)
		ip6[0], ip6[6], ip6[7] = 0x60, 6, 64
		binary.BigEndian.PutUint16(ip6[4:], 20)
		return slices.Concat(mac, []byte{0x86, 0xdd}, ip6)
	}
}
