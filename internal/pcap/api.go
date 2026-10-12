// Package pcap reads and writes classic libpcap and pcapng capture files.
// It is a small pure-Go implementation (no cgo, no libpcap).
package pcap

import (
	"errors"
	"io"
	"strconv"
	"time"
)

// Link types (http://www.tcpdump.org/linktypes.html) the rest of the program understands.
const (
	LinkTypeNull      uint16 = 0   // BSD loopback, host-order uint32 family
	LinkTypeEthernet  uint16 = 1   // EN10MB
	LinkTypeRaw       uint16 = 101 // raw IPv4/IPv6
	LinkTypeLoop      uint16 = 108 // OpenBSD loopback, network-order family
	LinkTypeLinuxSLL  uint16 = 113 // Linux cooked v1
	LinkTypeIPv4      uint16 = 228 // raw IPv4
	LinkTypeLinuxSLL2 uint16 = 276 // Linux cooked v2
)

// Format of a capture file.
type Format string

const (
	FormatPcap   Format = "pcap"
	FormatPcapNG Format = "pcapng"
)

var (
	// ErrUnknownFormat: the file starts with neither a pcap nor a pcapng magic.
	ErrUnknownFormat = errors.New("pcap: not a pcap or pcapng file")
	// ErrTruncatedFile: the file ends in the middle of a record/block.
	ErrTruncatedFile = errors.New("pcap: file is truncated")
)

// PacketError reports a packet record that cannot be used although the file
// structure around it is intact (a pcapng packet block naming an undeclared
// interface, or announcing more bytes than it holds). The record is consumed:
// Next can be called again to continue with the following one. It wraps
// ErrCorrupt.
type PacketError struct {
	Number int // the record's packet number: it counts like any other packet
	Err    error
}

func (e *PacketError) Error() string { return e.Err.Error() }

func (e *PacketError) Unwrap() error { return e.Err }

// Interface describes a capture interface (one per classic pcap file, one per IDB in pcapng).
type Interface struct {
	LinkType uint16
	SnapLen  uint32
	// TsResolution is the duration of one timestamp unit (1µs by default, 1ns for nanosecond pcap).
	TsResolution time.Duration
}

// Packet is one captured frame. Data is owned by the caller (not reused by the reader).
type Packet struct {
	Number    int // 1-based ordinal in the file, counting every packet record
	Timestamp time.Time
	CapLen    int // == len(Data)
	OrigLen   int // length on the wire
	Interface int // index into Reader.Interfaces()
	LinkType  uint16
	Data      []byte
}

// LinkTypeName returns the libpcap name of a link type ("EN10MB", "LINUX_SLL", ...).
func LinkTypeName(lt uint16) string {
	switch lt {
	case LinkTypeNull:
		return "NULL"
	case LinkTypeEthernet:
		return "EN10MB"
	case LinkTypeRaw:
		return "RAW"
	case LinkTypeLoop:
		return "LOOP"
	case LinkTypeLinuxSLL:
		return "LINUX_SLL"
	case LinkTypeIPv4:
		return "IPV4"
	case LinkTypeLinuxSLL2:
		return "LINUX_SLL2"
	}
	return "LINKTYPE_" + strconv.Itoa(int(lt))
}

// Reader reads packets sequentially.
type Reader struct{ impl readerImpl }

type readerImpl interface {
	format() Format
	interfaces() []Interface
	next() (Packet, error)
}

// NewReader detects the format by its magic number.
func NewReader(r io.Reader) (*Reader, error) {
	impl, err := openReader(r)
	if err != nil {
		return nil, err
	}
	return &Reader{impl: impl}, nil
}

func (r *Reader) Format() Format { return r.impl.format() }

// Interfaces seen so far (pcapng may declare interfaces between packets).
func (r *Reader) Interfaces() []Interface { return r.impl.interfaces() }

// Next returns the next packet, io.EOF at a clean end, ErrTruncatedFile on a
// partial trailing record, a *PacketError for an unusable record that can be
// skipped. Any other error ends the file.
func (r *Reader) Next() (Packet, error) { return r.impl.next() }

// NgWriter writes a single-section, single-interface pcapng file (microsecond timestamps).
type NgWriter struct {
	w       io.Writer
	snapLen uint32
}

// NewNgWriter writes the section header and interface description blocks.
func NewNgWriter(w io.Writer, linkType uint16, snapLen uint32) (*NgWriter, error) {
	if _, err := w.Write(ngFileHeader(linkType, snapLen)); err != nil {
		return nil, err
	}
	return &NgWriter{w: w, snapLen: snapLen}, nil
}

// WritePacket writes an enhanced packet block. data longer than snapLen is cut to snapLen;
// origLen is the original frame length (>= len(data)).
func (w *NgWriter) WritePacket(ts time.Time, data []byte, origLen int) error {
	block, err := enhancedPacketBlock(ts, data, origLen, w.snapLen)
	if err != nil {
		return err
	}
	_, err = w.w.Write(block)
	return err
}

// Writer writes a classic pcap file.
type Writer struct {
	w       io.Writer
	snapLen uint32
	nanos   bool
}

// NewWriter writes the global header (little-endian; nanosecond magic when nanos is true).
func NewWriter(w io.Writer, linkType uint16, snapLen uint32, nanos bool) (*Writer, error) {
	if _, err := w.Write(classicFileHeader(linkType, snapLen, nanos)); err != nil {
		return nil, err
	}
	return &Writer{w: w, snapLen: snapLen, nanos: nanos}, nil
}

// WritePacket writes one record; data longer than snapLen is cut to snapLen.
func (w *Writer) WritePacket(ts time.Time, data []byte, origLen int) error {
	record, err := classicRecord(ts, data, origLen, w.snapLen, w.nanos)
	if err != nil {
		return err
	}
	_, err = w.w.Write(record)
	return err
}
