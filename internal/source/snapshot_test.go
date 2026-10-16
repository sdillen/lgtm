package source_test

import (
	"testing"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
	"github.com/go-git/go-git/v6/plumbing"

	"github.com/sdillen/lgtm/internal/source"
)

const snapRef = "refs/lgtm/review/1"

// snapshot records the working tree under snapRef.
func snapshot(t *testing.T, src *source.Source) {
	t.Helper()
	if err := src.Snapshot(snapRef); err != nil {
		t.Fatal(err)
	}
}

// since resolves the changes since the snapshot.
func since(t *testing.T, src *source.Source) source.Changes {
	t.Helper()
	got, err := src.Since(snapRef)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestSinceASnapshotOfTheUnchangedWorkingTreeShowsNothing(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "a\nb\n")  // uncommitted edit
	f.write("new.txt", "new\n") // untracked file
	f.write("dir/x.go", "x\n")  // untracked file in a directory
	src := source.New(f.repo)

	snapshot(t, src)

	assertFiles(t, since(t, src), nil)
}

func TestSinceShowsOnlyTheChangesAfterTheSnapshot(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.write("keep.txt", "keep\n")
	f.write("gone.txt", "gone\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "a\nb\n")
	f.write("new.txt", "new\n")
	src := source.New(f.repo)
	snapshot(t, src)

	f.write("a.txt", "a\nb\nc\n")
	f.write("new.txt", "newer\n")
	f.remove("gone.txt")
	f.write("later.txt", "later\n")
	f.commit("agent round 2")
	f.write("keep.txt", "kept\n")

	got := since(t, src)

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "a\nb\n", New: "a\nb\nc\n"},
		{Path: "gone.txt", Status: source.Deleted, Old: "gone\n"},
		{Path: "keep.txt", Status: source.Modified, Old: "keep\n", New: "kept\n"},
		{Path: "later.txt", Status: source.Added, New: "later\n"},
		{Path: "new.txt", Status: source.Modified, Old: "new\n", New: "newer\n"},
	})
	if got.Base != snapRef {
		t.Errorf("Base = %q, want %q", got.Base, snapRef)
	}
}

func TestSnapshotDoesNotTouchTheIndexOrHead(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	f.branch("feature")
	f.write("a.txt", "b\n")
	f.write("new.txt", "new\n")
	src := source.New(f.repo)
	before := defaultChanges(t, f)
	head, err := f.repo.Head()
	if err != nil {
		t.Fatal(err)
	}

	snapshot(t, src)

	after := defaultChanges(t, f)
	assertFiles(t, after, before.Files)
	now, err := f.repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	if now.Hash() != head.Hash() || now.Name() != head.Name() {
		t.Errorf("HEAD moved from %v to %v", head, now)
	}
	st, err := f.worktree().Status()
	if err != nil {
		t.Fatal(err)
	}
	if s := st.File("new.txt"); s.Staging != ' ' && s.Staging != '?' {
		t.Errorf("new.txt was staged: %q", s.Staging)
	}
}

func TestSnapshotIsAReachableCommitUnderItsRef(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	f.write("a.txt", "b\n")
	src := source.New(f.repo)

	snapshot(t, src)

	ref, err := f.repo.Reference(plumbing.ReferenceName(snapRef), true)
	if err != nil {
		t.Fatalf("snapshot ref: %v", err)
	}
	c, err := f.repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatalf("snapshot is no commit: %v", err)
	}
	file, err := c.File("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := file.Contents(); got != "b\n" {
		t.Errorf("snapshot a.txt = %q, want the uncommitted %q", got, "b\n")
	}
}

func TestSnapshotOverwritesItsRef(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	src := source.New(f.repo)
	f.write("a.txt", "b\n")
	snapshot(t, src)

	f.write("a.txt", "c\n")
	snapshot(t, src)

	assertFiles(t, since(t, src), nil)
}

func TestSnapshotKeepsModesSymlinksAndNestedDirectories(t *testing.T) {
	f := newFixture(t, "main")
	f.write("bin/run.sh", "#!/bin/sh\n")
	f.write("deep/a/b/c.txt", "c\n")
	f.write("z.txt", "z\n")
	f.commit("base")
	if err := util.WriteFile(f.wt, "bin/new.sh", []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.wt.Symlink("z.txt", "link"); err != nil {
		t.Fatal(err)
	}
	f.write("deep/a/new.txt", "n\n")
	src := source.New(f.repo)

	snapshot(t, src)

	assertFiles(t, since(t, src), nil)
}

func TestSinceWorksInLinkedWorktree(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	linked := f.linkedWorktreeOn(memfs.New(), "agent")
	linked.write("a.txt", "b\n")
	src := source.New(linked.repo)
	snapshot(t, src)

	linked.write("a.txt", "c\n")

	assertFiles(t, since(t, src), []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "b\n", New: "c\n"},
	})
}

func TestSinceFailsForAMissingSnapshot(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")

	if _, err := source.New(f.repo).Since(snapRef); err == nil {
		t.Fatal("Since resolved a snapshot that was never taken")
	}
}

func TestRangeResolvesASnapshotRef(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	f.write("a.txt", "b\n")
	second := f.commit("second")
	src := source.New(f.repo)
	if err := src.SnapshotRev(snapRef, "HEAD~1"); err != nil {
		t.Fatal(err)
	}

	got, err := src.Range(snapRef, second.String())
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "a\n", New: "b\n"},
	})
}

func TestDeleteSnapshotsRemovesOnlyTheRefsUnderThePrefix(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.txt", "a\n")
	f.commit("base")
	src := source.New(f.repo)
	for _, ref := range []string{"refs/lgtm/mine/1", "refs/lgtm/mine/2", "refs/lgtm/other/1"} {
		if err := src.Snapshot(ref); err != nil {
			t.Fatal(err)
		}
	}

	if err := src.DeleteSnapshots("refs/lgtm/mine/"); err != nil {
		t.Fatal(err)
	}

	for ref, want := range map[string]bool{
		"refs/lgtm/mine/1": false, "refs/lgtm/mine/2": false, "refs/lgtm/other/1": true,
	} {
		_, err := f.repo.Reference(plumbing.ReferenceName(ref), false)
		if got := err == nil; got != want {
			t.Errorf("%s exists = %v, want %v", ref, got, want)
		}
	}
}
