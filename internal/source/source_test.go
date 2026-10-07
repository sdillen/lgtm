package source_test

import (
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/source"
)

func TestDefaultCombinesCommitsSinceMergeBaseWithUncommittedChanges(t *testing.T) {
	f := newFixture(t, "main")
	f.write("committed.txt", "one\n")
	f.write("dirty.txt", "a\n")
	f.commit("base")
	f.branch("feature")
	f.write("committed.txt", "one\ntwo\n")
	f.commit("feature work")
	f.write("dirty.txt", "a\nb\n")

	got := defaultChanges(t, f)

	assertFiles(t, got, []source.File{
		{Path: "committed.txt", Status: source.Modified, Old: "one\n", New: "one\ntwo\n"},
		{Path: "dirty.txt", Status: source.Modified, Old: "a\n", New: "a\nb\n"},
	})
	if got.Base != "main" {
		t.Errorf("Base = %q, want %q", got.Base, "main")
	}
}

func TestDefaultBranchIsTakenFromRemoteHEAD(t *testing.T) {
	f := newFixture(t, "main")
	f.write("base.txt", "base\n")
	f.commit("on main")
	f.branch("feature")
	f.write("develop.txt", "develop\n")
	develop := f.commit("on develop")
	f.setRef("refs/remotes/origin/develop", develop)
	f.setSymbolicRef("refs/remotes/origin/HEAD", "refs/remotes/origin/develop")
	f.write("feature.txt", "feature\n")
	f.commit("on feature")

	got := defaultChanges(t, f)

	assertFiles(t, got, []source.File{
		{Path: "feature.txt", Status: source.Added, New: "feature\n"},
	})
	if got.Base != "origin/develop" {
		t.Errorf("Base = %q, want %q", got.Base, "origin/develop")
	}
}

func TestDefaultBranchFallsBackToMaster(t *testing.T) {
	f := newFixture(t, "master")
	f.write("a.txt", "a\n")
	f.commit("on master")
	f.branch("feature")
	f.write("a.txt", "b\n")
	f.commit("on feature")

	got := defaultChanges(t, f)

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "a\n", New: "b\n"},
	})
	if got.Base != "master" {
		t.Errorf("Base = %q, want %q", got.Base, "master")
	}
}

func TestDefaultFailsWithoutDefaultBranch(t *testing.T) {
	f := newFixture(t, "trunk")
	f.write("a.txt", "a\n")
	f.commit("on trunk")

	if _, err := source.New(f.repo).Default(); err == nil {
		t.Fatal("want error when neither remote HEAD nor main/master exist")
	}
}

func TestDefaultIgnoresChangesOnDefaultBranchAfterTheFork(t *testing.T) {
	f := newFixture(t, "main")
	f.write("shared.txt", "v1\n")
	f.commit("base")
	f.branch("feature")
	f.write("feature.txt", "feature\n")
	f.commit("on feature")
	f.checkout("main")
	f.write("shared.txt", "v2\n")
	f.commit("main moves on")
	f.checkout("feature")

	got := defaultChanges(t, f)

	assertFiles(t, got, []source.File{
		{Path: "feature.txt", Status: source.Added, New: "feature\n"},
	})
}

func TestDefaultReportsAddedDeletedAndUntrackedFiles(t *testing.T) {
	f := newFixture(t, "main")
	f.write("committed-delete.txt", "x\n")
	f.write("dirty-delete.txt", "y\n")
	f.commit("base")
	f.branch("feature")
	f.remove("committed-delete.txt")
	f.write("committed-add.txt", "c\n")
	f.commit("on feature")
	f.remove("dirty-delete.txt")
	f.write("dir/untracked.txt", "u\n")

	got := defaultChanges(t, f)

	assertFiles(t, got, []source.File{
		{Path: "committed-add.txt", Status: source.Added, New: "c\n"},
		{Path: "committed-delete.txt", Status: source.Deleted, Old: "x\n"},
		{Path: "dir/untracked.txt", Status: source.Added, New: "u\n"},
		{Path: "dirty-delete.txt", Status: source.Deleted, Old: "y\n"},
	})
}

func TestDefaultDropsFilesWhoseChangesWereReverted(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "changed\n")
	f.commit("on feature")
	f.write("a.txt", "a\n")

	got := defaultChanges(t, f)

	assertFiles(t, got, nil)
}

func TestDefaultWorksInLinkedWorktree(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	linked := f.linkedWorktree("agent")
	linked.write("a.txt", "a\nb\n")
	linked.commit("agent work")
	linked.write("new.txt", "n\n")

	got := defaultChanges(t, linked)

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "a\n", New: "a\nb\n"},
		{Path: "new.txt", Status: source.Added, New: "n\n"},
	})
	if got.Base != "main" {
		t.Errorf("Base = %q, want %q", got.Base, "main")
	}
}

func TestOpenOutsideRepositoryFailsWithReadableError(t *testing.T) {
	dir := t.TempDir()

	_, err := source.Open(dir)

	if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("err = %v, want a 'not inside a git repository' error", err)
	}
}

func defaultChanges(t *testing.T, f *fixture) source.Changes {
	t.Helper()
	got, err := source.New(f.repo).Default()
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertFiles(t *testing.T, got source.Changes, want []source.File) {
	t.Helper()
	if len(got.Files) != len(want) {
		t.Fatalf("got %d files, want %d:\n got  %+v\n want %+v", len(got.Files), len(want), got.Files, want)
	}
	for i := range want {
		if got.Files[i] != want[i] {
			t.Errorf("file %d = %+v, want %+v", i, got.Files[i], want[i])
		}
	}
}
