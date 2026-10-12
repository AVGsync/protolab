package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// frame is a message cut out of a stream, before rule matching.
type frame struct {
	offset, length int
	issues         []string
	// partial is set when the frame is cut short by the end of the stream or
	// contains gap bytes.
	partial bool
	ts      string // timestamp of the segment holding the first byte
}

// frameStream fills st.frames and st.unframed. Every known byte of the
// stream ends up either in exactly one frame or in an unframed range; gap
// bytes outside frames are left to the capture defect report.
func (st *stream) frameStream(f *framing) {
	switch {
	case f.kind == spec.FramingLengthPrefixed && f.lengthFieldValid():
		st.frameLengthPrefixed(f)
	case f.kind == spec.FramingDelimiter && len(f.delimiter) > 0:
		st.frameDelimited(f.delimiter)
	case f.kind == spec.FramingFixed && f.size > 0:
		st.frameFixed(f.size)
	default:
		// The framing cannot be applied (the spec is invalid): nothing in the
		// stream is attributed to a message.
		st.markUnframed(0, len(st.data))
	}
}

func (f *framing) lengthFieldValid() bool {
	return f.lenOffset >= 0 && (f.lenSize == 1 || f.lenSize == 2 || f.lenSize == 4)
}

func (st *stream) frameLengthPrefixed(f *framing) {
	n := len(st.data)
	headerLen := f.lenOffset + f.lenSize
	pos := 0
	needSync := false
	for pos < n {
		if needSync {
			next := st.findSync(f.sync, pos)
			if next < 0 {
				st.markUnframed(pos, n-pos)
				return
			}
			st.markUnframed(pos, next-pos)
			pos, needSync = next, false
		}

		if headerLen > n-pos {
			st.emit(pos, n-pos, "Поток закончился внутри заголовка", true)
			return
		}
		if st.anyUnknown(pos, pos+headerLen) {
			// The length itself is unknown, so the message boundary cannot be
			// trusted: skip past the gap and look for the next sync pattern.
			st.markUnframed(pos, 1)
			pos = st.skipUnknown(pos + 1)
			needSync = true
			continue
		}

		total := f.readLength(st.data[pos+f.lenOffset:]) + f.adjust
		if total < f.minLen || total > f.maxLen {
			st.markUnframed(pos, 1)
			pos++
			needSync = true
			continue
		}
		if total > int64(n-pos) {
			st.emit(pos, n-pos, fmt.Sprintf("Ожидалось %d Б, в потоке %d Б", total, n-pos), true)
			return
		}
		st.emit(pos, int(total), "", false)
		pos += int(total)
	}
}

func (f *framing) readLength(b []byte) int64 {
	switch {
	case f.lenSize == 1:
		return int64(b[0])
	case f.lenSize == 2 && f.lenBig:
		return int64(binary.BigEndian.Uint16(b))
	case f.lenSize == 2:
		return int64(binary.LittleEndian.Uint16(b))
	case f.lenBig:
		return int64(binary.BigEndian.Uint32(b))
	default:
		return int64(binary.LittleEndian.Uint32(b))
	}
}

// findSync returns the first offset at or after from where the sync pattern
// occurs on known bytes, or -1. Without a pattern the framer can only assume
// that a message starts right here.
func (st *stream) findSync(sync []byte, from int) int {
	if len(sync) == 0 {
		return from
	}
	for i := from; i <= len(st.data)-len(sync); i++ {
		j := bytes.Index(st.data[i:], sync)
		if j < 0 {
			return -1
		}
		i += j
		if !st.anyUnknown(i, i+len(sync)) {
			return i
		}
	}
	return -1
}

// frameDelimited ends a message after every delimiter found on known bytes;
// zero-filled gap bytes never produce a boundary.
func (st *stream) frameDelimited(delim []byte) {
	n := len(st.data)
	start := 0
	for i := 0; i <= n-len(delim); {
		j := bytes.Index(st.data[i:], delim)
		if j < 0 {
			break
		}
		hit := i + j
		if st.anyUnknown(hit, hit+len(delim)) {
			i = hit + 1
			continue
		}
		end := hit + len(delim)
		st.emit(start, end-start, "", false)
		start, i = end, end
	}
	if start < n {
		st.emit(start, n-start, "Нет завершающего разделителя", true)
	}
}

func (st *stream) frameFixed(size int) {
	n := len(st.data)
	for pos := 0; pos < n; pos += size {
		if size > n-pos {
			st.emit(pos, n-pos, "Поток закончился внутри сообщения", true)
			return
		}
		st.emit(pos, size, "", false)
	}
}

// emit records a frame. A frame made only of gap bytes carries nothing that
// could be interpreted and is dropped; one that contains some is partial.
func (st *stream) emit(offset, length int, issue string, truncated bool) {
	unknown := st.countUnknown(offset, offset+length)
	if unknown == length {
		return
	}
	f := frame{offset: offset, length: length, partial: truncated || unknown > 0}
	if issue != "" {
		f.issues = append(f.issues, issue)
	}
	if unknown > 0 {
		f.issues = append(f.issues, fmt.Sprintf("%d Б неизвестны (пропуск в записи)", unknown))
	}
	f.ts = st.segs.timestampAt(offset)
	st.frames = append(st.frames, f)
}

// markUnframed records the known bytes of [offset, offset+length) as lying
// outside any message, merging with the previous range when adjacent.
func (st *stream) markUnframed(offset, length int) {
	end := offset + length
	for offset < end {
		offset = st.skipUnknown(offset)
		if offset >= end {
			return
		}
		runEnd := end
		if st.unknown != nil {
			if i := bytes.IndexByte(st.unknown[offset:end], 1); i >= 0 {
				runEnd = offset + i
			}
		}
		st.addUnframed(offset, runEnd-offset)
		offset = runEnd
	}
}

func (st *stream) addUnframed(offset, length int) {
	if k := len(st.unframed) - 1; k >= 0 && st.unframed[k].End() == offset {
		st.unframed[k].Length += length
		return
	}
	st.unframed = append(st.unframed, model.ByteRange{Offset: offset, Length: length})
}
