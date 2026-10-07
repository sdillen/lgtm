package source_test

import (
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-billy/v6/util"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"
	xworktree "github.com/go-git/go-git/v6/x/plumbing/worktree"
)

// fixture is an in-memory git repository with a working tree. Its git
// directory lives on a memfs as well, so linked worktrees can be added.
type fixture struct {
	t    *testing.T
	repo *git.Repository
	wt   billy.Filesystem
}

func newFixture(t *testing.T, defaultBranch string) *fixture {
	t.Helper()
	dot, wt := memfs.New(), memfs.New()
	storage := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())
	repo, err := git.Init(storage,
		git.WithWorkTree(wt),
		git.WithDefaultBranch(plumbing.NewBranchReferenceName(defaultBranch)))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, repo: repo, wt: wt}
}

// newDiskFixture is like newFixture, but the repository lives in a
// temporary directory, so the production entry point Open can be tested.
func newDiskFixture(t *testing.T, defaultBranch string) *fixture {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false,
		git.WithDefaultBranch(plumbing.NewBranchReferenceName(defaultBranch)))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, repo: repo, wt: osfs.New(dir)}
}

func (f *fixture) write(path, content string) {
	f.t.Helper()
	if err := util.WriteFile(f.wt, path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) remove(path string) {
	f.t.Helper()
	if err := f.wt.Remove(path); err != nil {
		f.t.Fatal(err)
	}
}

// commit stages everything in the working tree and commits it.
func (f *fixture) commit(msg string) plumbing.Hash {
	f.t.Helper()
	w := f.worktree()
	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		f.t.Fatal(err)
	}
	h, err := w.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Unix(0, 0)},
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return h
}

// branch creates branch name at HEAD and checks it out, keeping the
// working tree.
func (f *fixture) branch(name string) {
	f.t.Helper()
	err := f.worktree().Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Create: true,
		Keep:   true,
	})
	if err != nil {
		f.t.Fatal(err)
	}
}

// checkout switches to an existing branch.
func (f *fixture) checkout(name string) {
	f.t.Helper()
	err := f.worktree().Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(name)})
	if err != nil {
		f.t.Fatal(err)
	}
}

// setRef points ref at the commit h.
func (f *fixture) setRef(ref string, h plumbing.Hash) {
	f.t.Helper()
	if err := f.repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(ref), h)); err != nil {
		f.t.Fatal(err)
	}
}

// setSymbolicRef points ref at target, like refs/remotes/origin/HEAD.
func (f *fixture) setSymbolicRef(ref, target string) {
	f.t.Helper()
	r := plumbing.NewSymbolicReference(plumbing.ReferenceName(ref), plumbing.ReferenceName(target))
	if err := f.repo.Storer.SetReference(r); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) worktree() *git.Worktree {
	f.t.Helper()
	w, err := f.repo.Worktree()
	if err != nil {
		f.t.Fatal(err)
	}
	return w
}

// linkedWorktree adds a linked worktree on a new branch name at HEAD, like
// `git worktree add`, and returns a fixture operating on it.
func (f *fixture) linkedWorktree(name string) *fixture {
	f.t.Helper()
	return f.linkedWorktreeOn(memfs.New(), name)
}

// linkedWorktreeOn is like linkedWorktree, but checks out into wt.
func (f *fixture) linkedWorktreeOn(wt billy.Filesystem, name string) *fixture {
	f.t.Helper()
	m, err := xworktree.New(f.repo.Storer)
	if err != nil {
		f.t.Fatal(err)
	}
	if err := m.Add(wt, name); err != nil {
		f.t.Fatal(err)
	}
	repo, err := m.Open(wt)
	if err != nil {
		f.t.Fatal(err)
	}
	return &fixture{t: f.t, repo: repo, wt: wt}
}
