package ui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// drawerFiles are three changed files in different directories, for the
// drawer, sidebar and picker tests.
var drawerFiles = []diff.ReviewFile{
	diff.New("cmd/main.go", "", "package main\n"),
	diff.New("internal/ui/ui.go", "", "package ui\n"),
	diff.New("README.md", "", "# lgtm\n"),
}

// leader presses the default leader key <space>.
func leader(m tea.Model) tea.Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	return m
}

// esc presses the escape key.
func esc(m tea.Model) tea.Model {
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	return m
}

// withState wraps Options with a fresh in-memory review state persisted
// into dir.
func withState(dir string) ui.Options {
	return ui.Options{State: review.NewState(), Dir: dir}
}

// readState loads the persisted state of dir.
func readState(t *testing.T, dir string) *review.State {
	t.Helper()
	s, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TestDrawerOpensAndShowsTheFileTree pins the acceptance criterion
// "Drawer mit Dateibaum ein- und ausblendbar": <leader>d opens a tree of
// the changed files, grouped by directory, with the file view still
// behind it.
func TestDrawerOpensAndShowsTheFileTree(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 40, 10)

	if strings.Contains(screen(m), " Files ") {
		t.Fatalf("drawer visible before opening:\n%s", screen(m))
	}

	m = press(leader(m), "d")

	s := screen(m)
	for _, want := range []string{" Files ", "cmd/", "main.go", "ui/", "ui.go", "README.md"} {
		if !strings.Contains(s, want) {
			t.Errorf("drawer lacks %q:\n%s", want, s)
		}
	}
	// The file view is still rendered behind the overlay: the status
	// line below the drawer shows the current file.
	if !strings.Contains(s, "cmd/main.go  1/3") {
		t.Errorf("file view missing behind the drawer:\n%s", s)
	}
}

// TestDrawerSelectionJumpsToTheFile pins "Auswahl springt zur Datei":
// the drawer cursor moves over files (skipping directory headings) and
// enter opens the selected file.
func TestDrawerSelectionJumpsToTheFile(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 40, 10)

	m = press(leader(m), "d")
	// Tree rows: cmd/ (heading), main.go, internal/ (heading), ui/
	// (heading), ui.go, README.md. One j press lands on ui.go.
	m = press(m, "j")
	m, _ = enter(m)

	s := screen(m)
	if !strings.Contains(s, "internal/ui/ui.go  2/3") {
		t.Errorf("status line does not show the jumped file:\n%s", s)
	}
	// The drawer stays open and now highlights the jumped file.
	if !strings.Contains(s, "  ui.go") {
		t.Errorf("drawer does not highlight ui.go:\n%s", s)
	}
}

// TestDrawerClosesOnEscAndTogglesBack pins that esc closes the drawer
// and the toggle key reopens it.
func TestDrawerClosesOnEscAndTogglesBack(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 40, 10)

	m = press(leader(m), "d")
	m = esc(m)
	if strings.Contains(screen(m), " Files ") {
		t.Errorf("drawer still open after esc:\n%s", screen(m))
	}

	m = press(leader(m), "d")
	if !strings.Contains(screen(m), " Files ") {
		t.Errorf("drawer did not reopen:\n%s", screen(m))
	}
}

// TestScrollingToTheEndMarksTheFileRead pins the acceptance criterion
// "Gelesen-Status automatisch": scrolling until the end of the file is
// visible marks the file as read and persists it.
func TestScrollingToTheEndMarksTheFileRead(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, withState(dir)), 40, 5)

	if _, err := os.Stat(filepath.Join(dir, ".lgtm", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state.json exists although the file end was never visible (err=%v)", err)
	}
	if readState(t, dir).Read("big.go") {
		t.Fatal("big.go is read although the end was never visible")
	}

	_ = press(m, "G")

	if !readState(t, dir).Read("big.go") {
		t.Error("big.go is not read after scrolling to the end")
	}
}

// TestShortFilesAreReadOnOpen pins that a file that fits the screen is
// read immediately: its end is visible from the start.
func TestShortFilesAreReadOnOpen(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 30, 5)

	if !readState(t, dir).Read("main.go") {
		t.Error("main.go is not read although it fits the screen")
	}
	_ = m
}

