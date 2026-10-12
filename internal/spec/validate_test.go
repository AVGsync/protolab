package spec

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// Code below mirrors validateSpec of the TypeScript reference engine
// (frontend/src/mocks/engine/interpreter.ts): inputs and expected issues were
// produced by running it.
var referenceValidationCases = []struct {
	name  string
	input string
	valid bool
	want  []Issue
}{
	{
		name:  "tsTest",
		input: `{"format":"protolab/v1","framing":{"kind":"length_prefixed","lengthField":{"size":3}},"messages":[{"id":"a","match":[],"fields":[{"name":"x","type":"u7","offset":0}]}]}`,
		valid: false,
		want: []Issue{
			{Path: "$.protocol", Message: "Не указано имя протокола", Severity: SeverityWarning},
			{Path: "$.framing.lengthField.size", Message: "Размер поля длины: 1, 2 или 4", Severity: SeverityError},
			{Path: "$.framing.lengthAdjust", Message: "Ожидается число", Severity: SeverityError},
			{Path: "$.framing.sync", Message: "Без sync невозможна ресинхронизация после пропуска", Severity: SeverityWarning},
			{Path: "$.messages[0].match", Message: "Правило без условий совпадёт с любым сообщением", Severity: SeverityWarning},
			{Path: "$.messages[0].fields[0].type", Message: "Неизвестный тип \"u7\"", Severity: SeverityError},
		},
	},
	{
		name:  "empty",
		input: `{}`,
		valid: false,
		want: []Issue{
			{Path: "$.format", Message: "Поддерживается только формат \"protolab/v1\"", Severity: SeverityError},
			{Path: "$.protocol", Message: "Не указано имя протокола", Severity: SeverityWarning},
			{Path: "$.framing", Message: "Не задан способ выделения сообщений", Severity: SeverityError},
			{Path: "$.messages", Message: "Нет ни одного правила сообщения", Severity: SeverityWarning},
		},
	},
	{
		name:  "badFormat",
		input: `{"format":"v2","protocol":"P","framing":{"kind":"fixed","size":4},"messages":[{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[]}]}`,
		valid: false,
		want: []Issue{
			{Path: "$.format", Message: "Поддерживается только формат \"protolab/v1\"", Severity: SeverityError},
		},
	},
	{
		name:  "badKind",
		input: `{"format":"protolab/v1","protocol":"P","framing":{"kind":"stream"},"messages":[{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[]}]}`,
		valid: false,
		want: []Issue{
			{Path: "$.framing.kind", Message: "Ожидается length_prefixed, delimiter или fixed", Severity: SeverityError},
		},
	},
	{
		name:  "badDelimiter",
		input: `{"format":"protolab/v1","protocol":"P","framing":{"kind":"delimiter","delimiter":"0x0G"},"messages":[{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[]}]}`,
		valid: false,
		want: []Issue{
			{Path: "$.framing.delimiter", Message: "Некорректный hex", Severity: SeverityError},
		},
	},
	{
		name:  "badSync",
		input: `{"format":"protolab/v1","protocol":"P","framing":{"kind":"length_prefixed","lengthField":{"offset":0,"size":2,"endian":"le"},"lengthAdjust":2,"sync":"A5 0"},"messages":[{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[]}]}`,
		valid: false,
		want: []Issue{
			{Path: "$.framing.sync", Message: "Некорректный hex", Severity: SeverityError},
		},
	},
	{
		name:  "rules",
		input: `{"format":"protolab/v1","protocol":"P","framing":{"kind":"fixed","size":8},"messages":[{"id":"a","match":[{"offset":0,"hex":"zz"},{"offset":1,"hex":""}],"fields":[{"name":"","type":"u8","offset":0},{"name":"","type":"u8","offset":1},{"name":"n","type":"u8","offset":2},{"name":"n","type":"i16","offset":3},{"name":"nooff","type":"u8"},{"name":"arr","type":"array","offset":4},{"name":"later","type":"array","offset":4,"count":"cnt","itemSize":2,"items":[{"name":"v","type":"u8","offset":0},{"name":"w","type":"float","offset":1},{"name":"s","type":"bytes","offset":1}]},{"name":"cnt","type":"u8","offset":5},{"name":"txt","type":"ascii","offset":6},{"name":"ok","type":"bytes","offset":6,"length":"n"}]},{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[]},{"id":"","match":[],"fields":[]},{"id":"b","match":[{"offset":0,"hex":"0xA5, 0x01"}],"fields":[]}],"assertions":[{"id":"x","rule":"missing","actionKind":"k","checks":[{"field":"n","equals":"params.value"},{"field":"n","equals":"result.value"},{"field":"n","equals":"params."}]},{"id":"y","rule":"b","actionKind":"k","checks":[{"field":"n","equals":"observed.temp_1"}]}]}`,
		valid: false,
		want: []Issue{
			{Path: "$.messages[0].match[0].hex", Message: "Некорректный hex", Severity: SeverityError},
			{Path: "$.messages[0].match[1].hex", Message: "Некорректный hex", Severity: SeverityError},
			{Path: "$.messages[0].fields[0].name", Message: "Не задано имя поля", Severity: SeverityError},
			{Path: "$.messages[0].fields[1].name", Message: "Не задано имя поля", Severity: SeverityError},
			{Path: "$.messages[0].fields[1].name", Message: "Повторяющееся имя \"\"", Severity: SeverityError},
			{Path: "$.messages[0].fields[3].name", Message: "Повторяющееся имя \"n\"", Severity: SeverityError},
			{Path: "$.messages[0].fields[4].offset", Message: "Ожидается число", Severity: SeverityError},
			{Path: "$.messages[0].fields[5].count", Message: "count должен ссылаться на предыдущее поле", Severity: SeverityError},
			{Path: "$.messages[0].fields[5].itemSize", Message: "Не задан размер элемента", Severity: SeverityError},
			{Path: "$.messages[0].fields[6].count", Message: "count должен ссылаться на предыдущее поле", Severity: SeverityError},
			{Path: "$.messages[0].fields[6].items[1].type", Message: "Неизвестный тип \"float\"", Severity: SeverityError},
			{Path: "$.messages[0].fields[6].items[2].length", Message: "Не задана длина", Severity: SeverityError},
			{Path: "$.messages[0].fields[8].length", Message: "Не задана длина", Severity: SeverityError},
			{Path: "$.messages[1].id", Message: "Повторяющийся id \"a\"", Severity: SeverityError},
			{Path: "$.messages[2].id", Message: "Не задан id правила", Severity: SeverityError},
			{Path: "$.messages[2].match", Message: "Правило без условий совпадёт с любым сообщением", Severity: SeverityWarning},
			{Path: "$.assertions[0].rule", Message: "Нет правила \"missing\"", Severity: SeverityError},
			{Path: "$.assertions[0].checks[1].equals", Message: "Ожидается \"params.<имя>\" или \"observed.<имя>\"", Severity: SeverityError},
			{Path: "$.assertions[0].checks[2].equals", Message: "Ожидается \"params.<имя>\" или \"observed.<имя>\"", Severity: SeverityError},
		},
	},
	{
		name:  "noMessages",
		input: `{"format":"protolab/v1","protocol":"P","framing":{"kind":"delimiter","delimiter":"0D 0A"},"messages":[]}`,
		valid: true,
		want: []Issue{
			{Path: "$.messages", Message: "Нет ни одного правила сообщения", Severity: SeverityWarning},
		},
	},
}

