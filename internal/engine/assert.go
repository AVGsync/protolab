package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
)

// boundAssertion is an assertion together with the action of the session it
// is checked against and the expected values read from that action.
type boundAssertion struct {
	id, hypothesis string
	actionLabel    string
	checks         []boundCheck
}

type boundCheck struct {
	fieldIdx int
	equals   string
	expected float64
}

// bindAssertions resolves, for every rule, the assertions that apply in a
// session: the first linked action of the asserted kind provides the
// experiment conditions. Checks whose condition is missing or not numeric
// cannot be evaluated and are dropped.
func (c *compiled) bindAssertions(actions []model.ActionEntry) [][]boundAssertion {
	if len(actions) == 0 {
		return nil
	}
	bound := make([][]boundAssertion, len(c.rules))
	for ri := range c.rules {
		for _, a := range c.rules[ri].assertions {
			action := firstAction(actions, a.actionKind)
			if action == nil {
				continue
			}
			ba := boundAssertion{id: a.id, hypothesis: a.hypothesis, actionLabel: action.Label}
			for _, chk := range a.checks {
				expected, ok := actionValue(action, chk.equals)
				if !ok || chk.fieldIdx < 0 {
					continue
				}
				ba.checks = append(ba.checks, boundCheck{fieldIdx: chk.fieldIdx, equals: chk.equals, expected: expected})
			}
			bound[ri] = append(bound[ri], ba)
		}
	}
	return bound
}

func firstAction(actions []model.ActionEntry, kind string) *model.ActionEntry {
	for i := range actions {
		if actions[i].Kind == kind {
			return &actions[i]
		}
	}
	return nil
}

// applyAssertions compares top-level fields with the experiment conditions.
// The field value is taken with its scale, as the researcher sees it.
func applyAssertions(m *model.Message, r *rule, bound []boundAssertion) {
	for _, a := range bound {
		for _, chk := range a.checks {
			pf := &m.Fields[chk.fieldIdx]
			raw, ok := pf.Value.(int64)
			if !ok || pf.Status == model.FieldUnknown {
				continue
			}
			actual := float64(raw)
			if f := &r.fields[chk.fieldIdx]; f.src.Scale != 0 {
				actual = roundFixed(actual*f.src.Scale, f.decimals)
			}
			if math.Abs(actual-chk.expected) <= 1e-6 {
				continue
			}
			pf.Status = model.FieldViolation
			pf.Issue = fmt.Sprintf("%s ≠ %s = %s (действие «%s», проверка %s)",
				jsNumber(actual), chk.equals, jsNumber(chk.expected), a.actionLabel, a.id)
			if pf.HypothesisID == "" {
				pf.HypothesisID = a.hypothesis
			}
		}
	}
}

// actionValue reads "params.<name>" or "observed.<name>" of an action as a
// number. Values are JSON numbers or numeric strings ("21", " -5.5 ", "0x15").
func actionValue(a *model.ActionEntry, path string) (float64, bool) {
	root, rest, _ := strings.Cut(path, ".")
	key, _, _ := strings.Cut(rest, ".")
	var bag map[string]any
	switch root {
	case "params":
		bag = a.Params
	case "observed":
		bag = a.Observed
	default:
		return 0, false
	}
	v, ok := bag[key]
	if !ok {
		return 0, false
	}
	return toNumber(v)
}

func toNumber(v any) (float64, bool) {
	var x float64
	switch n := v.(type) {
	case float64:
		x = n
	case float32:
		x = float64(n)
	case int:
		x = float64(n)
	case int8:
		x = float64(n)
	case int16:
		x = float64(n)
	case int32:
		x = float64(n)
	case int64:
		x = float64(n)
	case uint:
		x = float64(n)
	case uint8:
		x = float64(n)
	case uint16:
		x = float64(n)
	case uint32:
		x = float64(n)
	case uint64:
		x = float64(n)
	case json.Number:
		return parseNumber(string(n))
	case string:
		return parseNumber(n)
	default:
		return 0, false
	}
	return x, !math.IsNaN(x) && !math.IsInf(x, 0)
}

// parseNumber accepts what JavaScript's Number() accepts for practical
// inputs: surrounding blanks, a sign, decimals, exponents and 0x literals.
// An empty string is not a number here, although Number("") is 0.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if len(s) > 2 && s[0] == '0' && (s[1] == 'x' || s[1] == 'X') {
		u, err := strconv.ParseUint(s[2:], 16, 64)
		return float64(u), err == nil
	}
	x, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(x) || math.IsInf(x, 0) {
		return 0, false
	}
	return x, true
}
