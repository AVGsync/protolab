package pcap

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"
)

// pcapng option codes used by the reader.
const (
	optEndOfOpt  = 0
	optTsResol   = 9  // if_tsresol
	optTsOffset  = 14 // if_tsoffset
	minSHBLength = 28
)

// ngInterface is an interface of the current section.
type ngInterface struct {
	index    int // position in ngReader.all, i.e. Packet.Interface
	linkType uint16
	snapLen  uint32
	unit     tsUnit
	offset   int64 // if_tsoffset, seconds
}

// ngReader reads pcapng: a sequence of sections, each starting with a Section
// Header Block that fixes the byte order and drops the interfaces declared before.
type ngReader struct {
	r       *bufio.Reader
	order   binary.ByteOrder
	all     []Interface   // every interface of the file, across sections
	section []ngInterface // interfaces of the current section, by interface ID
	number  int
	buf     []byte
}

func newNgReader(r *bufio.Reader) (*ngReader, error) {
	ng := &ngReader{r: r, order: binary.LittleEndian}
	typ, body, err := ng.readBlock()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrTruncatedFile
		}
		return nil, err
	}
	if typ != blockSHB {
		return nil, corrupt("pcapng file does not start with a section header")
	}
	if err := ng.startSection(body); err != nil {
		return nil, err
	}
	return ng, nil
}

func (ng *ngReader) format() Format { return FormatPcapNG }

func (ng *ngReader) interfaces() []Interface { return slices.Clone(ng.all) }

func (ng *ngReader) next() (Packet, error) {
	for {
		typ, body, err := ng.readBlock()
		if err != nil {
			return Packet{}, err
		}
		switch typ {
		case blockSHB:
			err = ng.startSection(body)
		case blockIDB:
			err = ng.addInterface(body)
		case blockEPB:
			return ng.enhancedPacket(body)
		case blockOPB:
			return ng.obsoletePacket(body)
		case blockSPB:
			return ng.simplePacket(body)
		}
		// Name resolution, statistics, custom and unknown blocks carry nothing we need.
		if err != nil {
			return Packet{}, err
		}
	}
}

// readBlock reads one block and returns its type and body (without the leading
// type/length and the trailing length). The body is only valid until the next call.
func (ng *ngReader) readBlock() (uint32, []byte, error) {
	var hdr [8]byte
	if err := readFull(ng.r, hdr[:]); err != nil {
		return 0, nil, err
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) == blockSHB {
		if err := ng.detectByteOrder(); err != nil {
			return 0, nil, err
		}
	}
	typ := ng.order.Uint32(hdr[0:4])
	total := ng.order.Uint32(hdr[4:8])
	switch {
	case total%4 != 0 || total < 12:
		return 0, nil, corrupt("block of type 0x%08x has invalid length %d", typ, total)
	case total > maxRecordLen:
		return 0, nil, corrupt("block of type 0x%08x has implausible length %d", typ, total)
	case typ == blockSHB && total < minSHBLength:
		return 0, nil, corrupt("section header block is %d bytes long", total)
	}

	rest := int(total) - 8
	if cap(ng.buf) < rest {
		ng.buf = make([]byte, rest)
	}
	buf := ng.buf[:rest]
	if err := readRest(ng.r, buf); err != nil {
		return 0, nil, err
	}
	if trailer := ng.order.Uint32(buf[rest-4:]); trailer != total {
		return 0, nil, corrupt("block of type 0x%08x: trailing length %d differs from leading length %d", typ, trailer, total)
	}
	return typ, buf[:rest-4], nil
}

// detectByteOrder peeks at the byte-order magic that follows the SHB length.
func (ng *ngReader) detectByteOrder() error {
	m, err := ng.r.Peek(4)
	if err != nil {
		return ErrTruncatedFile
	}
	switch {
	case binary.LittleEndian.Uint32(m) == byteOrderMagic:
		ng.order = binary.LittleEndian
	case binary.BigEndian.Uint32(m) == byteOrderMagic:
		ng.order = binary.BigEndian
	default:
		return corrupt("section header has unknown byte-order magic % x", m)
	}
	return nil
}

func (ng *ngReader) startSection(body []byte) error {
	// body: byte-order magic(4) major(2) minor(2) section length(8) options
	major, minor := ng.order.Uint16(body[4:6]), ng.order.Uint16(body[6:8])
	if major != 1 {
		return fmt.Errorf("pcap: unsupported pcapng version %d.%d", major, minor)
	}
	ng.section = ng.section[:0]
	return nil
}

