package source_test

import (
	"fmt"
	"testing"

	"github.com/sdillen/lgtm/internal/source"
)

// TestFilesListsTrackedAndUntrackedFiles pins the file listing the
// context picker and the project search build on: everything tracked,
// plus untracked files that are not ignored, sorted, with deleted files
// skipped.
func TestFilesListsTrackedAndUntrackedFiles(t *testing.T) {
	f := newFixture(t, "main")
	f.write(".gitignore", "ignored.txt\nvendor/\n")
	f.write("a.txt", "a\n")
	f.write("dir/b.txt", "b\n")
	f.write("gone.txt", "x\n")
	f.commit("base")
	f.remove("gone.txt")
	f.write("u.txt", "u\n")
	f.write("ignored.txt", "i\n")
	f.write("vendor/v.txt", "v\n")

	files, err := source.New(f.repo).Files()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "a.txt", "dir/b.txt", "u.txt"}
	if fmt.Sprint(files) != fmt.Sprint(want) {
		t.Errorf("Files() = %v, want %v", files, want)
	}
}

// TestFilesWorksInAnEmptyRepository pins that the listing needs no
// commits: untracked files are listed, like git status.
func TestFilesWorksInAnEmptyRepository(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")

	files, err := source.New(f.repo).Files()
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(files) != "[a.txt]" {
		t.Errorf("Files() = %v, want [a.txt]", files)
	}
}

// TestReadReturnsTheWorkingTreeVersion pins that Read yields the file
// as it is now, uncommitted changes included.
func TestReadReturnsTheWorkingTreeVersion(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "old\n")
	f.commit("base")
	f.write("a.txt", "new\n")

	got, err := source.New(f.repo).Read("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "new\n" {
		t.Errorf("Read(a.txt) = %q, want the working tree version", got)
	}
}

// TestReadFailsForMissingFiles pins that Read reports a path that is
// not in the working tree.
func TestReadFailsForMissingFiles(t *testing.T) {
	f := newFixture(t, "main")

	if _, err := source.New(f.repo).Read("missing.txt"); err == nil {
		t.Error("Read(missing.txt) did not fail")
	}
}
