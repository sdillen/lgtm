// Package review is the domain model of a review: its persisted machine
// state with the rounds and their snapshots, the read status per file
// and the reviewer's comments. The state lives in
// .lgtm/state.json, the open comments for the agent in .lgtm/review.md;
// .lgtm/ is excluded from git via .git/info/exclude.
package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Version is the schema version of state.json. It is exported for tests;
// a state file written by a newer lgtm is rejected.
const Version = 5

// FileState is the persisted state of one file.
type FileState struct {
	// Read is true once the file end was visible (or the user marked
	// the file read manually).
	Read bool `json:"read"`
	// Hash is the hash of the content that was read, so the status
	// can be reset when the content changed.
	Hash string `json:"hash,omitempty"`
}

// State is the persisted machine state of a review: the rounds, the
// read status keyed by file path and the comments.
type State struct {
	Version int `json:"version"`
	// ID names the review in its snapshot refs.
	ID string `json:"id,omitempty"`
	// Rounds are the review rounds in order; use the round methods
	// instead.
	Rounds []Round              `json:"rounds,omitempty"`
	Files  map[string]FileState `json:"files,omitempty"`
	// Pos is where the reviewer left the review; use Position.
	Pos *Position `json:"position,omitempty"`
	// CommentList holds the comments in the order they were added; it is
	// exported for JSON only, use the comment methods instead.
	CommentList []Comment `json:"comments,omitempty"`
}

// NewState returns an empty state.
func NewState() *State {
	return &State{Version: Version, ID: newID(), Files: map[string]FileState{}}
}

// Read reports whether path was read.
func (s *State) Read(path string) bool {
	f, ok := s.Files[path]
	return ok && f.Read
}

// HashOf returns the content hash remembered for path, "" if the file was
// never read.
func (s *State) HashOf(path string) string {
	return s.Files[path].Hash
}

// SetRead marks path as read, remembering the content that was read.
func (s *State) SetRead(path, content string) {
	if s.Files == nil {
		s.Files = map[string]FileState{}
	}
	s.Files[path] = FileState{Read: true, Hash: contentHash(content)}
}

// SetUnread forgets the read status of path.
func (s *State) SetUnread(path string) {
	delete(s.Files, path)
}

// Load reads the state from <dir>/.lgtm/state.json. A missing file yields
// an empty state; a malformed or too new state is an error.
func Load(dir string) (*State, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".lgtm", "state.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return NewState(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf(".lgtm/state.json: %w", err)
	}
	if s.Version > Version {
		return nil, fmt.Errorf(".lgtm/state.json: version %d is newer than supported version %d", s.Version, Version)
	}
	if s.Files == nil {
		s.Files = map[string]FileState{}
	}
	s.Version = Version
	return &s, nil
}

// Save writes the state to <dir>/.lgtm/state.json and the open comments
// for the agent to <dir>/.lgtm/review.md, each atomically, creating the
// .lgtm directory when needed.
func (s *State) Save(dir string) error {
	stateDir := filepath.Join(dir, ".lgtm")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", stateDir, err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	if err := writeAtomic(filepath.Join(stateDir, "state.json"), data); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	if err := writeAtomic(filepath.Join(stateDir, "review.md"), []byte(s.markdown())); err != nil {
		return fmt.Errorf("write review: %w", err)
	}
	return nil
}

// writeAtomic replaces the file at path with data via a temporary file,
// so readers never see a half-written file.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// contentHash hashes the content that was read, so later rounds can tell
// whether a file changed since it was read.
func contentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// EnsureExcluded makes sure the .lgtm directory is listed in the
// repository's .git/info/exclude, creating the file when needed. It
// resolves .git the way git does: a directory, or a gitdir: file for a
// linked worktree. git maps info/exclude to the common git dir, so the
// commondir file of the per-worktree git dir is followed. A directory
// without .git is left untouched.
func EnsureExcluded(dir string) error {
	gitDir, err := resolveGitDir(dir)
	if err != nil || gitDir == "" {
		return err
	}
	exclude := filepath.Join(gitDir, "info", "exclude")
	data, err := os.ReadFile(exclude)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("read %s: %w", exclude, err)
	}
	if hasExcludeEntry(data) {
		return nil
	}
	var b strings.Builder
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString(".lgtm/\n")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(exclude), err)
	}
	if err := os.WriteFile(exclude, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", exclude, err)
	}
	return nil
}

// hasExcludeEntry reports whether data already excludes .lgtm/.
func hasExcludeEntry(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if line == ".lgtm/" {
			return true
		}
	}
	return false
}

// resolveGitDir resolves dir/.git to the git directory whose
// info/exclude applies: the .git directory itself, or for a linked
// worktree the common git dir named by the per-worktree commondir file.
// "" means the directory has no .git.
func resolveGitDir(dir string) (string, error) {
	dotGit := filepath.Join(dir, ".git")
	fi, err := os.Lstat(dotGit)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", dotGit, err)
	}
	if fi.IsDir() {
		return dotGit, nil
	}
	data, err := os.ReadFile(dotGit)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", dotGit, err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", fmt.Errorf("%s: not a git directory or gitdir file", dotGit)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	// The per-worktree git dir names the shared git dir in commondir;
	// git maps info/exclude to that common dir.
	common, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if errors.Is(err, fs.ErrNotExist) {
		return gitDir, nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", filepath.Join(gitDir, "commondir"), err)
	}
	commonDir := strings.TrimSpace(string(common))
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(gitDir, commonDir)
	}
	return commonDir, nil
}
