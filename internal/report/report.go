// Package report renders the artifacts of a research report: a readable
// Markdown report, the machine-readable interpretation, the verification
// results and an examples file with expected results that `protolab verify`
// replays on the original captures.
package report

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/AVGsync/protolab/internal/model"
	"github.com/AVGsync/protolab/internal/spec"
)

// ExamplesFormat identifies the examples file.
const ExamplesFormat = "protolab-examples/v1"

// SessionRef identifies a session inside a capture file in a way that does not
// depend on ProtoLab ids: the capture is found by path and SHA-256, the
// session by its ordinal (Wireshark tcp.stream) and endpoints.
type SessionRef struct {
	Capture   string              `json:"capture"` // path relative to the project root, e.g. captures/<sha>.pcapng
	SHA256    string              `json:"sha256"`
	TCPStream int                 `json:"tcpStream"`
	Client    string              `json:"client"`
	Server    string              `json:"server"`
	Actions   []model.ActionEntry `json:"actions,omitempty"` // experiment conditions used by assertions
}

type CorpusExample struct {
	Name     string           `json:"name"`
	Run      string           `json:"run"`
	Sessions []SessionRef     `json:"sessions"`
	Expected model.RunSummary `json:"expected"`
}

// Examples is the examples file format.
type Examples struct {
	Format   string          `json:"format"`
	Spec     string          `json:"spec"` // file name of the interpretation next to this file
	SpecHash string          `json:"specHash"`
	Engine   string          `json:"engine"`
	Settings ExampleSettings `json:"settings"`
	Corpora  []CorpusExample `json:"corpora"`
}

type ExampleSettings struct {
	OverlapPolicy string `json:"overlapPolicy"`
	TargetPorts   []int  `json:"targetPorts"`
}

type RunData struct {
	Run      model.VerificationRun
	Findings []model.Finding
	Corpus   model.Corpus
	Sessions []SessionRef
}

type Input struct {
	Report         model.Report
	Project        model.Project
	Interpretation model.Interpretation
	Version        model.InterpretationVersion
	Runs           []RunData
	Hypotheses     []model.Hypothesis
	Observations   []model.Observation
}

// File is one rendered artifact.
type File struct {
	Kind string // report_md | interpretation | results | examples
	Name string
	Data []byte
}

// Names returns the artifact file names for an interpretation version.
func Names(interpretation string, version int) (md, specFile, results, examples string) {
	base := Slug(interpretation) + ".v" + fmt.Sprint(version)
	return "report." + base + ".md", "interpretation." + base + ".json", "results." + base + ".json", "examples." + base + ".json"
}

// ReproduceCommand tells how to re-apply the interpretation without the UI.
func ReproduceCommand(interpretation string, version int) string {
	_, specFile, _, examples := Names(interpretation, version)
	return strings.Join([]string{
		"protolab apply --spec " + specFile + " --input <capture.pcapng> --out results.json",
		"protolab verify --examples " + examples,
	}, "\n")
}

// Render produces all artifacts.
func Render(in Input) ([]File, error) {
	mdName, specName, resultsName, examplesName := Names(in.Interpretation.Name, in.Version.Version)

	specJSON, err := SpecFile(in.Version, in.Interpretation.Name)
	if err != nil {
		return nil, err
	}

	type runResult struct {
		Run      model.VerificationRun `json:"run"`
		Corpus   model.Corpus          `json:"corpus"`
		Findings []model.Finding       `json:"findings"`
	}
	results := make([]runResult, len(in.Runs))
	for i, r := range in.Runs {
		results[i] = runResult{Run: r.Run, Corpus: r.Corpus, Findings: r.Findings}
	}
	resultsJSON, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return nil, err
	}

	ex := Examples{
		Format:   ExamplesFormat,
		Spec:     specName,
		SpecHash: in.Version.Hash,
		Settings: ExampleSettings{OverlapPolicy: in.Project.Settings.OverlapPolicy, TargetPorts: in.Project.Settings.TargetPorts},
		Corpora:  []CorpusExample{},
	}
	for _, r := range in.Runs {
		if r.Run.Summary == nil {
			continue
		}
		ex.Engine = r.Run.EngineVersion
		ex.Corpora = append(ex.Corpora, CorpusExample{Name: r.Corpus.Name, Run: r.Run.ID, Sessions: r.Sessions, Expected: *r.Run.Summary})
	}
	examplesJSON, err := json.MarshalIndent(ex, "", "  ")
	if err != nil {
		return nil, err
	}

	return []File{
		{Kind: "report_md", Name: mdName, Data: []byte(Markdown(in))},
		{Kind: "interpretation", Name: specName, Data: specJSON},
		{Kind: "results", Name: resultsName, Data: resultsJSON},
		{Kind: "examples", Name: examplesName, Data: examplesJSON},
	}, nil
}

