package tcpflow

import (
	"fmt"
	"slices"
	"time"

	"github.com/AVGsync/protolab/internal/model"
)

// Session statuses.
const (
	statusComplete = "complete"
	statusPartial  = "partial"
	statusError    = "error"
)

// Diagnostic codes.
const (
	codeGap              = "tcp_gap"
	codeTruncated        = "tcp_truncated"
	codeOverlapConflict  = "overlap_conflict"
	codeMissingHandshake = "missing_handshake"
	codeChecksumOffload  = "checksum_offload"
	codeChecksumBad      = "checksum_error"
	codeStreamTooLarge   = "stream_too_large"
	codeReassemblyFailed = "reassembly_failed"
)

const (
	// maxGapDiagnostics bounds the per-gap diagnostics of one direction and
	// reason; further gaps are summarised in one diagnostic with a count.
	// A snaplen-limited capture has a truncated gap after almost every packet.
	maxGapDiagnostics = 20
	// maxPacketRefs bounds the example packets listed by a gap diagnostic.
	maxPacketRefs = 10
)

func (b *builder) session(packets []Packet, checks checksumStats) *Session {
	s := &Session{
		Client:    b.client,
		Server:    b.server(),
		Roles:     b.source.roles(),
		Packets:   packets,
		Handshake: b.source == byHandshake,
		Closed:    b.c.closed(),
	}
	s.Start, s.End = timeSpan(b.c.inputs)
	for i, h := range b.halves {
		s.Streams[i] = h.stream()
	}
	s.Status = b.status(s, checks)
	s.Diagnostics = b.diagnostics(s, checks)
	return s
}

// timeSpan returns the earliest and the latest packet time. They are the first
// and the last packet's times unless the capture's clock went backwards.
func timeSpan(inputs []Input) (start, end time.Time) {
	start, end = inputs[0].Timestamp, inputs[0].Timestamp
	for _, in := range inputs[1:] {
		if in.Timestamp.Before(start) {
			start = in.Timestamp
		}
		if in.Timestamp.After(end) {
			end = in.Timestamp
		}
	}
	return start, end
}

func (b *builder) status(s *Session, checks checksumStats) string {
	if b.halves[0].tooLarge || b.halves[1].tooLarge {
		return statusError
	}
	if checks.unverified {
		return statusPartial
	}
	for _, st := range s.Streams {
		if len(st.Gaps) > 0 || len(st.Conflicts) > 0 {
			return statusPartial
		}
	}
	if s.Handshake && s.Closed {
		return statusComplete
	}
	return statusPartial
}

var missingHandshakeMessages = map[roleSource]string{
	bySynAck:      "Начало соединения не захвачено: SYN клиента отсутствует, роли определены по SYN-ACK",
	byPort:        "Начало соединения не захвачено: роли определены по порту, первые байты потока неизвестны",
	byFirstSender: "Начало соединения не захвачено: роли сторон не установлены, первые байты потока неизвестны",
}

func (b *builder) diagnostics(s *Session, checks checksumStats) []model.Diagnostic {
	diags := []model.Diagnostic{}
	if msg, ok := missingHandshakeMessages[b.source]; ok {
		diags = append(diags, model.Diagnostic{Level: model.LevelWarning, Code: codeMissingHandshake, Message: msg})
	}
	for i, h := range b.halves {
		st := s.Streams[i]
		diags = h.gapDiagnostics(diags, st.Gaps)
		diags = conflictDiagnostics(diags, h.dir, st.Conflicts)
		switch {
		case h.overall:
			diags = append(diags, model.Diagnostic{
				Level: model.LevelError,
				Code:  codeStreamTooLarge,
				Message: fmt.Sprintf("%s: потоки записи вместе превышают предел %s, байты этого потока после смещения %d не сохранены",
					arrow(h.dir), formatSize(b.opt.maxTotal), h.length),
			})
		case h.tooLarge:
			diags = append(diags, model.Diagnostic{
				Level:   model.LevelError,
				Code:    codeStreamTooLarge,
				Message: fmt.Sprintf("%s: поток превышает предел %s, дальнейшие байты не сохранены", arrow(h.dir), formatSize(h.length)),
			})
		}
	}
	if checks.offloaded > 0 {
		diags = append(diags, model.Diagnostic{
			Level:         model.LevelInfo,
			Code:          codeChecksumOffload,
			Message:       "Неверная контрольная сумма TCP — вероятно checksum offloading на узле захвата; данные используются",
			Count:         checks.offloaded,
			PacketNumbers: checks.offloadedRef,
		})
	}
	if checks.damaged > 0 {
		diags = append(diags, model.Diagnostic{
			Level: model.LevelWarning,
			Code:  codeChecksumBad,
			Message: "Неверная контрольная сумма TCP у пакетов отправителя, чьи остальные пакеты верны — вероятно повреждение: " +
				"их номера последовательности и подтверждения не учитываются, байты используются, только если нет копии с верной суммой",
			Count:         checks.damaged,
			PacketNumbers: checks.damagedRef,
		})
	}
	return diags
}

