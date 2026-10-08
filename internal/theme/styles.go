package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// Styles are the semantic tokens of a theme mapped onto lipgloss styles.
// Text roles are styles, background washes and the cursor are plain colours
// the UI applies on top.
type Styles struct {
	// Status is the status line text (accent colour).
	Status lipgloss.Style
	// Gutter is the old/new line number gutter.
	Gutter lipgloss.Style
	// Fold is the "N unchanged lines" marker.
	Fold lipgloss.Style
	// Hunk is the unified view section header.
	Hunk lipgloss.Style
	// Purple and Orange are accent colours for telling elements apart.
	Purple lipgloss.Style
	Orange lipgloss.Style
	// Cursor is the background of the cursor row.
	Cursor color.Color

	// Added and Deleted are the colours of the +/- markers of changed
	// lines.
	Added   lipgloss.Style
	Deleted lipgloss.Style
	// AddedBG and DeletedBG are the background washes of changed lines.
	AddedBG   color.Color
	DeletedBG color.Color
	// WordAddedBG and WordDeletedBG are the intra-line word highlight
	// backgrounds (used by the token diff rendering).
	WordAddedBG   color.Color
	WordDeletedBG color.Color

	// CommentMust, CommentShould, CommentNit and CommentQuestion are the
	// comment categories.
	CommentMust     lipgloss.Style
	CommentShould   lipgloss.Style
	CommentNit      lipgloss.Style
	CommentQuestion lipgloss.Style
}

// palette is a theme's semantic colours before mapping onto styles. A nil
// colour is unset (the profile cannot show it).
type palette struct {
	status, gutter, fold, hunk, cursor color.Color
	purple, orange                     color.Color
	added, addedBG, deleted, deletedBG color.Color
	wordAddedBG, wordDeletedBG         color.Color
	must, should, nit, question        color.Color
}

// buildPalette parses the colour strings of a spec for one colour profile.
// Empty strings stay nil; invalid strings are errors.
func buildPalette(spec Spec, profile colorprofile.Profile) (palette, error) {
	var p palette
	parse := func(dst *color.Color, s string) error {
		c, err := parseHexColour(s)
		if err != nil {
			return err
		}
		if c.IsSet() {
			*dst = profile.Convert(toColor(c))
		}
		return nil
	}
	var err error
	for _, f := range []func() error{
		func() error { return parse(&p.status, spec.UI.Accent) },
		func() error { return parse(&p.gutter, spec.UI.Dim) },
		func() error { return parse(&p.fold, spec.UI.Dim) },
		func() error { return parse(&p.hunk, spec.UI.Dim) },
		func() error { return parse(&p.cursor, spec.UI.Cursor) },
		func() error { return parse(&p.purple, spec.UI.Purple) },
		func() error { return parse(&p.orange, spec.UI.Orange) },
		func() error { return parse(&p.added, spec.Diff.Added) },
		func() error { return parse(&p.addedBG, spec.Diff.AddedBG) },
		func() error { return parse(&p.deleted, spec.Diff.Deleted) },
		func() error { return parse(&p.deletedBG, spec.Diff.DeletedBG) },
		func() error { return parse(&p.wordAddedBG, spec.Diff.WordAddedBG) },
		func() error { return parse(&p.wordDeletedBG, spec.Diff.WordDeletedBG) },
		func() error { return parse(&p.must, spec.Comment.Must) },
		func() error { return parse(&p.should, spec.Comment.Should) },
		func() error { return parse(&p.nit, spec.Comment.Nit) },
		func() error { return parse(&p.question, spec.Comment.Question) },
	} {
		if err = f(); err != nil {
			return palette{}, err
		}
	}
	return p, nil
}

// buildStyles maps the palette onto lipgloss styles.
func buildStyles(p palette) Styles {
	return Styles{
		Status:          fgStyle(p.status),
		Gutter:          fgStyle(p.gutter),
		Fold:            fgStyle(p.fold),
		Hunk:            fgStyle(p.hunk),
		Purple:          fgStyle(p.purple),
		Orange:          fgStyle(p.orange),
		Cursor:          p.cursor,
		Added:           fgStyle(p.added),
		Deleted:         fgStyle(p.deleted),
		AddedBG:         p.addedBG,
		DeletedBG:       p.deletedBG,
		WordAddedBG:     p.wordAddedBG,
		WordDeletedBG:   p.wordDeletedBG,
		CommentMust:     fgStyle(p.must),
		CommentShould:   fgStyle(p.should),
		CommentNit:      fgStyle(p.nit),
		CommentQuestion: fgStyle(p.question),
	}
}

func fgStyle(c color.Color) lipgloss.Style {
	if c == nil {
		return lipgloss.Style{}
	}
	return lipgloss.NewStyle().Foreground(c)
}
