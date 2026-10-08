package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// numbered builds a file of n unique lines, so a test can find a
// specific line in the rendered view.
func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %03d\n", i)
	}
	return b.String()
}

// withLines builds a file of n unique lines with the given lines
// replaced, for fixtures with changes at known positions.
func withLines(n int, replace map[int]string) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		if r, ok := replace[i]; ok {
			b.WriteString(r)
		} else {
			fmt.Fprintf(&b, "line %03d\n", i)
		}
	}
	return b.String()
}

// minimapFile is a 100-line file with an added line in the middle (51)
// and near the end (96), for the marks of the minimap strip. At the
// size of the tests (80x11, ten body rows) each strip cell covers ten
// file rows: cell 5 covers lines 051-060, cell 9 lines 091-100.
var minimapFile = diff.New("map.txt", numbered(100),
	withLines(100, map[int]string{51: "added mid\n", 96: "added low\n"}))

// TestMinimapMarksChangesAndComments pins the acceptance criterion
// "Minimap-Scrollbar zeigt Änderungen und Kommentare": the strip cells
// covering the changed lines carry the added wash, the cell of a
// comment row the category colour. The strip is one column at the
// right edge of the file area.
func TestMinimapMarksChangesAndComments(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{minimapFile}, "main", themeFor, ui.Options{}), 80, 11)

	// Unfold everything, so the strip maps the whole file.
	m = press(m, "z", "R")

	// The added marks sit on the cells covering lines 051 and 096:
	// screen rows 5 and 9.
	if !endsWithCell(m.View().Content, "line 006", "48;2;31;44;56") {
		t.Errorf("minimap lacks the added mark of line 051:\n%s", screen(m))
	}
	if !endsWithCell(m.View().Content, "line 010", "48;2;31;44;56") {
		t.Errorf("minimap lacks the added mark of line 096:\n%s", screen(m))
	}

	// Comment context line 030 and jump back to the top: the cell
	// covering the comment row now carries the category colour.
	m = press(m, "3", "0", "G")
	m = typeText(press(m, "c"), "note")
	m = pressMsg(m, ctrl("s"))
	m = press(m, "g", "g")
	if !endsWithCell(m.View().Content, "line 004", "48;2;224;175;104") {
		t.Errorf("minimap lacks the comment mark:\n%s", screen(m))
	}
}

// TestMinimapShowsThePosition pins that the strip shows where the
// visible window sits: the cursor background fills the strip cell of
// the window, at the top of the file and after a jump into it.
func TestMinimapShowsThePosition(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{minimapFile}, "main", themeFor, ui.Options{}), 80, 11)
	m = press(m, "z", "R")

	// The window starts at the top: the first strip cell carries the
	// cursor background, at the end of the first screen row.
	if !endsWithCell(m.View().Content, "ine 001", "48;2;54;74;130") {
		t.Errorf("minimap lacks the position block at the top:\n%s", screen(m))
	}

	// A jump into the middle moves the block: after 40G the window
	// starts at line 031, so the cell of screen row 3 (line 034)
	// carries the position.
	m = press(m, "4", "0", "G")
	if !endsWithCell(m.View().Content, "line 034", "48;2;54;74;130") {
		t.Errorf("minimap lacks the position block after the jump:\n%s", screen(m))
	}
}

// TestMinimapToggle pins that m hides and shows the strip, and that
// :set [no]minimap does the same.
func TestMinimapToggle(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{minimapFile}, "main", themeFor, ui.Options{}), 80, 11)
	m = press(m, "z", "R")
	const cursorBG = "48;2;54;74;130"

	if !endsWithCell(m.View().Content, "ine 001", cursorBG) {
		t.Fatalf("minimap is not shown by default:\n%s", screen(m))
	}
	m = press(m, "m")
	if endsWithCell(m.View().Content, "ine 001", cursorBG) {
		t.Errorf("m did not hide the minimap:\n%s", screen(m))
	}
	m = press(m, "m")
	if !endsWithCell(m.View().Content, "ine 001", cursorBG) {
		t.Errorf("m did not show the minimap again:\n%s", screen(m))
	}

	m = typeText(press(m, ":"), "set nominimap")
	m = press(m, "enter")
	if endsWithCell(m.View().Content, "ine 001", cursorBG) {
		t.Errorf(":set nominimap did not hide the minimap:\n%s", screen(m))
	}
}

// TestMinimapSitsInTheLastColumn pins that the strip is one straight
// line at the right edge: every row of the file area, also short and
// empty ones and the rows below the last line, is as wide as the
// terminal, with and without wrap.
func TestMinimapSitsInTheLastColumn(t *testing.T) {
	short := diff.New("short.txt", "a\nb\n", "a\nc\n")
	for _, wrap := range []bool{false, true} {
		m := start(ui.New([]diff.ReviewFile{short}, "main", themeFor, ui.Options{}), 40, 11)
		if wrap {
			m = typeText(press(m, ":"), "set wrap")
			m = press(m, "enter")
		}
		lines := strings.Split(m.View().Content, "\n")
		for i, l := range lines[:10] {
			if w := ansi.StringWidth(l); w != 40 {
				t.Errorf("wrap=%v: row %d is %d cells wide, want 40:\n%s", wrap, i, w, screen(m))
			}
		}
	}
}

// endsWithCell reports whether the screen row containing needle ends
// with a cell of the given truecolour background, the strip cell of
// that row.
func endsWithCell(s, needle, bg string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return strings.HasSuffix(line, bg+"m \x1b[m")
		}
	}
	return false
}