func TestValidateMatchesReference(t *testing.T) {
	for _, tc := range referenceValidationCases {
		t.Run(tc.name, func(t *testing.T) {
			s, res := Parse([]byte(tc.input))
			if s == nil {
				t.Fatalf("not decoded: %+v", res.Issues)
			}
			if res.Valid != tc.valid || !reflect.DeepEqual(res.Issues, tc.want) {
				t.Errorf("valid %v issues:\n%s\nwant valid %v:\n%s", res.Valid, formatIssues(res.Issues), tc.valid, formatIssues(tc.want))
			}
		})
	}
}

func formatIssues(issues []Issue) string {
	var sb strings.Builder
	for _, is := range issues {
		sb.WriteString("  " + is.Severity + " " + is.Path + " " + is.Message + "\n")
	}
	return sb.String()
}

func TestShippedSpecsAreValid(t *testing.T) {
	for _, name := range []string{"testdata/thermolink_v1.json", "testdata/thermolink_v3.json"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		s, res := Parse(raw)
		if s == nil || !res.Valid || len(res.Issues) != 0 {
			t.Errorf("%s: %+v", name, res)
		}
		if got := Validate(s); len(got) != 0 {
			t.Errorf("%s: Validate = %+v", name, got)
		}
	}
}

func TestValidateExtraChecks(t *testing.T) {
	base := func(framing, fields string) string {
		return `{"format":"protolab/v1","protocol":"P","framing":` + framing +
			`,"messages":[{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":` + fields + `}]}`
	}
	const fixed = `{"kind":"fixed","size":4}`
	tests := []struct {
		name, input, path, message string
	}{
		{"fixed size", base(`{"kind":"fixed"}`, `[]`), "$.framing.size", "Ожидается положительный размер сообщения"},
		{"length field endian", base(`{"kind":"length_prefixed","lengthField":{"offset":0,"size":2,"endian":"big"},"lengthAdjust":0,"sync":"A5"}`, `[]`),
			"$.framing.lengthField.endian", `Ожидается "le" или "be"`},
		{"negative length field offset", base(`{"kind":"length_prefixed","lengthField":{"offset":-1,"size":2,"endian":"le"},"lengthAdjust":0,"sync":"A5"}`, `[]`),
			"$.framing.lengthField.offset", "Смещение не может быть отрицательным"},
		{"direction", `{"format":"protolab/v1","protocol":"P","framing":` + fixed + `,"messages":[{"id":"a","direction":"up","match":[{"offset":0,"hex":"A5"}],"fields":[]}]}`,
			"$.messages[0].direction", "Ожидается c2s, s2c или any"},
		{"negative match offset", `{"format":"protolab/v1","protocol":"P","framing":` + fixed + `,"messages":[{"id":"a","match":[{"offset":-1,"hex":"A5"}],"fields":[]}]}`,
			"$.messages[0].match[0].offset", "Смещение не может быть отрицательным"},
		{"field endian", base(fixed, `[{"name":"x","offset":0,"type":"u16","endian":"LE"}]`), "$.messages[0].fields[0].endian", `Ожидается "le" или "be"`},
		{"count refers to itself", base(fixed, `[{"name":"n","offset":0,"type":"array","count":"n","itemSize":1,"items":[]}]`),
			"$.messages[0].fields[0].count", "count должен ссылаться на предыдущее поле"},
		{"count refers to a non-integer", base(fixed, `[{"name":"s","offset":0,"type":"bytes","length":1},{"name":"a","offset":1,"type":"array","count":"s","itemSize":1,"items":[]}]`),
			"$.messages[0].fields[1].count", "count должен ссылаться на предыдущее поле"},
		{"negative item size", base(fixed, `[{"name":"n","offset":0,"type":"u8"},{"name":"a","offset":1,"type":"array","count":"n","itemSize":-2,"items":[]}]`),
			"$.messages[0].fields[1].itemSize", "Размер элемента должен быть положительным"},
		{"length refers to a later field", base(fixed, `[{"name":"s","offset":1,"type":"ascii","length":"n"},{"name":"n","offset":0,"type":"u8"}]`),
			"$.messages[0].fields[0].length", "length должен ссылаться на предыдущее целочисленное поле"},
		{"negative length", base(fixed, `[{"name":"s","offset":1,"type":"bytes","length":-1}]`), "$.messages[0].fields[0].length", "Длина не может быть отрицательной"},
		{"checksum algorithm", base(fixed, `[{"name":"c","offset":-1,"type":"u8","checksum":"crc8"}]`), "$.messages[0].fields[0].checksum", "Ожидается sum8 или xor8"},
		{"meaning status", base(fixed, `[{"name":"c","offset":0,"type":"u8","status":"maybe"}]`), "$.messages[0].fields[0].status", "Ожидается structural, hypothesis или confirmed"},
		{"missing offset in array items", base(fixed, `[{"name":"n","offset":0,"type":"u8"},{"name":"a","offset":1,"type":"array","count":"n","itemSize":1,"items":[{"name":"v","type":"u8"}]}]`),
			"$.messages[0].fields[1].items[0].offset", "Ожидается число"},
		{"null offset", base(fixed, `[{"name":"v","offset":null,"type":"u8"}]`), "$.messages[0].fields[0].offset", "Ожидается число"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, res := Parse([]byte(tt.input))
			if s == nil {
				t.Fatalf("not decoded: %+v", res.Issues)
			}
			want := Issue{Path: tt.path, Message: tt.message, Severity: SeverityError}
			if res.Valid || !reflect.DeepEqual(res.Issues, []Issue{want}) {
				t.Errorf("issues:\n%swant %+v", formatIssues(res.Issues), want)
			}
		})
	}
}

