package tcpflow

import (
	"net/netip"
	"slices"
	"strings"
	"testing"

	"github.com/AVGsync/protolab/internal/model"
)

func TestInOrderSession(t *testing.T) {
	pkts := seq(handshake(), []pk{
		C(PA, c1, s1, "hello"),     // 4
		Sv(A, s1, c1+5, ""),        // 5
		Sv(PA, s1, c1+5, "world!"), // 6
		C(A, c1+5, s1+6, ""),       // 7
	}, closing(5, 6)) // 8, 9, 10
	s := one(t, Options{}, pkts)

	if s.Index != 0 || s.Client != client || s.Server != server || s.Roles != RolesHandshake {
		t.Errorf("index=%d client=%v server=%v roles=%s", s.Index, s.Client, s.Server, s.Roles)
	}
	if !s.Handshake || !s.Closed || s.Status != statusComplete || len(s.Diagnostics) != 0 {
		t.Errorf("handshake=%v closed=%v status=%s diagnostics=%v", s.Handshake, s.Closed, s.Status, s.Diagnostics)
	}
	if !s.Start.Equal(t0) || !s.End.Equal(t0.Add(9*dt)) {
		t.Errorf("start=%v end=%v", s.Start, s.End)
	}
	wantData(t, s, model.C2S, "hello")
	wantData(t, s, model.S2C, "world!")
	for _, st := range s.Streams {
		if !st.StartKnown || len(st.Gaps) != 0 || len(st.Conflicts) != 0 {
			t.Errorf("%s: startKnown=%v gaps=%v conflicts=%v", st.Dir, st.StartKnown, st.Gaps, st.Conflicts)
		}
	}
	if seg := s.Stream(model.C2S).Segments; len(seg) != 1 || seg[0].Offset != 0 || seg[0].Length != 5 || seg[0].Packet != 4 {
		t.Errorf("c2s segments = %+v", seg)
	}

	// Relative numbers, Wireshark style.
	rel := map[int][2]uint32{1: {0, 0}, 2: {0, 1}, 3: {1, 1}, 4: {1, 1}, 6: {1, 6}, 8: {6, 7}, 9: {7, 7}, 10: {7, 8}}
	for n, want := range rel {
		if p := packet(s, n); p.Seq != want[0] || p.Ack != want[1] {
			t.Errorf("packet #%d: seq/ack = %d/%d, want %d/%d", n, p.Seq, p.Ack, want[0], want[1])
		}
	}
	for _, p := range s.Packets {
		wantOff := -1
		if p.Number == 4 || p.Number == 6 {
			wantOff = 0
		}
		if p.StreamOffset != wantOff || len(p.Annotations) != 0 {
			t.Errorf("packet #%d: offset=%d annotations=%v", p.Number, p.StreamOffset, p.Annotations)
		}
		if wantDir := model.C2S; (p.Number == 2 || p.Number == 5 || p.Number == 6 || p.Number == 9) && p.Dir == wantDir {
			t.Errorf("packet #%d direction %s", p.Number, p.Dir)
		}
	}
}

func TestSegmentationVariants(t *testing.T) {
	cases := []struct {
		name     string
		data     []pk
		total    uint32
		want     string
		segments []Segment
	}{
		{
			name:  "message split across segments",
			data:  []pk{C(PA, c1, s1, "HEAD"), C(PA, c1+4, s1, "ER-BODY")},
			total: 11, want: "HEADER-BODY",
			segments: []Segment{{Offset: 0, Length: 4, Packet: 4}, {Offset: 4, Length: 7, Packet: 5}},
		},
		{
			name:  "two messages coalesced in one segment",
			data:  []pk{C(PA, c1, s1, "MSG1MSG2")},
			total: 8, want: "MSG1MSG2",
			segments: []Segment{{Offset: 0, Length: 8, Packet: 4}},
		},
		{
			name:  "repacketized retransmission adds the tail",
			data:  []pk{C(PA, c1, s1, "abcd"), C(PA, c1, s1, "abcdefgh")},
			total: 8, want: "abcdefgh",
			segments: []Segment{{Offset: 0, Length: 4, Packet: 4}, {Offset: 0, Length: 8, Packet: 5}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := one(t, Options{}, seq(handshake(), tc.data, closing(tc.total, 0)))
			wantData(t, s, model.C2S, tc.want)
			got := s.Stream(model.C2S).Segments
			if len(got) != len(tc.segments) {
				t.Fatalf("segments = %+v", got)
			}
			for i, g := range got {
				w := tc.segments[i]
				if g.Offset != w.Offset || g.Length != w.Length || g.Packet != w.Packet {
					t.Errorf("segment %d = %+v, want %+v", i, g, w)
				}
			}
			if s.Status != statusComplete {
				t.Errorf("status = %s", s.Status)
			}
		})
	}
}

