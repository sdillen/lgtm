package review_test

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// TestAddCommentSaveAndLoadRoundtrip pins the persistence contract of
// comments: a comment added to the state survives Save and Load with its
// anchor, quote, category, text, open status and stable id.
func TestAddCommentSaveAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	added := s.AddComment(review.Comment{
		Path:     "cmd/main.go",
		Start:    3,
		End:      5,
		Quote:    "a\nb\nc",
		Category: review.Must,
		Text:     "split this up",
	})
	if added.ID == "" {
		t.Fatal("AddComment assigned no id")
	}
	if added.Status != review.Open {
		t.Errorf("status = %q, want %q", added.Status, review.Open)
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := restored.Comments()
	if len(got) != 1 {
		t.Fatalf("restored %d comments, want 1", len(got))
	}
	if got[0] != added {
		t.Errorf("restored comment = %+v, want %+v", got[0], added)
	}
}

// update rewrites the golden files: go test ./internal/review -update
var update = flag.Bool("update", false, "rewrite the golden files")

// assertGolden compares got with testdata/<name>, or rewrites the file
// with -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs:\n--- got\n%s\n--- want\n%s", name, got, want)
	}
}

// readReviewMD returns the content of <dir>/.lgtm/review.md.
func readReviewMD(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".lgtm", "review.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestSaveWritesReviewMarkdown pins the review.md contract for the agent:
// grouped by file, each comment headed by file:start-end and its
// category, followed by the quoted code with a language tag and the
// text. Files and comments are ordered by path and line.
func TestSaveWritesReviewMarkdown(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	s.AddComment(review.Comment{
		Path: "main.go", Start: 10, End: 12, Category: review.Should,
		Quote: "func main() {\n\trun()\n}",
		Text:  "Handle the error of run.",
	})
	s.AddComment(review.Comment{
		Path: "internal/ui/ui.go", Start: 4, End: 4, Category: review.Question,
		Quote: "const contextWindow = 3",
		Text:  "Why three lines?\nGit uses three as well, so maybe that's it.",
	})
	s.AddComment(review.Comment{
		Path: "main.go", Start: 2, End: 2, Category: review.Must,
		Quote: "import \"os\"",
		Text:  "Unused import.",
	})
	s.AddComment(review.Comment{
		Path: "README.md", Start: 1, End: 3, Category: review.Nit,
		Quote: "# lgtm\n\n```sh\ngo install\n```",
		Text:  "Fences inside the quote must not break the block.",
	})
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	assertGolden(t, "review.md", readReviewMD(t, dir))
}

// TestReviewMarkdownWithoutComments pins that review.md exists even
// without comments, so the agent can always read it.
func TestReviewMarkdownWithoutComments(t *testing.T) {
	dir := t.TempDir()
	if err := review.NewState().Save(dir); err != nil {
		t.Fatal(err)
	}

	assertGolden(t, "review-empty.md", readReviewMD(t, dir))
}

// TestReviewMarkdownListsOnlyOpenComments pins that comments which are
// no longer open (here: a done comment from an existing state.json) are
// not exported to the agent.
func TestReviewMarkdownListsOnlyOpenComments(t *testing.T) {
	dir := t.TempDir()
	state := `{"version": 2, "comments": [
		{"id": "1", "path": "a.go", "start": 1, "end": 1, "quote": "x", "category": "must", "status": "done", "text": "already fixed"},
		{"id": "2", "path": "a.go", "start": 2, "end": 2, "quote": "y", "category": "nit", "status": "open", "text": "still open"}
	]}`
	if err := os.MkdirAll(filepath.Join(dir, ".lgtm"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".lgtm", "state.json"), []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	md := readReviewMD(t, dir)
	if strings.Contains(md, "already fixed") {
		t.Errorf("review.md lists a done comment:\n%s", md)
	}
	if !strings.Contains(md, "still open") {
		t.Errorf("review.md lacks the open comment:\n%s", md)
	}
}

// TestCategoryNextCyclesFromTheDefault pins the <Tab> order of the
// comment float: should (the default) → must → nit → question → should.
func TestCategoryNextCyclesFromTheDefault(t *testing.T) {
	c := review.DefaultCategory
	var got []review.Category
	for range 5 {
		got = append(got, c)
		c = c.Next()
	}
	want := []review.Category{review.Should, review.Must, review.Nit, review.Question, review.Should}
	if !slices.Equal(got, want) {
		t.Errorf("cycle = %v, want %v", got, want)
	}
}
