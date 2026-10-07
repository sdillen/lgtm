package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/config"
)

func TestDirUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join("/xdg", "lgtm") {
		t.Errorf("Dir() = %q, want /xdg/lgtm", dir)
	}
}

func TestDirFallsBackToUserConfigDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	dir, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(want, "lgtm") {
		t.Errorf("Dir() = %q, want %s", dir, filepath.Join(want, "lgtm"))
	}
}

func TestLoadMissingConfigFileIsEmpty(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "" {
		t.Errorf("Theme = %q, want empty", cfg.Theme)
	}
}

func TestLoadParsesTheThemeSetting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("theme = \"my-theme\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "my-theme" {
		t.Errorf("Theme = %q, want my-theme", cfg.Theme)
	}
}

func TestLoadIgnoresUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"),
		[]byte("theme = \"x\"\n[future]\nsetting = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "x" {
		t.Errorf("Theme = %q, want x", cfg.Theme)
	}
}

func TestLoadMalformedConfigFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("no = [toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(dir)
	if err == nil {
		t.Fatal("malformed config loaded, want an error")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Errorf("err = %v, want a config error", err)
	}
}
