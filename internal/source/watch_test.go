package source_test

import (
	"testing"
	"time"

	"github.com/sdillen/lgtm/internal/source"
)

// watchTimeout bounds the wait for a change signal; quietPeriod is how
// long the watcher must stay silent for a change that is ignored.
const (
	watchTimeout = 3 * time.Second
	quietPeriod  = 400 * time.Millisecond
)

// watch opens the disk fixture and starts its watcher; the watcher is
// closed when the test ends.
func watch(t *testing.T, f *fixture) *source.Watcher {
	t.Helper()
	src, err := source.Open(f.wt.Root())
	if err != nil {
		t.Fatal(err)
	}
	w, err := src.Watch()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })
	return w
}

// expectChange waits for a change signal.
func expectChange(t *testing.T, w *source.Watcher) {
	t.Helper()
	select {
	case <-w.Changes():
	case <-time.After(watchTimeout):
		t.Fatal("no change reported")
	}
}

// expectQuiet asserts that no change signal arrives.
func expectQuiet(t *testing.T, w *source.Watcher) {
	t.Helper()
	select {
	case <-w.Changes():
		t.Fatal("change reported, want none")
	case <-time.After(quietPeriod):
	}
}

// TestWatchReportsAChangedFile: writing a file of the working tree is
// reported.
func TestWatchReportsAChangedFile(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.go", "package a\n")
	f.commit("init")
	w := watch(t, f)

	f.write("a.go", "package a\n\nfunc A() {}\n")
	expectChange(t, w)
}

// TestWatchIgnoresTheReviewState: lgtm writes .lgtm/ on every comment;
// that must not look like a change of the review.
func TestWatchIgnoresTheReviewState(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.go", "package a\n")
	f.write(".lgtm/state.json", "{}")
	f.commit("init")
	w := watch(t, f)

	f.write(".lgtm/state.json", `{"version": 4}`)
	f.write(".lgtm/review.md", "# Review\n")
	expectQuiet(t, w)
}

// TestWatchIgnoresIgnoredFiles: build output and other ignored files do
// not count, neither files nor whole ignored directories.
func TestWatchIgnoresIgnoredFiles(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write(".gitignore", "*.log\nbuild/\n")
	f.write("a.go", "package a\n")
	f.write("build/keep", "x")
	f.commit("init")
	w := watch(t, f)

	f.write("debug.log", "noise")
	f.write("build/out.bin", "noise")
	expectQuiet(t, w)
}

// TestWatchReportsFilesInNewDirectories: a directory created during the
// review is watched as well.
func TestWatchReportsFilesInNewDirectories(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.go", "package a\n")
	f.commit("init")
	w := watch(t, f)

	f.write("pkg/sub/b.go", "package sub\n")
	expectChange(t, w)
	// Drain the signal of the directory creation; a later write in the
	// new directory is reported on its own.
	time.Sleep(quietPeriod)
	select {
	case <-w.Changes():
	default:
	}
	f.write("pkg/sub/b.go", "package sub\n\nfunc B() {}\n")
	expectChange(t, w)
}

// TestWatchReportsANewCommit: a commit moves the branch ref, which
// changes the review even when the working tree content is the same.
func TestWatchReportsANewCommit(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.go", "package a\n")
	f.commit("init")
	f.write("a.go", "package a\n\nfunc A() {}\n")
	w := watch(t, f)

	f.commit("add A")
	expectChange(t, w)
}

// TestWatchCoalescesABurst: many writes in a row are reported once.
func TestWatchCoalescesABurst(t *testing.T) {
	f := newDiskFixture(t, "main")
	f.write("a.go", "package a\n")
	f.commit("init")
	w := watch(t, f)

	for i := range 20 {
		f.write("a.go", "package a\n// "+string(rune('a'+i))+"\n")
	}
	expectChange(t, w)
	expectQuiet(t, w)
}
