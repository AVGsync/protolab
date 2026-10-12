package netpkt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"slices"
	"testing"

	"github.com/AVGsync/protolab/internal/pcap"
)

var (
	client = netip.MustParseAddrPort("10.0.0.1:50000")
	server = netip.MustParseAddrPort("192.168.1.20:502")
)

func ethFrame(payload []byte, opt BuildOptions) []byte {
	return BuildEthernetTCP(client, server, 1000, 2000, PSH|ACK, 4096, payload, opt)
}

// ipPacket strips the Ethernet header of a built frame.
func ipPacket(payload []byte) []byte { return ethFrame(payload, BuildOptions{})[14:] }

// refreshChecksums recomputes the IPv4 header and TCP checksums of an IPv4 packet
// after a test edited its headers.
func refreshChecksums(ip []byte) {
	hdrLen := int(ip[0]&0x0f) * 4
	total := int(binary.BigEndian.Uint16(ip[2:4]))
	binary.BigEndian.PutUint16(ip[10:12], 0)
	binary.BigEndian.PutUint16(ip[10:12], ^fold(onesSum(ip[:hdrLen], 0)))
	src := netip.AddrFrom4([4]byte(ip[12:16]))
	dst := netip.AddrFrom4([4]byte(ip[16:20]))
	binary.BigEndian.PutUint16(ip[hdrLen+16:], TCPChecksum(src, dst, ip[hdrLen:total]))
}

func decodeOK(t *testing.T, linkType uint16, data []byte, origLen int) Segment {
	t.Helper()
	s, err := Decode(linkType, data, origLen)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	return s
}

func wantCode(t *testing.T, err error, code string) {
	t.Helper()
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("err = %v, want DecodeError %s", err, code)
	}
	if de.Code != code {
		t.Fatalf("code = %s (%s), want %s", de.Code, de.Msg, code)
	}
	if de.Msg == "" {
		t.Errorf("code %s has an empty message", code)
	}
}

func checkSegment(t *testing.T, s Segment, payload []byte) {
	t.Helper()
	if s.Src != client || s.Dst != server || s.Seq != 1000 || s.Ack != 2000 || s.Flags != PSH|ACK || s.Window != 4096 {
		t.Errorf("header fields = %+v", s)
	}
	if !bytes.Equal(s.Payload, payload) || s.PayloadLen != len(payload) || s.Truncated() {
		t.Errorf("payload = %x (len %d), want %x", s.Payload, s.PayloadLen, payload)
	}
	if s.Checksum != ChecksumOK {
		t.Errorf("checksum status = %d", s.Checksum)
	}
}

func TestFlagNames(t *testing.T) {
	got := (FIN | SYN | ACK | PSH | CWR).Names()
	if want := []string{"SYN", "FIN", "PSH", "ACK", "CWR"}; !slices.Equal(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}
	all := TCPFlags(0xff).Names()
	if want := []string{"SYN", "FIN", "RST", "PSH", "ACK", "URG", "ECE", "CWR"}; !slices.Equal(all, want) {
		t.Errorf("Names = %v, want %v", all, want)
	}
	if n := TCPFlags(0).Names(); n == nil || len(n) != 0 {
		t.Errorf("Names of no flags = %#v, want empty non-nil", n)
	}
}

