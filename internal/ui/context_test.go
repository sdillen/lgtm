package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/search"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// fakeRepo is a search.Repo over fixed contents, for the context
// tests: files beyond the review, their current content and the review
// file itself as it stands in the working tree.
type fakeRepo struct {
	files    []string
	contents map[string]string
}

func (r fakeRepo) Files() ([]string, error) { return r.files, nil }

func (r fakeRepo) Read(path string) (string, error) {
	c, ok := r.contents[path]
	if !ok {
		return "", errNotFound(path)
	}
	return c, nil
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) + ": not in the repo" }

// contextReview is a small review whose new version mentions Util, so
// the cursor can sit on the word.
var contextReview = diff.New("main.go",
	"old\nUtil is called\n",
	"new\nUtil is called\n",
)

// repo is the fake working tree behind the context tests: the review
// file plus two files beyond it.
var repo = fakeRepo{
	files: []string{"lib/helper.py", "lib/util.go", "main.go"},
	contents: map[string]string{
		"main.go":       "new\nUtil is called\n",
		"lib/util.go":   "package lib\n\n// Util does things.\nfunc Util() {}\n",
		"lib/helper.py": "# call Util here\n",
	},
}

func withSearch(r fakeRepo) ui.Options {
	return ui.Options{Search: search.New(r)}
}

func space() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "} }

func keyTab() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyTab} }

// openPickerAll opens the all-files picker with the leader sequence.
func openPickerAll(m tea.Model) tea.Model {
	m = pressMsg(m, space())
	return press(m, "f")
}

// TestAllFilesPickerListsRepoFiles pins the acceptance criterion
// "any repository file can be opened": the all-files picker lists the
// files beyond the review, with the review files marked.
func TestAllFilesPickerListsRepoFiles(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 12)

	m = openPickerAll(m)

	s := screen(m)
	for _, want := range []string{"lib/util.go", "lib/helper.py", "main.go  ●"} {
		if !strings.Contains(s, want) {
			t.Errorf("picker lacks %q:\n%s", want, s)
		}
	}
}

// TestAllFilesPickerOpensAReviewFile pins that the all-files picker can
// jump to a file of the review as well.
func TestAllFilesPickerOpensAReviewFile(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 12)

	m = openPickerAll(m)
	for _, r := range "main" {
		m = press(m, string(r))
	}
	m = press(m, "enter")

	if !strings.HasPrefix(screen(m), "   1      - old") {
		t.Errorf("picker did not open the review file:\n%s", screen(m))
	}
}

// TestContextFileOpensReadOnlyInTheNewState pins the acceptance
// criterion "arbitrary repository files open read-only in the new
// state": the all-files picker opens a file beyond the review, showing
// its working tree content with plain line numbers and the ctx marker.
func TestContextFileOpensReadOnlyInTheNewState(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 5)

	m = openPickerAll(m)
	for _, r := range "util" {
		m = press(m, string(r))
	}
	m = press(m, "enter")

	assertScreen(t, m, []string{
		"   1 package lib",
		"   2",
		"   3 // Util does things.",
		"   4 func Util() {}",
		"lib/util.go  ctx  1/4",
	})
}

// TestContextFileMotionsAndJumps pins that the vim motions and the
// line jumps work in a context file.
func TestContextFileMotionsAndJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 5)
	m = openPickerAll(m)
	for _, r := range "util" {
		m = press(m, string(r))
	}
	m = press(m, "enter")

	m = press(m, "G")
	if !strings.HasSuffix(screen(m), "lib/util.go  ctx  4/4") {
		t.Errorf("G did not reach the last line:\n%s", screen(m))
	}

	m = press(m, "g", "g")
	if !strings.HasSuffix(screen(m), "lib/util.go  ctx  1/4") {
		t.Errorf("gg did not reach the first line:\n%s", screen(m))
	}

	m = press(m, "5", "j")
	if !strings.HasSuffix(screen(m), "lib/util.go  ctx  4/4") {
		t.Errorf("counted j did not clamp to the last line:\n%s", screen(m))
	}
}

// TestContextFileHasNoComments pins that files beyond the review are
// read-only: the comment key does not open the comment float.
func TestContextFileHasNoComments(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 5)
	m = openPickerAll(m)
	for _, r := range "util" {
		m = press(m, string(r))
	}
	m = press(m, "enter")

	m = press(m, "c")

	if s := screen(m); strings.Contains(s, "<Tab> category") {
		t.Errorf("comment float opened on a context file:\n%s", s)
	}
}

