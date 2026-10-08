package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// MixColor blends a towards b: t=0 is a, t=1 is b. The blend runs in
// Lab space, so the midpoint of two colours does not turn muddy. An
// unset colour cannot be blended; the result then switches at t=0.5.
func MixColor(a, b color.Color, t float64) color.Color {
	t = min(max(t, 0), 1)
	ca, errA := toColorful(a)
	cb, errB := toColorful(b)
	if errA != nil || errB != nil || isNoColor(a) || isNoColor(b) {
		if t < 0.5 {
			return a
		}
		return b
	}
	return ca.BlendLab(cb, t).Clamped()
}

func isNoColor(c color.Color) bool {
	_, ok := c.(lipgloss.NoColor)
	return c == nil || ok
}

// mixStyle is b with its foreground blended from a's.
func mixStyle(a, b lipgloss.Style, t float64) lipgloss.Style {
	fa, fb := a.GetForeground(), b.GetForeground()
	if isNoColor(fa) || isNoColor(fb) {
		if t < 0.5 {
			return a
		}
		return b
	}
	return b.Foreground(MixColor(fa, fb, t))
}

// Mix blends the semantic styles of two themes: t=0 is a, t=1 is b.
// It drives the cross-fade of :theme; the colours of the syntax
// highlighter are not part of it.
func Mix(a, b Styles, t float64) Styles {
	c := func(x, y color.Color) color.Color {
		if x == nil || y == nil {
			if t < 0.5 {
				return x
			}
			return y
		}
		return MixColor(x, y, t)
	}
	return Styles{
		Status:          mixStyle(a.Status, b.Status, t),
		Gutter:          mixStyle(a.Gutter, b.Gutter, t),
		Fold:            mixStyle(a.Fold, b.Fold, t),
		Hunk:            mixStyle(a.Hunk, b.Hunk, t),
		Purple:          mixStyle(a.Purple, b.Purple, t),
		Orange:          mixStyle(a.Orange, b.Orange, t),
		Cursor:          c(a.Cursor, b.Cursor),
		Added:           mixStyle(a.Added, b.Added, t),
		Deleted:         mixStyle(a.Deleted, b.Deleted, t),
		AddedBG:         c(a.AddedBG, b.AddedBG),
		DeletedBG:       c(a.DeletedBG, b.DeletedBG),
		WordAddedBG:     c(a.WordAddedBG, b.WordAddedBG),
		WordDeletedBG:   c(a.WordDeletedBG, b.WordDeletedBG),
		CommentMust:     mixStyle(a.CommentMust, b.CommentMust, t),
		CommentShould:   mixStyle(a.CommentShould, b.CommentShould, t),
		CommentNit:      mixStyle(a.CommentNit, b.CommentNit, t),
		CommentQuestion: mixStyle(a.CommentQuestion, b.CommentQuestion, t),
	}
}
