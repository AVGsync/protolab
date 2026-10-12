package tcpflow

import (
	"net/netip"
	"slices"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

// Adversarial cases of the case requirements: each test names the property
// it protects.

func TestPortReuseSynAckWhileOldInstanceOpen(t *testing.T) {
	const ci2, si2 = 500000, 900000
	// The new connection's SYN was not captured and the old instance never
	// closed (the device rebooted): the SYN-ACK still starts a new instance.
	newConn := []pk{
		Sv(SA, si2, ci2+1, ""),
		C(A, ci2+1, si2+1, ""),
		C(PA, ci2+1, si2+1, "second"),
		Sv(PA, si2+1, ci2+7, "reply2"),
	}
	t.Run("after a handshake", func(t *testing.T) {
		old := seq(handshake(), []pk{C(PA, c1, s1, "first"), Sv(PA, s1, c1+5, "reply1")})
		ss := assemble(t, Options{}, seq(old, newConn))
		if len(ss) != 2 {
			t.Fatalf("got %d sessions, want 2", len(ss))
		}
		wantData(t, ss[0], model.C2S, "first")
		wantData(t, ss[0], model.S2C, "reply1")
		wantData(t, ss[1], model.C2S, "second")
		wantData(t, ss[1], model.S2C, "reply2")
		if ss[1].Roles != RolesInferred || ss[1].Status != statusPartial {
			t.Errorf("second: roles=%s status=%s", ss[1].Roles, ss[1].Status)
		}
	})
	t.Run("after a midstream pickup", func(t *testing.T) {
		old := []pk{C(PA, 7000, 3000, "first"), Sv(PA, 3000, 7005, "reply1")}
		ss := assemble(t, Options{TargetPorts: []int{502}}, seq(old, newConn))
		if len(ss) != 2 {
			t.Fatalf("got %d sessions, want 2", len(ss))
		}
		wantData(t, ss[0], model.C2S, "first")
		wantData(t, ss[1], model.C2S, "second")
	})
	t.Run("SYN-ACK retransmission stays in the instance", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{Sv(SA, si, c1, "")}, []pk{C(PA, c1, s1, "x")}, closing(1, 0)))
		wantNotes(t, s, 4, model.FlagRetransmission)
		wantData(t, s, model.C2S, "x")
		if s.Status != statusComplete {
			t.Errorf("status = %s", s.Status)
		}
	})
	t.Run("SYN-ACK whose SYN was missed, then a retransmitted SYN-ACK", func(t *testing.T) {
		s := one(t, Options{}, []pk{Sv(SA, si, c1, ""), Sv(SA, si, c1, ""), C(A, c1, s1, ""), C(PA, c1, s1, "x")})
		wantData(t, s, model.C2S, "x")
	})
}

func TestNewSynAfterCloseOrResetIsNewSession(t *testing.T) {
	for name, closer := range map[string][]pk{
		"FIN both ways": closing(5, 0),
		"RST":           {Sv(RA, s1, c1+5, "")},
		"RST from client": {
			C(R, c1+5, 0, ""),
		},
	} {
		t.Run(name, func(t *testing.T) {
			first := seq(handshake(), []pk{C(PA, c1, s1, "first")}, closer)
			// Same 4-tuple, same ISN again: still a new connection after the close.
			ss := assemble(t, Options{}, seq(first, handshake(), []pk{C(PA, c1, s1, "again")}))
			if len(ss) != 2 {
				t.Fatalf("got %d sessions", len(ss))
			}
			wantData(t, ss[0], model.C2S, "first")
			wantData(t, ss[1], model.C2S, "again")
		})
	}
}

