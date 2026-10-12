package pcap

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"testing"
	"time"
)

// byteOrder is what binary.LittleEndian and binary.BigEndian both provide.
type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

func readAll(t *testing.T, file []byte) (*Reader, []Packet, error) {
	t.Helper()
	r, err := NewReader(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	var pkts []Packet
	for {
		p, err := r.Next()
		if errors.Is(err, io.EOF) {
			return r, pkts, nil
		}
		if err != nil {
			return r, pkts, err
		}
		pkts = append(pkts, p)
	}
}

// classicFile builds a classic pcap file in the given byte order.
func classicFile(order byteOrder, magic uint32, network uint32, snap uint32, records ...[]byte) []byte {
	var b []byte
	b = order.AppendUint32(b, magic)
	b = order.AppendUint16(b, 2)
	b = order.AppendUint16(b, 4)
	b = order.AppendUint32(b, 0)
	b = order.AppendUint32(b, 0)
	b = order.AppendUint32(b, snap)
	b = order.AppendUint32(b, network)
	for _, r := range records {
		b = append(b, r...)
	}
	return b
}

func classicRec(order byteOrder, sec, frac uint32, data []byte, origLen uint32) []byte {
	var b []byte
	b = order.AppendUint32(b, sec)
	b = order.AppendUint32(b, frac)
	b = order.AppendUint32(b, uint32(len(data)))
	b = order.AppendUint32(b, origLen)
	return append(b, data...)
}

func TestLinkTypeName(t *testing.T) {
	cases := map[uint16]string{
		LinkTypeNull:      "NULL",
		LinkTypeEthernet:  "EN10MB",
		LinkTypeRaw:       "RAW",
		LinkTypeLoop:      "LOOP",
		LinkTypeLinuxSLL:  "LINUX_SLL",
		LinkTypeIPv4:      "IPV4",
		LinkTypeLinuxSLL2: "LINUX_SLL2",
		147:               "LINKTYPE_147",
	}
	for lt, want := range cases {
		if got := LinkTypeName(lt); got != want {
			t.Errorf("LinkTypeName(%d) = %q, want %q", lt, got, want)
		}
	}
}

func TestClassicByteOrdersAndResolutions(t *testing.T) {
	payload := []byte{1, 2, 3, 4, 5}
	cases := []struct {
		name  string
		order byteOrder
		magic uint32
		frac  uint32
		want  time.Time
		res   time.Duration
	}{
		{"le-micro", binary.LittleEndian, magicMicro, 250000, time.Unix(1700000000, 250000000), time.Microsecond},
		{"be-micro", binary.BigEndian, magicMicro, 1, time.Unix(1700000000, 1000), time.Microsecond},
		{"le-nano", binary.LittleEndian, magicNano, 123456789, time.Unix(1700000000, 123456789), time.Nanosecond},
		{"be-nano", binary.BigEndian, magicNano, 5, time.Unix(1700000000, 5), time.Nanosecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := classicFile(tc.order, tc.magic, uint32(LinkTypeEthernet), 65535,
				classicRec(tc.order, 1700000000, tc.frac, payload, 60))
			r, pkts, err := readAll(t, file)
			if err != nil {
				t.Fatal(err)
			}
			if r.Format() != FormatPcap {
				t.Errorf("format = %q", r.Format())
			}
			ifs := r.Interfaces()
			if len(ifs) != 1 || ifs[0].LinkType != LinkTypeEthernet || ifs[0].SnapLen != 65535 || ifs[0].TsResolution != tc.res {
				t.Errorf("interfaces = %+v", ifs)
			}
			if len(pkts) != 1 {
				t.Fatalf("got %d packets", len(pkts))
			}
			p := pkts[0]
			if !p.Timestamp.Equal(tc.want) || p.Number != 1 || p.CapLen != 5 || p.OrigLen != 60 ||
				!bytes.Equal(p.Data, payload) || p.LinkType != LinkTypeEthernet {
				t.Errorf("packet = %+v", p)
			}
		})
	}
}

func TestClassicLinkTypeIgnoresFCSBits(t *testing.T) {
	// FCS length 2 (4 bytes) with the P bit set in the upper bits of the field.
	network := uint32(0x2)<<28 | 1<<27 | uint32(LinkTypeEthernet)
	file := classicFile(binary.LittleEndian, magicMicro, network, 1500)
	r, pkts, err := readAll(t, file)
	if err != nil || len(pkts) != 0 {
		t.Fatalf("err=%v pkts=%d", err, len(pkts))
	}
	if lt := r.Interfaces()[0].LinkType; lt != LinkTypeEthernet {
		t.Errorf("link type = %d", lt)
	}
}

