package tcpflow

// seqUnwrapper maps 32-bit sequence numbers to 64-bit offsets from origin.
// A number is read as the offset closest to the highest one accepted so far
// (RFC 9293 sequence arithmetic around a moving point), which survives the
// wraparound at 2^32 and streams longer than 2 GiB. Only data and FIN segments
// move the point; acknowledgments and pure ACKs are read relative to it.
type seqUnwrapper struct {
	origin uint32
	high   int64
}

// at returns the offset of seq.
func (u *seqUnwrapper) at(seq uint32) int64 {
	return u.high + int64(int32(seq-u.origin-uint32(u.high)))
}

// advance returns the offset of seq and moves the reference point up to it.
func (u *seqUnwrapper) advance(seq uint32) int64 {
	off := u.at(seq)
	u.high = max(u.high, off)
	return off
}
