// Package source encapsulates all git access. It resolves the review subject
// and yields the old and new version of every changed file.
package source

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-git/go-billy/v6"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
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
// deleted files. Submodule marks a gitlink change, whose Old and New hold
// the commit hashes; OldMode and NewMode carry the file modes of a
// mode-only change.
type File struct {
	Path     string
	Status   Status
	Old, New string

	Submodule bool
	OldMode   uint32
	NewMode   uint32
}

// Changes is a resolved review subject.
type Changes struct {
	Base  string // short name of the ref the old version is based on
	Files []File // sorted by path
	// GitAttributes is the content of the .gitattributes of the
	// reviewed state, empty when it has none. The diff module parses
	// it for linguist-generated paths.
	GitAttributes string
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

// WorktreeDir returns the root directory of the working tree, "" when
// the repository has none. The review state lives there, in .lgtm/.
func (s *Source) WorktreeDir() string {
	wt, err := s.repo.Worktree()
	if err != nil {
		return ""
	}
	return wt.Filesystem().Root()
}

// worktree opens the working tree with the patterns of the repository's
// info/exclude as excludes. go-git reads info/exclude through the
// working tree filesystem, which keeps .git out of reach, and does not
// know the common git dir of a linked worktree; without the explicit
// excludes git status and lgtm would disagree about untracked files,
// e.g. lgtm's own .lgtm/ directory.
func (s *Source) worktree() (*git.Worktree, error) {
	wt, err := s.repo.Worktree()
	if err != nil {
		return nil, err
	}
	_, commonDir, err := gitDirs(wt.Filesystem().Root())
	if err != nil {
		return wt, nil // not on disk, e.g. an in-memory repository
	}
	data, err := os.ReadFile(filepath.Join(commonDir, "info", "exclude"))
	if err != nil {
		return wt, nil // no info/exclude
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		wt.Excludes = append(wt.Excludes, gitignore.ParsePattern(line, nil))
	}
	return wt, nil
}

// gitDirs resolves the .git of the working tree root: the git dir that
// holds HEAD and the common git dir that holds the refs. Both are the
// .git directory, except in a linked worktree, whose .git file names its
// own git dir, which in turn names the common dir.
func gitDirs(root string) (gitDir, commonDir string, err error) {
	dotGit := filepath.Join(root, ".git")
	fi, err := os.Stat(dotGit)
	if err != nil {
		return "", "", err
	}
	if fi.IsDir() {
		return dotGit, dotGit, nil
	}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", "", err
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", "", fmt.Errorf("%s is not a gitdir file", dotGit)
	}
	gitDir = strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return gitDir, gitDir, nil
	}
	commonDir = strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	return gitDir, filepath.Clean(commonDir), nil
}

// Files lists every file of the working tree: the files tracked by
// HEAD plus untracked files that are not ignored, sorted by path.
// Files deleted in the working tree are skipped. It is the file
// listing the context picker and the project search build on.
func (s *Source) Files() ([]string, error) {
	wt, err := s.worktree()
	if err != nil {
		return nil, fmt.Errorf("open working tree: %w", err)
	}
	paths := map[string]bool{}
	// A repository without commits has no HEAD tree yet; the untracked
	// files from the status below are then the whole listing.
	if head, err := s.headCommit(); err == nil {
		tree, err := head.Tree()
		if err != nil {
			return nil, fmt.Errorf("read HEAD tree: %w", err)
		}
		files := tree.Files()
		for {
			f, err := files.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("list HEAD tree: %w", err)
			}
			paths[f.Name] = true
		}
	}
	// Untracked files from the status: like git status, files that are
	// ignored are not listed. Staged additions are included even
	// though HEAD does not know them yet.
	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("working tree status: %w", err)
	}
	for p, st := range status {
		if st.Worktree == git.Untracked || st.Staging == git.Added {
			paths[p] = true
		}
	}
	var files []string
	for p := range paths {
		fi, err := wt.Filesystem().Lstat(p)
		if err != nil {
			continue // deleted in the working tree
		}
		if fi.Mode()&os.ModeSymlink != 0 || fi.Mode().IsRegular() {
			files = append(files, p)
		}
	}
	slices.Sort(files)
	return files, nil
}