func TestLinkTypes(t *testing.T) {
	payload := []byte("hello, device")
	ip := ipPacket(payload)
	eth := ethFrame(payload, BuildOptions{})

	sll := make([]byte, 16, 16+len(ip))
	binary.BigEndian.PutUint16(sll[0:2], 0) // packet sent to us
	binary.BigEndian.PutUint16(sll[2:4], 1) // ARPHRD_ETHER
	binary.BigEndian.PutUint16(sll[4:6], 6) // address length
	binary.BigEndian.PutUint16(sll[14:16], etherTypeIPv4)
	sll = append(sll, ip...)

	sll2 := make([]byte, 20, 20+len(ip))
	binary.BigEndian.PutUint16(sll2[0:2], etherTypeIPv4)
	binary.BigEndian.PutUint32(sll2[4:8], 3) // interface index
	binary.BigEndian.PutUint16(sll2[8:10], 1)
	sll2 = append(sll2, ip...)

	nullLE := append([]byte{2, 0, 0, 0}, ip...)
	nullBE := append([]byte{0, 0, 0, 2}, ip...)
	loop := append([]byte{0, 0, 0, 2}, ip...)

	cases := []struct {
		name     string
		linkType uint16
		data     []byte
	}{
		{"ethernet", pcap.LinkTypeEthernet, eth},
		{"linux sll", pcap.LinkTypeLinuxSLL, sll},
		{"linux sll2", pcap.LinkTypeLinuxSLL2, sll2},
		{"raw", pcap.LinkTypeRaw, ip},
		{"ipv4", pcap.LinkTypeIPv4, ip},
		{"null little-endian host", pcap.LinkTypeNull, nullLE},
		{"null big-endian host", pcap.LinkTypeNull, nullBE},
		{"loop", pcap.LinkTypeLoop, loop},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkSegment(t, decodeOK(t, tc.linkType, tc.data, len(tc.data)), payload)
		})
	}

	t.Run("unsupported link type", func(t *testing.T) {
		_, err := Decode(105, eth, len(eth)) // IEEE 802.11
		wantCode(t, err, CodeUnsupportedLink)
	})
	t.Run("null ipv6 families", func(t *testing.T) {
		for _, fam := range []byte{10, 24, 28, 30} {
			_, err := Decode(pcap.LinkTypeNull, append([]byte{fam, 0, 0, 0}, ip...), 4+len(ip))
			wantCode(t, err, CodeIPv6)
		}
	})
	t.Run("null other family", func(t *testing.T) {
		_, err := Decode(pcap.LinkTypeLoop, append([]byte{0, 0, 0, 7}, ip...), 4+len(ip))
		wantCode(t, err, CodeNotIP)
	})
	t.Run("raw ipv6", func(t *testing.T) {
		v6 := append([]byte{0x60}, make([]byte, 39)...)
		_, err := Decode(pcap.LinkTypeRaw, v6, len(v6))
		wantCode(t, err, CodeIPv6)
	})
	t.Run("sll non-ip protocol", func(t *testing.T) {
		arp := slices.Clone(sll)
		binary.BigEndian.PutUint16(arp[14:16], etherTypeARP)
		_, err := Decode(pcap.LinkTypeLinuxSLL, arp, len(arp))
		wantCode(t, err, CodeNotIP)
	})
}

func vlanTagged(eth []byte, tags ...uint16) []byte {
	out := slices.Clone(eth[:12])
	for _, tpid := range tags {
		out = binary.BigEndian.AppendUint16(out, tpid)
		out = binary.BigEndian.AppendUint16(out, 0x2064) // priority 1, VLAN 100
	}
	return append(out, eth[12:]...)
}

func TestVLANTags(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	eth := ethFrame(payload, BuildOptions{})
	for name, tags := range map[string][]uint16{
		"802.1Q":        {0x8100},
		"802.1ad QinQ":  {0x88a8, 0x8100},
		"0x9100 QinQ":   {0x9100, 0x8100},
		"three tags":    {0x88a8, 0x8100, 0x8100},
		"ipv6 in vlan":  {0x8100},
		"arp in vlan":   {0x8100},
		"cut in a tag":  {0x8100, 0x8100},
		"cut in a tag2": {0x8100},
	} {
		t.Run(name, func(t *testing.T) {
			f := vlanTagged(eth, tags...)
			switch name {
			case "ipv6 in vlan":
				binary.BigEndian.PutUint16(f[12+4*len(tags):], etherTypeIPv6)
				_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
				wantCode(t, err, CodeIPv6)
			case "arp in vlan":
				binary.BigEndian.PutUint16(f[12+4*len(tags):], etherTypeARP)
				_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
				wantCode(t, err, CodeNotIP)
			case "cut in a tag", "cut in a tag2":
				_, err := Decode(pcap.LinkTypeEthernet, f[:16], len(f))
				wantCode(t, err, CodeTruncatedHeader)
			default:
				checkSegment(t, decodeOK(t, pcap.LinkTypeEthernet, f, len(f)), payload)
			}
		})
	}
}

