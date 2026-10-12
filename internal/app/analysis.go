package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"sync/atomic"
	"time"

	"github.com/AVGsync/protolab/internal/jobs"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/pcap"
	"github.com/AVGsync/protolab/internal/tcpflow"
)

// analysisSlot computes the analysis of one capture at most once per settings generation.
type analysisSlot struct {
	started atomic.Bool
	done    chan struct{} // closed once res/err are set
	res     *captureAnalysis
	err     error
}

func newSlot() *analysisSlot { return &analysisSlot{done: make(chan struct{})} }

func filledSlot(res *captureAnalysis) *analysisSlot {
	s := newSlot()
	s.started.Store(true)
	s.res = res
	close(s.done)
	return s
}

// get runs compute in the first caller and makes the others wait for its result.
func (s *analysisSlot) get(compute func() (*captureAnalysis, error)) (*captureAnalysis, error) {
	if s.started.CompareAndSwap(false, true) {
		s.res, s.err = compute()
		close(s.done)
	} else {
		<-s.done
	}
	return s.res, s.err
}

// peek returns the result without waiting; ok is false while it is not ready.
func (s *analysisSlot) peek() (res *captureAnalysis, err error, ok bool) {
	select {
	case <-s.done:
		return s.res, s.err, true
	default:
		return nil, nil, false
	}
}

// sessionData is a session with everything derived from it.
type sessionData struct {
	dto     model.TcpSession // ActionIDs are filled when the session is read
	flow    *tcpflow.Session
	streams [2]model.DirectedStream
	packets []model.Packet
}

func (s *sessionData) stream(d model.Direction) *model.DirectedStream {
	if d == model.C2S {
		return &s.streams[0]
	}
	return &s.streams[1]
}

// captureAnalysis is the result of reading and reassembling a capture file.
type captureAnalysis struct {
	captureID   string
	sha256      string
	format      string
	linkType    string
	snapLen     int
	interfaces  int
	packetCount int
	first, last time.Time
	sessions    []*sessionData
	byID        map[string]*sessionData
	targets     int
	diagnostics []model.Diagnostic
}

type analyzeInput struct {
	path      string
	projectID string
	capture   model.Capture
	settings  model.ProjectSettings
	limits    Limits
}

// analysis returns the (cached) analysis of a capture.
func (p *project) analysis(captureID string) (*captureAnalysis, error) {
	p.mu.Lock()
	idx := findIndex(p.st.Captures, func(c *storeCapture) bool { return c.ID == captureID })
	if idx < 0 {
		p.mu.Unlock()
		return nil, notFound("Запись")
	}
	rec := p.st.Captures[idx]
	if rec.Status == "indexing" {
		p.mu.Unlock()
		return nil, conflict("not_ready", "Запись %s ещё индексируется", rec.FileName)
	}
	slot := p.slots[captureID]
	if slot == nil {
		slot = newSlot()
		p.slots[captureID] = slot
	}
	in := analyzeInput{
		path:      filepath.Join(p.dir(), filepath.FromSlash(rec.File)),
		projectID: p.id,
		capture:   rec.Capture,
		settings:  p.st.Project.Settings,
		limits:    p.app.cfg.Limits,
	}
	p.mu.Unlock()

	return slot.get(func() (*captureAnalysis, error) { return analyzeFile(p.app.ctx, in, nil) })
}

// setAnalysis stores an analysis computed during import; the caller holds p.mu.
func (p *project) setAnalysis(captureID string, res *captureAnalysis) {
	p.slots[captureID] = filledSlot(res)
}

// countingReader reports progress by bytes read.
type countingReader struct {
	r    io.Reader
	n    int64
	tick func(n int64)
}

func (c *countingReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += int64(n)
	if c.tick != nil {
		c.tick(c.n)
	}
	return n, err
}

type decodeStat struct {
	count   int
	packets []int
}

