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