func TestValidateNilAndNeverNil(t *testing.T) {
	if got := Validate(nil); len(got) != 1 || got[0].Path != "$" || got[0].Severity != SeverityError {
		t.Errorf("Validate(nil) = %+v", got)
	}
	raw, _ := os.ReadFile("testdata/thermolink_v3.json")
	s, _ := Parse(raw)
	if got := Validate(s); got == nil {
		t.Error("Validate returned nil")
	}
}

func TestParseDecodeErrors(t *testing.T) {
	tests := []struct {
		name, input, path, message string
	}{
		{"empty input", ``, "$", "Ожидается JSON-объект"},
		{"null", ` null `, "$", "Ожидается JSON-объект"},
		{"array at the top", `[1,2]`, "$", "Ожидается JSON-объект"},
		{"string at the top", `"spec"`, "$", "Ожидается JSON-объект"},
		{"truncated", `{"format":"protolab/v1"`, "$", "Некорректный JSON: неожиданный конец данных"},
		{"bad character", "{\n  \"format\": \"protolab/v1\",\n  \"protocol\" \"P\"\n}", "$", "Некорректный JSON: недопустимый символ '\"' (строка 3, столбец 14)"},
		{"bad literal", `{"format": tru}`, "$", "Некорректный JSON: недопустимый символ '}' (строка 1, столбец 15)"},
		{"trailing data", `{"format":"protolab/v1"} {"x":1}`, "$", "Лишние данные после JSON-объекта"},
		{"trailing brace", `{"format":"protolab/v1"}}`, "$", "Лишние данные после JSON-объекта"},
		{"trailing bracket", "{\"format\":\"protolab/v1\"}\n]", "$", "Лишние данные после JSON-объекта"},
		{"unknown top-level field", `{"format":"protolab/v1","extra":1}`, "$.extra", `Неизвестное поле "extra"`},
		{"unknown nested field", `{"framing":{"kind":"fixed","size":4},"messages":[{"id":"a","match":[],"fields":[{"name":"x","offset":0,"type":"u8"},{"name":"y","offset":1,"type":"u8","scal":2}]}]}`,
			"$.messages[0].fields[1].scal", `Неизвестное поле "scal"`},
		{"string instead of number", `{"messages":[{"id":"a","match":[{"offset":"0","hex":"A5"}]}]}`, "$.messages[0].match[0].offset", "Ожидается число"},
		{"fraction for an integer", `{"framing":{"kind":"fixed","size":4.5}}`, "$.framing.size", "Ожидается целое число"},
		{"number instead of string", `{"format":1}`, "$.format", "Ожидается строка"},
		{"object instead of array", `{"messages":{}}`, "$.messages", "Ожидается массив"},
		{"array instead of object", `{"framing":[]}`, "$.framing", "Ожидается объект"},
		{"bad enum label", `{"messages":[{"id":"a","fields":[{"name":"x","offset":0,"type":"u8","enum":{"1":2}}]}]}`, "$.messages[0].fields[0].enum.1", "Ожидается строка"},
		{"bad length", `{"messages":[{"id":"a","fields":[{"name":"x","offset":0,"type":"bytes","length":true}]}]}`, "$.messages[0].fields[0].length", "Ожидается целое число или имя поля"},
		{"fractional length", `{"messages":[{"id":"a","fields":[{"name":"x","offset":0,"type":"bytes","length":1.5}]}]}`, "$.messages[0].fields[0].length", "Ожидается целое число или имя поля"},
		{"first error in document order wins", `{"assertions":[{"id":"a","rule":"r","checks":[{"field":1}]}],"zzz":0}`, "$.assertions[0].checks[0].field", "Ожидается строка"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, res := Parse([]byte(tt.input))
			want := []Issue{{Path: tt.path, Message: tt.message, Severity: SeverityError}}
			if s != nil || res.Valid || !reflect.DeepEqual(res.Issues, want) {
				t.Errorf("spec %v, result %+v\nwant %+v", s != nil, res, want)
			}
		})
	}
}