// analyzeFile reads a capture, reassembles TCP and builds all DTOs.
// The file's SHA-256 is computed on the fly and compared with the recorded one.
func analyzeFile(ctx context.Context, in analyzeInput, progress jobs.Progress) (*captureAnalysis, error) {
	if progress == nil {
		progress = func(float64, string) {}
	}
	f, err := os.Open(in.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("исходный файл записи отсутствует: %s", filepath.Base(in.path))
		}
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := max(info.Size(), 1)

	hasher := sha256.New()
	counter := &countingReader{r: io.TeeReader(f, hasher)}
	var reported int64
	counter.tick = func(n int64) {
		if n-reported >= 1<<20 || n == size {
			reported = n
			progress(0.8*float64(n)/float64(size), "Чтение пакетов")
		}
	}
	br := bufio.NewReaderSize(counter, 1<<20)

	r, err := pcap.NewReader(br)
	if err != nil {
		return nil, err
	}

	res := &captureAnalysis{captureID: in.capture.ID, format: string(r.Format())}
	asm := tcpflow.NewAssembler(tcpflow.Options{
		OverlapPolicy:  in.settings.OverlapPolicy,
		TargetPorts:    in.settings.TargetPorts,
		MaxStreamBytes: in.limits.MaxStreamBytes,
	})
	decodeErrs := map[string]*decodeStat{}
	var readDiag []model.Diagnostic

	for {
		if res.packetCount%1024 == 0 && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pkt, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if errors.Is(err, pcap.ErrTruncatedFile) {
			readDiag = append(readDiag, model.Diagnostic{
				Level:   model.LevelWarning,
				Code:    "truncated_file",
				Message: fmt.Sprintf("Файл обрывается после пакета %d: последняя запись неполная и пропущена", res.packetCount),
			})
			break
		}
		if err != nil {
			readDiag = append(readDiag, model.Diagnostic{
				Level:   model.LevelError,
				Code:    "read_error",
				Message: fmt.Sprintf("Ошибка чтения после пакета %d: %v. Остаток файла не обработан", res.packetCount, err),
			})
			break
		}
		if pkt.Number > in.limits.MaxPackets {
			readDiag = append(readDiag, model.Diagnostic{
				Level:   model.LevelError,
				Code:    "packet_limit",
				Message: fmt.Sprintf("Превышен предел %d пакетов: анализ остановлен, остальные пакеты не обработаны", in.limits.MaxPackets),
			})
			break
		}
		res.packetCount = pkt.Number
		if res.first.IsZero() || pkt.Timestamp.Before(res.first) {
			res.first = pkt.Timestamp
		}
		if pkt.Timestamp.After(res.last) {
			res.last = pkt.Timestamp
		}

		seg, err := netpkt.Decode(pkt.LinkType, pkt.Data, pkt.OrigLen)
		if err != nil {
			code := netpkt.CodeMalformed
			var de *netpkt.DecodeError
			if errors.As(err, &de) {
				code = de.Code
			}
			st := decodeErrs[code]
			if st == nil {
				st = &decodeStat{}
				decodeErrs[code] = st
			}
			st.count++
			if len(st.packets) < 10 {
				st.packets = append(st.packets, pkt.Number)
			}
			continue
		}
		asm.Add(tcpflow.Input{Number: pkt.Number, Timestamp: pkt.Timestamp, Seg: seg})
	}

	// Finish hashing the parts of the file the reader did not consume.
	if _, err := io.Copy(io.Discard, br); err != nil {
		return nil, err
	}
	res.sha256 = hex.EncodeToString(hasher.Sum(nil))

	ifaces := r.Interfaces()
	res.interfaces = len(ifaces)
	if len(ifaces) > 0 {
		res.linkType = pcap.LinkTypeName(ifaces[0].LinkType)
		res.snapLen = int(ifaces[0].SnapLen)
	}

	progress(0.85, "Сборка TCP-сессий")
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	flows := asm.Finish()

	var sessionDiag []model.Diagnostic
	if len(flows) > in.limits.MaxSessions {
		sessionDiag = append(sessionDiag, model.Diagnostic{
			Level:   model.LevelError,
			Code:    "session_limit",
			Message: fmt.Sprintf("Превышен предел %d TCP-сессий: показаны первые %d из %d", in.limits.MaxSessions, in.limits.MaxSessions, len(flows)),
		})
		flows = flows[:in.limits.MaxSessions]
	}

	progress(0.95, "Индексация")
	res.byID = make(map[string]*sessionData, len(flows))
	for _, fl := range flows {
		sd := buildSession(in, fl)
		res.sessions = append(res.sessions, sd)
		res.byID[sd.dto.ID] = sd
		if sd.dto.IsTarget {
			res.targets++
		}
	}

	res.diagnostics = append(res.diagnostics, readDiag...)
	res.diagnostics = append(res.diagnostics, decodeDiagnostics(decodeErrs, ifaces)...)
	res.diagnostics = append(res.diagnostics, sessionDiag...)
	res.diagnostics = append(res.diagnostics, aggregateSessionDiagnostics(res.sessions)...)
	if in.capture.SHA256 != "" && in.capture.SHA256 != res.sha256 {
		res.diagnostics = append([]model.Diagnostic{{
			Level:   model.LevelError,
			Code:    "integrity_mismatch",
			Message: "SHA-256 файла не совпадает с записанным при импорте: исходная запись изменена, выводы по ней недостоверны",
		}}, res.diagnostics...)
	}
	return res, nil
}

