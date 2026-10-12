package tui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"
	"unicode"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/AVGsync/protolab/internal/model"
)

// Run shows the TUI until the user quits or ctx is cancelled. It returns
// ErrNotTerminal, without touching the terminal, when there is no real one.
func Run(ctx context.Context, opt Options) error {
	if !isTTY() {
		return ErrNotTerminal
	}
	p := tea.NewProgram(newUI(opt),
		tea.WithContext(ctx),
		tea.WithoutSignalHandler(), // main owns SIGINT/SIGTERM via signal.NotifyContext
	)
	_, err := p.Run()
	if errors.Is(err, tea.ErrInterrupted) || (ctx.Err() != nil && errors.Is(err, tea.ErrProgramKilled)) {
		return nil
	}
	return err
}

func isTTY() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd()) && os.Getenv("TERM") != "dumb"
}

// ---- messages -------------------------------------------------------------

type (
	snapMsg     Snapshot
	openedMsg   struct{ err error }
	toastEndMsg struct{ id int }
	demoDoneMsg struct{ err error }
)

// ---- key bindings ---------------------------------------------------------

type keyMap struct {
	Open, Copy, Log, Demo, Help, Quit      key.Binding
	Up, Down, PgUp, PgDn, Top, Bottom, Esc key.Binding
}

func newKeys(demo bool) keyMap {
	k := keyMap{
		Open:   key.NewBinding(key.WithKeys("enter", "o"), key.WithHelp("enter", "открыть")),
		Copy:   key.NewBinding(key.WithKeys("c", "y"), key.WithHelp("c", "копировать адрес")),
		Log:    key.NewBinding(key.WithKeys("l", "tab"), key.WithHelp("l", "журнал")),
		Demo:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "демо-проект")),
		Help:   key.NewBinding(key.WithKeys("?", "f1"), key.WithHelp("?", "справка")),
		Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "выход")),
		Up:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "вверх")),
		Down:   key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "вниз")),
		PgUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("PgUp", "страница вверх")),
		PgDn:   key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("PgDn", "страница вниз")),
		Top:    key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "в начало")),
		Bottom: key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "в конец")),
		Esc:    key.NewBinding(key.WithKeys("esc")),
	}
	k.Demo.SetEnabled(demo)
	return k
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Open, k.Copy, k.Log, k.Demo, k.Help, k.Quit}
}
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{{k.Open, k.Copy, k.Demo}, {k.Log, k.Up, k.Down, k.PgUp, k.PgDn, k.Top, k.Bottom}, {k.Help, k.Quit}}
}

// ---- styles ---------------------------------------------------------------

type styles struct {
	dim, accent, keyChip, ok, warn, bad, bold lipgloss.Style
	crimson, orange                           color.Color
}

// theme: 0 = unknown (terminal has not answered OSC 11 yet, or never will), 1 = dark, 2 = light.
func newStyles(theme int) styles {
	accent := lipgloss.Color("#CC4A0A") // 4.6:1 on white, 4.5:1 on black: readable without detection
	switch theme {
	case 1:
		accent = lipgloss.Color("#FF8A3D")
	case 2:
		accent = lipgloss.Color("#B93A00")
	}
	s := styles{
		crimson: lipgloss.Color("#D7263D"), orange: lipgloss.Color("#FF8A1F"),
		dim:     lipgloss.NewStyle().Faint(true), // follows the terminal theme
		bold:    lipgloss.NewStyle().Bold(true),
		accent:  lipgloss.NewStyle().Foreground(accent),
		keyChip: lipgloss.NewStyle().Foreground(accent).Bold(true),
		ok:      lipgloss.NewStyle().Foreground(lipgloss.Green),
		warn:    lipgloss.NewStyle().Foreground(lipgloss.Yellow),
		bad:     lipgloss.NewStyle().Foreground(lipgloss.Red),
	}
	return s
}

// ---- ui model -------------------------------------------------------------

type ui struct {
	opt  Options
	snap Snapshot
	now  time.Time
	w, h int

	st    styles
	keys  keyMap
	help  help.Model
	bar   progress.Model
	vp    viewport.Model
	nLogs int // len(logs) rendered into vp last time

	logOpen, helpOpen, confirmQuit bool
	toast                          string
	toastID                        int
}

