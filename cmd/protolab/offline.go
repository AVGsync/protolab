package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/AVGsync/protolab/internal/app"
	"github.com/AVGsync/protolab/internal/engine"
	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/report"
	"github.com/AVGsync/protolab/internal/spec"
)

func runDemo(args []string) int {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	data := fs.String("data", defaultDataDir(), "каталог данных")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	a, err := app.New(app.Config{DataDir: *data, Build: app.BuildInfo{Version: version, Commit: commit, Date: date}, Logger: logger})
	if err != nil {
		return fail("%v", err)
	}
	defer a.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	p, err := a.CreateDemoProject(ctx)
	if err != nil {
		return fail("%v", err)
	}
	fmt.Printf("Демо-проект «%s» создан: %s (%s)\n", p.Name, p.ID, a.DataDir())
	return 0
}

// loadSpec reads a spec file, accepting files exported with a "$meta" object.
func loadSpec(path string) (*spec.Spec, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}
	raw, err = report.StripMeta(raw)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	s, res := spec.Parse(raw)
	if !res.Valid {
		var b strings.Builder
		for _, is := range res.Issues {
			if is.Severity == "error" {
				fmt.Fprintf(&b, "\n  %s: %s", is.Path, is.Message)
			}
		}
		return nil, "", fmt.Errorf("%s: спецификация содержит ошибки:%s", path, b.String())
	}
	return s, spec.Hash(s), nil
}

func addrString(e model.Endpoint) string { return e.IP + ":" + strconv.Itoa(e.Port) }

type appliedSession struct {
	TCPStream int             `json:"tcpStream"`
	Client    string          `json:"client"`
	Server    string          `json:"server"`
	Status    string          `json:"status"`
	Messages  []model.Message `json:"messages"`
}

type applyOutput struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	Spec    struct {
		File     string `json:"file"`
		Protocol string `json:"protocol"`
		Hash     string `json:"hash"`
	} `json:"spec"`
	Capture struct {
		File        string             `json:"file"`
		SHA256      string             `json:"sha256"`
		Format      string             `json:"format"`
		Packets     int                `json:"packets"`
		Diagnostics []model.Diagnostic `json:"diagnostics"`
	} `json:"capture"`
	Summary  model.RunSummary `json:"summary"`
	Sessions []appliedSession `json:"sessions"`
}

