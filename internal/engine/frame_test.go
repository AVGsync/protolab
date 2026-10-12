package engine

import (
	"reflect"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

type wantFrame struct {
	offset, length int
	partial        bool
	issues         []string
}

func frameAll(t *testing.T, framingJSON string, data []byte, gaps []model.StreamGap) ([]wantFrame, []model.ByteRange) {
	t.Helper()
	c := compile(mustSpec(t, `{"format":"protolab/v1","framing":`+framingJSON+`,"messages":[]}`))
	st := newStream(model.C2S, StreamInput{Bytes: data, Gaps: gaps}, false)
	st.frameStream(&c.framing)
	got := make([]wantFrame, len(st.frames))
	for i, f := range st.frames {
		got[i] = wantFrame{offset: f.offset, length: f.length, partial: f.partial, issues: f.issues}
	}
	return got, st.unframed
}

func checkFrames(t *testing.T, gotFrames []wantFrame, gotUnframed []model.ByteRange, want []wantFrame, wantUnframed []model.ByteRange) {
	t.Helper()
	if len(gotFrames) != len(want) {
		t.Fatalf("frames = %+v, want %+v", gotFrames, want)
	}
	for i := range want {
		g, w := gotFrames[i], want[i]
		if g.offset != w.offset || g.length != w.length || g.partial != w.partial || !reflect.DeepEqual(g.issues, w.issues) {
			t.Errorf("frame %d = %+v, want %+v", i, g, w)
		}
	}
	if len(gotUnframed) != len(wantUnframed) || (len(wantUnframed) > 0 && !reflect.DeepEqual(gotUnframed, wantUnframed)) {
		t.Errorf("unframed = %+v, want %+v", gotUnframed, wantUnframed)
	}
}

const thermoFraming = `{"kind":"length_prefixed","lengthField":{"offset":3,"size":2,"endian":"le"},"lengthAdjust":6,"minLength":6,"maxLength":4096,"sync":"A5"}`

func TestFrameLengthPrefixed(t *testing.T) {
	req := tlFrame(0x01, 1)                    // 6 bytes
	resp := tlFrame(0x81, 1, 0x02, 0x10, 0x20) // 9 bytes
	meas := tlFrame(0x83, 2, 1, 0xE0, 0, 0, 0) // 11 bytes

	tests := []struct {
		name         string
		framing      string
		data         []byte
		gaps         []model.StreamGap
		want         []wantFrame
		wantUnframed []model.ByteRange
	}{
		{
			name:    "consecutive messages",
			framing: thermoFraming,
			data:    concat(req, resp, meas),
			want:    []wantFrame{{offset: 0, length: 6}, {offset: 6, length: 9}, {offset: 15, length: 11}},
		},
		{
			name:    "u32 big-endian length at offset 0",
			framing: `{"kind":"length_prefixed","lengthField":{"offset":0,"size":4,"endian":"be"},"lengthAdjust":4}`,
			data:    []byte{0, 0, 0, 2, 0xAA, 0xBB, 0, 0, 0, 0},
			want:    []wantFrame{{offset: 0, length: 6}, {offset: 6, length: 4}},
		},
		{
			name:    "u8 length",
			framing: `{"kind":"length_prefixed","lengthField":{"offset":1,"size":1,"endian":"le"},"lengthAdjust":2}`,
			data:    []byte{0x10, 1, 0xAA, 0x10, 0, 0x11, 2, 1, 2},
			want:    []wantFrame{{offset: 0, length: 3}, {offset: 3, length: 2}, {offset: 5, length: 4}},
		},
		{
			name:    "stream ends inside a body",
			framing: thermoFraming,
			data:    concat(req, resp[:7]),
			want: []wantFrame{
				{offset: 0, length: 6},
				{offset: 6, length: 7, partial: true, issues: []string{"Ожидалось 9 Б, в потоке 7 Б"}},
			},
		},
		{
			name:    "stream ends inside a header",
			framing: thermoFraming,
			data:    concat(req, resp[:3]),
			want: []wantFrame{
				{offset: 0, length: 6},
				{offset: 6, length: 3, partial: true, issues: []string{"Поток закончился внутри заголовка"}},
			},
		},
		{
			name:         "missing connection start resyncs on the sync pattern",
			framing:      thermoFraming,
			data:         concat([]byte{0x01, 0x02, 0x03, 0x04}, req),
			want:         []wantFrame{{offset: 4, length: 6}},
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 4}},
		},
		{
			name:         "length out of range without sync retries byte by byte",
			framing:      `{"kind":"length_prefixed","lengthField":{"offset":0,"size":1,"endian":"le"},"lengthAdjust":1,"minLength":2,"maxLength":8}`,
			data:         []byte{0x40, 0x40, 0x02, 0xAA, 0xBB},
			want:         []wantFrame{{offset: 2, length: 3}},
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 2}},
		},
		{
			name:    "gap inside a body keeps framing aligned",
			framing: thermoFraming,
			data:    concat(meas, req),
			gaps:    []model.StreamGap{{Offset: 6, Length: 3, Reason: model.GapLost}},
			want: []wantFrame{
				{offset: 0, length: 11, partial: true, issues: []string{"3 Б неизвестны (пропуск в записи)"}},
				{offset: 11, length: 6},
			},
		},
		{
			name:    "gap over a header resyncs after the gap",
			framing: thermoFraming,
			data:    concat(req, meas, resp),
			gaps:    []model.StreamGap{{Offset: 6, Length: 3, Reason: model.GapLost}},
			// Bytes 9..16 are the rest of the lost message: known but unframed.
			want:         []wantFrame{{offset: 0, length: 6}, {offset: 17, length: 9}},
			wantUnframed: []model.ByteRange{{Offset: 9, Length: 8}},
		},
		{
			name:    "header straddling the start of a gap",
			framing: thermoFraming,
			data:    concat(req, meas, resp),
			gaps:    []model.StreamGap{{Offset: 8, Length: 4, Reason: model.GapTruncated}},
			want:    []wantFrame{{offset: 0, length: 6}, {offset: 17, length: 9}},
			wantUnframed: []model.ByteRange{
				{Offset: 6, Length: 2},
				{Offset: 12, Length: 5},
			},
		},
		{
			name:    "stream starting inside a gap",
			framing: thermoFraming,
			data:    concat([]byte{0, 0, 0}, req),
			gaps:    []model.StreamGap{{Offset: 0, Length: 3, Reason: model.GapLost}},
			want:    []wantFrame{{offset: 3, length: 6}},
		},
		{
			name:    "trailing bytes that are all unknown are not a message",
			framing: thermoFraming,
			data:    concat(req, []byte{0, 0, 0}),
			gaps:    []model.StreamGap{{Offset: 6, Length: 3, Reason: model.GapLost}},
			want:    []wantFrame{{offset: 0, length: 6}},
		},
		{
			name:         "zero minLength cannot stall the framer",
			framing:      `{"kind":"length_prefixed","lengthField":{"offset":0,"size":1,"endian":"le"},"lengthAdjust":0,"minLength":0}`,
			data:         []byte{0, 0, 5},
			want:         []wantFrame{{offset: 2, length: 1, partial: true, issues: []string{"Ожидалось 5 Б, в потоке 1 Б"}}},
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 2}},
		},
		{
			name:         "no sync found until the end",
			framing:      thermoFraming,
			data:         []byte{0x01, 0x02, 0x03, 0xFF, 0xFF, 0x04},
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 6}},
		},
		{
			name:         "invalid length field size frames nothing",
			framing:      `{"kind":"length_prefixed","lengthField":{"offset":0,"size":3,"endian":"le"},"lengthAdjust":0}`,
			data:         req,
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 6}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames, unframed := frameAll(t, tt.framing, tt.data, tt.gaps)
			checkFrames(t, frames, unframed, tt.want, tt.wantUnframed)
		})
	}
}