func TestNonTCPTraffic(t *testing.T) {
	eth := ethFrame([]byte("x"), BuildOptions{})

	t.Run("ipv6", func(t *testing.T) {
		f := slices.Clone(eth)
		binary.BigEndian.PutUint16(f[12:14], etherTypeIPv6)
		_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
		wantCode(t, err, CodeIPv6)
	})
	t.Run("arp", func(t *testing.T) {
		f := slices.Clone(eth[:14])
		binary.BigEndian.PutUint16(f[12:14], etherTypeARP)
		f = append(f, make([]byte, 28)...)
		_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
		wantCode(t, err, CodeNotIP)
	})
	t.Run("802.3 length field", func(t *testing.T) {
		f := slices.Clone(eth)
		binary.BigEndian.PutUint16(f[12:14], 46)
		_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
		wantCode(t, err, CodeNotIP)
	})
	t.Run("udp", func(t *testing.T) {
		f := slices.Clone(eth)
		f[14+9] = protoUDP
		_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
		wantCode(t, err, CodeNotTCP)
	})
	t.Run("icmp", func(t *testing.T) {
		f := slices.Clone(eth)
		f[14+9] = 1
		_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
		wantCode(t, err, CodeNotTCP)
	})
	t.Run("udp fragment is reported as non-tcp", func(t *testing.T) {
		f := slices.Clone(eth)
		f[14+9] = protoUDP
		binary.BigEndian.PutUint16(f[14+6:], 0x2000)
		_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
		wantCode(t, err, CodeNotTCP)
	})
}

func TestFragments(t *testing.T) {
	eth := ethFrame([]byte("fragmented"), BuildOptions{})
	for name, field := range map[string]uint16{
		"first fragment (MF)":        0x2000,
		"middle fragment (MF+off)":   0x2000 | 185,
		"last fragment (offset)":     185,
		"MF with DF also set":        0x6000,
		"offset only, reserved flag": 0x8001,
	} {
		t.Run(name, func(t *testing.T) {
			f := slices.Clone(eth)
			binary.BigEndian.PutUint16(f[14+6:], field)
			_, err := Decode(pcap.LinkTypeEthernet, f, len(f))
			wantCode(t, err, CodeFragment)
		})
	}
	t.Run("DF alone is not a fragment", func(t *testing.T) {
		decodeOK(t, pcap.LinkTypeEthernet, eth, len(eth))
	})
}

func TestEthernetPaddingIsNotPayload(t *testing.T) {
	for _, payload := range [][]byte{nil, {1}, {1, 2, 3, 4, 5, 6}} {
		f := ethFrame(payload, BuildOptions{})
		padded := append(slices.Clone(f), make([]byte, 60-len(f))...)
		// Some captures also keep the 4-byte FCS.
		withFCS := append(slices.Clone(padded), 0xaa, 0xbb, 0xcc, 0xdd)
		for _, frame := range [][]byte{padded, withFCS} {
			s := decodeOK(t, pcap.LinkTypeEthernet, frame, len(frame))
			if s.PayloadLen != len(payload) || len(s.Payload) != len(payload) || s.Checksum != ChecksumOK {
				t.Errorf("payload %d bytes in a %d-byte frame: got %d/%d, checksum %d",
					len(payload), len(frame), len(s.Payload), s.PayloadLen, s.Checksum)
			}
		}
	}
}

func TestSnaplenTruncation(t *testing.T) {
	payload := bytes.Repeat([]byte{0x5a}, 200)
	f := ethFrame(payload, BuildOptions{})

	t.Run("payload cut", func(t *testing.T) {
		s := decodeOK(t, pcap.LinkTypeEthernet, f[:54+30], len(f))
		if len(s.Payload) != 30 || s.PayloadLen != 200 || !s.Truncated() || s.Checksum != ChecksumUnknown {
			t.Errorf("got %d/%d truncated=%v checksum=%d", len(s.Payload), s.PayloadLen, s.Truncated(), s.Checksum)
		}
	})
	t.Run("whole payload cut", func(t *testing.T) {
		s := decodeOK(t, pcap.LinkTypeEthernet, f[:54], len(f))
		if len(s.Payload) != 0 || s.PayloadLen != 200 || s.Checksum != ChecksumUnknown {
			t.Errorf("got %d/%d checksum=%d", len(s.Payload), s.PayloadLen, s.Checksum)
		}
	})
	for name, n := range map[string]int{"in tcp header": 14 + 20 + 12, "in ip header": 14 + 10, "in ethernet header": 8} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(pcap.LinkTypeEthernet, f[:n], len(f))
			wantCode(t, err, CodeTruncatedHeader)
		})
	}
	t.Run("short frame that was not truncated", func(t *testing.T) {
		_, err := Decode(pcap.LinkTypeEthernet, f[:14+20+12], 14+20+12)
		wantCode(t, err, CodeMalformed)
	})
	t.Run("tcp options not captured", func(t *testing.T) {
		ip := withTCPOptions(ipPacket(payload), 12)
		_, err := Decode(pcap.LinkTypeRaw, ip[:20+24], len(ip))
		wantCode(t, err, CodeTruncatedHeader)
	})
}

