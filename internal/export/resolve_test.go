package export_test

import (
	"os"
	"path/filepath"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/export"
)

// writeFile creates path with content, creating parent directories.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestResolvePrefersTheProjectTemplate pins the resolution order: a
// template in <dir>/.lgtm wins over the global one in the config
// directory.
func TestResolvePrefersTheProjectTemplate(t *testing.T) {
	dir := t.TempDir()
	cfgDir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".lgtm", "prompt.template"), "project template")
	writeFile(t, filepath.Join(cfgDir, "prompt.template"), "global template")

	got, err := export.Resolve(dir, cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "project template" {
		t.Errorf("Resolve = %q, want the project template", got)
	}
}

// TestResolveFallsBackToTheGlobalTemplate pins that the global template
// in the config directory is used when the project has none.
func TestResolveFallsBackToTheGlobalTemplate(t *testing.T) {
	dir := t.TempDir()
	cfgDir := t.TempDir()
	writeFile(t, filepath.Join(cfgDir, "prompt.template"), "global template")

	got, err := export.Resolve(dir, cfgDir)
	if err != nil {
		t.Fatal(err)
	}
	if got != "global template" {
		t.Errorf("Resolve = %q, want the global template", got)
	}
}

// TestResolveFallsBackToTheBuiltinTemplate pins that the built-in
// template is used when neither the project nor the config directory
// has one.
func TestResolveFallsBackToTheBuiltinTemplate(t *testing.T) {
	got, err := export.Resolve(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != export.DefaultTemplate {
		t.Error("Resolve did not return the built-in template")
	}
}

// TestResolveWithoutConfigDirSkipsTheGlobalTemplate pins that an empty
// config directory (no XDG config home) only consults project and
// built-in templates.
func TestResolveWithoutConfigDirSkipsTheGlobalTemplate(t *testing.T) {
	got, err := export.Resolve(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if got != export.DefaultTemplate {
		t.Error("Resolve did not return the built-in template without a config dir")
	}
}

// TestResolveReportsAnUnreadableProjectTemplate pins that a project
// template that exists but cannot be read is an error instead of being
// silently replaced by the global template.
func TestResolveReportsAnUnreadableProjectTemplate(t *testing.T) {
	dir := t.TempDir()
	cfgDir := t.TempDir()
	writeFile(t, filepath.Join(cfgDir, "prompt.template"), "global template")
	// A directory at the template path exists but cannot be read as a
	// file.
	if err := os.MkdirAll(filepath.Join(dir, ".lgtm", "prompt.template"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := export.Resolve(dir, cfgDir); err == nil {
		t.Fatal("Resolve succeeded although the project template is unreadable")
	}
}