func (ng *ngReader) addInterface(body []byte) error {
	// body: link type(2) reserved(2) snaplen(4) options
	if len(body) < 8 {
		return corrupt("interface description block is too short")
	}
	iface := ngInterface{
		index:    len(ng.all),
		linkType: ng.order.Uint16(body[0:2]),
		snapLen:  ng.order.Uint32(body[4:8]),
		unit:     tsUnit{exp: 6},
	}
	err := ng.walkOptions(body[8:], func(code uint16, val []byte) {
		switch {
		case code == optTsResol && len(val) >= 1:
			iface.unit = tsUnit{binary: val[0]&0x80 != 0, exp: val[0] & 0x7f}
		case code == optTsOffset && len(val) >= 8:
			iface.offset = int64(ng.order.Uint64(val))
		}
	})
	if err != nil {
		return err
	}
	ng.section = append(ng.section, iface)
	ng.all = append(ng.all, Interface{
		LinkType:     iface.linkType,
		SnapLen:      iface.snapLen,
		TsResolution: iface.unit.duration(),
	})
	return nil
}

// walkOptions calls fn for every option up to opt_endofopt or the end of b.
func (ng *ngReader) walkOptions(b []byte, fn func(code uint16, val []byte)) error {
	for len(b) >= 4 {
		code, n := ng.order.Uint16(b[0:2]), int(ng.order.Uint16(b[2:4]))
		if code == optEndOfOpt {
			return nil
		}
		if 4+n > len(b) {
			return corrupt("option %d overruns its block", code)
		}
		fn(code, b[4:4+n])
		b = b[min(4+(n+3)&^3, len(b)):]
	}
	return nil
}

func (ng *ngReader) enhancedPacket(body []byte) (Packet, error) {
	// body: interface ID(4) timestamp high(4) low(4) captured len(4) original len(4) data options
	if len(body) < 20 {
		return Packet{}, corrupt("enhanced packet block is too short")
	}
	return ng.timedPacket(body, ng.order.Uint32(body[0:4]))
}

func (ng *ngReader) obsoletePacket(body []byte) (Packet, error) {
	// Same layout as the EPB except for a 16-bit interface ID followed by a drop counter.
	if len(body) < 20 {
		return Packet{}, corrupt("packet block is too short")
	}
	return ng.timedPacket(body, uint32(ng.order.Uint16(body[0:2])))
}

func (ng *ngReader) timedPacket(body []byte, ifaceID uint32) (Packet, error) {
	iface, err := ng.iface(ifaceID)
	if err != nil {
		return Packet{}, err
	}
	ts := uint64(ng.order.Uint32(body[4:8]))<<32 | uint64(ng.order.Uint32(body[8:12]))
	capLen := ng.order.Uint32(body[12:16])
	origLen := ng.order.Uint32(body[16:20])
	if uint64(capLen) > uint64(len(body)-20) {
		return Packet{}, corrupt("packet %d: captured length %d exceeds its block", ng.number+1, capLen)
	}
	return ng.packet(iface, iface.unit.time(ts, iface.offset), body[20:20+capLen], origLen), nil
}

func (ng *ngReader) simplePacket(body []byte) (Packet, error) {
	// body: original len(4) data. The SPB implicitly belongs to interface 0, has no
	// timestamp, and its captured length follows from the snaplen and the block size.
	if len(body) < 4 {
		return Packet{}, corrupt("simple packet block is too short")
	}
	iface, err := ng.iface(0)
	if err != nil {
		return Packet{}, err
	}
	origLen := ng.order.Uint32(body[0:4])
	capLen := min(uint64(origLen), uint64(len(body)-4))
	if iface.snapLen > 0 {
		capLen = min(capLen, uint64(iface.snapLen))
	}
	return ng.packet(iface, time.Time{}, body[4:4+capLen], origLen), nil
}

func (ng *ngReader) iface(id uint32) (ngInterface, error) {
	if uint64(id) >= uint64(len(ng.section)) {
		return ngInterface{}, corrupt("packet %d refers to undeclared interface %d", ng.number+1, id)
	}
	return ng.section[id], nil
}

func (ng *ngReader) packet(iface ngInterface, ts time.Time, data []byte, origLen uint32) Packet {
	ng.number++
	capLen := len(data)
	return Packet{
		Number:    ng.number,
		Timestamp: ts,
		CapLen:    capLen,
		OrigLen:   max(int(origLen), capLen),
		Interface: iface.index,
		LinkType:  iface.linkType,
		Data:      slices.Clone(data),
	}
}
