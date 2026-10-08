package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/log/v2"
)

func TestSetupDisabledWritesNothing(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv(EnvDebug, "")

	closeLog, err := Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	log.Debug("hidden")

	if _, err := os.Stat(filepath.Join(state, "lgtm")); !os.IsNotExist(err) {
		t.Fatalf("log directory created although logging is disabled: %v", err)
	}
}

func TestSetupWritesToXDGStateHome(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	t.Setenv(EnvDebug, "1")

	closeLog, err := Setup()
	if err != nil {
		t.Fatal(err)
	}
	log.Debug("hello", "k", "v")
	closeLog()

	got, err := os.ReadFile(filepath.Join(state, "lgtm", "lgtm.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "hello") {
		t.Fatalf("log file lacks message: %q", got)
	}
}
