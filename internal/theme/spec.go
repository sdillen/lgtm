package theme

import (
	"fmt"
	"image/color"

	"github.com/BurntSushi/toml"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/lucasb-eyer/go-colorful"
)

// Spec is a raw theme: the semantic token colours and the syntax style.
// Custom themes are TOML files in this format; the built-in themes are the
// same structs in code.
//
// A theme file looks like this:
//
//	name = "my-theme"
//
//	[ui]
//	fg = "#c0caf5"      # plain text (reserved for future views)
//	bg = "#1a1b26"      # background (reserved for future views)
//	dim = "#565f89"     # gutters, fold markers, hunk headers
//	accent = "#7aa2f7"  # status line
//	cursor = "#2f334d"  # cursor row background
//	border = "#3b4261"  # overlay borders (reserved for future views)
//
//	[diff]
//	added = "#9ece6a"        # the " + " marker
//	added_bg = "#1f2e24"     # background wash of added lines
//	deleted = "#f7768e"      # the " - " marker
//	deleted_bg = "#322129"   # background wash of deleted lines
//	word_added_bg = "#41562a"    # intra-line word highlight (added)
//	word_deleted_bg = "#63302f"  # intra-line word highlight (deleted)
//
//	[comment]
//	must = "#f7768e"
//	should = "#e0af68"
//	nit = "#7dcfff"
//	question = "#9ece6a"
//
//	[syntax]
//	base = "tokyonight-night"  # any registered chroma style
//
//	[syntax.overrides]
//	Keyword = "#bb9af7"                       # a bare colour
//	Comment = { fg = "#565f89", italic = true }  # or a table
//
// All colours are "#rrggbb" strings. A missing token falls back to the
// built-in dark theme's value, so a custom theme may only override what it
// wants to change.
type Spec struct {
	// Name is shown in the UI; custom themes default to their file name.
	Name string `toml:"name"`

	UI      UISpec      `toml:"ui"`
	Diff    DiffSpec    `toml:"diff"`
	Comment CommentSpec `toml:"comment"`
	Syntax  SyntaxSpec  `toml:"syntax"`
}

// UISpec is the UI token set: status line accent, gutters and markers (dim),
// cursor row and (for future overlays) border.
type UISpec struct {
	FG     string `toml:"fg"`
	BG     string `toml:"bg"`
	Dim    string `toml:"dim"`
	Accent string `toml:"accent"`
	Cursor string `toml:"cursor"`
	Border string `toml:"border"`
}

// DiffSpec is the diff token set: the fg of the +/- markers, the background
// washes of added/deleted lines and the intra-line word highlight
// backgrounds.
type DiffSpec struct {
	Added         string `toml:"added"`
	AddedBG       string `toml:"added_bg"`
	Deleted       string `toml:"deleted"`
	DeletedBG     string `toml:"deleted_bg"`
	WordAddedBG   string `toml:"word_added_bg"`
	WordDeletedBG string `toml:"word_deleted_bg"`
}

// CommentSpec maps the four comment categories to colours.
type CommentSpec struct {
	Must     string `toml:"must"`
	Should   string `toml:"should"`
	Nit      string `toml:"nit"`
	Question string `toml:"question"`
}

// SyntaxSpec is the syntax style: a registered chroma style as base plus
// per-token overrides.
type SyntaxSpec struct {
	// Base is a chroma style name, e.g. "tokyonight-night".
	Base string `toml:"base"`
	// Overrides replace the base's entry for the named token type, e.g.
	// "Comment" or "Keyword". Sub-types without their own entry inherit
	// the override.
	Overrides map[string]Override `toml:"overrides"`
}

// Override is one syntax token override: either a bare colour string or a
// table with fg/bg and bold/italic/underline flags.
type Override struct {
	FG        string
	BG        string
	Bold      *bool
	Italic    *bool
	Underline *bool
}