func (h *half) gapDiagnostics(diags []model.Diagnostic, gaps []Gap) []model.Diagnostic {
	type rest struct{ gaps, bytes int }
	var shown [2]int
	var skipped [2]rest
	for _, g := range gaps {
		kind := 0
		if g.Reason == model.GapTruncated {
			kind = 1
		}
		if shown[kind] == maxGapDiagnostics {
			skipped[kind].gaps++
			skipped[kind].bytes += g.Length
			continue
		}
		shown[kind]++
		from, to := int64(g.Offset), int64(g.Offset+g.Length)
		d := model.Diagnostic{
			Level:         model.LevelWarning,
			Code:          codeGap,
			Message:       fmt.Sprintf("%s: %d Б неизвестны на смещении 0x%X (потерян сегмент)", arrow(h.dir), g.Length, g.Offset),
			PacketNumbers: h.neighbours(from, to),
		}
		if kind == 1 {
			d.Code = codeTruncated
			d.Message = fmt.Sprintf("%s: %d Б неизвестны на смещении 0x%X (пакет усечён snaplen)", arrow(h.dir), g.Length, g.Offset)
			d.PacketNumbers = h.truncatedBy(from, to, maxPacketRefs)
		}
		diags = append(diags, d)
	}
	for kind, r := range skipped {
		if r.gaps == 0 {
			continue
		}
		code, reason := codeGap, "потеряны сегменты"
		if kind == 1 {
			code, reason = codeTruncated, "пакеты усечены snaplen"
		}
		diags = append(diags, model.Diagnostic{
			Level:   model.LevelWarning,
			Code:    code,
			Message: fmt.Sprintf("%s: ещё участков с неизвестными байтами: %d, всего %d Б (%s)", arrow(h.dir), r.gaps, r.bytes, reason),
			Count:   r.gaps,
		})
	}
	return diags
}

// conflictDiagnostics reports each set of competing packets once, with the
// total number of bytes they disagree on.
func conflictDiagnostics(diags []model.Diagnostic, dir model.Direction, conflicts []Conflict) []model.Diagnostic {
	type group struct {
		packets []int
		chosen  int
		bytes   int
	}
	var groups []*group
	for _, c := range conflicts {
		i := slices.IndexFunc(groups, func(g *group) bool {
			return g.chosen == c.Chosen && slices.Equal(g.packets, c.Packets)
		})
		if i < 0 {
			groups = append(groups, &group{packets: c.Packets, chosen: c.Chosen})
			i = len(groups) - 1
		}
		groups[i].bytes += c.Length
	}
	for _, g := range groups {
		diags = append(diags, model.Diagnostic{
			Level:         model.LevelError,
			Code:          codeOverlapConflict,
			Message:       fmt.Sprintf("%s: повторная передача с другими байтами (%d Б), использован пакет #%d", arrow(dir), g.bytes, g.chosen),
			PacketNumbers: slices.Clone(g.packets),
		})
	}
	return diags
}

func arrow(d model.Direction) string {
	if d == model.C2S {
		return "C→S"
	}
	return "S→C"
}

func formatSize(n int64) string {
	switch {
	case n > 0 && n%(1<<20) == 0:
		return fmt.Sprintf("%d МиБ", n>>20)
	case n > 0 && n%(1<<10) == 0:
		return fmt.Sprintf("%d КиБ", n>>10)
	}
	return fmt.Sprintf("%d Б", n)
}