var decodeMessages = map[string]struct{ level, text string }{
	netpkt.CodeNotIP:           {model.LevelInfo, "Кадры без IPv4 (ARP, LLDP и др.) пропущены"},
	netpkt.CodeNotTCP:          {model.LevelInfo, "Пакеты IPv4 без TCP (UDP, ICMP и др.) пропущены"},
	netpkt.CodeIPv6:            {model.LevelWarning, "IPv6 не поддерживается в основном сценарии — пакеты пропущены"},
	netpkt.CodeFragment:        {model.LevelWarning, "Фрагменты IPv4 не собираются — пакеты пропущены"},
	netpkt.CodeUnsupportedLink: {model.LevelError, "Неподдерживаемый тип канального уровня — пакеты пропущены"},
	netpkt.CodeMalformed:       {model.LevelWarning, "Повреждённые заголовки — пакеты пропущены"},
	netpkt.CodeTruncatedHeader: {model.LevelWarning, "Заголовки TCP/IP обрезаны при захвате — пакеты пропущены"},
}

func decodeDiagnostics(stats map[string]*decodeStat, ifaces []pcap.Interface) []model.Diagnostic {
	codes := make([]string, 0, len(stats))
	for code := range stats {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	out := make([]model.Diagnostic, 0, len(codes))
	for _, code := range codes {
		st := stats[code]
		m, ok := decodeMessages[code]
		if !ok {
			m = struct{ level, text string }{model.LevelWarning, "Пакеты не разобраны (" + code + ")"}
		}
		text := m.text
		if code == netpkt.CodeUnsupportedLink && len(ifaces) > 0 {
			text = fmt.Sprintf("Неподдерживаемый тип канального уровня %s — пакеты пропущены", pcap.LinkTypeName(ifaces[0].LinkType))
		}
		out = append(out, model.Diagnostic{Level: m.level, Code: code, Message: text, Count: st.count, PacketNumbers: st.packets})
	}
	return out
}

var captureLevelMessage = map[string]string{
	"tcp_gap":           "Потерянные TCP-сегменты: неизвестные участки помечены, нулями не подменяются",
	"tcp_truncated":     "Пакеты усечены snaplen: хвосты полезной нагрузки неизвестны",
	"overlap_conflict":  "Повторные передачи с различающимися данными (выбор по политике проекта)",
	"missing_handshake": "Соединения без начала: роли сторон не установлены по рукопожатию",
	"checksum_offload":  "Неверные контрольные суммы TCP (вероятно checksum offloading)",
	"stream_too_large":  "Потоки превышают предел размера: часть байтов не сохранена",
}

// aggregateSessionDiagnostics summarises session diagnostics per code for the capture.
func aggregateSessionDiagnostics(sessions []*sessionData) []model.Diagnostic {
	type agg struct {
		d     model.Diagnostic
		order int
	}
	byCode := map[string]*agg{}
	for _, s := range sessions {
		for _, d := range s.dto.Diagnostics {
			n := max(d.Count, 1)
			if a, ok := byCode[d.Code]; ok {
				a.d.Count += n
				continue
			}
			msg, ok := captureLevelMessage[d.Code]
			if !ok {
				msg = d.Message
			}
			byCode[d.Code] = &agg{d: model.Diagnostic{Level: d.Level, Code: d.Code, Message: msg, Count: n}, order: len(byCode)}
		}
	}
	list := make([]*agg, 0, len(byCode))
	for _, a := range byCode {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].order < list[j].order })
	out := make([]model.Diagnostic, len(list))
	for i, a := range list {
		out[i] = a.d
	}
	return out
}

func endpoint(ap netip.AddrPort) model.Endpoint {
	return model.Endpoint{IP: ap.Addr().String(), Port: int(ap.Port())}
}

