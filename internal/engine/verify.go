package engine

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// maxExamples bounds Finding.Examples.
const maxExamples = 5

// findingOrder ranks finding kinds in the report, most important first.
var findingOrder = map[string]int{
	model.FindingViolation:        0,
	model.FindingPartial:          1,
	model.FindingAmbiguous:        2,
	model.FindingCaptureDefect:    3,
	model.FindingUnknownType:      4,
	model.FindingUnknownRegion:    5,
	model.FindingInapplicableRule: 6,
}

// testHookSession, when set, runs before a session is verified. Tests use it
// to simulate a failure inside the engine.
var testHookSession func(SessionInput)

func verify(ctx context.Context, s *spec.Spec, inputs []SessionInput, progress func(done, total int)) (Result, error) {
	c := compile(s)
	total := newTally(len(c.rules))
	sessionsWithErrors := 0

	for i := range inputs {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		in := &inputs[i]
		t, err := c.verifySession(ctx, in)
		switch {
		case err == nil:
			total.merge(t)
			if t.hasErrors {
				sessionsWithErrors++
			}
		case ctx.Err() != nil:
			return Result{}, ctx.Err()
		default:
			// A failure in one session must not block the rest of the corpus.
			sessionsWithErrors++
			total.bucket("engine:"+in.Session.ID, model.Finding{
				Kind:     model.FindingCaptureDefect,
				Severity: model.LevelError,
				Message:  "Ошибка обработки сессии: " + err.Error(),
			})
		}
		if progress != nil {
			progress(i+1, len(inputs))
		}
	}
	return total.result(c, len(inputs), sessionsWithErrors), nil
}

// sessionPanic is a panic recovered while verifying one session.
type sessionPanic struct{ value any }

func (p sessionPanic) Error() string {
	if err, ok := p.value.(error); ok {
		return err.Error()
	}
	return fmt.Sprint(p.value)
}

// verifySession tallies one session on its own, so that a failure halfway
// through leaves the corpus totals untouched.
func (c *compiled) verifySession(ctx context.Context, in *SessionInput) (t *tally, err error) {
	defer func() {
		if r := recover(); r != nil {
			t, err = nil, sessionPanic{r}
		}
	}()
	if testHookSession != nil {
		testHookSession(*in)
	}

	t = newTally(len(c.rules))
	sess := c.newSession(in, false)
	t.captureDefects(sess)
	for _, u := range sess.unframed {
		t.unknownBytes += u.Length
		f := t.bucket("unframed", model.Finding{
			Kind:     model.FindingUnknownRegion,
			Severity: model.LevelWarning,
			Message:  "Байты вне сообщений: синхронизация кадров потеряна (начало не захвачено или пропуск)",
		})
		t.example(f, sess, sess.streamOf(u.Direction), u.Offset, u.Length)
	}

	b := c.newBuilder(in)
	for i, ref := range sess.order {
		if i&0xfff == 0xfff && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		st, f := sess.frame(ref)
		m := b.message(st, f)
		t.message(c, sess, st, &m)
	}
	return t, nil
}

func (s *session) streamOf(d model.Direction) *stream {
	if d == model.C2S {
		return s.streams[0]
	}
	return s.streams[1]
}

// ref points a finding example at stream bytes of the session.
func (s *session) ref(st *stream, offset, length int) model.SourceRef {
	return model.SourceRef{
		CaptureID:     s.in.Session.CaptureID,
		CaptureSHA256: s.in.Session.Capture.SHA256,
		SessionID:     s.in.Session.ID,
		Direction:     st.dir,
		Offset:        offset,
		Length:        max(1, length),
		PacketNumbers: st.segs.packets(offset, length),
	}
}

// tally accumulates verification results: of one session, or of the corpus.
type tally struct {
	keys     map[string]int  // finding key -> index into findings
	keyOrder []string        // keys in order of first occurrence
	findings []model.Finding // in order of first occurrence

	messages, ok, violations, partial, ambiguous, unknownType int
	unknownBytes, totalBytes                                  int
	matched, ruleViolations                                   []int
	hasErrors                                                 bool
}

func newTally(rules int) *tally {
	return &tally{keys: make(map[string]int), matched: make([]int, rules), ruleViolations: make([]int, rules)}
}

// bucket counts one occurrence of the finding identified by key; init
// describes the finding when the key is seen for the first time.
func (t *tally) bucket(key string, init model.Finding) *model.Finding {
	i, ok := t.keys[key]
	if !ok {
		i = len(t.findings)
		t.keys[key] = i
		t.keyOrder = append(t.keyOrder, key)
		init.Examples = []model.SourceRef{}
		t.findings = append(t.findings, init)
	}
	f := &t.findings[i]
	f.Occurrences++
	return f
}

