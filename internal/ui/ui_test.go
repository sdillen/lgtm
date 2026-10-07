package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// themeFor is the fixed theme of the UI tests: the built-in night theme,
// so the screens are deterministic.
var themeFor = theme.Default(theme.KindDark)

var file = diff.ReviewFile{
	Path: "main.go",
	Lines: []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "package main"},
		{Kind: diff.Deleted, Old: 2, Text: "old"},
		{Kind: diff.Added, New: 2, Text: "new"},
		{Kind: diff.Context, Old: 3, New: 3, Text: "end"},
	},
}

// big is a file with two single-line changes (b -> B, j -> J) surrounded by
// unchanged runs: a leading run of 1 line, a middle run of 7 lines (c..i)
// and a trailing run of 10 lines (k..t).
var big = diff.New("big.go",
	"a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl\nm\nn\no\np\nq\nr\ns\nt\n",
	"a\nB\nc\nd\ne\nf\ng\nh\ni\nJ\nk\nl\nm\nn\no\np\nq\nr\ns\nt\n",
)

var small = diff.New("small.txt", "x\n", "X\n")

func TestShowsFirstFileWithOldAndNewLineNumbers(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor), 30, 5)

	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"        2 + new",
		"   3    3   end",
		"main.go  1/1  +1 −1",
	})
}

func TestJAndKMoveTheCursorAndScroll(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor), 30, 3)

	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"main.go  1/1  +1 −1",
	})

	m = press(m, "j", "j", "j", "j")
	assertScreen(t, m, []string{
		"        2 + new",
		"   3    3   end",
		"main.go  1/1  +1 −1",
	})

	// Moving up only scrolls once the cursor leaves the top row.
	m = press(m, "k")
	assertScreen(t, m, []string{
		"        2 + new",
		"   3    3   end",
		"main.go  1/1  +1 −1",
	})

	m = press(m, "k")
	assertScreen(t, m, []string{
		"   2      - old",
		"        2 + new",
		"main.go  1/1  +1 −1",
	})

	m = press(m, "k", "k")
	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"main.go  1/1  +1 −1",
	})
}

func TestCountsRepeatAMotion(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	m = press(m, "5", "j")
	assertScreen(t, m, []string{
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"big.go  1/1  +2 −2",
	})
}

func TestQQuits(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor), 30, 5)

	_, cmd := m.Update(key("q"))

	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q returned %T, want tea.QuitMsg", cmd())
	}
}

// TestSyntaxHighlightingColoursAddedAndContextLines pins the acceptance
// criterion "syntax highlighting for added and unchanged lines": the Go
// keywords of the added line ("return", "true") and of the context line
// ("package") carry their token colours in the rendered view.
func TestSyntaxHighlightingColoursAddedAndContextLines(t *testing.T) {
	kw := diff.ReviewFile{
		Path: "kw.go",
		Lines: []diff.Line{
			{Kind: diff.Context, Old: 1, New: 1, Text: "package main"},
			{Kind: diff.Added, New: 2, Text: "return true"},
		},
	}
	m := start(ui.New([]diff.ReviewFile{kw}, "main", themeFor), 30, 5)
	content := m.View().Content

	// "return" is a Go keyword: #bb9af7 in tokyo night night.
	if !strings.Contains(content, "38;2;187;154;247") {
		t.Errorf("view lacks the keyword colour:\n%s", content)
	}
	// "true" is a Go constant: #e0af68.
	if !strings.Contains(content, "38;2;224;175;104") {
		t.Errorf("view lacks the constant colour:\n%s", content)
	}
	// "package" is a Go namespace keyword: #7dcfff.
	if !strings.Contains(content, "38;2;125;207;255") {
		t.Errorf("view lacks the namespace colour:\n%s", content)
	}
}

// TestDeletedLinesAreSyntaxHighlightedDimmed pins the "dimmed deleted
// lines" behaviour: "old" is a Go identifier and renders in the name
// colour #c0caf5 darkened to 55%.
func TestDeletedLinesAreSyntaxHighlightedDimmed(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor), 30, 5)
	content := m.View().Content

	if !strings.Contains(content, "38;2;105;111;134") {
		t.Errorf("view lacks the dimmed name colour:\n%s", content)
	}
}

func TestThemeStylesTheStatusLine(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor), 30, 5)
	content := m.View().Content

	// The status line uses the theme accent #7aa2f7.
	if !strings.Contains(content, "38;2;122;162;247") {
		t.Errorf("view lacks the status colour:\n%s", content)
	}
}

func TestLightThemeRendersTheDayPalette(t *testing.T) {
	day := theme.Default(theme.KindLight)
	m := start(ui.New([]diff.ReviewFile{file}, "main", day), 30, 5)
	content := m.View().Content

	// The status line uses the day accent #2e7de9.
	if !strings.Contains(content, "38;2;46;125;233") {
		t.Errorf("view lacks the day status colour:\n%s", content)
	}
}

