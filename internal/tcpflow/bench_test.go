package tcpflow

import (
	"math/rand/v2"
	"net/netip"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/netpkt"
)

// benchCapture builds conns connections with pkts data segments of size bytes
// each, client and server alternating, with a little reordering and duplication.
func benchCapture(conns, pkts, size int) []Input {
	rnd := rand.New(rand.NewPCG(1, 2))
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i)
	}
	srv := netip.MustParseAddrPort("10.0.0.2:502")
	var inputs []Input
	type state struct {
		cli      netip.AddrPort
		cseq     uint32
		sseq     uint32
		sent     int
		reorder  bool
		previous netpkt.Segment
	}
	states := make([]*state, conns)
	for i := range states {
		st := &state{cli: netip.AddrPortFrom(netip.AddrFrom4([4]byte{10, 1, byte(i >> 8), byte(i)}), uint16(20000+i)), cseq: rnd.Uint32(), sseq: rnd.Uint32()}
		states[i] = st
		inputs = append(inputs,
			Input{Seg: netpkt.Segment{Src: st.cli, Dst: srv, Seq: st.cseq, Flags: netpkt.SYN}},
			Input{Seg: netpkt.Segment{Src: srv, Dst: st.cli, Seq: st.sseq, Ack: st.cseq + 1, Flags: netpkt.SYN | netpkt.ACK}},
		)
		st.cseq++
		st.sseq++
	}
	for round := 0; round < pkts; round++ {
		for _, st := range states {
			src, dst, seqp, ack := st.cli, srv, &st.cseq, st.sseq
			if round%2 == 1 {
				src, dst, seqp, ack = srv, st.cli, &st.sseq, st.cseq
			}
			seg := netpkt.Segment{Src: src, Dst: dst, Seq: *seqp, Ack: ack, Flags: netpkt.PSH | netpkt.ACK, Payload: payload, PayloadLen: size}
			*seqp += uint32(size)
			switch r := rnd.IntN(100); {
			case r < 2 && st.previous.PayloadLen > 0:
				inputs = append(inputs, Input{Seg: seg}, Input{Seg: st.previous}) // retransmission
			case r < 4:
				inputs = append(inputs, Input{Seg: seg})
				if n := len(inputs); n >= 2 {
					inputs[n-1], inputs[n-2] = inputs[n-2], inputs[n-1] // reordering
				}
			default:
				inputs = append(inputs, Input{Seg: seg})
			}
			st.previous = seg
		}
	}
	t := time.Unix(1700000000, 0)
	for i := range inputs {
		inputs[i].Number = i + 1
		inputs[i].Timestamp = t.Add(time.Duration(i) * time.Microsecond)
	}
	return inputs
}

func BenchmarkAssemble100MiB(b *testing.B) {
	inputs := benchCapture(1000, 250, 420) // 250k segments, ~100 MiB of payload
	var total int64
	for _, in := range inputs {
		total += int64(len(in.Seg.Payload))
	}
	b.SetBytes(total)
	b.ResetTimer()
	for range b.N {
		a := NewAssembler(Options{})
		for _, in := range inputs {
			a.Add(in)
		}
		if got := len(a.Finish()); got != 1000 {
			b.Fatalf("sessions = %d", got)
		}
	}
}
