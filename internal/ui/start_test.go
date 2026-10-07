package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// startFiles are two files in two directories, for the overview and
// the distribution bars of the start screen.
var startFiles = []diff.ReviewFile{
	diff.New("internal/ui/window.go", "a\nb\nc\n", "a\nB\nc\nd\n"),
	diff.New("internal/diff/kind.go", "x\n", "X\ny\n"),
}

// TestStartScreenShowsTheOverview pins the acceptance criterion
// "Startscreen mit Übersicht": branch, base, round, the size of the
// review and the distribution over the directories, in a box over the
// dimmed view.
func TestStartScreenShowsTheOverview(t *testing.T) {
	m := start(ui.New(startFiles, "main", themeFor, ui.Options{
		StartScreen: true,
		Branch:      "feat/wow",
		Base:        "main",
		Round:       3,
	}), 60, 18)

	assertGolden(t, "start.txt", screen(m))

	for _, want := range []string{
		"Branch feat/wow  Base main",
		"Runde 3",
		"2 Dateien · +4 −2",
		"internal/ui",
		"internal/diff",
		"Press any key to review",
	} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("start screen lacks %q:\n%s", want, screen(m))
		}
	}
}

// TestStartScreenDismissesOnTheFirstKey pins the acceptance criterion
// "verschwindet beim ersten Tastendruck": the first key hides the
// overview and acts normally.
func TestStartScreenDismissesOnTheFirstKey(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{StartScreen: true}), 40, 5)

	if !strings.Contains(screen(m), "Press any key to review") {
		t.Fatalf("start screen not shown:\n%s", screen(m))
	}

	// Five j's: the first dismisses the overview and moves the
	// cursor, so the view scrolled one row.
	m = press(m, "j", "j", "j", "j", "j")
	assertScreen(t, m, []string{
		"        2 + B",
		"⋯ 7 unveränderte Zeilen ⋯",
		"  10      - j",
		"       10 + J",
		"big.go  1/1  +2 −2",
	})
}

// TestStartScreenOffByDefault pins that the overview is opt-in: the
// zero options show the review directly.
func TestStartScreenOffByDefault(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 30, 5)

	if strings.Contains(screen(m), "Press any key to review") {
		t.Errorf("start screen shown without the option:\n%s", screen(m))
	}
}

// assertGolden compares the screen with the golden file in testdata.
// LGTM_UPDATE_GOLDEN=1 writes the golden files.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if os.Getenv("LGTM_UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden %s: %v (set LGTM_UPDATE_GOLDEN=1 to create it)", name, err)
	}
	if string(want) != got {
		t.Errorf("golden %s:\n%s\nwant:\n%s", name, got, want)
	}
}
