package pcap

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"time"
)

// classicReader reads the libpcap format: a 24-byte global header followed by
// records made of a 16-byte header and the captured bytes.
type classicReader struct {
	r      *bufio.Reader
	order  binary.ByteOrder
	iface  Interface
	unitNs int64 // nanoseconds per unit of the sub-second timestamp field
	number int
	hdr    [16]byte
}

func newClassicReader(r *bufio.Reader, order binary.ByteOrder, nanos bool) (*classicReader, error) {
	var h [24]byte
	if err := readRest(r, h[:]); err != nil {
		return nil, err
	}
	major, minor := order.Uint16(h[4:6]), order.Uint16(h[6:8])
	if major != 2 {
		return nil, fmt.Errorf("pcap: unsupported pcap version %d.%d", major, minor)
	}
	// h[8:16] holds thiszone and sigfigs; every writer stores zeros and libpcap ignores them.
	c := &classicReader{
		r:      r,
		order:  order,
		unitNs: 1000,
		iface: Interface{
			// The upper 16 bits carry FCS length and reserved bits, not the link type.
			LinkType:     uint16(order.Uint32(h[20:24]) & 0xffff),
			SnapLen:      order.Uint32(h[16:20]),
			TsResolution: time.Microsecond,
		},
	}
	if nanos {
		c.unitNs = 1
		c.iface.TsResolution = time.Nanosecond
	}
	return c, nil
}

func (c *classicReader) format() Format { return FormatPcap }

func (c *classicReader) interfaces() []Interface { return []Interface{c.iface} }

func (c *classicReader) next() (Packet, error) {
	if err := readFull(c.r, c.hdr[:]); err != nil {
		return Packet{}, err
	}
	sec := c.order.Uint32(c.hdr[0:4])
	frac := c.order.Uint32(c.hdr[4:8])
	capLen := c.order.Uint32(c.hdr[8:12])
	origLen := c.order.Uint32(c.hdr[12:16])
	if capLen > maxRecordLen {
		return Packet{}, corrupt("record %d: captured length %d is not plausible", c.number+1, capLen)
	}

	data, err := readAnnounced(c.r, nil, int(capLen))
	if err != nil {
		return Packet{}, err
	}
	c.number++
	return Packet{
		Number:    c.number,
		Timestamp: time.Unix(int64(sec), int64(frac)*c.unitNs),
		CapLen:    int(capLen),
		OrigLen:   int(max(origLen, capLen)),
		Interface: 0,
		LinkType:  c.iface.LinkType,
		Data:      data,
	}, nil
}