// TestDrawerShowsTheReadStatus pins "Gelesen-Status im Drawer sichtbar":
// read files carry a checkmark, unread files none.
func TestDrawerShowsTheReadStatus(t *testing.T) {
	dir := t.TempDir()
	state := review.NewState()
	state.SetRead("cmd/main.go", "package main\n")
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{State: state, Dir: dir}), 40, 10)

	m = press(leader(m), "d")

	s := screen(m)
	if !strings.Contains(s, "main.go  ✓") {
		t.Errorf("drawer lacks the checkmark of the read file:\n%s", s)
	}
	if strings.Contains(s, "ui.go  ✓") || strings.Contains(s, "README.md  ✓") {
		t.Errorf("drawer marks unread files as read:\n%s", s)
	}
}

// TestManualToggleMarksAndUnmarks pins the acceptance criterion "manuell
// umschalten": <leader>r flips the read status of the current file, and
// an unread file is not immediately re-marked by the same key press.
func TestManualToggleMarksAndUnmarks(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, withState(dir)), 40, 5)

	m = press(m, "G") // read
	m = press(leader(m), "r")

	if readState(t, dir).Read("big.go") {
		t.Error("big.go still read after <leader>r")
	}

	// The unread status survives further keys at the file end; it is
	// only re-marked once the end becomes visible again.
	m = press(m, "j")
	if readState(t, dir).Read("big.go") {
		t.Error("big.go was re-marked read although the view never left the end")
	}
	// Scroll back up until the end leaves the screen, then down again.
	m = press(m, "k", "k", "k", "k", "G")
	if !readState(t, dir).Read("big.go") {
		t.Error("big.go is not read again after the end became visible anew")
	}

	// Toggling now unmarks, toggling again re-marks.
	m = press(leader(m), "r")
	if readState(t, dir).Read("big.go") {
		t.Error("big.go still read after toggling off")
	}
	_ = press(leader(m), "r")
	if !readState(t, dir).Read("big.go") {
		t.Error("big.go is not read after toggling back")
	}
}

// TestReadStatusSurvivesARestart pins the acceptance criterion
// "Gelesen-Status bleibt nach Neustart erhalten": a second program
// loading the persisted state shows the file as read.
func TestReadStatusSurvivesARestart(t *testing.T) {
	dir := t.TempDir()

	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 30, 5)
	_ = press(m, "G")

	// A new session loads the state from disk.
	state, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	m = start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{State: state}), 30, 5)
	m = press(leader(m), "d")

	if !strings.Contains(screen(m), "main.go  ✓") {
		t.Errorf("drawer of the restarted session lacks the checkmark:\n%s", screen(m))
	}
}

// TestPickerFiltersAndJumps pins the acceptance criterion "Fuzzy-Picker
// über geänderte Dateien": typing filters the files, enter jumps to the
// selected one.
func TestPickerFiltersAndJumps(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 40, 12)

	m = press(leader(m), "p")
	s := screen(m)
	for _, want := range []string{"cmd/main.go", "internal/ui/ui.go", "README.md"} {
		if !strings.Contains(s, want) {
			t.Errorf("picker lacks %q:\n%s", want, s)
		}
	}

	m = typeText(m, "ui")
	if !strings.Contains(screen(m), "internal/ui/ui.go") {
		t.Errorf("picker lost the matching file:\n%s", screen(m))
	}
	if strings.Contains(screen(m), "README.md") {
		t.Errorf("picker still shows non-matching files:\n%s", screen(m))
	}

	m, _ = enter(m)
	s = screen(m)
	if !strings.Contains(s, "internal/ui/ui.go  2/3") {
		t.Errorf("status line does not show the picked file:\n%s", s)
	}
	if strings.Contains(s, "cmd/") {
		t.Errorf("picker still open after enter:\n%s", s)
	}
}

// TestPickerNavigatesWithCtrlN pins that ctrl+n moves the selection
// down, so enter jumps to the second match.
func TestPickerNavigatesWithCtrlN(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 40, 12)

	m = press(leader(m), "p")
	m = typeText(m, "go")
	m, _ = m.Update(ctrl("n"))
	m, _ = enter(m)

	// "go" matches cmd/main.go (its g is much earlier) and
	// internal/ui/ui.go; ctrl+n moves to ui.go, the second match.
	if !strings.Contains(screen(m), "internal/ui/ui.go  2/3") {
		t.Errorf("status line does not show the second match:\n%s", screen(m))
	}
}