func TestZeroWindowProbes(t *testing.T) {
	// The receiver advertises a zero window; the sender probes with the next
	// byte, repeatedly, then sends it again with the rest once the window opens.
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "abcd"),    // 4
		Sv(A, s1, c1+4, ""),      // 5: window 0 in reality
		C(A, c1+4, s1, "e"),      // 6: zero-window probe (new byte)
		Sv(A, s1, c1+4, ""),      // 7: not accepted
		C(A, c1+4, s1, "e"),      // 8: probe again
		Sv(A, s1, c1+4, ""),      // 9
		C(PA, c1+4, s1, "efgh"),  // 10: window open
		Sv(A, s1, c1+8, ""),      // 11
		C(PA, c1+8, s1, "ijkl"),  // 12
		Sv(A, s1, c1+12, ""),     // 13
		C(A, c1+11, s1, "\x00"),  // 14: keep-alive with a garbage byte
		Sv(A, s1, c1+12, ""),     // 15
		C(PA, c1+12, s1, "mnop"), // 16
	}, closing(16, 0)))
	wantData(t, s, model.C2S, "abcdefghijklmnop")
	wantGaps(t, s, model.C2S)
	if c := s.Stream(model.C2S).Conflicts; len(c) != 0 {
		t.Errorf("conflicts = %+v", c)
	}
	if packet(s, 6).StreamOffset != 4 {
		t.Errorf("first probe offset = %d, want 4", packet(s, 6).StreamOffset)
	}
	if s.Status != statusComplete {
		t.Errorf("status = %s", s.Status)
	}
}

func TestPartialOverlapOldAndNewBytes(t *testing.T) {
	// A retransmission repeats part of the delivered bytes (with one of them
	// different) and carries new bytes after them.
	for _, tc := range []struct {
		policy string
		want   string
		chosen int
	}{
		{PolicyFirst, "abcdefghij", 4},
		{PolicyLast, "abcdXfghij", 5},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			s := one(t, Options{OverlapPolicy: tc.policy}, seq(handshake(), []pk{
				C(PA, c1, s1, "abcdef"),     // 4
				C(PA, c1+2, s1, "cdXfghij"), // 5: old "cdef" with X instead of e, new "ghij"
			}, closing(10, 0)))
			wantData(t, s, model.C2S, tc.want)
			c := s.Stream(model.C2S).Conflicts
			if len(c) != 1 || c[0].Offset != 4 || c[0].Length != 1 || c[0].Chosen != tc.chosen || !slices.Equal(c[0].Packets, []int{4, 5}) {
				t.Fatalf("conflicts = %+v", c)
			}
			wantNotes(t, s, 5, model.FlagOverlapConflict)
			if p := packet(s, 5); p.StreamOffset != 2 {
				t.Errorf("offset of the partial retransmission = %d", p.StreamOffset)
			}
			d := diag(t, s, codeOverlapConflict)
			if !slices.Equal(d.PacketNumbers, []int{4, 5}) {
				t.Errorf("diagnostic packets = %v", d.PacketNumbers)
			}
		})
	}
}

func TestOutOfOrderAcrossGapLaterFilled(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "aaaa"),      // 4
		C(PA, c1+8, s1, "cccc"),    // 5: "bbbb" missing for now
		C(PA, c1+12, s1, "dddd"),   // 6
		Sv(A, s1, c1+4, ""),        // 7: duplicate ACK
		C(PA, c1+4, s1, "bbbb"),    // 8: fills the hole
		C(PA, c1+8, s1, "cccc"),    // 9: spurious retransmission
		Sv(A, s1, c1+16, ""),       // 10
		C(PA, c1+16, s1, "eeee"),   // 11
		C(PA, c1+20, s1, "ffff"),   // 12
		C(PA, c1+12, s1, "ddddee"), // 13: retransmission straddling old data
	}, closing(24, 0)))
	wantData(t, s, model.C2S, "aaaabbbbccccddddeeeeffff")
	wantGaps(t, s, model.C2S)
	wantNotes(t, s, 8, model.FlagOutOfOrder)
	wantNotes(t, s, 9, model.FlagRetransmission)
	wantNotes(t, s, 13, model.FlagRetransmission)
	if s.Status != statusComplete {
		t.Errorf("status = %s", s.Status)
	}
	var offs []int
	for _, sg := range s.Stream(model.C2S).Segments {
		offs = append(offs, sg.Packet)
	}
	if !slices.Equal(offs, []int{4, 8, 5, 6, 11, 12}) {
		t.Errorf("segment packets in stream order = %v", offs)
	}
}