// withTCPOptions inserts n bytes of NOP options after the fixed TCP header.
func withTCPOptions(ip []byte, n int) []byte {
	out := slices.Clone(ip[:40])
	out = append(out, bytes.Repeat([]byte{1}, n)...) // NOP
	out = append(out, ip[40:]...)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(out)))
	out[20+12] = byte((20+n)/4) << 4
	refreshChecksums(out)
	return out
}

func TestHeaderOptions(t *testing.T) {
	payload := []byte("after options")

	t.Run("tcp options", func(t *testing.T) {
		ip := withTCPOptions(ipPacket(payload), 12)
		checkSegment(t, decodeOK(t, pcap.LinkTypeRaw, ip, len(ip)), payload)
	})
	t.Run("ip options", func(t *testing.T) {
		ip := ipPacket(payload)
		out := slices.Clone(ip[:20])
		out = append(out, 1, 1, 1, 0) // NOP NOP NOP EOL
		out = append(out, ip[20:]...)
		out[0] = 0x46
		binary.BigEndian.PutUint16(out[2:4], uint16(len(out)))
		refreshChecksums(out)
		checkSegment(t, decodeOK(t, pcap.LinkTypeRaw, out, len(out)), payload)
	})
}

func TestMalformedHeaders(t *testing.T) {
	payload := []byte("0123456789")
	edit := func(fn func(ip []byte)) []byte {
		ip := ipPacket(payload)
		fn(ip)
		return ip
	}
	cases := map[string][]byte{
		"ip version 5": edit(func(ip []byte) { ip[0] = 0x55 }),
		"ihl below 5":  edit(func(ip []byte) { ip[0] = 0x44 }),
		"total length below header": edit(func(ip []byte) {
			binary.BigEndian.PutUint16(ip[2:4], 16)
		}),
		"total length beyond frame": edit(func(ip []byte) {
			binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)+20))
		}),
		"no room for tcp header": edit(func(ip []byte) {
			binary.BigEndian.PutUint16(ip[2:4], 30)
		}),
		"tcp data offset below 5": edit(func(ip []byte) { ip[32] = 4 << 4 }),
		"tcp data offset beyond segment": edit(func(ip []byte) {
			ip[32] = 15 << 4
		}),
	}
	for name, ip := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(pcap.LinkTypeRaw, ip, len(ip))
			wantCode(t, err, CodeMalformed)
		})
	}
	t.Run("raw with unknown ip version", func(t *testing.T) {
		_, err := Decode(pcap.LinkTypeRaw, []byte{0x10, 0, 0, 0}, 4)
		wantCode(t, err, CodeMalformed)
	})
	t.Run("empty raw frame", func(t *testing.T) {
		_, err := Decode(pcap.LinkTypeRaw, nil, 0)
		wantCode(t, err, CodeMalformed)
	})
}

func TestSegmentationOffloadZeroLength(t *testing.T) {
	payload := bytes.Repeat([]byte{7}, 3000)
	ip := ipPacket(payload[:100])
	// A 3000-byte payload does not fit the builder's checks for this test, so build it by hand.
	big := append(slices.Clone(ip[:40]), payload...)
	binary.BigEndian.PutUint16(big[2:4], 0)
	s := decodeOK(t, pcap.LinkTypeRaw, big, len(big))
	if s.PayloadLen != 3000 || len(s.Payload) != 3000 {
		t.Errorf("payload %d/%d, want 3000", len(s.Payload), s.PayloadLen)
	}
}

