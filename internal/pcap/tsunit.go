package pcap

import (
	"math/bits"
	"time"
)

// tsUnit is a pcapng timestamp resolution (if_tsresol): one unit is 10^-exp
// seconds, or 2^-exp seconds when binary is set.
type tsUnit struct {
	binary bool
	exp    uint8 // 0..127
}

// time converts a count of units since the epoch, plus offset seconds, exactly
// (no floating point), down to the nanosecond.
func (u tsUnit) time(ts uint64, offset int64) time.Time {
	var sec, nsec uint64
	if u.binary {
		sec, nsec = splitBinary(ts, uint(u.exp))
	} else {
		sec, nsec = splitDecimal(ts, int(u.exp))
	}
	return time.Unix(int64(sec)+offset, int64(nsec))
}

// duration is the length of one unit, rounded to the nanosecond (at least 1ns).
func (u tsUnit) duration() time.Duration {
	if u.binary {
		if u.exp >= 31 {
			return time.Nanosecond
		}
		half := uint64(1) << u.exp >> 1
		return time.Duration(max((1_000_000_000+half)>>u.exp, 1))
	}
	if u.exp > 9 {
		return time.Nanosecond
	}
	d, _ := pow10(9 - int(u.exp))
	return time.Duration(d)
}

func splitDecimal(ts uint64, exp int) (sec, nsec uint64) {
	div, ok := pow10(exp)
	if ok {
		sec, ts = ts/div, ts%div
	}
	// ts now holds the sub-second part: frac / 10^exp seconds.
	if exp <= 9 {
		m, _ := pow10(9 - exp)
		return sec, ts * m
	}
	if d, ok := pow10(exp - 9); ok {
		return sec, ts / d
	}
	return sec, 0
}

func splitBinary(ts uint64, exp uint) (sec, nsec uint64) {
	if exp >= 64 {
		sec = 0
	} else {
		sec, ts = ts>>exp, ts&(1<<exp-1)
	}
	// nsec = ts * 10^9 / 2^exp; the product needs up to 94 bits.
	hi, lo := bits.Mul64(ts, 1_000_000_000)
	switch {
	case exp == 0:
		return sec, lo
	case exp < 64:
		return sec, lo>>exp | hi<<(64-exp)
	default:
		return sec, hi >> (exp - 64)
	}
}

// pow10 returns 10^n and false when it does not fit in a uint64.
func pow10(n int) (uint64, bool) {
	if n > 19 {
		return 0, false
	}
	p := uint64(1)
	for range n {
		p *= 10
	}
	return p, true
}
