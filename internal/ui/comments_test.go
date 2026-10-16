package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/review"
	"github.com/sdillen/lgtm/internal/ui"
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
		"main.go  1/1  +1 −1   1 must",
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

	if s := screen(restarted); !strings.Contains(s, "▎ should: keep me") || !strings.Contains(s, "● 1 should") {
		t.Errorf("comment lost after restart:\n%s", s)
	}
}

// TestCCommentsADeletedLine pins deleted-line comments: c on a deleted
// line opens the float anchored in the old state, the title names the
// old line with the (alt, entfernt) marker, and saving stores the old
// anchor with the deleted line as quote, shown inline below the line.
func TestCCommentsADeletedLine(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = press(m, "j", "c")
	if s := screen(m); !strings.Contains(s, "main.go:2 (alt, entfernt)") {
		t.Fatalf("float does not show the old anchor:\n%s", s)
	}
	m = typeText(m, "keep this line")
	m = pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	c := got[0]
	if !c.Old || c.Start != 2 || c.End != 2 || c.Quote != "old" || c.Text != "keep this line" {
		t.Errorf("comment = %+v, want the old anchor on line 2", c)
	}
	if s := screen(m); !strings.Contains(s, "▎ should: keep this line") {
		t.Errorf("comment not shown inline:\n%s", s)
	}
}

// TestVSelectsDeletedLinesAndCommentsThem pins range comments on the
// old side: a selection of deleted lines only anchors in the old state
// and quotes the removed code.
func TestVSelectsDeletedLinesAndCommentsThem(t *testing.T) {
	dir := t.TempDir()
	f := diff.New("gone.go", "a\nb\nc\nd\n", "a\nd\n")
	m := start(ui.New([]diff.ReviewFile{f}, "main", themeFor, withState(dir)), 60, 12)

	m = press(m, "j", "V", "j", "c")
	if s := screen(m); !strings.Contains(s, "gone.go:2-3 (alt, entfernt)") {
		t.Fatalf("float does not show the deleted range:\n%s", s)
	}
	m = typeText(m, "keep both")
	pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	if c := got[0]; !c.Old || c.Start != 2 || c.End != 3 || c.Quote != "b\nc" {
		t.Errorf("comment = %+v", c)
	}
}

// TestCCommentsAContextLine pins context comments: unchanged code can be
// commented like changed lines, with its new line number.
func TestCCommentsAContextLine(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = press(m, "c")
	if s := screen(m); !strings.Contains(s, "main.go:1") {
		t.Fatalf("float does not show the context anchor:\n%s", s)
	}
	m = typeText(m, "use the existing helper")
	pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	if c := got[0]; c.Start != 1 || c.End != 1 || c.Quote != "package main" || c.Old {
		t.Errorf("comment = %+v, want the context line 1", c)
	}
}

// TestCCommentsTheWholeFile pins file comments: C opens the float
// without a line anchor, the title names the file, and the saved
// comment covers the whole file without a quote, shown below the first
// line.
func TestCCommentsTheWholeFile(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)

	m = press(m, "C")
	if s := screen(m); !strings.Contains(s, "main.go (Datei)") {
		t.Fatalf("float does not show the file anchor:\n%s", s)
	}
	m = typeText(m, "split this file")
	m = pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	if c := got[0]; c.Path != "main.go" || c.Start != 0 || c.End != 0 || c.Quote != "" {
		t.Errorf("comment = %+v, want a file comment", c)
	}
	if s := screen(m); !strings.Contains(s, "▎ should: split this file") {
		t.Errorf("file comment not shown inline:\n%s", s)
	}
}

// TestLeaderCCommentsTheWholeReview pins global comments: <leader>c
// opens the float without a file, the title says global, and the saved
// comment has no path; it is shown at the top of the first file only.
func TestLeaderCCommentsTheWholeReview(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file, small}, "main", themeFor, withState(dir)), 60, 12)

	m = press(leader(m), "c")
	if s := screen(m); !strings.Contains(s, "global") {
		t.Fatalf("float does not show the global anchor:\n%s", s)
	}
	m = typeText(m, "nice overall")
	m = pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("persisted %d comments, want 1", len(got))
	}
	if c := got[0]; c.Path != "" || c.Start != 0 || c.End != 0 {
		t.Errorf("comment = %+v, want a global comment", c)
	}
	if s := screen(m); !strings.Contains(s, "▎ should: nice overall") {
		t.Errorf("global comment not shown inline:\n%s", s)
	}

	// The second file does not repeat the global comment.
	m = press(m, "]", "f")
	if s := screen(m); strings.Contains(s, "nice overall") {
		t.Errorf("global comment shown in the second file:\n%s", s)
	}
}

// TestCOnACommentEditsIt pins the edit flow: c on a comment row opens
// the float with the comment's text; saving replaces the text and
// category but keeps the anchor and the id.
func TestCOnACommentEditsIt(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)
	m = press(m, "j", "j", "c")
	m = typeText(m, "rename this")
	m = pressMsg(m, ctrl("s"))
	id := readState(t, dir).Comments()[0].ID

	// The note row sits below the added line.
	m = press(m, "j", "c")
	if s := screen(m); !strings.Contains(s, "rename this") {
		t.Fatalf("float does not show the existing text:\n%s", s)
	}
	m = tab(m)
	m = typeText(m, " or reword")
	pressMsg(m, ctrl("s"))

	got := readState(t, dir).Comments()
	if len(got) != 1 {
		t.Fatalf("edit stored %d comments, want 1", len(got))
	}
	c := got[0]
	if c.ID != id || c.Text != "rename this or reword" || c.Category != review.Must ||
		c.Start != 2 || c.End != 2 || c.Quote != "new" {
		t.Errorf("edited comment = %+v", c)
	}
}