func newUI(opt Options) ui {
	m := ui{opt: opt, now: time.Now(), keys: newKeys(opt.CreateDemo != nil)}
	m.help = help.New()
	m.help.ShortSeparator = "  "
	m.restyle(0)
	m.bar = progress.New(progress.WithoutPercentage(), progress.WithColors(m.st.crimson, m.st.orange))
	if os.Getenv("PROTOLAB_ASCII") != "" {
		m.bar = progress.New(progress.WithoutPercentage(), progress.WithFillCharacters('#', '-'))
	}
	m.vp = viewport.New()
	m.snap = opt.Source.Snapshot() // first frame is never empty
	return m
}

// restyle rebuilds every theme-dependent style. The bubbles defaults for help
// (#4A4A4A on dark) are too low-contrast to read, so we replace them.
func (m *ui) restyle(theme int) {
	m.st = newStyles(theme)
	m.help.Styles.ShortKey, m.help.Styles.ShortDesc = m.st.keyChip, m.st.dim
	m.help.Styles.ShortSeparator, m.help.Styles.Ellipsis = m.st.dim, m.st.dim
	m.help.Styles.FullKey, m.help.Styles.FullDesc, m.help.Styles.FullSeparator = m.st.keyChip, m.st.dim, m.st.dim
}

// pollEvery: 250 ms while a job runs (smooth progress), 1 s otherwise (uptime clock only).
func (m ui) pollEvery() time.Duration {
	if n, _ := m.running(); n > 0 {
		return 250 * time.Millisecond
	}
	return time.Second
}

func poll(src Source, after time.Duration) tea.Cmd {
	// The closure runs in the command goroutine, not in Update: Snapshot() may take locks.
	return tea.Tick(after, func(time.Time) tea.Msg { return snapMsg(src.Snapshot()) })
}

func (m ui) Init() tea.Cmd {
	return tea.Batch(poll(m.opt.Source, 250*time.Millisecond), tea.RequestBackgroundColor)
}

func (m ui) running() (n int, avg float64) {
	for _, j := range m.snap.Jobs {
		if j.Status == "running" || j.Status == "queued" {
			n++
			avg += j.Progress
		}
	}
	if n > 0 {
		avg /= float64(n)
	}
	return
}

func (m ui) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.relayout()
	case tea.BackgroundColorMsg:
		if msg.IsDark() {
			m.restyle(1)
		} else {
			m.restyle(2)
		}
	case snapMsg:
		m.snap, m.now = Snapshot(msg), time.Now()
		m.syncLog()
		return m, poll(m.opt.Source, m.pollEvery())
	case openedMsg:
		if msg.err != nil {
			return m.flash("Браузер не открылся ("+shortErr(msg.err)+"). Адрес скопирован в буфер.", tea.SetClipboard(m.snap.URL))
		}
		return m.flash("Открываю " + m.snap.URL)
	case demoDoneMsg:
		if msg.err != nil {
			return m.flash("Демо-проект: " + shortErr(msg.err))
		}
		return m.flash("Демо-проект создан")
	case toastEndMsg:
		if msg.id == m.toastID {
			m.toast = ""
		}
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

func (m ui) flash(text string, extra ...tea.Cmd) (tea.Model, tea.Cmd) {
	m.toast = text
	m.toastID++
	id := m.toastID
	cmds := append(extra, tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastEndMsg{id} }))
	return m, tea.Batch(cmds...)
}

func (m ui) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmQuit { // y / enter / second Ctrl+C = quit, anything else = stay
		if s := msg.String(); s == "y" || s == "Y" || s == "enter" || s == "ctrl+c" {
			return m, tea.Quit
		}
		m.confirmQuit = false
		return m, nil
	}
	if m.helpOpen { // any key closes help, except quit
		m.helpOpen = false
		if !key.Matches(msg, m.keys.Quit) {
			return m, nil
		}
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		if n, _ := m.running(); n > 0 {
			m.confirmQuit = true
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Open):
		return m, func() tea.Msg { return openedMsg{m.opt.OpenBrowser(m.snap.URL)} }
	case key.Matches(msg, m.keys.Copy):
		return m.flash("Адрес скопирован: "+m.snap.URL, tea.SetClipboard(m.snap.URL))
	case key.Matches(msg, m.keys.Demo) && m.opt.CreateDemo != nil:
		return m, func() tea.Msg { return demoDoneMsg{m.opt.CreateDemo()} }
	case key.Matches(msg, m.keys.Log):
		m.logOpen = !m.logOpen
		m.relayout()
	case key.Matches(msg, m.keys.Help):
		m.helpOpen = true
	case m.logOpen: // do NOT forward to viewport.Update: its default keys (l, d, f, b...) clash with ours
		switch {
		case key.Matches(msg, m.keys.Up):
			m.vp.ScrollUp(1)
		case key.Matches(msg, m.keys.Down):
			m.vp.ScrollDown(1)
		case key.Matches(msg, m.keys.PgUp):
			m.vp.PageUp()
		case key.Matches(msg, m.keys.PgDn):
			m.vp.PageDown()
		case key.Matches(msg, m.keys.Top):
			m.vp.GotoTop()
		case key.Matches(msg, m.keys.Bottom):
			m.vp.GotoBottom()
		case key.Matches(msg, m.keys.Esc):
			m.logOpen = false
			m.relayout()
		}
	}
	return m, nil
}