// TestJumplistMovesAcrossFiles pins the acceptance criterion
// "<C-o>/<C-i> work across files": back returns to the review, forward
// returns to the context file.
func TestJumplistMovesAcrossFiles(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 4)
	m = openPickerAll(m)
	for _, r := range "util" {
		m = press(m, string(r))
	}
	m = press(m, "enter")

	m = pressMsg(m, ctrl("o"))
	assertScreen(t, m, []string{
		"   1      - old",
		"        1 + new",
		"⋯ 1 unveränderte Zeile ⋯",
		"main.go  1/1  +1 −1",
	})

	m = pressMsg(m, ctrl("i"))
	assertScreen(t, m, []string{
		"   1 package lib",
		"   2",
		"   3 // Util does things.",
		"lib/util.go  ctx  1/4",
	})
}

// TestJumplistRecordsLineJumps pins that gg/G are jumps: back and
// forward move between the top and the bottom of the file.
func TestJumplistRecordsLineJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{review8()}, "main", themeFor, ui.Options{}), 40, 6)

	m = press(m, "G")
	assertScreen(t, m, []string{
		"        4 + bar two",
		"        5 + foo three",
		"        6 + bar three",
		"        7 + foo four",
		"        8 + bar four",
		"big8.txt  1/1  +8 −0",
	})

	m = pressMsg(m, ctrl("o"))
	assertScreen(t, m, []string{
		"        1 + foo one",
		"        2 + bar one",
		"        3 + foo two",
		"        4 + bar two",
		"        5 + foo three",
		"big8.txt  1/1  +8 −0",
	})

	m = pressMsg(m, ctrl("i"))
	if !strings.HasSuffix(screen(m), "        8 + bar four\nbig8.txt  1/1  +8 −0") {
		t.Errorf("C-i did not return to the last line:\n%s", screen(m))
	}

	m = press(m, "g", "g")
	m = pressMsg(m, ctrl("o"))
	if !strings.HasSuffix(screen(m), "        8 + bar four\nbig8.txt  1/1  +8 −0") {
		t.Errorf("C-o after gg did not return to the last line:\n%s", screen(m))
	}
}

// review8 is an added file of eight lines, so every line is a row and
// the cursor position is visible in the scroll offset.
func review8() diff.ReviewFile {
	return diff.New("big8.txt", "", strings.Join([]string{
		"foo one", "bar one", "foo two", "bar two",
		"foo three", "bar three", "foo four", "bar four",
	}, "\n")+"\n")
}

// TestSlashSearchFindsAndNRepeats pins the acceptance criterion
// "/ n N": / jumps to the first match after the cursor, n and N move
// through the matches, wrapping at both ends.
func TestSlashSearchFindsAndNRepeats(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{review8()}, "main", themeFor, ui.Options{}), 40, 6)

	first := []string{
		"        1 + foo one",
		"        2 + bar one",
		"        3 + foo two",
		"        4 + bar two",
		"        5 + foo three",
		"big8.txt  1/1  +8 −0",
	}
	last := []string{
		"        3 + foo two",
		"        4 + bar two",
		"        5 + foo three",
		"        6 + bar three",
		"        7 + foo four",
		"big8.txt  1/1  +8 −0",
	}

	// /foo jumps to the second match; the view has not scrolled yet.
	m = press(m, "/")
	m = typeText(m, "foo")
	m, _ = enter(m)
	assertScreen(t, m, first)

	// n moves to the third match, still visible.
	m = press(m, "n")
	assertScreen(t, m, first)

	// n reaches the last match and scrolls.
	m = press(m, "n")
	assertScreen(t, m, last)

	// n wraps to the first match.
	m = press(m, "n")
	assertScreen(t, m, first)

	// N wraps back to the last match.
	m = press(m, "N")
	assertScreen(t, m, last)
}

// TestNSearchesBackwards pins that N moves to the previous match and
// scrolls the view back up.
func TestNSearchesBackwards(t *testing.T) {
	sparse := diff.New("sparse.txt", "", strings.Join([]string{
		"foo one", "pad two", "pad three", "pad four",
		"pad five", "pad six", "foo two", "pad eight",
	}, "\n")+"\n")
	m := start(ui.New([]diff.ReviewFile{sparse}, "main", themeFor, ui.Options{}), 40, 6)

	first := []string{
		"        1 + foo one",
		"        2 + pad two",
		"        3 + pad three",
		"        4 + pad four",
		"        5 + pad five",
		"sparse.txt  1/1  +8 −0",
	}
	last := []string{
		"        3 + pad three",
		"        4 + pad four",
		"        5 + pad five",
		"        6 + pad six",
		"        7 + foo two",
		"sparse.txt  1/1  +8 −0",
	}

	m = press(m, "/")
	m = typeText(m, "foo")
	m, _ = enter(m)
	assertScreen(t, m, last)

	m = press(m, "N")
	assertScreen(t, m, first)

	m = press(m, "n")
	assertScreen(t, m, last)
}

