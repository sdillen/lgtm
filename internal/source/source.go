// Package source encapsulates all git access. It resolves the review subject
// and yields the old and new version of every changed file.
package source

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"

	"github.com/go-git/go-billy/v6"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// Status tells how a file changed between the old and the new version.
type Status int

const (
	Modified Status = iota
	Added
	Deleted
)

// File is one changed file. Old is empty for added files, New is empty for
// deleted files.
type File struct {
	Path     string
	Status   Status
	Old, New string
}

// Changes is a resolved review subject.
type Changes struct {
	Base  string // short name of the ref the old version is based on
	Files []File // sorted by path
}

// Source reads review subjects from a git repository.
type Source struct {
	repo *git.Repository
}

// Open opens the repository containing dir. Linked worktrees are opened
// together with the git directory they share.
func Open(dir string) (*Source, error) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, fmt.Errorf("%s is not inside a git repository", dir)
	}
	if err != nil {
		return nil, fmt.Errorf("open repository: %w", err)
	}
	return New(repo), nil
}

// New returns a Source reading from repo, which must have a working tree.
func New(repo *git.Repository) *Source {
	return &Source{repo: repo}
}

// Default resolves the default review subject: everything from the merge
// base of HEAD and the default branch up to the working tree, i.e. commits
// and uncommitted changes together.
func (s *Source) Default() (Changes, error) {
	baseRef, err := s.defaultBranch()
	if err != nil {
		return Changes{}, err
	}
	head, err := s.headCommit()
	if err != nil {
		return Changes{}, err
	}
	baseCommit, err := s.repo.CommitObject(baseRef.Hash())
	if err != nil {
		return Changes{}, fmt.Errorf("read %s: %w", baseRef.Name().Short(), err)
	}
	bases, err := head.MergeBase(baseCommit)
	if err != nil {
		return Changes{}, fmt.Errorf("merge base with %s: %w", baseRef.Name().Short(), err)
	}
	if len(bases) == 0 {
		return Changes{}, fmt.Errorf("HEAD has no common history with %s", baseRef.Name().Short())
	}
	baseTree, err := bases[0].Tree()
	if err != nil {
		return Changes{}, fmt.Errorf("read merge base tree: %w", err)
	}
	files, err := s.workingTreeChanges(baseTree, head)
	if err != nil {
		return Changes{}, err
	}
	return Changes{Base: baseRef.Name().Short(), Files: files}, nil
}

// Range resolves a..b: the changes between the trees of the two refs, i.e.
// a commit span without uncommitted working tree changes. Base is a as the
// user gave it.
func (s *Source) Range(a, b string) (Changes, error) {
	aCommit, err := s.resolveCommit(a)
	if err != nil {
		return Changes{}, err
	}
	bCommit, err := s.resolveCommit(b)
	if err != nil {
		return Changes{}, err
	}
	aTree, err := aCommit.Tree()
	if err != nil {
		return Changes{}, fmt.Errorf("read tree of %s: %w", a, err)
	}
	bTree, err := bCommit.Tree()
	if err != nil {
		return Changes{}, fmt.Errorf("read tree of %s: %w", b, err)
	}
	files, err := diffTrees(aTree, bTree)
	if err != nil {
		return Changes{}, err
	}
	return Changes{Base: a, Files: files}, nil
}

// Commit resolves rev: the changes of exactly that commit against its
// first parent, like git show. A root commit is compared against the empty
// tree, so every file appears as added. Base is the short hash of the
// parent, empty for a root commit.
func (s *Source) Commit(rev string) (Changes, error) {
	c, err := s.resolveCommit(rev)
	if err != nil {
		return Changes{}, err
	}
	tree, err := c.Tree()
	if err != nil {
		return Changes{}, fmt.Errorf("read tree of %s: %w", rev, err)
	}
	base := ""
	baseTree := &object.Tree{}
	if c.NumParents() > 0 {
		parent, err := s.repo.CommitObject(c.ParentHashes[0])
		if err != nil {
			return Changes{}, fmt.Errorf("read parent of %s: %w", rev, err)
		}
		baseTree, err = parent.Tree()
		if err != nil {
			return Changes{}, fmt.Errorf("read parent tree of %s: %w", rev, err)
		}
		base = parent.Hash.String()[:7]
	}
	files, err := diffTrees(baseTree, tree)
	if err != nil {
		return Changes{}, err
	}
	return Changes{Base: base, Files: files}, nil
}

// resolveCommit resolves rev (branch, tag, hash, HEAD~n, …) to a commit,
// with an error that names the unresolvable ref.
func (s *Source) resolveCommit(rev string) (*object.Commit, error) {
	hash, err := s.repo.ResolveRevision(plumbing.Revision(rev))
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return nil, fmt.Errorf("cannot resolve %q", rev)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot resolve %q: %w", rev, err)
	}
	if hash == nil {
		return nil, fmt.Errorf("cannot resolve %q", rev)
	}
	c, err := s.repo.CommitObject(*hash)
	if err != nil {
		return nil, fmt.Errorf("%q is not a commit: %w", rev, err)
	}
	return c, nil
}

