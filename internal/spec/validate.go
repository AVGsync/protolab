package spec

import (
	"fmt"
	"regexp"
)

// Issue severities.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

var (
	intTypes = map[string]bool{
		"u8": true, "i8": true, "u16": true, "i16": true, "u32": true, "i32": true,
	}
	fieldTypes = map[string]bool{
		"u8": true, "i8": true, "u16": true, "i16": true, "u32": true, "i32": true,
		"bytes": true, "ascii": true, "array": true,
	}
	meaningStatuses = map[string]bool{
		"": true, MeaningStructural: true, MeaningHypothesis: true, MeaningConfirmed: true,
	}
	actionPathRe = regexp.MustCompile(`^(params|observed)\.\w+$`)
)

// IsIntType reports whether t is one of the integer field types.
func IsIntType(t string) bool { return intTypes[t] }

type validator struct {
	issues []Issue
	// missingOffset holds paths of field objects that had no "offset" key in the
	// source JSON. A decoded struct cannot tell a missing offset from 0, so Parse
	// collects them from the raw document.
	missingOffset map[string]bool
}

func (v *validator) err(path, msg string) {
	v.issues = append(v.issues, Issue{Path: path, Message: msg, Severity: SeverityError})
}

func (v *validator) warn(path, msg string) {
	v.issues = append(v.issues, Issue{Path: path, Message: msg, Severity: SeverityWarning})
}

func validate(s *Spec, missingOffset map[string]bool) []Issue {
	v := &validator{issues: []Issue{}, missingOffset: missingOffset}
	if s == nil {
		v.err("$", "Ожидается JSON-объект")
		return v.issues
	}
	if s.Format != Format {
		v.err("$.format", `Поддерживается только формат "protolab/v1"`)
	}
	if s.Protocol == "" {
		v.warn("$.protocol", "Не указано имя протокола")
	}
	v.framing(&s.Framing)

	ids := make(map[string]bool, len(s.Messages))
	for i := range s.Messages {
		v.message(&s.Messages[i], fmt.Sprintf("$.messages[%d]", i), ids)
	}
	if len(s.Messages) == 0 {
		v.warn("$.messages", "Нет ни одного правила сообщения")
	}

	for i, a := range s.Assertions {
		p := fmt.Sprintf("$.assertions[%d]", i)
		if !ids[a.Rule] {
			v.err(p+".rule", fmt.Sprintf("Нет правила %q", a.Rule))
		}
		for j, c := range a.Checks {
			if !actionPathRe.MatchString(c.Equals) {
				v.err(fmt.Sprintf("%s.checks[%d].equals", p, j), `Ожидается "params.<имя>" или "observed.<имя>"`)
			}
		}
	}
	return v.issues
}

func (v *validator) framing(f *Framing) {
	switch f.Kind {
	case "":
		v.err("$.framing", "Не задан способ выделения сообщений")
	case FramingLengthPrefixed:
		lf := f.LengthField
		if lf == nil || (lf.Size != 1 && lf.Size != 2 && lf.Size != 4) {
			v.err("$.framing.lengthField.size", "Размер поля длины: 1, 2 или 4")
		}
		if lf != nil {
			if lf.Offset < 0 {
				v.err("$.framing.lengthField.offset", "Смещение не может быть отрицательным")
			}
			if !validEndian(lf.Endian) {
				v.err("$.framing.lengthField.endian", `Ожидается "le" или "be"`)
			}
		}
		if f.LengthAdjust == nil {
			v.err("$.framing.lengthAdjust", "Ожидается число")
		}
		if f.Sync != "" {
			if _, ok := ParseHex(f.Sync); !ok {
				v.err("$.framing.sync", "Некорректный hex")
			}
		} else {
			v.warn("$.framing.sync", "Без sync невозможна ресинхронизация после пропуска")
		}
	case FramingDelimiter:
		if _, ok := ParseHex(f.Delimiter); !ok {
			v.err("$.framing.delimiter", "Некорректный hex")
		}
	case FramingFixed:
		if f.Size <= 0 {
			v.err("$.framing.size", "Ожидается положительный размер сообщения")
		}
	default:
		v.err("$.framing.kind", "Ожидается length_prefixed, delimiter или fixed")
	}
}

func (v *validator) message(r *MessageRule, p string, ids map[string]bool) {
	switch {
	case r.ID == "":
		v.err(p+".id", "Не задан id правила")
	case ids[r.ID]:
		v.err(p+".id", fmt.Sprintf("Повторяющийся id %q", r.ID))
	}
	ids[r.ID] = true

	switch r.Direction {
	case "", "any", "c2s", "s2c":
	default:
		v.err(p+".direction", "Ожидается c2s, s2c или any")
	}
	if len(r.Match) == 0 {
		v.warn(p+".match", "Правило без условий совпадёт с любым сообщением")
	}
	for j, c := range r.Match {
		if _, ok := ParseHex(c.Hex); !ok {
			v.err(fmt.Sprintf("%s.match[%d].hex", p, j), "Некорректный hex")
		}
		if c.Offset < 0 {
			v.err(fmt.Sprintf("%s.match[%d].offset", p, j), "Смещение не может быть отрицательным")
		}
	}
	v.fields(r.Fields, p+".fields")
}

func (v *validator) fields(fields []FieldRule, path string) {
	names := make(map[string]bool, len(fields))
	// Integer fields parsed so far on this level: the only valid targets of
	// count/length references, because only they put a value into scope.
	ints := make(map[string]bool, len(fields))

	for i := range fields {
		f := &fields[i]
		p := fmt.Sprintf("%s[%d]", path, i)
		if f.Name == "" {
			v.err(p+".name", "Не задано имя поля")
		}
		if names[f.Name] {
			v.err(p+".name", fmt.Sprintf("Повторяющееся имя %q", f.Name))
		}
		if !fieldTypes[f.Type] {
			v.err(p+".type", fmt.Sprintf("Неизвестный тип %q", f.Type))
		}
		if v.missingOffset[p] {
			v.err(p+".offset", "Ожидается число")
		}
		if !validEndian(f.Endian) {
			v.err(p+".endian", `Ожидается "le" или "be"`)
		}

		switch f.Type {
		case "array":
			if f.Count == "" || !ints[f.Count] {
				v.err(p+".count", "count должен ссылаться на предыдущее поле")
			}
			switch {
			case f.ItemSize == 0:
				v.err(p+".itemSize", "Не задан размер элемента")
			case f.ItemSize < 0:
				v.err(p+".itemSize", "Размер элемента должен быть положительным")
			}
			v.fields(f.Items, p+".items")
		case "bytes", "ascii":
			switch {
			case f.Length == nil:
				v.err(p+".length", "Не задана длина")
			case f.Length.Ref != "" && !ints[f.Length.Ref]:
				v.err(p+".length", "length должен ссылаться на предыдущее целочисленное поле")
			case f.Length.Ref == "" && f.Length.N < 0:
				v.err(p+".length", "Длина не может быть отрицательной")
			}
		}

		switch f.Checksum {
		case "", "sum8", "xor8":
		default:
			v.err(p+".checksum", "Ожидается sum8 или xor8")
		}
		if !meaningStatuses[f.Status] {
			v.err(p+".status", "Ожидается structural, hypothesis или confirmed")
		}

		names[f.Name] = true
		if intTypes[f.Type] {
			ints[f.Name] = true
		}
	}
}

func validEndian(e string) bool { return e == "" || e == "le" || e == "be" }
