// Package netpkt decodes and builds link-layer/IPv4/TCP frames.
package netpkt

import "net/netip"

// TCPFlags is the TCP flags byte.
type TCPFlags uint8

const (
	FIN TCPFlags = 1 << iota
	SYN
	RST
	PSH
	ACK
	URG
	ECE
	CWR
)

// Has reports whether all bits of x are set.
func (f TCPFlags) Has(x TCPFlags) bool { return f&x == x }

// Names lists set flags in the order SYN, FIN, RST, PSH, ACK, URG, ECE, CWR.
func (f TCPFlags) Names() []string {
	names := make([]string, 0, len(flagNames))
	for _, fn := range flagNames {
		if f.Has(fn.flag) {
			names = append(names, fn.name)
		}
	}
	return names
}

// ChecksumStatus of the TCP checksum.
type ChecksumStatus uint8

const (
	ChecksumOK      ChecksumStatus = iota
	ChecksumBad                    // mismatch (often checksum offloading on the capturing host)
	ChecksumUnknown                // cannot verify: segment truncated by snaplen
)

// Segment is a decoded TCP/IPv4 segment.
type Segment struct {
	Src, Dst netip.AddrPort
	Seq, Ack uint32
	Flags    TCPFlags
	Window   uint16
	// Payload holds the captured payload bytes; it may be shorter than PayloadLen
	// when the frame was truncated by the capture snaplen.
	Payload    []byte
	PayloadLen int // from the IPv4 total length
	Checksum   ChecksumStatus
}

// Truncated reports whether part of the payload was not captured.
func (s *Segment) Truncated() bool { return len(s.Payload) < s.PayloadLen }

// Decode error codes (DecodeError.Code), used as diagnostic codes.
const (
	CodeUnsupportedLink = "unsupported_linktype"
	CodeNotIP           = "non_ip" // ARP, LLDP, ...
	CodeIPv6            = "ipv6_unsupported"
	CodeFragment        = "ip_fragment" // IPv4 fragment (no reassembly)
	CodeNotTCP          = "non_tcp"     // UDP, ICMP, ...
	CodeMalformed       = "malformed"   // inconsistent headers
	CodeTruncatedHeader = "truncated_header"
)

// DecodeError explains why a frame is not a usable TCP/IPv4 segment.
type DecodeError struct {
	Code string
	Msg  string
}

func (e *DecodeError) Error() string { return e.Code + ": " + e.Msg }

// Decode parses a frame of the given pcap link type. origLen is the on-wire length.
// Ethernet II with any number of 802.1Q/802.1ad tags, Linux SLL/SLL2, raw IPv4 and
// BSD loopback are supported. Payload aliases data.
func Decode(linkType uint16, data []byte, origLen int) (Segment, error) {
	f := frame{data: data, wireLen: max(origLen, len(data))}
	return f.decodeLink(linkType)
}

// BuildOptions tweak BuildEthernetTCP.
type BuildOptions struct {
	SrcMAC, DstMAC [6]byte
	IPID           uint16
	TTL            uint8 // 64 if zero
	BadChecksum    bool  // write a wrong TCP checksum (simulates checksum offloading)
}

// BuildEthernetTCP returns an Ethernet II + IPv4 + TCP frame (no TCP options) with valid
// IPv4 and TCP checksums unless opt.BadChecksum.
func BuildEthernetTCP(src, dst netip.AddrPort, seq, ack uint32, flags TCPFlags, window uint16, payload []byte, opt BuildOptions) []byte {
	return buildEthernetTCP(src, dst, seq, ack, flags, window, payload, opt)
}

// TCPChecksum computes the TCP checksum over the IPv4 pseudo-header and the TCP
// segment (header with checksum field zeroed + payload).
func TCPChecksum(src, dst netip.Addr, segment []byte) uint16 {
	sum := pseudoHeaderSum(src, dst, len(segment))
	if len(segment) >= 18 {
		sum = onesSum(segment[:16], sum)
		sum = onesSum(segment[18:], sum)
	} else {
		sum = onesSum(segment, sum)
	}
	return ^fold(sum)
}
