// Package theme loads built-in and custom themes, detects the terminal
// background, and maps semantic tokens and chroma styles onto lipgloss
// styles. A theme is a TOML file with semantic tokens: UI colours, diff
// backgrounds, comment categories and a chroma style as syntax base plus
// per-token overrides. Tokyo Night Day and Tokyo Night Night are built in.
package theme

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Kind classifies a terminal background as dark or light.
type Kind int

const (
	// KindDark is a dark background: the default and the tokyo night night
	// palette.
	KindDark Kind = iota
	// KindLight is a light background: the tokyo night day palette.
	KindLight
)

// Detect classifies a terminal background colour. The threshold follows
// lipgloss: an HSL lightness below 0.5 is dark, everything else light.
func Detect(c color.Color) Kind {
	col, err := toColorful(c)
	if err != nil {
		return KindDark
	}
	_, _, l := col.Hsl()
	if l < 0.5 {
		return KindDark
	}
	return KindLight
}

// Theme is a fully resolved theme: the semantic styles for the UI and the
// syntax highlighter, both adapted to one colour profile.
type Theme struct {
	// Name is the theme name, e.g. "tokyo-night-night" or the basename of
	// a custom theme file.
	Name string

	// Styles are the semantic tokens mapped onto lipgloss styles.
	Styles Styles

	// Highlighter colours code with the theme's syntax style.
	Highlighter *Highlighter

	spec Spec
}

// Default returns the built-in theme for a light or dark background:
// tokyo-night-day for light terminals, tokyo-night-night for dark ones.
func Default(kind Kind) Theme {
	return MustResolve(builtinSpec(kind), colorprofile.TrueColor)
}

// Load loads the theme with the given name: first from
// <dir>/themes/<name>.toml, then from the built-in themes, so a custom
// theme can shadow a built-in one. dir is the lgtm config directory, for
// example $XDG_CONFIG_HOME/lgtm.
func Load(name, dir string, profile colorprofile.Profile) (Theme, error) {
	if name == "" {
		return Theme{}, fmt.Errorf("theme name is empty")
	}
	if name != filepath.Base(name) {
		return Theme{}, fmt.Errorf("invalid theme name %q", name)
	}
	if spec, ok, err := loadCustom(name, dir); err != nil {
		return Theme{}, err
	} else if ok {
		return resolve(spec, profile)
	}
	for _, spec := range builtinSpecs() {
		if spec.Name == name {
			return resolve(spec, profile)
		}
	}
	return Theme{}, fmt.Errorf("theme %q not found (looked in %s and in the built-in themes)",
		name, filepath.Join(dir, "themes"))
}

// loadCustom reads <dir>/themes/<name>.toml. It reports ok=false when the
// file does not exist.
func loadCustom(name, dir string) (Spec, bool, error) {
	var spec Spec
	path := filepath.Join(dir, "themes", name+".toml")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return spec, false, nil
	}
	if err != nil {
		return spec, false, fmt.Errorf("theme %q: %w", name, err)
	}
	if err := decodeSpec(data, &spec); err != nil {
		return spec, false, fmt.Errorf("theme %q: %w", name, err)
	}
	if spec.Name == "" {
		spec.Name = name
	}
	return spec, true, nil
}

// ForProfile re-resolves the theme for another colour profile, e.g. the
// 256-colour fallback for terminals without truecolor support.
func (t Theme) ForProfile(p colorprofile.Profile) Theme {
	return MustResolve(t.spec, p)
}

// BackgroundKind asks the terminal for its background colour and classifies
// it. It reports ok=false when the terminal cannot be queried (e.g. not a
// terminal at all); the caller then falls back to the dark theme.
func BackgroundKind(in, out term.File) (Kind, bool) {
	bg, err := lipgloss.BackgroundColor(in, out)
	if err != nil || bg == nil {
		return KindDark, false
	}
	return Detect(bg), true
}
