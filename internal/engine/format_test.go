package engine

import (
	"math"
	"testing"
)

func TestJSNumber(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "0"},
		{41, "41"},
		{-5.5, "-5.5"},
		{22.4, "22.4"},
		{add(0.1, 0.2), "0.30000000000000004"},
		{1e21, "1e+21"},
		{123456789012345680000, "123456789012345680000"},
		{1e-6, "0.000001"},
		{1e-7, "1e-7"},
		{-1.5e-7, "-1.5e-7"},
		{4.294967295e24, "4.294967295e+24"},
		{math.Inf(1), "Infinity"},
		{math.NaN(), "NaN"},
	}
	for _, tt := range tests {
		if got := jsNumber(tt.in); got != tt.want {
			t.Errorf("jsNumber(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// add keeps the sum out of exact constant arithmetic.
func add(a, b float64) float64 { return a + b }

func TestRoundFixed(t *testing.T) {
	tests := []struct {
		v        float64
		decimals int
		want     float64
	}{
		{22.400000000000002, 1, 22.4},
		{0.25, 1, 0.3},   // exact tie: away from zero, unlike strconv
		{-0.25, 1, -0.3}, // sign is applied after rounding the magnitude
		{2.5, 0, 3},
		{0.35, 1, 0.3}, // 0.35 is slightly below the tie in binary
		{1.005, 2, 1},  // likewise 1.00499999...
		{0.375, 1, 0.4},
		{1e22, 0, 1e22},
		{-0.04, 1, 0},
	}
	for _, tt := range tests {
		if got := roundFixed(tt.v, tt.decimals); got != tt.want {
			t.Errorf("roundFixed(%v, %d) = %v, want %v", tt.v, tt.decimals, got, tt.want)
		}
	}
}

func TestScaleDecimals(t *testing.T) {
	tests := []struct {
		scale float64
		want  int
	}{
		{1, 0}, {10, 0}, {0.5, 1}, {0.1, 1}, {0.25, 1}, {0.05, 2}, {0.01, 2},
		{0.001, 3}, {0.0025, 3}, {1e-7, 7}, {-0.5, 1}, {1e-300, 100},
	}
	for _, tt := range tests {
		if got := scaleDecimals(tt.scale); got != tt.want {
			t.Errorf("scaleDecimals(%v) = %d, want %d", tt.scale, got, tt.want)
		}
	}
}

func TestToNumber(t *testing.T) {
	tests := []struct {
		in   any
		want float64
		ok   bool
	}{
		{21.0, 21, true},
		{int(7), 7, true},
		{"21", 21, true},
		{" -5.5 ", -5.5, true},
		{"1e1", 10, true},
		{"0x15", 21, true},
		{"", 0, false},
		{"Infinity", 0, false},
		{"abc", 0, false},
		{true, 0, false},
		{nil, 0, false},
	}
	for _, tt := range tests {
		got, ok := toNumber(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("toNumber(%#v) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestGenericPathAndIssueKind(t *testing.T) {
	paths := map[string]string{
		"value":                 "value",
		"entries[3].value":      "entries[].value",
		"a[10].b[2].c":          "a[].b[].c",
		"odd[x]":                "odd[x]",
		"entries[12]":           "entries[]",
		"unterminated[12":       "unterminated[12",
		"entries[1].items[0]x":  "entries[].items[]x",
		"name-with-[]-brackets": "name-with-[]-brackets",
	}
	for in, want := range paths {
		if got := genericPath(in); got != want {
			t.Errorf("genericPath(%q) = %q, want %q", in, got, want)
		}
	}
	kinds := map[string]string{
		"Значение 7 отсутствует в перечислении":             "Значение",
		"21 ≠ params.value = 22 (действие «x», проверка a)": "assertion",
		"": "",
	}
	for in, want := range kinds {
		if got := issueKind(in); got != want {
			t.Errorf("issueKind(%q) = %q, want %q", in, got, want)
		}
	}
}