// TestDDDeletesACommentAfterConfirmation pins the delete flow: the
// first dd only asks for confirmation, other keys disarm it, and the
// next dd removes the comment.
func TestDDDeletesACommentAfterConfirmation(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)
	m = press(m, "j", "j", "c")
	m = typeText(m, "remove me")
	m = pressMsg(m, ctrl("s"))
	m = press(m, "j") // onto the note row

	m = press(m, "d", "d")
	if s := screen(m); !strings.Contains(s, "dd again") {
		t.Fatalf("first dd does not ask for confirmation:\n%s", s)
	}
	if n := len(readState(t, dir).Comments()); n != 1 {
		t.Fatalf("first dd removed the comment already")
	}

	// Other keys disarm the confirmation.
	m = press(m, "j", "k", "d", "d")
	if n := len(readState(t, dir).Comments()); n != 1 {
		t.Fatalf("dd after other keys removed the comment")
	}
	if s := screen(m); !strings.Contains(s, "dd again") {
		t.Errorf("dd after other keys does not ask again:\n%s", s)
	}

	m = press(m, "d", "d")
	if n := len(readState(t, dir).Comments()); n != 0 {
		t.Errorf("%d comments after the confirmed dd, want 0", n)
	}
	if s := screen(m); strings.Contains(s, "▎ should: remove me") {
		t.Errorf("deleted comment still shown inline:\n%s", s)
	}
}

// TestXMarksACommentAsDone pins the done marking: x sets the status, the
// comment stays inline but dimmed, and review.md no longer lists it.
func TestXMarksACommentAsDone(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)
	m = press(m, "j", "j", "c")
	m = typeText(m, "fix this")
	m = pressMsg(m, ctrl("s"))

	m = press(m, "j", "x")
	got := readState(t, dir).Comments()
	if len(got) != 1 || got[0].Status != review.Done {
		t.Fatalf("comment status = %+v, want done", got)
	}
	md, err := os.ReadFile(filepath.Join(dir, ".lgtm", "review.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(md), "fix this") {
		t.Errorf("review.md still lists the done comment:\n%s", md)
	}
	row := rowContaining(m, "fix this")
	if row == "" {
		t.Fatalf("done comment no longer shown inline:\n%s", screen(m))
	}
	// The dim fold colour #51597d replaces the should colour #e0af68.
	if !strings.Contains(row, "38;2;81;89;125") || strings.Contains(row, "38;2;224;175;104") {
		t.Errorf("done comment not dimmed: %q", row)
	}
}

// TestNextCommentJumpsAcrossFiles pins ]k/[k: the keys jump to the next
// and previous comment, also into the next file, and land on the
// comment row.
func TestNextCommentJumpsAcrossFiles(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file, small}, "main", themeFor, ui.Options{}), 60, 12)
	m = press(m, "j", "j", "c")
	m = typeText(m, "one")
	m = pressMsg(m, ctrl("s"))
	m = press(m, "]", "f")
	m = press(m, "j", "c")
	m = typeText(m, "two")
	m = pressMsg(m, ctrl("s"))

	m = press(m, "g", "g", "[", "k")
	if s := screen(m); !strings.Contains(s, "main.go  1/2") || !strings.Contains(s, "▎ should: one") {
		t.Fatalf("[k does not jump back to the comment of the previous file:\n%s", s)
	}
	m = press(m, "]", "k")
	if s := screen(m); !strings.Contains(s, "small.txt  2/2") || !strings.Contains(s, "▎ should: two") {
		t.Errorf("]k does not jump to the comment of the next file:\n%s", s)
	}
}

// TestDDAndXNeedACommentUnderTheCursor pins the guard of the comment
// maintenance keys: dd and x on a code line report instead of acting.
func TestDDAndXNeedACommentUnderTheCursor(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 60, 12)

	m = press(m, "d", "d")
	if s := screen(m); !strings.Contains(s, "move the cursor to a comment to delete") {
		t.Errorf("dd on a code line gives no hint:\n%s", s)
	}
	m = press(m, "x")
	if s := screen(m); !strings.Contains(s, "move the cursor to a comment to mark as done") {
		t.Errorf("x on a code line gives no hint:\n%s", s)
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
	m := startNoMinimap(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 10)

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
		"main.go  1/1  +1 −1  ● 1 should",
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
	// The rows end in the minimap column; trim it before matching.
	trimmed := trimRows(screen(m))
	if !strings.Contains(trimmed, "⋯ 7 unveränderte Zeilen ⋯\n            ▎ should: note") {
		t.Fatalf("comment not shown below the closed fold:\n%s", screen(m))
	}

	m = press(m, "g", "g", "j", "j", "j", "j", "z", "o")

	if s := screen(m); strings.Contains(s, "⋯ 7 unveränderte Zeilen ⋯") {
		t.Errorf("zo on the comment row did not open its fold:\n%s", s)
	}
}
