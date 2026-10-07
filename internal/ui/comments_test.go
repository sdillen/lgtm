package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// tab presses <Tab>.
func tab(m tea.Model) tea.Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return m
}

// rowContaining returns the rendered row (with styles) whose text
// contains s, "" if there is none.
func rowContaining(m tea.Model, s string) string {
	for _, row := range strings.Split(m.View().Content, "\n") {
		if strings.Contains(ansi.Strip(row), s) {
			return row
		}
	}
	return ""
}

// TestCOpensTheFloatAndCtrlSSavesTheComment pins the core of the review
// loop: c opens the comment float on the cursor line, typed text goes
// into it and <C-s> stores the comment with its anchor, the quoted code
// and the default category, persisted right away.
func TestCOpensTheFloatAndCtrlSSavesTheComment(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = press(m, "j", "j", "c")
	if s := screen(m); !strings.Contains(s, "should") || !strings.Contains(s, "main.go:2") {
		t.Fatalf("float does not show category and location:\n%s", s)
	}
	m = typeText(m, "rename this")
	if s := screen(m); !strings.Contains(s, "rename this") {
		t.Fatalf("float does not show the typed text:\n%s", s)
	}
	m = pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	c := got[0]
	if c.Path != "main.go" || c.Start != 2 || c.End != 2 || c.Quote != "new" ||
		c.Text != "rename this" || c.Category != review.Should {
		t.Errorf("comment = %+v", c)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lgtm", "review.md")); err != nil {
		t.Errorf("review.md not written: %v", err)
	}
	if s := screen(m); strings.Contains(s, "<C-s>") {
		t.Errorf("float still open after saving:\n%s", s)
	}
}

// TestSavedCommentsAppearInlineInTheirCategoryColour pins that a saved
// comment shows as a virtual row under its last line, in the colour of
// its category, and that the status line counts the comments.
func TestSavedCommentsAppearInlineInTheirCategoryColour(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(t.TempDir())), 50, 7)

	m = press(m, "j", "j", "c")
	m = tab(m) // should → must
	m = typeText(m, "rename this")
	m = pressMsg(m, ctrl("s"))

	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"        2 + new",
		"            ▎ must: rename this",
		"   3    3   end",
		"",
		"main.go  1/1  +1 −1  1 comment",
	})
	// must is #f7768e in tokyo night night.
	if row := rowContaining(m, "rename this"); !strings.Contains(row, "38;2;247;118;142") {
		t.Errorf("comment row lacks the must colour: %q", row)
	}
}

// TestVSelectsARangeAndCCommentsIt pins range comments: V starts a line
// selection, motions extend it, and c anchors the comment to the new-file
// lines it covers, quoting them. Deleted lines inside the selection are
// not part of the new-file quote.
func TestVSelectsARangeAndCCommentsIt(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = press(m, "V")
	if s := screen(m); !strings.Contains(s, "-- VISUAL --") {
		t.Fatalf("status line does not show the selection:\n%s", s)
	}
	m = press(m, "j", "j", "c")
	if s := screen(m); !strings.Contains(s, "main.go:1-2") {
		t.Fatalf("float does not show the range:\n%s", s)
	}
	m = typeText(m, "block")
	m = pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	if c := got[0]; c.Start != 1 || c.End != 2 || c.Quote != "package main\nnew" {
		t.Errorf("comment = %+v", c)
	}
	if s := screen(m); strings.Contains(s, "-- VISUAL --") {
		t.Errorf("selection still active after saving:\n%s", s)
	}
}

// TestEscCancelsTheSelection pins that <Esc> leaves the line selection
// without commenting.
func TestEscCancelsTheSelection(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 12)

	m = esc(press(m, "V", "j"))

	if s := screen(m); strings.Contains(s, "-- VISUAL --") {
		t.Errorf("selection still active after <Esc>:\n%s", s)
	}
}

// TestEscEscDiscardsTheDraft pins that <Esc><Esc> closes the float
// without storing anything; a single <Esc> only leaves insert mode.
func TestEscEscDiscardsTheDraft(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = typeText(press(m, "c"), "never mind")
	m = esc(m)
	if s := screen(m); !strings.Contains(s, "never mind") || !strings.Contains(s, "NORMAL") {
		t.Fatalf("one <Esc> should only leave insert mode:\n%s", s)
	}
	m = esc(m)

	if s := screen(m); strings.Contains(s, "never mind") {
		t.Errorf("float still open after <Esc><Esc>:\n%s", s)
	}
	if n := len(readState(t, dir).Comments()); n != 0 {
		t.Errorf("discarded draft stored %d comments", n)
	}
}

