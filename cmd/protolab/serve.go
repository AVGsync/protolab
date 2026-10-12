package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/mattn/go-isatty"

	"github.com/AVGsync/protolab/internal/app"
	"github.com/AVGsync/protolab/internal/browser"
	"github.com/AVGsync/protolab/internal/logx"
	"github.com/AVGsync/protolab/internal/server"
	"github.com/AVGsync/protolab/internal/tui"
	"github.com/AVGsync/protolab/internal/webui"
)

const defaultAddr = "127.0.0.1:7420"

type serveFlags struct {
	addr      string
	data      string
	noTUI     bool
	open      bool
	demo      bool
	maxCapMB  int64
	debug     bool
	addrGiven bool
}

func parseServeFlags(args []string) (serveFlags, error) {
	var f serveFlags
	fs := flag.NewFlagSet("protolab", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&f.addr, "addr", defaultAddr, "")
	fs.StringVar(&f.data, "data", defaultDataDir(), "")
	fs.BoolVar(&f.noTUI, "no-tui", false, "")
	fs.BoolVar(&f.open, "open", false, "")
	fs.BoolVar(&f.demo, "demo", false, "")
	fs.Int64Var(&f.maxCapMB, "max-capture-mb", 512, "")
	fs.BoolVar(&f.debug, "debug", false, "")
	if err := fs.Parse(args); err != nil {
		return f, err
	}
	if fs.NArg() > 0 {
		return f, fmt.Errorf("неизвестная команда %q (protolab help)", fs.Arg(0))
	}
	fs.Visit(func(fl *flag.Flag) {
		if fl.Name == "addr" {
			f.addrGiven = true
		}
	})
	if f.maxCapMB <= 0 {
		return f, errors.New("--max-capture-mb должен быть положительным")
	}
	return f, nil
}

// listen binds the requested address; the default port falls back to a free one.
func listen(f serveFlags) (net.Listener, error) {
	ln, err := net.Listen("tcp", f.addr)
	if err == nil || f.addrGiven {
		return ln, err
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

func runServe(args []string) int {
	f, err := parseServeFlags(args)
	if err != nil {
		return fail("%v", err)
	}
	interactive := !f.noTUI && isatty.IsTerminal(os.Stdout.Fd()) && isatty.IsTerminal(os.Stdin.Fd())

	level := slog.LevelInfo
	if f.debug {
		level = slog.LevelDebug
	}
	ring := logx.NewRing(500)
	var mirror io.Writer = os.Stderr
	if interactive {
		mirror = nil // the TUI shows the log panel instead
	}
	logger := slog.New(logx.NewHandler(ring, level, mirror))
	slog.SetDefault(logger)

	limits := app.DefaultLimits()
	limits.MaxCaptureBytes = f.maxCapMB << 20
	a, err := app.New(app.Config{
		DataDir: f.data,
		Limits:  limits,
		Build:   app.BuildInfo{Version: version, Commit: commit, Date: date},
		Logger:  logger,
	})
	if err != nil {
		return fail("%v", err)
	}
	defer a.Close()

	ln, err := listen(f)
	if err != nil {
		return fail("не удалось открыть %s: %v", f.addr, err)
	}
	tcp := ln.Addr().(*net.TCPAddr)
	loopback := tcp.IP.IsLoopback()
	host := "127.0.0.1"
	if !loopback && !tcp.IP.IsUnspecified() {
		host = tcp.IP.String()
	}
	url := "http://" + net.JoinHostPort(host, strconv.Itoa(tcp.Port))
	if !loopback {
		logger.Warn("сервер доступен из сети: проверка Host/Origin отключена", "addr", ln.Addr().String())
	}

	srv := &http.Server{
		Handler:           server.New(a, server.Options{Logger: logger, Static: webui.FS(), AllowAnyHost: !loopback}),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	logger.Info("ProtoLab запущен", "url", url, "data", a.DataDir(), "version", version)
	if !webui.Built() {
		logger.Warn("веб-интерфейс не встроен в бинарь: выполните make web и пересоберите")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.Preload()
	if f.demo && len(a.ListProjects()) == 0 {
		go func() {
			if _, err := a.CreateDemoProject(ctx); err != nil {
				logger.Error("демо-проект", "err", err)
			}
		}()
	}
	if f.open {
		if err := browser.Open(url); err != nil {
			logger.Warn("не удалось открыть браузер", "err", err)
		}
	}

	if interactive {
		err = tui.Run(ctx, tui.Options{
			Source:      &snapshotSource{app: a, ring: ring, url: url},
			OpenBrowser: browser.Open,
			CreateDemo: func() error {
				_, err := a.CreateDemoProject(ctx)
				return err
			},
		})
		if errors.Is(err, tui.ErrNotTerminal) {
			interactive = false
		} else if err != nil {
			logger.Error("TUI", "err", err)
		}
	}
	if !interactive {
		fmt.Fprintf(os.Stderr, "\nProtoLab %s — откройте %s (Ctrl+C — выход)\n\n", version, url)
		select {
		case <-ctx.Done():
		case err := <-serveErr:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return fail("HTTP: %v", err)
			}
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return 0
}

// snapshotSource feeds the TUI.
type snapshotSource struct {
	app  *app.App
	ring *logx.Ring
	url  string
}

func (s *snapshotSource) Snapshot() tui.Snapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	snap := tui.Snapshot{
		Version:   version,
		URL:       s.url,
		DataDir:   s.app.DataDir(),
		Jobs:      s.app.Jobs().List(8),
		MemAlloc:  ms.HeapAlloc,
		StartedAt: s.app.StartedAt(),
	}
	for _, p := range s.app.Summaries() {
		snap.Projects = append(snap.Projects, tui.ProjectSummary{ID: p.ID, Name: p.Name, Captures: p.Captures, Sessions: p.Sessions, Runs: p.Runs})
	}
	for _, l := range s.ring.Lines() {
		snap.Logs = append(snap.Logs, tui.LogLine{Time: l.Time, Level: l.Level, Message: l.Message})
	}
	return snap
}
