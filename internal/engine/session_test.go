package engine

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

func TestThermoLinkSession(t *testing.T) {
	s := thermoLink(t)
	req := tlFrame(0x03, 1, 1)
	resp := tlFrame(0x83, 1, 1, 0xE0, 0x00, 0x48, 0x02, 0x8C, 0x51, 0x01, 0x00)
	in := newInput("s1",
		StreamInput{Bytes: req, Segments: []model.StreamSegment{seg(0, len(req), 4, 10)}},
		// The response arrives in two segments.
		StreamInput{Bytes: resp, Segments: []model.StreamSegment{seg(0, 8, 6, 14), seg(8, len(resp)-8, 7, 15)}},
	)
	got := ParseSession(s, in)
	if len(got.Messages) != 2 || len(got.Unframed) != 0 {
		t.Fatalf("messages %d unframed %v", len(got.Messages), got.Unframed)
	}
	m := got.Messages[1]
	if *m.RuleID != "get_measurement_resp" || m.Status != model.MsgOK || m.Index != 1 || m.Direction != model.S2C {
		t.Fatalf("message = %+v", m)
	}
	if !reflect.DeepEqual(m.PacketNumbers, []int{6, 7}) || m.Timestamp != at(14) {
		t.Errorf("packets %v timestamp %s", m.PacketNumbers, m.Timestamp)
	}
	checksum := strconv.Itoa(int(resp[len(resp)-1]))
	for name, display := range map[string]string{"temperature": "22.4 °C", "humidity": "58.4 %", "uptime": "86412 с", "magic": "165", "checksum": checksum} {
		if f := mustField(t, m, name); f.Display != display {
			t.Errorf("%s = %q, want %q", name, f.Display, display)
		}
	}
	if f := mustField(t, m, "magic"); f.MeaningStatus != "confirmed" || f.HypothesisID != "h-01" {
		t.Errorf("magic = %+v", f)
	}
	if string(m.Data) != string(resp) || len(m.Unknown) != 0 || m.Issues == nil {
		t.Errorf("data %X unknown %v issues %v", m.Data, m.Unknown, m.Issues)
	}
}

