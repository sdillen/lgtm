package source_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/source"
)

// TestRangeReportsSubmoduleChanges pins that a gitlink change between
// two trees becomes a submodule file entry whose Old and New hold the
// commit hashes, so the UI can show a compact info entry instead of
// trying to read the submodule contents.
func TestRangeReportsSubmoduleChanges(t *testing.T) {
	f := newFixture(t, "main")
	fileBlob := f.blob("base\n")
	sub1 := plumbing.NewHash(strings.Repeat("a", 40))
	sub2 := plumbing.NewHash(strings.Repeat("b", 40))
	baseTree := f.rawTree([]object.TreeEntry{
		{Name: "a.txt", Mode: filemode.Regular, Hash: fileBlob},
		{Name: "lib", Mode: filemode.Submodule, Hash: sub1},
	})
	baseCommit := f.rawCommit("refs/heads/main", "base", baseTree)
	fileBlob2 := f.blob("changed\n")
	featureTree := f.rawTree([]object.TreeEntry{
		{Name: "a.txt", Mode: filemode.Regular, Hash: fileBlob2},
		{Name: "lib", Mode: filemode.Submodule, Hash: sub2},
	})
	f.rawCommit("refs/heads/feature", "update submodule", featureTree, baseCommit)

	got, err := source.New(f.repo).Range("main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "a.txt", Status: source.Modified, Old: "base\n", New: "changed\n"},
		{Path: "lib", Status: source.Modified, Submodule: true, Old: sub1.String(), New: sub2.String()},
	})
}

// TestRangeReportsAddedAndDeletedSubmodules pins the gitlink add and
// delete cases: the missing side has no hash.
func TestRangeReportsAddedAndDeletedSubmodules(t *testing.T) {
	f := newFixture(t, "main")
	fileBlob := f.blob("a\n")
	sub1 := plumbing.NewHash(strings.Repeat("a", 40))
	baseTree := f.rawTree([]object.TreeEntry{
		{Name: "a.txt", Mode: filemode.Regular, Hash: fileBlob},
	})
	baseCommit := f.rawCommit("refs/heads/main", "base", baseTree)
	addedTree := f.rawTree([]object.TreeEntry{
		{Name: "a.txt", Mode: filemode.Regular, Hash: fileBlob},
		{Name: "new-lib", Mode: filemode.Submodule, Hash: sub1},
	})
	addedCommit := f.rawCommit("refs/heads/add", "add submodule", addedTree, baseCommit)
	deletedTree := f.rawTree([]object.TreeEntry{
		{Name: "a.txt", Mode: filemode.Regular, Hash: fileBlob},
	})
	f.rawCommit("refs/heads/del", "delete submodule", deletedTree, addedCommit)

	got, err := source.New(f.repo).Range("add", "del")
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, got, []source.File{
		{Path: "new-lib", Status: source.Deleted, Submodule: true, Old: sub1.String()},
	})

	got, err = source.New(f.repo).Range("main", "add")
	if err != nil {
		t.Fatal(err)
	}
	assertFiles(t, got, []source.File{
		{Path: "new-lib", Status: source.Added, Submodule: true, New: sub1.String()},
	})
}

// TestDefaultIncludesCommittedSubmoduleChanges pins that the default
// review subject carries submodule changes of the committed span
// instead of failing to read the gitlink contents. The working tree
// matches the reviewed state: the gitlink is staged and .gitmodules
// names the submodule.
func TestDefaultIncludesCommittedSubmoduleChanges(t *testing.T) {
	f := newFixture(t, "main")
	gitmodules := "[submodule \"lib\"]\n\tpath = lib\n\turl = https://example.com/lib\n"
	f.write(".gitmodules", gitmodules)
	f.write("a.txt", "base\n")
	f.commit("base")
	sub1 := plumbing.NewHash(strings.Repeat("a", 40))
	sub2 := plumbing.NewHash(strings.Repeat("b", 40))
	baseTree := f.rawTree([]object.TreeEntry{
		{Name: ".gitmodules", Mode: filemode.Regular, Hash: f.blob(gitmodules)},
		{Name: "a.txt", Mode: filemode.Regular, Hash: f.blob("base\n")},
		{Name: "lib", Mode: filemode.Submodule, Hash: sub1},
	})
	baseCommit := f.rawCommit("refs/heads/main", "base", baseTree)
	featureTree := f.rawTree([]object.TreeEntry{
		{Name: ".gitmodules", Mode: filemode.Regular, Hash: f.blob(gitmodules)},
		{Name: "a.txt", Mode: filemode.Regular, Hash: f.blob("base\n")},
		{Name: "lib", Mode: filemode.Submodule, Hash: sub2},
	})
	f.rawCommit("refs/heads/feature", "update submodule", featureTree, baseCommit)
	f.stageGitlink("lib", sub2)
	if err := f.wt.MkdirAll("lib", 0o755); err != nil {
		t.Fatal(err)
	}
	f.setSymbolicRef("HEAD", "refs/heads/feature")

	got := defaultChanges(t, f)

	assertFiles(t, got, []source.File{
		{Path: "lib", Status: source.Modified, Submodule: true, Old: sub1.String(), New: sub2.String()},
	})
}