func TestFrameDelimited(t *testing.T) {
	tests := []struct {
		name         string
		framing      string
		data         []byte
		gaps         []model.StreamGap
		want         []wantFrame
		wantUnframed []model.ByteRange
	}{
		{
			name:    "single byte delimiter with an unterminated tail",
			framing: `{"kind":"delimiter","delimiter":"7E"}`,
			data:    []byte{1, 2, 0x7E, 3, 0x7E, 4},
			want: []wantFrame{
				{offset: 0, length: 3},
				{offset: 3, length: 2},
				{offset: 5, length: 1, partial: true, issues: []string{"Нет завершающего разделителя"}},
			},
		},
		{
			name:    "two byte delimiter",
			framing: `{"kind":"delimiter","delimiter":"0D 0A"}`,
			data:    []byte("AT\r\nOK\r\n"),
			want:    []wantFrame{{offset: 0, length: 4}, {offset: 4, length: 4}},
		},
		{
			name:    "self-overlapping delimiter is matched without overlap",
			framing: `{"kind":"delimiter","delimiter":"00 00"}`,
			data:    []byte{0xAA, 0, 0, 0, 0xBB, 0, 0},
			want:    []wantFrame{{offset: 0, length: 3}, {offset: 3, length: 4}},
		},
		{
			name:    "a delimiter inside a gap is not a boundary",
			framing: `{"kind":"delimiter","delimiter":"7E"}`,
			data:    []byte{1, 0x7E, 2, 0x7E},
			gaps:    []model.StreamGap{{Offset: 1, Length: 1, Reason: model.GapLost}},
			want:    []wantFrame{{offset: 0, length: 4, partial: true, issues: []string{"1 Б неизвестны (пропуск в записи)"}}},
		},
		{
			name:         "malformed delimiter frames nothing",
			framing:      `{"kind":"delimiter","delimiter":"zz"}`,
			data:         []byte{1, 2, 3},
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 3}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames, unframed := frameAll(t, tt.framing, tt.data, tt.gaps)
			checkFrames(t, frames, unframed, tt.want, tt.wantUnframed)
		})
	}
}

