package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net/netip"
	"slices"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/netpkt"
	"github.com/AVGsync/protolab/internal/pcap"
	"github.com/AVGsync/protolab/internal/store"
)

// captureBuilder writes a little-endian, single-interface pcapng file and can
// splice raw blocks into it.
type captureBuilder struct {
	t   *testing.T
	buf bytes.Buffer
	w   *pcap.NgWriter
	at  time.Time
}

func newCaptureBuilder(t *testing.T) *captureBuilder {
	b := &captureBuilder{t: t, at: time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)}
	w, err := pcap.NewNgWriter(&b.buf, pcap.LinkTypeEthernet, 65535)
	if err != nil {
		t.Fatal(err)
	}
	b.w = w
	return b
}

func (b *captureBuilder) tcp(src, dst string, seq, ack uint32, flags netpkt.TCPFlags, payload string) *captureBuilder {
	frame := netpkt.BuildEthernetTCP(netip.MustParseAddrPort(src), netip.MustParseAddrPort(dst), seq, ack, flags, 8192, []byte(payload), netpkt.BuildOptions{})
	b.at = b.at.Add(time.Millisecond)
	if err := b.w.WritePacket(b.at, frame, len(frame)); err != nil {
		b.t.Fatal(err)
	}
	return b
}

// conversation is a complete TCP connection with one request and one response.
func (b *captureBuilder) conversation(client string, isn uint32, req, resp string) *captureBuilder {
	const srv, sisn = "10.0.0.2:5020", 7000
	c, s := isn+1, uint32(sisn+1)
	b.tcp(client, srv, isn, 0, netpkt.SYN, "")
	b.tcp(srv, client, sisn, c, netpkt.SYN|netpkt.ACK, "")
	b.tcp(client, srv, c, s, netpkt.ACK, "")
	b.tcp(client, srv, c, s, netpkt.PSH|netpkt.ACK, req)
	b.tcp(srv, client, s, c+uint32(len(req)), netpkt.PSH|netpkt.ACK, resp)
	c, s = c+uint32(len(req)), s+uint32(len(resp))
	b.tcp(client, srv, c, s, netpkt.FIN|netpkt.ACK, "")
	b.tcp(srv, client, s, c+1, netpkt.FIN|netpkt.ACK, "")
	return b.tcp(client, srv, c+1, s+1, netpkt.ACK, "")
}

// rawEPB appends an enhanced packet block with arbitrary header values.
func (b *captureBuilder) rawEPB(iface, capLen uint32, data []byte) *captureBuilder {
	body := binary.LittleEndian.AppendUint32(nil, iface)
	body = binary.LittleEndian.AppendUint64(body, 0)
	body = binary.LittleEndian.AppendUint32(body, capLen)
	body = binary.LittleEndian.AppendUint32(body, capLen)
	body = append(body, data...)
	for len(body)%4 != 0 {
		body = append(body, 0)
	}
	total := uint32(12 + len(body))
	blk := binary.LittleEndian.AppendUint32(nil, 6)
	blk = binary.LittleEndian.AppendUint32(blk, total)
	blk = append(blk, body...)
	blk = binary.LittleEndian.AppendUint32(blk, total)
	b.buf.Write(blk)
	return b
}

func (b *captureBuilder) bytes() []byte { return b.buf.Bytes() }

