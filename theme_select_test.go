package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/config"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

func TestParseThemeFlag(t *testing.T) {
	subj, opt, err := parse([]string{"--theme", "tokyo-night-day"})
	if err != nil {
		t.Fatal(err)
	}
	if opt.theme != "tokyo-night-day" {
		t.Errorf("theme = %q, want tokyo-night-day", opt.theme)
	}
	if subj.kind != subjectDefault {
		t.Errorf("kind = %v, want subjectDefault", subj.kind)
	}
}

func TestParseThemeFlagEqualsForm(t *testing.T) {
	_, opt, err := parse([]string{"--theme=paper"})
	if err != nil {
		t.Fatal(err)
	}
	if opt.theme != "paper" {
		t.Errorf("theme = %q, want paper", opt.theme)
	}
}

func TestParseThemeFlagBeforeAndAfterSubject(t *testing.T) {
	for _, args := range [][]string{
		{"--theme", "paper", "abc1234"},
		{"abc1234", "--theme", "paper"},
	} {
		subj, opt, err := parse(args)
		if err != nil {
			t.Fatalf("parse(%v): %v", args, err)
		}
		if opt.theme != "paper" || subj.kind != subjectCommit || subj.rev != "abc1234" {
			t.Errorf("parse(%v) = (%+v, %+v), want commit abc1234 with theme paper", args, subj, opt)
		}
	}
}

func TestParseUnknownFlagFails(t *testing.T) {
	_, _, err := parse([]string{"--nope"})
	if err == nil {
		t.Fatal("unknown flag accepted, want an error")
	}
}

func TestResolveThemeFlagWinsOverConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), "theme = \"config-theme\"\n")
	writeTheme(t, dir, "flag-theme")

	th, err := resolveTheme("flag-theme", loadConfig(t, dir), dir, detectStub(theme.KindDark, true))
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "flag-theme" {
		t.Errorf("theme = %q, want the flag theme", th.Name)
	}
}

func TestResolveThemeConfigWinsOverDetection(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "config.toml"), "theme = \"config-theme\"\n")
	writeTheme(t, dir, "config-theme")

	th, err := resolveTheme("", loadConfig(t, dir), dir, detectStub(theme.KindLight, true))
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "config-theme" {
		t.Errorf("theme = %q, want the config theme", th.Name)
	}
}

func TestResolveThemeAutoDetectsBackground(t *testing.T) {
	th, err := resolveTheme("", config.Config{}, t.TempDir(), detectStub(theme.KindLight, true))
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "tokyo-night-day" {
		t.Errorf("theme = %q, want tokyo-night-day for a light terminal", th.Name)
	}

	th, err = resolveTheme("", config.Config{}, t.TempDir(), detectStub(theme.KindDark, true))
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "tokyo-night-night" {
		t.Errorf("theme = %q, want tokyo-night-night for a dark terminal", th.Name)
	}
}

func TestResolveThemeDefaultsToDarkWhenDetectionFails(t *testing.T) {
	th, err := resolveTheme("", config.Config{}, t.TempDir(), detectStub(theme.KindDark, false))
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "tokyo-night-night" {
		t.Errorf("theme = %q, want the dark fallback", th.Name)
	}
}

func TestResolveThemeUnknownNameFails(t *testing.T) {
	_, err := resolveTheme("does-not-exist", config.Config{}, t.TempDir(), detectStub(theme.KindDark, false))
	if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("err = %v, want a not-found error naming the theme", err)
	}
}

// loadConfig loads the config.toml in dir.
func loadConfig(t *testing.T, dir string) config.Config {
	t.Helper()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// detectStub is a BackgroundKind stand-in for tests.
func detectStub(kind theme.Kind, ok bool) func() (theme.Kind, bool) {
	return func() (theme.Kind, bool) { return kind, ok }
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTheme(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, "themes", name+".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, "[ui]\naccent = \"#123456\"\n")
}
