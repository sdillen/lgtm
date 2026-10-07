package diff_test

import (
	"reflect"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

// TestRenamesPairsAnIdenticalAddedAndDeletedFile pins the simplest
// rename: identical content moves to a new path.
func TestRenamesPairsAnIdenticalAddedAndDeletedFile(t *testing.T) {
	got := diff.Renames([]diff.File{
		{Path: "a.go", Status: diff.StatusDeleted, Old: "package a\n"},
		{Path: "b.go", Status: diff.StatusAdded, New: "package a\n"},
	})

	want := []diff.File{
		{Path: "b.go", OldPath: "a.go", Status: diff.StatusModified, Old: "package a\n", New: "package a\n"},
	}
	assertRenames(t, got, want)
}

// TestRenamesKeepsAnEditedFilePair pins that a rename with edits keeps
// the old content, so the UI can show the content diff of the move.
func TestRenamesKeepsAnEditedFilePair(t *testing.T) {
	got := diff.Renames([]diff.File{
		{Path: "old.go", Status: diff.StatusDeleted, Old: "package p\n\nfunc A() {}\n"},
		{Path: "new.go", Status: diff.StatusAdded, New: "package p\n\nfunc A() {}\n\nfunc B() {}\n"},
	})

	want := []diff.File{
		{Path: "new.go", OldPath: "old.go", Status: diff.StatusModified, Old: "package p\n\nfunc A() {}\n", New: "package p\n\nfunc A() {}\n\nfunc B() {}\n"},
	}
	assertRenames(t, got, want)
}

// TestRenamesAtExactlyFiftyPercent pins the threshold: a similarity of
// exactly 50% is a rename, one of 44% is not.
func TestRenamesAtExactlyFiftyPercent(t *testing.T) {
	// 2 common of 8 lines total: 2*2/8 = 0.5.
	got := diff.Renames([]diff.File{
		{Path: "a.txt", Status: diff.StatusDeleted, Old: "x\nx\n"},
		{Path: "b.txt", Status: diff.StatusAdded, New: "x\nx\na\nb\nc\nd\n"},
	})
	if len(got) != 1 || got[0].OldPath != "a.txt" {
		t.Errorf("50%% similarity is not a rename: %+v", got)
	}

	// 2 common of 9 lines total: 4/9 < 0.5.
	got = diff.Renames([]diff.File{
		{Path: "a.txt", Status: diff.StatusDeleted, Old: "x\nx\n"},
		{Path: "b.txt", Status: diff.StatusAdded, New: "x\nx\na\nb\nc\nd\ne\n"},
	})
	if len(got) != 2 {
		t.Errorf("44%% similarity became a rename: %+v", got)
	}
}

// TestRenamesPickTheBestMatch pins the greedy pairing: every added file
// is paired with the deleted file it resembles most, each file only
// once, ties broken by path.
func TestRenamesPickTheBestMatch(t *testing.T) {
	got := diff.Renames([]diff.File{
		{Path: "a.go", Status: diff.StatusDeleted, Old: "package x\n"},
		{Path: "b.go", Status: diff.StatusDeleted, Old: "package y\n"},
		{Path: "c.go", Status: diff.StatusAdded, New: "package x\n"},
		{Path: "d.go", Status: diff.StatusAdded, New: "package y\n"},
	})

	want := []diff.File{
		{Path: "c.go", OldPath: "a.go", Status: diff.StatusModified, Old: "package x\n", New: "package x\n"},
		{Path: "d.go", OldPath: "b.go", Status: diff.StatusModified, Old: "package y\n", New: "package y\n"},
	}
	assertRenames(t, got, want)
}

// TestRenamesLeaveDissimilarAndUnpairedFilesAlone pins that files below
// the threshold and files without a partner keep their entries.
func TestRenamesLeaveDissimilarAndUnpairedFilesAlone(t *testing.T) {
	got := diff.Renames([]diff.File{
		{Path: "a.go", Status: diff.StatusDeleted, Old: "one\n"},
		{Path: "b.go", Status: diff.StatusAdded, New: "one\ntwo\nthree\nfour\nfive\n"},
		{Path: "c.go", Status: diff.StatusModified, Old: "x\n", New: "y\n"},
		{Path: "d.go", Status: diff.StatusAdded, New: "only added\n"},
		{Path: "e.go", Status: diff.StatusDeleted, Old: "only deleted\n"},
	})

	want := []diff.File{
		{Path: "a.go", Status: diff.StatusDeleted, Old: "one\n"},
		{Path: "b.go", Status: diff.StatusAdded, New: "one\ntwo\nthree\nfour\nfive\n"},
		{Path: "c.go", Status: diff.StatusModified, Old: "x\n", New: "y\n"},
		{Path: "d.go", Status: diff.StatusAdded, New: "only added\n"},
		{Path: "e.go", Status: diff.StatusDeleted, Old: "only deleted\n"},
	}
	assertRenames(t, got, want)
}

// TestRenamesEmptyFilesAreNotPaired pins that two empty files are not a
// rename: without a status the pair is not detectable anyway, and the
// similarity of two empty contents is defined as 0.
func TestRenamesEmptyFilesAreNotPaired(t *testing.T) {
	got := diff.Renames([]diff.File{
		{Path: "a.txt", Status: diff.StatusDeleted, Old: ""},
		{Path: "b.txt", Status: diff.StatusAdded, New: ""},
	})

	if len(got) != 2 {
		t.Errorf("empty files were paired: %+v", got)
	}
}

// TestRenamesPairSubmodulesWithIdenticalHashes pins that a moved
// submodule is detected like a moved file: the gitlink entries carry
// their commit hashes as content.
func TestRenamesPairSubmodulesWithIdenticalHashes(t *testing.T) {
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	got := diff.Renames([]diff.File{
		{Path: "old/lib", Status: diff.StatusDeleted, Old: hash, Submodule: true},
		{Path: "new/lib", Status: diff.StatusAdded, New: hash, Submodule: true},
	})

	want := []diff.File{
		{Path: "new/lib", OldPath: "old/lib", Status: diff.StatusModified, Old: hash, New: hash, Submodule: true},
	}
	assertRenames(t, got, want)
}

func assertRenames(t *testing.T, got, want []diff.File) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d files, want %d:\n got  %+v\n want %+v", len(got), len(want), got, want)
	}
	for i := range want {
		if !reflect.DeepEqual(got[i], want[i]) {
			t.Errorf("file %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