func TestIdenticalRetransmission(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "abcdef"), // 4
		C(PA, c1, s1, "abcdef"), // 5
		C(PA, c1+2, s1, "cd"),   // 6
	}, closing(6, 0)))
	wantData(t, s, model.C2S, "abcdef")
	wantNotes(t, s, 5, model.FlagRetransmission)
	wantNotes(t, s, 6, model.FlagRetransmission)
	if packet(s, 5).StreamOffset != -1 || len(s.Stream(model.C2S).Segments) != 1 {
		t.Errorf("duplicate contributed: offset=%d segments=%v", packet(s, 5).StreamOffset, s.Stream(model.C2S).Segments)
	}
	if s.Status != statusComplete || len(s.Stream(model.C2S).Conflicts) != 0 {
		t.Errorf("status=%s conflicts=%v", s.Status, s.Stream(model.C2S).Conflicts)
	}
}

func TestConflictingOverlap(t *testing.T) {
	cases := []struct {
		policy    string
		pkts      []pk
		want      string
		conflicts []Conflict
		notes     map[int][]string
		offsets   map[int]int
		message   string
	}{
		{
			policy:    PolicyFirst,
			pkts:      []pk{C(PA, c1, s1, "abcdef"), C(PA, c1, s1, "abXYef")},
			want:      "abcdef",
			conflicts: []Conflict{{Offset: 2, Length: 2, Packets: []int{4, 5}, Chosen: 4}},
			notes:     map[int][]string{5: {model.FlagRetransmission, model.FlagOverlapConflict}},
			offsets:   map[int]int{4: 0, 5: -1},
			message:   "C→S: повторная передача с другими байтами (2 Б), использован пакет #4",
		},
		{
			policy:    PolicyLast,
			pkts:      []pk{C(PA, c1, s1, "abcdef"), C(PA, c1, s1, "abXYef")},
			want:      "abXYef",
			conflicts: []Conflict{{Offset: 2, Length: 2, Packets: []int{4, 5}, Chosen: 5}},
			notes:     map[int][]string{5: {model.FlagRetransmission, model.FlagOverlapConflict}},
			offsets:   map[int]int{4: 0, 5: 0},
			message:   "C→S: повторная передача с другими байтами (2 Б), использован пакет #5",
		},
		{
			// Separate differing runs give separate conflicts but one diagnostic.
			policy: PolicyFirst,
			pkts:   []pk{C(PA, c1, s1, "abcdefgh"), C(PA, c1, s1, "Xbcdefg!")},
			want:   "abcdefgh",
			conflicts: []Conflict{
				{Offset: 0, Length: 1, Packets: []int{4, 5}, Chosen: 4},
				{Offset: 7, Length: 1, Packets: []int{4, 5}, Chosen: 4},
			},
			notes:   map[int][]string{5: {model.FlagRetransmission, model.FlagOverlapConflict}},
			message: "C→S: повторная передача с другими байтами (2 Б), использован пакет #4",
		},
		{
			// A partial overlap that also brings new bytes is not a retransmission.
			policy:    PolicyFirst,
			pkts:      []pk{C(PA, c1, s1, "abcdef"), C(PA, c1+4, s1, "XYgh")},
			want:      "abcdefgh",
			conflicts: []Conflict{{Offset: 4, Length: 2, Packets: []int{4, 5}, Chosen: 4}},
			notes:     map[int][]string{5: {model.FlagOverlapConflict}},
			offsets:   map[int]int{5: 4},
			message:   "C→S: повторная передача с другими байтами (2 Б), использован пакет #4",
		},
		{
			policy:    PolicyLast,
			pkts:      []pk{C(PA, c1, s1, "aaaa"), C(PA, c1, s1, "bbbb"), C(PA, c1, s1, "cccc")},
			want:      "cccc",
			conflicts: []Conflict{{Offset: 0, Length: 4, Packets: []int{4, 5, 6}, Chosen: 6}},
			message:   "C→S: повторная передача с другими байтами (4 Б), использован пакет #6",
		},
		{
			policy:    PolicyFirst,
			pkts:      []pk{C(PA, c1, s1, "aaaa"), C(PA, c1, s1, "bbbb"), C(PA, c1, s1, "cccc")},
			want:      "aaaa",
			conflicts: []Conflict{{Offset: 0, Length: 4, Packets: []int{4, 5, 6}, Chosen: 4}},
			message:   "C→S: повторная передача с другими байтами (4 Б), использован пакет #4",
		},
	}
	for _, tc := range cases {
		t.Run(tc.policy+"/"+tc.want, func(t *testing.T) {
			total := uint32(len(tc.want))
			s := one(t, Options{OverlapPolicy: tc.policy}, seq(handshake(), tc.pkts, closing(total, 0)))
			wantData(t, s, model.C2S, tc.want)
			got := s.Stream(model.C2S).Conflicts
			if len(got) != len(tc.conflicts) {
				t.Fatalf("conflicts = %+v, want %+v", got, tc.conflicts)
			}
			for i := range got {
				g, w := got[i], tc.conflicts[i]
				if g.Offset != w.Offset || g.Length != w.Length || g.Chosen != w.Chosen || !slices.Equal(g.Packets, w.Packets) {
					t.Errorf("conflict %d = %+v, want %+v", i, g, w)
				}
			}
			for n, notes := range tc.notes {
				wantNotes(t, s, n, notes...)
			}
			for n, off := range tc.offsets {
				if got := packet(s, n).StreamOffset; got != off {
					t.Errorf("packet #%d offset = %d, want %d", n, got, off)
				}
			}
			var msgs []string
			for _, d := range s.Diagnostics {
				if d.Code == codeOverlapConflict {
					msgs = append(msgs, d.Message)
					if d.Level != model.LevelError || !slices.Equal(d.PacketNumbers, tc.conflicts[0].Packets) {
						t.Errorf("diagnostic = %+v", d)
					}
				}
			}
			if len(msgs) != 1 || msgs[0] != tc.message {
				t.Errorf("conflict diagnostics = %q, want [%q]", msgs, tc.message)
			}
			if s.Status != statusPartial {
				t.Errorf("status = %s, want partial", s.Status)
			}
		})
	}
}