// SpecFile renders the spec with a "$meta" object describing where it came from.
func SpecFile(v model.InterpretationVersion, name string) ([]byte, error) {
	raw, err := json.Marshal(v.Spec)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	meta, err := json.Marshal(map[string]any{"name": name, "version": v.Version, "hash": v.Hash, "createdAt": v.CreatedAt})
	if err != nil {
		return nil, err
	}
	obj["$meta"] = meta
	return json.MarshalIndent(obj, "", "  ")
}

// StripMeta removes the "$meta" object added by SpecFile so the strict decoder accepts the file.
func StripMeta(raw []byte) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if _, ok := obj["$meta"]; !ok {
		return raw, nil
	}
	delete(obj, "$meta")
	return json.Marshal(obj)
}

// Slug keeps ASCII letters and digits, joined by dashes.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimSuffix(b.String(), "-")
	if out == "" {
		return "spec"
	}
	return out
}

var statusLabel = map[string]string{
	"proposed":  "предложена",
	"testing":   "проверяется",
	"supported": "подтверждается",
	"refuted":   "опровергнута",
}

var meaningLabel = map[string]string{
	spec.MeaningStructural: "структура",
	spec.MeaningHypothesis: "гипотеза",
	spec.MeaningConfirmed:  "подтверждено",
}

// Markdown renders the readable report.
func Markdown(in Input) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	v := in.Version
	sp := v.Spec

	w("# %s\n\n", in.Report.Title)
	w("Проект: **%s**. Интерпретация: **%s v%d** (SHA-256 `%s`). Сформировано %s.\n\n", in.Project.Name, in.Interpretation.Name, v.Version, v.Hash, in.Report.CreatedAt)

	w("## Описание протокола\n\n")
	if sp.Transport != nil && sp.Transport.Port != 0 {
		w("Транспорт: TCP, порт устройства %d.\n\n", sp.Transport.Port)
	}
	w("Кадрирование: %s.\n\n", describeFraming(sp.Framing))
	for _, m := range sp.Messages {
		title := m.ID
		if m.Title != "" {
			title += " — " + m.Title
		}
		w("### %s\n\n", title)
		conds := make([]string, len(m.Match))
		for i, c := range m.Match {
			conds[i] = fmt.Sprintf("@%d = %s", c.Offset, c.Hex)
		}
		dir := m.Direction
		if dir == "" {
			dir = "any"
		}
		w("Направление: %s. Признак: %s.\n\n", dir, strings.Join(conds, " и "))
		w("| Поле | Смещение | Тип | Назначение | Основание |\n|---|---|---|---|---|\n")
		writeFields(&b, m.Fields, "")
		b.WriteString("\n")
	}
	if len(sp.Assertions) > 0 {
		w("### Проверки по журналу действий\n\n")
		for _, a := range sp.Assertions {
			checks := make([]string, len(a.Checks))
			for i, c := range a.Checks {
				checks[i] = c.Field + " = " + c.Equals
			}
			w("- `%s`: сообщения %s в сессиях действий «%s» — %s.\n", a.ID, a.Rule, a.ActionKind, strings.Join(checks, ", "))
		}
		b.WriteString("\n")
	}

	w("## Проверенные примеры и область применимости\n\n")
	if len(in.Runs) == 0 {
		w("Проверки не выбраны: выводы отчёта не подтверждены прогоном на корпусе.\n\n")
	}
	for _, r := range in.Runs {
		s := r.Run.Summary
		if s == nil {
			continue
		}
		w("### Проверка %s — корпус «%s»\n\n", r.Run.ID, r.Corpus.Name)
		w("Корпус: %d сессий, SHA-256 корпуса `%s`. Движок %s, перекрытия: %s.\n\n", len(r.Corpus.SessionIDs), r.Corpus.Hash, r.Run.EngineVersion, r.Run.Settings.OverlapPolicy)
		w("| Сообщений | Разобрано | Нарушения | Неполные | Неоднозначные | Неизвестный тип | Неописанные байты |\n|---|---|---|---|---|---|---|\n")
		w("| %d | %d | %d | %d | %d | %d | %d из %d |\n\n", s.Messages, s.OK, s.Violations, s.Partial, s.Ambiguous, s.UnknownType, s.UnknownBytes, s.TotalBytes)
		if len(s.InapplicableRules) > 0 {
			w("Правила, не сработавшие на корпусе (не проверены): %s.\n\n", strings.Join(s.InapplicableRules, ", "))
		}
		if r.Run.Stale {
			w("> Результат устарел: %s.\n\n", strings.Join(r.Run.StaleReasons, "; "))
		}
		if len(r.Findings) > 0 {
			w("Находки:\n\n")
			for _, f := range r.Findings {
				ex := ""
				if len(f.Examples) > 0 {
					e := f.Examples[0]
					ex = fmt.Sprintf(" (пример: %s/%s @%d, пакеты #%s)", e.SessionID, e.Direction, e.Offset, joinInts(e.PacketNumbers))
				}
				w("- [%s] %s — %d случ.%s\n", f.Kind, f.Message, f.Occurrences, ex)
			}
			b.WriteString("\n")
		}
	}
	w("> Совпадение на выбранных примерах не доказывает назначение полей и универсальность модели: см. основания и контрпримеры гипотез.\n\n")

	w("## Гипотезы: основания и контрпримеры\n\n")
	obsByID := map[string]model.Observation{}
	for _, o := range in.Observations {
		obsByID[o.ID] = o
	}
	hyps := append([]model.Hypothesis(nil), in.Hypotheses...)
	sort.SliceStable(hyps, func(i, j int) bool { return hyps[i].CreatedAt < hyps[j].CreatedAt })
	for _, h := range hyps {
		w("### %s [%s]\n\n%s\n\n", h.Title, statusLabel[h.Status], h.Statement)
		for _, id := range h.ObservationIDs {
			if o, ok := obsByID[id]; ok {
				w("- основание: %s — `%s` (%s/%s @%d)\n", o.Note, o.SnapshotHex, o.Source.SessionID, o.Source.Direction, o.Source.Offset)
			}
		}
		for _, c := range h.Counterexamples {
			w("- контрпример: %s (%s/%s @%d)\n", c.Note, c.Source.SessionID, c.Source.Direction, c.Source.Offset)
		}
		for _, alt := range h.Alternatives {
			w("- альтернатива: %s\n", alt)
		}
		if len(h.History) > 0 {
			last := h.History[len(h.History)-1]
			w("- последнее изменение статуса: %s — %s\n", last.At, last.Note)
		}
		b.WriteString("\n")
	}

	w("## Открытые вопросы\n\n")
	open := 0
	for _, h := range hyps {
		if h.Status == "proposed" || h.Status == "testing" {
			w("- %s\n", h.Title)
			open++
		}
	}
	for _, m := range sp.Messages {
		walkFields(m.Fields, func(f spec.FieldRule) {
			if f.Status == spec.MeaningStructural && f.Meaning != "" {
				w("- %s.%s: структура описана, назначение не подтверждено\n", m.ID, f.Name)
				open++
			}
		})
	}
	if open == 0 {
		w("Нет.\n")
	}

	w("\n## Повторное применение\n\n```\n%s\n```\n", ReproduceCommand(in.Interpretation.Name, v.Version))
	w("\nФайл примеров содержит ссылки на исходные записи (по SHA-256 и номеру TCP-сессии) и ожидаемые сводки; `protolab verify` пересчитывает их и сообщает о расхождениях.\n")
	return b.String()
}