// TestTabCyclesTheCategoryInTheFloat pins the <Tab> order in the float:
// should → must → nit → question → should.
func TestTabCyclesTheCategoryInTheFloat(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 12)
	m = press(m, "c")

	for _, want := range []string{"should", "must", "nit", "question", "should"} {
		if title := floatTitle(m); !strings.HasPrefix(title, want+" ") {
			t.Errorf("float title = %q, want category %q", title, want)
		}
		m = tab(m)
	}
}

// TestCommentsSurviveARestart pins that comments are loaded from
// .lgtm/state.json and shown inline again in a new session.
func TestCommentsSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)
	m = typeText(press(m, "c"), "keep me")
	pressMsg(m, ctrl("s"))

	restarted := start(ui.New([]diff.ReviewFile{file}, "main", themeFor,
		ui.Options{State: readState(t, dir), Dir: dir}), 60, 12)

	if s := screen(restarted); !strings.Contains(s, "▎ should: keep me") || !strings.Contains(s, "1 comment") {
		t.Errorf("comment lost after restart:\n%s", s)
	}
}

// TestDeletedLinesCannotBeCommentedYet pins that a comment needs a line
// of the new file: c on a deleted line reports instead of anchoring
// somewhere else.
func TestDeletedLinesCannotBeCommentedYet(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 12)

	m = press(m, "j", "c")

	if s := screen(m); strings.Contains(s, "<C-s>") || !strings.Contains(s, "deleted lines") {
		t.Errorf("c on a deleted line opened the float or gave no hint:\n%s", s)
	}
}

// TestFloatEditsWithMinimalVimBindings pins the normal mode bindings of
// the float: 0 and $ move within the line, x deletes, i/a/A re-enter
// insert mode, <CR> starts a new line.
func TestFloatEditsWithMinimalVimBindings(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = typeText(press(m, "c"), "xhello")
	m = esc(m)
	m = press(m, "0", "x", "A")
	m = typeText(m, " world")
	m, _ = enter(m)
	m = typeText(m, "second")
	m = esc(m)
	m = press(m, "0", "i")
	m = typeText(m, "a ")
	pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 || got[0].Text != "hello world\na second" {
		t.Errorf("comments = %+v, want text %q", got, "hello world\na second")
	}
}

// floatTitle returns the text of the float's title row: the first row
// inside the box border.
func floatTitle(m tea.Model) string {
	for _, row := range strings.Split(screen(m), "\n") {
		if i := strings.Index(row, "│ "); i >= 0 {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(row[i+len("│ "):]), "│"))
		}
	}
	return ""
}

// TestLongCommentsWrapInline pins that the inline comment rows wrap at
// the view width instead of cutting the text off.
func TestLongCommentsWrapInline(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 10)

	m = typeText(press(m, "c"), "one two three four five six seven")
	m = pressMsg(m, ctrl("s"))

	assertScreen(t, m, []string{
		"   1    1   package main",
		"            ▎ should: one two three four",
		"            ▎ five six seven",
		"   2      - old",
		"        2 + new",
		"   3    3   end",
		"",
		"",
		"",
		"main.go  1/1  +1 −1  1 comment",
	})
}

// TestCOnAFoldMarkerAsksForACodeLine pins that c needs a visible code
// line: on a fold marker it reports instead of commenting a hidden line.
func TestCOnAFoldMarkerAsksForACodeLine(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 60, 12)

	m = press(m, "c") // the cursor starts on the marker of the leading fold

	if s := screen(m); strings.Contains(s, "<C-s>") || !strings.Contains(s, "code line") {
		t.Errorf("c on a fold marker opened the float or gave no hint:\n%s", s)
	}
}

// TestZoOnACommentInAClosedFoldOpensTheFold pins that a comment row shown
// below a closed fold belongs to that fold: zo there opens it.
func TestZoOnACommentInAClosedFoldOpensTheFold(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 60, 20)
	// Comment line 5 (inside the middle fold), then close all folds.
	m = press(m, "j", "j", "j", "z", "o", "j", "j")
	m = typeText(press(m, "c"), "note")
	m = pressMsg(m, ctrl("s"))
	m = press(m, "z", "M")
	if s := screen(m); !strings.Contains(s, "⋯ 7 unveränderte Zeilen ⋯\n            ▎ should: note") {
		t.Fatalf("comment not shown below the closed fold:\n%s", s)
	}

	m = press(m, "g", "g", "j", "j", "j", "j", "z", "o")

	if s := screen(m); strings.Contains(s, "⋯ 7 unveränderte Zeilen ⋯") {
		t.Errorf("zo on the comment row did not open its fold:\n%s", s)
	}
}
