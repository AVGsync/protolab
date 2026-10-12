package demo

import (
	"net/netip"
	"slices"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
)

const tcpWindow = 64240

// packet is one TCP segment of a connection before it is turned into a frame.
type packet struct {
	at       time.Duration // from the start of the capture
	dir      model.Direction
	flags    netpkt.TCPFlags
	seq, ack uint32
	payload  []byte

	ipID        uint16
	badChecksum bool
	truncated   bool // must not fit into the snaplen
	captured    bool // false when the capture lost the packet
}

// connection is the packet timeline of one session.
type connection struct {
	spec    sessionSpec
	packets []*packet
	// data lists the payload packets of each direction in sending order.
	data [2][]*packet
}

func dirIndex(d model.Direction) int {
	if d == model.C2S {
		return 0
	}
	return 1
}

// timeline builds the packets of a session: handshake, the exchanges, and the
// closing handshake, then applies the capture defects.
func timeline(spec sessionSpec, seed uint32) *connection {
	rnd := newPRNG(seed)
	isn := [2]uint32{spec.clientISN, spec.serverISN}
	for i := range isn {
		if isn[i] == 0 {
			isn[i] = rnd.uint32()
		}
	}
	ipID := [2]uint16{uint16(rnd.uint32()), uint16(rnd.uint32())}

	c := &connection{spec: spec}
	next := isn // sequence number of the next byte each side sends
	at := spec.start

	send := func(dir model.Direction, flags netpkt.TCPFlags, payload []byte, delay time.Duration) {
		at += delay
		self, peer := dirIndex(dir), 1-dirIndex(dir)
		p := &packet{at: at, dir: dir, flags: flags, seq: next[self], payload: payload, captured: true}
		if flags.Has(netpkt.ACK) {
			p.ack = next[peer]
		}
		p.ipID = ipID[self]
		ipID[self]++

		next[self] += uint32(len(payload))
		if flags&(netpkt.SYN|netpkt.FIN) != 0 {
			next[self]++
		}
		c.packets = append(c.packets, p)
		if len(payload) > 0 {
			c.data[self] = append(c.data[self], p)
		}
	}
	segments := func(dir model.Direction, msg []byte, splits []int, first, rest time.Duration) {
		for i, piece := range split(msg, splits) {
			delay := rest
			if i == 0 {
				delay = first
			}
			send(dir, netpkt.PSH|netpkt.ACK, piece, delay)
		}
	}

	send(model.C2S, netpkt.SYN, nil, 0)
	send(model.S2C, netpkt.SYN|netpkt.ACK, nil, 310*us)
	send(model.C2S, netpkt.ACK, nil, 180*us)

	for _, ex := range spec.exchanges {
		at += 40 * ms
		if ex.pause != 0 {
			at += ex.pause - 40*ms
		}
		segments(model.C2S, ex.req, ex.reqSplit, 120*us, 95*us)
		send(model.S2C, netpkt.ACK, nil, 260*us)
		if ex.resp != nil {
			segments(model.S2C, ex.resp, ex.respSplit, 3800*us, 140*us)
			send(model.C2S, netpkt.ACK, nil, 210*us)
		}
	}

	if !spec.defects.noClose {
		send(model.C2S, netpkt.FIN|netpkt.ACK, nil, 15*ms)
		send(model.S2C, netpkt.FIN|netpkt.ACK, nil, 400*us)
		send(model.C2S, netpkt.ACK, nil, 150*us)
	}

	c.applyDefects(ipID)
	return c
}

// split cuts msg at the given offsets; offsets outside the message are ignored.
func split(msg []byte, points []int) [][]byte {
	cuts := []int{0}
	for _, p := range slices.Sorted(slices.Values(points)) {
		if p > 0 && p < len(msg) {
			cuts = append(cuts, p)
		}
	}
	cuts = append(cuts, len(msg))

	pieces := make([][]byte, 0, len(cuts)-1)
	for i := 0; i+1 < len(cuts); i++ {
		pieces = append(pieces, msg[cuts[i]:cuts[i+1]])
	}
	return pieces
}

func (c *connection) segment(ref segmentRef) *packet { return c.data[dirIndex(ref.dir)][ref.index] }

// applyDefects reproduces the capture problems of the scenario, in the order
// in which they happen on the wire.
func (c *connection) applyDefects(ipID [2]uint16) {
	d := c.spec.defects

	if d.retransmit != nil {
		orig := c.segment(*d.retransmit)
		dup := *orig
		dup.at += 1400 * us
		dup.payload = slices.Clone(orig.payload)
		if d.retransmitConflict {
			dup.payload[len(dup.payload)/2] ^= 0x5a
		}
		dup.ipID = ipID[dirIndex(orig.dir)]
		c.packets = append(c.packets, &dup)
	}

	if d.reorder != nil {
		first := c.segment(*d.reorder)
		second := c.segment(segmentRef{d.reorder.dir, d.reorder.index + 1})
		first.at, second.at = second.at, first.at
	}

	if d.drop != nil {
		c.segment(*d.drop).captured = false
	}

	if d.truncate != nil {
		c.segment(*d.truncate).truncated = true
	}

	if d.dropLeading > 0 {
		byTime := slices.Clone(c.packets)
		slices.SortStableFunc(byTime, func(a, b *packet) int { return int(a.at - b.at) })
		for _, p := range byTime[:d.dropLeading] {
			p.captured = false
		}
	}

	if d.badChecksum != "" {
		for _, p := range c.packets {
			p.badChecksum = p.dir == d.badChecksum
		}
	}
}

// frame renders a packet as an Ethernet frame.
func (c *connection) frame(p *packet) []byte {
	src, dst := c.spec.client, c.spec.server
	if p.dir == model.S2C {
		src, dst = dst, src
	}
	return netpkt.BuildEthernetTCP(src, dst, p.seq, p.ack, p.flags, tcpWindow, p.payload, netpkt.BuildOptions{
		SrcMAC:      hostMAC(src.Addr()),
		DstMAC:      hostMAC(dst.Addr()),
		IPID:        p.ipID,
		BadChecksum: p.badChecksum,
	})
}

// hostMAC derives a locally administered address from a LAN address. Hosts
// outside the LAN are reached through the gateway and share its address.
func hostMAC(ip netip.Addr) [6]byte {
	if !lan.Contains(ip) {
		ip = gatewayIP
	}
	a := ip.As4()
	return [6]byte{0x02, 0x00, a[0], a[1], a[2], a[3]}
}

var (
	lan       = netip.MustParsePrefix("192.168.10.0/24")
	gatewayIP = netip.MustParseAddr("192.168.10.1")
)