// relayout resizes the log viewport to the space the current mode gives it.
func (m *ui) relayout() {
	m.vp.SetWidth(max(m.w, 1))
	m.vp.SetHeight(max(m.h-6, 1)) // header(4) + title(1) + footer(1)
	m.nLogs = -1
	m.syncLog()
}

func (m *ui) syncLog() {
	if m.nLogs == len(m.snap.Logs) && m.nLogs >= 0 {
		return
	}
	atBottom := m.vp.AtBottom()
	lines := make([]string, len(m.snap.Logs))
	for i, l := range m.snap.Logs {
		lines[i] = m.logLine(l, max(m.w, 1))
	}
	m.vp.SetContentLines(lines)
	if atBottom { // tail -f behaviour until the user scrolls up
		m.vp.GotoBottom()
	}
	m.nLogs = len(m.snap.Logs)
}

// ---- view -----------------------------------------------------------------

func (m ui) View() tea.View {
	var body string
	switch {
	case m.w == 0:
		body = ""
	case m.w < 52 || m.h < 10:
		body = m.viewTiny()
	case m.helpOpen:
		body = m.viewHelp()
	case m.w >= 112 && m.h >= 16 && !m.logOpen:
		body = m.viewWide()
	default:
		body = m.viewStacked()
	}
	v := tea.NewView(clamp(body, m.w, m.h))
	v.AltScreen = true
	v.WindowTitle = "ProtoLab · " + strings.TrimPrefix(m.snap.URL, "http://")
	if n, avg := m.running(); n > 0 { // taskbar/tab progress in Windows Terminal, iTerm2, kitty, ConEmu
		v.ProgressBar = tea.NewProgressBar(tea.ProgressBarDefault, int(avg*100))
	}
	return v
}

func (m ui) viewTiny() string {
	return strings.Join([]string{
		fit(m.snap.URL, m.w),
		fit(m.st.dim.Render("enter — открыть   q — выход"), m.w),
	}, "\n")
}

func (m ui) viewHelp() string {
	w := m.w
	lines := []string{m.brand(w), ""}
	lines = append(lines, strings.Split(m.help.FullHelpView(m.keys.FullHelp()), "\n")...)
	lines = append(lines, "",
		m.st.dim.Render("Сервер слушает только 127.0.0.1 и снаружи недоступен."),
		m.st.dim.Render("Выход останавливает сервер и отменяет задачи."),
		"", m.st.dim.Render("Любая клавиша — закрыть справку"))
	return strings.Join(lines, "\n")
}

// viewStacked is the 80x24 layout: everything in one column.
func (m ui) viewStacked() string {
	w := m.w
	head := m.headerLines(w)
	foot := m.footer(w)
	if m.logOpen {
		out := append(head, m.section("ЖУРНАЛ", m.jobSummary(), w))
		out = append(out, strings.Split(m.vp.View(), "\n")...)
		return strings.Join(append(out, foot), "\n")
	}
	budget := m.h - len(head) - 1 /*footer*/ - 3 /*section titles*/ - 3 /*blank rows*/
	np, nj := min(max(len(m.snap.Projects), 1), 5), min(max(len(m.snap.Jobs), 1), 4)
	for np+nj > budget-4 && (np > 1 || nj > 1) { // keep >= 4 log rows
		if np >= nj && np > 1 {
			np--
		} else if nj > 1 {
			nj--
		}
	}
	nl := max(budget-np-nj, 1)
	out := append([]string(nil), head...)
	out = append(out, "", m.projectHeader(w))
	out = append(out, m.projectRows(w, np)...)
	out = append(out, "", m.section("ЗАДАЧИ", "", w))
	out = append(out, m.jobRows(w, nj)...)
	out = append(out, "", m.section("ЖУРНАЛ", m.st.dim.Render("l — развернуть"), w))
	out = append(out, m.tailLog(w, nl)...)
	return strings.Join(append(out, foot), "\n")
}