func (t *tally) example(f *model.Finding, s *session, st *stream, offset, length int) {
	if len(f.Examples) < maxExamples {
		f.Examples = append(f.Examples, s.ref(st, offset, length))
	}
}

// merge adds a session tally. Keys new to t are appended in the order the
// session first saw them and examples are taken in session order, which is
// exactly what tallying the session directly into t would produce.
func (t *tally) merge(o *tally) {
	t.messages += o.messages
	t.ok += o.ok
	t.violations += o.violations
	t.partial += o.partial
	t.ambiguous += o.ambiguous
	t.unknownType += o.unknownType
	t.unknownBytes += o.unknownBytes
	t.totalBytes += o.totalBytes
	for i := range t.matched {
		t.matched[i] += o.matched[i]
		t.ruleViolations[i] += o.ruleViolations[i]
	}
	for i, key := range o.keyOrder {
		of := o.findings[i]
		j, ok := t.keys[key]
		if !ok {
			t.keys[key] = len(t.findings)
			t.keyOrder = append(t.keyOrder, key)
			t.findings = append(t.findings, of)
			continue
		}
		f := &t.findings[j]
		f.Occurrences += of.Occurrences
		room := maxExamples - len(f.Examples)
		f.Examples = append(f.Examples, of.Examples[:min(room, len(of.Examples))]...)
	}
}

func (t *tally) captureDefects(s *session) {
	for _, st := range s.streams {
		for _, g := range st.gaps {
			t.hasErrors = true
			msg := "Потерянные сегменты — участки потока неизвестны"
			if g.Reason == model.GapTruncated {
				msg = "Пакеты усечены при захвате — хвосты неизвестны"
			}
			f := t.bucket("gap:"+g.Reason, model.Finding{Kind: model.FindingCaptureDefect, Severity: model.LevelWarning, Message: msg})
			t.example(f, s, st, g.Offset, g.Length)
		}
	}
	if s.in.Session.Flags.OverlapConflicts > 0 {
		t.hasErrors = true
		f := t.bucket("conflict", model.Finding{Kind: model.FindingCaptureDefect, Severity: model.LevelWarning, Message: "Повторные передачи с разными данными"})
		// Conflict ranges are not part of the input; the example points at the session.
		t.example(f, s, s.streams[0], 0, 1)
	}
}

func (t *tally) message(c *compiled, s *session, st *stream, m *model.Message) {
	t.messages++
	t.totalBytes += m.Length
	ruleID := "null"
	stat := -1
	if m.RuleID != nil {
		ruleID = *m.RuleID
		if i, ok := c.ruleIndex[ruleID]; ok {
			stat = i
			t.matched[i]++
		}
	}

	switch m.Status {
	case model.MsgOK:
		t.ok++
	case model.MsgViolation:
		t.violations++
		if stat >= 0 {
			t.ruleViolations[stat]++
		}
	case model.MsgPartial:
		t.partial++
		msg := "Сообщение неполное"
		if len(m.Issues) > 0 {
			msg = m.Issues[0]
		}
		f := t.bucket("partial:"+ruleID, model.Finding{Kind: model.FindingPartial, Severity: model.LevelWarning, RuleID: derefOr(m.RuleID), Message: msg})
		t.example(f, s, st, m.Offset, m.Length)
	case model.MsgAmbiguous:
		t.ambiguous++
		f := t.bucket("ambiguous:"+strings.Join(m.CandidateRuleIDs, ","), model.Finding{
			Kind:     model.FindingAmbiguous,
			Severity: model.LevelWarning,
			Message:  "Подходят несколько правил: " + strings.Join(m.CandidateRuleIDs, ", "),
		})
		t.example(f, s, st, m.Offset, m.Length)
	case model.MsgUnknownType:
		t.unknownType++
		head := messageHead(st, m)
		f := t.bucket("unknown_type:"+string(m.Direction)+":"+head, model.Finding{
			Kind:     model.FindingUnknownType,
			Severity: model.LevelInfo,
			Message:  directionLabel(m.Direction) + ": сообщения «" + head + " …» не описаны ни одним правилом",
		})
		t.example(f, s, st, m.Offset, m.Length)
	}
	if m.Status != model.MsgOK && m.Status != model.MsgUnknownType {
		t.hasErrors = true
	}

	t.fieldViolations(s, st, m, ruleID, m.Fields)

	switch {
	case m.RuleID != nil && m.Status != model.MsgPartial:
		for _, u := range m.Unknown {
			t.unknownRegion(s, st, m, ruleID, u)
		}
	case m.RuleID == nil:
		t.unknownBytes += m.Length
	}
}

