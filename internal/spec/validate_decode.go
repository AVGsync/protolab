package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func parse(raw []byte) (*Spec, ValidationResult) {
	if isJSONNull(raw) {
		return nil, invalid(errorIssue("$", "Ожидается JSON-объект"))
	}
	s, err := Decode(raw)
	if err != nil {
		return nil, invalid(decodeIssue(raw, err))
	}
	issues := validate(s, missingOffsets(raw))
	return s, ValidationResult{Valid: !hasErrors(issues), Issues: issues}
}

func invalid(issue Issue) ValidationResult {
	return ValidationResult{Valid: false, Issues: []Issue{issue}}
}

func errorIssue(path, msg string) Issue {
	return Issue{Path: path, Message: msg, Severity: SeverityError}
}

func hasErrors(issues []Issue) bool {
	for _, is := range issues {
		if is.Severity == SeverityError {
			return true
		}
	}
	return false
}

func isJSONNull(raw []byte) bool { return string(bytes.TrimSpace(raw)) == "null" }

// decodeIssue turns an error of the strict decoder into a single issue with a
// precise path. encoding/json reports type errors with a dotted path without
// array indices and unknown fields without any location, so the document is
// replayed against the Spec type to find the offending value.
func decodeIssue(raw []byte, err error) Issue {
	var syntax *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return errorIssue("$", "Ожидается JSON-объект")
	case errors.Is(err, io.ErrUnexpectedEOF):
		return errorIssue("$", "Некорректный JSON: неожиданный конец данных")
	case errors.As(err, &syntax):
		return errorIssue("$", syntaxMessage(raw, syntax))
	}
	if issue, ok := locateDecodeError(raw); ok {
		return issue
	}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) && typeErr.Field != "" {
		return errorIssue("$."+typeErr.Field, "Неверный тип значения")
	}
	return errorIssue("$", "Некорректный JSON: "+err.Error())
}

func syntaxMessage(raw []byte, e *json.SyntaxError) string {
	pos := int(e.Offset)
	if pos > len(raw) {
		pos = len(raw)
	}
	line, col := lineCol(raw, pos)
	what := "синтаксическая ошибка"
	switch {
	case strings.HasPrefix(e.Error(), "invalid character") && pos > 0:
		// The scanner stops right after the offending byte.
		r, _ := utf8.DecodeRune(raw[pos-1:])
		what = "недопустимый символ " + strconv.QuoteRune(r)
		line, col = lineCol(raw, pos-1)
	case strings.Contains(e.Error(), "unexpected end of JSON input"):
		what = "неожиданный конец данных"
	}
	return fmt.Sprintf("Некорректный JSON: %s (строка %d, столбец %d)", what, line, col)
}

func lineCol(raw []byte, pos int) (line, col int) {
	before := raw[:pos]
	line = bytes.Count(before, []byte{'\n'}) + 1
	lineStart := bytes.LastIndexByte(before, '\n') + 1
	return line, utf8.RuneCount(before[lineStart:]) + 1
}

var lengthType = reflect.TypeOf(Length{})

// schemaWalker streams a syntactically valid document and checks every value
// against the Go type it decodes into, following encoding/json rules: null is
// accepted anywhere, object keys match field names exactly or case-insensitively,
// integers must be written without fraction or exponent.
type schemaWalker struct {
	dec *json.Decoder
}

func locateDecodeError(raw []byte) (Issue, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	w := schemaWalker{dec: dec}

	tok, err := dec.Token()
	if err != nil || tok == nil {
		return Issue{}, false
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errorIssue("$", "Ожидается JSON-объект"), true
	}
	issue, err := w.object(reflect.TypeOf(Spec{}), "$")
	if err != nil {
		return Issue{}, false
	}
	if issue != nil {
		return *issue, true
	}
	if _, err := dec.Token(); err != io.EOF {
		return errorIssue("$", "Лишние данные после JSON-объекта"), true
	}
	return Issue{}, false
}