// viewWide (>=112 cols): projects and jobs side by side, the log below at full width.
func (m ui) viewWide() string {
	w := m.w
	lw := 46
	rw := w - lw - 3
	head := m.headerLines(w)
	foot := m.footer(w)
	n := min(max(max(len(m.snap.Projects), len(m.snap.Jobs)), 1), 6)
	n = min(n, max(m.h-len(head)-1-2-3-4, 1)) // keep >= 4 log rows
	left := append([]string{m.projectHeader(lw)}, m.projectRows(lw, n)...)
	right := append([]string{m.section("ЗАДАЧИ", "", rw)}, m.jobRows(rw, n)...)
	sep := m.st.dim.Render(" │ ")
	out := append([]string(nil), head...)
	out = append(out, "")
	for i := 0; i <= n; i++ {
		l, r := strings.Repeat(" ", lw), ""
		if i < len(left) {
			l = fit(left[i], lw)
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, l+sep+r)
	}
	out = append(out, "", m.section("ЖУРНАЛ", "", w))
	out = append(out, m.tailLog(w, m.h-len(out)-1)...)
	return strings.Join(append(out, foot), "\n")
}

func (m ui) headerLines(w int) []string {
	s := m.snap
	up := m.now.Sub(s.StartedAt).Truncate(time.Second)
	top := row(m.brand(w), m.st.dim.Render(fmt.Sprintf("%s · работает %s", s.Version, dur(up))), w)
	url := m.st.accent.Hyperlink(s.URL).Underline(true).Render(s.URL) // OSC 8: Ctrl+click in modern terminals
	label := m.st.dim.Render("Веб-интерфейс  ")
	right := m.st.keyChip.Render("enter") + m.st.dim.Render(" открыть в браузере")
	if ansi.StringWidth(label)+ansi.StringWidth(s.URL)+2+ansi.StringWidth(right) > w {
		right = m.st.keyChip.Render("enter") // the URL itself is never truncated
	}
	l1 := row(label+url, right, w)
	l2 := row(m.st.dim.Render("Данные         ")+s.DataDir, m.st.dim.Render(fmt.Sprintf("ОЗУ %d МБ", s.MemAlloc>>20)), w)
	return []string{top, "", l1, l2}
}

func (m ui) brand(w int) string {
	g := lipgloss.Blend1D(len("ProtoLab"), m.st.crimson, m.st.orange)
	var b strings.Builder
	for i, r := range "ProtoLab" {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(g[i]).Render(string(r)))
	}
	return b.String() + m.st.dim.Render("  Лаборатория протоколов")
}

func (m ui) section(title, right string, w int) string {
	return row(m.st.bold.Render(title), right, w)
}

func (m ui) projectHeader(w int) string {
	return row(m.st.bold.Render("ПРОЕКТЫ"), m.st.dim.Render(fmt.Sprintf("%8s %7s %7s", "захваты", "сессии", "прогоны")), w)
}

func (m ui) projectRows(w, n int) []string {
	ps := m.snap.Projects
	if len(ps) == 0 {
		return []string{m.st.dim.Render("Проектов пока нет. Откройте веб-интерфейс или нажмите d (демо).")}
	}
	var out []string
	for i, p := range ps {
		if n > 1 && i == n-1 && len(ps) > n { // last visible row becomes "… и ещё N"
			out = append(out, m.st.dim.Render(fmt.Sprintf("… и ещё %d", len(ps)-i)))
			break
		}
		out = append(out, row(clean(p.Name), fmt.Sprintf("%8d %7d %7d", p.Captures, p.Sessions, p.Runs), w))
		if len(out) == n {
			break
		}
	}
	return out
}

var kindRU = map[string]string{"import_capture": "Импорт захвата", "import_project": "Импорт проекта", "verification": "Проверка", "report": "Отчёт"}

