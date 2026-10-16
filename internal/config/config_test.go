package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sdillen/lgtm/internal/config"
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

func TestLoadParsesLeaderAndKeyBindings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(`leader = ","

[keys]
top = "<home>"
down = ["j", "<down>"]
up = []
`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Leader != "," {
		t.Errorf("Leader = %q, want ,", cfg.Leader)
	}
	want := map[string]config.KeySeqs{
		"top":  {"<home>"},
		"down": {"j", "<down>"},
		"up":   {},
	}
	if !reflect.DeepEqual(cfg.Keys, want) {
		t.Errorf("Keys = %#v, want %#v", cfg.Keys, want)
	}
}

func TestLoadRejectsNonStringKeyBindings(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("[keys]\ntop = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(dir); err == nil {
		t.Fatal("numeric key binding loaded, want an error")
	}
}

func TestLoadParsesTheSmoothScrollSetting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("smooth_scroll = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SmoothScroll == nil {
		t.Fatal("SmoothScroll = nil, want false")
	}
	if *cfg.SmoothScroll {
		t.Error("SmoothScroll = true, want false")
	}
}

func TestLoadWithoutSmoothScrollSettingMeansTheDefault(t *testing.T) {
	cfg, err := config.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SmoothScroll != nil {
		t.Errorf("SmoothScroll = %v, want nil (the default)", *cfg.SmoothScroll)
	}
}

func TestLoadParsesTheIconsSetting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("icons = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Icons {
		t.Error("Icons = false, want true")
	}
}
