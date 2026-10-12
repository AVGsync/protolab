package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/AVGsync/protolab/internal/model"
)

type fakeSrc struct{ s Snapshot }

func (f fakeSrc) Snapshot() Snapshot { return f.s }

func sample(jobs ...model.Job) Snapshot {
	var logs []LogLine
	for i := 0; i < 80; i++ {
		logs = append(logs, LogLine{Time: time.Now(), Level: "INFO", Message: fmt.Sprintf("запрос %d \x1b[31mкрасный\x1b[0m 日本語 😀", i)})
	}
	var ps []ProjectSummary
	for i := 0; i < 9; i++ {
		ps = append(ps, ProjectSummary{ID: fmt.Sprint(i), Name: fmt.Sprintf("Проект с очень длинным названием номер %d", i), Captures: i, Sessions: i * 10, Runs: i})
	}
	return Snapshot{Version: "v0.1.0", URL: "http://127.0.0.1:7420", DataDir: "/data", Projects: ps, Jobs: jobs, Logs: logs, StartedAt: time.Now().Add(-time.Hour)}
}

var jobs = []model.Job{
	{ID: "1", Kind: "verification", Title: "Проверка спецификации", Status: "running", Progress: 0.4, Stage: "сессия 4 из 10"},
	{ID: "2", Kind: "import_capture", Title: "Импорт a.pcap", Status: "failed", Error: "битый заголовок"},
	{ID: "3", Kind: "report", Status: "queued"},
	{ID: "4", Kind: "import_capture", Title: "Импорт b.pcap", Status: "succeeded", StartedAt: "2026-01-01T10:00:00Z", FinishedAt: "2026-01-01T10:00:03Z"},
}

// The frame must never be taller or wider than the terminal, at any size, in any mode.
func TestViewFitsTerminal(t *testing.T) {
	for _, snap := range []Snapshot{sample(jobs...), sample(), {URL: "http://127.0.0.1:1", Version: "dev"}} {
		for w := 20; w <= 220; w += 23 {
			for h := 4; h <= 60; h += 9 {
				for _, mode := range []string{"main", "log", "help"} {
					u := newUI(Options{Source: fakeSrc{snap}, OpenBrowser: func(string) error { return nil }})
					var m tea.Model = u
					m, _ = m.Update(tea.WindowSizeMsg{Width: w, Height: h})
					switch mode {
					case "log":
						m, _ = m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
					case "help":
						m, _ = m.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
					}
					lines := strings.Split(m.View().Content, "\n")
					if len(lines) > h {
						t.Fatalf("%dx%d %s: %d lines > height", w, h, mode, len(lines))
					}
					for i, l := range lines {
						if got := ansi.StringWidth(l); got > w {
							t.Fatalf("%dx%d %s: line %d is %d cells wide: %q", w, h, mode, i, got, ansi.Strip(l))
						}
						if strings.ContainsRune(ansi.Strip(l), 0x1b) {
							t.Fatalf("escape leaked into %q", l)
						}
					}
				}
			}
		}
	}
}

func TestKeys(t *testing.T) {
	opened := ""
	u := newUI(Options{Source: fakeSrc{sample(jobs...)}, OpenBrowser: func(u string) error { opened = u; return nil }})
	var m tea.Model = u
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if msg := cmd(); msg.(openedMsg).err != nil || opened != "http://127.0.0.1:7420" {
		t.Fatalf("enter must open the URL, got %q", opened)
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}) // a job is running: confirm first
	if cmd != nil || !m.(ui).confirmQuit {
		t.Fatal("q with running jobs must ask for confirmation")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}) // second Ctrl+C quits at once
	if cmd == nil {
		t.Fatal("ctrl+c in confirmation must quit")
	}
	idle := newUI(Options{Source: fakeSrc{sample()}})
	_, cmd = tea.Model(idle).Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q without jobs must quit immediately")
	}
}
