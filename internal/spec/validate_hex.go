package spec

import (
	"encoding/hex"
	"strings"
	"unicode"
)

// ParseHex decodes a byte pattern written in any of the notations people paste
// from hex editors and Wireshark: "A5 01", "a501", "0xA5, 0x01", "A5:01".
// It reports false for an empty pattern, an odd number of digits or any other
// character.
func ParseHex(s string) ([]byte, bool) {
	s = strings.ReplaceAll(strings.ToLower(s), "0x", "")
	digits := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case isHexSeparator(r):
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
			digits = append(digits, byte(r))
		default:
			return nil, false
		}
	}
	if len(digits) == 0 || len(digits)%2 != 0 {
		return nil, false
	}
	out := make([]byte, len(digits)/2)
	if _, err := hex.Decode(out, digits); err != nil {
		return nil, false
	}
	return out, true
}

func isHexSeparator(r rune) bool {
	switch r {
	case ',', ':', ';', '-':
		return true
	}
	return unicode.IsSpace(r)
}
