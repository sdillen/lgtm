package ui_test

import (
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// longLine is a file with one very long added line, for the wrap and
// horizontal scroll tests.
var longLine = diff.New("long.txt", "", strings.Repeat("x", 60)+"\n")

// TestLongLinesDoNotWrapByDefault pins the acceptance criterion "lange
// Zeilen standardmäßig nicht umbrechen": the line is cut at the window
// edge and does not fill the rows below.
func TestLongLinesDoNotWrapByDefault(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{longLine}, "main", themeFor, ui.Options{}), 20, 2)

	assertScreen(t, m, []string{
		"        1 + xxxxxxxx", // gutter 9 + marker 3 + 8 cells = 20
		"long.txt  1/1  +1 −0",
	})
}

// TestZLAndZHScrollHorizontally pins the acceptance criterion
// "horizontal mit zl/zh scrollen": zl reveals the end of the line, zh
// scrolls back.
func TestZLAndZHScrollHorizontally(t *testing.T) {
	line := diff.New("long.txt", "", "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ\n")
	m := start(ui.New([]diff.ReviewFile{line}, "main", themeFor, ui.Options{}), 20, 2)

	got := screen(m)
	if !strings.Contains(got, "abcdefgh") {
		t.Fatalf("start shows the left side:\n%s", got)
	}

	// zl scrolls right by half a window; six scrolls reach the end.
	m = press(m, "z", "l", "z", "l", "z", "l", "z", "l", "z", "l", "z", "l")
	if got := screen(m); !strings.Contains(got, "WXYZ") {
		t.Errorf("zl did not reveal the end of the line:\n%s", got)
	}

	m = press(m, "z", "h", "z", "h", "z", "h", "z", "h", "z", "h", "z", "h")
	if got := screen(m); !strings.Contains(got, "abcdefgh") {
		t.Errorf("zh did not scroll back to the start:\n%s", got)
	}
}

// TestSetWrapWrapsLongLines pins the acceptance criterion ":set wrap
// schaltet Softwrap ein": with wrap the long line fills the rows below
// instead of being cut.
func TestSetWrapWrapsLongLines(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{longLine}, "main", themeFor, ui.Options{}), 40, 4)

	m, _ = enter(typeText(press(m, ":"), "set wrap"))
	assertScreen(t, m, []string{
		"        1 + xxxxxxxxxxxxxxxxxxxxxxxxxxxx", // 12 cells of gutter and marker + 28 x's
		"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",         // 32 cells left of the 72-cell row
		"long.txt  1/1  +1 −0  [wrap]",
	})

	m, _ = enter(typeText(press(m, ":"), "set nowrap"))
	assertScreen(t, m, []string{
		"        1 + xxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		"",
		"",
		"long.txt  1/1  +1 −0",
	})
}

// TestWrapCutsExtremelyLongLines pins that even with wrap on a single
// line cannot flood the view: after a cap it is cut with "…".
func TestWrapCutsExtremelyLongLines(t *testing.T) {
	huge := diff.New("huge.txt", "", strings.Repeat("x", 100000)+"\n")
	m := start(ui.New([]diff.ReviewFile{huge}, "main", themeFor, ui.Options{}), 20, 5)

	m, _ = enter(typeText(press(m, ":"), "set wrap"))
	rows := strings.Split(screen(m), "\n")
	// The status line shows the marker; the body above is capped.
	body := rows[:len(rows)-1]
	if len(body) != 4 {
		t.Fatalf("wrapped body has %d rows, want 4 capped rows", len(body))
	}
	if !strings.Contains(screen(m), "…") {
		t.Errorf("extremely long line is not cut with …:\n%s", screen(m))
	}
}

// TestSetIgnoreWSHidesWhitespaceOnlyChanges pins the acceptance
// criterion ":set ignorews blendet Whitespace-only-Änderungen aus": the
// changed pair becomes a plain context line, and the toggle restores it.
func TestSetIgnoreWSHidesWhitespaceOnlyChanges(t *testing.T) {
	ws := diff.New("f.txt", "a b\n", "a  b\n")
	m := start(ui.New([]diff.ReviewFile{ws}, "main", themeFor, ui.Options{}), 30, 2)

	// Without the option the whitespace change is a deleted/added pair.
	assertScreen(t, m, []string{
		"   1      - a b",
		"f.txt  1/1  +1 −1",
	})

	m, _ = enter(typeText(press(m, ":"), "set ignorews"))
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"f.txt  1/1  +0 −0  [ignorews]",
	})

	// The hidden pair is a context line: zo shows the new text.
	m = press(m, "z", "o")
	assertScreen(t, m, []string{
		"   1    1   a  b",
		"f.txt  1/1  +0 −0  [ignorews]",
	})

	m, _ = enter(typeText(press(m, ":"), "set noignorews"))
	assertScreen(t, m, []string{
		"   1      - a b",
		"f.txt  1/1  +1 −1",
	})
}

// TestSetIgnoreWSKeepsRealChanges pins that the option hides only
// whitespace: a real edit stays visible.
func TestSetIgnoreWSKeepsRealChanges(t *testing.T) {
	f := diff.New("f.txt", "a b\nx\n", "a  b\ny\n")
	m := start(ui.New([]diff.ReviewFile{f}, "main", themeFor, ui.Options{}), 30, 4)

	m, _ = enter(typeText(press(m, ":"), "set ignorews"))
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯", // the hidden pair is a context run
		"   2      - x",
		"        2 + y",
		"f.txt  1/1  +1 −1  [ignorews]",
	})
}

// TestSetReportsTheNewOptions pins that :set without arguments names
// the wrap and ignorews options next to the existing ones.
func TestSetReportsTheNewOptions(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{small}, "main", themeFor, ui.Options{}), 50, 5)

	m, _ = enter(typeText(press(m, ":"), "set wrap"))
	m, _ = enter(typeText(press(m, ":"), "set"))
	if got := screen(m); !strings.Contains(got, "wrap noignorews") {
		t.Errorf(":set did not report the options:\n%s", got)
	}
}
