package review_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// TestLoadOnFreshDirReturnsEmptyState pins that a missing state.json is
// not an error: a fresh repository starts with every file unread.
func TestLoadOnFreshDirReturnsEmptyState(t *testing.T) {
	s, err := review.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if s.Read("a.go") {
		t.Error("fresh state marks a.go as read")
	}
}

// TestSetReadSaveAndLoadRoundtrip pins the persistence contract of the
// read status: after SetRead and Save, the state lives in
// .lgtm/state.json with the file, the read flag and a content hash, and
// Load restores it.
func TestSetReadSaveAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()

	s := review.NewState()
	s.SetRead("cmd/main.go", "package main\n")
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".lgtm", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("state.json is not valid JSON: %v\n%s", err, data)
	}
	if raw["version"] != float64(review.Version) {
		t.Errorf("version = %v, want %d", raw["version"], review.Version)
	}
	files, ok := raw["files"].(map[string]any)
	if !ok {
		t.Fatalf("state.json lacks the files map: %v", raw)
	}
	entry, ok := files["cmd/main.go"].(map[string]any)
	if !ok {
		t.Fatalf("state.json lacks cmd/main.go: %v", files)
	}
	if entry["read"] != true {
		t.Errorf("read = %v, want true", entry["read"])
	}
	hash, _ := entry["hash"].(string)
	if hash == "" {
		t.Error("state.json lacks the content hash")
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Read("cmd/main.go") {
		t.Error("restored state does not mark cmd/main.go as read")
	}
	if restored.Read("other.go") {
		t.Error("restored state marks a never-read file as read")
	}
}

// TestSetReadHashIsDeterministic pins that the same content yields the
// same hash, so later rounds can detect changed content.
func TestSetReadHashIsDeterministic(t *testing.T) {
	a := review.NewState()
	a.SetRead("f.go", "same content")
	b := review.NewState()
	b.SetRead("f.go", "same content")

	if a.HashOf("f.go") == "" || a.HashOf("f.go") != b.HashOf("f.go") {
		t.Errorf("hashes differ: %q vs %q", a.HashOf("f.go"), b.HashOf("f.go"))
	}
}

// TestSetUnreadForgetsTheFile pins the manual toggle: unreading a file
// removes it from the persisted state.
func TestSetUnreadForgetsTheFile(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	s.SetRead("a.go", "content")
	s.SetUnread("a.go")
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".lgtm", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "a.go") {
		t.Errorf("state still mentions a.go:\n%s", data)
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Read("a.go") {
		t.Error("restored state marks the unread file as read")
	}
}

// TestLoadRejectsANewerStateVersion pins the versioned schema: a state
// written by a future lgtm must not be silently mangled.
func TestLoadRejectsANewerStateVersion(t *testing.T) {
	dir := t.TempDir()
	write := filepath.Join(dir, ".lgtm", "state.json")
	if err := os.MkdirAll(filepath.Dir(write), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(write, []byte(`{"version": 99, "files": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := review.Load(dir); err == nil {
		t.Error("Load accepted a state from a newer version")
	}
}

// TestLoadRejectsMalformedState pins that a corrupt state.json is an
// error instead of silently losing the read status.
func TestLoadRejectsMalformedState(t *testing.T) {
	dir := t.TempDir()
	write := filepath.Join(dir, ".lgtm", "state.json")
	if err := os.MkdirAll(filepath.Dir(write), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(write, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := review.Load(dir); err == nil {
		t.Error("Load accepted a malformed state")
	}
}

// TestSaveOverwritesThePreviousState pins that a second save replaces the
// file instead of appending a second JSON document to it.
func TestSaveOverwritesThePreviousState(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	s.SetRead("a.go", "one")
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	s.SetRead("b.go", "two")
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatalf("state after the second save is not a valid document: %v", err)
	}
	if !restored.Read("a.go") || !restored.Read("b.go") {
		t.Errorf("missing entries after overwrite: a.go=%v b.go=%v",
			restored.Read("a.go"), restored.Read("b.go"))
	}
}

// TestEnsureExcludedAddsLgtmToInfoExclude pins the acceptance criterion
// ".lgtm/ steht in .git/info/exclude": the entry is added without
// touching existing lines, and running it again changes nothing.
func TestEnsureExcludedAddsLgtmToInfoExclude(t *testing.T) {
	dir := t.TempDir()
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exclude, []byte("# git ls-files --others --exclude-from=.git/info/exclude\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := review.EnsureExcluded(dir); err != nil {
		t.Fatal(err)
	}
	if err := review.EnsureExcluded(dir); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "\n.lgtm/\n") {
		t.Errorf("info/exclude lacks the .lgtm/ entry:\n%s", got)
	}
	if !strings.Contains(got, "# git ls-files") {
		t.Errorf("info/exclude lost its existing content:\n%s", got)
	}
	if n := strings.Count(got, ".lgtm/"); n != 1 {
		t.Errorf(".lgtm/ appears %d times, want exactly once:\n%s", n, got)
	}
}

// TestEnsureExcludedCreatesInfoExclude pins that the exclude file may not
// exist yet; the .git/info directory is created on the way.
func TestEnsureExcludedCreatesInfoExclude(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := review.EnsureExcluded(dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != ".lgtm/\n" {
		t.Errorf("info/exclude = %q, want %q", data, ".lgtm/\n")
	}
}

// TestEnsureExcludedFollowsAGitFile pins the linked worktree case: .git
// is a file pointing at the per-worktree git dir, whose commondir file
// names the common git dir. Like git's --git-path info/exclude, the
// entry lands in the common git dir's info/exclude.
func TestEnsureExcludedFollowsAGitFile(t *testing.T) {
	base := t.TempDir()
	worktree := filepath.Join(base, "agent")
	common := filepath.Join(base, "repo", ".git")
	perWorktree := filepath.Join(common, "worktrees", "agent")
	if err := os.MkdirAll(perWorktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	// A .git file pointing at the per-worktree git dir, relative to the
	// worktree root, like git writes it.
	gitfile := "gitdir: " + filepath.Join("..", "repo", ".git", "worktrees", "agent") + "\n"
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte(gitfile), 0o644); err != nil {
		t.Fatal(err)
	}
	// commondir points back from the per-worktree git dir to the common
	// git dir.
	commondir := filepath.Join("..", "..")
	if err := os.WriteFile(filepath.Join(perWorktree, "commondir"), []byte(commondir), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := review.EnsureExcluded(worktree); err != nil {
		t.Fatal(err)
	}

	exclude := filepath.Join(common, "info", "exclude")
	data, err := os.ReadFile(exclude)
	if err != nil {
		t.Fatalf("entry not in the common git dir: %v", err)
	}
	if string(data) != ".lgtm/\n" {
		t.Errorf("info/exclude = %q, want %q", data, ".lgtm/\n")
	}
}

func TestClearRemovesTheStateDirectoryAndReportsIt(t *testing.T) {
	dir := t.TempDir()
	if err := review.NewState().Save(dir); err != nil {
		t.Fatal(err)
	}

	cleared, err := review.Clear(dir)

	if err != nil {
		t.Fatal(err)
	}
	if !cleared {
		t.Error("cleared = false, want true")
	}
	if _, err := os.Stat(filepath.Join(dir, ".lgtm")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".lgtm still exists: %v", err)
	}
}

func TestClearWithoutStateIsANoOp(t *testing.T) {
	cleared, err := review.Clear(t.TempDir())

	if err != nil {
		t.Fatal(err)
	}
	if cleared {
		t.Error("cleared = true, want false")
	}
}