// diffTrees lists the files that differ between two trees.
func diffTrees(oldTree, newTree *object.Tree) ([]File, error) {
	changes, err := object.DiffTree(oldTree, newTree)
	if err != nil {
		return nil, fmt.Errorf("diff trees: %w", err)
	}
	var files []File
	for _, c := range changes {
		path := c.To.Name
		if path == "" {
			path = c.From.Name
		}
		old, inOld, err := treeContent(oldTree, path)
		if err != nil {
			return nil, err
		}
		new, inNew, err := treeContent(newTree, path)
		if err != nil {
			return nil, err
		}
		if f, changed := fileFor(path, old, inOld, new, inNew); changed {
			files = append(files, f)
		}
	}
	return files, nil
}

// defaultBranch finds the default branch: the HEAD of a remote (origin
// first), otherwise a local main or master.
func (s *Source) defaultBranch() (*plumbing.Reference, error) {
	remotes, err := s.repo.Remotes()
	if err != nil {
		return nil, fmt.Errorf("list remotes: %w", err)
	}
	candidates := []plumbing.ReferenceName{plumbing.NewRemoteHEADReferenceName("origin")}
	for _, r := range remotes {
		if name := r.Config().Name; name != "origin" {
			candidates = append(candidates, plumbing.NewRemoteHEADReferenceName(name))
		}
	}
	candidates = append(candidates,
		plumbing.NewBranchReferenceName("main"),
		plumbing.NewBranchReferenceName("master"))
	for _, name := range candidates {
		ref, err := s.repo.Reference(name, true)
		if err == nil {
			return ref, nil
		}
	}
	return nil, errors.New("cannot determine the default branch: no remote HEAD, main or master")
}

func (s *Source) headCommit() (*object.Commit, error) {
	ref, err := s.repo.Head()
	if err != nil {
		return nil, fmt.Errorf("resolve HEAD: %w", err)
	}
	c, err := s.repo.CommitObject(ref.Hash())
	if err != nil {
		return nil, fmt.Errorf("read HEAD commit: %w", err)
	}
	return c, nil
}

// workingTreeChanges compares base with the working tree. Candidates are the
// paths changed by commits since base plus those changed in the working
// tree; files whose content ends up unchanged are dropped.
func (s *Source) workingTreeChanges(base *object.Tree, head *object.Commit) ([]File, error) {
	wt, err := s.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("open working tree: %w", err)
	}
	headTree, err := head.Tree()
	if err != nil {
		return nil, fmt.Errorf("read HEAD tree: %w", err)
	}
	committed, err := object.DiffTree(base, headTree)
	if err != nil {
		return nil, fmt.Errorf("diff merge base with HEAD: %w", err)
	}
	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("working tree status: %w", err)
	}

	paths := map[string]bool{}
	for _, c := range committed {
		if c.From.Name != "" {
			paths[c.From.Name] = true
		}
		if c.To.Name != "" {
			paths[c.To.Name] = true
		}
	}
	for p, st := range status {
		if st.Staging != git.Unmodified || st.Worktree != git.Unmodified {
			paths[p] = true
		}
	}

	var files []File
	for _, p := range slices.Sorted(maps.Keys(paths)) {
		old, inOld, err := treeContent(base, p)
		if err != nil {
			return nil, err
		}
		cur, inNew, err := fsContent(wt.Filesystem(), p)
		if err != nil {
			return nil, fmt.Errorf("read %s from working tree: %w", p, err)
		}
		if f, changed := fileFor(p, old, inOld, cur, inNew); changed {
			files = append(files, f)
		}
	}
	return files, nil
}

// fileFor builds the File entry for a path, reporting whether the content
// actually changed between the old and the new version.
func fileFor(path, old string, inOld bool, new string, inNew bool) (File, bool) {
	switch {
	case inOld && inNew && old != new:
		return File{Path: path, Status: Modified, Old: old, New: new}, true
	case !inOld && inNew:
		return File{Path: path, Status: Added, New: new}, true
	case inOld && !inNew:
		return File{Path: path, Status: Deleted, Old: old}, true
	}
	return File{}, false
}

func treeContent(tree *object.Tree, path string) (string, bool, error) {
	f, err := tree.File(path)
	if errors.Is(err, object.ErrFileNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	content, err := f.Contents()
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return content, true, nil
}

// fsContent reads path from the working tree. Like git, it yields a
// symlink's target instead of following it.
func fsContent(fs billy.Filesystem, path string) (string, bool, error) {
	fi, err := fs.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := fs.Readlink(path)
		if err != nil {
			return "", false, err
		}
		return target, true, nil
	}
	if !fi.Mode().IsRegular() {
		return "", false, nil
	}
	f, err := fs.Open(path)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}
