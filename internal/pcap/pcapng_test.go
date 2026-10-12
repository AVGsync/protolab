package pcap

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"testing"
	"time"
)

// ngBuilder assembles pcapng files block by block in a chosen byte order.
type ngBuilder struct {
	order byteOrder
	b     []byte
}

func pad4(b []byte) []byte {
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func (g *ngBuilder) block(typ uint32, body []byte) *ngBuilder {
	body = pad4(body)
	total := uint32(12 + len(body))
	g.b = g.order.AppendUint32(g.b, typ)
	g.b = g.order.AppendUint32(g.b, total)
	g.b = append(g.b, body...)
	g.b = g.order.AppendUint32(g.b, total)
	return g
}

func (g *ngBuilder) shb() *ngBuilder {
	var body []byte
	body = g.order.AppendUint32(body, byteOrderMagic)
	body = g.order.AppendUint16(body, 1)
	body = g.order.AppendUint16(body, 0)
	body = g.order.AppendUint64(body, ^uint64(0))
	return g.block(blockSHB, body)
}

func (g *ngBuilder) option(code uint16, val []byte) []byte {
	var b []byte
	b = g.order.AppendUint16(b, code)
	b = g.order.AppendUint16(b, uint16(len(val)))
	return pad4(append(b, val...))
}

func (g *ngBuilder) idb(linkType uint16, snap uint32, opts ...[]byte) *ngBuilder {
	var body []byte
	body = g.order.AppendUint16(body, linkType)
	body = g.order.AppendUint16(body, 0)
	body = g.order.AppendUint32(body, snap)
	for _, o := range opts {
		body = append(body, o...)
	}
	if len(opts) > 0 {
		body = append(body, 0, 0, 0, 0) // opt_endofopt
	}
	return g.block(blockIDB, body)
}

func (g *ngBuilder) epb(iface uint32, ts uint64, data []byte, origLen uint32) *ngBuilder {
	var body []byte
	body = g.order.AppendUint32(body, iface)
	body = g.order.AppendUint32(body, uint32(ts>>32))
	body = g.order.AppendUint32(body, uint32(ts))
	body = g.order.AppendUint32(body, uint32(len(data)))
	body = g.order.AppendUint32(body, origLen)
	return g.block(blockEPB, append(body, data...))
}

func (g *ngBuilder) opb(iface uint16, ts uint64, data []byte, origLen uint32) *ngBuilder {
	var body []byte
	body = g.order.AppendUint16(body, iface)
	body = g.order.AppendUint16(body, 0xffff) // drops count: unknown
	body = g.order.AppendUint32(body, uint32(ts>>32))
	body = g.order.AppendUint32(body, uint32(ts))
	body = g.order.AppendUint32(body, uint32(len(data)))
	body = g.order.AppendUint32(body, origLen)
	return g.block(blockOPB, append(body, data...))
}

func (g *ngBuilder) spb(origLen uint32, data []byte) *ngBuilder {
	body := g.order.AppendUint32(nil, origLen)
	return g.block(blockSPB, append(body, data...))
}

func TestNgBothByteOrders(t *testing.T) {
	for _, order := range []byteOrder{binary.LittleEndian, binary.BigEndian} {
		g := &ngBuilder{order: order}
		ts := uint64(1700000000)*1_000_000 + 42
		g.shb().idb(LinkTypeEthernet, 262144).
			block(0x00000004, []byte{0, 0, 0, 0}). // name resolution block: skipped
			epb(0, ts, []byte{1, 2, 3, 4, 5}, 1514).
			block(0x00000005, make([]byte, 16)). // interface statistics: skipped
			block(0x40000bad, []byte("custom")).
			epb(0, ts+1, []byte{6}, 1)

		r, pkts, err := readAll(t, g.b)
		if err != nil {
			t.Fatalf("%v: %v", order, err)
		}
		if r.Format() != FormatPcapNG || len(pkts) != 2 {
			t.Fatalf("%v: format=%q packets=%d", order, r.Format(), len(pkts))
		}
		p := pkts[0]
		if !p.Timestamp.Equal(time.Unix(1700000000, 42000)) || p.CapLen != 5 || p.OrigLen != 1514 ||
			p.LinkType != LinkTypeEthernet || !bytes.Equal(p.Data, []byte{1, 2, 3, 4, 5}) || p.Number != 1 {
			t.Errorf("%v: packet = %+v", order, p)
		}
		if pkts[1].Number != 2 || !bytes.Equal(pkts[1].Data, []byte{6}) {
			t.Errorf("%v: second packet = %+v", order, pkts[1])
		}
		ifs := r.Interfaces()
		if len(ifs) != 1 || ifs[0].SnapLen != 262144 || ifs[0].TsResolution != time.Microsecond {
			t.Errorf("%v: interfaces = %+v", order, ifs)
		}
	}
}

func TestNgTimestampResolutionAndOffset(t *testing.T) {
	g := &ngBuilder{order: binary.LittleEndian}
	offset := g.order.AppendUint64(nil, uint64(1000))
	g.shb().
		idb(LinkTypeRaw, 0, g.option(optTsResol, []byte{9})).                                // nanoseconds
		idb(LinkTypeRaw, 0, g.option(optTsResol, []byte{0x80 | 20})).                        // 2^-20 s
		idb(LinkTypeRaw, 0, g.option(optTsResol, []byte{3}), g.option(optTsOffset, offset)). // ms + 1000 s
		epb(0, 1700000000_123456789, []byte{0x45}, 1).
		epb(1, 5<<20|1<<19, []byte{0x45}, 1). // 5.5 s
		epb(2, 2500, []byte{0x45}, 1)         // 2.5 s + 1000 s

	r, pkts, err := readAll(t, g.b)
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Time{time.Unix(1700000000, 123456789), time.Unix(5, 500000000), time.Unix(1002, 500000000)}
	for i, p := range pkts {
		if !p.Timestamp.Equal(want[i]) {
			t.Errorf("packet %d: ts = %v, want %v", i+1, p.Timestamp, want[i])
		}
		if p.Interface != i {
			t.Errorf("packet %d: interface = %d", i+1, p.Interface)
		}
	}
	res := []time.Duration{time.Nanosecond, 954 * time.Nanosecond, time.Millisecond}
	for i, ifc := range r.Interfaces() {
		if ifc.TsResolution != res[i] {
			t.Errorf("interface %d: resolution = %v, want %v", i, ifc.TsResolution, res[i])
		}
	}
}

func TestTsUnitConversionsAreExact(t *testing.T) {
	cases := []struct {
		unit tsUnit
		ts   uint64
		want time.Time
	}{
		{tsUnit{exp: 0}, 7, time.Unix(7, 0)},
		{tsUnit{exp: 12}, 3_000_000_000_001_999, time.Unix(3000, 1)}, // picoseconds round down
		{tsUnit{exp: 25}, 1 << 63, time.Unix(0, 922)},                // 2^63 * 1e-25 s = 922 ns
		{tsUnit{binary: true, exp: 1}, 3, time.Unix(1, 500000000)},
		{tsUnit{binary: true, exp: 64}, 1 << 63, time.Unix(0, 500000000)},
		{tsUnit{binary: true, exp: 32}, 10<<32 | 1<<31, time.Unix(10, 500000000)},
	}
	for _, tc := range cases {
		if got := tc.unit.time(tc.ts, 0); !got.Equal(tc.want) {
			t.Errorf("%+v.time(%d) = %v, want %v", tc.unit, tc.ts, got, tc.want)
		}
	}
}

func TestNgSimpleAndObsoletePackets(t *testing.T) {
	g := &ngBuilder{order: binary.BigEndian}
	frame := bytes.Repeat([]byte{0x11}, 10)
	g.shb().idb(LinkTypeEthernet, 6).
		spb(10, frame[:6]). // truncated by snaplen 6
		spb(3, frame[:3]).  // short frame, block padded to 4 bytes
		opb(0, 1_000_000, frame, 10)

	_, pkts, err := readAll(t, g.b)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 3 {
		t.Fatalf("got %d packets", len(pkts))
	}
	if pkts[0].CapLen != 6 || pkts[0].OrigLen != 10 || !pkts[0].Timestamp.IsZero() {
		t.Errorf("spb 1 = %+v", pkts[0])
	}
	if pkts[1].CapLen != 3 || pkts[1].OrigLen != 3 {
		t.Errorf("spb 2 = %+v", pkts[1])
	}
	if pkts[2].CapLen != 10 || !pkts[2].Timestamp.Equal(time.Unix(1, 0)) || pkts[2].Number != 3 {
		t.Errorf("opb = %+v", pkts[2])
	}
}

func TestNgSectionsResetInterfaces(t *testing.T) {
	g := &ngBuilder{order: binary.LittleEndian}
	g.shb().idb(LinkTypeEthernet, 0).epb(0, 1, []byte{1}, 1)
	// The second section is big-endian and declares a different interface 0.
	g.order = binary.BigEndian
	g.shb().idb(LinkTypeLinuxSLL, 0).epb(0, 2, []byte{2}, 1)

	r, pkts, err := readAll(t, g.b)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 2 || pkts[0].LinkType != LinkTypeEthernet || pkts[1].LinkType != LinkTypeLinuxSLL {
		t.Fatalf("packets = %+v", pkts)
	}
	if pkts[0].Interface != 0 || pkts[1].Interface != 1 || len(r.Interfaces()) != 2 {
		t.Errorf("interface indexes %d, %d of %d", pkts[0].Interface, pkts[1].Interface, len(r.Interfaces()))
	}

	// An interface of the previous section is not visible in the next one.
	g = &ngBuilder{order: binary.LittleEndian}
	g.shb().idb(LinkTypeEthernet, 0).idb(LinkTypeEthernet, 0).shb().idb(LinkTypeRaw, 0).epb(1, 1, []byte{1}, 1)
	if _, _, err := readAll(t, g.b); !errors.Is(err, ErrCorrupt) {
		t.Errorf("err = %v, want ErrCorrupt for an undeclared interface", err)
	}
}

func TestNgDamagedFiles(t *testing.T) {
	base := func() *ngBuilder {
		g := &ngBuilder{order: binary.LittleEndian}
		return g.shb().idb(LinkTypeEthernet, 0).epb(0, 1, []byte{1, 2, 3, 4, 5, 6, 7, 8}, 8)
	}

	t.Run("truncated block", func(t *testing.T) {
		g := base()
		_, pkts, err := readAll(t, g.b[:len(g.b)-3])
		if !errors.Is(err, ErrTruncatedFile) || len(pkts) != 0 {
			t.Fatalf("err=%v pkts=%d", err, len(pkts))
		}
	})
	t.Run("truncated block header", func(t *testing.T) {
		g := base()
		g.b = append(g.b, 6, 0, 0)
		_, pkts, err := readAll(t, g.b)
		if !errors.Is(err, ErrTruncatedFile) || len(pkts) != 1 {
			t.Fatalf("err=%v pkts=%d", err, len(pkts))
		}
	})
	t.Run("trailer mismatch", func(t *testing.T) {
		g := base()
		g.b[len(g.b)-4]++
		if _, _, err := readAll(t, g.b); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("captured length beyond block", func(t *testing.T) {
		g := &ngBuilder{order: binary.LittleEndian}
		g.shb().idb(LinkTypeEthernet, 0)
		var body []byte
		body = g.order.AppendUint32(body, 0)
		body = g.order.AppendUint64(body, 0)
		body = g.order.AppendUint32(body, 100) // captured length
		body = g.order.AppendUint32(body, 100)
		g.block(blockEPB, append(body, 1, 2, 3, 4))
		if _, _, err := readAll(t, g.b); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("length not a multiple of 4", func(t *testing.T) {
		g := base()
		g.b = append(g.b, 1, 0, 0, 0, 13, 0, 0, 0, 0, 13, 0, 0, 0)
		_, _, err := readAll(t, g.b)
		if !errors.Is(err, ErrCorrupt) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad byte-order magic", func(t *testing.T) {
		g := base()
		g.b[8] = 0
		if _, err := NewReader(bytes.NewReader(g.b)); !errors.Is(err, ErrCorrupt) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("unsupported version", func(t *testing.T) {
		g := base()
		g.b[12] = 2 // major version 2
		if _, err := NewReader(bytes.NewReader(g.b)); err == nil {
			t.Fatal("accepted pcapng major version 2")
		}
	})
}

func TestNgWriterRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewNgWriter(&buf, LinkTypeEthernet, 16)
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2025, 1, 2, 3, 4, 5, 678901234, time.UTC)
	frames := [][]byte{bytes.Repeat([]byte{7}, 40), {1, 2, 3}, {1, 2, 3, 4}}
	for i, f := range frames {
		if err := w.WritePacket(ts.Add(time.Duration(i)*time.Millisecond), f, len(f)+i); err != nil {
			t.Fatal(err)
		}
	}
	r, pkts, err := readAll(t, buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(pkts) != 3 {
		t.Fatalf("got %d packets", len(pkts))
	}
	if ifs := r.Interfaces(); len(ifs) != 1 || ifs[0].LinkType != LinkTypeEthernet || ifs[0].SnapLen != 16 ||
		ifs[0].TsResolution != time.Microsecond {
		t.Errorf("interfaces = %+v", ifs)
	}
	if p := pkts[0]; p.CapLen != 16 || p.OrigLen != 40 || !p.Timestamp.Equal(ts.Truncate(time.Microsecond)) {
		t.Errorf("packet 1 = %+v", p)
	}
	if p := pkts[1]; p.CapLen != 3 || p.OrigLen != 4 || !bytes.Equal(p.Data, frames[1]) {
		t.Errorf("packet 2 = %+v", p)
	}
	if p := pkts[2]; p.CapLen != 4 || p.OrigLen != 6 || !bytes.Equal(p.Data, frames[2]) {
		t.Errorf("packet 3 = %+v", p)
	}
}

func TestReaderDataIsNotReused(t *testing.T) {
	g := &ngBuilder{order: binary.LittleEndian}
	g.shb().idb(LinkTypeEthernet, 0).epb(0, 1, []byte{1, 1, 1, 1}, 4).epb(0, 2, []byte{2, 2, 2, 2}, 4)
	_, pkts, err := readAll(t, g.b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pkts[0].Data, []byte{1, 1, 1, 1}) {
		t.Errorf("first packet data overwritten: %v", pkts[0].Data)
	}
}

func TestNgDamagedPacketBlocksAreSkipped(t *testing.T) {
	g := &ngBuilder{order: binary.LittleEndian}
	g.shb().idb(LinkTypeEthernet, 0).
		epb(0, 1, []byte{1}, 1).
		epb(7, 2, []byte{2}, 1) // undeclared interface
	var body []byte
	body = g.order.AppendUint32(body, 0)
	body = g.order.AppendUint64(body, 3)
	body = g.order.AppendUint32(body, 100) // captured length beyond the block
	body = g.order.AppendUint32(body, 100)
	g.block(blockEPB, append(body, 3, 3, 3, 3))
	g.block(0x00000bad, []byte("unknown block type")).
		idb(LinkTypeRaw, 0, g.option(optTsResol, []byte{9})). // a second interface mid-file
		epb(1, 4, []byte{4}, 1).
		epb(0, 5, []byte{5, 5, 5, 5, 5, 5, 5, 5}, 8)
	g.b = g.b[:len(g.b)-6] // the capture was cut while writing the last block

	r, err := NewReader(bytes.NewReader(g.b))
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	var skipped []int
	for {
		p, err := r.Next()
		var pe *PacketError
		switch {
		case err == nil:
			got = append(got, p.Number)
			continue
		case errors.As(err, &pe):
			if !errors.Is(err, ErrCorrupt) {
				t.Errorf("packet error %v does not wrap ErrCorrupt", err)
			}
			skipped = append(skipped, pe.Number)
			continue
		case !errors.Is(err, ErrTruncatedFile):
			t.Fatalf("err = %v, want ErrTruncatedFile at the end", err)
		}
		break
	}
	if !slices.Equal(got, []int{1, 4}) || !slices.Equal(skipped, []int{2, 3}) {
		t.Errorf("read %v, skipped %v; want [1 4] and [2 3]", got, skipped)
	}
	if ifs := r.Interfaces(); len(ifs) != 2 || ifs[1].TsResolution != time.Nanosecond {
		t.Errorf("interfaces = %+v", ifs)
	}
}