// Read returns the current content of path, as it is in the working
// tree. It is how the UI opens files beyond the review in their new
// state.
func (s *Source) Read(path string) (string, error) {
	wt, err := s.worktree()
	if err != nil {
		return "", fmt.Errorf("open working tree: %w", err)
	}
	content, ok, err := fsContent(wt.Filesystem(), path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if !ok {
		return "", fmt.Errorf("%s is not in the working tree", path)
	}
	return content, nil
}

// HeadBranch returns the short name of the branch HEAD points at, ""
// when HEAD is detached. It names the branch in the prompt template.
func (s *Source) HeadBranch() string {
	ref, err := s.repo.Head()
	if err != nil {
		return ""
	}
	if ref.Name().IsBranch() {
		return ref.Name().Short()
	}
	return ""
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
	attrs, _, err := s.worktreeAttrs()
	if err != nil {
		return Changes{}, err
	}
	if attrs == "" {
		// The .gitattributes may be committed without reaching the
		// working tree, e.g. after a fresh clone.
		attrs, _, _ = s.headAttrs()
	}
	return Changes{Base: baseRef.Name().Short(), Files: files, GitAttributes: attrs}, nil
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
	attrs, _, err := treeContent(bTree, ".gitattributes")
	if err != nil {
		return Changes{}, err
	}
	return Changes{Base: a, Files: files, GitAttributes: attrs}, nil
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
	attrs, _, err := treeContent(tree, ".gitattributes")
	if err != nil {
		return Changes{}, err
	}
	return Changes{Base: base, Files: files, GitAttributes: attrs}, nil
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
	return treeChanges(changes, oldTree, newTree)
}

// treeChanges turns the tree changes into file entries. Submodule
// changes carry the commit hashes instead of contents, and mode-only
// changes carry the two modes.
func treeChanges(changes object.Changes, oldTree, newTree *object.Tree) ([]File, error) {
	var files []File
	for _, c := range changes {
		path := c.To.Name
		if path == "" {
			path = c.From.Name
		}
		oldMode, newMode := c.From.TreeEntry.Mode, c.To.TreeEntry.Mode
		if oldMode == filemode.Submodule || newMode == filemode.Submodule {
			old, new := "", ""
			if oldMode == filemode.Submodule {
				old = c.From.TreeEntry.Hash.String()
			}
			if newMode == filemode.Submodule {
				new = c.To.TreeEntry.Hash.String()
			}
			files = append(files, File{
				Path:      path,
				Status:    submoduleStatus(oldMode, newMode),
				Submodule: true,
				Old:       old,
				New:       new,
			})
			continue
		}
		old, oldMode, inOld, err := treeEntry(oldTree, path)
		if err != nil {
			return nil, err
		}
		new, newMode, inNew, err := treeEntry(newTree, path)
		if err != nil {
			return nil, err
		}
		if f, changed := entryFile(path, old, oldMode, inOld, new, newMode, inNew); changed {
			files = append(files, f)
		}
	}
	return files, nil
}

// submoduleStatus is the status of a gitlink change: added when only the
// new side has the link, deleted when only the old side has it.
func submoduleStatus(oldMode, newMode filemode.FileMode) Status {
	switch {
	case oldMode != filemode.Submodule:
		return Added
	case newMode != filemode.Submodule:
		return Deleted
	default:
		return Modified
	}
}

// entryFile builds the File entry for a path, reporting whether the
// change is worth showing: the contents differ, a side is missing, or
// only the file mode changed.
func entryFile(path, old string, oldMode filemode.FileMode, inOld bool, new string, newMode filemode.FileMode, inNew bool) (File, bool) {
	switch {
	case inOld && inNew && old == new && oldMode != newMode && oldMode != 0 && newMode != 0:
		return File{Path: path, Status: Modified, Old: old, New: new, OldMode: uint32(oldMode), NewMode: uint32(newMode)}, true
	case inOld && inNew && old != new:
		return File{Path: path, Status: Modified, Old: old, New: new}, true
	case !inOld && inNew:
		return File{Path: path, Status: Added, New: new}, true
	case inOld && !inNew:
		return File{Path: path, Status: Deleted, Old: old}, true
	}
	return File{}, false
}

// treeEntry reads the content and mode of path in tree. For a gitlink
// entry the content is the commit hash the link points at; treeContent
// would try to read it as a blob and fail.
func treeEntry(tree *object.Tree, path string) (content string, mode filemode.FileMode, ok bool, err error) {
	e, err := tree.FindEntry(path)
	if errors.Is(err, object.ErrEntryNotFound) || errors.Is(err, object.ErrFileNotFound) || errors.Is(err, object.ErrDirectoryNotFound) {
		return "", filemode.Empty, false, nil
	}
	if err != nil {
		return "", filemode.Empty, false, fmt.Errorf("read %s: %w", path, err)
	}
	if e.Mode == filemode.Submodule {
		return e.Hash.String(), e.Mode, true, nil
	}
	f, err := tree.TreeEntryFile(e)
	if err != nil {
		return "", filemode.Empty, false, fmt.Errorf("read %s: %w", path, err)
	}
	content, err = f.Contents()
	if err != nil {
		return "", filemode.Empty, false, fmt.Errorf("read %s: %w", path, err)
	}
	return content, e.Mode, true, nil
}

// fsEntry reads the content and mode of path from the working tree.
// Like git, a symlink's target is the content. A submodule directory
// yields the commit hash it checks out, from the subHashes map.
func fsEntry(fs billy.Filesystem, path string, subHashes map[string]plumbing.Hash) (content string, mode filemode.FileMode, ok bool, err error) {
	fi, err := fs.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", filemode.Empty, false, nil
	}
	if err != nil {
		return "", filemode.Empty, false, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := fs.Readlink(path)
		if err != nil {
			return "", filemode.Empty, false, err
		}
		return target, filemode.Symlink, true, nil
	}
	if fi.IsDir() {
		if h, isSub := subHashes[path]; isSub {
			return h.String(), filemode.Submodule, true, nil
		}
		return "", filemode.Empty, false, nil
	}
	if !fi.Mode().IsRegular() {
		return "", filemode.Empty, false, nil
	}
	f, err := fs.Open(path)
	if err != nil {
		return "", filemode.Empty, false, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", filemode.Empty, false, err
	}
	mode = filemode.Regular
	if fi.Mode()&0o111 != 0 {
		mode = filemode.Executable
	}
	return string(b), mode, true, nil
}

