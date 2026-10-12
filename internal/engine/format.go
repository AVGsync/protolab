package engine

import (
	"bytes"
	"math"
	"strconv"
	"strings"
)

// The reference implementation runs in JavaScript, so displayed numbers follow
// JavaScript conventions: Number.prototype.toString for printing and
// Number.prototype.toFixed for rounding scaled values.

// jsNumber formats v like Number.prototype.toString: the shortest digits that
// round-trip, plain notation for 1e-6 <= |v| < 1e21 (41, 22.4, 0.000001) and
// exponent notation otherwise (1e+21, 1.5e-7).
func jsNumber(v float64) string {
	if v == 0 {
		return "0" // also -0
	}
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "Infinity"
	case math.IsInf(v, -1):
		return "-Infinity"
	}
	if a := math.Abs(v); a >= 1e-6 && a < 1e21 {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	// strconv pads the exponent to two digits ("1.5e-07"); JavaScript does not.
	mant, exp, _ := strings.Cut(strconv.FormatFloat(v, 'e', -1, 64), "e")
	return mant + "e" + exp[:1] + strings.TrimLeft(exp[1:], "0")
}

// maxFixedDigits is the largest precision toFixed accepts.
const maxFixedDigits = 100

// roundFixed returns Number(v.toFixed(decimals)). toFixed rounds the exact
// binary value of v and resolves exact ties away from zero, while strconv
// resolves them to even (0.25 -> "0.3" in JavaScript, "0.2" in Go), so ties are
// detected and nudged up before formatting.
func roundFixed(v float64, decimals int) float64 {
	a := math.Abs(v)
	if math.IsNaN(v) || math.IsInf(v, 0) || a >= 1e21 {
		return v // toFixed falls back to toString here
	}
	if isTie(a, decimals) {
		a = math.Nextafter(a, math.Inf(1))
	}
	r, err := strconv.ParseFloat(strconv.FormatFloat(a, 'f', decimals, 64), 64)
	if err != nil {
		return v
	}
	return math.Copysign(r, v)
}

// formatScaled returns jsNumber(roundFixed(v, decimals)).
func formatScaled(v float64, decimals int) string {
	var buf [32]byte
	return string(appendScaled(buf[:0], v, decimals))
}

// appendScaled appends jsNumber(roundFixed(v, decimals)) to dst without the
// round trip through float64 in the common case: when the toFixed digits have
// at most 15 significant digits they convert to a double and back unchanged,
// so trimming their trailing zeros already yields the shortest form.
func appendScaled(dst []byte, v float64, decimals int) []byte {
	a := math.Abs(v)
	if math.IsNaN(v) || a >= 1e21 {
		return append(dst, jsNumber(roundFixed(v, decimals))...)
	}
	if isTie(a, decimals) {
		a = math.Nextafter(a, math.Inf(1))
	}
	start := len(dst)
	if v < 0 {
		dst = append(dst, '-')
	}
	digitsStart := len(dst)
	dst = strconv.AppendFloat(dst, a, 'f', decimals, 64)
	if bytes.IndexByte(dst[digitsStart:], '.') >= 0 {
		dst = bytes.TrimRight(dst, "0")
		dst = bytes.TrimSuffix(dst, []byte("."))
	}
	num := dst[digitsStart:]
	if len(num) == 1 && num[0] == '0' {
		return append(dst[:start], '0') // also -0
	}
	intPart, frac, _ := bytes.Cut(num, []byte("."))
	significant := len(intPart) + len(frac)
	if len(intPart) == 1 && intPart[0] == '0' {
		fracDigits := bytes.TrimLeft(frac, "0")
		if len(frac)-len(fracDigits) >= 6 {
			significant = math.MaxInt // below 1e-6: exponent notation
		} else {
			significant = len(fracDigits)
		}
	}
	if significant > 15 {
		return append(dst[:start], jsNumber(roundFixed(v, decimals))...)
	}
	return dst
}

// isTie reports whether a*10^d lies exactly halfway between two integers, i.e.
// whether a*2^(d+1)*5^d is an odd integer. Writing a = m*2^q with m odd, that
// product is m*5^d*2^(q+d+1), which is an odd integer exactly when
// a*2^(d+1) = m is. Ldexp computes the latter without rounding.
func isTie(a float64, d int) bool {
	t := math.Ldexp(a, d+1)
	return t == math.Trunc(t) && math.Mod(t, 2) == 1
}

// scaleDecimals is max(0, ceil(-log10(scale))): the number of decimals a
// scaled value is rounded to. math.Log10 may be off by an ulp near powers of
// ten, so the estimate is corrected against correctly rounded 10^-d.
func scaleDecimals(scale float64) int {
	s := math.Abs(scale)
	if s == 0 || math.IsNaN(s) || math.IsInf(s, 0) {
		return 0
	}
	d := max(0, int(math.Ceil(-math.Log10(s))))
	for d > 0 && s >= negPow10(d-1) {
		d--
	}
	for d < maxFixedDigits && s < negPow10(d) {
		d++
	}
	return min(d, maxFixedDigits)
}

// negPow10 returns the double nearest to 10^-d.
func negPow10(d int) float64 {
	v, _ := strconv.ParseFloat("1e-"+strconv.Itoa(d), 64)
	return v
}

const hexDigits = "0123456789ABCDEF"

// hexString encodes b as uppercase hex, optionally separating bytes with a space.
func hexString(b []byte, spaced bool) string {
	if len(b) == 0 {
		return ""
	}
	var sb strings.Builder
	if spaced {
		sb.Grow(len(b)*3 - 1)
	} else {
		sb.Grow(len(b) * 2)
	}
	for i, c := range b {
		if spaced && i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteByte(hexDigits[c>>4])
		sb.WriteByte(hexDigits[c&0x0f])
	}
	return sb.String()
}

// asciiString shows printable ASCII as is and every other byte as '.'.
func asciiString(b []byte) string {
	buf := make([]byte, len(b))
	for i, c := range b {
		if c >= 0x20 && c < 0x7f {
			buf[i] = c
		} else {
			buf[i] = '.'
		}
	}
	return string(buf)
}
