package netpkt

import (
	"encoding/binary"
	"net/netip"
)

// onesSum adds b as big-endian 16-bit words to sum (RFC 1071), padding an odd
// trailing byte with zero. It adds 32-bit words at a time: since 2^16 ≡ 1 modulo
// 0xffff, folding the wide sum later gives the same one's complement result.
func onesSum(b []byte, sum uint64) uint64 {
	for len(b) >= 8 {
		v := binary.BigEndian.Uint64(b)
		sum += v>>32 + v&0xffffffff
		b = b[8:]
	}
	for len(b) >= 2 {
		sum += uint64(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint64(b[0]) << 8
	}
	return sum
}

// fold reduces a wide sum to 16 bits with end-around carry.
func fold(sum uint64) uint16 {
	for sum > 0xffff {
		sum = sum>>16 + sum&0xffff
	}
	return uint16(sum)
}

// pseudoHeaderSum covers the IPv4 pseudo-header (RFC 9293, 3.1), or the IPv6 one
// (RFC 8200, 8.1) when an address is not IPv4.
func pseudoHeaderSum(src, dst netip.Addr, length int) uint64 {
	src, dst = src.Unmap(), dst.Unmap()
	if src.Is4() && dst.Is4() {
		s, d := src.As4(), dst.As4()
		sum := onesSum(s[:], 0)
		sum = onesSum(d[:], sum)
		return sum + protoTCP + uint64(uint16(length))
	}
	s, d := src.As16(), dst.As16()
	sum := onesSum(s[:], 0)
	sum = onesSum(d[:], sum)
	return sum + protoTCP + uint64(uint32(length))
}

// checksumValid verifies a complete segment. Summing it together with the
// transmitted checksum must give 0xffff; unlike comparing with a recomputed
// value, this also accepts 0xffff for 0x0000 (both are zero in one's complement).
func checksumValid(src, dst netip.Addr, segment []byte) bool {
	return fold(onesSum(segment, pseudoHeaderSum(src, dst, len(segment)))) == 0xffff
}

// badChecksum returns a checksum that never verifies. Inverting suffices except
// for 0x0000, whose inverse 0xffff is the other one's complement zero.
func badChecksum(sum uint16) uint16 {
	if sum == 0 {
		return 1
	}
	return ^sum
}