// TestRangeReportsModeOnlyChanges pins that a change of the executable
// bit alone becomes a mode change entry carrying both modes, so the UI
// can show a compact info entry instead of an empty diff.
func TestRangeReportsModeOnlyChanges(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("script.sh", "#!/bin/sh\n")
	f.commit("base")
	f.branch("feature")
	if err := os.Chmod(filepath.Join(f.wt.Root(), "script.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	f.commit("chmod")

	got, err := source.New(f.repo).Range("main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "script.sh", Status: source.Modified, Old: "#!/bin/sh\n", New: "#!/bin/sh\n", OldMode: 0o100644, NewMode: 0o100755},
	})
}

// TestRangeReportsBinaryFiles pins that binary contents are carried
// through unchanged; the classification into a size-only entry happens
// in the diff module.
func TestRangeReportsBinaryFiles(t *testing.T) {
	f := newFixture(t, "main")
	f.write("img.png", "\x00\x01\x02")
	f.commit("base")
	f.branch("feature")
	f.write("img.png", "\x00\x01\x02\x03")
	f.commit("changed")

	got, err := source.New(f.repo).Range("main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	assertFiles(t, got, []source.File{
		{Path: "img.png", Status: source.Modified, Old: "\x00\x01\x02", New: "\x00\x01\x02\x03"},
	})
}

// TestDefaultCarriesTheGitAttributesOfTheWorktree pins that the review
// subject carries the .gitattributes of the reviewed state, so the diff
// module can mark linguist-generated paths.
func TestDefaultCarriesTheGitAttributesOfTheWorktree(t *testing.T) {
	f := newFixture(t, "main")
	f.write(".gitattributes", "*.pb.go linguist-generated\n")
	f.write("api.pb.go", "x\n")
	f.commit("base")
	f.branch("feature")
	f.write("api.pb.go", "y\n")
	f.commit("feature work")
	f.write("new.pb.go", "n\n")

	got := defaultChanges(t, f)

	attrs := diff.ParseGitAttributes(got.GitAttributes)
	if !attrs.Generated("api.pb.go") || !attrs.Generated("new.pb.go") {
		t.Errorf("GitAttributes %q does not mark the pb.go files", got.GitAttributes)
	}
	if attrs.Generated("main.go") {
		t.Errorf("GitAttributes %q marks main.go", got.GitAttributes)
	}
}

// TestRangeCarriesTheGitAttributesOfTheNewSide pins that a range review
// reads the .gitattributes of the new side of the range.
func TestRangeCarriesTheGitAttributesOfTheNewSide(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.md", "x\n")
	f.commit("base")
	f.branch("feature")
	f.write(".gitattributes", "*.md linguist-generated\n")
	f.commit("add attributes")

	got, err := source.New(f.repo).Range("main", "feature")
	if err != nil {
		t.Fatal(err)
	}

	attrs := diff.ParseGitAttributes(got.GitAttributes)
	if !attrs.Generated("a.md") {
		t.Errorf("GitAttributes %q does not mark a.md", got.GitAttributes)
	}
}

// TestCommitCarriesTheGitAttributesOfTheCommit pins the single-commit
// review subject.
func TestCommitCarriesTheGitAttributesOfTheCommit(t *testing.T) {
	f := newFixture(t, "main")
	f.write("a.md", "x\n")
	f.commit("base")
	f.branch("feature")
	f.write(".gitattributes", "*.md linguist-generated\n")
	h := f.commit("add attributes")

	got, err := source.New(f.repo).Commit(h.String())
	if err != nil {
		t.Fatal(err)
	}

	attrs := diff.ParseGitAttributes(got.GitAttributes)
	if !attrs.Generated("a.md") {
		t.Errorf("GitAttributes %q does not mark a.md", got.GitAttributes)
	}
}