func describeFraming(f spec.Framing) string {
	switch f.Kind {
	case spec.FramingLengthPrefixed:
		if f.LengthField == nil {
			return "по полю длины"
		}
		adjust := 0
		if f.LengthAdjust != nil {
			adjust = *f.LengthAdjust
		}
		s := fmt.Sprintf("длина u%d %s по смещению %d, полная длина = значение + %d", f.LengthField.Size*8, strings.ToUpper(f.LengthField.Endian), f.LengthField.Offset, adjust)
		if f.Sync != "" {
			s += ", ресинхронизация по `" + f.Sync + "`"
		}
		return s
	case spec.FramingDelimiter:
		return "разделитель `" + f.Delimiter + "`"
	case spec.FramingFixed:
		return fmt.Sprintf("фиксированный размер %d Б", f.Size)
	}
	return f.Kind
}

func writeFields(b *strings.Builder, fields []spec.FieldRule, prefix string) {
	for _, f := range fields {
		typ := f.Type
		if f.Endian != "" && f.Type != "u8" && f.Type != "i8" {
			typ += " " + f.Endian
		}
		if f.Scale != 0 {
			typ += fmt.Sprintf(" ×%g", f.Scale)
		}
		meaning := f.Meaning
		if meaning == "" {
			meaning = "—"
		}
		if f.Checksum != "" {
			meaning += " [" + f.Checksum + "]"
		}
		basis := meaningLabel[f.Status]
		if basis == "" {
			basis = meaningLabel[spec.MeaningStructural]
		}
		if f.Hypothesis != "" {
			basis += " (" + f.Hypothesis + ")"
		}
		fmt.Fprintf(b, "| %s%s | %d | %s | %s | %s |\n", prefix, f.Name, f.Offset, typ, meaning, basis)
		if len(f.Items) > 0 {
			writeFields(b, f.Items, prefix+f.Name+"[].")
		}
	}
}

func walkFields(fields []spec.FieldRule, fn func(spec.FieldRule)) {
	for _, f := range fields {
		fn(f)
		walkFields(f.Items, fn)
	}
}

func joinInts(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return strings.Join(parts, ", #")
}
