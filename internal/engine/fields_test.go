package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

// parseOne frames data as a single fixed-size message and parses it with one
// rule made of fieldsJSON.
func parseOne(t *testing.T, fieldsJSON string, data []byte, gaps ...model.StreamGap) model.Message {
	t.Helper()
	s := mustSpec(t, fmt.Sprintf(`{"format":"protolab/v1","framing":{"kind":"fixed","size":%d},
		"messages":[{"id":"m","title":"Сообщение","match":[],"fields":%s}]}`, len(data), fieldsJSON))
	in := oneSegment(data, 1, 0)
	in.Gaps = gaps
	got := ParseSession(s, newInput("s", in, StreamInput{}))
	if len(got.Messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(got.Messages))
	}
	return got.Messages[0]
}

type wantField struct {
	name, typ, display, status, issue string
	value                             any
}

func checkField(t *testing.T, f model.ParsedField, w wantField) {
	t.Helper()
	if f.Type != w.typ || f.Display != w.display || f.Status != w.status || f.Issue != w.issue || f.Value != w.value {
		t.Errorf("%s = {type %q value %#v display %q status %q issue %q}\nwant {type %q value %#v display %q status %q issue %q}",
			w.name, f.Type, f.Value, f.Display, f.Status, f.Issue, w.typ, w.value, w.display, w.status, w.issue)
	}
}

func TestFieldTypes(t *testing.T) {
	data := []byte{
		0xFE,       // 0
		0x34, 0x12, // 1..2
		0xFF, 0xFF, // 3..4
		0x78, 0x56, 0x34, 0x12, // 5..8
		0xFE, 0xFF, 0xFF, 0xFF, // 9..12
		'H', 'i', 0x01, '"', // 13..16
		0xAB, 0xCD, // 17..18
	}
	fields := `[
		{"name":"u8","offset":0,"type":"u8"},
		{"name":"i8","offset":0,"type":"i8"},
		{"name":"u16le","offset":1,"type":"u16"},
		{"name":"u16be","offset":1,"type":"u16","endian":"be"},
		{"name":"i16","offset":3,"type":"i16","endian":"le"},
		{"name":"u32le","offset":5,"type":"u32"},
		{"name":"u32be","offset":5,"type":"u32","endian":"be"},
		{"name":"i32","offset":9,"type":"i32"},
		{"name":"i32be","offset":9,"type":"i32","endian":"be"},
		{"name":"text","offset":13,"type":"ascii","length":4},
		{"name":"raw","offset":17,"type":"bytes","length":2},
		{"name":"empty","offset":17,"type":"bytes","length":0},
		{"name":"last","offset":-1,"type":"u8"},
		{"name":"outside","offset":18,"type":"u16"},
		{"name":"before","offset":-30,"type":"u8"},
		{"name":"weird","offset":0,"type":"u7"}
	]`
	m := parseOne(t, fields, data)
	tests := []wantField{
		{name: "u8", typ: "u8", value: int64(254), display: "254", status: "ok"},
		{name: "i8", typ: "i8", value: int64(-2), display: "-2", status: "ok"},
		{name: "u16le", typ: "u16le", value: int64(0x1234), display: "4660", status: "ok"},
		{name: "u16be", typ: "u16be", value: int64(0x3412), display: "13330", status: "ok"},
		{name: "i16", typ: "i16le", value: int64(-1), display: "-1", status: "ok"},
		{name: "u32le", typ: "u32le", value: int64(0x12345678), display: "305419896", status: "ok"},
		{name: "u32be", typ: "u32be", value: int64(0x78563412), display: "2018915346", status: "ok"},
		{name: "i32", typ: "i32le", value: int64(-2), display: "-2", status: "ok"},
		{name: "i32be", typ: "i32be", value: int64(-16777217), display: "-16777217", status: "ok"},
		{name: "text", typ: "ascii", value: `Hi."`, display: `"Hi.\""`, status: "ok"},
		{name: "raw", typ: "bytes", value: "ABCD", display: "AB CD", status: "ok"},
		{name: "empty", typ: "bytes", value: "", display: "", status: "ok"},
		{name: "last", typ: "u8", value: int64(0xCD), display: "205", status: "ok"},
		{name: "outside", typ: "u16le", value: nil, display: "—", status: "violation", issue: "Поле выходит за границы сообщения"},
		{name: "before", typ: "u8", value: nil, display: "—", status: "violation", issue: "Поле выходит за границы сообщения"},
		{name: "weird", typ: "u7", value: nil, display: "—", status: "violation", issue: `Неизвестный тип "u7"`},
	}
	for _, w := range tests {
		checkField(t, mustField(t, m, w.name), w)
	}
	if f := mustField(t, m, "last"); f.Offset != len(data)-1 || f.Path != "last" {
		t.Errorf("negative offset resolved to %d (path %q)", f.Offset, f.Path)
	}
	if m.Status != model.MsgViolation {
		t.Errorf("status = %s, want violation", m.Status)
	}
}

