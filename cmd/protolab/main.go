// Command protolab is the ProtoLab binary: a local server with a terminal UI
// and a browser UI for researching undocumented binary protocols over TCP.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Set by the linker: -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

const usage = `ProtoLab — лаборатория протоколов.

Использование:
  protolab [флаги]                  запустить сервер и терминальный интерфейс
  protolab demo [--data DIR]        создать демо-проект ThermoLink
  protolab apply --spec F --input F применить интерпретацию к записи без интерфейса
  protolab verify --examples F      повторить проверку из отчёта и сравнить с ожидаемым
  protolab version                  версия программы

Флаги сервера:
  --addr ADDR        адрес HTTP (по умолчанию 127.0.0.1:7420; если порт занят — свободный)
  --data DIR         каталог данных (по умолчанию %s)
  --no-tui           без терминального интерфейса, журнал в stderr
  --open             сразу открыть браузер
  --demo             создать демо-проект, если проектов нет
  --max-capture-mb N предел размера записи (по умолчанию 512)
  --debug            подробный журнал

В терминальном интерфейсе: o / Enter — открыть веб-интерфейс, c — копировать адрес,
d — демо-проект, l — журнал, ? — справка, q — выход.
`

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			fmt.Printf("protolab %s (commit %s, built %s, %s/%s)\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
			return
		case "help", "-h", "--help":
			fmt.Printf(usage, defaultDataDir())
			return
		case "demo":
			os.Exit(runDemo(args[1:]))
		case "apply":
			os.Exit(runApply(args[1:]))
		case "verify":
			os.Exit(runVerify(args[1:]))
		}
	}
	os.Exit(runServe(args))
}

// defaultDataDir follows platform conventions; PROTOLAB_DATA overrides it.
func defaultDataDir() string {
	if d := os.Getenv("PROTOLAB_DATA"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			return filepath.Join(d, "ProtoLab")
		}
	case "darwin":
		if home != "" {
			return filepath.Join(home, "Library", "Application Support", "ProtoLab")
		}
	default:
		if d := os.Getenv("XDG_DATA_HOME"); d != "" {
			return filepath.Join(d, "protolab")
		}
		if home != "" {
			return filepath.Join(home, ".local", "share", "protolab")
		}
	}
	return "protolab-data"
}

func fail(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "protolab: "+format+"\n", args...)
	return 1
}