func TestOutOfOrder(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1+5, s1, "world"), // 4
		C(PA, c1, s1, "hello"),   // 5
	}, closing(10, 0)))
	wantData(t, s, model.C2S, "helloworld")
	wantGaps(t, s, model.C2S)
	wantNotes(t, s, 4)
	wantNotes(t, s, 5, model.FlagOutOfOrder)
	segs := s.Stream(model.C2S).Segments
	if len(segs) != 2 || segs[0].Packet != 5 || segs[1].Packet != 4 || segs[1].Offset != 5 {
		t.Errorf("segments not sorted by offset: %+v", segs)
	}
	if s.Status != statusComplete {
		t.Errorf("status = %s", s.Status)
	}
}

func TestLostSegment(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "aaaa"),   // 4
		C(PA, c1+8, s1, "cccc"), // 5: "bbbb" was lost
		Sv(A, s1, c1+12, ""),    // 6
	}, closing(12, 0)))
	wantData(t, s, model.C2S, "aaaa\x00\x00\x00\x00cccc")
	wantGaps(t, s, model.C2S, Gap{Offset: 4, Length: 4, Reason: model.GapLost})
	wantNotes(t, s, 5)
	d := diag(t, s, codeGap)
	if d.Message != "C→S: 4 Б неизвестны на смещении 0x4 (потерян сегмент)" || d.Level != model.LevelWarning ||
		!slices.Equal(d.PacketNumbers, []int{4, 5}) {
		t.Errorf("gap diagnostic = %+v", d)
	}
	if s.Status != statusPartial {
		t.Errorf("status = %s", s.Status)
	}
}