func TestFieldDisplayScaleEnumUnit(t *testing.T) {
	tests := []struct {
		field   string
		data    []byte
		display string
		status  string
		issue   string
	}{
		{`{"name":"t","offset":0,"type":"i16","scale":0.1,"unit":"°C"}`, []byte{0xE0, 0x00}, "22.4 °C", "ok", ""},
		{`{"name":"t","offset":0,"type":"i16","scale":0.1,"unit":"°C"}`, []byte{0xC9, 0xFF}, "-5.5 °C", "ok", ""},
		{`{"name":"t","offset":0,"type":"u16","scale":0.1}`, []byte{0x9A, 0x01}, "41", "ok", ""},
		{`{"name":"t","offset":0,"type":"u8","scale":0.25}`, []byte{1}, "0.3", "ok", ""},
		{`{"name":"t","offset":0,"type":"i8","scale":0.25}`, []byte{0xFF}, "-0.3", "ok", ""},
		{`{"name":"t","offset":0,"type":"u8","scale":0.125}`, []byte{3}, "0.4", "ok", ""},
		{`{"name":"t","offset":0,"type":"u8","scale":0.001}`, []byte{7}, "0.007", "ok", ""},
		{`{"name":"t","offset":0,"type":"u8","scale":1e-7}`, []byte{1}, "1e-7", "ok", ""},
		{`{"name":"t","offset":0,"type":"u8","scale":1e15}`, []byte{200}, "200000000000000000", "ok", ""},
		{`{"name":"t","offset":0,"type":"u32","scale":1e15}`, []byte{0xFF, 0xFF, 0xFF, 0xFF}, "4.294967295e+24", "ok", ""},
		{`{"name":"t","offset":0,"type":"i8","scale":-0.5}`, []byte{3}, "-1.5", "ok", ""},
		{`{"name":"u","offset":0,"type":"u32","unit":"с"}`, []byte{0x8C, 0x51, 0x01, 0x00}, "86412 с", "ok", ""},
		{`{"name":"e","offset":0,"type":"u8","enum":{"1":"уставка"},"unit":"шт"}`, []byte{1}, "уставка (1)", "ok", ""},
		{`{"name":"e","offset":0,"type":"u8","enum":{"1":"уставка"}}`, []byte{7}, "? (7)", "violation", "Значение 7 отсутствует в перечислении"},
		{`{"name":"e","offset":0,"type":"u8","enum":{"2":""}}`, []byte{2}, " (2)", "ok", ""},
	}
	for _, tt := range tests {
		t.Run(tt.field+fmt.Sprint(tt.data), func(t *testing.T) {
			m := parseOne(t, "["+tt.field+"]", tt.data)
			f := m.Fields[0]
			if f.Display != tt.display || f.Status != tt.status || f.Issue != tt.issue {
				t.Errorf("got display %q status %q issue %q, want %q %q %q", f.Display, f.Status, f.Issue, tt.display, tt.status, tt.issue)
			}
		})
	}
}