func TestLongLinesAreCutAtTheTerminalWidth(t *testing.T) {
	long := diff.ReviewFile{Path: "long.txt", Lines: []diff.Line{
		{Kind: diff.Added, New: 1, Text: "0123456789abcdef"},
	}}
	m := start(ui.New([]diff.ReviewFile{long}, "main", themeFor), 20, 2)

	assertScreen(t, m, []string{
		"        1 + 01234567",
		"long.txt  1/1  +1 −0",
	})
}

func TestTabsAreExpandedToSpaces(t *testing.T) {
	tabbed := diff.ReviewFile{Path: "t.go", Lines: []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "\tx"},
	}}
	m := start(ui.New([]diff.ReviewFile{tabbed}, "main", themeFor), 30, 2)

	assertScreen(t, m, []string{
		"   1    1       x",
		"t.go  1/1  +0 −0",
	})
}

func TestWithoutChangesShowsHintNamingTheReviewSubject(t *testing.T) {
	m := start(ui.New(nil, "main..feature", themeFor), 30, 5)

	m = press(m, "j", "k")
	assertScreen(t, m, []string{
		"Nothing to review.",
		"",
		"No changes in main..feature.",
		"",
		"Press q to quit.",
	})
}

func TestWithoutChangesShowsHintWithoutSubject(t *testing.T) {
	m := start(ui.New(nil, "", themeFor), 30, 5)

	assertScreen(t, m, []string{
		"Nothing to review.",
		"",
		"Press q to quit.",
	})
}

func TestUnchangedRunsAreFoldedByDefaultWithLineCounts(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 8)

	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestZoOpensTheFoldUnderTheCursor(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 14)

	// Cursor onto the middle fold marker.
	m = press(m, "j", "j", "j")
	m = press(m, "z", "o")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"   3    3   c",
		"   4    4   d",
		"   5    5   e",
		"   6    6   f",
		"   7    7   g",
		"   8    8   h",
		"   9    9   i",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestZcClosesTheFoldUnderTheCursor(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 8)

	// Open the middle fold, then close it again.
	m = press(m, "j", "j", "j", "z", "o", "z", "c")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestZROpensAllFoldsAndZMClosesAll(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 23)

	m = press(m, "z", "R")
	assertScreen(t, m, []string{
		"   1    1   a",
		"   2      - b",
		"        2 + B",
		"   3    3   c",
		"   4    4   d",
		"   5    5   e",
		"   6    6   f",
		"   7    7   g",
		"   8    8   h",
		"   9    9   i",
		"  10      - j",
		"       10 + J",
		"  11   11   k",
		"  12   12   l",
		"  13   13   m",
		"  14   14   n",
		"  15   15   o",
		"  16   16   p",
		"  17   17   q",
		"  18   18   r",
		"  19   19   s",
		"  20   20   t",
		"big.go  1/1  +2 −2",
	})
}

func TestZMClosesAllFolds(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 8)

	m = press(m, "z", "M")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestUnifiedToggleShowsTheClassicDiff(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 17)

	m = press(m, "u")
	assertScreen(t, m, []string{
		"@@ -1,5 +1,5 @@",
		"   1    1   a",
		"   2      - b",
		"        2 + B",
		"   3    3   c",
		"   4    4   d",
		"   5    5   e",
		"@@ -7,7 +7,7 @@",
		"   7    7   g",
		"   8    8   h",
		"   9    9   i",
		"  10      - j",
		"       10 + J",
		"  11   11   k",
		"  12   12   l",
		"  13   13   m",
		"big.go  1/1  +2 −2  [unified]",
	})
}

func TestUnifiedToggleBackToTheFullFileView(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 8)

	m = press(m, "u", "u")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestBracketCJumpsBetweenHunks(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	m = press(m, "]", "c")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})

	m = press(m, "]", "c")
	assertScreen(t, m, []string{
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"big.go  1/1  +2 −2",
	})

	m = press(m, "[", "c")
	assertScreen(t, m, []string{
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"big.go  1/1  +2 −2",
	})

	// There is no hunk before the first one, so [c stays put.
	m = press(m, "[", "c")
	assertScreen(t, m, []string{
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"big.go  1/1  +2 −2",
	})
}

func TestBracketFJumpsBetweenFiles(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big, small}, "main", themeFor), 40, 3)

	m = press(m, "]", "f")
	assertScreen(t, m, []string{
		"   1      - x",
		"        1 + X",
		"small.txt  2/2  +1 −1",
	})

	m = press(m, "[", "f")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"big.go  1/2  +2 −2",
	})
}