func TestTruncatedPayload(t *testing.T) {
	t.Run("tail filled later by a full retransmission", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "0123456789").cut(4), // 4
			C(PA, c1, s1, "0123456789"),        // 5
		}, closing(10, 0)))
		wantData(t, s, model.C2S, "0123456789")
		wantGaps(t, s, model.C2S)
		wantNotes(t, s, 4, model.FlagTruncated)
		wantNotes(t, s, 5)
		if packet(s, 4).StreamOffset != 0 || packet(s, 5).StreamOffset != 0 || s.Status != statusComplete {
			t.Errorf("offsets %d,%d status %s", packet(s, 4).StreamOffset, packet(s, 5).StreamOffset, s.Status)
		}
		if segs := s.Stream(model.C2S).Segments; len(segs) != 2 || segs[0].Length != 10 || segs[1].Length != 10 {
			t.Errorf("segments = %+v", segs)
		}
	})
	t.Run("tail never captured", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "0123456789").cut(4), // 4
			C(PA, c1+10, s1, "abcdef"),         // 5
		}, closing(16, 0)))
		wantData(t, s, model.C2S, "0123\x00\x00\x00\x00\x00\x00abcdef")
		wantGaps(t, s, model.C2S, Gap{Offset: 4, Length: 6, Reason: model.GapTruncated})
		d := diag(t, s, codeTruncated)
		if d.Message != "C→S: 6 Б неизвестны на смещении 0x4 (пакет усечён snaplen)" || !slices.Equal(d.PacketNumbers, []int{4}) {
			t.Errorf("diagnostic = %+v", d)
		}
	})
	t.Run("headers-only capture", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "aaaa").cut(0),   // 4
			C(PA, c1+4, s1, "bbbb").cut(0), // 5
			Sv(PA, s1, c1+8, "resp").cut(2),
		}, closing(8, 4)))
		wantGaps(t, s, model.C2S, Gap{Offset: 0, Length: 8, Reason: model.GapTruncated})
		wantGaps(t, s, model.S2C, Gap{Offset: 2, Length: 2, Reason: model.GapTruncated})
		if packet(s, 4).StreamOffset != 0 || packet(s, 5).StreamOffset != 4 || len(s.Stream(model.C2S).Segments) != 2 {
			t.Errorf("truncated packets do not map to the stream: %+v", s.Stream(model.C2S).Segments)
		}
		d := diag(t, s, codeTruncated)
		if d.Message != "C→S: 8 Б неизвестны на смещении 0x0 (пакет усечён snaplen)" || !slices.Equal(d.PacketNumbers, []int{4, 5}) {
			t.Errorf("diagnostic = %+v", d)
		}
	})
	t.Run("truncated duplicate of known bytes adds nothing", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "abcdef"),        // 4
			C(PA, c1, s1, "abcdef").cut(2), // 5
		}, closing(6, 0)))
		wantGaps(t, s, model.C2S)
		wantNotes(t, s, 5, model.FlagRetransmission, model.FlagTruncated)
	})
}

func TestMidstreamPickup(t *testing.T) {
	mid := []pk{
		C(PA, 7000, 3000, "req1"),  // 1
		Sv(PA, 3000, 7004, "resp"), // 2
		C(PA, 7004, 3004, "req2"),  // 3
	}
	t.Run("roles from the target port", func(t *testing.T) {
		s := one(t, Options{TargetPorts: []int{502}}, mid)
		if s.Roles != RolesInferred || s.Client != client || s.Handshake {
			t.Fatalf("roles=%s client=%v handshake=%v", s.Roles, s.Client, s.Handshake)
		}
		wantData(t, s, model.C2S, "req1req2")
		wantData(t, s, model.S2C, "resp")
		if s.Stream(model.C2S).StartKnown || s.Stream(model.S2C).StartKnown {
			t.Error("stream start reported as known")
		}
		if p := packet(s, 1); p.Seq != 1 || p.Ack != 1 {
			t.Errorf("relative seq/ack = %d/%d, want 1/1", p.Seq, p.Ack)
		}
		d := diag(t, s, codeMissingHandshake)
		if d.Message != "Начало соединения не захвачено: роли определены по порту, первые байты потока неизвестны" || d.Level != model.LevelWarning {
			t.Errorf("diagnostic = %+v", d)
		}
		if s.Status != statusPartial {
			t.Errorf("status = %s", s.Status)
		}
	})
	t.Run("target port wins over the first sender", func(t *testing.T) {
		s := one(t, Options{TargetPorts: []int{502}}, mid[1:])
		if s.Client != client || s.Roles != RolesInferred {
			t.Errorf("client=%v roles=%s", s.Client, s.Roles)
		}
	})
	t.Run("no clue: the first sender is the client", func(t *testing.T) {
		s := one(t, Options{}, mid[1:])
		if s.Client != server || s.Server != client || s.Roles != RolesUnknown {
			t.Errorf("client=%v roles=%s", s.Client, s.Roles)
		}
		wantData(t, s, model.C2S, "resp")
		if d := diag(t, s, codeMissingHandshake); d.Message != "Начало соединения не захвачено: роли сторон не установлены, первые байты потока неизвестны" {
			t.Errorf("diagnostic = %+v", d)
		}
	})
	t.Run("both ports are targets", func(t *testing.T) {
		if s := one(t, Options{TargetPorts: []int{502, 40000}}, mid); s.Roles != RolesUnknown {
			t.Errorf("roles = %s", s.Roles)
		}
	})
}