func importSync(t *testing.T, a *App, p model.Project, name string, data []byte) model.Capture {
	t.Helper()
	res, err := a.ImportCapture(p.ID, name, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if j := waitJob(t, a, res.JobID); j.Status != "succeeded" {
		t.Fatalf("import job: %+v", j)
	}
	c, err := a.GetCapture(res.Capture.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func diagnosticByCode(ds []model.Diagnostic, code string) *model.Diagnostic {
	i := slices.IndexFunc(ds, func(d model.Diagnostic) bool { return d.Code == code })
	if i < 0 {
		return nil
	}
	return &ds[i]
}

func TestDamagedCaptureImportsWhatIsReadable(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Повреждённая запись", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	b := newCaptureBuilder(t)
	b.conversation("10.0.0.1:40001", 100, "ping", "pong")  // packets 1-8
	b.rawEPB(3, 4, []byte{1, 2, 3, 4})                     // 9: undeclared interface
	b.rawEPB(0, 999, []byte{1, 2, 3, 4})                   // 10: captured length beyond the block
	b.conversation("10.0.0.1:40002", 200, "ask", "answer") // 11-18
	b.tcp("10.0.0.1:40003", "10.0.0.2:5020", 300, 0, netpkt.SYN, "")
	data := b.bytes()
	data = data[:len(data)-10] // the last block (packet 19) was cut short

	c := importSync(t, a, p, "damaged.pcapng", data)
	if c.Status != "ready" || c.SessionCount != 2 || c.PacketCount != 18 {
		t.Fatalf("capture = status %s, %d sessions, %d packets; diagnostics %+v", c.Status, c.SessionCount, c.PacketCount, c.Diagnostics)
	}
	if d := diagnosticByCode(c.Diagnostics, "damaged_record"); d == nil || d.Count != 2 || !slices.Equal(d.PacketNumbers, []int{9, 10}) {
		t.Errorf("damaged_record diagnostic = %+v", d)
	}
	if d := diagnosticByCode(c.Diagnostics, "truncated_file"); d == nil || d.Level != model.LevelWarning {
		t.Errorf("truncated_file diagnostic = %+v", d)
	}
	sessions, err := a.ListSessions(p.ID, SessionFilter{})
	if err != nil || len(sessions) != 2 {
		t.Fatalf("sessions: %d %v", len(sessions), err)
	}
	for i, want := range []string{"ask", "ping"} { // ordered by start time
		_ = i
		found := false
		for _, s := range sessions {
			got, _, err := a.StreamBytes(s.ID, model.C2S, 0, -1)
			found = found || (err == nil && string(got) == want && s.Status == "complete")
		}
		if !found {
			t.Errorf("no complete session carrying %q", want)
		}
	}
}

func TestOneOversizedSessionDoesNotBlockTheOthers(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxStreamBytes = 16
	a, err := New(Config{DataDir: t.TempDir(), Limits: limits, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Предел потока", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}
	b := newCaptureBuilder(t)
	b.conversation("10.0.0.1:40001", 100, "short", "ok")
	b.conversation("10.0.0.1:40002", 200, "this request is longer than the stream limit", "ok")
	b.conversation("10.0.0.1:40003", 300, "short", "ok")
	c := importSync(t, a, p, "limit.pcapng", b.bytes())
	if c.Status != "ready" || c.SessionCount != 3 {
		t.Fatalf("capture = %+v", c)
	}
	if d := diagnosticByCode(c.Diagnostics, "stream_too_large"); d == nil || d.Level != model.LevelError || d.Count != 1 {
		t.Errorf("stream_too_large diagnostic = %+v", d)
	}
	sessions, _ := a.ListSessions(p.ID, SessionFilter{})
	statuses := map[string]int{}
	for _, s := range sessions {
		statuses[s.Status]++
	}
	if statuses["complete"] != 2 || statuses["error"] != 1 {
		t.Errorf("statuses = %v", statuses)
	}
}

func TestLimitsAreEnforcedExplicitly(t *testing.T) {
	limits := Limits{
		MaxCaptureBytes: 4 << 10,
		MaxPackets:      12,
		MaxSessions:     1,
		MaxMessages:     1,
		MaxMessageBytes: 64 << 10,
		MaxStreamBytes:  1 << 20,
	}
	a, err := New(Config{DataDir: t.TempDir(), Limits: limits, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Пределы", TargetPorts: []int{5020}})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("capture size", func(t *testing.T) {
		big := newCaptureBuilder(t)
		for i := range 20 {
			big.conversation("10.0.0.1:41000", uint32(i)<<20, "request", "response")
		}
		_, err := a.ImportCapture(p.ID, "big.pcapng", bytes.NewReader(big.bytes()))
		if e, ok := AsError(err); !ok || e.Code != "capture_too_large" || e.Status != 413 {
			t.Fatalf("err = %v", err)
		}
	})

	b := newCaptureBuilder(t)
	b.conversation("10.0.0.1:40001", 100, "\xa5\x01\x01\x00\x00\xa7\xa5\x01\x02\x00\x00\xa8", "x")
	b.conversation("10.0.0.1:40002", 200, "second", "y")
	c := importSync(t, a, p, "limits.pcapng", b.bytes())

	t.Run("packets", func(t *testing.T) {
		d := diagnosticByCode(c.Diagnostics, "packet_limit")
		if d == nil || d.Level != model.LevelError || c.PacketCount != 12 {
			t.Errorf("packet_limit diagnostic = %+v, packet count %d", d, c.PacketCount)
		}
	})
	t.Run("sessions", func(t *testing.T) {
		d := diagnosticByCode(c.Diagnostics, "session_limit")
		if d == nil || d.Level != model.LevelError || c.SessionCount != 1 {
			t.Errorf("session_limit diagnostic = %+v, session count %d", d, c.SessionCount)
		}
	})
	t.Run("messages", func(t *testing.T) {
		in, _ := newInterpretationWithFixedFraming(t, a, p.ID)
		sessions, _ := a.ListSessions(p.ID, SessionFilter{})
		_, err := a.SessionMessages(sessions[0].ID, in)
		if e, ok := AsError(err); !ok || e.Code != "message_limit" {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("declared", func(t *testing.T) {
		got := a.SystemInfo().Limits
		if got.MaxPackets != 12 || got.MaxSessions != 1 || got.MaxMessages != 1 || got.MaxCaptureBytes != 4<<10 {
			t.Errorf("declared limits = %+v", got)
		}
	})
}

// newInterpretationWithFixedFraming creates an interpretation whose version
// cuts every stream into 6-byte messages and returns the version id.
func newInterpretationWithFixedFraming(t *testing.T, a *App, projectID string) (string, string) {
	t.Helper()
	in, err := a.CreateInterpretation(projectID, CreateInterpretationInput{
		Name: "Шесть байт",
		Spec: []byte(`{"format":"protolab/v1","framing":{"kind":"fixed","size":6},"messages":[{"id":"any","match":[],"fields":[]}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	vs, err := a.ListVersions(in.ID)
	if err != nil || len(vs) != 1 {
		t.Fatalf("versions: %v %v", vs, err)
	}
	return vs[0].ID, in.ID
}

func TestPreviousRunWithinTheSameMillisecond(t *testing.T) {
	run := func(id, at string) store.RunRecord {
		return store.RunRecord{Run: model.VerificationRun{ID: id, StartedAt: at, Status: "succeeded", InterpretationID: "i", CorpusID: "c"}}
	}
	const ms = "2026-10-12T10:00:00.123Z"
	runs := []store.RunRecord{
		run("r1", "2026-10-12T10:00:00.122Z"),
		run("r2", ms),
		run("r3", ms),
		run("r4", ms),
	}
	for _, tc := range []struct{ id, want string }{{"r1", ""}, {"r2", "r1"}, {"r3", "r2"}, {"r4", "r3"}} {
		i := slices.IndexFunc(runs, func(r store.RunRecord) bool { return r.Run.ID == tc.id })
		if got := previousRun(runs, runs[i].Run); got != tc.want {
			t.Errorf("previousRun(%s) = %q, want %q", tc.id, got, tc.want)
		}
	}
	// Other interpretations, corpora and unsuccessful runs never count.
	runs[2].Run.Status = "failed"
	runs = append(runs, store.RunRecord{Run: model.VerificationRun{ID: "x", StartedAt: ms, Status: "succeeded", InterpretationID: "j", CorpusID: "c"}})
	runs = append(runs, run("r5", ms))
	if got := previousRun(runs, runs[len(runs)-1].Run); got != "r4" {
		t.Errorf("previousRun(r5) = %q, want r4", got)
	}
	if got := previousRun(runs, runs[3].Run); got != "r2" {
		t.Errorf("previousRun(r4) = %q, want r2 (r3 failed)", got)
	}
}

func TestRunsStartedInTheSameMillisecond(t *testing.T) {
	a := newTestApp(t, t.TempDir())
	p, err := a.CreateDemoProject(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	pr, _ := a.project(p.ID)
	runs, _ := a.ListRuns(p.ID)
	base := runs[0] // the latest demo run
	const at = "2099-01-01T00:00:00.000Z"
	var ids []string
	for range 3 {
		rec, err := a.newRun(pr, base.VersionID, base.CorpusID, at)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.executeRun(context.Background(), pr, rec.Run.ID, nil); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.Run.ID)
	}
	runs, _ = a.ListRuns(p.ID)
	if runs[0].ID != ids[2] || runs[1].ID != ids[1] || runs[2].ID != ids[0] {
		t.Fatalf("listing order = %s %s %s, want %v reversed", runs[0].ID, runs[1].ID, runs[2].ID, ids)
	}
	want := []string{base.ID, ids[0], ids[1]}
	for i, id := range ids {
		r, _ := a.GetRun(id)
		if r.PreviousRunID != want[i] {
			t.Errorf("run %d: previousRunId = %q, want %q", i, r.PreviousRunID, want[i])
		}
	}
}