// runApply applies an interpretation to a capture without the UI.
func runApply(args []string) int {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	specPath := fs.String("spec", "", "файл интерпретации (JSON)")
	input := fs.String("input", "", "запись PCAP/PCAPNG")
	out := fs.String("out", "-", "куда записать результат (JSON), - = stdout")
	port := fs.Int("port", 0, "порт устройства (по умолчанию из spec.transport.port)")
	all := fs.Bool("all", false, "разобрать все TCP-сессии, а не только порт устройства")
	overlap := fs.String("overlap", "first", "политика перекрытий: first | last")
	strict := fs.Bool("strict", false, "код выхода 3, если есть нарушения или неполные сообщения")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *specPath == "" || *input == "" {
		return fail("нужны --spec и --input")
	}
	sp, hash, err := loadSpec(*specPath)
	if err != nil {
		return fail("%v", err)
	}
	devicePort := *port
	if devicePort == 0 && sp.Transport != nil {
		devicePort = sp.Transport.Port
	}
	settings := model.ProjectSettings{OverlapPolicy: *overlap, ChecksumPolicy: "warn", TargetPorts: []int{}}
	if devicePort > 0 {
		settings.TargetPorts = []int{devicePort}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	capture, err := app.AnalyzeFile(ctx, *input, settings, app.Limits{})
	if err != nil {
		return fail("%s: %v", *input, err)
	}

	var res applyOutput
	res.Tool, res.Version = "protolab", version
	res.Spec.File, res.Spec.Protocol, res.Spec.Hash = filepath.Base(*specPath), sp.Protocol, hash
	res.Capture.File, res.Capture.SHA256, res.Capture.Format = capture.File, capture.SHA256, capture.Format
	res.Capture.Packets, res.Capture.Diagnostics = capture.Packets, capture.Diagnostics
	res.Sessions = []appliedSession{}

	var inputs []engine.SessionInput
	for _, in := range capture.Sessions {
		if !*all && !in.Session.IsTarget {
			continue
		}
		parsed := engine.ParseSession(sp, in)
		res.Sessions = append(res.Sessions, appliedSession{
			TCPStream: in.Session.Index,
			Client:    addrString(in.Session.Client),
			Server:    addrString(in.Session.Server),
			Status:    in.Session.Status,
			Messages:  parsed.Messages,
		})
		inputs = append(inputs, in)
	}
	result, err := engine.Verify(ctx, sp, inputs, nil)
	if err != nil {
		return fail("%v", err)
	}
	res.Summary = result.Summary

	if err := writeJSONFile(*out, res); err != nil {
		return fail("%v", err)
	}
	s := res.Summary
	fmt.Fprintf(os.Stderr, "%s: %d сессий, %d сообщений; разобрано %d, нарушений %d, неполных %d, неизвестный тип %d, неописано %d из %d Б\n",
		capture.File, s.Sessions, s.Messages, s.OK, s.Violations, s.Partial, s.UnknownType, s.UnknownBytes, s.TotalBytes)
	if *strict && (s.Violations > 0 || s.Partial > 0) {
		return 3
	}
	return 0
}

func writeJSONFile(path string, v any) error {
	var w io.Writer = os.Stdout
	if path != "-" {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// runVerify replays the examples file of a report and compares the summaries.
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	examplesPath := fs.String("examples", "", "файл примеров отчёта (examples.*.json)")
	specOverride := fs.String("spec", "", "другая интерпретация вместо указанной в примерах")
	base := fs.String("base", "", "корень проекта, относительно которого лежат captures/ (по умолчанию ищется вверх от файла примеров)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *examplesPath == "" {
		return fail("нужен --examples")
	}
	raw, err := os.ReadFile(*examplesPath)
	if err != nil {
		return fail("%v", err)
	}
	var ex report.Examples
	if err := json.Unmarshal(raw, &ex); err != nil {
		return fail("%s: %v", *examplesPath, err)
	}
	if ex.Format != report.ExamplesFormat {
		return fail("%s: неизвестный формат %q", *examplesPath, ex.Format)
	}
	dir := filepath.Dir(*examplesPath)
	specPath := *specOverride
	if specPath == "" {
		specPath = filepath.Join(dir, ex.Spec)
	}
	sp, hash, err := loadSpec(specPath)
	if err != nil {
		return fail("%v", err)
	}
	if hash != ex.SpecHash {
		fmt.Fprintf(os.Stderr, "внимание: хеш интерпретации %s отличается от проверенного в отчёте %s\n", short(hash), short(ex.SpecHash))
	}
	if ex.Engine != "" && ex.Engine != engine.Version {
		fmt.Fprintf(os.Stderr, "внимание: отчёт сделан движком %s, сейчас %s\n", ex.Engine, engine.Version)
	}

	roots := captureRoots(*base, dir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	type analysisKey struct {
		path              string
		overlap, checksum string
	}
	cache := map[analysisKey]*app.OfflineCapture{}

	failed := false
	for _, c := range ex.Corpora {
		settings := exampleSettings(ex, c)
		var inputs []engine.SessionInput
		problems := []string{}
		for _, ref := range c.Sessions {
			path, ok := findCapture(roots, ref.Capture)
			if !ok {
				problems = append(problems, "не найден файл "+ref.Capture)
				continue
			}
			key := analysisKey{path, settings.OverlapPolicy, settings.ChecksumPolicy}
			capture := cache[key]
			if capture == nil {
				if capture, err = app.AnalyzeFile(ctx, path, settings, app.Limits{}); err != nil {
					problems = append(problems, fmt.Sprintf("%s: %v", ref.Capture, err))
					continue
				}
				cache[key] = capture
			}
			if capture.SHA256 != ref.SHA256 {
				problems = append(problems, fmt.Sprintf("%s: SHA-256 не совпадает с отчётом — запись изменена", ref.Capture))
				continue
			}
			in, ok := sessionByRef(capture, ref)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: сессия tcp.stream=%d %s → %s не найдена", ref.Capture, ref.TCPStream, ref.Client, ref.Server))
				continue
			}
			in.Actions = ref.Actions
			inputs = append(inputs, in)
		}
		result, err := engine.Verify(ctx, sp, inputs, nil)
		if err != nil {
			return fail("%v", err)
		}
		diffs := compareSummaries(c.Expected, result.Summary)
		status := "совпадает"
		if len(problems) > 0 || len(diffs) > 0 {
			status = "РАСХОЖДЕНИЕ"
			failed = true
		}
		fmt.Printf("%-28s %s (%d сессий, %d сообщений)\n", c.Name, status, result.Summary.Sessions, result.Summary.Messages)
		for _, p := range problems {
			fmt.Printf("  ! %s\n", p)
		}
		for _, d := range diffs {
			fmt.Printf("  - %s\n", d)
		}
	}
	if failed {
		return 1
	}
	return 0
}

// exampleSettings are the reassembly settings a corpus of the examples file
// was verified with: its own when recorded, else the file's. Files written
// before the checksum policy was recorded were made with "warn".
func exampleSettings(ex report.Examples, c report.CorpusExample) model.ProjectSettings {
	es := ex.Settings
	if c.Settings != nil {
		es.OverlapPolicy, es.ChecksumPolicy = c.Settings.OverlapPolicy, c.Settings.ChecksumPolicy
		if c.Settings.TargetPorts != nil {
			es.TargetPorts = c.Settings.TargetPorts
		}
	}
	out := model.ProjectSettings{OverlapPolicy: es.OverlapPolicy, ChecksumPolicy: es.ChecksumPolicy, TargetPorts: es.TargetPorts}
	if out.OverlapPolicy == "" {
		out.OverlapPolicy = "first"
	}
	if out.ChecksumPolicy == "" {
		out.ChecksumPolicy = "warn"
	}
	if out.TargetPorts == nil {
		out.TargetPorts = []int{}
	}
	return out
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// captureRoots lists directories where "captures/..." paths are resolved: the
// base, else the directory of the examples file and its parents. The directory
// is made absolute first: for "examples.x.json" run from the report folder it
// is ".", whose parents would otherwise never be visited.
func captureRoots(base, examplesDir string) []string {
	if base != "" {
		return []string{base}
	}
	if abs, err := filepath.Abs(examplesDir); err == nil {
		examplesDir = abs
	}
	roots := []string{examplesDir}
	d := examplesDir
	for range 4 {
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		roots = append(roots, parent)
		d = parent
	}
	return roots
}

func findCapture(roots []string, rel string) (string, bool) {
	if filepath.IsAbs(rel) {
		_, err := os.Stat(rel)
		return rel, err == nil
	}
	for _, root := range roots {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p, true
		}
	}
	return "", false
}

func sessionByRef(c *app.OfflineCapture, ref report.SessionRef) (engine.SessionInput, bool) {
	for _, in := range c.Sessions {
		s := in.Session
		if s.Index == ref.TCPStream && addrString(s.Client) == ref.Client && addrString(s.Server) == ref.Server {
			return in, true
		}
	}
	return engine.SessionInput{}, false
}

func compareSummaries(want, got model.RunSummary) []string {
	var out []string
	check := func(name string, w, g int) {
		if w != g {
			out = append(out, fmt.Sprintf("%s: ожидалось %d, получено %d", name, w, g))
		}
	}
	check("сессий", want.Sessions, got.Sessions)
	check("сессий с ошибками", want.SessionsWithErrors, got.SessionsWithErrors)
	check("сообщений", want.Messages, got.Messages)
	check("разобрано", want.OK, got.OK)
	check("нарушений", want.Violations, got.Violations)
	check("неполных", want.Partial, got.Partial)
	check("неоднозначных", want.Ambiguous, got.Ambiguous)
	check("неизвестный тип", want.UnknownType, got.UnknownType)
	check("неописанных байтов", want.UnknownBytes, got.UnknownBytes)
	check("байтов в сообщениях", want.TotalBytes, got.TotalBytes)
	w, g := slices.Clone(want.InapplicableRules), slices.Clone(got.InapplicableRules)
	slices.Sort(w)
	slices.Sort(g)
	if !slices.Equal(w, g) {
		out = append(out, fmt.Sprintf("неприменимые правила: ожидалось %v, получено %v", w, g))
	}
	gotRules := map[string]model.RuleStat{}
	for _, r := range got.ByRule {
		gotRules[r.RuleID] = r
	}
	for _, r := range want.ByRule {
		g := gotRules[r.RuleID]
		check("правило "+r.RuleID+": сообщений", r.Matched, g.Matched)
		check("правило "+r.RuleID+": нарушений", r.Violations, g.Violations)
	}
	return out
}
