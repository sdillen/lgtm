package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// liveReview is a review whose source changes while it runs: reload
// returns the files currently set in next, changes is the watcher's
// signal channel.
type liveReview struct {
	next    []diff.ReviewFile
	changes chan struct{}
	reloads int
}

func newLiveReview() *liveReview {
	return &liveReview{changes: make(chan struct{}, 1)}
}

func (l *liveReview) options(dir string) ui.Options {
	opts := withState(dir)
	opts.Changes = l.changes
	opts.Reload = func() ([]diff.ReviewFile, error) {
		l.reloads++
		return l.next, nil
	}
	return opts
}

// change makes the files of next the current state of the source and
// lets the model see the watcher signal: Init's wait for the signal
// yields a message, which the model answers with a check of the
// source, whose result the model takes in.
func (l *liveReview) change(m tea.Model, next ...diff.ReviewFile) tea.Model {
	l.next = next
	l.changes <- struct{}{}
	msg := m.Init()()
	m, cmd := m.Update(msg)
	m, _ = m.Update(cmd())
	return m
}

// mainV1 is a file with a comment-worthy line; mainV2 inserts two lines
// above it.
var (
	mainV1 = diff.New("main.go", "package main\n", "package main\n\nfunc a() {}\n\nfunc b() {}\n")
	mainV2 = diff.New("main.go", "package main\n", "package main\n\n// x\n// y\nfunc a() {}\n\nfunc b() {}\n")
)

// TestChangesOnlyShowAHint: while the review runs, a change of the
// source shows a hint in the status line, but the view stays as it was.
func TestChangesOnlyShowAHint(t *testing.T) {
	l := newLiveReview()
	other := diff.New("other.go", "", "package main\n")
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, l.options(t.TempDir())), 60, 10)

	m = l.change(m, mainV2, other)

	s := screen(m)
	if !strings.Contains(s, "↻ 2 Dateien geändert – R neu laden") {
		t.Errorf("status line misses the change hint:\n%s", s)
	}
	if strings.Contains(s, "// x") || strings.Contains(s, "other.go") {
		t.Errorf("view changed without R:\n%s", s)
	}
}

// TestRReloadsAndKeepsTheCursorOnItsCode: R shows the new state of the
// source, drops the hint and keeps the cursor on the code line it was
// on, even though lines were inserted above it.
func TestRReloadsAndKeepsTheCursorOnItsCode(t *testing.T) {
	dir := t.TempDir()
	l := newLiveReview()
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, l.options(dir)), 60, 12)
	m = press(m, "G") // func b() {}, line 5
	m = l.change(m, mainV2)

	m = press(m, "R")

	s := screen(m)
	if !strings.Contains(s, "// x") {
		t.Fatalf("R did not show the new state:\n%s", s)
	}
	if strings.Contains(s, "↻") {
		t.Errorf("hint still shown after R:\n%s", s)
	}
	// The cursor stays on func b(), now line 7: c comments it.
	m = press(m, "c")
	m = typeText(m, "note")
	pressMsg(m, ctrl("s"))
	got := readState(t, dir).Comments()
	if len(got) != 1 || got[0].Start != 7 || got[0].Quote != "func b() {}" {
		t.Errorf("comments after reload = %+v, want one on line 7 (func b)", got)
	}
}

// TestRKeepsCommentsOnTheirCode: a comment made before the reload sits
// on its code afterwards and is saved with its new anchor.
func TestRKeepsCommentsOnTheirCode(t *testing.T) {
	dir := t.TempDir()
	l := newLiveReview()
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, l.options(dir)), 60, 14)
	m = press(m, "G", "c")
	m = typeText(m, "rename b")
	m = pressMsg(m, ctrl("s"))
	m = l.change(m, mainV2)

	m = press(m, "R")

	got := readState(t, dir).Comments()
	if len(got) != 1 || got[0].Start != 7 || got[0].Orphaned {
		t.Fatalf("persisted comments = %+v, want one on line 7", got)
	}
	lines := strings.Split(screen(m), "\n")
	for i, line := range lines {
		if strings.Contains(line, "rename b") {
			if i == 0 || !strings.Contains(lines[i-1], "func b() {}") {
				t.Errorf("comment not below its code:\n%s", screen(m))
			}
			return
		}
	}
	t.Errorf("comment not shown after reload:\n%s", screen(m))
}

