package theme_test

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// parseHex turns a hex colour into a color.Color for Detect.
func parseHex(s string) (color.Color, error) {
	c := chroma.ParseColour(s)
	if !c.IsSet() {
		return nil, fmt.Errorf("invalid colour %q", s)
	}
	return color.RGBA{R: c.Red(), G: c.Green(), B: c.Blue(), A: 0xff}, nil
}

// styleFG returns the ANSI-rendered text of a style, e.g. for asserting the
// exact colour triplet "38;2;r;g;b" of a semantic token.
func styleFG(s lipgloss.Style) string {
	return s.Render("x")
}

func TestDetectClassifiesBackgroundAsDarkOrLight(t *testing.T) {
	tests := []struct {
		hex  string
		want theme.Kind
	}{
		{"#1a1b26", theme.KindDark}, // tokyo night night background
		{"#000000", theme.KindDark},
		{"#e1e2e7", theme.KindLight}, // tokyo night day background
		{"#ffffff", theme.KindLight},
		{"#808080", theme.KindLight}, // exactly the lightness threshold
	}
	for _, tt := range tests {
		col, err := parseHex(tt.hex)
		if err != nil {
			t.Fatal(err)
		}
		if got := theme.Detect(col); got != tt.want {
			t.Errorf("Detect(%s) = %v, want %v", tt.hex, got, tt.want)
		}
	}
}

func TestDefaultDarkThemeIsTokyoNightNight(t *testing.T) {
	th := theme.Default(theme.KindDark)

	if th.Name != "tokyo-night-night" {
		t.Errorf("Name = %q, want tokyo-night-night", th.Name)
	}

	// The semantic tokens map onto lipgloss styles with the tokyo night
	// night palette.
	checks := []struct {
		name  string
		style lipgloss.Style
		want  string // expected rgb triplet
	}{
		{"status", th.Styles.Status, "122;162;247"},   // accent #7aa2f7
		{"gutter", th.Styles.Gutter, "86;95;137"},     // dim #565f89
		{"fold", th.Styles.Fold, "86;95;137"},         // dim #565f89
		{"hunk", th.Styles.Hunk, "86;95;137"},         // dim #565f89
		{"added", th.Styles.Added, "158;206;106"},     // #9ece6a
		{"deleted", th.Styles.Deleted, "247;118;142"}, // #f7768e
		{"must", th.Styles.CommentMust, "247;118;142"},
		{"should", th.Styles.CommentShould, "224;175;104"},
		{"nit", th.Styles.CommentNit, "125;207;255"},
		{"question", th.Styles.CommentQuestion, "158;206;106"},
	}
	for _, c := range checks {
		got := styleFG(c.style)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s renders %q, want it to contain %q", c.name, got, c.want)
		}
	}

	// Background washes and cursor are colours, not text styles.
	checksBG := []struct {
		name string
		col  color.Color
		want string // expected rgb triplet
	}{
		{"cursor", th.Styles.Cursor, "41;46;66"},                 // #292e42
		{"added_bg", th.Styles.AddedBG, "31;46;36"},              // #1f2e24
		{"deleted_bg", th.Styles.DeletedBG, "50;33;41"},          // #322129
		{"word_added_bg", th.Styles.WordAddedBG, "65;86;42"},     // #41562a
		{"word_deleted_bg", th.Styles.WordDeletedBG, "99;48;47"}, // #63302f
	}
	for _, c := range checksBG {
		got := lipgloss.NewStyle().Background(c.col).Render("x")
		if !strings.Contains(got, c.want) {
			t.Errorf("%s renders %q, want it to contain %q", c.name, got, c.want)
		}
	}
}

func TestDefaultLightThemeIsTokyoNightDay(t *testing.T) {
	th := theme.Default(theme.KindLight)

	if th.Name != "tokyo-night-day" {
		t.Errorf("Name = %q, want tokyo-night-day", th.Name)
	}
	if got := styleFG(th.Styles.Status); !strings.Contains(got, "46;125;233") {
		t.Errorf("status renders %q, want the day accent #2e7de9", got)
	}
	if got := styleFG(th.Styles.Added); !strings.Contains(got, "88;117;57") {
		t.Errorf("added renders %q, want the day green #587539", got)
	}
	if got := styleFG(th.Styles.Deleted); !strings.Contains(got, "245;42;101") {
		t.Errorf("deleted renders %q, want the day red #f52a65", got)
	}
}

func TestDefaultThemesDifferByBackground(t *testing.T) {
	day := theme.Default(theme.KindLight)
	night := theme.Default(theme.KindDark)

	if styleFG(day.Styles.Status) == styleFG(night.Styles.Status) {
		t.Error("day and night status styles are identical, want different palettes")
	}
	if day.Name == night.Name {
		t.Errorf("both themes are named %q", day.Name)
	}
}

func TestBackgroundKindFallsBackWhenNotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "not-a-tty")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })

	kind, ok := theme.BackgroundKind(f, f)
	if ok {
		t.Error("BackgroundKind on a file succeeded, want ok=false")
	}
	if kind != theme.KindDark {
		t.Errorf("kind = %v, want the dark fallback", kind)
	}
}