func TestStrayPacketsDoNotInflateTheStream(t *testing.T) {
	// A corrupted segment (bad checksum, garbage sequence and acknowledgment
	// numbers) must not turn the stream into tens of megabytes of "lost" bytes.
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "abcd"),
		Sv(A, s1, c1+0x40000000, "").bad(), // corrupted acknowledgment number
		Sv(PA, s1, c1+4, "ok"),
	}, closing(4, 2)))
	if n := len(s.Stream(model.C2S).Data); n != 4 {
		t.Errorf("c2s length = %d, want 4", n)
	}
	if s.Status == statusError {
		t.Errorf("status = %s, diagnostics %v", s.Status, codes(s))
	}
}

func TestChecksumOffloadVersusCorruption(t *testing.T) {
	t.Run("the local host's segments fail: offload", func(t *testing.T) {
		pkts := seq(handshake(), []pk{
			C(PA, c1, s1, "abc"),
			Sv(A, s1, c1+3, ""),
			C(PA, c1+3, s1, "def"),
			C(PA, c1+3, s1, "dXf"), // different bytes: resolved by the overlap policy only
		}, closing(6, 0), []pk{C(RA, c1+7, s1+1, "")})
		for i := range pkts[:len(pkts)-1] { // the kernel computes the checksum of a RST itself
			if !pkts[i].fromServer {
				pkts[i] = pkts[i].bad()
			}
		}
		for _, policy := range []string{PolicyFirst, PolicyLast} {
			s := one(t, Options{OverlapPolicy: policy}, pkts)
			d := diag(t, s, codeChecksumOffload)
			if d.Level != model.LevelInfo || d.Count != 7 {
				t.Errorf("diagnostic = %+v", d)
			}
			if slices.Contains(codes(s), codeChecksumBad) {
				t.Errorf("offload reported as damage: %v", codes(s))
			}
			want := map[string]string{PolicyFirst: "abcdef", PolicyLast: "abcdXf"}[policy]
			wantData(t, s, model.C2S, want)
		}
		s := one(t, Options{ChecksumPolicy: ChecksumIgnore}, pkts)
		if slices.Contains(codes(s), codeChecksumOffload) || len(packet(s, 4).Annotations) != 0 {
			t.Errorf("ignored checksums still reported: %v %v", codes(s), packet(s, 4).Annotations)
		}
	})
	t.Run("one bad segment among good ones: corruption", func(t *testing.T) {
		pkts := seq(handshake(), []pk{
			C(PA, c1, s1, "abc"),
			C(PA, c1+3, s1, "dXf").bad(), // 5: damaged on the way
			C(PA, c1+3, s1, "def"),       // 6: the receiver dropped #5, the sender retransmits
			C(PA, c1+6, s1, "ghi"),
		}, closing(9, 0))
		s := one(t, Options{}, pkts)
		d := diag(t, s, codeChecksumBad)
		if d.Level != model.LevelWarning || !slices.Equal(d.PacketNumbers, []int{5}) || d.Count != 1 {
			t.Errorf("diagnostic = %+v", d)
		}
		if slices.Contains(codes(s), codeChecksumOffload) {
			t.Errorf("corruption reported as offload: %v", codes(s))
		}
		wantNotes(t, s, 5, model.FlagChecksumWarning)
		// The receiver dropped the damaged copy, so the application got the
		// retransmitted bytes, whatever the overlap policy.
		for _, policy := range []string{PolicyFirst, PolicyLast} {
			s := one(t, Options{OverlapPolicy: policy}, pkts)
			wantData(t, s, model.C2S, "abcdefghi")
			c := s.Stream(model.C2S).Conflicts
			if len(c) != 1 || c[0].Chosen != 6 || !slices.Equal(c[0].Packets, []int{5, 6}) {
				t.Errorf("%s: conflicts = %+v", policy, c)
			}
		}
	})
	t.Run("damaged copy first, intact copy never arrives", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "abc"),
			C(PA, c1+3, s1, "dXf").bad(),
			C(PA, c1+6, s1, "ghi"),
			Sv(A, s1, c1+9, ""),
		}, closing(9, 0)))
		wantData(t, s, model.C2S, "abcdXfghi")
		if s.Status != statusPartial {
			t.Errorf("status = %s: bytes of a damaged packet are not verified", s.Status)
		}
	})
	t.Run("intact copy confirms the bytes of a damaged one", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "abc"),
			C(PA, c1+3, s1, "def").bad(), // the damage was in the header or the checksum itself
			C(PA, c1+3, s1, "def"),
			C(PA, c1+6, s1, "ghi"),
		}, closing(9, 0)))
		wantData(t, s, model.C2S, "abcdefghi")
		if s.Status != statusComplete {
			t.Errorf("status = %s", s.Status)
		}
	})
	t.Run("damaged sequence number is not trusted", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "abc"),
			C(PA, c1+0x10000003, s1, "def").bad(), // a flipped bit in the sequence number
			C(PA, c1+3, s1, "def"),
		}, closing(6, 0)))
		wantData(t, s, model.C2S, "abcdef")
		wantGaps(t, s, model.C2S)
	})
}

