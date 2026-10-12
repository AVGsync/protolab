package engine

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// Field issue texts shared with the verification grouping.
const (
	issueOutOfBounds = "Поле выходит за границы сообщения"
	issueUnknown     = "Байты поля неизвестны (пропуск в записи)"
)

// fieldParser decodes the fields of one message. Its buffers are reused
// across messages of a session.
type fieldParser struct {
	msg     []byte
	unknown []byte // gap mask of msg, nil when the message has no gap bytes
	covered []byte // 1 for every byte claimed by a field

	// Scope of count/length references, one frame of len(fields) slots per
	// nesting level, pushed and popped like a stack.
	values []int64
	isSet  []bool
}

func (p *fieldParser) reset(msg, unknown []byte) {
	p.msg, p.unknown = msg, unknown
	if cap(p.covered) < len(msg) {
		p.covered = make([]byte, len(msg))
	} else {
		p.covered = p.covered[:len(msg)]
		clear(p.covered)
	}
	p.values, p.isSet = p.values[:0], p.isSet[:0]
}

// parse decodes one level of fields. base is the offset of the enclosing
// array item (0 for the message); negative field offsets always count from the
// end of the message.
func (p *fieldParser) parse(fields []field, base int, prefix string) []model.ParsedField {
	top := len(p.values)
	for range fields {
		p.values = append(p.values, 0)
		p.isSet = append(p.isSet, false)
	}
	out := make([]model.ParsedField, len(fields))
	for i := range fields {
		out[i] = p.parseField(&fields[i], base, prefix, top, top+i)
	}
	p.values, p.isSet = p.values[:top], p.isSet[:top]
	return out
}

func (p *fieldParser) parseField(f *field, base int, prefix string, top, slot int) model.ParsedField {
	src := f.src
	offset := addSat(base, src.Offset)
	if src.Offset < 0 {
		offset = len(p.msg) + src.Offset
	}
	length := p.sizeOf(f, top)
	pf := model.ParsedField{
		Path:          prefix + src.Name,
		Name:          src.Name,
		Offset:        offset,
		Length:        clampInt(length),
		Type:          f.typeName,
		Status:        model.FieldOK,
		Meaning:       src.Meaning,
		MeaningStatus: f.meaningStatus,
		HypothesisID:  src.Hypothesis,
	}

	if f.kind == kindInvalid {
		pf.Length, pf.Display = 0, "—"
		violate(&pf, fmt.Sprintf("Неизвестный тип %q", src.Type))
		return pf
	}
	if offset < 0 || length < 0 || offset > len(p.msg) || length > int64(len(p.msg)-offset) {
		pf.Display = "—"
		violate(&pf, issueOutOfBounds)
		return pf
	}
	end := offset + int(length)
	for i := offset; i < end; i++ {
		p.covered[i] = 1
	}
	if p.unknown != nil && bytes.IndexByte(p.unknown[offset:end], 1) >= 0 {
		pf.Status, pf.Display, pf.Issue = model.FieldUnknown, "??", issueUnknown
		return pf
	}

	raw := p.msg[offset:end]
	switch f.kind {
	case kindArray:
		p.parseArray(&pf, f, top)
	case kindBytes:
		pf.Value = hexString(raw, false)
		pf.Display = hexString(raw, true)
	case kindASCII:
		s := asciiString(raw)
		pf.Value = s
		pf.Display = strconv.Quote(s) // equals JSON.stringify for printable ASCII
	case kindInt:
		v := readInt(raw, f)
		p.values[slot], p.isSet[slot] = v, true
		p.describeInt(&pf, f, v, offset)
	}
	return pf
}

func (p *fieldParser) sizeOf(f *field, top int) int64 {
	switch f.kind {
	case kindInt:
		return int64(f.size)
	case kindArray:
		return mulSat(p.ref(f, top), int64(f.src.ItemSize))
	case kindBytes, kindASCII:
		switch l := f.src.Length; {
		case l == nil:
			return 0
		case l.Ref != "":
			return p.ref(f, top)
		default:
			return int64(l.N)
		}
	}
	return 0
}

// ref resolves a count/length reference; an unresolved one is 0.
func (p *fieldParser) ref(f *field, top int) int64 {
	for _, j := range f.refs {
		if p.isSet[top+j] {
			return p.values[top+j]
		}
	}
	return 0
}

func (p *fieldParser) parseArray(pf *model.ParsedField, f *field, top int) {
	count := p.ref(f, top)
	itemSize := f.src.ItemSize
	pf.Value = count
	pf.Display = fmt.Sprintf("%d × %d Б", count, itemSize)
	if itemSize <= 0 && count > 0 {
		// Without a positive item size the items do not consume the message
		// and their number is unbounded.
		violate(pf, "Не задан размер элемента")
		return
	}

	// count*itemSize fits into the message here, so count is small.
	n := int(max(count, 0))
	pf.Children = make([]model.ParsedField, n)
	for i := range n {
		itemBase := pf.Offset + i*itemSize
		path := pf.Path + "[" + strconv.Itoa(i) + "]"
		item := model.ParsedField{
			Path:   path,
			Name:   "[" + strconv.Itoa(i) + "]",
			Offset: itemBase,
			Length: itemSize,
			Type:   "struct",
			Status: model.FieldOK,
		}
		item.Children = p.parse(f.items, itemBase, path+".")
		if hasViolation(item.Children) {
			item.Status = model.FieldViolation
		}
		pf.Children[i] = item
	}
	if hasViolation(pf.Children) {
		pf.Status = model.FieldViolation
	}
}