func TestRuleMatching(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":2},"messages":[
		{"id":"any","match":[{"offset":0,"hex":"A5"}],"fields":[{"name":"a","offset":0,"type":"u8"}]},
		{"id":"req","direction":"c2s","match":[{"offset":0,"hex":"A5"},{"offset":1,"hex":"01"}],"fields":[]},
		{"id":"zero","match":[{"offset":0,"hex":"00"}],"fields":[]},
		{"id":"far","match":[{"offset":1,"hex":"02 03"}],"fields":[]}
	]}`)
	c2s := []byte{0xA5, 0x01, 0xA5, 0x02, 0x00, 0x00, 0x7E, 0x02}
	s2c := []byte{0xA5, 0x01}
	in := newInput("s",
		StreamInput{Bytes: c2s, Gaps: []model.StreamGap{{Offset: 4, Length: 1}}, Segments: []model.StreamSegment{seg(0, 4, 1, 1), seg(5, 3, 3, 3)}},
		oneSegment(s2c, 2, 2),
	)
	got := ParseSession(s, in).Messages
	if len(got) != 5 {
		t.Fatalf("got %d messages", len(got))
	}
	byOffset := func(dir model.Direction, off int) model.Message {
		for _, m := range got {
			if m.Direction == dir && m.Offset == off {
				return m
			}
		}
		t.Fatalf("no %s message at %d", dir, off)
		return model.Message{}
	}

	amb := byOffset(model.C2S, 0)
	if amb.Status != model.MsgAmbiguous || *amb.RuleID != "any" || !reflect.DeepEqual(amb.CandidateRuleIDs, []string{"any", "req"}) {
		t.Errorf("ambiguous message = %+v", amb)
	}
	if want := []string{"Подходят правила: any, req"}; !reflect.DeepEqual(amb.Issues, want) {
		t.Errorf("issues = %q", amb.Issues)
	}
	if len(amb.Fields) != 1 {
		t.Errorf("ambiguous message must be parsed by the first rule, got %d fields", len(amb.Fields))
	}
	// The direction filter excludes "req" on the server side.
	if m := byOffset(model.S2C, 0); m.Status != model.MsgOK || *m.RuleID != "any" || m.CandidateRuleIDs != nil {
		t.Errorf("s2c message = %+v", m)
	}
	// Zero-filled gap bytes must not satisfy "00", so no rule claims the message.
	if m := byOffset(model.C2S, 4); m.RuleID != nil || m.Status != model.MsgUnknownType || len(m.Fields) != 0 {
		t.Errorf("gap message = %+v", m)
	}
	// A match condition reaching past the message never holds.
	if m := byOffset(model.C2S, 6); m.RuleID != nil || !reflect.DeepEqual(m.Unknown, []model.ByteRange{{Offset: 0, Length: 2}}) {
		t.Errorf("unmatched message = %+v", m)
	}
}

func TestMessagesAreOrderedByTime(t *testing.T) {
	s := thermoLink(t)
	a, b, c := tlFrame(0x01, 1), tlFrame(0x01, 2), tlFrame(0x01, 3)
	c2s := concat(a, b, c)
	in := newInput("s",
		// Segment 2 (message b) was captured before segment 1 (reordering).
		StreamInput{Bytes: c2s, Segments: []model.StreamSegment{seg(0, 6, 1, 10), seg(6, 6, 3, 30), seg(12, 6, 2, 5)}},
		StreamInput{Bytes: tlFrame(0x81, 1, 0), Segments: []model.StreamSegment{seg(0, 7, 4, 20)}},
	)
	got := ParseSession(s, in).Messages
	var order []string
	for i, m := range got {
		if m.Index != i {
			t.Errorf("message %d has index %d", i, m.Index)
		}
		order = append(order, string(m.Direction)+":"+m.Timestamp[17:23])
	}
	want := []string{"c2s:00.005", "c2s:00.010", "s2c:00.020", "c2s:00.030"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v", order, want)
	}
}

func TestTiesKeepClientFirst(t *testing.T) {
	s := thermoLink(t)
	in := newInput("s", oneSegment(tlFrame(0x01, 1), 1, 7), oneSegment(tlFrame(0x81, 1, 0), 2, 7))
	got := ParseSession(s, in).Messages
	if got[0].Direction != model.C2S || got[1].Direction != model.S2C {
		t.Errorf("order = %s, %s", got[0].Direction, got[1].Direction)
	}
}

func TestTimestampFallsBackToLastSegment(t *testing.T) {
	s := thermoLink(t)
	msg := concat(tlFrame(0x01, 1), tlFrame(0x01, 2))
	// The first message's segment is lost; only the second one was captured.
	in := newInput("s", StreamInput{Bytes: msg, Segments: []model.StreamSegment{seg(6, 6, 9, 50)}}, StreamInput{})
	got := ParseSession(s, in).Messages
	if len(got) != 2 {
		t.Fatalf("got %d messages", len(got))
	}
	for _, m := range got {
		if m.Timestamp != at(50) {
			t.Errorf("message at %d: timestamp %s", m.Offset, m.Timestamp)
		}
	}
	if got[0].Offset != 0 || len(got[0].PacketNumbers) != 0 || !reflect.DeepEqual(got[1].PacketNumbers, []int{9}) {
		t.Errorf("packets = %v / %v", got[0].PacketNumbers, got[1].PacketNumbers)
	}
}

func TestMessageDataIsCopied(t *testing.T) {
	s := thermoLink(t)
	data := tlFrame(0x01, 1)
	got := ParseSession(s, newInput("s", oneSegment(data, 1, 0), StreamInput{}))
	data[0] = 0
	if got.Messages[0].Data[0] != 0xA5 {
		t.Error("message data aliases the input stream")
	}
}

func TestEmptySessionAndNilSpec(t *testing.T) {
	got := ParseSession(thermoLink(t), newInput("s", StreamInput{}, StreamInput{}))
	if got.Messages == nil || len(got.Messages) != 0 || got.Unframed == nil {
		t.Errorf("empty session = %+v", got)
	}
	data := []byte{1, 2, 3}
	got = ParseSession(nil, newInput("s", oneSegment(data, 1, 0), StreamInput{}))
	if len(got.Messages) != 0 || len(got.Unframed) != 1 || got.Unframed[0] != (Unframed{Direction: model.C2S, Offset: 0, Length: 3}) {
		t.Errorf("nil spec = %+v", got)
	}
}

func TestAssertions(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":4},
		"messages":[{"id":"set","match":[{"offset":0,"hex":"02"}],"fields":[
			{"name":"type","offset":0,"type":"u8"},
			{"name":"param","offset":1,"type":"u8","hypothesis":"h-own"},
			{"name":"value","offset":2,"type":"i16"}
		]},{"id":"meas","match":[{"offset":0,"hex":"83"}],"fields":[
			{"name":"type","offset":0,"type":"u8"},
			{"name":"temp","offset":1,"type":"i16","scale":0.1},
			{"name":"tail","offset":3,"type":"u8"}
		]}],
		"assertions":[
			{"id":"a-set","rule":"set","actionKind":"set_param","hypothesis":"h-04","checks":[
				{"field":"param","equals":"params.paramId"},
				{"field":"value","equals":"params.value"},
				{"field":"nope","equals":"params.value"},
				{"field":"type","equals":"params.missing"}
			]},
			{"id":"a-meas","rule":"meas","actionKind":"measure","checks":[{"field":"temp","equals":"observed.temperature"}]}
		]}`)
	setMsg := []byte{0x02, 0x01, 0x15, 0x00}  // param 1, value 21
	measMsg := []byte{0x83, 0xE0, 0x00, 0x00} // temperature 22.4

	action := func(kind, label string, params, observed map[string]any) model.ActionEntry {
		return model.ActionEntry{Kind: kind, Label: label, Params: params, Observed: observed}
	}
	tests := []struct {
		name    string
		msg     []byte
		actions []model.ActionEntry
		field   string
		issue   string
		hyp     string
	}{
		{"numbers match", setMsg, []model.ActionEntry{action("set_param", "Задать 21", map[string]any{"paramId": 1.0, "value": 21.0}, nil)}, "value", "", ""},
		{"numeric strings match", setMsg, []model.ActionEntry{action("set_param", "Задать 21", map[string]any{"paramId": " 1 ", "value": "21"}, nil)}, "value", "", ""},
		{"hex string matches", setMsg, []model.ActionEntry{action("set_param", "Задать 21", map[string]any{"paramId": "0x1", "value": "0x15"}, nil)}, "value", "", ""},
		{"mismatch", setMsg, []model.ActionEntry{action("set_param", "Задать 22", map[string]any{"paramId": 1.0, "value": 22.0}, nil)},
			"value", "21 ≠ params.value = 22 (действие «Задать 22», проверка a-set)", "h-04"},
		{"own hypothesis is kept", setMsg, []model.ActionEntry{action("set_param", "Задать", map[string]any{"paramId": "2", "value": 21}, nil)},
			"param", "1 ≠ params.paramId = 2 (действие «Задать», проверка a-set)", "h-own"},
		{"first action of the kind wins", setMsg, []model.ActionEntry{
			action("other", "Чужое", map[string]any{"value": 99.0}, nil),
			action("set_param", "Первое", map[string]any{"value": 21.0}, nil),
			action("set_param", "Второе", map[string]any{"value": 5.0}, nil),
		}, "value", "", ""},
		{"non-numeric condition is skipped", setMsg, []model.ActionEntry{action("set_param", "x", map[string]any{"value": "двадцать"}, nil)}, "value", "", ""},
		{"empty string is not zero", setMsg, []model.ActionEntry{action("set_param", "x", map[string]any{"value": ""}, nil)}, "value", "", ""},
		{"scaled value within tolerance", measMsg, []model.ActionEntry{action("measure", "Измерить", nil, map[string]any{"temperature": 22.4})}, "temp", "", ""},
		{"scaled value mismatch", measMsg, []model.ActionEntry{action("measure", "Измерить", nil, map[string]any{"temperature": "22.5"})},
			"temp", "22.4 ≠ observed.temperature = 22.5 (действие «Измерить», проверка a-meas)", ""},
		{"no linked action", measMsg, nil, "temp", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseSession(s, newInput("s", oneSegment(tt.msg, 1, 0), StreamInput{}, tt.actions...))
			m := got.Messages[0]
			f := mustField(t, m, tt.field)
			wantStatus, wantMsg := model.FieldOK, model.MsgOK
			if tt.issue != "" {
				wantStatus, wantMsg = model.FieldViolation, model.MsgViolation
			}
			if f.Issue != tt.issue || f.Status != wantStatus || m.Status != wantMsg {
				t.Errorf("field %+v message %s", f, m.Status)
			}
			if tt.hyp != "" && f.HypothesisID != tt.hyp {
				t.Errorf("hypothesis = %q, want %q", f.HypothesisID, tt.hyp)
			}
		})
	}
}

