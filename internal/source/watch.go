package source

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/osfs"
	"github.com/go-git/go-git/v6/plumbing/format/gitignore"
)

// settleDelay is how long the watcher waits after the last relevant file
// event before it reports a change, so a burst of writes (an agent
// editing many files, a checkout) is reported once.
const settleDelay = 150 * time.Millisecond

// Watcher reports changes to the working tree and the refs while the
// review runs. It only signals that something changed; what changed is
// found by resolving the review subject again. Ignored files, the .git
// directory (except HEAD and the branch refs) and lgtm's own .lgtm
// directory are not watched.
type Watcher struct {
	fsw     *fsnotify.Watcher
	root    string
	wtfs    billy.Filesystem
	gitDirs []string // git dirs whose HEAD, packed-refs and refs/ count

	mu     sync.Mutex
	scopes map[string]*gitignore.Scope // ignore scope per watched dir, relative to root

	changes chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup
}

// Watch starts watching the working tree and the refs of the
// repository. The returned watcher must be closed.
func (s *Source) Watch() (*Watcher, error) {
	root := s.WorktreeDir()
	if root == "" {
		return nil, errors.New("watch: the repository has no working tree")
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("watch: %w", err)
	}
	w := &Watcher{
		fsw:     fsw,
		root:    root,
		wtfs:    osfs.New(root),
		scopes:  map[string]*gitignore.Scope{},
		changes: make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	base, err := gitignore.RootPatterns(w.wtfs)
	if err != nil {
		_ = fsw.Close()
		return nil, fmt.Errorf("watch: read ignore patterns: %w", err)
	}
	if err := w.addTree("", gitignore.NewScope(base)); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	if err := w.addGitDirs(); err != nil {
		_ = fsw.Close()
		return nil, err
	}
	w.wg.Add(1)
	go w.run()
	return w, nil
}

// Changes delivers a signal when the working tree or the refs changed.
// Signals that are not received in time coalesce into one.
func (w *Watcher) Changes() <-chan struct{} {
	return w.changes
}

// Close stops the watcher.
func (w *Watcher) Close() error {
	close(w.done)
	err := w.fsw.Close()
	w.wg.Wait()
	return err
}

// run turns the relevant file events into change signals, once the
// events have settled.
func (w *Watcher) run() {
	defer w.wg.Done()
	var settle <-chan time.Time
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			if w.relevant(ev) {
				settle = time.After(settleDelay)
			}
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
		case <-settle:
			settle = nil
			select {
			case w.changes <- struct{}{}:
			default: // a signal is pending already
			}
		}
	}
}

// relevant reports whether the event changes the review: a change of a
// file that is not ignored, or of HEAD or a branch ref. A new directory
// is watched from now on.
func (w *Watcher) relevant(ev fsnotify.Event) bool {
	for _, dir := range w.gitDirs {
		if rel, ok := within(dir, ev.Name); ok {
			if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() && ev.Has(fsnotify.Create) {
				// A new ref namespace, e.g. refs/heads/feat/.
				_ = w.fsw.Add(ev.Name)
			}
			return refPath(rel)
		}
	}
	rel, ok := within(w.root, ev.Name)
	if !ok || rel == "." {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if parts[0] == ".git" || parts[0] == ".lgtm" {
		return false
	}
	w.mu.Lock()
	scope := w.scopes[filepath.Dir(rel)]
	w.mu.Unlock()
	if scope == nil {
		return false // below an ignored or unwatched directory
	}
	fi, err := os.Lstat(ev.Name)
	isDir := err == nil && fi.IsDir()
	if scope.Match(parts, isDir) {
		return false
	}
	if isDir && ev.Has(fsnotify.Create) {
		sub, err := w.descend(scope, rel)
		if err == nil {
			_ = w.addTree(rel, sub)
		}
	}
	return true
}

// refPath reports whether rel, a path inside a git dir, names HEAD, the
// packed refs or a branch or tag ref. lgtm's own refs and lock files do
// not count.
func refPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasSuffix(rel, ".lock") {
		return false
	}
	switch {
	case rel == "HEAD" || rel == "packed-refs":
		return true
	case strings.HasPrefix(rel, "refs/lgtm/") || rel == "refs/lgtm":
		return false
	case strings.HasPrefix(rel, "refs/"):
		return true
	}
	return false
}

// addTree watches the directory rel (relative to the root) and every
// directory below it that is not ignored.
func (w *Watcher) addTree(rel string, scope *gitignore.Scope) error {
	if err := w.fsw.Add(filepath.Join(w.root, rel)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // removed again before it was watched
		}
		return fmt.Errorf("watch %s: %w", filepath.Join(w.root, rel), err)
	}
	key := rel
	if key == "" {
		key = "."
	}
	w.mu.Lock()
	w.scopes[key] = scope
	w.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(w.root, rel))
	if err != nil {
		return nil // removed while walking
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		child := filepath.Join(rel, e.Name())
		if rel == "" && (e.Name() == ".git" || e.Name() == ".lgtm") {
			continue
		}
		if scope.Match(strings.Split(filepath.ToSlash(child), "/"), true) {
			continue
		}
		sub, err := w.descend(scope, child)
		if err != nil {
			continue
		}
		if err := w.addTree(child, sub); err != nil {
			return err
		}
	}
	return nil
}

// descend returns the ignore scope of the subdirectory rel.
func (w *Watcher) descend(parent *gitignore.Scope, rel string) (*gitignore.Scope, error) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return parent.Descend(parts, func() ([]gitignore.Pattern, error) {
		return gitignore.DirPatterns(w.wtfs, parts)
	})
}

// addGitDirs watches HEAD and the refs: the git dir of the working tree
// and, for a linked worktree, the common git dir with the branch refs.
func (w *Watcher) addGitDirs() error {
	gitDir, commonDir, err := gitDirs(w.root)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	w.gitDirs = []string{gitDir}
	if commonDir != gitDir {
		w.gitDirs = append(w.gitDirs, commonDir)
	}
	for _, dir := range w.gitDirs {
		if err := w.fsw.Add(dir); err != nil {
			return fmt.Errorf("watch %s: %w", dir, err)
		}
	}
	return filepath.WalkDir(filepath.Join(commonDir, "refs"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // a missing refs dir is fine
		}
		if !d.IsDir() {
			return nil
		}
		if d.Name() == "lgtm" && filepath.Dir(path) == filepath.Join(commonDir, "refs") {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(path); err != nil {
			return fmt.Errorf("watch %s: %w", path, err)
		}
		return nil
	})
}

// within returns path relative to dir when path lies inside dir.
func within(dir, path string) (string, bool) {
	rel, err := filepath.Rel(dir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}