func (m ui) jobRows(w, n int) []string {
	js := m.snap.Jobs
	if len(js) == 0 {
		return []string{m.st.dim.Render("Фоновых задач нет.")}
	}
	titleW := min(max(w*38/100, 18), 36)
	barW := min(max(w/6, 8), 24)
	var out []string
	for _, j := range js[:min(n, len(js))] {
		title := clean(j.Title)
		if title == "" {
			title = kindRU[j.Kind]
		}
		var tail string
		switch j.Status {
		case "queued":
			tail = m.st.dim.Render("в очереди")
		case "running":
			m.bar.SetWidth(barW)
			tail = m.bar.ViewAs(j.Progress) + fmt.Sprintf(" %3d%% ", int(j.Progress*100)) + m.st.dim.Render(clean(j.Stage))
		case "succeeded":
			tail = m.st.ok.Render("готово") + m.st.dim.Render(" "+m.elapsed(j))
		case "failed":
			tail = m.st.bad.Render("ошибка") + m.st.dim.Render(" "+clean(j.Error))
		default:
			tail = m.st.dim.Render("отменено")
		}
		out = append(out, fit(fit(title, titleW)+" "+tail, w))
	}
	return out
}

func (m ui) elapsed(j model.Job) string {
	s, err1 := time.Parse(time.RFC3339, j.StartedAt)
	f, err2 := time.Parse(time.RFC3339, j.FinishedAt)
	if err1 != nil || err2 != nil {
		return ""
	}
	return strings.Replace(fmt.Sprintf("%.1f с", f.Sub(s).Seconds()), ".", ",", 1)
}

func (m ui) jobSummary() string {
	n, _ := m.running()
	if n == 0 {
		return m.st.dim.Render("задач в работе нет")
	}
	return m.st.accent.Render(fmt.Sprintf("задач в работе: %d", n))
}

func (m ui) logLine(l LogLine, w int) string {
	lvl := fmt.Sprintf("%-5s", l.Level)
	switch l.Level {
	case "ERROR":
		lvl = m.st.bad.Render(lvl)
	case "WARN":
		lvl = m.st.warn.Render(lvl)
	default:
		lvl = m.st.dim.Render(lvl)
	}
	return fit(m.st.dim.Render(l.Time.Format("15:04:05"))+" "+lvl+" "+clean(l.Message), w)
}

func (m ui) tailLog(w, n int) []string {
	ls := m.snap.Logs
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	out := make([]string, 0, n)
	for _, l := range ls {
		out = append(out, m.logLine(l, w))
	}
	for len(out) < n { // keep the footer pinned to the bottom row
		out = append(out, "")
	}
	return out
}

func (m ui) footer(w int) string {
	if m.confirmQuit {
		n, _ := m.running()
		return m.st.warn.Render(fmt.Sprintf("Выполняется задач: %d. Выйти и отменить их?", n)) + m.st.dim.Render("  y — да, любая другая клавиша — нет")
	}
	right := m.help.ShortHelpView([]key.Binding{m.keys.Help, m.keys.Quit}) // always visible
	m.help.SetWidth(w - ansi.StringWidth(right) - 2)
	left := m.help.ShortHelpView([]key.Binding{m.keys.Open, m.keys.Copy, m.keys.Log, m.keys.Demo})
	if m.logOpen {
		left = m.help.ShortHelpView([]key.Binding{m.keys.Up, m.keys.PgUp, m.keys.Bottom, m.keys.Log})
	}
	if m.toast != "" { // a toast temporarily replaces the hints, never the pinned "? q"
		left = m.st.accent.Render(m.toast)
	}
	return row(left, right, w)
}

// ---- helpers --------------------------------------------------------------

// clean makes untrusted text (file names from a capture, device strings, log
// arguments) safe to draw: no escape sequences, no control or bidi/format
// characters. Without it a crafted name can recolor, retitle or spoof the UI.
func clean(s string) string {
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
}

// fit truncates (with "…") or pads s to exactly w cells; ANSI-aware.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// row puts l on the left and r on the right of a w-cell line.
func row(l, r string, w int) string {
	rw := ansi.StringWidth(r)
	if rw >= w {
		return fit(r, w)
	}
	gap := w - rw - ansi.StringWidth(l)
	if gap < 1 {
		return fit(l, w-rw-1) + " " + r
	}
	return l + strings.Repeat(" ", gap) + r
}

// clamp is the last line of defence: never emit more than h rows of w cells,
// whatever the layout code computed (a wrapped line would scroll the whole frame).
func clamp(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	return strings.Join(lines, "\n")
}

func dur(d time.Duration) string {
	h, mi, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, mi, s)
	}
	return fmt.Sprintf("%02d:%02d", mi, s)
}

func shortErr(err error) string {
	s := err.Error()
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