// TestStarSearchesTheWordUnderTheCursor pins the acceptance criterion
// "* #": the word under the cursor is searched as a whole word, * goes
// forward and # backwards, wrapping.
func TestStarSearchesTheWordUnderTheCursor(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{star8()}, "main", themeFor, ui.Options{}), 40, 6)

	m = press(m, "*")
	assertScreen(t, m, []string{
		"        3 + pad three",
		"        4 + pad four",
		"        5 + pad five",
		"        6 + pad six",
		"        7 + alpha omega",
		"star8.txt  1/1  +8 −0",
	})

	m = press(m, "#")
	assertScreen(t, m, []string{
		"        1 + alpha beta",
		"        2 + pad two",
		"        3 + pad three",
		"        4 + pad four",
		"        5 + pad five",
		"star8.txt  1/1  +8 −0",
	})

	// Move the cursor onto "beta" of the last line and search it: *
	// wraps to the first line, # wraps back.
	m = press(m, "G")
	for range 6 {
		m = press(m, "l")
	}
	m = press(m, "*")
	assertScreen(t, m, []string{
		"        1 + alpha beta",
		"        2 + pad two",
		"        3 + pad three",
		"        4 + pad four",
		"        5 + pad five",
		"star8.txt  1/1  +8 −0",
	})

	m = press(m, "#")
	assertScreen(t, m, []string{
		"        4 + pad four",
		"        5 + pad five",
		"        6 + pad six",
		"        7 + alpha omega",
		"        8 + gamma beta",
		"star8.txt  1/1  +8 −0",
	})
}

// star8 has the word "alpha" on the first and the seventh line and
// "beta" on the first and the last line.
func star8() diff.ReviewFile {
	return diff.New("star8.txt", "", strings.Join([]string{
		"alpha beta", "pad two", "pad three", "pad four",
		"pad five", "pad six", "alpha omega", "gamma beta",
	}, "\n")+"\n")
}

// TestLeftRightMoveTheColumnAndScrollHorizontally pins the h/l motions
// that position the word under the cursor: moving right past the edge
// scrolls the view horizontally, h scrolls back.
func TestLeftRightMoveTheColumnAndScrollHorizontally(t *testing.T) {
	long := diff.New("long.txt", "", "0123456789abcdefghij\nsecond\n")
	m := start(ui.New([]diff.ReviewFile{long}, "main", themeFor, ui.Options{}), 16, 4)

	first := strings.Split(screen(m), "\n")[0]
	if !strings.HasPrefix(first, "        1 + 0123") {
		t.Fatalf("long line not truncated:\n%s", screen(m))
	}

	for range 20 {
		m = press(m, "l")
	}
	scrolled := strings.Split(screen(m), "\n")[0]
	if !strings.HasPrefix(scrolled, "   1 + 012345678") {
		t.Errorf("l did not scroll the view horizontally:\n%s", screen(m))
	}

	for range 20 {
		m = press(m, "h")
	}
	if strings.Split(screen(m), "\n")[0] != first {
		t.Errorf("h did not scroll back:\n%s", screen(m))
	}
}

// TestGrepListsTheResultsAndJumps pins the acceptance criterion
// "gr delivers a navigable result list": the repo-wide word search
// shows path, line and text, enter jumps to the hit.
func TestGrepListsTheResultsAndJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 12)
	m = press(m, "j", "j") // onto "Util is called"

	m = press(m, "g", "r")

	s := screen(m)
	for _, want := range []string{
		"gr: Util",
		"lib/helper.py:1  # call Util here",
		"lib/util.go:3  // Util does things.",
		"lib/util.go:4  func Util() {}",
		"main.go:2  Util is called",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("gr results lack %q:\n%s", want, s)
		}
	}

	// Enter jumps to the first hit; the list stays open.
	m = press(m, "enter")
	if !strings.HasSuffix(screen(m), "lib/helper.py  ctx  1/1") {
		t.Errorf("enter did not jump to the first hit:\n%s", screen(m))
	}

	// The next hit opens the second file at its line.
	m = press(m, "j", "enter")
	if !strings.HasSuffix(screen(m), "lib/util.go  ctx  3/4") {
		t.Errorf("enter did not jump to the second hit:\n%s", screen(m))
	}

	// Esc closes the list.
	m = press(m, "esc")
	if strings.Contains(screen(m), "gr: Util") {
		t.Errorf("esc did not close the results:\n%s", screen(m))
	}
}

// TestGrepWithoutSearchIsReported pins the message when the repo
// search is not available.
func TestGrepWithoutSearchIsReported(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, ui.Options{}), 60, 5)
	m = press(m, "j", "j")

	m = press(m, "g", "r")

	if !strings.Contains(screen(m), "no word under the cursor") &&
		!strings.Contains(screen(m), "repo search is not available") {
		t.Errorf("gr did not report a problem:\n%s", screen(m))
	}
}

