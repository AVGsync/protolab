package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AVGsync/protolab/internal/demo"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// TestControlLoad measures the control load of the task through the real
// service layer. It is skipped unless PROTOLAB_LOADTEST is set:
//
//	PROTOLAB_LOADTEST=1 go test -run TestControlLoad -v -timeout 30m ./internal/app/
//
// PROTOLAB_LOADTEST_CAPTURE=<file> keeps the generated capture in that file
// (generated once, reused while it exists); PROTOLAB_LOADTEST_HEAPPROFILE=<dir>
// writes the live heap after every phase; PROTOLAB_LOADTEST_OUT=<file> gets the
// result table. docs/performance.md has the results.
func TestControlLoad(t *testing.T) {
	if os.Getenv("PROTOLAB_LOADTEST") == "" {
		t.Skip("control load: set PROTOLAB_LOADTEST=1")
	}
	plan := controlLoadPlan()

	path := os.Getenv("PROTOLAB_LOADTEST_CAPTURE")
	if path == "" {
		path = filepath.Join(t.TempDir(), "control-load.pcapng")
	}
	var want loadStats
	statsPath := path + ".json"
	if b, err := os.ReadFile(statsPath); err == nil && json.Unmarshal(b, &want) == nil && fileExists(path) {
		t.Logf("reusing %s", path)
	} else {
		start := time.Now()
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		want, err = writeControlLoad(f, plan)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(want)
		_ = os.WriteFile(statsPath, b, 0o644)
		t.Logf("generated in %v", time.Since(start).Round(time.Millisecond))
	}
	t.Logf("capture: %.1f MiB, %d packets, %d TCP connections (%d ThermoLink), %d messages, largest %d B, %d retransmissions, %d reordered pairs",
		float64(want.Bytes)/(1<<20), want.Packets, want.TCPConns, want.TargetConns, want.Messages, want.MaxMessage, want.Retransmits, want.Reordered)

	a, err := New(Config{DataDir: t.TempDir(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	p, err := a.CreateProject(model.CreateProjectInput{Name: "Контрольная нагрузка", TargetPorts: []int{loadDevicePort}})
	if err != nil {
		t.Fatal(err)
	}

	var results []phaseResult
	measure := func(name string, fn func() string) {
		runtime.GC()
		debug.FreeOSMemory()
		resetPeakRSS()
		before := readStatus()["VmRSS"]
		var ms0 runtime.MemStats
		runtime.ReadMemStats(&ms0)
		start := time.Now()
		note := fn()
		elapsed := time.Since(start)
		peak := readStatus()["VmHWM"]
		var ms1 runtime.MemStats
		runtime.ReadMemStats(&ms1)
		runtime.GC()
		var ms2 runtime.MemStats
		runtime.ReadMemStats(&ms2)
		r := phaseResult{name, elapsed, before, peak, ms1.TotalAlloc - ms0.TotalAlloc, ms2.HeapAlloc, note}
		if dir := os.Getenv("PROTOLAB_LOADTEST_HEAPPROFILE"); dir != "" {
			// The heap that stays alive after the phase, for go tool pprof.
			if f, err := os.Create(filepath.Join(dir, fmt.Sprintf("heap-%d.prof", len(results)+1))); err == nil {
				_ = pprof.WriteHeapProfile(f)
				f.Close()
			}
		}
		results = append(results, r)
		t.Logf("%-28s %8.2f s  RSS before %5d MiB  peak %5d MiB  allocated %6d MiB  live heap after %5d MiB  %s",
			name, elapsed.Seconds(), before>>20, peak>>20, r.alloc>>20, r.live>>20, note)
	}

	var capture model.Capture
	measure("import (read+reassembly+index)", func() string {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		res, err := a.ImportCapture(p.ID, "control-load.pcapng", bufio.NewReaderSize(f, 1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if j := waitJob(t, a, res.JobID); j.Status != "succeeded" {
			t.Fatalf("import: %+v", j)
		}
		capture, _ = a.GetCapture(res.Capture.ID)
		return fmt.Sprintf("%d packets, %d sessions (%d target)", capture.PacketCount, capture.SessionCount, capture.TargetSessionCount)
	})
	if capture.PacketCount != want.Packets || capture.SessionCount != want.TCPConns || capture.TargetSessionCount != want.TargetConns {
		t.Errorf("capture: %d packets, %d sessions, %d target; want %d, %d, %d",
			capture.PacketCount, capture.SessionCount, capture.TargetSessionCount, want.Packets, want.TCPConns, want.TargetConns)
	}
	for _, d := range capture.Diagnostics {
		t.Logf("diagnostic %s %s ×%d: %s", d.Level, d.Code, d.Count, d.Message)
	}

	target := true
	sessions, err := a.ListSessions(p.ID, SessionFilter{Target: &target})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(sessions))
	for i, s := range sessions {
		ids[i] = s.ID
		if s.Status != "complete" {
			t.Errorf("session %s: %s %+v", s.ID, s.Status, s.Diagnostics)
		}
	}

	vs := loadSpec(t)
	in, err := a.CreateInterpretation(p.ID, CreateInterpretationInput{Name: "ThermoLink v3 + dump", Spec: vs})
	if err != nil {
		t.Fatal(err)
	}
	versions, _ := a.ListVersions(in.ID)
	versionID := versions[0].ID

	measure("framing+parsing (UI path)", func() string {
		n, ok, largest := 0, 0, 0
		for _, id := range ids {
			msgs, err := a.SessionMessages(id, versionID)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range msgs {
				n++
				if m.Status == model.MsgOK {
					ok++
				}
				largest = max(largest, m.Length)
			}
		}
		if n != want.Messages || ok != n {
			t.Errorf("messages: %d (ok %d), want %d all ok", n, ok, want.Messages)
		}
		return fmt.Sprintf("%d messages in %d sessions, largest %d B", n, len(ids), largest)
	})

	measure("verification run", func() string {
		corpus, err := a.CreateCorpus(p.ID, model.CreateCorpusInput{Name: "Все целевые", SessionIDs: ids})
		if err != nil {
			t.Fatal(err)
		}
		res, err := a.StartRun(p.ID, model.StartRunInput{VersionID: versionID, CorpusID: corpus.ID})
		if err != nil {
			t.Fatal(err)
		}
		if j := waitJob(t, a, res.JobID); j.Status != "succeeded" {
			t.Fatalf("run: %+v", j)
		}
		run, _ := a.GetRun(res.Run.ID)
		s := run.Summary
		if s.Messages != want.Messages || s.OK != want.Messages || s.UnknownBytes != 0 {
			t.Errorf("run summary: %+v", s)
		}
		return fmt.Sprintf("%d sessions, %d messages, %d ok, %d unframed bytes", s.Sessions, s.Messages, s.OK, s.UnknownBytes)
	})

	measure("re-analysis from file (cold)", func() string {
		pr, _ := a.project(p.ID)
		pr.mu.Lock()
		pr.invalidate()
		pr.mu.Unlock()
		res, err := pr.analysis(capture.ID)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%d sessions", len(res.sessions))
	})

	var table strings.Builder
	fmt.Fprintf(&table, "\n| Операция | Время, с | Пик RSS, МиБ | Выделено, МиБ | Живая куча после, МиБ |\n|---|---|---|---|---|\n")
	for _, r := range results {
		fmt.Fprintf(&table, "| %s | %.1f | %d | %d | %d |\n", r.name, r.elapsed.Seconds(), r.peak>>20, r.alloc>>20, r.live>>20)
	}
	t.Log(table.String())
	if out := os.Getenv("PROTOLAB_LOADTEST_OUT"); out != "" {
		_ = os.WriteFile(out, []byte(table.String()), 0o644)
	}
}

type phaseResult struct {
	name         string
	elapsed      time.Duration
	before, peak uint64
	alloc, live  uint64
	note         string
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadSpec is the demo's v3 interpretation with two additions for the
// control load: the default maximum frame length (64 KiB responses) and the
// dump request/response pair.
func loadSpec(t *testing.T) json.RawMessage {
	t.Helper()
	s := demo.Build(demo.Options{}).Versions[2].Spec
	s.Framing.MaxLength = nil
	header := func() []spec.FieldRule {
		return []spec.FieldRule{
			{Name: "magic", Offset: 0, Type: "u8"},
			{Name: "type", Offset: 1, Type: "u8"},
			{Name: "seq", Offset: 2, Type: "u8"},
			{Name: "length", Offset: 3, Type: "u16", Endian: "le"},
		}
	}
	sum := spec.FieldRule{Name: "checksum", Offset: -1, Type: "u8", Checksum: "sum8"}
	s.Messages = append(s.Messages,
		spec.MessageRule{ID: "dump_req", Direction: "c2s", Match: []spec.MatchCondition{{Offset: 0, Hex: "A5"}, {Offset: 1, Hex: "05"}},
			Fields: append(header(), spec.FieldRule{Name: "block", Offset: 5, Type: "u16", Endian: "le"}, sum)},
		spec.MessageRule{ID: "dump_resp", Direction: "s2c", Match: []spec.MatchCondition{{Offset: 0, Hex: "A5"}, {Offset: 1, Hex: "85"}},
			Fields: append(header(), spec.FieldRule{Name: "data", Offset: 5, Type: "bytes", Length: &spec.Length{Ref: "length"}}, sum)},
	)
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// resetPeakRSS resets VmHWM of this process (Linux: clear_refs value 5).
func resetPeakRSS() { _ = os.WriteFile("/proc/self/clear_refs", []byte("5"), 0) }

// readStatus returns the memory fields of /proc/self/status in bytes.
func readStatus() map[string]uint64 {
	out := map[string]uint64{}
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return out
	}
	for _, line := range bytes.Split(b, []byte("\n")) {
		name, rest, ok := strings.Cut(string(line), ":")
		if !ok || !strings.HasPrefix(name, "Vm") {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 2 && f[1] == "kB" {
			if v, err := strconv.ParseUint(f[0], 10, 64); err == nil {
				out[name] = v << 10
			}
		}
	}
	return out
}
