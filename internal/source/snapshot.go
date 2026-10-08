package source

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/filemode"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// Snapshot records the reviewed state of the working tree, uncommitted
// and untracked (not ignored) changes included, as a commit on top of
// HEAD and points ref at it, e.g. refs/lgtm/<review>/<round>. The ref
// keeps the snapshot safe from garbage collection. HEAD, the index and
// the working tree stay untouched; an existing ref is overwritten.
func (s *Source) Snapshot(ref string) error {
	wt, err := s.worktree()
	if err != nil {
		return fmt.Errorf("open working tree: %w", err)
	}
	head, err := s.headCommit()
	if err != nil {
		return err
	}
	headTree, err := head.Tree()
	if err != nil {
		return fmt.Errorf("read HEAD tree: %w", err)
	}
	entries, err := treeEntries(headTree)
	if err != nil {
		return err
	}
	dirty, err := dirtyPaths(wt)
	if err != nil {
		return err
	}
	var subHashes map[string]plumbing.Hash
	if len(dirty) > 0 {
		if subHashes, err = submoduleHashes(wt); err != nil {
			return err
		}
	}
	for _, p := range dirty {
		content, mode, ok, err := fsEntry(wt.Filesystem(), p, subHashes)
		if err != nil {
			return fmt.Errorf("read %s from working tree: %w", p, err)
		}
		switch {
		case !ok:
			delete(entries, p)
		case mode == filemode.Submodule:
			entries[p] = object.TreeEntry{Mode: mode, Hash: plumbing.NewHash(content)}
		default:
			h, err := s.storeBlob(content)
			if err != nil {
				return fmt.Errorf("store %s: %w", p, err)
			}
			entries[p] = object.TreeEntry{Mode: mode, Hash: h}
		}
	}
	tree, err := s.storeTree(entries)
	if err != nil {
		return err
	}
	sig := object.Signature{Name: "lgtm", Email: "lgtm@localhost", When: time.Now()}
	commit, err := s.storeObject(&object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      "lgtm review snapshot\n",
		TreeHash:     tree,
		ParentHashes: []plumbing.Hash{head.Hash},
	})
	if err != nil {
		return fmt.Errorf("store snapshot commit: %w", err)
	}
	return s.setRef(ref, commit)
}

// SnapshotRev records the commit rev as the reviewed state under ref,
// for review subjects without working tree changes: a range or a
// single commit.
func (s *Source) SnapshotRev(ref, rev string) error {
	c, err := s.resolveCommit(rev)
	if err != nil {
		return err
	}
	return s.setRef(ref, c.Hash)
}

// Since resolves the changes from the commit rev, typically a snapshot
// ref, up to the working tree: committed and uncommitted changes since
// the snapshot, the interdiff of a review round. Base is rev as given.
func (s *Source) Since(rev string) (Changes, error) {
	c, err := s.resolveCommit(rev)
	if err != nil {
		return Changes{}, err
	}
	tree, err := c.Tree()
	if err != nil {
		return Changes{}, fmt.Errorf("read tree of %s: %w", rev, err)
	}
	head, err := s.headCommit()
	if err != nil {
		return Changes{}, err
	}
	files, err := s.workingTreeChanges(tree, head)
	if err != nil {
		return Changes{}, err
	}
	attrs, err := s.attrs()
	if err != nil {
		return Changes{}, err
	}
	return Changes{Base: rev, Files: files, GitAttributes: attrs}, nil
}

// setRef points the reference name at the commit h.
func (s *Source) setRef(name string, h plumbing.Hash) error {
	ref := plumbing.NewHashReference(plumbing.ReferenceName(name), h)
	if err := s.repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("set %s: %w", name, err)
	}
	return nil
}

// treeEntries lists every non-tree entry of tree, keyed by its full path.
func treeEntries(tree *object.Tree) (map[string]object.TreeEntry, error) {
	entries := map[string]object.TreeEntry{}
	w := object.NewTreeWalker(tree, true, nil)
	defer w.Close()
	for {
		name, e, err := w.Next()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("walk HEAD tree: %w", err)
		}
		if e.Mode != filemode.Dir {
			entries[name] = e
		}
	}
}

// storeTree stores the nested tree objects for the entries keyed by
// full path and returns the hash of the root tree.
func (s *Source) storeTree(entries map[string]object.TreeEntry) (plumbing.Hash, error) {
	sub := map[string]map[string]object.TreeEntry{}
	var tree object.Tree
	for p, e := range entries {
		dir, rest, nested := strings.Cut(p, "/")
		if !nested {
			e.Name = p
			tree.Entries = append(tree.Entries, e)
			continue
		}
		if sub[dir] == nil {
			sub[dir] = map[string]object.TreeEntry{}
		}
		sub[dir][rest] = e
	}
	for dir, children := range sub {
		h, err := s.storeTree(children)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		tree.Entries = append(tree.Entries, object.TreeEntry{Name: dir, Mode: filemode.Dir, Hash: h})
	}
	// git orders tree entries by name, a directory as if its name ended
	// with a slash.
	sortKey := func(e object.TreeEntry) string {
		if e.Mode == filemode.Dir {
			return e.Name + "/"
		}
		return e.Name
	}
	slices.SortFunc(tree.Entries, func(a, b object.TreeEntry) int {
		return strings.Compare(sortKey(a), sortKey(b))
	})
	h, err := s.storeObject(&tree)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store snapshot tree: %w", err)
	}
	return h, nil
}

// storeBlob stores content as a blob object.
func (s *Source) storeBlob(content string) (plumbing.Hash, error) {
	o := s.repo.Storer.NewEncodedObject()
	o.SetType(plumbing.BlobObject)
	o.SetSize(int64(len(content)))
	w, err := o.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := io.WriteString(w, content); err != nil {
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return s.repo.Storer.SetEncodedObject(o)
}

// storeObject encodes and stores a tree or commit object.
func (s *Source) storeObject(obj interface {
	Encode(plumbing.EncodedObject) error
}) (plumbing.Hash, error) {
	o := s.repo.Storer.NewEncodedObject()
	if err := obj.Encode(o); err != nil {
		return plumbing.ZeroHash, err
	}
	return s.repo.Storer.SetEncodedObject(o)
}

// DeleteSnapshots removes every reference whose name starts with prefix,
// e.g. refs/lgtm/<review>/, so the snapshots are no longer protected from
// garbage collection.
func (s *Source) DeleteSnapshots(prefix string) error {
	refs, err := s.repo.References()
	if err != nil {
		return fmt.Errorf("list references: %w", err)
	}
	var names []plumbing.ReferenceName
	err = refs.ForEach(func(r *plumbing.Reference) error {
		if strings.HasPrefix(r.Name().String(), prefix) {
			names = append(names, r.Name())
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("list references: %w", err)
	}
	for _, n := range names {
		if err := s.repo.Storer.RemoveReference(n); err != nil {
			return fmt.Errorf("remove %s: %w", n, err)
		}
	}
	return nil
}