func TestFrameFixed(t *testing.T) {
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	tests := []struct {
		name         string
		framing      string
		gaps         []model.StreamGap
		want         []wantFrame
		wantUnframed []model.ByteRange
	}{
		{
			name:    "short last message",
			framing: `{"kind":"fixed","size":4}`,
			want: []wantFrame{
				{offset: 0, length: 4},
				{offset: 4, length: 4},
				{offset: 8, length: 2, partial: true, issues: []string{"Поток закончился внутри сообщения"}},
			},
		},
		{
			name:    "messages touched by gaps are partial, fully lost ones are dropped",
			framing: `{"kind":"fixed","size":5}`,
			gaps:    []model.StreamGap{{Offset: 3, Length: 1, Reason: model.GapLost}, {Offset: 5, Length: 5, Reason: model.GapLost}},
			want:    []wantFrame{{offset: 0, length: 5, partial: true, issues: []string{"1 Б неизвестны (пропуск в записи)"}}},
		},
		{
			name:         "non-positive size frames nothing",
			framing:      `{"kind":"fixed","size":0}`,
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 10}},
		},
		{
			name:         "unknown framing kind frames nothing",
			framing:      `{"kind":"magic"}`,
			wantUnframed: []model.ByteRange{{Offset: 0, Length: 10}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frames, unframed := frameAll(t, tt.framing, data, tt.gaps)
			checkFrames(t, frames, unframed, tt.want, tt.wantUnframed)
		})
	}
}

// Every known byte must end up in exactly one frame or one unframed range,
// and no unframed range may contain a gap byte.
func TestFramingAccountsForEveryKnownByte(t *testing.T) {
	data := concat([]byte{9, 9}, tlFrame(1, 1), tlFrame(0x83, 2, 1, 2, 3, 4, 5), []byte{0xA5, 0xFF, 0xFF, 0xFF, 0xFF}, tlFrame(2, 3, 7, 8))
	gapSets := [][]model.StreamGap{
		nil,
		{{Offset: 0, Length: 1}},
		{{Offset: 3, Length: 4}},
		{{Offset: 8, Length: 9}, {Offset: 20, Length: 2}},
		{{Offset: 25, Length: 100}},
	}
	for _, gaps := range gapSets {
		frames, unframed := frameAll(t, thermoFraming, data, gaps)
		mask := unknownMask(len(data), gaps)
		owner := make([]int, len(data))
		for _, f := range frames {
			for i := f.offset; i < f.offset+f.length; i++ {
				owner[i]++
			}
		}
		for _, u := range unframed {
			for i := u.Offset; i < u.End(); i++ {
				if mask != nil && mask[i] == 1 {
					t.Errorf("gaps %v: unframed range %+v contains gap byte %d", gaps, u, i)
				}
				owner[i]++
			}
		}
		for i, n := range owner {
			known := mask == nil || mask[i] == 0
			if known && n != 1 {
				t.Errorf("gaps %v: known byte %d is accounted %d times", gaps, i, n)
			}
			if !known && n > 1 {
				t.Errorf("gaps %v: gap byte %d is accounted %d times", gaps, i, n)
			}
		}
	}
}