func (t *tally) fieldViolations(s *session, st *stream, m *model.Message, ruleID string, fields []model.ParsedField) {
	for i := range fields {
		pf := &fields[i]
		if pf.Status == model.FieldViolation {
			path := genericPath(pf.Path)
			msg := pf.Issue
			if msg == "" {
				msg = "Нарушение правила"
			}
			f := t.bucket("violation:"+ruleID+":"+path+":"+issueKind(pf.Issue), model.Finding{
				Kind:      model.FindingViolation,
				Severity:  model.LevelError,
				RuleID:    derefOr(m.RuleID),
				FieldPath: path,
				Message:   msg,
			})
			offset, length := m.Offset+pf.Offset, pf.Length
			if pf.Offset < 0 || pf.Length < 0 || pf.Offset+pf.Length > m.Length {
				// The field does not fit: point at the message it was expected in.
				offset, length = m.Offset, m.Length
			}
			t.example(f, s, st, offset, length)
		}
		t.fieldViolations(s, st, m, ruleID, pf.Children)
	}
}

func (t *tally) unknownRegion(s *session, st *stream, m *model.Message, ruleID string, u model.ByteRange) {
	t.unknownBytes += u.Length
	fromEnd := u.End() == m.Length
	where := "байты " + strconv.Itoa(u.Offset) + "–" + strconv.Itoa(u.End()-1)
	if fromEnd && u.Length == 1 {
		where = "последний байт"
	}
	// Regions at the end of variable-length messages share a key by their size.
	pos := strconv.Itoa(u.Offset)
	if fromEnd {
		pos = "end-" + strconv.Itoa(u.Length)
	}
	f := t.bucket("unknown_region:"+ruleID+":"+pos, model.Finding{
		Kind:     model.FindingUnknownRegion,
		Severity: model.LevelInfo,
		RuleID:   ruleID,
		Message:  ruleID + ": " + where + " не описаны полями",
	})
	t.example(f, s, st, m.Offset+u.Offset, u.Length)
}

func (t *tally) result(c *compiled, sessions, sessionsWithErrors int) Result {
	sum := model.RunSummary{
		Sessions:           sessions,
		SessionsWithErrors: sessionsWithErrors,
		Messages:           t.messages,
		OK:                 t.ok,
		Violations:         t.violations,
		Partial:            t.partial,
		Ambiguous:          t.ambiguous,
		UnknownType:        t.unknownType,
		UnknownBytes:       t.unknownBytes,
		TotalBytes:         t.totalBytes,
		InapplicableRules:  []string{},
		ByRule:             make([]model.RuleStat, len(c.rules)),
	}
	for i, r := range c.rules {
		sum.ByRule[i] = model.RuleStat{RuleID: r.id, Matched: t.matched[i], Violations: t.ruleViolations[i]}
		if t.matched[i] == 0 {
			sum.InapplicableRules = append(sum.InapplicableRules, r.id)
		}
	}
	for _, id := range sum.InapplicableRules {
		t.bucket("inapplicable:"+id, model.Finding{
			Kind:     model.FindingInapplicableRule,
			Severity: model.LevelInfo,
			RuleID:   id,
			Message:  "Правило «" + id + "» не сработало ни на одном сообщении корпуса — его корректность не проверена",
		})
	}

	findings := t.findings
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if findingOrder[a.Kind] != findingOrder[b.Kind] {
			return findingOrder[a.Kind] < findingOrder[b.Kind]
		}
		return a.Occurrences > b.Occurrences
	})
	if findings == nil {
		findings = []model.Finding{}
	}
	return Result{Summary: sum, Findings: findings}
}

// messageHead shows the first two bytes of a message, "??" for unknown ones.
func messageHead(st *stream, m *model.Message) string {
	n := min(2, len(m.Data))
	parts := make([]string, n)
	for i := range n {
		if st.unknown != nil && st.unknown[m.Offset+i] == 1 {
			parts[i] = "??"
		} else {
			parts[i] = hexString(m.Data[i:i+1], false)
		}
	}
	return strings.Join(parts, " ")
}

func directionLabel(d model.Direction) string {
	if d == model.C2S {
		return "C→S"
	}
	return "S→C"
}

// genericPath replaces array indices so that all items share a finding:
// "entries[3].value" -> "entries[].value".
func genericPath(path string) string {
	if !strings.Contains(path, "[") {
		return path
	}
	var sb strings.Builder
	sb.Grow(len(path))
	for i := 0; i < len(path); i++ {
		if path[i] == '[' {
			j := i + 1
			for j < len(path) && path[j] >= '0' && path[j] <= '9' {
				j++
			}
			if j > i+1 && j < len(path) && path[j] == ']' {
				sb.WriteString("[]")
				i = j
				continue
			}
		}
		sb.WriteByte(path[i])
	}
	return sb.String()
}

// issueKind groups violations of one field by the kind of check that failed:
// assertions by their "≠", everything else by the first word of the issue.
func issueKind(issue string) string {
	if strings.Contains(issue, "≠") {
		return "assertion"
	}
	kind, _, _ := strings.Cut(issue, " ")
	return kind
}

func derefOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