func TestSynAckOnly(t *testing.T) {
	s := one(t, Options{}, []pk{
		Sv(SA, si, c1, ""),          // 1: the client's SYN was not captured
		C(A, c1, s1, ""),            // 2
		C(PA, c1+5, s1, "world"),    // 3: "hello" was lost
		Sv(A, s1, c1+10, ""),        // 4
		Sv(PA, s1, c1+10, "answer"), // 5
	})
	if s.Roles != RolesInferred || s.Client != client || s.Handshake {
		t.Fatalf("roles=%s client=%v handshake=%v", s.Roles, s.Client, s.Handshake)
	}
	// The SYN-ACK acknowledges ISN+1, so both stream starts are known.
	if !s.Stream(model.C2S).StartKnown || !s.Stream(model.S2C).StartKnown {
		t.Error("stream start not derived from the SYN-ACK")
	}
	wantGaps(t, s, model.C2S, Gap{Offset: 0, Length: 5, Reason: model.GapLost})
	wantData(t, s, model.S2C, "answer")
	if d := diag(t, s, codeMissingHandshake); d.Message != "Начало соединения не захвачено: SYN клиента отсутствует, роли определены по SYN-ACK" {
		t.Errorf("diagnostic = %+v", d)
	}
}

func TestLateSynJoinsTheInstance(t *testing.T) {
	ss := assemble(t, Options{}, []pk{Sv(SA, si, c1, ""), C(S, ci, 0, ""), C(A, c1, s1, ""), C(PA, c1, s1, "x")})
	if len(ss) != 1 || ss[0].Roles != RolesHandshake {
		t.Fatalf("sessions=%d roles=%s", len(ss), ss[0].Roles)
	}
}

func TestPortReuse(t *testing.T) {
	const ci2, si2 = 500000, 900000
	second := []pk{
		C(S, ci2, 0, ""),
		Sv(SA, si2, ci2+1, ""),
		C(A, ci2+1, si2+1, ""),
		C(PA, ci2+1, si2+1, "second"),
		C(FA, ci2+7, si2+1, ""),
		Sv(FA, si2+1, ci2+8, ""),
		C(A, ci2+8, si2+2, ""),
	}
	t.Run("after a clean close", func(t *testing.T) {
		first := seq(handshake(), []pk{C(PA, c1, s1, "first")}, closing(5, 0))
		ss := assemble(t, Options{}, seq(first, second))
		if len(ss) != 2 {
			t.Fatalf("got %d sessions", len(ss))
		}
		wantData(t, ss[0], model.C2S, "first")
		wantData(t, ss[1], model.C2S, "second")
		for i, s := range ss {
			if s.Index != i || s.Status != statusComplete || s.Roles != RolesHandshake {
				t.Errorf("session %d: index=%d status=%s roles=%s", i, s.Index, s.Status, s.Roles)
			}
		}
		if len(ss[0].Packets) != 7 || ss[1].Packets[0].Number != 8 {
			t.Errorf("packets split %d/%d", len(ss[0].Packets), len(ss[1].Packets))
		}
	})
	t.Run("while the old instance looks open", func(t *testing.T) {
		first := seq(handshake(), []pk{C(PA, c1, s1, "first")}) // FIN not captured
		ss := assemble(t, Options{}, seq(first, second))
		if len(ss) != 2 || ss[0].Closed || ss[0].Status != statusPartial || ss[1].Status != statusComplete {
			t.Fatalf("sessions = %d", len(ss))
		}
	})
	t.Run("new connection whose SYN was missed", func(t *testing.T) {
		first := seq(handshake(), []pk{C(PA, c1, s1, "first")}, closing(5, 0))
		ss := assemble(t, Options{}, seq(first, second[1:]))
		if len(ss) != 2 || ss[1].Roles != RolesInferred {
			t.Fatalf("sessions = %d", len(ss))
		}
		wantData(t, ss[1], model.C2S, "second")
	})
	t.Run("retried SYN after RST is a new attempt", func(t *testing.T) {
		ss := assemble(t, Options{}, []pk{C(S, ci, 0, ""), Sv(RA, 0, c1, ""), C(S, ci, 0, "")})
		if len(ss) != 2 || !ss[0].Closed {
			t.Fatalf("sessions = %d", len(ss))
		}
	})
}

