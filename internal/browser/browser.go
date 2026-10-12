// Package browser opens http(s) URLs in the user's default browser without
// touching the terminal and without blocking the caller for long.
package browser

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// ErrNoDisplay means there is no graphical session to open a browser in
// (typically SSH or a container). Callers should show the URL instead.
var ErrNoDisplay = errors.New("нет графической сессии")

type opener struct {
	name       string
	args       []string
	ignoreExit bool // explorer.exe returns 1 even on success
}

// Open starts the platform URL handler and returns once it is clear that the
// handler started (or failed fast). It never writes to the terminal.
func Open(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		strings.ContainsAny(raw, " \t\r\n'\"`$<>|&^") {
		return fmt.Errorf("browser: небезопасный адрес %q", raw)
	}
	cands := candidates(raw)
	if len(cands) == 0 {
		return ErrNoDisplay
	}
	var errs []error
	for _, c := range cands {
		path, err := exec.LookPath(c.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c.args...)
		// nil std streams = os.DevNull: a text-mode "browser" (lynx, w3m) can
		// not draw over the TUI, and xdg-open chatter never reaches the screen.
		detach(cmd)
		if err := cmd.Start(); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.name, err))
			continue
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }() // always reap, never leak a zombie
		select {
		case err := <-done:
			if err != nil && !c.ignoreExit {
				errs = append(errs, fmt.Errorf("%s: %w", c.name, err))
				continue
			}
			return nil
		case <-time.After(1500 * time.Millisecond):
			return nil // handler is still running (browser attached): good enough
		}
	}
	if len(errs) == 0 {
		return fmt.Errorf("browser: не найдена программа для открытия ссылок (xdg-open, wslview, open)")
	}
	return errors.Join(errs...)
}

func candidates(u string) []opener {
	switch runtime.GOOS {
	case "darwin":
		return []opener{{name: "open", args: []string{u}}}
	case "windows":
		return []opener{
			{name: "rundll32", args: []string{"url.dll,FileProtocolHandler", u}},
			{name: "powershell", args: []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Process '" + u + "'"}},
		}
	}
	var c []opener
	if env := os.Getenv("BROWSER"); env != "" && !isWSL() {
		// $BROWSER is a ":"-separated list; "%s" marks the URL position.
		for _, item := range strings.Split(env, ":") {
			f := strings.Fields(item)
			if len(f) == 0 {
				continue
			}
			args, hasURL := f[1:], false
			for i, a := range args {
				if strings.Contains(a, "%s") {
					args[i], hasURL = strings.ReplaceAll(a, "%s", u), true
				}
			}
			if !hasURL {
				args = append(args, u)
			}
			c = append(c, opener{name: f[0], args: args})
		}
	}
	if isWSL() {
		// Prefer the Windows browser: it is what the user normally uses and it
		// reaches 127.0.0.1 of the WSL2 VM through localhost forwarding.
		c = append(c,
			opener{name: "wslview", args: []string{u}},
			opener{name: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", u}},
			opener{name: "explorer.exe", args: []string{u}, ignoreExit: true},
			opener{name: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", "Start-Process '" + u + "'"}},
		)
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		c = append(c,
			opener{name: "xdg-open", args: []string{u}},
			opener{name: "gio", args: []string{"open", u}},
			opener{name: "x-www-browser", args: []string{u}}, // GUI by definition; "www-browser" is text-mode
		)
	}
	return c
}

func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}
