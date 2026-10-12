package pcap

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ErrCorrupt is wrapped by errors about structurally impossible content:
// absurd record or block lengths, mismatched block trailers, unknown interfaces.
var ErrCorrupt = errors.New("pcap: corrupt file")

// maxRecordLen bounds a single captured packet or pcapng block. Real snapshot
// lengths are at most 256 KiB, so anything above this is a damaged length field,
// and trusting it would make the reader allocate gigabytes.
const maxRecordLen = 256 << 20

// Classic pcap magic numbers, as read in the byte order of the writing host.
const (
	magicMicro uint32 = 0xa1b2c3d4
	magicNano  uint32 = 0xa1b23c4d
)

// pcapng block types and the section byte-order magic.
const (
	blockSHB uint32 = 0x0a0d0d0a
	blockIDB uint32 = 0x00000001
	blockOPB uint32 = 0x00000002 // obsolete Packet Block
	blockSPB uint32 = 0x00000003
	blockEPB uint32 = 0x00000006

	byteOrderMagic uint32 = 0x1a2b3c4d
)

func openReader(r io.Reader) (readerImpl, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReaderSize(r, 64<<10)
	}
	magic, err := br.Peek(4)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, ErrUnknownFormat
		}
		return nil, err
	}

	// The SHB block type is a palindrome, so it reads the same in both byte orders.
	if binary.LittleEndian.Uint32(magic) == blockSHB {
		return newNgReader(br)
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		switch order.Uint32(magic) {
		case magicMicro:
			return newClassicReader(br, order, false)
		case magicNano:
			return newClassicReader(br, order, true)
		}
	}
	return nil, ErrUnknownFormat
}

// readFull reads exactly len(buf) bytes. A clean end of input before the first
// byte is io.EOF; an end in the middle is ErrTruncatedFile.
func readFull(r io.Reader, buf []byte) error {
	_, err := io.ReadFull(r, buf)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrTruncatedFile
	}
	return err
}

// readRest is readFull for data that must be present because a header announced it.
func readRest(r io.Reader, buf []byte) error {
	err := readFull(r, buf)
	if errors.Is(err, io.EOF) {
		return ErrTruncatedFile
	}
	return err
}

func corrupt(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
}
