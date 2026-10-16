package source_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-billy/v6/osfs"
	git "github.com/go-git/go-git/v6"

	"github.com/sdillen/lgtm/internal/source"
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

func TestOpenFindsLinkedWorktreeOnDisk(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	dir := t.TempDir()
	linked := f.linkedWorktreeOn(osfs.New(dir), "agent")
	linked.write("sub/a.txt", "s\n")
	linked.commit("agent work")
	linked.write("a.txt", "a\nb\n")

	src, err := source.Open(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := src.Default()
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "a\n", New: "a\nb\n"},
		{Path: "sub/a.txt", Status: source.Added, New: "s\n"},
	})
}

func TestRangeShowsChangesBetweenTwoCommits(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "one\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "two\n")
	f.write("b.txt", "b\n")
	f.commit("feature work")
	f.write("dirty.txt", "dirty\n")

	got, err := source.New(f.repo).Range("main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "one\n", New: "two\n"},
		{Path: "b.txt", Status: source.Added, New: "b\n"},
	})
	if got.Base != "main" {
		t.Errorf("Base = %q, want %q", got.Base, "main")
	}
}

func TestRangeIgnoresUncommittedChanges(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "one\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "two\n")
	f.commit("feature work")
	f.write("a.txt", "three\n")

	got, err := source.New(f.repo).Range("main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "one\n", New: "two\n"},
	})
}

func TestRangeEmptySpanShowsNoChanges(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")

	got, err := source.New(f.repo).Range("main", "main")
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, nil)
}

func TestRangeFailsWhenRefCannotBeResolved(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")

	for _, rev := range []string{"main", "feature"} {
		_, err := source.New(f.repo).Range("does-not-exist", rev)
		if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
			t.Errorf("Range(does-not-exist, %s) err = %v, want an error naming the ref", rev, err)
		}
	}
	_, err := source.New(f.repo).Range("main", "does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("Range(main, does-not-exist) err = %v, want an error naming the ref", err)
	}
}

func TestCommitShowsChangesAgainstParent(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "one\n")
	base := f.commit("base")
	f.write("a.txt", "two\n")
	f.write("b.txt", "b\n")
	second := f.commit("second")
	f.write("dirty.txt", "dirty\n")

	got, err := source.New(f.repo).Commit(second.String())
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "one\n", New: "two\n"},
		{Path: "b.txt", Status: source.Added, New: "b\n"},
	})
	if got.Base != base.String()[:7] {
		t.Errorf("Base = %q, want %q", got.Base, base.String()[:7])
	}
}

func TestCommitOfRootCommitShowsEveryFileAsAdded(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	root := f.commit("root")

	got, err := source.New(f.repo).Commit(root.String())
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Added, New: "a\n"},
	})
}

func TestCommitOfMergeShowsChangesAgainstFirstParent(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "base\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "feature\n")
	feature := f.commit("feature work")
	f.checkout("main")
	f.write("main.txt", "main\n")
	mainWork := f.commit("main work")
	// Fabricate the merge commit: main takes feature's a.txt and keeps
	// its own main.txt.
	f.write("a.txt", "feature\n")
	merged := f.commitWithParents("merge feature", mainWork, feature)

	got, err := source.New(f.repo).Commit(merged.String())
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "base\n", New: "feature\n"},
	})
}

func TestCommitFailsWhenRefCannotBeResolved(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")

	_, err := source.New(f.repo).Commit("does-not-exist")
	if err == nil || !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("err = %v, want an error naming the ref", err)
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

// TestWorktreeDirNamesTheWorkingTreeRoot pins the directory the review
// state persists to: the root of the working tree, not the directory the
// program was started in.
func TestWorktreeDirNamesTheWorkingTreeRoot(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")

	src, err := source.Open(f.wt.Root())
	if err != nil {
		t.Fatal(err)
	}
	root := src.WorktreeDir()
	if root == "" {
		t.Fatal("WorktreeDir is empty")
	}
	fi, err := os.Stat(filepath.Join(root, "a.txt"))
	if err != nil {
		t.Fatalf("WorktreeDir %q is not the worktree root: %v", root, err)
	}
	if !fi.Mode().IsRegular() {
		t.Errorf("%s is not a regular file", filepath.Join(root, "a.txt"))
	}
}

// TestWorktreeDirWorksFromASubdirectory pins that opening the repository
// from a subdirectory still yields the worktree root.
func TestWorktreeDirWorksFromASubdirectory(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("sub/a.txt", "a\n")
	f.commit("base")

	src, err := source.Open(f.wt.Root())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := src.WorktreeDir(), f.wt.Root(); got != want {
		t.Errorf("WorktreeDir = %q, want %q", got, want)
	}
}

// TestHeadBranchNamesTheCurrentBranch pins the template data of the
// prompt: the branch HEAD points at.
func TestHeadBranchNamesTheCurrentBranch(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	f.branch("feature")

	src := source.New(f.repo)
	if got := src.HeadBranch(); got != "feature" {
		t.Errorf("HeadBranch = %q, want %q", got, "feature")
	}
}

// TestHeadBranchEmptyWhenDetached pins that a detached HEAD has no
// branch name.
func TestHeadBranchEmptyWhenDetached(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	h := f.commit("base")
	if err := f.worktree().Checkout(&git.CheckoutOptions{Hash: h}); err != nil {
		t.Fatal(err)
	}

	src := source.New(f.repo)
	if got := src.HeadBranch(); got != "" {
		t.Errorf("HeadBranch = %q, want empty for a detached HEAD", got)
	}
}

// TestDefaultHonoursInfoExclude: like git status, untracked files
// excluded by .git/info/exclude are not part of the review, e.g. lgtm's
// own .lgtm/ directory. The same holds for a linked worktree, whose
// info/exclude lives in the common git dir.
func TestDefaultHonoursInfoExclude(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	root := f.wt.Root()
	if err := os.MkdirAll(filepath.Join(root, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "info", "exclude"), []byte("# local\n.lgtm/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.write(".lgtm/state.json", "{}")
	f.write("b.txt", "b\n")
	dir := t.TempDir()
	linked := f.linkedWorktreeOn(osfs.New(dir), "agent")
	linked.write(".lgtm/state.json", "{}")
	linked.write("c.txt", "c\n")

	for name, wt := range map[string]string{"main": root, "linked": dir} {
		src, err := source.Open(wt)
		if err != nil {
			t.Fatal(err)
		}
		got, err := src.Default()
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range got.Files {
			if strings.HasPrefix(file.Path, ".lgtm/") {
				t.Errorf("%s: review lists the excluded %s", name, file.Path)
			}
		}
		files, err := src.Files()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range files {
			if strings.HasPrefix(p, ".lgtm/") {
				t.Errorf("%s: Files lists the excluded %s", name, p)
			}
		}
	}
}
