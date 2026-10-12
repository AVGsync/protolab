package demo

import (
	"encoding/binary"
	"net/netip"
	"strings"
)

// Background traffic of the workstation that is not TCP/IPv4. The importer
// skips such frames and counts them in the capture diagnostics: ARP as
// "non_ip", UDP as "non_tcp", IPv6 as "ipv6_unsupported".

var (
	be = binary.BigEndian

	dnsServer = netip.AddrPortFrom(gatewayIP, 53)
	mdnsIPv4  = netip.AddrPortFrom(netip.MustParseAddr("224.0.0.251"), 5353)
	mdnsIPv6  = netip.AddrPortFrom(netip.MustParseAddr("ff02::fb"), 5353)
)

const (
	etherTypeIPv4 = 0x0800
	etherTypeARP  = 0x0806
	etherTypeIPv6 = 0x86dd

	protoUDP = 17

	arpRequest = 1
	arpReply   = 2

	dnsTypeA   = 1
	dnsTypePTR = 12
	dnsClassIN = 1
)

// day1NonTCP: the client resolves the controller's MAC address before the
// first session, looks up the name of the HTTPS server of "tls-noise" and
// announces a service query over mDNS on both IP versions.
func day1NonTCP() []nonTCPSpec {
	const lookupID = 0x3a7c
	resolver := client(53514)
	return []nonTCPSpec{
		{at: 4900 * ms, frame: arpFrame(arpRequest, clientIP, device.Addr())},
		{at: 4900*ms + 350*us, frame: arpFrame(arpReply, device.Addr(), clientIP)},
		{at: 199_980 * ms, frame: udp4Frame(resolver, dnsServer, 0x1c21, 64, dnsQuery(lookupID, "example.com", dnsTypeA))},
		{at: 199_980*ms + 11_200*us, frame: udp4Frame(dnsServer, resolver, 0x7d03, 64, dnsAnswer(lookupID, "example.com", website.Addr()))},
		{at: 330 * second, frame: udp4Frame(client(5353), mdnsIPv4, 0x1c22, 255, dnsQuery(0, "_tc-config._tcp.local", dnsTypePTR))},
		{at: 330*second + 150*us, frame: udp6Frame(clientLinkLocal(), mdnsIPv6, dnsQuery(0, "_tc-config._tcp.local", dnsTypePTR))},
	}
}

// ethernet frames a payload, padding it to the 60-byte minimum (without FCS).
func ethernet(dst, src [6]byte, etherType uint16, payload []byte) []byte {
	b := make([]byte, 0, max(60, 14+len(payload)))
	b = append(b, dst[:]...)
	b = append(b, src[:]...)
	b = be.AppendUint16(b, etherType)
	b = append(b, payload...)
	for len(b) < 60 {
		b = append(b, 0)
	}
	return b
}