// TestOrphanedCommentsStayVisibleAndMarked: a comment whose code is gone
// after R stays in the review, shown at the top of its file with the
// verwaist marker, and the status line counts it.
func TestOrphanedCommentsStayVisibleAndMarked(t *testing.T) {
	dir := t.TempDir()
	l := newLiveReview()
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, l.options(dir)), 70, 14)
	m = press(m, "G", "c")
	m = typeText(m, "rename b")
	m = pressMsg(m, ctrl("s"))
	gone := diff.New("main.go", "package main\n", "package main\n\nfunc a() {}\n")
	m = l.change(m, gone)

	m = press(m, "R")
	if s := screen(m); !strings.Contains(s, "1 Kommentar verwaist") {
		t.Errorf("reload does not report the orphan:\n%s", s)
	}
	m = pressMsg(m, tea.KeyPressMsg{Code: tea.KeyEscape}) // clears the message

	s := screen(m)
	if !strings.Contains(s, "main.go:5 (verwaist)") || !strings.Contains(s, "rename b") {
		t.Errorf("orphaned comment not shown with its marker:\n%s", s)
	}
	if !strings.Contains(s, "1 verwaist") {
		t.Errorf("status line does not count the orphan:\n%s", s)
	}
	if got := readState(t, dir).Comments(); len(got) != 1 || !got[0].Orphaned {
		t.Errorf("persisted comments = %+v, want one orphaned", got)
	}
}

// TestOrphansOfAFileThatLeftTheReviewShowInTheFirstFile: when the file
// of a comment is no longer part of the review, its orphaned comments
// show at the top of the first file, like global comments.
func TestOrphansOfAFileThatLeftTheReviewShowInTheFirstFile(t *testing.T) {
	l := newLiveReview()
	other := diff.New("other.go", "", "package other\n")
	m := start(ui.New([]diff.ReviewFile{mainV1, other}, "main", themeFor, l.options(t.TempDir())), 70, 14)
	m = press(m, "]", "f", "c")
	m = typeText(m, "why other")
	m = pressMsg(m, ctrl("s"))
	m = l.change(m, mainV1)

	m = press(m, "R")

	if s := screen(m); !strings.Contains(s, "other.go:1 (verwaist)") || !strings.Contains(s, "why other") {
		t.Errorf("orphan of the gone file not shown in the first file:\n%s", s)
	}
}

// TestRWithoutReloadReportsIt: without a reload function R says so.
func TestRWithoutReloadReportsIt(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, withState(t.TempDir())), 60, 10)
	m = press(m, "R")
	if s := screen(m); !strings.Contains(s, "reload is not available") {
		t.Errorf("no message:\n%s", s)
	}
}

// TestACheckFromBeforeRIsDropped: a background check that started before
// R reports an older state of the source; it must not bring the hint
// back for the freshly loaded files.
func TestACheckFromBeforeRIsDropped(t *testing.T) {
	l := newLiveReview()
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, l.options(t.TempDir())), 60, 12)
	l.next = []diff.ReviewFile{mainV2}
	l.changes <- struct{}{}
	m, check := m.Update(m.Init()())
	stale := check() // resolved while the source was at mainV2

	l.next = []diff.ReviewFile{diff.New("main.go", "package main\n", "package main\n\n// z\n")}
	m = press(m, "R")
	m, _ = m.Update(stale)
	m = pressMsg(m, tea.KeyPressMsg{Code: tea.KeyEscape}) // clears the reload message

	if s := screen(m); strings.Contains(s, "↻") {
		t.Errorf("stale check brought the hint back:\n%s", s)
	}
}

// TestRKeepsHidingWhitespaceChanges: with :set ignorews the reloaded
// files hide whitespace-only changes as well.
func TestRKeepsHidingWhitespaceChanges(t *testing.T) {
	l := newLiveReview()
	m := start(ui.New([]diff.ReviewFile{mainV1}, "main", themeFor, l.options(t.TempDir())), 60, 12)
	m = press(m, ":")
	m = typeText(m, "set ignorews")
	m = pressMsg(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	l.next = []diff.ReviewFile{diff.New("main.go", "package main\n\nfunc a() {}\n", "package main\n\nfunc a()  {}\n")}

	m = press(m, "R")
	m = pressMsg(m, tea.KeyPressMsg{Code: tea.KeyEscape}) // clears the reload message

	if s := screen(m); !strings.Contains(s, "+0 −0") {
		t.Errorf("reloaded file shows the whitespace change:\n%s", s)
	}
}
