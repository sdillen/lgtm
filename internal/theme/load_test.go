package theme_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/sdillen/lgtm/internal/theme"
)

// writeTheme stores a TOML theme file in dir/themes/<name>.toml.
func writeTheme(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, "themes", name+".toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCustomThemeFromConfigDir(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "paper", `
[ui]
accent = "#ff0000"
fg = "#111111"
bg = "#222222"
border = "#333333"

[comment]
must = "#00ff00"
`)
	th, err := theme.Load("paper", dir, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "paper" {
		t.Errorf("Name = %q, want the file name paper", th.Name)
	}
	if got := styleFG(th.Styles.Status); !strings.Contains(got, "255;0;0") {
		t.Errorf("status renders %q, want the overridden accent #ff0000", got)
	}
	if got := styleFG(th.Styles.CommentMust); !strings.Contains(got, "0;255;0") {
		t.Errorf("must renders %q, want the overridden #00ff00", got)
	}
	// Tokens the theme does not set fall back to the built-in dark values.
	if got := styleFG(th.Styles.CommentQuestion); !strings.Contains(got, "158;206;106") {
		t.Errorf("question renders %q, want the dark default #9ece6a", got)
	}
}

func TestLoadCustomThemeUsesTheNameField(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "file-name", `name = "display-name"`+"\n")
	th, err := theme.Load("file-name", dir, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "display-name" {
		t.Errorf("Name = %q, want display-name from the file", th.Name)
	}
}

func TestLoadCustomThemeShadowsBuiltin(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "tokyo-night-night", `
[ui]
accent = "#ff0000"
`)
	th, err := theme.Load("tokyo-night-night", dir, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	if got := styleFG(th.Styles.Status); !strings.Contains(got, "255;0;0") {
		t.Errorf("status renders %q, want the custom theme to win over the builtin", got)
	}
}

func TestLoadBuiltinByDefault(t *testing.T) {
	dir := t.TempDir()
	th, err := theme.Load("tokyo-night-day", dir, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	if th.Name != "tokyo-night-day" {
		t.Errorf("Name = %q, want tokyo-night-day", th.Name)
	}
}

func TestLoadMissingThemeFails(t *testing.T) {
	_, err := theme.Load("does-not-exist", t.TempDir(), colorprofile.TrueColor)
	if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("err = %v, want a not-found error naming the theme", err)
	}
}

func TestLoadInvalidThemeNameFails(t *testing.T) {
	for _, name := range []string{"", "a/b", "../etc/passwd", `a\b`} {
		if _, err := theme.Load(name, t.TempDir(), colorprofile.TrueColor); err == nil {
			t.Errorf("Load(%q) succeeded, want an error", name)
		}
	}
}

func TestLoadMalformedThemeFails(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "broken", "this is not [ toml")
	if _, err := theme.Load("broken", dir, colorprofile.TrueColor); err == nil {
		t.Error("malformed theme loaded, want an error")
	}
}

func TestLoadInvalidColourFails(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "bad-colour", `[ui]
accent = "not-a-colour"
`)
	if _, err := theme.Load("bad-colour", dir, colorprofile.TrueColor); err == nil {
		t.Error("theme with an invalid colour loaded, want an error")
	}
}

func TestLoadUnknownSyntaxBaseFails(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "bad-base", `[syntax]
base = "no-such-chroma-style"
`)
	if _, err := theme.Load("bad-base", dir, colorprofile.TrueColor); err == nil ||
		!strings.Contains(err.Error(), "no-such-chroma-style") {
		t.Errorf("err = %v, want an unknown syntax base error", err)
	}
}

func TestLoadUnknownTokenTypeFails(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "bad-token", `[syntax.overrides]
NotAToken = "#ff0000"
`)
	if _, err := theme.Load("bad-token", dir, colorprofile.TrueColor); err == nil ||
		!strings.Contains(err.Error(), "NotAToken") {
		t.Errorf("err = %v, want an unknown token type error", err)
	}
}

func TestLoadInvalidOverrideFails(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "bad-override", `[syntax.overrides]
Keyword = { fg = 42 }
`)
	if _, err := theme.Load("bad-override", dir, colorprofile.TrueColor); err == nil {
		t.Error("override with a non-string fg loaded, want an error")
	}
}

func TestForProfileFallsBackTo256Colours(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "flat", `
[ui]
accent = "#ffffff"
`)
	th, err := theme.Load("flat", dir, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	if got := styleFG(th.Styles.Status); !strings.Contains(got, "38;2;255;255;255") {
		t.Errorf("truecolor status renders %q, want the exact rgb colour", got)
	}

	th256 := th.ForProfile(colorprofile.ANSI256)
	// Pure white is palette entry 231 of the 256 colour cube.
	if got := styleFG(th256.Styles.Status); !strings.Contains(got, "38;5;231") {
		t.Errorf("256 status renders %q, want the palette colour 38;5;231", got)
	}
}

func TestResolveIsIdempotentForProfiles(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "paper", `[ui]
accent = "#ff0000"
`)
	th, err := theme.Load("paper", dir, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	again := th.ForProfile(colorprofile.TrueColor)
	if styleFG(again.Styles.Status) != styleFG(th.Styles.Status) {
		t.Errorf("ForProfile(truecolor) changed the styles: %q != %q",
			styleFG(again.Styles.Status), styleFG(th.Styles.Status))
	}
}

func TestNamesListsBuiltInAndCustomThemes(t *testing.T) {
	dir := t.TempDir()
	writeTheme(t, dir, "paper", "[ui]\n")
	writeTheme(t, dir, "tokyo-night-day", "[ui]\n") // shadows a built-in

	got := strings.Join(theme.Names(dir), " ")

	if want := "paper tokyo-night-day tokyo-night-night"; got != want {
		t.Errorf("Names = %q, want %q", got, want)
	}
}

func TestNamesWithoutConfigDirListsTheBuiltInThemes(t *testing.T) {
	got := strings.Join(theme.Names(filepath.Join(t.TempDir(), "missing")), " ")

	if want := "tokyo-night-day tokyo-night-night"; got != want {
		t.Errorf("Names = %q, want %q", got, want)
	}
}