func (w *schemaWalker) value(tok json.Token, t reflect.Type, path string) (*Issue, error) {
	if tok == nil {
		return nil, nil
	}
	if t == lengthType {
		return checkLength(tok, path), nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return w.value(tok, t.Elem(), path)
	case reflect.Struct:
		if !isDelim(tok, '{') {
			return mismatch(path, "Ожидается объект"), nil
		}
		return w.object(t, path)
	case reflect.Map:
		if !isDelim(tok, '{') {
			return mismatch(path, "Ожидается объект"), nil
		}
		return w.mapEntries(t, path)
	case reflect.Slice:
		if !isDelim(tok, '[') {
			return mismatch(path, "Ожидается массив"), nil
		}
		return w.array(t, path)
	case reflect.String:
		if _, ok := tok.(string); !ok {
			return mismatch(path, "Ожидается строка"), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := tok.(json.Number)
		if !ok {
			return mismatch(path, "Ожидается число"), nil
		}
		if _, err := strconv.ParseInt(string(n), 10, t.Bits()); err != nil {
			return mismatch(path, "Ожидается целое число"), nil
		}
	case reflect.Float32, reflect.Float64:
		n, ok := tok.(json.Number)
		if !ok {
			return mismatch(path, "Ожидается число"), nil
		}
		if _, err := strconv.ParseFloat(string(n), t.Bits()); err != nil {
			return mismatch(path, "Число вне допустимого диапазона"), nil
		}
	case reflect.Bool:
		if _, ok := tok.(bool); !ok {
			return mismatch(path, "Ожидается true или false"), nil
		}
	}
	return nil, nil
}

// object walks the members of an object whose opening brace was consumed.
func (w *schemaWalker) object(t reflect.Type, path string) (*Issue, error) {
	fields := jsonFields(t)
	for w.dec.More() {
		keyTok, err := w.dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)
		f, ok := lookupField(fields, key)
		if !ok {
			return mismatch(path+"."+key, fmt.Sprintf("Неизвестное поле %q", key)), nil
		}
		if issue, err := w.next(f.typ, path+"."+f.name); issue != nil || err != nil {
			return issue, err
		}
	}
	_, err := w.dec.Token() // closing brace
	return nil, err
}

func (w *schemaWalker) mapEntries(t reflect.Type, path string) (*Issue, error) {
	for w.dec.More() {
		keyTok, err := w.dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)
		if issue, err := w.next(t.Elem(), path+"."+key); issue != nil || err != nil {
			return issue, err
		}
	}
	_, err := w.dec.Token()
	return nil, err
}

func (w *schemaWalker) array(t reflect.Type, path string) (*Issue, error) {
	for i := 0; w.dec.More(); i++ {
		if issue, err := w.next(t.Elem(), fmt.Sprintf("%s[%d]", path, i)); issue != nil || err != nil {
			return issue, err
		}
	}
	_, err := w.dec.Token()
	return nil, err
}

func (w *schemaWalker) next(t reflect.Type, path string) (*Issue, error) {
	tok, err := w.dec.Token()
	if err != nil {
		return nil, err
	}
	return w.value(tok, t, path)
}

func checkLength(tok json.Token, path string) *Issue {
	switch v := tok.(type) {
	case string:
		return nil
	case json.Number:
		if _, err := strconv.ParseInt(string(v), 10, 64); err == nil {
			return nil
		}
	}
	return mismatch(path, "Ожидается целое число или имя поля")
}

func mismatch(path, msg string) *Issue {
	issue := errorIssue(path, msg)
	return &issue
}

func isDelim(tok json.Token, d json.Delim) bool {
	got, ok := tok.(json.Delim)
	return ok && got == d
}

type jsonField struct {
	name string
	typ  reflect.Type
}

func jsonFields(t reflect.Type) []jsonField {
	out := make([]jsonField, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		switch name {
		case "-":
			continue
		case "":
			name = sf.Name
		}
		out = append(out, jsonField{name: name, typ: sf.Type})
	}
	return out
}

// lookupField matches a key the way encoding/json does: an exact match wins,
// otherwise the first case-insensitive one.
func lookupField(fields []jsonField, key string) (jsonField, bool) {
	for _, f := range fields {
		if f.name == key {
			return f, true
		}
	}
	for _, f := range fields {
		if strings.EqualFold(f.name, key) {
			return f, true
		}
	}
	return jsonField{}, false
}

// offsetProbe mirrors the parts of a spec where a missing "offset" key must be
// reported. Offset is a pointer so that absence (and null) stay visible.
type offsetProbe struct {
	Messages []struct {
		Fields []fieldProbe `json:"fields"`
	} `json:"messages"`
}

type fieldProbe struct {
	Offset *json.RawMessage `json:"offset"`
	Items  []fieldProbe     `json:"items"`
}

// missingOffsets returns the paths of field objects without an offset.
func missingOffsets(raw []byte) map[string]bool {
	var probe offsetProbe
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil
	}
	missing := make(map[string]bool)
	var walk func(fields []fieldProbe, path string)
	walk = func(fields []fieldProbe, path string) {
		for i, f := range fields {
			p := fmt.Sprintf("%s[%d]", path, i)
			if f.Offset == nil {
				missing[p] = true
			}
			walk(f.Items, p+".items")
		}
	}
	for i, m := range probe.Messages {
		walk(m.Fields, fmt.Sprintf("$.messages[%d].fields", i))
	}
	return missing
}