func TestClassicTruncatedAndCorrupt(t *testing.T) {
	le := binary.LittleEndian
	good := classicRec(le, 1, 0, []byte("abcdef"), 6)

	t.Run("partial record data", func(t *testing.T) {
		file := classicFile(le, magicMicro, 1, 65535, good, good[:len(good)-2])
		_, pkts, err := readAll(t, file)
		if !errors.Is(err, ErrTruncatedFile) || len(pkts) != 1 {
			t.Fatalf("err=%v pkts=%d", err, len(pkts))
		}
	})
	t.Run("partial record header", func(t *testing.T) {
		file := classicFile(le, magicMicro, 1, 65535, good, good[:7])
		_, pkts, err := readAll(t, file)
		if !errors.Is(err, ErrTruncatedFile) || len(pkts) != 1 {
			t.Fatalf("err=%v pkts=%d", err, len(pkts))
		}
	})
	t.Run("absurd captured length", func(t *testing.T) {
		var rec []byte
		rec = le.AppendUint32(rec, 1)
		rec = le.AppendUint32(rec, 0)
		rec = le.AppendUint32(rec, 0x7fffffff)
		rec = le.AppendUint32(rec, 0x7fffffff)
		_, _, err := readAll(t, classicFile(le, magicMicro, 1, 65535, rec))
		if !errors.Is(err, ErrCorrupt) {
			t.Fatalf("err = %v, want ErrCorrupt", err)
		}
	})
	t.Run("short global header", func(t *testing.T) {
		file := classicFile(le, magicMicro, 1, 65535)[:20]
		if _, err := NewReader(bytes.NewReader(file)); !errors.Is(err, ErrTruncatedFile) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("original length below captured length", func(t *testing.T) {
		_, pkts, err := readAll(t, classicFile(le, magicMicro, 1, 65535, classicRec(le, 1, 0, []byte("abcd"), 2)))
		if err != nil || pkts[0].OrigLen != 4 {
			t.Fatalf("err=%v pkt=%+v", err, pkts)
		}
	})
}

func TestUnknownFormat(t *testing.T) {
	for _, file := range [][]byte{nil, {0xd4, 0xc3}, []byte("GIF89a......................")} {
		if _, err := NewReader(bytes.NewReader(file)); !errors.Is(err, ErrUnknownFormat) {
			t.Errorf("NewReader(%q) err = %v", file, err)
		}
	}
}

func TestClassicWriterRoundTrip(t *testing.T) {
	ts := time.Date(2024, 5, 6, 7, 8, 9, 123456789, time.UTC)
	frames := [][]byte{bytes.Repeat([]byte{0xab}, 100), {1, 2, 3}, {}}
	for _, nanos := range []bool{false, true} {
		var buf bytes.Buffer
		w, err := NewWriter(&buf, LinkTypeLinuxSLL, 64, nanos)
		if err != nil {
			t.Fatal(err)
		}
		for i, f := range frames {
			if err := w.WritePacket(ts.Add(time.Duration(i)*time.Second), f, len(f)); err != nil {
				t.Fatal(err)
			}
		}
		// origLen below len(data) is raised to len(data).
		if err := w.WritePacket(ts, []byte{9, 9}, 1); err != nil {
			t.Fatal(err)
		}

		r, pkts, err := readAll(t, buf.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if ifs := r.Interfaces(); ifs[0].LinkType != LinkTypeLinuxSLL || ifs[0].SnapLen != 64 {
			t.Errorf("interface = %+v", ifs[0])
		}
		if len(pkts) != 4 {
			t.Fatalf("got %d packets", len(pkts))
		}
		wantTS := ts.Truncate(time.Microsecond)
		if nanos {
			wantTS = ts
		}
		if !pkts[0].Timestamp.Equal(wantTS) {
			t.Errorf("nanos=%v: ts = %v, want %v", nanos, pkts[0].Timestamp, wantTS)
		}
		if pkts[0].CapLen != 64 || pkts[0].OrigLen != 100 || !bytes.Equal(pkts[0].Data, frames[0][:64]) {
			t.Errorf("snaplen cut: caplen=%d origlen=%d", pkts[0].CapLen, pkts[0].OrigLen)
		}
		if pkts[1].CapLen != 3 || pkts[1].OrigLen != 3 || pkts[2].CapLen != 0 || pkts[3].OrigLen != 2 {
			t.Errorf("packets = %+v", pkts[1:])
		}
		for i, p := range pkts {
			if p.Number != i+1 {
				t.Errorf("packet %d has number %d", i, p.Number)
			}
		}
	}
}

func TestWritersRejectTimesOutsideFormat(t *testing.T) {
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, 1, 0, false)
	if err := w.WritePacket(time.Unix(-1, 0), []byte{1}, 1); err == nil {
		t.Error("classic writer accepted a time before 1970")
	}
	ng, _ := NewNgWriter(&buf, 1, 0)
	if err := ng.WritePacket(time.Time{}, []byte{1}, 1); err == nil {
		t.Error("pcapng writer accepted the zero time")
	}
}

func TestDamagedLengthDoesNotAllocateAnnouncedSize(t *testing.T) {
	le := binary.LittleEndian
	var rec []byte
	rec = le.AppendUint32(rec, 1)
	rec = le.AppendUint32(rec, 0)
	rec = le.AppendUint32(rec, 200<<20) // plausible-looking but damaged
	rec = le.AppendUint32(rec, 200<<20)
	rec = append(rec, make([]byte, 100)...)
	file := classicFile(le, magicMicro, 1, 65535, rec)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, _, err := readAll(t, file)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrTruncatedFile) {
		t.Fatalf("err = %v", err)
	}
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 8<<20 {
		t.Errorf("reader allocated %d bytes for a 100-byte record", grew)
	}
}
