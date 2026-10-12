package pcap

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"
)

func FuzzReader(f *testing.F) {
	var ng, classic bytes.Buffer
	nw, _ := NewNgWriter(&ng, LinkTypeEthernet, 64)
	cw, _ := NewWriter(&classic, LinkTypeEthernet, 64, true)
	for i := range 3 {
		frame := bytes.Repeat([]byte{byte(i)}, 40+i*30)
		nw.WritePacket(time.Unix(int64(i), 0), frame, len(frame))
		cw.WritePacket(time.Unix(int64(i), 0), frame, len(frame))
	}
	f.Add(ng.Bytes())
	f.Add(classic.Bytes())
	g := &ngBuilder{order: binary.BigEndian}
	g.shb().idb(LinkTypeRaw, 0, g.option(optTsResol, []byte{0x85})).spb(4, []byte{1, 2, 3, 4}).opb(0, 7, []byte{9}, 1)
	f.Add(g.b)
	f.Fuzz(func(t *testing.T, file []byte) {
		r, err := NewReader(bytes.NewReader(file))
		if err != nil {
			return
		}
		for n := 1; ; n++ {
			p, err := r.Next()
			if errors.Is(err, io.EOF) || err != nil {
				return
			}
			if p.Number != n || p.CapLen != len(p.Data) || p.OrigLen < p.CapLen || p.Interface >= len(r.Interfaces()) {
				t.Fatalf("inconsistent packet %+v", p)
			}
		}
	})
}
