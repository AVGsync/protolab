package engine

import (
	"strconv"
	"strings"

	"github.com/AVGsync/protolab/internal/spec"
)

// compiled is a spec prepared for repeated application: hex patterns decoded,
// field widths and references resolved, assertions grouped by rule. Compiling
// once per run keeps per-message work free of parsing and map lookups.
type compiled struct {
	framing framing
	rules   []rule
	// ruleIndex maps a rule id to its RuleStat slot. Like the reference
	// implementation, a duplicated id (rejected by validation) resolves to the
	// last rule with that id.
	ruleIndex map[string]int
}

type framing struct {
	kind string

	// length_prefixed
	lenOffset int
	lenSize   int
	lenBig    bool
	adjust    int64
	minLen    int64
	maxLen    int64
	sync      []byte

	delimiter []byte // delimiter
	size      int    // fixed
}

type rule struct {
	id         string
	title      string
	direction  string
	match      []matchCond
	fields     []field
	assertions []assertion
}

type matchCond struct {
	offset  int
	pattern []byte // nil when the hex is malformed: such a condition never holds
}

type fieldKind uint8

const (
	kindInvalid fieldKind = iota
	kindInt
	kindBytes
	kindASCII
	kindArray
)

type field struct {
	src  *spec.FieldRule
	kind fieldKind

	typeName  string // ParsedField.Type: "u8", "i16le", "bytes", ...
	size      int    // integer width in bytes
	signed    bool
	bigEndian bool

	decimals      int // rounding of scaled values
	meaningStatus string

	// enum is FieldRule.Enum keyed by value. Keys are matched against the
	// decimal text of the raw value, so "01" or "1.0" never match anything.
	enum    map[int64]string
	hasEnum bool

	// refs lists the earlier integer fields of the same level a count/length
	// reference may resolve to, latest first. At run time the first one that
	// produced a value wins, which is how a name-keyed scope behaves.
	refs  []int
	items []field
}

type assertion struct {
	id         string
	actionKind string
	hypothesis string
	checks     []check
}

type check struct {
	equals   string // "params.<name>" | "observed.<name>"
	fieldIdx int    // first top-level field with the checked name, -1 if none
}

// DefaultMaxLength is the largest length_prefixed message when the spec sets
// no maxLength: a 64 KiB payload with room for a header.
const DefaultMaxLength = 65_536 + 64

func compile(s *spec.Spec) *compiled {
	c := &compiled{ruleIndex: make(map[string]int)}
	if s == nil {
		return c
	}
	c.framing = compileFraming(&s.Framing)
	c.rules = make([]rule, len(s.Messages))
	for i := range s.Messages {
		c.rules[i] = compileRule(&s.Messages[i], s.Assertions)
		c.ruleIndex[s.Messages[i].ID] = i
	}
	return c
}

func compileFraming(f *spec.Framing) framing {
	out := framing{kind: f.Kind, size: f.Size}
	out.delimiter, _ = spec.ParseHex(f.Delimiter)
	out.sync, _ = spec.ParseHex(f.Sync)
	if lf := f.LengthField; lf != nil {
		out.lenOffset, out.lenSize, out.lenBig = lf.Offset, lf.Size, lf.Endian == "be"
	}
	if f.LengthAdjust != nil {
		out.adjust = int64(*f.LengthAdjust)
	}
	out.minLen = int64(out.lenOffset + out.lenSize)
	if f.MinLength != nil {
		out.minLen = int64(*f.MinLength)
	}
	// A zero-length message would never advance the framer.
	out.minLen = max(out.minLen, 1)
	out.maxLen = DefaultMaxLength
	if f.MaxLength != nil {
		out.maxLen = int64(*f.MaxLength)
	}
	return out
}

func compileRule(r *spec.MessageRule, assertions []spec.Assertion) rule {
	out := rule{
		id:        r.ID,
		title:     r.Title,
		direction: r.Direction,
		match:     make([]matchCond, len(r.Match)),
		fields:    compileFields(r.Fields),
	}
	for i, m := range r.Match {
		out.match[i].offset = m.Offset
		out.match[i].pattern, _ = spec.ParseHex(m.Hex)
	}
	for _, a := range assertions {
		if a.Rule != r.ID {
			continue
		}
		ca := assertion{id: a.ID, actionKind: a.ActionKind, hypothesis: a.Hypothesis, checks: make([]check, len(a.Checks))}
		for j, chk := range a.Checks {
			ca.checks[j] = check{equals: chk.Equals, fieldIdx: firstFieldNamed(r.Fields, chk.Field)}
		}
		out.assertions = append(out.assertions, ca)
	}
	return out
}

func compileFields(rules []spec.FieldRule) []field {
	out := make([]field, len(rules))
	for i := range rules {
		fr := &rules[i]
		f := field{src: fr, meaningStatus: spec.EffectiveMeaningStatus(fr)}
		switch {
		case spec.IsIntType(fr.Type):
			f.kind = kindInt
			f.size = intWidth(fr.Type)
			f.signed = fr.Type[0] == 'i'
			f.bigEndian = fr.Endian == "be"
			f.typeName = fr.Type
			if f.size > 1 {
				f.typeName += endianSuffix(f.bigEndian)
			}
		case fr.Type == "bytes":
			f.kind, f.typeName = kindBytes, fr.Type
		case fr.Type == "ascii":
			f.kind, f.typeName = kindASCII, fr.Type
		case fr.Type == "array":
			f.kind, f.typeName = kindArray, fr.Type
			f.items = compileFields(fr.Items)
		default:
			f.kind, f.typeName = kindInvalid, fr.Type
		}
		if fr.Scale != 0 {
			f.decimals = scaleDecimals(fr.Scale)
		}
		if fr.Enum != nil {
			f.hasEnum = true
			f.enum = make(map[int64]string, len(fr.Enum))
			for k, label := range fr.Enum {
				if v, err := strconv.ParseInt(k, 10, 64); err == nil && strconv.FormatInt(v, 10) == k {
					f.enum[v] = label
				}
			}
		}
		if ref := sizeRef(fr); ref != "" {
			for j := i - 1; j >= 0; j-- {
				if rules[j].Name == ref && spec.IsIntType(rules[j].Type) {
					f.refs = append(f.refs, j)
				}
			}
		}
		out[i] = f
	}
	return out
}

func sizeRef(fr *spec.FieldRule) string {
	switch fr.Type {
	case "array":
		return fr.Count
	case "bytes", "ascii":
		if fr.Length != nil {
			return fr.Length.Ref
		}
	}
	return ""
}

func intWidth(t string) int {
	switch {
	case strings.HasSuffix(t, "8"):
		return 1
	case strings.HasSuffix(t, "16"):
		return 2
	default:
		return 4
	}
}

func endianSuffix(big bool) string {
	if big {
		return "be"
	}
	return "le"
}

func firstFieldNamed(fields []spec.FieldRule, name string) int {
	for i := range fields {
		if fields[i].Name == name {
			return i
		}
	}
	return -1
}