func TestFieldExpectAndChecksum(t *testing.T) {
	tests := []struct {
		name  string
		field string
		data  []byte
		issue string
	}{
		{"equals holds", `{"name":"f","offset":0,"type":"u8","expect":{"equals":165}}`, []byte{0xA5}, ""},
		{"equals fails", `{"name":"f","offset":0,"type":"u8","expect":{"equals":165}}`, []byte{1}, "Ожидалось 165, получено 1"},
		{"fractional equals", `{"name":"f","offset":0,"type":"u8","expect":{"equals":1.5}}`, []byte{1}, "Ожидалось 1.5, получено 1"},
		{"below min", `{"name":"f","offset":0,"type":"i8","expect":{"min":10}}`, []byte{0xFB}, "Значение -5 меньше минимума 10"},
		{"above max", `{"name":"f","offset":0,"type":"u8","expect":{"max":2.5}}`, []byte{3}, "Значение 3 больше максимума 2.5"},
		{"not one of", `{"name":"f","offset":0,"type":"u8","expect":{"oneOf":[0,1]}}`, []byte{2}, "Значение 2 не входит в {0, 1}"},
		{"empty oneOf rejects everything", `{"name":"f","offset":0,"type":"u8","expect":{"oneOf":[]}}`, []byte{2}, "Значение 2 не входит в {}"},
		{"first failing condition wins", `{"name":"f","offset":0,"type":"u8","expect":{"min":5,"max":1}}`, []byte{3}, "Значение 3 меньше минимума 5"},
		{"expect overrides enum issue", `{"name":"f","offset":0,"type":"u8","enum":{"0":"a"},"expect":{"max":1}}`, []byte{3}, "Значение 3 больше максимума 1"},
		{"sum8 holds", `{"name":"f","offset":3,"type":"u8","checksum":"sum8"}`, []byte{0xA5, 0x01, 0x02, 0xA8}, ""},
		{"sum8 fails", `{"name":"f","offset":-1,"type":"u8","checksum":"sum8"}`, []byte{0xA5, 0x01, 0x02, 0x00}, "Контрольная сумма sum8: ожидалось 0xA8"},
		{"sum8 wraps", `{"name":"f","offset":2,"type":"u8","checksum":"sum8"}`, []byte{0xFF, 0x03, 0x02}, ""},
		{"xor8 holds", `{"name":"f","offset":3,"type":"u8","checksum":"xor8"}`, []byte{0xA5, 0x01, 0x02, 0xA6}, ""},
		{"xor8 fails with short hex", `{"name":"f","offset":2,"type":"u8","checksum":"xor8"}`, []byte{0x01, 0x04, 0x00}, "Контрольная сумма xor8: ожидалось 0x5"},
		{"unknown algorithm", `{"name":"f","offset":1,"type":"u8","checksum":"crc8"}`, []byte{1, 1}, `Неизвестный алгоритм контрольной суммы "crc8"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := parseOne(t, "["+tt.field+"]", tt.data)
			f := m.Fields[0]
			wantStatus := model.FieldOK
			if tt.issue != "" {
				wantStatus = model.FieldViolation
			}
			if f.Issue != tt.issue || f.Status != wantStatus {
				t.Errorf("issue %q status %s, want %q %s", f.Issue, f.Status, tt.issue, wantStatus)
			}
		})
	}
}

func TestFieldArrays(t *testing.T) {
	fields := `[
		{"name":"count","offset":0,"type":"u8"},
		{"name":"entries","offset":1,"type":"array","count":"count","itemSize":3,"items":[
			{"name":"id","offset":0,"type":"u8","enum":{"1":"уставка","2":"гистерезис"}},
			{"name":"value","offset":1,"type":"i16","scale":0.5},
			{"name":"tail","offset":-1,"type":"u8"}
		]},
		{"name":"sum","offset":-1,"type":"u8","checksum":"sum8"}
	]`
	data := []byte{2, 1, 0x2A, 0x00, 3, 0xFF, 0xFF, 0x00}
	data[len(data)-1] = func() byte {
		var s byte
		for _, b := range data[:len(data)-1] {
			s += b
		}
		return s
	}()

	m := parseOne(t, fields, data)
	arr := mustField(t, m, "entries")
	if arr.Type != "array" || arr.Value != int64(2) || arr.Display != "2 × 3 Б" || arr.Length != 6 || len(arr.Children) != 2 {
		t.Fatalf("array = %+v", arr)
	}
	item := arr.Children[1]
	if item.Path != "entries[1]" || item.Name != "[1]" || item.Type != "struct" || item.Offset != 4 || item.Length != 3 || item.Value != nil {
		t.Errorf("item = %+v", item)
	}
	if item.Status != model.FieldViolation {
		t.Errorf("item status = %s, want violation (enum)", item.Status)
	}
	id := item.Children[0]
	if id.Path != "entries[1].id" || id.Issue != "Значение 3 отсутствует в перечислении" {
		t.Errorf("child = %+v", id)
	}
	if v := arr.Children[0].Children[1]; v.Display != "21" || v.Path != "entries[0].value" {
		t.Errorf("scaled child = %+v", v)
	}
	// Negative offsets inside items count from the end of the message.
	if tail := item.Children[2]; tail.Offset != len(data)-1 {
		t.Errorf("tail offset = %d", tail.Offset)
	}
	if arr.Status != model.FieldViolation || m.Status != model.MsgViolation {
		t.Errorf("violation did not propagate: array %s message %s", arr.Status, m.Status)
	}
	if len(m.Unknown) != 0 {
		t.Errorf("unknown = %+v", m.Unknown)
	}

	t.Run("count too large", func(t *testing.T) {
		m := parseOne(t, fields, []byte{9, 1, 2, 3, 0})
		arr := mustField(t, m, "entries")
		if arr.Status != model.FieldViolation || arr.Issue != "Поле выходит за границы сообщения" || arr.Length != 27 || arr.Children != nil {
			t.Errorf("array = %+v", arr)
		}
	})
	t.Run("unresolved count gives an empty array", func(t *testing.T) {
		m := parseOne(t, `[{"name":"a","offset":0,"type":"array","count":"nope","itemSize":2,"items":[]}]`, []byte{1, 2})
		if f := m.Fields[0]; f.Value != int64(0) || f.Display != "0 × 2 Б" || f.Status != model.FieldOK || len(f.Children) != 0 {
			t.Errorf("array = %+v", f)
		}
	})
	t.Run("zero item size with items", func(t *testing.T) {
		m := parseOne(t, `[{"name":"n","offset":0,"type":"u8"},{"name":"a","offset":1,"type":"array","count":"n","itemSize":0,"items":[]}]`, []byte{200, 0})
		if f := m.Fields[1]; f.Status != model.FieldViolation || f.Issue != "Не задан размер элемента" || f.Children != nil {
			t.Errorf("array = %+v", f)
		}
	})
}

func TestFieldLengthReferences(t *testing.T) {
	fields := `[
		{"name":"n","offset":0,"type":"u8"},
		{"name":"name","offset":1,"type":"ascii","length":"n"},
		{"name":"missing","offset":1,"type":"bytes","length":"nope"},
		{"name":"neg","offset":4,"type":"i8"},
		{"name":"bad","offset":1,"type":"bytes","length":"neg"}
	]`
	m := parseOne(t, fields, []byte{3, 'a', 'b', 'c', 0xFF})
	if f := mustField(t, m, "name"); f.Value != "abc" || f.Length != 3 {
		t.Errorf("name = %+v", f)
	}
	if f := mustField(t, m, "missing"); f.Value != "" || f.Length != 0 || f.Status != model.FieldOK {
		t.Errorf("missing = %+v", f)
	}
	// A negative length cannot be satisfied by any message.
	if f := mustField(t, m, "bad"); f.Status != model.FieldViolation || f.Issue != issueOutOfBounds {
		t.Errorf("bad = %+v", f)
	}
}

func TestFieldsOverGapBytes(t *testing.T) {
	fields := `[
		{"name":"a","offset":0,"type":"u8"},
		{"name":"b","offset":1,"type":"u16"},
		{"name":"sum","offset":-1,"type":"u8","checksum":"sum8"}
	]`
	data := []byte{0x10, 0, 0, 0x99}
	m := parseOne(t, fields, data, model.StreamGap{Offset: 1, Length: 2, Reason: model.GapLost})
	if f := mustField(t, m, "b"); f.Status != model.FieldUnknown || f.Display != "??" || f.Value != nil || f.Issue != issueUnknown {
		t.Errorf("b = %+v", f)
	}
	// The checksum covers unknown bytes and cannot be verified.
	if f := mustField(t, m, "sum"); f.Status != model.FieldOK {
		t.Errorf("sum = %+v", f)
	}
	if m.Status != model.MsgPartial || len(m.Issues) != 1 || m.Issues[0] != "2 Б неизвестны (пропуск в записи)" {
		t.Errorf("message status %s issues %q", m.Status, m.Issues)
	}
}

func TestMeaningStatusAndPaths(t *testing.T) {
	fields := `[
		{"name":"a","offset":0,"type":"u8"},
		{"name":"b","offset":0,"type":"u8","meaning":"Маркер","hypothesis":"h-1"},
		{"name":"c","offset":0,"type":"u8","meaning":"Маркер","status":"confirmed"}
	]`
	m := parseOne(t, fields, []byte{1})
	want := map[string]string{"a": "structural", "b": "hypothesis", "c": "confirmed"}
	for name, status := range want {
		if f := mustField(t, m, name); f.MeaningStatus != status {
			t.Errorf("%s meaningStatus = %q, want %q", name, f.MeaningStatus, status)
		}
	}
	if f := mustField(t, m, "b"); f.HypothesisID != "h-1" || f.Meaning != "Маркер" {
		t.Errorf("b = %+v", f)
	}
}

func TestUncoveredBytes(t *testing.T) {
	m := parseOne(t, `[{"name":"a","offset":1,"type":"u8"},{"name":"b","offset":4,"type":"u16"}]`, []byte{0, 1, 2, 3, 4, 5, 6})
	want := []model.ByteRange{{Offset: 0, Length: 1}, {Offset: 2, Length: 2}, {Offset: 6, Length: 1}}
	if fmt.Sprint(m.Unknown) != fmt.Sprint(want) {
		t.Errorf("unknown = %v, want %v", m.Unknown, want)
	}
	if !strings.HasPrefix(*m.RuleID, "m") || m.RuleTitle != "Сообщение" {
		t.Errorf("rule = %v %q", m.RuleID, m.RuleTitle)
	}
}