// worktreeAttrs reads the .gitattributes of the working tree.
func (s *Source) worktreeAttrs() (string, bool, error) {
	wt, err := s.worktree()
	if err != nil {
		return "", false, fmt.Errorf("open working tree: %w", err)
	}
	return fsContent(wt.Filesystem(), ".gitattributes")
}

// headAttrs reads the .gitattributes of the HEAD tree.
func (s *Source) headAttrs() (string, bool, error) {
	head, err := s.headCommit()
	if err != nil {
		return "", false, err
	}
	tree, err := head.Tree()
	if err != nil {
		return "", false, fmt.Errorf("read HEAD tree: %w", err)
	}
	return treeContent(tree, ".gitattributes")
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

// workingTreeChanges compares base with the working tree. The committed
// part comes from the trees, the uncommitted part overlays the working
// tree status on top; files whose content ends up unchanged are dropped.
func (s *Source) workingTreeChanges(base *object.Tree, head *object.Commit) ([]File, error) {
	wt, err := s.worktree()
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
	files, err := treeChanges(committed, base, headTree)
	if err != nil {
		return nil, err
	}

	status, err := wt.Status()
	if err != nil {
		return nil, fmt.Errorf("working tree status: %w", err)
	}
	var dirty []string
	for p, st := range status {
		if st.Staging != git.Unmodified || st.Worktree != git.Unmodified {
			dirty = append(dirty, p)
		}
	}
	if len(dirty) == 0 {
		return files, nil
	}
	slices.Sort(dirty)

	subs, err := wt.Submodules()
	if err != nil {
		return nil, fmt.Errorf("list submodules: %w", err)
	}
	subStatus, err := subs.Status()
	if err != nil {
		return nil, fmt.Errorf("read submodule status: %w", err)
	}
	subHashes := make(map[string]plumbing.Hash, len(subStatus))
	for _, s := range subStatus {
		current := s.Current
		if current.IsZero() {
			current = s.Expected
		}
		subHashes[s.Path] = current
	}

	dirtyIndex := make(map[string]bool, len(dirty))
	for _, p := range dirty {
		dirtyIndex[p] = true
	}
	// The dirty entries replace the committed ones: a reverted change
	// disappears, an uncommitted edit is the new truth.
	files = slices.DeleteFunc(files, func(f File) bool { return dirtyIndex[f.Path] })
	for _, p := range dirty {
		old, oldMode, inOld, err := treeEntry(base, p)
		if err != nil {
			return nil, err
		}
		cur, newMode, inNew, err := fsEntry(wt.Filesystem(), p, subHashes)
		if err != nil {
			return nil, fmt.Errorf("read %s from working tree: %w", p, err)
		}
		if oldMode == filemode.Submodule || newMode == filemode.Submodule {
			switch {
			case inOld && inNew && old != cur:
				files = append(files, File{Path: p, Status: Modified, Submodule: true, Old: old, New: cur})
			case !inOld && inNew:
				files = append(files, File{Path: p, Status: Added, Submodule: true, New: cur})
			case inOld && !inNew:
				files = append(files, File{Path: p, Status: Deleted, Submodule: true, Old: old})
			}
			continue
		}
		if f, changed := entryFile(p, old, oldMode, inOld, cur, newMode, inNew); changed {
			files = append(files, f)
		}
	}
	slices.SortFunc(files, func(a, b File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
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
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}
