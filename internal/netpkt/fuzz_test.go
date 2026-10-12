package netpkt

import (
	"testing"

	"github.com/AVGsync/protolab/internal/pcap"
)

func FuzzDecode(f *testing.F) {
	frame := ethFrame([]byte("seed payload"), BuildOptions{})
	f.Add(uint16(pcap.LinkTypeEthernet), frame, len(frame))
	f.Add(uint16(pcap.LinkTypeEthernet), vlanTagged(frame, 0x88a8, 0x8100), len(frame)+8)
	f.Add(uint16(pcap.LinkTypeRaw), frame[14:], len(frame)-14)
	f.Add(uint16(pcap.LinkTypeNull), append([]byte{2, 0, 0, 0}, frame[14:]...), len(frame)-10)
	f.Add(uint16(pcap.LinkTypeLinuxSLL2), append(make([]byte, 20), frame[14:]...), len(frame)+6)
	f.Add(uint16(pcap.LinkTypeEthernet), frame[:40], len(frame))
	f.Fuzz(func(t *testing.T, linkType uint16, data []byte, origLen int) {
		s, err := Decode(linkType, data, origLen)
		if err != nil {
			if _, ok := err.(*DecodeError); !ok {
				t.Fatalf("error of type %T", err)
			}
			return
		}
		if len(s.Payload) > s.PayloadLen || s.PayloadLen < 0 {
			t.Fatalf("payload %d of %d", len(s.Payload), s.PayloadLen)
		}
		if s.Truncated() && s.Checksum != ChecksumUnknown {
			t.Fatalf("checksum verified on a truncated segment")
		}
	})
}