// arpFrame is an ARP request (broadcast) or reply (unicast) from sender about target.
func arpFrame(op uint16, sender, target netip.Addr) []byte {
	senderMAC, targetMAC := hostMAC(sender), hostMAC(target)
	dst := targetMAC
	if op == arpRequest {
		dst = [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		targetMAC = [6]byte{}
	}
	spa, tpa := sender.As4(), target.As4()
	b := be.AppendUint16(nil, 1) // Ethernet
	b = be.AppendUint16(b, etherTypeIPv4)
	b = append(b, 6, 4)
	b = be.AppendUint16(b, op)
	b = append(b, senderMAC[:]...)
	b = append(b, spa[:]...)
	b = append(b, targetMAC[:]...)
	b = append(b, tpa[:]...)
	return ethernet(dst, senderMAC, etherTypeARP, b)
}

// macOf returns the destination MAC address of an IPv4 address or an IPv6
// group: the multicast mapping for groups, the host (or gateway) otherwise.
func macOf(ip netip.Addr) [6]byte {
	switch {
	case ip.Is4() && ip.IsMulticast():
		a := ip.As4()
		return [6]byte{0x01, 0x00, 0x5e, a[1] & 0x7f, a[2], a[3]}
	case ip.Is6():
		a := ip.As16()
		return [6]byte{0x33, 0x33, a[12], a[13], a[14], a[15]}
	}
	return hostMAC(ip)
}

func udp4Frame(src, dst netip.AddrPort, ipID uint16, ttl uint8, payload []byte) []byte {
	udp := udpDatagram(src, dst, payload)
	ip := make([]byte, 20, 20+len(udp))
	ip[0] = 4<<4 | 5
	be.PutUint16(ip[2:4], uint16(20+len(udp)))
	be.PutUint16(ip[4:6], ipID)
	ip[8] = ttl
	ip[9] = protoUDP
	s, d := src.Addr().As4(), dst.Addr().As4()
	copy(ip[12:16], s[:])
	copy(ip[16:20], d[:])
	be.PutUint16(ip[10:12], ^fold(onesSum(ip, 0)))
	return ethernet(macOf(dst.Addr()), hostMAC(src.Addr()), etherTypeIPv4, append(ip, udp...))
}

func udp6Frame(src, dst netip.AddrPort, payload []byte) []byte {
	udp := udpDatagram(src, dst, payload)
	ip := make([]byte, 40, 40+len(udp))
	ip[0] = 6 << 4
	be.PutUint16(ip[4:6], uint16(len(udp)))
	ip[6] = protoUDP
	ip[7] = 255 // hop limit required by mDNS
	s, d := src.Addr().As16(), dst.Addr().As16()
	copy(ip[8:24], s[:])
	copy(ip[24:40], d[:])
	return ethernet(macOf(dst.Addr()), hostMAC(clientIP), etherTypeIPv6, append(ip, udp...))
}

// udpDatagram builds a UDP header and payload with the checksum over the
// pseudo-header of the address family of src.
func udpDatagram(src, dst netip.AddrPort, payload []byte) []byte {
	n := 8 + len(payload)
	b := make([]byte, 8, n)
	be.PutUint16(b[0:2], src.Port())
	be.PutUint16(b[2:4], dst.Port())
	be.PutUint16(b[4:6], uint16(n))
	b = append(b, payload...)

	pseudo := src.Addr().AsSlice()
	pseudo = append(pseudo, dst.Addr().AsSlice()...)
	if src.Addr().Is4() {
		pseudo = append(pseudo, 0, protoUDP)
		pseudo = be.AppendUint16(pseudo, uint16(n))
	} else {
		pseudo = be.AppendUint32(pseudo, uint32(n))
		pseudo = append(pseudo, 0, 0, 0, protoUDP)
	}
	sum := ^fold(onesSum(b, onesSum(pseudo, 0)))
	if sum == 0 {
		sum = 0xffff // zero means "no checksum"
	}
	be.PutUint16(b[6:8], sum)
	return b
}

// clientLinkLocal is the IPv6 link-local address of the client, formed from
// its MAC address (modified EUI-64).
func clientLinkLocal() netip.AddrPort {
	mac := hostMAC(clientIP)
	a := [16]byte{0: 0xfe, 1: 0x80}
	copy(a[8:], []byte{mac[0] ^ 0x02, mac[1], mac[2], 0xff, 0xfe, mac[3], mac[4], mac[5]})
	return netip.AddrPortFrom(netip.AddrFrom16(a), 5353)
}

func dnsHeader(id, flags uint16, questions, answers int) []byte {
	b := be.AppendUint16(nil, id)
	b = be.AppendUint16(b, flags)
	b = be.AppendUint16(b, uint16(questions))
	b = be.AppendUint16(b, uint16(answers))
	return be.AppendUint32(b, 0) // no authority and additional records
}

func dnsQuestion(b []byte, name string, qtype uint16) []byte {
	for label := range strings.SplitSeq(name, ".") {
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0)
	b = be.AppendUint16(b, qtype)
	return be.AppendUint16(b, dnsClassIN)
}

// dnsQuery asks one question; a non-zero id is a recursive unicast query.
func dnsQuery(id uint16, name string, qtype uint16) []byte {
	var flags uint16
	if id != 0 {
		flags = 0x0100 // recursion desired
	}
	return dnsQuestion(dnsHeader(id, flags, 1, 0), name, qtype)
}

// dnsAnswer answers an A query with one address.
func dnsAnswer(id uint16, name string, addr netip.Addr) []byte {
	b := dnsQuestion(dnsHeader(id, 0x8180, 1, 1), name, dnsTypeA)
	b = append(b, 0xc0, 0x0c) // the name of the question
	b = be.AppendUint16(b, dnsTypeA)
	b = be.AppendUint16(b, dnsClassIN)
	b = be.AppendUint32(b, 3600) // TTL
	a := addr.As4()
	b = be.AppendUint16(b, uint16(len(a)))
	return append(b, a[:]...)
}

// onesSum adds b as big-endian 16-bit words to sum (RFC 1071).
func onesSum(b []byte, sum uint32) uint32 {
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(be.Uint16(b[i:]))
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	return sum
}

func fold(sum uint32) uint16 {
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return uint16(sum)
}