func TestSynRetransmission(t *testing.T) {
	s := one(t, Options{}, seq([]pk{C(S, ci, 0, "")}, handshake(), []pk{C(PA, c1, s1, "ok")}, closing(2, 0)))
	wantNotes(t, s, 1)
	wantNotes(t, s, 2, model.FlagRetransmission)
	if !s.Handshake || s.Status != statusComplete {
		t.Errorf("handshake=%v status=%s", s.Handshake, s.Status)
	}
}

func TestReset(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "abc"),
		Sv(RA, s1, c1+3, "diagnostic text in RST"), // 5
	}))
	if !s.Closed || s.Status != statusComplete {
		t.Errorf("closed=%v status=%s", s.Closed, s.Status)
	}
	wantData(t, s, model.S2C, "")
	if packet(s, 5).StreamOffset != -1 {
		t.Error("RST payload placed in the stream")
	}
}

func TestKeepAlive(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), []pk{
		C(PA, c1, s1, "abc"),   // 4
		Sv(A, s1, c1+3, ""),    // 5
		C(A, c1+2, s1, "\x00"), // 6: probe with a garbage byte
		Sv(A, s1, c1+3, ""),    // 7
		C(A, c1+2, s1, ""),     // 8: probe without data
		Sv(A, s1-1, c1+3, ""),  // 9: server probe before sending anything
	}, closing(3, 0)))
	wantData(t, s, model.C2S, "abc")
	for _, n := range []int{6, 8, 9} {
		wantNotes(t, s, n, model.FlagKeepAlive)
		if packet(s, n).StreamOffset != -1 {
			t.Errorf("keep-alive #%d placed in the stream", n)
		}
	}
	if len(s.Stream(model.C2S).Conflicts) != 0 || s.Status != statusComplete {
		t.Errorf("conflicts=%v status=%s", s.Stream(model.C2S).Conflicts, s.Status)
	}

	t.Run("probe opening a midstream capture", func(t *testing.T) {
		for name, pkts := range map[string][]pk{
			"after own pure ACK": {C(A, 7000, 3000, ""), C(A, 6999, 3000, "\x00"), Sv(A, 3000, 7000, ""), C(PA, 7000, 3000, "data")},
			"after peer ACK":     {Sv(A, 3000, 7000, ""), C(A, 6999, 3000, "\x00"), C(PA, 7000, 3000, "data")},
		} {
			s := one(t, Options{TargetPorts: []int{502}}, pkts)
			wantData(t, s, model.C2S, "data")
			wantGaps(t, s, model.C2S)
			wantNotes(t, s, 2, model.FlagKeepAlive)
			_ = name
		}
	})
}

