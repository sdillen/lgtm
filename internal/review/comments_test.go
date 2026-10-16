package review_test

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sdillen/lgtm/internal/review"
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

// TestAnchorKindsSurviveSaveAndLoad pins the persisted anchor of every
// comment kind: deleted-line comments remember that they live in the old
// file state, file comments carry no line range and global comments no
// path.
func TestAnchorKindsSurviveSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	deleted := s.AddComment(review.Comment{Path: "a.go", Start: 4, End: 4, Old: true, Quote: "gone", Text: "why removed?"})
	file := s.AddComment(review.Comment{Path: "a.go", Text: "restructure this file"})
	global := s.AddComment(review.Comment{Text: "nice work overall"})
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := restored.Comments()
	if len(got) != 3 {
		t.Fatalf("restored %d comments, want 3", len(got))
	}
	if got[0] != deleted || got[1] != file || got[2] != global {
		t.Errorf("restored comments differ:\n%+v\n%+v\n%+v", got[0], got[1], got[2])
	}
}

// TestUpdateCommentReplacesTextAndCategory pins the edit flow: the text
// and category are replaced, the anchor, quote, id and status stay, and
// an unknown id is reported.
func TestUpdateCommentReplacesTextAndCategory(t *testing.T) {
	s := review.NewState()
	c := s.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "old", Category: review.Should, Text: "rename"})

	got, ok := s.UpdateComment(c.ID, "rename to load", review.Must)
	if !ok {
		t.Fatal("UpdateComment did not find the comment")
	}
	if got.Text != "rename to load" || got.Category != review.Must ||
		got.Start != 2 || got.Quote != "old" || got.Status != review.Open || got.ID != c.ID {
		t.Errorf("updated comment = %+v", got)
	}
	if _, ok := s.UpdateComment("nope", "x", review.Must); ok {
		t.Error("UpdateComment accepted an unknown id")
	}
}

// TestDeleteCommentRemovesIt pins that a deleted comment is gone from
// the state and from the persisted file, and that deleting twice is
// reported as not found.
func TestDeleteCommentRemovesIt(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	keep := s.AddComment(review.Comment{Path: "a.go", Start: 1, Text: "keep"})
	drop := s.AddComment(review.Comment{Path: "b.go", Start: 1, Text: "drop"})
	if !s.DeleteComment(drop.ID) {
		t.Fatal("DeleteComment did not find the comment")
	}
	if s.DeleteComment(drop.ID) {
		t.Error("DeleteComment removed the comment twice")
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := restored.Comments()
	if len(got) != 1 || got[0] != keep {
		t.Errorf("comments after delete = %+v, want only %+v", got, keep)
	}
}

// TestSetCommentStatusMarksDone pins the done marking: the status is
// persisted and the comment leaves the open list the prompt and
// review.md use.
func TestSetCommentStatusMarksDone(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	c := s.AddComment(review.Comment{Path: "a.go", Start: 1, Text: "fix"})

	got, ok := s.SetCommentStatus(c.ID, review.Done)
	if !ok {
		t.Fatal("SetCommentStatus did not find the comment")
	}
	if got.Status != review.Done {
		t.Errorf("status = %q, want %q", got.Status, review.Done)
	}
	if n := len(s.OpenComments()); n != 0 {
		t.Errorf("%d open comments after marking done, want 0", n)
	}
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Comments(); len(got) != 1 || got[0].Status != review.Done {
		t.Errorf("restored = %+v, want the done comment", got)
	}
}

// TestLocationNamesEveryAnchorKind pins the location strings of
// review.md, the prompt and the float title for all anchor kinds: a line
// range, a single line, a deleted line in the old state, a whole file
// and the whole review.
func TestLocationNamesEveryAnchorKind(t *testing.T) {
	tests := []struct {
		c    review.Comment
		want string
	}{
		{review.Comment{Path: "main.go", Start: 3, End: 5}, "main.go:3-5"},
		{review.Comment{Path: "main.go", Start: 4, End: 4}, "main.go:4"},
		{review.Comment{Path: "main.go", Start: 7, End: 7, Old: true}, "main.go:7 (alt, entfernt)"},
		{review.Comment{Path: "main.go"}, "main.go (Datei)"},
		{review.Comment{}, "global"},
		{review.Comment{Path: "main.go", Start: 3, End: 5, Orphaned: true}, "main.go:3-5 (verwaist)"},
		{review.Comment{Path: "main.go", Start: 7, End: 7, Old: true, Orphaned: true}, "main.go:7 (alt, entfernt, verwaist)"},
		{review.Comment{Path: "main.go", Orphaned: true}, "main.go (Datei, verwaist)"},
	}
	for _, tt := range tests {
		if got := tt.c.Location(); got != tt.want {
			t.Errorf("Location(%+v) = %q, want %q", tt.c, got, tt.want)
		}
	}
}

// TestReviewMarkdownRendersEveryAnchorKind pins the review.md contract
// for the anchor kinds of this issue: global comments first under their
// own heading, file comments with the (Datei) marker, deleted-line
// comments with their old line and the (alt, entfernt) marker, and no
// code quote for file and global comments.
func TestReviewMarkdownRendersEveryAnchorKind(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	s.AddComment(review.Comment{Path: "main.go", Start: 1, End: 1, Category: review.Should, Quote: "package main", Text: "Context note."})
	s.AddComment(review.Comment{Path: "main.go", Start: 2, End: 2, Old: true, Category: review.Must, Quote: "old", Text: "Do not remove this."})
	s.AddComment(review.Comment{Path: "main.go", Category: review.Nit, Text: "Split this file."})
	s.AddComment(review.Comment{Category: review.Question, Text: "Is the approach right overall?"})
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	assertGolden(t, "review-anchors.md", readReviewMD(t, dir))
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

// TestOpenCommentsListsOnlyOpenInOrder pins the domain accessor the
// prompt uses: only comments with status open, in the order they were
// added.
func TestOpenCommentsListsOnlyOpenInOrder(t *testing.T) {
	s := review.NewState()
	s.AddComment(review.Comment{Path: "a.go", Start: 1, Text: "first"})
	s.AddComment(review.Comment{Path: "b.go", Start: 1, Text: "done"})
	s.AddComment(review.Comment{Path: "c.go", Start: 1, Text: "third"})
	// The status is part of the persisted comment; CommentList is the
	// only place to change it until the review module grows a method.
	s.CommentList[1].Status = review.Status("done")

	open := s.OpenComments()
	if len(open) != 2 || open[0].Text != "first" || open[1].Text != "third" {
		t.Errorf("OpenComments = %+v, want the open comments in order", open)
	}
}
