package ui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// benchBig is a 10k-line review file, every line a change, so the view
// has one row per line and the syntax highlighter has to work for every
// visible row.
func benchBig(n int) diff.ReviewFile {
	lines := make([]diff.Line, n)
	for i := range lines {
		lines[i] = diff.Line{Kind: diff.Added, New: i + 1, Text: fmt.Sprintf("line %d := value + %d // note", i, i%7)}
	}
	return diff.ReviewFile{Path: "bench.go", Lines: lines}
}

// benchMinified is one very long line of dense code, like a bundled or
// minified asset: the lexer produces thousands of segments for a single
// row.
func benchMinified() diff.ReviewFile {
	text := strings.Repeat("return a+b*c-d/e;", 2000)
	return diff.ReviewFile{Path: "bundle.js", Lines: []diff.Line{
		{Kind: diff.Added, New: 1, Text: text},
	}}
}

func benchStart(b *testing.B, f diff.ReviewFile) tea.Model {
	b.Helper()
	m := ui.New([]diff.ReviewFile{f}, "bench", themeFor, ui.Options{})
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m2
}

func benchPress(b *testing.B, m tea.Model, keys ...string) tea.Model {
	b.Helper()
	for _, k := range keys {
		m, _ = m.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
	}
	return m
}

func BenchmarkRenderFrame(b *testing.B) {
	big := benchBig(10000)
	minified := benchMinified()
	cases := []struct {
		name string
		file diff.ReviewFile
	}{
		{name: "10kLinesTop", file: big},
		{name: "10kLinesEnd", file: big},
		{name: "Minified", file: minified},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			m := benchStart(b, c.file)
			if c.name == "10kLinesEnd" {
				// Warm the cache at the end of the file, so the
				// benchmark measures the steady-state frame, not
				// the jump.
				m = benchPress(b, m, "G")
			}
			m.View()
			b.ResetTimer()
			for b.Loop() {
				m.View()
			}
		})
	}
}

func BenchmarkScroll(b *testing.B) {
	big := benchBig(10000)
	minified := benchMinified()
	cases := []struct {
		name string
		file diff.ReviewFile
	}{
		{name: "10kLines", file: big},
		{name: "Minified", file: minified},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			m := benchStart(b, c.file)
			m.View()
			b.ResetTimer()
			for b.Loop() {
				m = benchPress(b, m, "j")
				m.View()
			}
		})
	}
}
