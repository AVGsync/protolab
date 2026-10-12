package netpkt

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

func buildEthernetTCP(src, dst netip.AddrPort, seq, ack uint32, flags TCPFlags, window uint16, payload []byte, opt BuildOptions) []byte {
	const ethLen, ipLen, tcpLen = 14, 20, 20
	total := ipLen + tcpLen + len(payload)
	if total > 0xffff {
		panic(fmt.Sprintf("netpkt: %d-byte payload does not fit in an IPv4 packet", len(payload)))
	}
	srcIP, dstIP := mustIPv4(src.Addr()), mustIPv4(dst.Addr())
	ttl := opt.TTL
	if ttl == 0 {
		ttl = 64
	}
	be := binary.BigEndian

	b := make([]byte, ethLen+total)
	copy(b[0:6], opt.DstMAC[:])
	copy(b[6:12], opt.SrcMAC[:])
	be.PutUint16(b[12:14], etherTypeIPv4)

	ip := b[ethLen : ethLen+ipLen]
	ip[0] = 4<<4 | ipLen/4
	be.PutUint16(ip[2:4], uint16(total))
	be.PutUint16(ip[4:6], opt.IPID)
	be.PutUint16(ip[6:8], 0x4000) // don't fragment
	ip[8] = ttl
	ip[9] = protoTCP
	copy(ip[12:16], srcIP[:])
	copy(ip[16:20], dstIP[:])
	be.PutUint16(ip[10:12], ^fold(onesSum(ip, 0)))

	tcp := b[ethLen+ipLen:]
	be.PutUint16(tcp[0:2], src.Port())
	be.PutUint16(tcp[2:4], dst.Port())
	be.PutUint32(tcp[4:8], seq)
	be.PutUint32(tcp[8:12], ack)
	tcp[12] = tcpLen / 4 << 4
	tcp[13] = byte(flags)
	be.PutUint16(tcp[14:16], window)
	copy(tcp[tcpLen:], payload)

	sum := TCPChecksum(src.Addr(), dst.Addr(), tcp)
	if opt.BadChecksum {
		sum = badChecksum(sum)
	}
	be.PutUint16(tcp[16:18], sum)
	return b
}

func mustIPv4(a netip.Addr) [4]byte {
	a = a.Unmap()
	if !a.Is4() {
		panic(fmt.Sprintf("netpkt: IPv4 address expected, got %v", a))
	}
	return a.As4()
}