// buildSession converts a reassembled flow into API objects.
func buildSession(in analyzeInput, fl *tcpflow.Session) *sessionData {
	ignoreChecksum := in.settings.ChecksumPolicy == "ignore"
	id := sessionID(in.capture.ID, fl.Index)
	cleanFlags := func(flags []string) []string {
		out := make([]string, 0, len(flags))
		for _, f := range flags {
			if ignoreChecksum && f == model.FlagChecksumWarning {
				continue
			}
			out = append(out, f)
		}
		return out
	}

	sd := &sessionData{flow: fl}
	flags := model.SessionFlags{Handshake: fl.Handshake, Closed: fl.Closed}

	sd.packets = make([]model.Packet, len(fl.Packets))
	for i, p := range fl.Packets {
		var off *int
		if p.StreamOffset >= 0 {
			v := p.StreamOffset
			off = &v
		}
		ann := cleanFlags(p.Annotations)
		for _, a := range ann {
			switch a {
			case model.FlagRetransmission:
				flags.Retransmissions++
			case model.FlagOutOfOrder:
				flags.OutOfOrder++
			case model.FlagChecksumWarning:
				flags.ChecksumWarnings++
			}
		}
		sd.packets[i] = model.Packet{
			Number:        p.Number,
			Timestamp:     model.PacketTime(p.Timestamp),
			Direction:     p.Dir,
			TCPFlags:      nonNil(p.Flags.Names()),
			Seq:           p.Seq,
			Ack:           p.Ack,
			PayloadLength: p.PayloadLen,
			StreamOffset:  off,
			Flags:         ann,
		}
	}

	for i, dir := range model.Directions {
		st := fl.Streams[i]
		ds := model.DirectedStream{
			SessionID:  id,
			Direction:  dir,
			Segments:   []model.StreamSegment{},
			Gaps:       []model.StreamGap{},
			Conflicts:  []model.OverlapConflict{},
			StartKnown: true,
		}
		if st != nil {
			ds.Length = len(st.Data)
			ds.StartKnown = st.StartKnown
			for _, s := range st.Segments {
				ds.Segments = append(ds.Segments, model.StreamSegment{
					Offset:       s.Offset,
					Length:       s.Length,
					PacketNumber: s.Packet,
					Timestamp:    model.PacketTime(s.Timestamp),
					Flags:        cleanFlags(s.Annotations),
				})
			}
			for _, g := range st.Gaps {
				ds.Gaps = append(ds.Gaps, model.StreamGap{Offset: g.Offset, Length: g.Length, Reason: g.Reason})
			}
			for _, c := range st.Conflicts {
				ds.Conflicts = append(ds.Conflicts, model.OverlapConflict{
					Offset: c.Offset, Length: c.Length, PacketNumbers: slices.Clone(c.Packets), ChosenPacket: c.Chosen,
				})
			}
		}
		flags.Gaps += len(ds.Gaps)
		flags.OverlapConflicts += len(ds.Conflicts)
		sd.streams[i] = ds
	}

	diags := make([]model.Diagnostic, 0, len(fl.Diagnostics))
	for _, d := range fl.Diagnostics {
		if ignoreChecksum && d.Code == "checksum_offload" {
			continue
		}
		d.SessionID = id
		diags = append(diags, d)
	}

	sd.dto = model.TcpSession{
		ID:          id,
		ProjectID:   in.projectID,
		CaptureID:   in.capture.ID,
		Capture:     model.CaptureRef{FileName: in.capture.FileName, SHA256: in.capture.SHA256},
		Index:       fl.Index,
		Client:      endpoint(fl.Client),
		Server:      endpoint(fl.Server),
		Roles:       fl.Roles,
		StartedAt:   model.PacketTime(fl.Start),
		EndedAt:     model.PacketTime(fl.End),
		PacketCount: len(fl.Packets),
		Bytes:       model.ByteCounts{C2S: sd.streams[0].Length, S2C: sd.streams[1].Length},
		Status:      fl.Status,
		Flags:       flags,
		IsTarget:    isTarget(in.settings.TargetPorts, int(fl.Server.Port())),
		Diagnostics: diags,
		ActionIDs:   []string{},
	}
	return sd
}

// isTarget: without configured ports every session is a research target.
func isTarget(ports []int, port int) bool {
	return len(ports) == 0 || slices.Contains(ports, port)
}