func TestParseAcceptsCaseInsensitiveKeys(t *testing.T) {
	// encoding/json matches field names case-insensitively; so must Parse.
	s, res := Parse([]byte(`{"Format":"protolab/v1","protocol":"P","Framing":{"kind":"fixed","size":2},"messages":[{"id":"a","match":[{"offset":0,"hex":"A5"}],"fields":[{"name":"x","OFFSET":0,"type":"u8"}]}]}`))
	if s == nil || !res.Valid {
		t.Errorf("result = %+v", res)
	}
}

func TestParseHex(t *testing.T) {
	tests := []struct {
		in   string
		want []byte
	}{
		{"A5 01", []byte{0xA5, 0x01}},
		{"a501", []byte{0xA5, 0x01}},
		{"0xA5, 0x01", []byte{0xA5, 0x01}},
		{"0XA5;0X01", []byte{0xA5, 0x01}},
		{"a5:01:ff", []byte{0xA5, 0x01, 0xFF}},
		{"a5-01\t0d\n0a", []byte{0xA5, 0x01, 0x0D, 0x0A}},
		{"00x1", []byte{0x01}}, // "0x" is removed wherever it appears
		{"", nil},
		{"A5 0", nil},
		{"zz", nil},
		{"0x", nil},
		{"A5 x1", nil},
	}
	for _, tt := range tests {
		got, ok := ParseHex(tt.in)
		if ok != (tt.want != nil) || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseHex(%q) = %X, %v; want %X", tt.in, got, ok, tt.want)
		}
	}
}