// TestGotoDefJumpsDirectlyToTheOnlyDefinition pins the acceptance
// criterion "gd finds definitions": with a single definition the jump
// happens immediately.
func TestGotoDefJumpsDirectlyToTheOnlyDefinition(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(repo)), 60, 5)
	m = press(m, "j", "j")

	m = press(m, "g", "d")

	assertScreen(t, m, []string{
		"   1 package lib",
		"   2",
		"   3 // Util does things.",
		"   4 func Util() {}",
		"lib/util.go  ctx  4/4",
	})
}

// TestGotoDefShowsASelectionForMultipleDefinitions pins that gd lists
// the candidates when several lines define the symbol.
func TestGotoDefShowsASelectionForMultipleDefinitions(t *testing.T) {
	two := fakeRepo{
		files: []string{"lib/util.go", "main.go"},
		contents: map[string]string{
			"main.go":     "new\nUtil is called\n",
			"lib/util.go": "package lib\n\nfunc Util() {}\n\nvar Util = 1\n",
		},
	}
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(two)), 60, 12)
	m = press(m, "j", "j")

	m = press(m, "g", "d")

	s := screen(m)
	for _, want := range []string{"gd: Util", "lib/util.go:3  func Util() {}", "lib/util.go:5  var Util = 1"} {
		if !strings.Contains(s, want) {
			t.Errorf("gd results lack %q:\n%s", want, s)
		}
	}

	m = press(m, "enter")
	if !strings.HasSuffix(screen(m), "lib/util.go  ctx  3/5") {
		t.Errorf("enter did not jump to the definition:\n%s", screen(m))
	}
}

// TestGotoDefWithoutDefinitionsIsReported pins the message when gd
// finds nothing.
func TestGotoDefWithoutDefinitionsIsReported(t *testing.T) {
	none := fakeRepo{
		files:    []string{"main.go"},
		contents: map[string]string{"main.go": "new\nUtil is called\n"},
	}
	m := start(ui.New([]diff.ReviewFile{contextReview}, "main", themeFor, withSearch(none)), 60, 5)
	m = press(m, "j", "j")

	m = press(m, "g", "d")

	if !strings.Contains(screen(m), "no definition of Util found") {
		t.Errorf("gd did not report the miss:\n%s", screen(m))
	}
}

// TestJumplistRecordsSearchJumps pins that the search jumps are part
// of the jumplist: C-o returns from the match to the cursor position
// before the search.
func TestJumplistRecordsSearchJumps(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{review8()}, "main", themeFor, ui.Options{}), 40, 6)

	m = press(m, "G") // to the last line
	m = press(m, "/")
	m = typeText(m, "foo")
	m, _ = enter(m) // to the first match, wrapping

	m = pressMsg(m, ctrl("o"))
	if !strings.HasSuffix(screen(m), "        8 + bar four\nbig8.txt  1/1  +8 −0") {
		t.Errorf("C-o did not return before the search:\n%s", screen(m))
	}
}

// TestEmptyReviewIgnoresContextActions pins that the context actions
// are inert when no file is open: no panic and no view change.
func TestEmptyReviewIgnoresContextActions(t *testing.T) {
	m := start(ui.New(nil, "main", themeFor, withSearch(repo)), 40, 5)

	m = press(m, "h", "l", "*", "#", "g", "r", "g", "d")

	if !strings.Contains(screen(m), "Nothing to review.") {
		t.Errorf("context actions changed the empty view:\n%s", screen(m))
	}
}

// TestJumplistRestoresTheColumn pins that the jumplist entries carry
// the cursor column: C-o returns to the column the jump left from.
func TestJumplistRestoresTheColumn(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{review8()}, "main", themeFor, ui.Options{}), 40, 6)

	m = press(m, "l", "l", "l") // onto the space of "foo one"
	m = press(m, "G")
	m = pressMsg(m, ctrl("o"))

	// With the column restored, * finds no word on the space;
	// without it, * would search the word at column 0.
	m = press(m, "*")
	if !strings.Contains(screen(m), "no word under the cursor") {
		t.Errorf("C-o did not restore the column:\n%s", screen(m))
	}
}

// TestTabMovesForwardInTheJumplist pins that tab works as C-i, for
// terminals that send tab for ctrl+i.
func TestTabMovesForwardInTheJumplist(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{review8()}, "main", themeFor, ui.Options{}), 40, 6)

	m = press(m, "G")
	m = pressMsg(m, ctrl("o"))
	m = pressMsg(m, keyTab())

	if !strings.HasSuffix(screen(m), "        8 + bar four\nbig8.txt  1/1  +8 −0") {
		t.Errorf("tab did not move forward in the jumplist:\n%s", screen(m))
	}
}
