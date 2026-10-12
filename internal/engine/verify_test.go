package engine

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

// thermoCorpus builds three sessions: a clean one, one with garbage before the
// first message, an undescribed message type and a rule violation, and one
// with a lost segment and a conflicting retransmission.
func thermoCorpus() []SessionInput {
	reqA, measReqA := tlFrame(0x01, 1), tlFrame(0x03, 2, 1)
	respA := tlFrame(0x81, 1, 2, 1, 0x15, 0x00, 5, 0x2C, 0x01)
	measRespA := tlFrame(0x83, 2, 1, 0xE0, 0, 0x48, 2, 0x8C, 0x51, 1, 0)
	a := newInput("a",
		StreamInput{Bytes: concat(reqA, measReqA), Segments: []model.StreamSegment{seg(0, 6, 1, 0), seg(6, 7, 3, 20)}},
		StreamInput{Bytes: concat(respA, measRespA), Segments: []model.StreamSegment{seg(0, 13, 2, 10), seg(13, 15, 4, 30)}},
	)

	garbage := []byte{0x01, 0x02, 0x03, 0xFF}
	setReq := tlFrame(0x02, 1, 1, 0x15, 0x00)
	unk, setResp := tlFrame(0x55, 1), tlFrame(0x82, 1, 5, 1)
	b := newInput("b",
		StreamInput{Bytes: concat(garbage, setReq), Segments: []model.StreamSegment{seg(0, 4, 1, 0), seg(4, 9, 2, 1)}},
		StreamInput{Bytes: concat(unk, setResp), Segments: []model.StreamSegment{seg(0, 6, 3, 2), seg(6, 8, 4, 3)}},
		model.ActionEntry{ID: "act-1", Kind: "set_param", Label: "Задать 21", Params: map[string]any{"paramId": 1.0, "value": "21"}},
	)

	measReqC := tlFrame(0x03, 1, 1)
	measRespC := tlFrame(0x83, 1, 1, 0xE0, 0, 0x48, 2, 0x8C, 0x51, 1, 0)
	for i := 6; i < 9; i++ {
		measRespC[i] = 0 // gap bytes are zero-filled by reassembly
	}
	c := newInput("c",
		StreamInput{Bytes: measReqC, Segments: []model.StreamSegment{seg(0, 7, 1, 0)}},
		StreamInput{
			Bytes:    measRespC,
			Gaps:     []model.StreamGap{{Offset: 6, Length: 3, Reason: model.GapLost}},
			Segments: []model.StreamSegment{seg(0, 6, 2, 5), seg(9, 6, 3, 6)},
		},
		model.ActionEntry{ID: "act-2", Kind: "get_measurement", Label: "Измерить", Observed: map[string]any{"temperature": 99.0}},
	)
	c.Session.Flags.OverlapConflicts = 1
	return []SessionInput{a, b, c}
}