func TestSequenceWraparound(t *testing.T) {
	const isn = 0xfffffff0
	full := []pk{
		C(S, isn, 0, ""),
		Sv(SA, si, isn+1, ""),
		C(A, isn+1, s1, ""),
		C(PA, isn+1, s1, "0123456789"),  // 4: 0xfffffff1..0xfffffffa
		C(PA, isn+11, s1, "abcdefghij"), // 5: crosses 2^32
		C(PA, 5, s1, "KLMNOP"),          // 6: after the wrap
		Sv(A, s1, 11, ""),               // 7
	}
	t.Run("with handshake", func(t *testing.T) {
		s := one(t, Options{}, full)
		wantData(t, s, model.C2S, "0123456789abcdefghijKLMNOP")
		wantGaps(t, s, model.C2S)
		if p := packet(s, 6); p.Seq != 21 || p.StreamOffset != 20 {
			t.Errorf("packet after wrap: seq=%d offset=%d", p.Seq, p.StreamOffset)
		}
	})
	t.Run("lost segment across the wrap", func(t *testing.T) {
		s := one(t, Options{}, slices.Delete(slices.Clone(full), 4, 5))
		wantGaps(t, s, model.C2S, Gap{Offset: 10, Length: 10, Reason: model.GapLost})
	})
	t.Run("midstream, out of order across the wrap", func(t *testing.T) {
		s := one(t, Options{TargetPorts: []int{502}}, []pk{
			C(PA, 4, 3000, "bbbb"),
			C(PA, 0xfffffffc, 3000, "aaaaaaaa"),
		})
		wantData(t, s, model.C2S, "aaaaaaaabbbb")
		wantNotes(t, s, 2, model.FlagOutOfOrder)
	})
}

func TestTrailingGaps(t *testing.T) {
	t.Run("implied by the FIN", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{
			C(PA, c1, s1, "abcd"),
			C(FA, c1+10, s1, ""),
			Sv(FA, s1, c1+11, ""),
			C(A, c1+11, s1+1, ""),
		}))
		wantData(t, s, model.C2S, "abcd\x00\x00\x00\x00\x00\x00")
		wantGaps(t, s, model.C2S, Gap{Offset: 4, Length: 6, Reason: model.GapLost})
	})
	t.Run("implied by the peer's ACK", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{C(PA, c1, s1, "abcd"), Sv(A, s1, c1+10, "")}))
		wantGaps(t, s, model.C2S, Gap{Offset: 4, Length: 6, Reason: model.GapLost})
	})
	t.Run("ACK of the FIN adds nothing", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), []pk{C(FA|P, c1, s1, "abcd"), Sv(FA, s1, c1+5, ""), C(A, c1+5, s1+1, "")}))
		wantData(t, s, model.C2S, "abcd")
		wantGaps(t, s, model.C2S)
		if s.Status != statusComplete {
			t.Errorf("status = %s", s.Status)
		}
	})
	t.Run("retransmitted FIN", func(t *testing.T) {
		s := one(t, Options{}, seq(handshake(), closing(0, 0), []pk{C(FA, c1, s1+1, "")}))
		wantNotes(t, s, 7, model.FlagRetransmission)
	})
}

func TestStreamLimit(t *testing.T) {
	s := one(t, Options{MaxStreamBytes: 8}, seq(handshake(), []pk{
		C(PA, c1, s1, "abcdef"),   // 4
		C(PA, c1+6, s1, "ghijkl"), // 5
	}, closing(12, 0)))
	wantData(t, s, model.C2S, "abcdefgh")
	if s.Status != statusError {
		t.Errorf("status = %s, want error", s.Status)
	}
	d := diag(t, s, codeStreamTooLarge)
	if d.Level != model.LevelError || d.Message != "C→S: поток превышает предел 8 Б, дальнейшие байты не сохранены" {
		t.Errorf("diagnostic = %+v", d)
	}
	segs := s.Stream(model.C2S).Segments
	if len(segs) != 2 || segs[1].Offset != 6 || segs[1].Length != 2 {
		t.Errorf("segments = %+v", segs)
	}
	if got := formatSize(DefaultMaxStreamBytes); got != "64 МиБ" {
		t.Errorf("formatSize = %q", got)
	}
}

func TestChecksumOffload(t *testing.T) {
	// The capture was taken on the client: every segment it sent fails the check.
	pkts := seq(handshake(), []pk{
		C(PA, c1, s1, "abc"),
		Sv(A, s1, c1+3, ""),
	}, closing(3, 0))
	for i := range pkts {
		if !pkts[i].fromServer {
			pkts[i] = pkts[i].bad()
		}
	}
	s := one(t, Options{}, pkts)
	wantData(t, s, model.C2S, "abc")
	wantNotes(t, s, 4, model.FlagChecksumWarning)
	wantNotes(t, s, 5)
	d := diag(t, s, codeChecksumOffload)
	if d.Level != model.LevelInfo || d.Count != 5 || !slices.Equal(d.PacketNumbers, []int{1, 3, 4, 6, 8}) ||
		d.Message != "Неверная контрольная сумма TCP — вероятно checksum offloading на узле захвата; данные используются" {
		t.Errorf("diagnostic = %+v", d)
	}
	if s.Status != statusComplete {
		t.Errorf("status = %s", s.Status)
	}
}

