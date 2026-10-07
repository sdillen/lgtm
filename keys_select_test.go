package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// configHome points the config directory at a fresh temporary directory
// and returns the lgtm config directory inside it.
func configHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	dir := filepath.Join(home, "lgtm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// keyPress is a key press of a printable key.
func keyPress(k string) tea.KeyPressMsg {
	if k == " " {
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	}
	return tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
}

// pressAll presses printable keys in order.
func pressAll(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(keyPress(k))
	}
	return m
}

func TestRunAppliesKeyBindingsAndLeaderFromConfig(t *testing.T) {
	cfgDir := configHome(t)
	writeFile(t, filepath.Join(cfgDir, "config.toml"),
		"leader = \",\"\n[keys]\ntoggle-unified = \"U\"\n")
	dir := repoWithRange(t)
	got := captureProgram(t)
	if err := run(dir, []string{"main..feature"}); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 40, Height: 5})

	m = pressAll(m, "U")
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "[unified]") {
		t.Errorf("U did not toggle the unified view:\n%s", screen)
	}

	// The leader is "," now: ,t opens the command line with :theme.
	m = pressAll(m, ",", "t")
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, ":theme") {
		t.Errorf(",t did not open :theme:\n%s", screen)
	}
}

func TestRunRejectsKeyBindingsForUnknownActions(t *testing.T) {
	cfgDir := configHome(t)
	writeFile(t, filepath.Join(cfgDir, "config.toml"), "[keys]\nfly = \"f\"\n")
	dir := repoWithRange(t)
	captureProgram(t)

	err := run(dir, []string{"main..feature"})
	if err == nil || !strings.Contains(err.Error(), "fly") {
		t.Errorf("err = %v, want an error naming the unknown action", err)
	}
}

func TestRunMalformedConfigFails(t *testing.T) {
	cfgDir := configHome(t)
	writeFile(t, filepath.Join(cfgDir, "config.toml"), "no = [toml")
	dir := repoWithRange(t)
	captureProgram(t)

	err := run(dir, []string{"main..feature"})
	if err == nil || !strings.Contains(err.Error(), "config") {
		t.Errorf("err = %v, want a config error", err)
	}
}

func TestRunThemeCommandLoadsCustomThemes(t *testing.T) {
	cfgDir := configHome(t)
	writeTheme(t, cfgDir, "mine")
	dir := repoWithRange(t)
	got := captureProgram(t)
	if err := run(dir, []string{"main..feature"}); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 40, Height: 5})

	m = pressAll(m, ":", "t", "h", "e", "m", "e", " ", "m", "i", "n", "e")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = pressAll(m, ":", "t", "h", "e", "m", "e")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "theme: mine") {
		t.Errorf(":theme mine did not switch:\n%s", screen)
	}
}