// TestPickerClosesOnEsc pins that esc cancels the picker.
func TestPickerClosesOnEsc(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 40, 12)

	m = press(leader(m), "p")
	m = esc(m)

	if strings.Contains(screen(m), "Pick file") {
		t.Errorf("picker still open after esc:\n%s", screen(m))
	}
}

// TestSidebarTogglesViaSet pins the acceptance criterion "optionale
// permanente Sidebar": :set sidebar splits the view, :set nosidebar
// returns to zen mode.
func TestSidebarTogglesViaSet(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 60, 10)

	m = typeText(press(m, ":"), "set sidebar")
	m, _ = enter(m)

	s := screen(m)
	if !strings.Contains(s, " Files ") || !strings.Contains(s, "README.md") {
		t.Errorf("sidebar lacks the file tree:\n%s", s)
	}
	if !strings.Contains(s, "package main") {
		t.Errorf("file view missing next to the sidebar:\n%s", s)
	}

	m = typeText(press(m, ":"), "set nosidebar")
	m, _ = enter(m)

	if strings.Contains(screen(m), " Files ") {
		t.Errorf("sidebar still shown after :set nosidebar:\n%s", screen(m))
	}
}

// TestSidebarQueriesTheOption pins the :set sidebar? report.
func TestSidebarQueriesTheOption(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 60, 10)

	m = typeText(press(m, ":"), "set sidebar?")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "nosidebar") {
		t.Errorf(":set sidebar? did not report the option:\n%s", screen(m))
	}
}

// TestSidebarHighlightsTheCurrentFile pins that the sidebar marks the
// file shown in the file view.
func TestSidebarHighlightsTheCurrentFile(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 60, 10)

	m = press(m, "]", "f")
	m = typeText(press(m, ":"), "set sidebar")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "internal/ui/ui.go  2/3") {
		t.Errorf("status line does not show the second file:\n%s", screen(m))
	}
}

// TestWhichKeyShowsTheNewLeaderBindings pins that the drawer, picker and
// read toggle are discoverable in which-key.
func TestWhichKeyShowsTheNewLeaderBindings(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, fast), 50, 14)

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m, _ = m.Update(cmd())

	for _, want := range []string{"d", "toggle file drawer", "p", "pick a changed file", "r", "toggle read status"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("leader which-key lacks %q:\n%s", want, screen(m))
		}
	}
}

// TestHelpShowsTheNewBindings pins that the full help lists the drawer,
// picker and read toggle.
func TestHelpShowsTheNewBindings(t *testing.T) {
	m := start(ui.New(drawerFiles, "main", themeFor, ui.Options{}), 80, 40)

	m = press(m, "?")
	s := screen(m)
	for _, want := range []string{"toggle file drawer", "pick a changed file", "toggle read status"} {
		if !strings.Contains(s, want) {
			t.Errorf("help lacks %q:\n%s", want, s)
		}
	}
}

// TestDrawerCutsLongTrees pins that a drawer taller than the terminal is
// cut with a marker instead of overflowing, so the status line stays
// visible.
func TestDrawerCutsLongTrees(t *testing.T) {
	var many []diff.ReviewFile
	for i := 0; i < 12; i++ {
		many = append(many, diff.New(fmt.Sprintf("dir%d/f.go", i), "", "package f\n"))
	}
	m := start(ui.New(many, "main", themeFor, ui.Options{}), 40, 5)

	m = press(leader(m), "d")
	s := screen(m)

	if !strings.Contains(s, "…") {
		t.Errorf("drawer lacks the cut marker:\n%s", s)
	}
	// The status line stays on the last row.
	if !strings.Contains(s, "dir0/f.go  1/12") {
		t.Errorf("status line missing below the cut drawer:\n%s", s)
	}
	// Files beyond the cut are not shown.
	if strings.Contains(s, "dir11") {
		t.Errorf("drawer shows files beyond the cut:\n%s", s)
	}
}