func TestMissingStartIsNotDeclaredKnown(t *testing.T) {
	// Only the server's data: no SYN, no SYN-ACK, no target ports.
	other := netip.MustParseAddrPort("10.0.0.9:7777")
	s := one(t, Options{}, []pk{
		Sv(PA, 3000, 7000, "data").ports(server, other),
		C(A, 7000, 3004, "").ports(other, server),
	})
	if s.Roles != RolesUnknown || s.Handshake {
		t.Errorf("roles=%s handshake=%v", s.Roles, s.Handshake)
	}
	for _, st := range s.Streams {
		if st.StartKnown {
			t.Errorf("%s: start reported as known", st.Dir)
		}
	}
	if s.Status == statusComplete {
		t.Error("session without a handshake reported complete")
	}
	_ = diag(t, s, codeMissingHandshake)

	t.Run("SYN captured, SYN-ACK not", func(t *testing.T) {
		s := one(t, Options{}, []pk{C(S, ci, 0, ""), C(A, c1, s1, ""), Sv(PA, s1, c1, "hi"), C(PA, c1, s1+2, "yo")})
		if s.Roles != RolesHandshake || !s.Stream(model.C2S).StartKnown {
			t.Errorf("roles=%s c2s known=%v", s.Roles, s.Stream(model.C2S).StartKnown)
		}
		if s.Stream(model.S2C).StartKnown {
			t.Error("server stream start declared known without the SYN-ACK")
		}
	})
}

func TestOneFailingSessionDoesNotBreakTheOthers(t *testing.T) {
	other := netip.MustParseAddrPort("10.0.0.1:40001")
	orig := build
	t.Cleanup(func() { build = orig })
	build = func(c *conn, opt *options) *Session {
		if c.key.lo == other || c.key.hi == other {
			var m map[string]int
			m["boom"]++ // a defect in the reassembly of this connection only
		}
		return orig(c, opt)
	}
	ss := assemble(t, Options{}, []pk{
		C(S, ci, 0, ""),
		C(S, 77, 0, "").ports(other, server),
		Sv(SA, si, c1, ""),
		C(A, c1, s1, ""),
		C(PA, c1, s1, "fine"),
		C(PA, 78, 0, "lost").ports(other, server),
	})
	if len(ss) != 2 {
		t.Fatalf("got %d sessions", len(ss))
	}
	wantData(t, ss[0], model.C2S, "fine")
	bad := ss[1]
	if bad.Status != statusError || bad.Index != 1 || bad.Client != other || len(bad.Packets) != 2 {
		t.Fatalf("failed session = %+v", bad)
	}
	if d := diag(t, bad, codeReassemblyFailed); d.Level != model.LevelError {
		t.Errorf("diagnostic = %+v", d)
	}
	if bad.Packets[1].Number != 6 || bad.Packets[1].Dir != model.C2S || bad.Stream(model.C2S).Data == nil {
		t.Errorf("packets = %+v", bad.Packets)
	}
}