func TestCtrlDMovesDownHalfAPageAndCtrlUBack(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	// The window scrolls half a page down, the cursor stays on its
	// screen row.
	m = pressMsg(m, ctrl("d"))
	assertScreen(t, m, []string{
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"big.go  1/1  +2 −2",
	})

	m = pressMsg(m, ctrl("d"))
	assertScreen(t, m, []string{
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})

	m = pressMsg(m, ctrl("u"))
	assertScreen(t, m, []string{
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"big.go  1/1  +2 −2",
	})

	m = pressMsg(m, ctrl("u"))
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestGgGoesToTheFirstLineAndGToTheLast(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	m = press(m, "G")
	assertScreen(t, m, []string{
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"⋯ 10 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})

	m = press(m, "g", "g")
	assertScreen(t, m, []string{
		"⋯ 1 unveränderte Zeile ⋯",
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"big.go  1/1  +2 −2",
	})
}

func TestZtPutsTheCursorAtTheTopAndZzCentersIt(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 7)

	// Unfold everything and move the cursor onto the second hunk, then
	// put it at the top of the screen.
	m = press(m, "z", "R", "]", "c", "]", "c", "z", "t")
	assertScreen(t, m, []string{
		"  10      - j",
		"       10 + J",
		"  11   11   k",
		"  12   12   l",
		"  13   13   m",
		"  14   14   n",
		"big.go  1/1  +2 −2",
	})

	// Centering scrolls the context above the cursor into view.
	m = press(m, "z", "z")
	assertScreen(t, m, []string{
		"   7    7   g",
		"   8    8   h",
		"   9    9   i",
		"  10      - j",
		"       10 + J",
		"  11   11   k",
		"big.go  1/1  +2 −2",
	})
}

// BenchmarkViewLargeFile measures rendering the visible part of a large
// unfolded file, the core of the scroll path.
func BenchmarkViewLargeFile(b *testing.B) {
	large := diff.New("big.txt",
		strings.Repeat("a line of context\n", 5000),
		strings.Repeat("a line of context\n", 5000)+"new line\n")
	m := start(ui.New([]diff.ReviewFile{large}, "main", themeFor), 120, 40)
	m = press(m, "z", "R")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.View()
	}
}

func TestHunkJumpInUnifiedModeMovesBetweenSections(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 4)

	m = press(m, "u", "]", "c")
	assertScreen(t, m, []string{
		"   4    4   d",
		"   5    5   e",
		"@@ -7,7 +7,7 @@",
		"big.go  1/1  +2 −2  [unified]",
	})

	m = press(m, "[", "c")
	assertScreen(t, m, []string{
		"@@ -1,5 +1,5 @@",
		"   1    1   a",
		"   2      - b",
		"big.go  1/1  +2 −2  [unified]",
	})
}

func TestBracketCInsideASectionReturnsToItsHeader(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 4)

	// Jump to the second section, move onto its lines, then [c must
	// land on the current section's header, not the previous one.
	m = press(m, "u", "]", "c", "j", "j", "[", "c")
	assertScreen(t, m, []string{
		"@@ -7,7 +7,7 @@",
		"   7    7   g",
		"   8    8   h",
		"big.go  1/1  +2 −2  [unified]",
	})
}

func TestCountRepeatsHunkJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	// Two hunks exist, so a count of three lands on the last one.
	m = press(m, "3", "]", "c")
	assertScreen(t, m, []string{
		"   2      - b",
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"big.go  1/1  +2 −2",
	})
}

func TestCountRepeatsFileJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big, small}, "main", themeFor), 40, 3)

	m = press(m, "2", "]", "f")
	assertScreen(t, m, []string{
		"   1      - x",
		"        1 + X",
		"small.txt  2/2  +1 −1",
	})
}

func TestCountedGJumpsToTheFileLine(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	m = press(m, "z", "R", "5", "G")
	assertScreen(t, m, []string{
		"   2      - b",
		"        2 + B",
		"   3    3   c",
		"   4    4   d",
		"big.go  1/1  +2 −2",
	})
}

func TestCountedGInUnifiedModeSnapsToTheNextVisibleLine(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 4)

	// Line 7 is the first visible line of the second section; line 6
	// is not shown in the unified view.
	m = press(m, "u", "7", "G")
	assertScreen(t, m, []string{
		"   4    4   d",
		"   5    5   e",
		"@@ -7,7 +7,7 @@",
		"big.go  1/1  +2 −2  [unified]",
	})
}

func TestCountedZzCentersAndCountedZtShowsTheLineAtTheTop(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor), 40, 5)

	m = press(m, "z", "R", "5", "z", "z")
	assertScreen(t, m, []string{
		"        2 + B",
		"   3    3   c",
		"   4    4   d",
		"   5    5   e",
		"big.go  1/1  +2 −2",
	})

	m = press(m, "7", "z", "t")
	assertScreen(t, m, []string{
		"   6    6   f",
		"   7    7   g",
		"   8    8   h",
		"   9    9   i",
		"big.go  1/1  +2 −2",
	})
}

func start(m tea.Model, width, height int) tea.Model {
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

func pressMsg(m tea.Model, keys ...tea.KeyPressMsg) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(k)
	}
	return m
}

func key(k string) tea.KeyPressMsg {
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func ctrl(k string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: []rune(k)[0], Mod: tea.ModCtrl}
}

func assertScreen(t *testing.T, m tea.Model, want []string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	// The terminal is a rectangle; trailing blank rows carry no
	// information.
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	got := strings.Join(lines, "\n")
	if got != strings.Join(want, "\n") {
		t.Errorf("screen:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
}
