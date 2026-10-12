package demo

import (
	"bytes"
	"cmp"
	"fmt"
	"hash/fnv"
	"slices"
	"time"

	"github.com/AVGsync/protolab/internal/pcap"
)

// sessionSeed derives the random choices of a session (initial sequence
// numbers, IP ids) from its capture and key, so that adding or reordering
// sessions does not change the others.
func sessionSeed(fileName, key string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(fileName))
	h.Write([]byte{0})
	h.Write([]byte(key))
	return h.Sum32()
}

// record is one frame of the capture file.
type record struct {
	at    time.Duration // from the start of the capture
	frame []byte
	// truncated: the scenario expects the frame not to fit into the snaplen.
	truncated bool
}

// records lists the captured frames of all sessions and the background
// frames in the order of their timestamps.
func (cs captureSpec) records() []record {
	var out []record
	for _, ss := range cs.sessions {
		c := timeline(ss, sessionSeed(cs.fileName, ss.key))
		for _, p := range c.packets {
			if p.captured {
				out = append(out, record{at: p.at, frame: c.frame(p), truncated: p.truncated})
			}
		}
	}
	for _, n := range cs.nonTCP {
		out = append(out, record{at: n.at, frame: n.frame})
	}
	slices.SortStableFunc(out, func(a, b record) int { return cmp.Compare(a.at, b.at) })
	return out
}

// render writes the capture file. The writer cuts frames longer than the
// snaplen; a frame that is cut although the scenario does not say so (or the
// other way round) means the scenario is wrong, which is a programming error.
func (cs captureSpec) render() []byte {
	var buf bytes.Buffer
	var write func(ts time.Time, data []byte, origLen int) error
	if cs.pcapng {
		w, err := pcap.NewNgWriter(&buf, pcap.LinkTypeEthernet, cs.snapLen)
		must(err)
		write = w.WritePacket
	} else {
		w, err := pcap.NewWriter(&buf, pcap.LinkTypeEthernet, cs.snapLen, false)
		must(err)
		write = w.WritePacket
	}
	for i, r := range cs.records() {
		if cut := len(r.frame) > int(cs.snapLen); cut != r.truncated {
			panic(fmt.Sprintf("demo: %s, frame %d: %d bytes, snaplen %d, truncation expected: %v",
				cs.fileName, i+1, len(r.frame), cs.snapLen, r.truncated))
		}
		must(write(cs.start.Add(r.at), r.frame, len(r.frame)))
	}
	return buf.Bytes()
}

func must(err error) {
	if err != nil {
		panic("demo: " + err.Error())
	}
}