func TestLateSegmentOfReplacedInstance(t *testing.T) {
	first := seq(handshake(), []pk{C(PA, c1, s1, "first")}, closing(5, 0))
	for name, isn := range map[string][2]uint32{
		"random ISNs":   {0x80000000, 0x90000000},
		"RFC 6528 ISNs": {ci + 2_500_000, si + 2_500_000}, // the same 4-tuple 10 s later
	} {
		t.Run(name, func(t *testing.T) {
			ci2, si2 := isn[0], isn[1]
			second := []pk{
				C(S, ci2, 0, ""),              // 8
				Sv(SA, si2, ci2+1, ""),        // 9
				Sv(FA, s1, c1+6, ""),          // 10: late retransmitted FIN of the first instance
				C(A, ci2+1, si2+1, ""),        // 11
				C(PA, ci2+1, si2+1, "second"), // 12
				Sv(PA, si2+1, ci2+7, "reply"), // 13
				C(A, c1+6, s1+1, ""),          // 14: and the first instance's last ACK, late too
			}
			ss := assemble(t, Options{}, seq(first, second))
			if len(ss) != 2 {
				t.Fatalf("got %d sessions", len(ss))
			}
			s := ss[1]
			if n := len(s.Stream(model.S2C).Data); n != 5 {
				t.Errorf("s2c length of the new instance = %d, want 5 (diagnostics %v)", n, codes(s))
			}
			wantData(t, s, model.C2S, "second")
			if s.Status == statusError {
				t.Errorf("status = %s", s.Status)
			}
			if name == "random ISNs" && (len(ss[0].Packets) != 9 || ss[0].Packets[7].Number != 10) {
				t.Errorf("late segments not returned to the first instance: %d packets", len(ss[0].Packets))
			}
		})
	}
}

func TestTotalStreamBudget(t *testing.T) {
	// Every session announces far more bytes than it carries: a crafted or
	// badly damaged capture must not make the reassembly allocate them all.
	var pkts []pk
	for i := range 4 {
		cp := netip.AddrPortFrom(client.Addr(), uint16(41000+i))
		pkts = append(pkts,
			C(S, ci, 0, "").ports(cp, server),
			Sv(SA, si, c1, "").ports(server, cp),
			C(PA, c1, s1, "head").ports(cp, server),
			C(PA, c1+40, s1, "tail").ports(cp, server), // 36 bytes lost in between
		)
	}
	ss := assemble(t, Options{MaxTotalBytes: 100}, pkts)
	var lengths []int
	for _, s := range ss {
		lengths = append(lengths, len(s.Stream(model.C2S).Data))
	}
	if !slices.Equal(lengths, []int{44, 44, 12, 0}) {
		t.Fatalf("lengths = %v", lengths)
	}
	for i, s := range ss {
		cut := slices.Contains(codes(s), codeStreamTooLarge)
		if cut != (i >= 2) || (cut && s.Status != statusError) {
			t.Errorf("session %d: status %s diagnostics %v", i, s.Status, codes(s))
		}
	}
	if d := diag(t, ss[2], codeStreamTooLarge); d.Message != "C→S: потоки записи вместе превышают предел 100 Б, байты этого потока после смещения 12 не сохранены" {
		t.Errorf("message = %q", d.Message)
	}
}
