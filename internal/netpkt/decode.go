package netpkt

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/AVGsync/protolab/internal/pcap"
)

var flagNames = [...]struct {
	flag TCPFlags
	name string
}{
	{SYN, "SYN"}, {FIN, "FIN"}, {RST, "RST"}, {PSH, "PSH"},
	{ACK, "ACK"}, {URG, "URG"}, {ECE, "ECE"}, {CWR, "CWR"},
}

const (
	etherTypeIPv4 = 0x0800
	etherTypeIPv6 = 0x86dd
	etherTypeARP  = 0x0806
	etherTypeLLDP = 0x88cc
	// Values below 0x0600 are IEEE 802.3 length fields, not EtherTypes.
	minEtherType = 0x0600

	protoTCP = 6
	protoUDP = 17
)

// isVLANTag reports 802.1Q, 802.1ad and the pre-standard 0x9100 QinQ tag.
func isVLANTag(etherType uint16) bool {
	return etherType == 0x8100 || etherType == 0x88a8 || etherType == 0x9100
}

// frame is a captured frame; wireLen is its length on the wire (>= len(data)).
type frame struct {
	data    []byte
	wireLen int
}

func (f frame) truncated() bool { return len(f.data) < f.wireLen }

func decodeErr(code, format string, args ...any) error {
	return &DecodeError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// short reports a header that does not fit in the captured bytes: a snaplen cut
// when the frame was truncated, otherwise a frame too short to be valid.
func (f frame) short(header string, need, have int) error {
	if f.truncated() {
		return decodeErr(CodeTruncatedHeader, "заголовок %s захвачен не полностью (%d из %d Б)", header, have, need)
	}
	return decodeErr(CodeMalformed, "кадр слишком короткий для заголовка %s (%d из %d Б)", header, have, need)
}

func (f frame) decodeLink(linkType uint16) (Segment, error) {
	switch linkType {
	case pcap.LinkTypeEthernet:
		if len(f.data) < 14 {
			return Segment{}, f.short("Ethernet", 14, len(f.data))
		}
		return f.etherPayload(be16(f.data[12:14]), 14)
	case pcap.LinkTypeLinuxSLL:
		// 16-byte pseudo-header: packet type, ARPHRD type, address length, address(8), protocol.
		if len(f.data) < 16 {
			return Segment{}, f.short("Linux SLL", 16, len(f.data))
		}
		return f.etherPayload(be16(f.data[14:16]), 16)
	case pcap.LinkTypeLinuxSLL2:
		// 20-byte pseudo-header starting with the protocol.
		if len(f.data) < 20 {
			return Segment{}, f.short("Linux SLL2", 20, len(f.data))
		}
		return f.etherPayload(be16(f.data[0:2]), 20)
	case pcap.LinkTypeRaw, pcap.LinkTypeIPv4:
		return f.rawIP()
	case pcap.LinkTypeNull:
		return f.loopback(false)
	case pcap.LinkTypeLoop:
		return f.loopback(true)
	}
	return Segment{}, decodeErr(CodeUnsupportedLink, "тип канального уровня %s не поддерживается", pcap.LinkTypeName(linkType))
}

// etherPayload dispatches on an EtherType whose payload starts at off, skipping
// any number of VLAN tags (4 bytes each: tag control information, inner EtherType).
func (f frame) etherPayload(etherType uint16, off int) (Segment, error) {
	for isVLANTag(etherType) {
		if len(f.data) < off+4 {
			return Segment{}, f.short("802.1Q", off+4, len(f.data))
		}
		etherType = be16(f.data[off+2 : off+4])
		off += 4
	}
	switch etherType {
	case etherTypeIPv4:
		return f.ipv4(off)
	case etherTypeIPv6:
		return Segment{}, decodeErr(CodeIPv6, "пакет IPv6")
	case etherTypeARP:
		return Segment{}, decodeErr(CodeNotIP, "кадр ARP")
	case etherTypeLLDP:
		return Segment{}, decodeErr(CodeNotIP, "кадр LLDP")
	}
	if etherType < minEtherType {
		return Segment{}, decodeErr(CodeNotIP, "кадр IEEE 802.3 (поле длины %d)", etherType)
	}
	return Segment{}, decodeErr(CodeNotIP, "EtherType 0x%04X не IPv4", etherType)
}

func (f frame) rawIP() (Segment, error) {
	if len(f.data) == 0 {
		return Segment{}, f.short("IP", 1, 0)
	}
	switch v := f.data[0] >> 4; v {
	case 4:
		return f.ipv4(0)
	case 6:
		return Segment{}, decodeErr(CodeIPv6, "пакет IPv6")
	default:
		return Segment{}, decodeErr(CodeMalformed, "неизвестная версия IP %d", v)
	}
}

// loopback decodes BSD loopback encapsulation: a 4-byte address family in the
// capturing host's byte order (NULL) or in network byte order (LOOP).
func (f frame) loopback(networkOrder bool) (Segment, error) {
	if len(f.data) < 4 {
		return Segment{}, f.short("loopback", 4, len(f.data))
	}
	family := binary.BigEndian.Uint32(f.data[0:4])
	if !networkOrder {
		// Families are small numbers, so the right byte order gives the smaller value.
		family = min(family, binary.LittleEndian.Uint32(f.data[0:4]))
	}
	switch family {
	case 2: // AF_INET everywhere
		return f.ipv4(4)
	case 10, 24, 28, 30: // AF_INET6 on Linux, OpenBSD/NetBSD, FreeBSD, macOS
		return Segment{}, decodeErr(CodeIPv6, "пакет IPv6")
	}
	return Segment{}, decodeErr(CodeNotIP, "семейство адресов %d не IPv4", family)
}

func (f frame) ipv4(off int) (Segment, error) {
	ip := f.data[off:]
	wire := f.wireLen - off
	if len(ip) < 20 {
		return Segment{}, f.short("IPv4", 20, len(ip))
	}
	if v := ip[0] >> 4; v != 4 {
		return Segment{}, decodeErr(CodeMalformed, "версия IP %d в заголовке IPv4", v)
	}
	hdrLen := int(ip[0]&0x0f) * 4
	if hdrLen < 20 {
		return Segment{}, decodeErr(CodeMalformed, "длина заголовка IPv4 %d Б меньше 20", hdrLen)
	}
	if len(ip) < hdrLen {
		return Segment{}, f.short("IPv4", hdrLen, len(ip))
	}

	total := int(be16(ip[2:4]))
	if total == 0 {
		// Segmentation offload: the capturing host recorded the large segment
		// before the NIC split it, with the length field left at zero. Like
		// Wireshark, take the length from the frame.
		total = wire
	}
	if total < hdrLen {
		return Segment{}, decodeErr(CodeMalformed, "общая длина IPv4 %d Б меньше заголовка (%d Б)", total, hdrLen)
	}
	if total > wire {
		return Segment{}, decodeErr(CodeMalformed, "общая длина IPv4 %d Б больше кадра (%d Б)", total, wire)
	}

	switch proto := ip[9]; proto {
	case protoTCP:
	case protoUDP:
		return Segment{}, decodeErr(CodeNotTCP, "пакет UDP")
	case 1:
		return Segment{}, decodeErr(CodeNotTCP, "пакет ICMP")
	default:
		return Segment{}, decodeErr(CodeNotTCP, "IP-протокол %d не TCP", proto)
	}
	// MF flag or a non-zero fragment offset.
	if frag := be16(ip[6:8]); frag&0x3fff != 0 {
		return Segment{}, decodeErr(CodeFragment, "фрагмент IPv4 (смещение %d Б, MF=%d)", int(frag&0x1fff)*8, frag>>13&1)
	}

	src := netip.AddrFrom4([4]byte(ip[12:16]))
	dst := netip.AddrFrom4([4]byte(ip[16:20]))
	// The total length, not the frame length, bounds the segment: bytes after it
	// are link-layer padding (60-byte Ethernet minimum) or an FCS.
	captured := ip[hdrLen:min(len(ip), total)]
	return f.tcp(src, dst, captured, total-hdrLen)
}

// tcp decodes a TCP segment of the given logical length, of which seg is the captured part.
func (f frame) tcp(src, dst netip.Addr, seg []byte, length int) (Segment, error) {
	if length < 20 {
		return Segment{}, decodeErr(CodeMalformed, "в пакете IPv4 %d Б, меньше заголовка TCP", length)
	}
	if len(seg) < 20 {
		return Segment{}, f.short("TCP", 20, len(seg))
	}
	hdrLen := int(seg[12]>>4) * 4
	if hdrLen < 20 {
		return Segment{}, decodeErr(CodeMalformed, "длина заголовка TCP %d Б меньше 20", hdrLen)
	}
	if hdrLen > length {
		return Segment{}, decodeErr(CodeMalformed, "заголовок TCP (%d Б) длиннее сегмента (%d Б)", hdrLen, length)
	}
	if len(seg) < hdrLen {
		return Segment{}, f.short("TCP", hdrLen, len(seg))
	}

	s := Segment{
		Src:        netip.AddrPortFrom(src, be16(seg[0:2])),
		Dst:        netip.AddrPortFrom(dst, be16(seg[2:4])),
		Seq:        binary.BigEndian.Uint32(seg[4:8]),
		Ack:        binary.BigEndian.Uint32(seg[8:12]),
		Flags:      TCPFlags(seg[13]),
		Window:     be16(seg[14:16]),
		Payload:    seg[hdrLen:len(seg):len(seg)],
		PayloadLen: length - hdrLen,
		Checksum:   ChecksumUnknown,
	}
	if len(seg) == length {
		s.Checksum = ChecksumBad
		if checksumValid(src, dst, seg) {
			s.Checksum = ChecksumOK
		}
	}
	return s, nil
}

func be16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