func TestDataOnSyn(t *testing.T) {
	s := one(t, Options{}, []pk{C(S, ci, 0, "early"), Sv(SA, si, c1+5, ""), C(PA, c1+5, s1, "late")})
	wantData(t, s, model.C2S, "earlylate")
	if p := packet(s, 1); p.Seq != 0 || p.StreamOffset != 0 {
		t.Errorf("SYN with data: seq=%d offset=%d", p.Seq, p.StreamOffset)
	}
}

func TestManyGapsAreSummarised(t *testing.T) {
	var data []pk
	for i := range uint32(26) {
		data = append(data, C(PA, c1+4*i, s1, "xx"))
	}
	s := one(t, Options{}, seq(handshake(), data))
	if n := len(s.Stream(model.C2S).Gaps); n != 25 {
		t.Fatalf("gaps = %d", n)
	}
	var individual int
	var summary *model.Diagnostic
	for i, d := range s.Diagnostics {
		if d.Code != codeGap {
			continue
		}
		if d.Count > 0 {
			summary = &s.Diagnostics[i]
		} else {
			individual++
		}
	}
	if individual != maxGapDiagnostics || summary == nil || summary.Count != 5 ||
		summary.Message != "C→S: ещё участков с неизвестными байтами: 5, всего 10 Б (потеряны сегменты)" {
		t.Errorf("individual=%d summary=%+v", individual, summary)
	}
}

func TestSessionsOrderedByFirstPacket(t *testing.T) {
	other := netip.MustParseAddrPort("10.0.0.1:40001")
	ss := assemble(t, Options{}, []pk{
		C(S, ci, 0, ""),
		C(S, 77, 0, "").ports(other, server),
		Sv(SA, si, c1, ""),
		Sv(SA, 88, 78, "").ports(server, other),
	})
	if len(ss) != 2 || ss[0].Packets[0].Number != 1 || ss[1].Packets[0].Number != 2 || ss[1].Index != 1 {
		t.Fatalf("sessions not ordered: %d", len(ss))
	}
	if ss[1].Client != other || len(ss[1].Packets) != 2 {
		t.Errorf("second session client=%v packets=%d", ss[1].Client, len(ss[1].Packets))
	}
}

func TestAddCopiesPayload(t *testing.T) {
	a := NewAssembler(Options{})
	buf := []byte("first")
	for i, p := range seq(handshake(), []pk{C(PA, c1, s1, "first"), C(PA, c1+5, s1, "secnd")}) {
		seg := p.segment()
		if len(seg.Payload) > 0 {
			copy(buf, seg.Payload)
			seg.Payload = buf // the caller reuses one buffer
		}
		a.Add(Input{Number: i + 1, Timestamp: t0, Seg: seg})
	}
	if got := string(a.Finish()[0].Stream(model.C2S).Data); got != "firstsecnd" {
		t.Errorf("data = %q", got)
	}
}

func TestSeqUnwrapper(t *testing.T) {
	u := seqUnwrapper{origin: 0xf0000000}
	steps := []int64{0, 1 << 30, 2 << 30, 3 << 30, 4 << 30, 5 << 30}
	for _, off := range steps {
		if got := u.advance(0xf0000000 + uint32(off)); got != off {
			t.Fatalf("advance to %d gave %d", off, got)
		}
	}
	behind := int64(5<<30 - 100)
	if got := u.at(0xf0000000 + uint32(behind)); got != behind {
		t.Errorf("at = %d, want %d", got, behind)
	}
}

func TestEmptyAndAckOnlyStreams(t *testing.T) {
	s := one(t, Options{}, seq(handshake(), closing(0, 0)))
	if len(s.Stream(model.C2S).Data) != 0 || len(s.Stream(model.S2C).Data) != 0 || s.Status != statusComplete {
		t.Errorf("status=%s", s.Status)
	}
	for _, st := range s.Streams {
		if st.Segments == nil || st.Gaps == nil || st.Conflicts == nil {
			t.Errorf("%s: nil slices", st.Dir)
		}
	}
	if s.Diagnostics == nil || !strings.HasPrefix(s.Roles, "hand") {
		t.Error("diagnostics must be a non-nil slice")
	}
}
