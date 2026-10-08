package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// themeFor is the fixed theme of the highlighting tests, so the colours
// are deterministic.
var themeFor = theme.Default(theme.KindDark)

// bigFile is a review file of n added lines, so the view has no folds
// and one row per line.
func bigFile(n int) diff.ReviewFile {
	lines := make([]diff.Line, n)
	for i := range lines {
		lines[i] = diff.Line{Kind: diff.Added, New: i + 1, Text: fmt.Sprintf("line %d", i)}
	}
	return diff.ReviewFile{Path: "big.go", Lines: lines}
}

func startM(m Model, width, height int) Model {
	m2, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m2.(Model)
}

func pressM(m Model, keys ...string) Model {
	for _, k := range keys {
		m2, _ := m.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
		m = m2.(Model)
	}
	return m
}

// TestHighlightingIsLazyForTheVisibleArea pins the core of the
// performance story: rendering the first frame of a huge file highlights
// only the visible rows plus the prefetch buffer, not the whole file.
func TestHighlightingIsLazyForTheVisibleArea(t *testing.T) {
	m := startM(New([]diff.ReviewFile{bigFile(10000)}, "big", themeFor, Options{}), 100, 40)
	m.View()

	if got := m.hl.newDone; got == 10000 {
		t.Fatalf("first frame highlighted all %d lines, want only the visible area plus buffer", got)
	}
	if got := m.hl.newDone; got > m.bodyHeight()+2*highlightBuffer {
		t.Errorf("first frame highlighted %d new-file lines, want at most %d", got, m.bodyHeight()+2*highlightBuffer)
	}
	if got := m.hl.newDone; got == 0 {
		t.Error("first frame highlighted nothing, want the visible rows highlighted")
	}
}

// TestHighlightingFollowsTheCursor pins that scrolling to the end of a
// huge file highlights the lines around the new position, and that the
// rendered frame shows the lines of the end of the file.
func TestHighlightingFollowsTheCursor(t *testing.T) {
	m := startM(New([]diff.ReviewFile{bigFile(10000)}, "big", themeFor, Options{}), 100, 40)
	m.View()
	first := m.hl.newDone

	m = pressM(m, "G")
	m.View()

	if got := m.hl.newDone; got <= first {
		t.Errorf("after G highlighted %d new-file lines, want more than the first frame's %d", got, first)
	}
	if got := m.hl.newDone; got < 9900 {
		t.Errorf("after G highlighted %d new-file lines, want the end of the file highlighted", got)
	}
	if got := m.hl.newDone; got > 10000 {
		t.Errorf("highlighted %d new-file lines, more than the file has", got)
	}

	content := ansi.Strip(m.View().Content)
	if !strings.Contains(content, "line 9999") {
		t.Errorf("view after G lacks the last line:\n%s", content)
	}
}

// TestDeletedLineHighlightingIsLazy pins that dimmed deleted lines are
// only lexed when they become visible: the deleted line at the end of a
// huge file stays unlexed while the view is at the top.
func TestDeletedLineHighlightingIsLazy(t *testing.T) {
	lines := make([]diff.Line, 10000)
	lines[0] = diff.Line{Kind: diff.Deleted, Old: 1, Text: "gone"}
	for i := 1; i < 9999; i++ {
		lines[i] = diff.Line{Kind: diff.Context, Old: i + 1, New: i, Text: fmt.Sprintf("line %d", i)}
	}
	lines[9999] = diff.Line{Kind: diff.Deleted, Old: 10000, Text: "gone"}
	f := diff.ReviewFile{Path: "big.go", Lines: lines}

	m := startM(New([]diff.ReviewFile{f}, "big", themeFor, Options{}), 100, 40)
	m.View()

	if got := len(m.hl.dim); got != 1 {
		t.Errorf("lexed %d dimmed deleted lines, want only the visible one", got)
	}
}

// The line cache must never change what is drawn: every frame of a
// session of scrolling, cursor and column moves equals the same frame
// drawn without the cache.
func TestLineCacheMatchesUncachedRendering(t *testing.T) {
	f := bigFile(60)
	for i := range f.Lines {
		f.Lines[i].Text = fmt.Sprintf("\tfunc f%d() { return a + b } // note", i)
		if i%3 == 0 {
			f.Lines[i].Old = i + 1
			f.Lines[i].Words = []diff.Word{{Start: 6, End: 10}}
		}
	}
	m := startM(New([]diff.ReviewFile{f}, "main", themeFor, Options{}), 50, 12)
	for _, k := range []string{"j", "j", "l", "l", "k", "j", "j", "j", "$", "0", "j", "k"} {
		next, _ := m.Update(tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
		m = next.(Model)
		for i := range m.bodyHeight() {
			j := m.offset + i
			rows := m.rows()
			if j >= len(rows) || rows[j].line < 0 {
				continue
			}
			idx := rows[j].line
			cursor := j == m.cursor
			l := m.files[m.fileIdx].Lines[idx]
			mark := -1
			if cursor && idx == m.cursorViewLine() {
				mark = colMark(l.Text, m.col)
			}
			// twice: the second call is served from the cache
			for range 2 {
				if got, want := m.renderLine(l, idx, cursor), m.drawLine(l, idx, cursor, mark); got != want {
					t.Fatalf("key %q line %d: cached %q, uncached %q", k, idx, got, want)
				}
			}
		}
	}
}

// The screen row cache must never change a frame: every frame of a
// session of scrolling, cursor, column and horizontal scroll moves,
// minimap toggles and resizes equals the same frame drawn with empty
// caches.
func TestScreenCacheMatchesUncachedFrames(t *testing.T) {
	f := bigFile(60)
	for i := range f.Lines {
		f.Lines[i].Text = fmt.Sprintf("\tfunc f%d() { return a + b } // a note that runs past the width", i)
		if i%3 == 0 {
			f.Lines[i].Old = i + 1
			f.Lines[i].Words = []diff.Word{{Start: 6, End: 10}}
		}
	}
	m := startM(New([]diff.ReviewFile{f}, "main", themeFor, Options{}), 50, 12)
	steps := []tea.Msg{}
	for _, k := range []string{"j", "j", "l", "z", "l", "j", "m", "k", "$", "z", "h", "0", "m", "j"} {
		steps = append(steps, tea.KeyPressMsg{Code: []rune(k)[0], Text: k})
	}
	steps = append(steps, tea.WindowSizeMsg{Width: 40, Height: 10}, tea.KeyPressMsg{Code: 'j', Text: "j"})
	for i, msg := range steps {
		next, _ := m.Update(msg)
		m = next.(Model)
		m.View() // warm the caches
		warm := m.View().Content
		m.hl.rendered, m.hl.screens = nil, nil
		if cold := m.View().Content; warm != cold {
			t.Fatalf("step %d (%v): cached frame\n%q\nuncached\n%q", i, msg, warm, cold)
		}
	}
}
