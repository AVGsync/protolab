//go:build linux

package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fake(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func setup(t *testing.T, env map[string]string) string {
	dir := t.TempDir()
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	for _, k := range []string{"DISPLAY", "WAYLAND_DISPLAY", "WSL_DISTRO_NAME", "WSL_INTEROP", "BROWSER"} {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return dir
}

const u = "http://127.0.0.1:47213"

func TestNoDisplay(t *testing.T) {
	setup(t, nil)
	if err := Open(u); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("want ErrNoDisplay, got %v", err)
	}
}

func TestXdgOpenOK(t *testing.T) {
	d := setup(t, map[string]string{"DISPLAY": ":0"})
	fake(t, d, "xdg-open", `echo "noise on stdout"; echo "noise on stderr" >&2; echo "$1" > `+d+`/arg`)
	if err := Open(u); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(d + "/arg")
	if string(b) != u+"\n" {
		t.Fatalf("arg = %q", b)
	}
}

func TestFallbackToNextOnFailure(t *testing.T) {
	d := setup(t, map[string]string{"DISPLAY": ":0"})
	fake(t, d, "xdg-open", `exit 3`)
	fake(t, d, "gio", `echo "$@" > `+d+`/gio.args`)
	if err := Open(u); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(d + "/gio.args")
	if string(b) != "open "+u+"\n" {
		t.Fatalf("gio args = %q", b)
	}
}

func TestAllFail(t *testing.T) {
	d := setup(t, map[string]string{"DISPLAY": ":0"})
	fake(t, d, "xdg-open", `exit 3`)
	if err := Open(u); err == nil {
		t.Fatal("want error")
	}
}

func TestLongRunningHandlerIsSuccess(t *testing.T) {
	d := setup(t, map[string]string{"DISPLAY": ":0"})
	fake(t, d, "xdg-open", `exec sleep 20`)
	start := time.Now()
	if err := Open(u); err != nil {
		t.Fatal(err)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("blocked for %v", el)
	}
}

func TestWSLPrefersWslviewThenExplorerExit1(t *testing.T) {
	d := setup(t, map[string]string{"WSL_DISTRO_NAME": "Ubuntu"})
	fake(t, d, "explorer.exe", `echo "$1" > `+d+`/explorer.arg; exit 1`) // explorer.exe: exit code 1 on success
	if err := Open(u); err != nil {
		t.Fatalf("explorer.exe exit 1 must count as success: %v", err)
	}
	if b, _ := os.ReadFile(d + "/explorer.arg"); string(b) != u+"\n" {
		t.Fatalf("explorer arg = %q", b)
	}
	fake(t, d, "wslview", `echo "$1" > `+d+`/wslview.arg`)
	if err := Open(u); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(d + "/wslview.arg"); string(b) != u+"\n" {
		t.Fatalf("wslview arg = %q", b)
	}
}

func TestBrowserEnv(t *testing.T) {
	d := setup(t, map[string]string{"BROWSER": "mybrowser --new-window %s:other"})
	fake(t, d, "mybrowser", `echo "$@" > `+d+`/b.args`)
	if err := Open(u); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(d + "/b.args"); string(b) != "--new-window "+u+"\n" {
		t.Fatalf("args = %q", b)
	}
}

func TestRejectsUnsafeURL(t *testing.T) {
	setup(t, map[string]string{"DISPLAY": ":0"})
	for _, bad := range []string{"file:///etc/passwd", "javascript:alert(1)", "http://127.0.0.1:1/'; calc", "http://x/ y", "ftp://x"} {
		if err := Open(bad); err == nil || errors.Is(err, ErrNoDisplay) {
			t.Errorf("%q must be rejected, got %v", bad, err)
		}
	}
}
