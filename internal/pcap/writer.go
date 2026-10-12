package pcap

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

var (
	le = binary.LittleEndian

	errTimeRange = errors.New("pcap: timestamp outside the range of the file format")
	errOrigLen   = errors.New("pcap: original length does not fit in 32 bits")
)

// cut applies the snapshot length. The original length is never below the
// number of bytes the caller actually had.
func cut(data []byte, origLen int, snapLen uint32) ([]byte, uint32, error) {
	origLen = max(origLen, len(data))
	if origLen > math.MaxUint32 {
		return nil, 0, errOrigLen
	}
	if snapLen > 0 && len(data) > int(snapLen) {
		data = data[:snapLen]
	}
	return data, uint32(origLen), nil
}

// ngFileHeader is a little-endian Section Header Block followed by one Interface
// Description Block without options (so timestamps are in microseconds).
func ngFileHeader(linkType uint16, snapLen uint32) []byte {
	const shbLen, idbLen = 28, 20
	b := make([]byte, 0, shbLen+idbLen)

	b = le.AppendUint32(b, blockSHB)
	b = le.AppendUint32(b, shbLen)
	b = le.AppendUint32(b, byteOrderMagic)
	b = le.AppendUint16(b, 1)              // major version
	b = le.AppendUint16(b, 0)              // minor version
	b = le.AppendUint64(b, math.MaxUint64) // section length -1: not specified
	b = le.AppendUint32(b, shbLen)

	b = le.AppendUint32(b, blockIDB)
	b = le.AppendUint32(b, idbLen)
	b = le.AppendUint16(b, linkType)
	b = le.AppendUint16(b, 0) // reserved
	b = le.AppendUint32(b, snapLen)
	b = le.AppendUint32(b, idbLen)
	return b
}

func enhancedPacketBlock(ts time.Time, data []byte, origLen int, snapLen uint32) ([]byte, error) {
	us := ts.UnixMicro()
	if us < 0 {
		return nil, errTimeRange
	}
	data, orig, err := cut(data, origLen, snapLen)
	if err != nil {
		return nil, err
	}
	padded := (len(data) + 3) &^ 3
	total := 32 + padded

	b := make([]byte, 0, total)
	b = le.AppendUint32(b, blockEPB)
	b = le.AppendUint32(b, uint32(total))
	b = le.AppendUint32(b, 0) // interface ID
	b = le.AppendUint32(b, uint32(uint64(us)>>32))
	b = le.AppendUint32(b, uint32(us))
	b = le.AppendUint32(b, uint32(len(data)))
	b = le.AppendUint32(b, orig)
	b = append(b, data...)
	b = append(b, make([]byte, padded-len(data))...)
	b = le.AppendUint32(b, uint32(total))
	return b, nil
}

func classicFileHeader(linkType uint16, snapLen uint32, nanos bool) []byte {
	magic := magicMicro
	if nanos {
		magic = magicNano
	}
	b := make([]byte, 0, 24)
	b = le.AppendUint32(b, magic)
	b = le.AppendUint16(b, 2) // major version
	b = le.AppendUint16(b, 4) // minor version
	b = le.AppendUint32(b, 0) // thiszone
	b = le.AppendUint32(b, 0) // sigfigs
	b = le.AppendUint32(b, snapLen)
	b = le.AppendUint32(b, uint32(linkType))
	return b
}

func classicRecord(ts time.Time, data []byte, origLen int, snapLen uint32, nanos bool) ([]byte, error) {
	sec := ts.Unix()
	if sec < 0 || sec > math.MaxUint32 {
		return nil, errTimeRange
	}
	frac := uint32(ts.Nanosecond())
	if !nanos {
		frac /= 1000
	}
	data, orig, err := cut(data, origLen, snapLen)
	if err != nil {
		return nil, err
	}

	b := make([]byte, 0, 16+len(data))
	b = le.AppendUint32(b, uint32(sec))
	b = le.AppendUint32(b, frac)
	b = le.AppendUint32(b, uint32(len(data)))
	b = le.AppendUint32(b, orig)
	b = append(b, data...)
	return b, nil
}
