package export

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bufferCloser is an io.WriteCloser for a test terminal.
type bufferCloser struct{ *bytes.Buffer }

func (b bufferCloser) Close() error { return nil }

// noTerminal simulates a session without a tty.
func noTerminal() {
	openTTY = func() (ttyWriter, error) {
		return nil, errors.New("no tty")
	}
}

// fakeTerminal captures what Copy would write to the tty.
func fakeTerminal() *bytes.Buffer {
	buf := &bytes.Buffer{}
	openTTY = func() (ttyWriter, error) { return bufferCloser{buf}, nil }
	return buf
}

// fakeTool installs a script under PATH that appends its stdin to the
// file returned; failingTool installs one that fails.
func fakeTool(t *testing.T, name string) string {
	t.Helper()
	capture := filepath.Join(t.TempDir(), name+".clip")
	script := "#!/bin/sh\ncat > " + capture + "\n"
	installTool(t, name, script)
	return capture
}

func failingTool(t *testing.T, name string) {
	t.Helper()
	installTool(t, name, "#!/bin/sh\nexit 1\n")
}

func installTool(t *testing.T, name, script string) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestCopyWritesOSC52ToTheTerminal pins the primary path: the prompt
// lands in the terminal clipboard selection as the base64 OSC 52
// escape, without touching any local tool.
func TestCopyWritesOSC52ToTheTerminal(t *testing.T) {
	tty := fakeTerminal()
	failingTool(t, "wl-copy")
	failingTool(t, "xclip")
	failingTool(t, "pbcopy")

	if err := Copy("hello"); err != nil {
		t.Fatal(err)
	}
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("hello")) + "\x07"
	if tty.String() != want {
		t.Errorf("terminal got %q, want the OSC 52 escape %q", tty.String(), want)
	}
}

// TestCopyFallsBackToAClipboardTool pins the fallback: without a
// terminal the text goes through the first working tool, wl-copy first.
func TestCopyFallsBackToAClipboardTool(t *testing.T) {
	noTerminal()
	wlCapture := fakeTool(t, "wl-copy")
	xclipCapture := fakeTool(t, "xclip")

	if err := Copy("hello"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(wlCapture); err != nil || string(data) != "hello" {
		t.Errorf("wl-copy got %q, %v; want hello", data, err)
	}
	if _, err := os.ReadFile(xclipCapture); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("xclip was invoked although wl-copy worked: %v", err)
	}
}

// TestCopyFallsThroughAFailingTool pins the order of the fallback
// chain: a tool that fails does not stop the attempt.
func TestCopyFallsThroughAFailingTool(t *testing.T) {
	noTerminal()
	failingTool(t, "wl-copy")
	xclipCapture := fakeTool(t, "xclip")

	if err := Copy("hello"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(xclipCapture); err != nil || string(data) != "hello" {
		t.Errorf("xclip got %q, %v; want hello after wl-copy failed", data, err)
	}
}

// TestCopyReportsWhenNothingWorks pins that a session without a
// terminal and without any clipboard tool is an error, not silent
// success.
func TestCopyReportsWhenNothingWorks(t *testing.T) {
	noTerminal()
	failingTool(t, "wl-copy")
	failingTool(t, "xclip")
	failingTool(t, "pbcopy")

	if err := Copy("hello"); err == nil {
		t.Fatal("Copy succeeded although no clipboard is available")
	} else if !strings.Contains(err.Error(), "wl-copy") {
		t.Errorf("error %q does not name the tried tools", err)
	}
}
