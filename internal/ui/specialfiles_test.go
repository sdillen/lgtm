package ui_test

import (
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// TestBinaryFileShowsOnlyTheSizes pins the acceptance criterion
// "bei Binärdateien nur Metadaten (Größe alt/neu)": the view is one
// info row, not a garbled line diff.
func TestBinaryFileShowsOnlyTheSizes(t *testing.T) {
	bin := diff.New("img.png", "\x00\x01", "\x00\x01\x02\x03")
	m := start(ui.New([]diff.ReviewFile{bin}, "main", themeFor, ui.Options{}), 30, 2)

	assertScreen(t, m, []string{
		"binär: 2 → 4 Bytes",
		"img.png  1/1  +0 −0",
	})
}

// TestDeletedBinaryShowsTheOldSize pins the deleted binary case: only
// the old size is known.
func TestDeletedBinaryShowsTheOldSize(t *testing.T) {
	bin := diff.New("gone.bin", "\x00\x01\x02", "")
	m := start(ui.New([]diff.ReviewFile{bin}, "main", themeFor, ui.Options{}), 30, 2)

	assertScreen(t, m, []string{
		"binär: 3 Bytes (gelöscht)",
		"gone.bin  1/1  +0 −0",
	})
}

// TestModeChangeShowsAnInfoEntry pins the mode change display: a
// compact entry with both modes instead of an empty diff.
func TestModeChangeShowsAnInfoEntry(t *testing.T) {
	mode := diff.Mode("script.sh", 0o100644, 0o100755)
	m := start(ui.New([]diff.ReviewFile{mode}, "main", themeFor, ui.Options{}), 30, 2)

	assertScreen(t, m, []string{
		"modus: 100644 → 100755",
		"script.sh  1/1  +0 −0",
	})
}

// TestSubmoduleChangeShowsAnInfoEntry pins the submodule display: the
// old and new commit hashes, shortened.
func TestSubmoduleChangeShowsAnInfoEntry(t *testing.T) {
	sub := diff.Submodule("lib", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	m := start(ui.New([]diff.ReviewFile{sub}, "main", themeFor, ui.Options{}), 40, 2)

	assertScreen(t, m, []string{
		"submodul: aaaaaaa → bbbbbbb",
		"lib  1/1  +0 −0",
	})

	added := diff.Submodule("new-lib", "", "cccccccccccccccccccccccccccccccccccccccc")
	m = start(ui.New([]diff.ReviewFile{added}, "main", themeFor, ui.Options{}), 40, 2)
	assertScreen(t, m, []string{
		"submodul: ccccccc",
		"new-lib  1/1  +0 −0",
	})

	deleted := diff.Submodule("old-lib", "dddddddddddddddddddddddddddddddddddddddd", "")
	m = start(ui.New([]diff.ReviewFile{deleted}, "main", themeFor, ui.Options{}), 40, 2)
	assertScreen(t, m, []string{
		"submodul: ddddddd (gelöscht)",
		"old-lib  1/1  +0 −0",
	})
}

// TestRenameShowsOldToNew pins the acceptance criterion "Renames als
// alt → neu": the status line names the old and the new path, and the
// content diff is the only body.
func TestRenameShowsOldToNew(t *testing.T) {
	renamed := diff.New("b.go", "package a\n", "package b\n\nfunc B() {}\n")
	renamed.OldPath = "a.go"
	m := start(ui.New([]diff.ReviewFile{renamed}, "main", themeFor, ui.Options{}), 30, 5)

	assertScreen(t, m, []string{
		"   1      - package a",
		"        1 + package b",
		"        2 +",
		"        3 + func B() {}",
		"a.go → b.go  1/1  +3 −1",
	})
}

// TestDeletedFileIsCollapsedAndExpandable pins the acceptance
// criterion "gelöschte Dateien eingeklappt, alter Inhalt aufklappbar":
// the file starts as one marker row, zo opens it, zc collapses it
// again.
func TestDeletedFileIsCollapsedAndExpandable(t *testing.T) {
	gone := diff.New("gone.txt", "x\ny\nz\n", "")
	m := start(ui.New([]diff.ReviewFile{gone}, "main", themeFor, ui.Options{}), 30, 2)

	assertScreen(t, m, []string{
		"⋯ 3 gelöschte Zeilen ⋯",
		"gone.txt  1/1  +0 −3",
	})

	m = press(m, "z", "o")
	assertScreen(t, m, []string{
		"   1      - x",
		"gone.txt  1/1  +0 −3",
	})

	m = press(m, "z", "c")
	assertScreen(t, m, []string{
		"⋯ 3 gelöschte Zeilen ⋯",
		"gone.txt  1/1  +0 −3",
	})
}

// TestDeletedFileCollapsesWithZMAndOpensWithZR pins that the whole-file
// collapse joins the fold commands: zM closes it, zR opens it.
func TestDeletedFileCollapsesWithZMAndOpensWithZR(t *testing.T) {
	gone := diff.New("gone.txt", "x\n", "")
	m := start(ui.New([]diff.ReviewFile{gone}, "main", themeFor, ui.Options{}), 30, 2)

	m = press(m, "z", "R")
	assertScreen(t, m, []string{
		"   1      - x",
		"gone.txt  1/1  +0 −1",
	})

	m = press(m, "z", "M")
	assertScreen(t, m, []string{
		"⋯ 1 gelöschte Zeile ⋯",
		"gone.txt  1/1  +0 −1",
	})
}

// TestGeneratedFileIsCollapsed pins the acceptance criterion
// "generierte Dateien eingeklappt": a vendor file starts collapsed and
// expands like a deleted file.
func TestGeneratedFileIsCollapsed(t *testing.T) {
	gen := diff.NewWithAttrs("vendor/lib.go", "b\n", "B\n", func(string) bool { return true })
	m := startNoMinimap(ui.New([]diff.ReviewFile{gen}, "main", themeFor, ui.Options{}), 30, 2)

	assertScreen(t, m, []string{
		"⋯ generierte Datei: 2 Zeilen ⋯",
		"vendor/lib.go  1/1  +1 −1",
	})

	m = press(m, "z", "o")
	assertScreen(t, m, []string{
		"   1      - b",
		"vendor/lib.go  1/1  +1 −1",
	})
}

// TestFoldKeysDoNotCollapseRegularFiles pins that zc leaves a regular
// file expanded when the cursor is not on a fold.
func TestFoldKeysDoNotCollapseRegularFiles(t *testing.T) {
	regular := diff.New("f.txt", "a\n", "A\n")
	m := start(ui.New([]diff.ReviewFile{regular}, "main", themeFor, ui.Options{}), 30, 5)

	m = press(m, "z", "c", "z", "M")
	screen := screen(m)
	if !strings.Contains(screen, "- a") || !strings.Contains(screen, "+ A") {
		t.Errorf("zc/zM collapsed a regular file:\n%s", screen)
	}
}

// TestNewFileHasNoBackgroundWash pins the acceptance criterion "neue
// Dateien ohne flächig eingefärbten Hintergrund": an added file shows
// only the gutter markers, while a changed line of an existing file
// keeps the wash.
func TestNewFileHasNoBackgroundWash(t *testing.T) {
	added := diff.New("new.go", "", "package main\n")
	m := startNoMinimap(ui.New([]diff.ReviewFile{added}, "main", themeFor, ui.Options{}), 30, 5)

	// #1f2c38 is the added wash of the night theme: the ANSI background
	// sequence must not appear in a new file.
	if strings.Contains(m.View().Content, "48;2;31;44;56") {
		t.Errorf("new file carries the added background wash:\n%s", m.View().Content)
	}

	changed := diff.New("f.go", "package main\n", "package main2\n")
	m = startNoMinimap(ui.New([]diff.ReviewFile{changed}, "main", themeFor, ui.Options{}), 30, 5)
	if !strings.Contains(m.View().Content, "48;2;31;44;56") {
		t.Errorf("changed file lost the added background wash:\n%s", m.View().Content)
	}
}

// TestFileKindMarkersInTheStatusLine pins that the status line shows
// the rename and the option states next to the unified marker.
func TestFileKindMarkersInTheStatusLine(t *testing.T) {
	renamed := diff.New("b.txt", "x\n", "X\n")
	renamed.OldPath = "a.txt"
	m := start(ui.New([]diff.ReviewFile{renamed}, "main", themeFor, ui.Options{}), 60, 5)

	m, _ = enter(typeText(press(m, ":"), "set wrap ignorews"))
	m, _ = enter(typeText(press(m, ":"), "set unified"))

	status := strings.Split(screen(m), "\n")
	last := status[len(status)-1]
	for _, want := range []string{"a.txt → b.txt", "[wrap]", "[ignorews]", "[unified]"} {
		if !strings.Contains(last, want) {
			t.Errorf("status line lacks %q:\n%s", want, last)
		}
	}
}
