package demo

import (
	"fmt"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// The research history of ThermoLink, a port of the frontend fixtures
// (frontend/src/mocks/fixtures/research.ts and db.ts). Version 1 assumes that a
// setting is one unsigned byte and that the temperature is unsigned; the
// second day's capture (interval 600, calibration −15, −5.5 °C outdoors)
// refutes both, version 2 switches to i16 and describes the rest of the
// protocol, version 3 confirms what the extended corpus supports.

// at parses an RFC 3339 time of the research history into model.EntityTime.
func at(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	must(err)
	return model.EntityTime(t)
}

func ptr[T any](v T) *T { return &v }

// specBuilder writes the specs with project hypothesis ids.
type specBuilder struct{ hyp func(seed string) string }

func (b specBuilder) header(seqStatus string) []spec.FieldRule {
	return []spec.FieldRule{
		{Name: "magic", Offset: 0, Type: "u8", Expect: &spec.Expect{Equals: ptr[float64](frameMagic)}, Meaning: "Маркер начала кадра", Status: spec.MeaningConfirmed, Hypothesis: b.hyp("h-01")},
		{Name: "type", Offset: 1, Type: "u8", Meaning: "Код типа сообщения", Status: spec.MeaningStructural},
		{Name: "seq", Offset: 2, Type: "u8", Meaning: "Номер запроса (ответ повторяет номер)", Status: seqStatus, Hypothesis: b.hyp("h-07")},
		{Name: "length", Offset: 3, Type: "u16", Endian: "le", Meaning: "Длина полезной нагрузки, Б", Status: spec.MeaningConfirmed, Hypothesis: b.hyp("h-02")},
	}
}

func (b specBuilder) checksum() spec.FieldRule {
	return spec.FieldRule{
		Name: "checksum", Offset: -1, Type: "u8", Checksum: "sum8",
		Meaning: "Контрольная сумма sum8 всех предыдущих байтов", Status: spec.MeaningHypothesis, Hypothesis: b.hyp("h-05"),
	}
}

// fields appends the fields of a message to a copy of the header.
func fields(header []spec.FieldRule, rest ...spec.FieldRule) []spec.FieldRule {
	return append(append([]spec.FieldRule{}, header...), rest...)
}

func match(typ byte) []spec.MatchCondition {
	return []spec.MatchCondition{{Offset: 0, Hex: "A5"}, {Offset: 1, Hex: fmt.Sprintf("%02X", typ)}}
}

func framing() spec.Framing {
	return spec.Framing{
		Kind:         spec.FramingLengthPrefixed,
		LengthField:  &spec.LengthField{Offset: 3, Size: 2, Endian: "le"},
		LengthAdjust: ptr(6),
		MinLength:    ptr(6),
		MaxLength:    ptr(4096),
		Sync:         "A5",
	}
}

// The enums are built per spec: versions and datasets share no maps.

func statusEnum() map[string]string {
	return map[string]string{"0": "OK", "1": "вне диапазона"}
}

func paramEnum() map[string]string {
	return map[string]string{
		"1": "уставка",
		"2": "гистерезис",
		"3": "режим",
		"4": "вентилятор",
		"5": "интервал",
		"6": "калибровка",
	}
}

// v1 frames messages by length and describes the requests of the action log.
func (b specBuilder) v1() spec.Spec {
	h := b.header(spec.MeaningHypothesis)
	return spec.Spec{
		Format:    spec.Format,
		Protocol:  "ThermoLink",
		Transport: &spec.Transport{Port: devicePort},
		Framing:   framing(),
		Messages: []spec.MessageRule{
			{ID: "read_params_req", Title: "Чтение параметров — запрос", Direction: "c2s", Match: match(typeReadParam), Fields: fields(h)},
			{
				ID: "read_params_resp", Title: "Чтение параметров — ответ", Direction: "s2c", Match: match(typeReadParam | responseFlag),
				Fields: fields(h,
					spec.FieldRule{Name: "count", Offset: 5, Type: "u8", Meaning: "Количество параметров", Status: spec.MeaningHypothesis},
					spec.FieldRule{Name: "entries", Offset: 6, Type: "array", Count: "count", ItemSize: 3, Items: []spec.FieldRule{
						{Name: "param_id", Offset: 0, Type: "u8", Meaning: "Идентификатор параметра", Status: spec.MeaningHypothesis},
						{Name: "value", Offset: 1, Type: "u16", Endian: "le", Meaning: "Значение параметра", Status: spec.MeaningHypothesis},
					}},
				),
			},
			{
				ID: "set_param_req", Title: "Изменение настройки — запрос", Direction: "c2s", Match: match(typeSetParam), Hypothesis: b.hyp("h-03"),
				Fields: fields(h,
					spec.FieldRule{Name: "param_id", Offset: 5, Type: "u8", Meaning: "Идентификатор параметра", Status: spec.MeaningHypothesis},
					// The wrong assumption of the first day: one unsigned byte.
					spec.FieldRule{Name: "value", Offset: 6, Type: "u8", Meaning: "Новое значение", Status: spec.MeaningHypothesis, Hypothesis: b.hyp("h-03")},
				),
			},
			{
				ID: "set_param_resp", Title: "Изменение настройки — ответ", Direction: "s2c", Match: match(typeSetParam | responseFlag),
				Fields: fields(h,
					spec.FieldRule{Name: "status", Offset: 5, Type: "u8", Enum: statusEnum(), Meaning: "Результат", Status: spec.MeaningHypothesis},
					spec.FieldRule{Name: "param_id", Offset: 6, Type: "u8", Meaning: "Идентификатор параметра", Status: spec.MeaningHypothesis},
				),
			},
			{
				ID: "get_measurement_req", Title: "Измерение — запрос", Direction: "c2s", Match: match(typeMeasure),
				Fields: fields(h, spec.FieldRule{Name: "channel", Offset: 5, Type: "u8", Meaning: "Номер канала", Status: spec.MeaningHypothesis}),
			},
			{
				ID: "get_measurement_resp", Title: "Измерение — ответ", Direction: "s2c", Match: match(typeMeasure | responseFlag),
				Fields: fields(h,
					spec.FieldRule{Name: "channel", Offset: 5, Type: "u8", Meaning: "Номер канала", Status: spec.MeaningHypothesis},
					// Unsigned: no negative temperature was seen on the first day.
					spec.FieldRule{Name: "temperature", Offset: 6, Type: "u16", Endian: "le", Scale: 0.1, Unit: "°C", Meaning: "Температура", Status: spec.MeaningHypothesis, Hypothesis: b.hyp("h-06")},
					spec.FieldRule{Name: "humidity", Offset: 8, Type: "u16", Endian: "le", Scale: 0.1, Unit: "%", Meaning: "Влажность", Status: spec.MeaningHypothesis},
				),
			},
		},
		Assertions: []spec.Assertion{
			{
				ID: "a-set-value", Rule: "set_param_req", ActionKind: "set_param", Hypothesis: b.hyp("h-03"),
				Checks: []spec.Check{{Field: "param_id", Equals: "params.paramId"}, {Field: "value", Equals: "params.value"}},
			},
			{
				ID: "a-measurement", Rule: "get_measurement_resp", ActionKind: "get_measurement", Hypothesis: b.hyp("h-06"),
				Checks: []spec.Check{{Field: "temperature", Equals: "observed.temperature"}, {Field: "humidity", Equals: "observed.humidity"}},
			},
		},
	}
}

// refined are the messages of v2 and v3: signed 16-bit values, the checksum,
// the journal and the heartbeat. v3 (confirmed) also confirms the sequence
// number, enumerates the parameters and expects a known response status.
func (b specBuilder) refined(confirmed bool) []spec.MessageRule {
	seqStatus, paramStatus := spec.MeaningHypothesis, spec.MeaningHypothesis
	var params map[string]string
	var status *spec.Expect
	if confirmed {
		seqStatus, paramStatus = spec.MeaningConfirmed, spec.MeaningConfirmed
		params = paramEnum()
		status = &spec.Expect{OneOf: []float64{0, 1}}
	}
	h := b.header(seqStatus)
	sum := b.checksum()
	paramID := func(offset int) spec.FieldRule {
		return spec.FieldRule{Name: "param_id", Offset: offset, Type: "u8", Enum: params, Meaning: "Идентификатор параметра", Status: paramStatus}
	}
	return []spec.MessageRule{
		{ID: "read_params_req", Title: "Чтение параметров — запрос", Direction: "c2s", Match: match(typeReadParam), Fields: fields(h, sum)},
		{
			ID: "read_params_resp", Title: "Чтение параметров — ответ", Direction: "s2c", Match: match(typeReadParam | responseFlag),
			Fields: fields(h,
				spec.FieldRule{Name: "count", Offset: 5, Type: "u8", Meaning: "Количество параметров", Status: spec.MeaningConfirmed},
				spec.FieldRule{Name: "entries", Offset: 6, Type: "array", Count: "count", ItemSize: 3, Items: []spec.FieldRule{
					paramID(0),
					{Name: "value", Offset: 1, Type: "i16", Endian: "le", Meaning: "Значение параметра", Status: spec.MeaningConfirmed, Hypothesis: b.hyp("h-04")},
				}},
				sum,
			),
		},
		{
			ID: "set_param_req", Title: "Изменение настройки — запрос", Direction: "c2s", Match: match(typeSetParam), Hypothesis: b.hyp("h-04"),
			Fields: fields(h,
				paramID(5),
				spec.FieldRule{Name: "value", Offset: 6, Type: "i16", Endian: "le", Meaning: "Новое значение", Status: spec.MeaningConfirmed, Hypothesis: b.hyp("h-04")},
				sum,
			),
		},
		{
			ID: "set_param_resp", Title: "Изменение настройки — ответ", Direction: "s2c", Match: match(typeSetParam | responseFlag),
			Fields: fields(h,
				spec.FieldRule{Name: "status", Offset: 5, Type: "u8", Enum: statusEnum(), Expect: status, Meaning: "Результат", Status: spec.MeaningHypothesis},
				paramID(6),
				sum,
			),
		},
		{
			ID: "get_measurement_req", Title: "Измерение — запрос", Direction: "c2s", Match: match(typeMeasure),
			Fields: fields(h, spec.FieldRule{Name: "channel", Offset: 5, Type: "u8", Meaning: "Номер канала", Status: spec.MeaningConfirmed}, sum),
		},
		{
			ID: "get_measurement_resp", Title: "Измерение — ответ", Direction: "s2c", Match: match(typeMeasure | responseFlag),
			Fields: fields(h,
				spec.FieldRule{Name: "channel", Offset: 5, Type: "u8", Meaning: "Номер канала", Status: spec.MeaningConfirmed},
				spec.FieldRule{Name: "temperature", Offset: 6, Type: "i16", Endian: "le", Scale: 0.1, Unit: "°C", Meaning: "Температура", Status: spec.MeaningConfirmed, Hypothesis: b.hyp("h-06")},
				spec.FieldRule{Name: "humidity", Offset: 8, Type: "u16", Endian: "le", Scale: 0.1, Unit: "%", Meaning: "Влажность", Status: spec.MeaningConfirmed},
				spec.FieldRule{Name: "uptime", Offset: 10, Type: "u32", Endian: "le", Unit: "с", Meaning: "Время работы устройства?", Status: spec.MeaningHypothesis, Hypothesis: b.hyp("h-08")},
				sum,
			),
		},
		{
			ID: "read_log_req", Title: "Чтение журнала — запрос", Direction: "c2s", Match: match(typeReadLog),
			Fields: fields(h, spec.FieldRule{Name: "count", Offset: 5, Type: "u8", Meaning: "Запрошено записей", Status: spec.MeaningHypothesis}, sum),
		},
		{
			ID: "read_log_resp", Title: "Чтение журнала — ответ", Direction: "s2c", Match: match(typeReadLog | responseFlag),
			Fields: fields(h,
				spec.FieldRule{Name: "count", Offset: 5, Type: "u8", Meaning: "Количество записей", Status: spec.MeaningConfirmed},
				spec.FieldRule{Name: "entries", Offset: 6, Type: "array", Count: "count", ItemSize: 8, Items: []spec.FieldRule{
					{Name: "timestamp", Offset: 0, Type: "u32", Endian: "le", Meaning: "Unix-время события?", Status: spec.MeaningHypothesis},
					{Name: "code", Offset: 4, Type: "u8", Meaning: "Код события", Status: spec.MeaningHypothesis},
					{Name: "value", Offset: 5, Type: "i16", Endian: "le", Scale: 0.1, Meaning: "Значение", Status: spec.MeaningHypothesis},
					{Name: "flags", Offset: 7, Type: "u8", Meaning: "Флаги (назначение неизвестно)", Status: spec.MeaningStructural},
				}},
				sum,
			),
		},
		{ID: "heartbeat", Title: "Heartbeat", Direction: "c2s", Match: match(typeHeartbeat), Fields: fields(h, sum)},
		{ID: "heartbeat_ack", Title: "Heartbeat — ответ", Direction: "s2c", Match: match(typeHeartbeat | responseFlag), Fields: fields(h, sum)},
	}
}

func (b specBuilder) v2() spec.Spec {
	s := b.v1()
	s.Messages = b.refined(false)
	s.Assertions[0].Hypothesis = b.hyp("h-04")
	return s
}

func (b specBuilder) v3() spec.Spec {
	s := b.v2()
	s.Messages = b.refined(true)
	return s
}

func versions(hyp func(string) string) []VersionSeed {
	b := specBuilder{hyp: hyp}
	return []VersionSeed{
		{Spec: b.v1(), Comment: "Кадрирование по длине; запросы чтения/записи/измерения", CreatedAt: at("2026-09-14T12:50:00Z")},
		{Spec: b.v2(), Comment: "value → i16 LE, температура → i16; журнал, heartbeat, контрольная сумма", CreatedAt: at("2026-09-15T13:50:00Z")},
		{Spec: b.v3(), Comment: "seq подтверждён; перечисление параметров; ожидание статуса ответа", CreatedAt: at("2026-09-16T09:55:00Z")},
	}
}

// Indexes into Dataset.Corpora and Dataset.Runs.
const (
	corpusMain = iota
	corpusExtended
	corpusDefects
)

const (
	runV1Main = iota
	runV1Extended
	runV2Extended
	runV2Defects
	runV3Extended
)

func corpora() []CorpusSeed {
	return []CorpusSeed{
		corpusMain:     {Name: "Основной (14.09)", Description: "Все действия первого дня, целевой порт", CaptureFiles: []string{day1File}, OnlyTarget: true},
		corpusExtended: {Name: "Расширенный (14.09 + 15.09)", Description: "Основной корпус + дополнительная запись с новыми значениями параметров", CaptureFiles: []string{day1File, day2File}, OnlyTarget: true},
		corpusDefects:  {Name: "Дефекты захвата", Description: "Записи с потерями, усечением, конфликтами и без начала соединения", CaptureFiles: []string{day3File}, OnlyTarget: true},
	}
}

// runs replay the research: v1 passes the first day and fails on the second,
// v2 and v3 pass the extended corpus, v2 meets the capture defects.
func runs() []RunSeed {
	return []RunSeed{
		runV1Main:     {Version: 0, Corpus: corpusMain},
		runV1Extended: {Version: 0, Corpus: corpusExtended},
		runV2Extended: {Version: 1, Corpus: corpusExtended},
		runV2Defects:  {Version: 1, Corpus: corpusDefects},
		runV3Extended: {Version: 2, Corpus: corpusExtended},
	}
}

func event(when, status, note string) model.HypothesisEvent {
	return model.HypothesisEvent{At: at(when), Status: status, Note: note}
}

// hypotheses: every status change names what backs it. Claims about
// verification runs are checked by the tests of the demo project.
func hypotheses() []HypothesisSeed {
	list := []HypothesisSeed{
		{
			SeedID:       "h-01",
			Title:        "Байт 0 = 0xA5 — маркер начала кадра",
			Statement:    "Каждое сообщение в обоих направлениях начинается с байта 0xA5.",
			Status:       "supported",
			Alternatives: []string{"Часть поля версии протокола"},
			RuleRefs:     []string{"*.magic"},
			History: []model.HypothesisEvent{
				event("2026-09-14T11:20:00Z", "proposed", "Замечено в первых 8 сессиях"),
				event("2026-09-15T14:05:00Z", "supported", "Проверка v2 на расширенном корпусе: все сообщения распознаны, неизвестных типов нет"),
			},
		},
		{
			SeedID:       "h-02",
			Title:        "Байты 3–4 — длина полезной нагрузки (u16 LE)",
			Statement:    "Полная длина сообщения = значение u16 LE по смещению 3 + 6. Граница TCP-сегмента не совпадает с границей сообщения.",
			Status:       "supported",
			Alternatives: []string{"Длина включает заголовок (+1/+5)"},
			RuleRefs:     []string{"framing.lengthField"},
			History: []model.HypothesisEvent{
				event("2026-09-14T11:40:00Z", "proposed", "Ответы разного размера коррелируют с байтом 3"),
				event("2026-09-14T13:10:00Z", "supported", "Ответ чтения журнала (167 Б) в 3 сегментах кадрируется корректно"),
			},
		},
		{
			SeedID:       "h-03",
			Title:        "Значение настройки — u8 по смещению 6",
			Statement:    "В запросе 0x02 новое значение настройки кодируется одним байтом по смещению 6.",
			Status:       "refuted",
			Alternatives: []string{"u16/i16 LE по смещению 6–7"},
			RuleRefs:     []string{"set_param_req.value (v1)"},
			History: []model.HypothesisEvent{
				event("2026-09-14T12:00:00Z", "proposed", "«Задать 21» → 0x15 по смещению 6"),
				event("2026-09-14T12:30:00Z", "testing", "Совпало для 21, 23, 18 — но байт 7 всегда 0x00"),
				event("2026-09-15T10:40:00Z", "refuted", "Интервал 600 → 58 02, калибровка −15 → F1 FF: проверка v1 на расширенном корпусе нарушает a-set-value"),
			},
		},
		{
			SeedID:       "h-04",
			Title:        "Значение настройки — i16 LE по смещению 6–7",
			Statement:    "Новое значение настройки — знаковое 16-битное целое little-endian; масштаб зависит от параметра.",
			Status:       "supported",
			Alternatives: []string{"u16 LE (опровергается калибровкой −15)"},
			RuleRefs:     []string{"set_param_req.value", "read_params_resp.entries[].value"},
			History: []model.HypothesisEvent{
				event("2026-09-15T10:45:00Z", "testing", "Объясняет 21/23/18/2/600/−15"),
				event("2026-09-15T14:05:00Z", "supported", "Проверка a-set-value проходит на всём расширенном корпусе (v2)"),
			},
		},
		{
			SeedID:       "h-05",
			Title:        "Последний байт — контрольная сумма sum8",
			Statement:    "Последний байт = сумма всех предыдущих байтов сообщения по модулю 256.",
			Status:       "testing",
			Alternatives: []string{"XOR всех байтов", "CRC-8"},
			RuleRefs:     []string{"*.checksum"},
			History: []model.HypothesisEvent{
				event("2026-09-15T13:00:00Z", "testing", "Совпадает везде, но нет примера с намеренно испорченным кадром"),
			},
		},
		{
			SeedID:       "h-06",
			Title:        "Температура — i16 LE × 0.1 °C",
			Statement:    "В ответе 0x83 температура — знаковое i16 LE по смещению 6, масштаб 0.1 °C.",
			Status:       "supported",
			Alternatives: []string{"u16 × 0.1 (опровергается −5.5 °C)"},
			RuleRefs:     []string{"get_measurement_resp.temperature"},
			History: []model.HypothesisEvent{
				event("2026-09-14T12:40:00Z", "testing", "22.4 → E0 00, 19.8 → C6 00; отрицательных значений нет, в v1 поле описано как u16"),
				event("2026-09-15T14:05:00Z", "supported", "Канал 3: −5.5 °C → C9 FF — работает только знаковое: v1 (u16) нарушает a-measurement, v2 (i16) — нет"),
			},
		},
		{
			SeedID:       "h-07",
			Title:        "Байт 2 — номер запроса",
			Statement:    "Клиент увеличивает байт 2 в каждом запросе сессии; ответ повторяет значение запроса.",
			Status:       "supported",
			Alternatives: []string{"Номер канала/устройства"},
			RuleRefs:     []string{"*.seq"},
			History: []model.HypothesisEvent{
				event("2026-09-14T13:30:00Z", "proposed", "В сессии с двумя запросами значения 1 и 2"),
				event("2026-09-16T10:00:00Z", "supported", "Подтверждено на всех многозапросных сессиях"),
			},
		},
		{
			SeedID:       "h-08",
			Title:        "u32 по смещению 10 в ответе 0x83 — время работы",
			Statement:    "Монотонно растущее значение; разница между измерениями ≈ разнице времени захвата в секундах.",
			Status:       "proposed",
			Alternatives: []string{"Счётчик измерений", "Unix-время"},
			RuleRefs:     []string{"get_measurement_resp.uptime"},
			History: []model.HypothesisEvent{
				event("2026-09-15T13:30:00Z", "proposed", "86412 → 86473 за 60 с захвата"),
			},
		},
	}
	for i := range list {
		h := &list[i]
		h.CreatedAt = h.History[0].At
		h.UpdatedAt = h.History[len(h.History)-1].At
	}
	return list
}

// fieldRef points at a field of the session with the given key. FieldRef
// resolves to the first session with the client port, so a session whose port
// an earlier session of the capture already used cannot be referenced.
func fieldRef(sessionKey, rule, field string) FieldRef {
	for _, cs := range captureSpecs() {
		for _, ss := range cs.sessions {
			if ss.key != sessionKey {
				continue
			}
			for _, other := range cs.sessions {
				if other.client == ss.client && other.start < ss.start {
					panic(fmt.Sprintf("demo: session %q reuses the client port of %q", ss.key, other.key))
				}
			}
			return FieldRef{CaptureFile: cs.fileName, ClientPort: int(ss.client.Port()), Rule: rule, Field: field}
		}
	}
	panic(fmt.Sprintf("demo: unknown session %q", sessionKey))
}

func observations() []ObservationSeed {
	obs := func(key, rule, field, note, createdAt string, hyps ...string) ObservationSeed {
		return ObservationSeed{Ref: fieldRef(key, rule, field), Note: note, HypothesisIDs: hyps, CreatedAt: at(createdAt)}
	}
	return []ObservationSeed{
		obs("read-params", "read_params_resp", "magic", "Ответы начинаются с A5, как и запросы", "2026-09-14T11:18:00Z", "h-01"),
		obs("set-21", "set_param_req", "value", "«Задать 21» → по смещению 6 байт 0x15, следом 0x00", "2026-09-14T11:58:00Z", "h-03", "h-04"),
		obs("set-23", "set_param_req", "value", "«Задать 23» → 0x17 0x00", "2026-09-14T12:02:00Z", "h-03", "h-04"),
		obs("read-log", "read_log_resp", "length", "Ответ чтения журнала (167 Б) пришёл в трёх TCP-сегментах; в байтах 3–4 значение 161", "2026-09-14T13:05:00Z", "h-02"),
		obs("set-interval", "set_param_req", "value", "Интервал 600 → 58 02: старший байт ненулевой, u8 не подходит", "2026-09-15T10:35:00Z", "h-03", "h-04"),
		obs("set-calibration", "set_param_req", "value", "Калибровка −15 → F1 FF: значение знаковое", "2026-09-15T10:38:00Z", "h-04"),
		obs("meas-outdoor", "get_measurement_resp", "temperature", "−5.5 °C в клиенте → C9 FF (−55 как i16)", "2026-09-15T11:10:00Z", "h-06"),
		obs("meas-2", "get_measurement_resp", "uptime", "Растёт между измерениями: 86412 → 86473", "2026-09-15T13:28:00Z", "h-08"),
	}
}

// counterexamples point at the bytes the v1 run on the extended corpus flags.
func counterexamples() []CounterexampleSeed {
	return []CounterexampleSeed{
		{HypothesisID: "h-03", Ref: fieldRef("set-interval", "set_param_req", "value"), Note: "Интервал 600: u8 даёт 88", Run: runV1Extended},
		{HypothesisID: "h-03", Ref: fieldRef("set-calibration", "set_param_req", "value"), Note: "Калибровка −15: u8 даёт 241", Run: runV1Extended},
	}
}