func TestVerifySummaryAndFindings(t *testing.T) {
	res, err := Verify(context.Background(), thermoLink(t), thermoCorpus(), nil)
	if err != nil {
		t.Fatal(err)
	}
	wantSummary := model.RunSummary{
		Sessions:           3,
		SessionsWithErrors: 2,
		Messages:           9,
		OK:                 6,
		Violations:         1,
		Partial:            1,
		UnknownType:        1,
		UnknownBytes:       4 + 6,
		TotalBytes:         41 + 23 + 22,
		InapplicableRules:  []string{"read_log_req", "read_log_resp", "heartbeat", "heartbeat_ack"},
		ByRule: []model.RuleStat{
			{RuleID: "read_params_req", Matched: 1},
			{RuleID: "read_params_resp", Matched: 1},
			{RuleID: "set_param_req", Matched: 1},
			{RuleID: "set_param_resp", Matched: 1, Violations: 1},
			{RuleID: "get_measurement_req", Matched: 2},
			{RuleID: "get_measurement_resp", Matched: 2},
			{RuleID: "read_log_req"},
			{RuleID: "read_log_resp"},
			{RuleID: "heartbeat"},
			{RuleID: "heartbeat_ack"},
		},
	}
	if !reflect.DeepEqual(res.Summary, wantSummary) {
		t.Errorf("summary =\n%+v\nwant\n%+v", res.Summary, wantSummary)
	}

	type brief struct {
		kind, severity, ruleID, fieldPath, message string
		occurrences, examples                      int
	}
	want := []brief{
		{"violation", "error", "set_param_resp", "status", "Значение 5 не входит в {0, 1}", 1, 1},
		{"partial", "warning", "get_measurement_resp", "", "3 Б неизвестны (пропуск в записи)", 1, 1},
		{"capture_defect", "warning", "", "", "Потерянные сегменты — участки потока неизвестны", 1, 1},
		{"capture_defect", "warning", "", "", "Повторные передачи с разными данными", 1, 1},
		{"unknown_type", "info", "", "", "S→C: сообщения «A5 55 …» не описаны ни одним правилом", 1, 1},
		{"unknown_region", "warning", "", "", "Байты вне сообщений: синхронизация кадров потеряна (начало не захвачено или пропуск)", 1, 1},
		{"inapplicable_rule", "info", "read_log_req", "", "Правило «read_log_req» не сработало ни на одном сообщении корпуса — его корректность не проверена", 1, 0},
		{"inapplicable_rule", "info", "read_log_resp", "", "Правило «read_log_resp» не сработало ни на одном сообщении корпуса — его корректность не проверена", 1, 0},
		{"inapplicable_rule", "info", "heartbeat", "", "Правило «heartbeat» не сработало ни на одном сообщении корпуса — его корректность не проверена", 1, 0},
		{"inapplicable_rule", "info", "heartbeat_ack", "", "Правило «heartbeat_ack» не сработало ни на одном сообщении корпуса — его корректность не проверена", 1, 0},
	}
	var got []brief
	for _, f := range res.Findings {
		got = append(got, brief{f.Kind, f.Severity, f.RuleID, f.FieldPath, f.Message, f.Occurrences, len(f.Examples)})
		if f.ID != "" || f.RunID != "" || f.Examples == nil {
			t.Errorf("finding %+v: ids must be empty and examples non-nil", f)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings =\n%+v\nwant\n%+v", got, want)
	}

	examples := map[int]model.SourceRef{
		0: {SessionID: "b", Direction: model.S2C, Offset: 11, Length: 1, PacketNumbers: []int{4}},
		1: {SessionID: "c", Direction: model.S2C, Offset: 0, Length: 15, PacketNumbers: []int{2, 3}},
		2: {SessionID: "c", Direction: model.S2C, Offset: 6, Length: 3, PacketNumbers: []int{}},
		3: {SessionID: "c", Direction: model.C2S, Offset: 0, Length: 1, PacketNumbers: []int{1}},
		4: {SessionID: "b", Direction: model.S2C, Offset: 0, Length: 6, PacketNumbers: []int{3}},
		5: {SessionID: "b", Direction: model.C2S, Offset: 0, Length: 4, PacketNumbers: []int{1}},
	}
	for i, w := range examples {
		w.CaptureID, w.CaptureSHA256 = "cap-1", "sha-1"
		if g := res.Findings[i].Examples[0]; !reflect.DeepEqual(g, w) {
			t.Errorf("finding %d example = %+v, want %+v", i, g, w)
		}
	}
}

func TestVerifyGroupsArrayItemsAndCapsExamples(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"fixed","size":4},"messages":[
		{"id":"r","match":[],"fields":[
			{"name":"n","offset":0,"type":"u8"},
			{"name":"items","offset":1,"type":"array","count":"n","itemSize":1,"items":[
				{"name":"v","offset":0,"type":"u8","expect":{"max":9}}
			]}
		]}]}`)
	// Seven messages, each with two items out of range.
	var data []byte
	for range 7 {
		data = append(data, 3, 1, 20, 30)
	}
	res, err := Verify(context.Background(), s, []SessionInput{newInput("s", oneSegment(data, 1, 0), StreamInput{})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ path, message string }
	got := map[key]int{}
	for _, f := range res.Findings {
		if f.Kind != model.FindingViolation {
			continue
		}
		got[key{f.FieldPath, f.Message}] = f.Occurrences
		if len(f.Examples) != maxExamples {
			t.Errorf("%s: %d examples, want %d", f.FieldPath, len(f.Examples), maxExamples)
		}
	}
	want := map[key]int{
		{"items[].v", "Значение 20 больше максимума 9"}: 14, // grouped by path and check, message of the first
		{"items[]", "Нарушение правила"}:                14, // the items themselves
		{"items", "Нарушение правила"}:                  7,  // the array
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("violations = %v, want %v", got, want)
	}
	if res.Findings[0].Occurrences < res.Findings[2].Occurrences {
		t.Error("findings of one kind must be ordered by occurrences")
	}
	if res.Summary.Violations != 7 || res.Summary.ByRule[0].Violations != 7 {
		t.Errorf("summary = %+v", res.Summary)
	}
}

func TestVerifyUnknownRegions(t *testing.T) {
	s := mustSpec(t, `{"format":"protolab/v1","framing":{"kind":"delimiter","delimiter":"7E"},"messages":[
		{"id":"r","match":[{"offset":0,"hex":"01"}],"fields":[{"name":"a","offset":0,"type":"u8"},{"name":"b","offset":4,"type":"u8"}]}
	]}`)
	// "01 x x x b 7E": bytes 1–3 and the trailing delimiter are not described.
	data := []byte{1, 0, 0, 0, 2, 0x7E, 1, 0, 0, 0, 2, 9, 0x7E}
	res, err := Verify(context.Background(), s, []SessionInput{newInput("s", oneSegment(data, 1, 0), StreamInput{})}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, f := range res.Findings {
		if f.Kind == model.FindingUnknownRegion {
			messages = append(messages, f.Message)
		}
	}
	want := []string{"r: байты 1–3 не описаны полями", "r: последний байт не описаны полями", "r: байты 5–6 не описаны полями"}
	if !reflect.DeepEqual(messages, want) {
		t.Errorf("unknown regions = %q, want %q", messages, want)
	}
	if res.Summary.UnknownBytes != 3+1+3+2 {
		t.Errorf("unknownBytes = %d", res.Summary.UnknownBytes)
	}
}

func TestVerifyRecoversFromPanics(t *testing.T) {
	testHookSession = func(in SessionInput) {
		switch in.Session.ID {
		case "b":
			panic("boom")
		case "c":
			panic(errors.New("index out of range"))
		}
	}
	defer func() { testHookSession = nil }()

	res, err := Verify(context.Background(), thermoLink(t), thermoCorpus(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Sessions != 3 || res.Summary.SessionsWithErrors != 2 || res.Summary.Messages != 4 || res.Summary.OK != 4 {
		t.Errorf("summary = %+v", res.Summary)
	}
	var failures []string
	for _, f := range res.Findings {
		if f.Kind == model.FindingCaptureDefect && f.Severity == model.LevelError {
			failures = append(failures, f.Message)
		}
	}
	want := []string{"Ошибка обработки сессии: boom", "Ошибка обработки сессии: index out of range"}
	if !reflect.DeepEqual(failures, want) {
		t.Errorf("failures = %q", failures)
	}
}

func TestVerifyCancellation(t *testing.T) {
	inputs := thermoCorpus()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	if _, err := Verify(ctx, thermoLink(t), inputs, func(int, int) { calls++ }); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Errorf("cancelled before start: err %v, progress calls %d", err, calls)
	}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	var seen [][2]int
	_, err := Verify(ctx, thermoLink(t), inputs, func(done, total int) {
		seen = append(seen, [2]int{done, total})
		cancel()
	})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(seen, [][2]int{{1, 3}}) {
		t.Errorf("cancelled after the first session: err %v, progress %v", err, seen)
	}
}

func TestVerifyProgressAndEmptyCorpus(t *testing.T) {
	var seen [][2]int
	_, err := Verify(context.Background(), thermoLink(t), thermoCorpus(), func(done, total int) {
		seen = append(seen, [2]int{done, total})
	})
	if err != nil || !reflect.DeepEqual(seen, [][2]int{{1, 3}, {2, 3}, {3, 3}}) {
		t.Errorf("progress = %v, err %v", seen, err)
	}

	res, err := Verify(context.Background(), thermoLink(t), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Summary.Sessions != 0 || len(res.Summary.InapplicableRules) != 10 || len(res.Findings) != 10 {
		t.Errorf("empty corpus = %+v", res)
	}
	res, err = Verify(context.Background(), nil, nil, nil)
	if err != nil || res.Findings == nil || res.Summary.InapplicableRules == nil || res.Summary.ByRule == nil {
		t.Errorf("nil spec = %+v, %v", res, err)
	}
}