func TestAssertionSkipsUnknownField(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":2},
		"messages":[{"id":"m","match":[],"fields":[{"name":"v","offset":1,"type":"u8"}]}],
		"assertions":[{"id":"a","rule":"m","actionKind":"k","checks":[{"field":"v","equals":"params.v"}]}]}`)
	in := newInput("s",
		StreamInput{Bytes: []byte{1, 0}, Gaps: []model.StreamGap{{Offset: 1, Length: 1}}, Segments: []model.StreamSegment{seg(0, 1, 1, 0)}},
		StreamInput{},
		model.ActionEntry{Kind: "k", Label: "x", Params: map[string]any{"v": 5}})
	m := ParseSession(s, in).Messages[0]
	if f := m.Fields[0]; f.Status != model.FieldUnknown || strings.Contains(f.Issue, "≠") {
		t.Errorf("field = %+v", f)
	}
}

func TestStatusPrecedence(t *testing.T) {
	// Partial wins over ambiguous and violation; unknown type wins over partial.
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":3},"messages":[
		{"id":"a","match":[{"offset":0,"hex":"01"}],"fields":[{"name":"x","offset":0,"type":"u8","expect":{"equals":9}}]},
		{"id":"b","match":[{"offset":0,"hex":"01"}],"fields":[]}
	]}`)
	data := []byte{0x01, 0, 0, 0x02, 0, 0}
	in := newInput("s", StreamInput{Bytes: data, Gaps: []model.StreamGap{{Offset: 2, Length: 2}}, Segments: []model.StreamSegment{seg(0, 6, 1, 0)}}, StreamInput{})
	got := ParseSession(s, in).Messages
	if got[0].Status != model.MsgPartial || got[1].Status != model.MsgUnknownType {
		t.Errorf("statuses = %s, %s", got[0].Status, got[1].Status)
	}
	wantIssues := []string{"1 Б неизвестны (пропуск в записи)", "Подходят правила: a, b"}
	if !reflect.DeepEqual(got[0].Issues, wantIssues) {
		t.Errorf("issues = %q", got[0].Issues)
	}
}

// MaxMessages bounds what ParseSession builds: a session with more messages
// is reported as TooMany instead of being built (and framing stops recording
// frames), so a framing that cuts a long stream into bytes cannot exhaust memory.
func TestParseSessionMaxMessages(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":1},"messages":[]}`)
	in := newInput("s", oneSegment(make([]byte, 10), 1, 1), oneSegment(make([]byte, 5), 2, 2))
	for _, tc := range []struct {
		max, messages int
		tooMany       bool
	}{{0, 15, false}, {15, 15, false}, {14, 0, true}, {1, 0, true}} {
		in.MaxMessages = tc.max
		got := ParseSession(s, in)
		if len(got.Messages) != tc.messages || got.TooMany != tc.tooMany || got.Messages == nil || got.Unframed == nil {
			t.Errorf("max %d: %d messages, tooMany %v", tc.max, len(got.Messages), got.TooMany)
		}
	}
	// Framing keeps at most max+1 frames per direction.
	c := compile(s)
	in.MaxMessages = 3
	sess := c.newSession(&in, true, in.MaxMessages+1)
	if n := len(sess.streams[0].frames); n != 4 {
		t.Errorf("frames recorded past the cap: %d", n)
	}
}