func (p *fieldParser) describeInt(pf *model.ParsedField, f *field, v int64, offset int) {
	src := f.src
	pf.Value = v
	raw := strconv.FormatInt(v, 10)

	label, inEnum := src.Enum[raw]
	switch {
	case src.Enum != nil:
		if !inEnum {
			label = "?"
		}
		pf.Display = label + " (" + raw + ")"
	case src.Scale != 0:
		pf.Display = jsNumber(roundFixed(float64(v)*src.Scale, f.decimals))
	default:
		pf.Display = raw
	}
	if src.Enum == nil && src.Unit != "" {
		pf.Display += " " + src.Unit
	}

	// Later checks overwrite the issue of earlier ones, as in the reference.
	if src.Enum != nil && !inEnum {
		violate(pf, fmt.Sprintf("Значение %d отсутствует в перечислении", v))
	}
	if issue := expectIssue(src.Expect, v); issue != "" {
		violate(pf, issue)
	}
	if issue := p.checksumIssue(src.Checksum, v, offset); issue != "" {
		violate(pf, issue)
	}
}

// expectIssue checks the raw value; the first failing condition is reported.
func expectIssue(e *spec.Expect, v int64) string {
	if e == nil {
		return ""
	}
	x := float64(v)
	switch {
	case e.Equals != nil && x != *e.Equals:
		return fmt.Sprintf("Ожидалось %s, получено %d", jsNumber(*e.Equals), v)
	case e.Min != nil && x < *e.Min:
		return fmt.Sprintf("Значение %d меньше минимума %s", v, jsNumber(*e.Min))
	case e.Max != nil && x > *e.Max:
		return fmt.Sprintf("Значение %d больше максимума %s", v, jsNumber(*e.Max))
	case e.OneOf != nil && !containsFloat(e.OneOf, x):
		return fmt.Sprintf("Значение %d не входит в {%s}", v, joinNumbers(e.OneOf))
	}
	return ""
}

// checksumIssue verifies sum8/xor8 over the message bytes before the field.
// When some of those bytes are unknown the checksum cannot be verified, and
// claiming a mismatch would blame the protocol for a capture defect.
func (p *fieldParser) checksumIssue(algo string, v int64, offset int) string {
	if algo == "" {
		return ""
	}
	if p.unknown != nil && bytes.IndexByte(p.unknown[:offset], 1) >= 0 {
		return ""
	}
	var want byte
	switch algo {
	case "sum8":
		for _, b := range p.msg[:offset] {
			want += b
		}
	case "xor8":
		for _, b := range p.msg[:offset] {
			want ^= b
		}
	default:
		return fmt.Sprintf("Неизвестный алгоритм контрольной суммы %q", algo)
	}
	if int64(want) == v {
		return ""
	}
	return fmt.Sprintf("Контрольная сумма %s: ожидалось 0x%X", algo, want)
}

func violate(pf *model.ParsedField, issue string) {
	pf.Status, pf.Issue = model.FieldViolation, issue
}

func readInt(b []byte, f *field) int64 {
	var u uint32
	switch {
	case f.size == 1:
		u = uint32(b[0])
	case f.size == 2 && f.bigEndian:
		u = uint32(binary.BigEndian.Uint16(b))
	case f.size == 2:
		u = uint32(binary.LittleEndian.Uint16(b))
	case f.bigEndian:
		u = binary.BigEndian.Uint32(b)
	default:
		u = binary.LittleEndian.Uint32(b)
	}
	if !f.signed {
		return int64(u)
	}
	switch f.size {
	case 1:
		return int64(int8(u))
	case 2:
		return int64(int16(u))
	default:
		return int64(int32(u))
	}
}

func hasViolation(fields []model.ParsedField) bool {
	for i := range fields {
		if fields[i].Status == model.FieldViolation {
			return true
		}
	}
	return false
}

func containsFloat(list []float64, x float64) bool {
	for _, v := range list {
		if v == x {
			return true
		}
	}
	return false
}

func joinNumbers(list []float64) string {
	buf := make([]byte, 0, len(list)*4)
	for i, v := range list {
		if i > 0 {
			buf = append(buf, ", "...)
		}
		buf = append(buf, jsNumber(v)...)
	}
	return string(buf)
}

func addSat(a, b int) int {
	if b > 0 && a > math.MaxInt-b {
		return math.MaxInt
	}
	if b < 0 && a < math.MinInt-b {
		return math.MinInt
	}
	return a + b
}

func mulSat(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	p := a * b
	if p/b != a || (a == -1 && b == math.MinInt64) || (b == -1 && a == math.MinInt64) {
		if (a < 0) != (b < 0) {
			return math.MinInt64
		}
		return math.MaxInt64
	}
	return p
}

func clampInt(v int64) int {
	return int(max(min(v, math.MaxInt), math.MinInt))
}