func TestChecksums(t *testing.T) {
	payload := []byte("checksum me")

	t.Run("bad checksum", func(t *testing.T) {
		f := ethFrame(payload, BuildOptions{BadChecksum: true})
		s := decodeOK(t, pcap.LinkTypeEthernet, f, len(f))
		if s.Checksum != ChecksumBad || !bytes.Equal(s.Payload, payload) {
			t.Errorf("checksum = %d, payload %q", s.Checksum, s.Payload)
		}
	})
	t.Run("payload corruption is detected", func(t *testing.T) {
		f := ethFrame(payload, BuildOptions{})
		f[len(f)-1] ^= 0x01
		if s := decodeOK(t, pcap.LinkTypeEthernet, f, len(f)); s.Checksum != ChecksumBad {
			t.Errorf("checksum = %d", s.Checksum)
		}
	})
	t.Run("checksum field is ignored by TCPChecksum", func(t *testing.T) {
		seg := ipPacket(payload)[20:]
		want := TCPChecksum(client.Addr(), server.Addr(), seg)
		seg[16], seg[17] = 0x12, 0x34
		if got := TCPChecksum(client.Addr(), server.Addr(), seg); got != want {
			t.Errorf("TCPChecksum depends on the checksum field: %04x vs %04x", got, want)
		}
	})
	t.Run("odd length", func(t *testing.T) {
		f := ethFrame([]byte("odd"), BuildOptions{})
		if s := decodeOK(t, pcap.LinkTypeEthernet, f, len(f)); s.Checksum != ChecksumOK {
			t.Errorf("checksum = %d", s.Checksum)
		}
	})
	t.Run("negative zero", func(t *testing.T) {
		// Choose two payload bytes so that the correct checksum is 0x0000; then
		// 0xffff must verify too, and BadChecksum must still produce a bad one.
		ip := ipPacket([]byte{0, 0})
		c := TCPChecksum(client.Addr(), server.Addr(), ip[20:])
		binary.BigEndian.PutUint16(ip[40:42], c)
		if got := TCPChecksum(client.Addr(), server.Addr(), ip[20:]); got != 0 {
			t.Fatalf("constructed checksum = %04x, want 0", got)
		}
		for _, field := range []uint16{0x0000, 0xffff} {
			binary.BigEndian.PutUint16(ip[36:38], field)
			if s := decodeOK(t, pcap.LinkTypeRaw, ip, len(ip)); s.Checksum != ChecksumOK {
				t.Errorf("checksum field %04x: status %d", field, s.Checksum)
			}
		}
		binary.BigEndian.PutUint16(ip[36:38], badChecksum(0))
		if s := decodeOK(t, pcap.LinkTypeRaw, ip, len(ip)); s.Checksum != ChecksumBad {
			t.Errorf("badChecksum(0) verifies")
		}
	})
}

func TestBuildEthernetTCP(t *testing.T) {
	opt := BuildOptions{
		SrcMAC: [6]byte{2, 0, 0, 0, 0, 1},
		DstMAC: [6]byte{2, 0, 0, 0, 0, 2},
		IPID:   0x1234,
	}
	f := ethFrame([]byte("abc"), opt)
	if len(f) != 14+20+20+3 {
		t.Fatalf("frame length %d", len(f))
	}
	if !bytes.Equal(f[0:6], opt.DstMAC[:]) || !bytes.Equal(f[6:12], opt.SrcMAC[:]) {
		t.Errorf("MAC addresses = % x", f[0:12])
	}
	ip := f[14:34]
	if fold(onesSum(ip, 0)) != 0xffff {
		t.Error("IPv4 header checksum does not verify")
	}
	if ip[8] != 64 || binary.BigEndian.Uint16(ip[6:8]) != 0x4000 || binary.BigEndian.Uint16(ip[4:6]) != 0x1234 {
		t.Errorf("ttl=%d flags=%04x id=%04x", ip[8], binary.BigEndian.Uint16(ip[6:8]), binary.BigEndian.Uint16(ip[4:6]))
	}
	opt.TTL = 128
	if f := ethFrame(nil, opt); f[14+8] != 128 {
		t.Errorf("ttl = %d", f[14+8])
	}
}
