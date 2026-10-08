package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// flashFrames and fadeFrames are the lengths of the reload flash
// (~0.7 s) and the theme cross-fade (~0.4 s) in animation frames.
const (
	flashFrames = 45
	fadeFrames  = 24
)

// easeOut is a quadratic ease-out: fast start, soft landing.
func easeOut(t float64) float64 { return 1 - (1-t)*(1-t) }

// flashTween is the reload flash: lines that were not in the review
// before the reload light up in the accent colour and fade back to
// their normal wash, so a reviewer sees what the agent just changed.
type flashTween struct {
	seen   map[string]struct{} // changed lines before the reload
	files  map[string]struct{} // files before the reload
	frames int
}

func flashKey(path string, l diff.Line) string {
	return path + "\x00" + string(rune('0'+l.Kind)) + "\x00" + l.Text
}

// startFlash remembers the changed lines of the files shown before a
// reload; whatever is changed afterwards and not in that set flashes.
func (m *Model) startFlash(before []diff.ReviewFile) {
	m.flash = nil
	if !m.animOn() {
		return
	}
	f := &flashTween{
		seen:  map[string]struct{}{},
		files: make(map[string]struct{}, len(before)),
	}
	for _, file := range before {
		f.files[file.Path] = struct{}{}
		for _, l := range file.Lines {
			if l.Kind != diff.Context {
				f.seen[flashKey(file.Path, l)] = struct{}{}
			}
		}
	}
	m.flash = f
}

// fresh reports whether a changed line is new since the reload.
func (f *flashTween) fresh(path string, l diff.Line) bool {
	if l.Kind == diff.Context {
		return false
	}
	if _, ok := f.files[path]; !ok {
		return false
	}
	_, seen := f.seen[flashKey(path, l)]
	return !seen
}

// dropIdleFlash ends the flash right away when the reload brought no
// new line, so a reload without news schedules no frames.
func (m *Model) dropIdleFlash() {
	if m.flash == nil {
		return
	}
	for _, f := range m.files {
		for _, l := range f.Lines {
			if m.flash.fresh(f.Path, l) {
				return
			}
		}
	}
	m.flash = nil
}

// flashWash is the wash of a changed line while the reload flash
// runs: the normal wash blended towards the accent colour. Lines that
// were already there, and files that are new, keep their wash.
func (m Model) flashWash(path string, l diff.Line, wash color.Color) color.Color {
	f := m.flash
	if f == nil || wash == nil || !f.fresh(path, l) {
		return wash
	}
	k := 0.7 * (1 - easeOut(float64(f.frames)/flashFrames))
	return theme.MixColor(wash, m.theme.Styles.Status.GetForeground(), k)
}

// themeFade is the cross-fade of :theme: the semantic styles blend
// from the old to the new theme; the syntax colours switch at the
// midpoint.
type themeFade struct {
	from, to theme.Theme
	frames   int
	swapped  bool
}

// startThemeFade makes th the theme and fades to it from the current
// one; without animations the theme switches at once.
func (m *Model) startThemeFade(th theme.Theme) {
	old := m.theme
	m.theme = th
	if !m.animOn() {
		m.rehighlight()
		return
	}
	m.theme.Styles, m.theme.Highlighter = old.Styles, old.Highlighter
	m.fade = &themeFade{from: old, to: th}
}

func (m *Model) rehighlight() {
	if len(m.files) > 0 {
		m.hl = newFileHighlights(m.theme.Highlighter, m.files[m.fileIdx])
	}
}

// stepFade advances the cross-fade one frame.
func (m *Model) stepFade() {
	f := m.fade
	f.frames++
	t := float64(f.frames) / fadeFrames
	if t >= 1 {
		m.theme = f.to
		m.fade = nil
		m.rehighlight()
		return
	}
	m.theme.Styles = theme.Mix(f.from.Styles, f.to.Styles, easeOut(t))
	if t >= 0.5 && !f.swapped {
		f.swapped = true
		m.theme.Highlighter = f.to.Highlighter
		m.rehighlight()
	}
}

// approveAccents are the stops of the approval gradient.
func (m Model) approveAccents() []color.Color {
	st := m.theme.Styles
	return []color.Color{
		st.Status.GetForeground(),
		st.CommentNit.GetForeground(),
		st.CommentQuestion.GetForeground(),
		st.CommentShould.GetForeground(),
		st.Status.GetForeground(),
	}
}

// gradientText colours s rune by rune along the stops; offset shifts
// the gradient, so a growing offset makes it travel.
func gradientText(s string, offset int, bold bool, stops []color.Color) string {
	runes := []rune(s)
	cols := lipgloss.Blend1D(max(len(runes), 2)*3, stops...)
	var b strings.Builder
	for i, r := range runes {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		c := cols[(i*3+offset)%len(cols)]
		b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(bold).Render(string(r)))
	}
	return b.String()
}

// lgtmBox is the confirmation moment of :lgtm: a "✓ LGTM" whose
// gradient shimmers inside a border that runs through the same
// colours.
func (m Model) lgtmBox() string {
	stops := m.approveAccents()
	body := strings.Join([]string{
		"",
		gradientText("✓ LGTM", m.shimmer, true, stops),
		"",
		m.theme.Styles.Fold.Render("review approved"),
	}, "\n")
	ring := append(lipgloss.Blend1D(24, stops...), lipgloss.Blend1D(24, reverse(stops)...)...)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForegroundBlend(ring...).
		BorderForegroundBlendOffset(m.shimmer).
		Padding(0, 1).
		Render(body)
}

func reverse(s []color.Color) []color.Color {
	r := make([]color.Color, len(s))
	for i, c := range s {
		r[len(s)-1-i] = c
	}
	return r
}