// UnmarshalTOML accepts both forms:
//
//	Keyword = "#bb9af7"
//	Keyword = { fg = "#bb9af7", italic = true }
func (o *Override) UnmarshalTOML(v any) error {
	switch t := v.(type) {
	case string:
		o.FG = t
		return nil
	case map[string]any:
		for k, val := range t {
			switch k {
			case "fg", "bg":
				s, ok := val.(string)
				if !ok {
					return fmt.Errorf("override %q must be a colour string", k)
				}
				if k == "fg" {
					o.FG = s
				} else {
					o.BG = s
				}
			case "bold", "italic", "underline":
				b, ok := val.(bool)
				if !ok {
					return fmt.Errorf("override %q must be a bool", k)
				}
				switch k {
				case "bold":
					o.Bold = &b
				case "italic":
					o.Italic = &b
				case "underline":
					o.Underline = &b
				}
			default:
				return fmt.Errorf("unknown override field %q", k)
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid syntax override: want a colour string or a table")
	}
}

// entry converts the override into a chroma style entry.
func (o Override) entry() (chroma.StyleEntry, error) {
	var e chroma.StyleEntry
	var err error
	if o.FG != "" {
		if e.Colour, err = parseHexColour(o.FG); err != nil {
			return e, fmt.Errorf("syntax override fg: %w", err)
		}
	}
	if o.BG != "" {
		if e.Background, err = parseHexColour(o.BG); err != nil {
			return e, fmt.Errorf("syntax override bg: %w", err)
		}
	}
	if o.Bold != nil {
		e.Bold = trilean(*o.Bold)
	}
	if o.Italic != nil {
		e.Italic = trilean(*o.Italic)
	}
	if o.Underline != nil {
		e.Underline = trilean(*o.Underline)
	}
	return e, nil
}

func trilean(b bool) chroma.Trilean {
	if b {
		return chroma.Yes
	}
	return chroma.No
}

// decodeSpec parses a theme TOML file. Unknown fields are ignored so that
// future theme formats stay readable.
func decodeSpec(data []byte, spec *Spec) error {
	if err := toml.Unmarshal(data, spec); err != nil {
		return fmt.Errorf("invalid theme: %w", err)
	}
	return nil
}

// builtinSpecs are the two Tokyo Night themes.
func builtinSpecs() []Spec {
	return []Spec{builtinSpec(KindDark), builtinSpec(KindLight)}
}

// builtinSpec returns the built-in theme for a light or dark background.
func builtinSpec(kind Kind) Spec {
	if kind == KindLight {
		return Spec{
			Name: "tokyo-night-day",
			UI: UISpec{
				FG:     "#3760bf",
				BG:     "#e1e2e7",
				Dim:    "#848cb5",
				Accent: "#2e7de9",
				Cursor: "#d3d6e5",
				Border: "#b7c1e3",
			},
			Diff: DiffSpec{
				Added:         "#587539",
				AddedBG:       "#e6efe3",
				Deleted:       "#e04b4b",
				DeletedBG:     "#f4e3e3",
				WordAddedBG:   "#c5d8b5",
				WordDeletedBG: "#e7c4c4",
			},
			Comment: CommentSpec{
				Must:     "#e04b4b",
				Should:   "#b5881f",
				Nit:      "#007197",
				Question: "#587539",
			},
			Syntax: SyntaxSpec{Base: "tokyonight-day"},
		}
	}
	return Spec{
		Name: "tokyo-night-night",
		UI: UISpec{
			FG:     "#c0caf5",
			BG:     "#1a1b26",
			Dim:    "#565f89",
			Accent: "#7aa2f7",
			Cursor: "#2f334d",
			Border: "#3b4261",
		},
		Diff: DiffSpec{
			Added:         "#9ece6a",
			AddedBG:       "#1f2e24",
			Deleted:       "#f7768e",
			DeletedBG:     "#322129",
			WordAddedBG:   "#41562a",
			WordDeletedBG: "#63302f",
		},
		Comment: CommentSpec{
			Must:     "#f7768e",
			Should:   "#e0af68",
			Nit:      "#7dcfff",
			Question: "#9ece6a",
		},
		Syntax: SyntaxSpec{Base: "tokyonight-night"},
	}
}

// Spec returns the raw theme spec the theme was resolved from, e.g. to
// derive a custom theme from a built-in one.
func (t Theme) Spec() Spec { return t.spec }

// Resolve turns a spec into a theme for one colour profile. Missing tokens
// fall back to the built-in dark theme.
func Resolve(spec Spec, profile colorprofile.Profile) (Theme, error) {
	return resolve(spec, profile)
}

// resolve turns a spec into a theme for one colour profile. Missing tokens
// fall back to the built-in dark theme.
func resolve(spec Spec, profile colorprofile.Profile) (Theme, error) {
	base := builtinSpec(KindDark)
	merge(&base, &spec)
	spec = base
	if spec.Syntax.Base == "" {
		spec.Syntax.Base = "tokyonight-night"
	}
	if spec.Name == "" {
		spec.Name = "tokyo-night-night"
	}

	pal, err := buildPalette(spec, profile)
	if err != nil {
		return Theme{}, err
	}
	hl, err := newHighlighter(spec.Syntax, profile)
	if err != nil {
		return Theme{}, err
	}
	return Theme{
		Name:        spec.Name,
		Styles:      buildStyles(pal),
		Highlighter: hl,
		spec:        spec,
	}, nil
}

// MustResolve resolves a spec and panics on error. It is for specs that are
// valid by construction, like the built-in themes.
func MustResolve(spec Spec, profile colorprofile.Profile) Theme {
	th, err := resolve(spec, profile)
	if err != nil {
		panic(err)
	}
	return th
}

// merge overlays src onto dst: every non-empty field of src wins.
func merge(dst, src *Spec) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	mergeStr(&dst.UI.FG, src.UI.FG)
	mergeStr(&dst.UI.BG, src.UI.BG)
	mergeStr(&dst.UI.Dim, src.UI.Dim)
	mergeStr(&dst.UI.Accent, src.UI.Accent)
	mergeStr(&dst.UI.Cursor, src.UI.Cursor)
	mergeStr(&dst.UI.Border, src.UI.Border)
	mergeStr(&dst.Diff.Added, src.Diff.Added)
	mergeStr(&dst.Diff.AddedBG, src.Diff.AddedBG)
	mergeStr(&dst.Diff.Deleted, src.Diff.Deleted)
	mergeStr(&dst.Diff.DeletedBG, src.Diff.DeletedBG)
	mergeStr(&dst.Diff.WordAddedBG, src.Diff.WordAddedBG)
	mergeStr(&dst.Diff.WordDeletedBG, src.Diff.WordDeletedBG)
	mergeStr(&dst.Comment.Must, src.Comment.Must)
	mergeStr(&dst.Comment.Should, src.Comment.Should)
	mergeStr(&dst.Comment.Nit, src.Comment.Nit)
	mergeStr(&dst.Comment.Question, src.Comment.Question)
	if src.Syntax.Base != "" {
		dst.Syntax.Base = src.Syntax.Base
	}
	if len(src.Syntax.Overrides) > 0 {
		dst.Syntax.Overrides = src.Syntax.Overrides
	}
}

func mergeStr(dst *string, src string) {
	if src != "" {
		*dst = src
	}
}

// parseHexColour parses a "#rrggbb" colour. An empty string yields the
// unset colour; any other invalid string is an error.
func parseHexColour(s string) (chroma.Colour, error) {
	if s == "" {
		return 0, nil
	}
	c := chroma.ParseColour(s)
	if !c.IsSet() {
		return 0, fmt.Errorf("invalid colour %q (want #rrggbb)", s)
	}
	return c, nil
}

// toColor converts a chroma colour into a color.Color.
func toColor(c chroma.Colour) color.Color {
	return color.RGBA{R: c.Red(), G: c.Green(), B: c.Blue(), A: 0xff}
}

// toColorful converts a color.Color into a colorful.Color for luminance
// calculations.
func toColorful(c color.Color) (colorful.Color, error) {
	if c == nil {
		return colorful.Color{}, fmt.Errorf("nil colour")
	}
	col, ok := colorful.MakeColor(c)
	if !ok {
		return colorful.Color{}, fmt.Errorf("cannot convert %T", c)
	}
	return col, nil
}
