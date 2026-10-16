package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sdillen/lgtm/internal/review"
)

// relocated adds c to a fresh state, relocates it from prev to next and
// returns the comment afterwards.
func relocated(t *testing.T, c review.Comment, prev, next []review.File) review.Comment {
	t.Helper()
	s := review.NewState()
	s.AddComment(c)
	s.Relocate(prev, next)
	got := s.Comments()
	if len(got) != 1 {
		t.Fatalf("relocate left %d comments, want 1", len(got))
	}
	return got[0]
}

// TestRelocateFollowsLinesInsertedBefore pins the basic contract: lines
// inserted above a comment shift its anchor down to where the quoted
// code is now.
func TestRelocateFollowsLinesInsertedBefore(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "one\ntwo\nthree\n"}}
	next := []review.File{{Path: "a.go", New: "zero\nhalf\none\ntwo\nthree\n"}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 2, End: 3, Quote: "two\nthree"}, prev, next)
	if got.Start != 4 || got.End != 5 || got.Orphaned {
		t.Errorf("anchor = %d-%d orphaned=%v, want 4-5 not orphaned", got.Start, got.End, got.Orphaned)
	}
}

// TestRelocateFollowsLinesDeletedBefore: lines removed above a comment
// shift its anchor up.
func TestRelocateFollowsLinesDeletedBefore(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "x\ny\none\ntwo\n"}}
	next := []review.File{{Path: "a.go", New: "one\ntwo\n"}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 4, End: 4, Quote: "two"}, prev, next)
	if got.Start != 2 || got.End != 2 || got.Orphaned {
		t.Errorf("anchor = %d-%d orphaned=%v, want 2-2", got.Start, got.End, got.Orphaned)
	}
}

// TestRelocatePicksTheFileOfTheComment: the anchor is searched in the
// comment's own file, not in another file of the review.
func TestRelocatePicksTheFileOfTheComment(t *testing.T) {
	prev := []review.File{
		{Path: "a.go", New: "same\n"},
		{Path: "b.go", New: "other\nsame\n"},
	}
	next := []review.File{
		{Path: "a.go", New: "same\n"},
		{Path: "b.go", New: "new\nother\nsame\n"},
	}
	got := relocated(t, review.Comment{Path: "b.go", Start: 2, End: 2, Quote: "same"}, prev, next)
	if got.Start != 3 {
		t.Errorf("start = %d, want 3", got.Start)
	}
}

// TestRelocateKeepsTheSameOccurrenceOfARepeatedQuote: when the quoted
// code occurs several times, the comment stays on the occurrence it was
// made on, even when an insertion before moves that occurrence nearer
// to another one's old place.
func TestRelocateKeepsTheSameOccurrenceOfARepeatedQuote(t *testing.T) {
	before := "func a() {\n\treturn nil\n}\n\nfunc b() {\n\treturn nil\n}\n"
	// Three lines inserted at the top: b's return moves from 6 to 9,
	// a's from 2 to 5, which is nearer to 6.
	after := "// x\n// y\n// z\n" + before
	prev := []review.File{{Path: "a.go", New: before}}
	next := []review.File{{Path: "a.go", New: after}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 6, End: 6, Quote: "\treturn nil"}, prev, next)
	if got.Start != 9 {
		t.Errorf("start = %d, want 9 (the return of b)", got.Start)
	}
}

// TestRelocateFollowsMovedLines: a block moved to another place of the
// file takes its comment along.
func TestRelocateFollowsMovedLines(t *testing.T) {
	before := "func a() {}\n\nfunc b() {\n\tb()\n}\n\nfunc c() {}\n"
	after := "func b() {\n\tb()\n}\n\nfunc a() {}\n\nfunc c() {}\n"
	prev := []review.File{{Path: "a.go", New: before}}
	next := []review.File{{Path: "a.go", New: after}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 3, End: 5, Quote: "func b() {\n\tb()\n}"}, prev, next)
	if got.Start != 1 || got.End != 3 || got.Orphaned {
		t.Errorf("anchor = %d-%d orphaned=%v, want 1-3", got.Start, got.End, got.Orphaned)
	}
}

// TestRelocateOrphansChangedCode: when the quoted code itself changed,
// the comment is orphaned, keeps its last anchor and is not dropped.
func TestRelocateOrphansChangedCode(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "a\nx := 1\nb\n"}}
	next := []review.File{{Path: "a.go", New: "a\nx := 2\nb\n"}}
	s := review.NewState()
	s.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "x := 1", Text: "why 1?"})
	if n := s.Relocate(prev, next); n != 1 {
		t.Errorf("Relocate reported %d orphans, want 1", n)
	}
	got := s.Comments()[0]
	if !got.Orphaned || got.Start != 2 || got.End != 2 || got.Quote != "x := 1" || got.Text != "why 1?" {
		t.Errorf("comment = %+v, want orphaned with its last anchor, quote and text", got)
	}
}

// TestRelocateOrphansCodeOfAPartlyChangedRange: a range comment needs
// all its quoted lines; one changed line orphans it.
func TestRelocateOrphansCodeOfAPartlyChangedRange(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "a\nb\nc\n"}}
	next := []review.File{{Path: "a.go", New: "a\nB\nc\n"}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 1, End: 3, Quote: "a\nb\nc"}, prev, next)
	if !got.Orphaned {
		t.Errorf("comment = %+v, want orphaned", got)
	}
}

// TestRelocateAnchorsAnOrphanAgainWhenItsCodeReturns: orphaning is not
// final; when the quoted code shows up in a later reload, the comment
// is anchored there again.
func TestRelocateAnchorsAnOrphanAgainWhenItsCodeReturns(t *testing.T) {
	v1 := []review.File{{Path: "a.go", New: "a\nx := 1\n"}}
	v2 := []review.File{{Path: "a.go", New: "a\nx := 2\n"}}
	v3 := []review.File{{Path: "a.go", New: "new\na\nx := 1\n"}}
	s := review.NewState()
	s.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "x := 1"})
	s.Relocate(v1, v2)
	if n := s.Relocate(v2, v3); n != 0 {
		t.Errorf("Relocate reported %d orphans, want 0", n)
	}
	got := s.Comments()[0]
	if got.Orphaned || got.Start != 3 {
		t.Errorf("comment = %+v, want anchored on line 3 again", got)
	}
}

// TestRelocateKeepsTheStatus: relocating does not reopen done comments,
// orphaned or not.
func TestRelocateKeepsTheStatus(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "a\n"}}
	next := []review.File{{Path: "a.go", New: "b\n"}}
	s := review.NewState()
	c := s.AddComment(review.Comment{Path: "a.go", Start: 1, End: 1, Quote: "a"})
	s.SetCommentStatus(c.ID, review.Done)
	s.Relocate(prev, next)
	if got := s.Comments()[0]; got.Status != review.Done || !got.Orphaned {
		t.Errorf("comment = %+v, want done and orphaned", got)
	}
}

// TestRelocateFollowsARename: a file renamed since the last load takes
// its line and file comments to the new path.
func TestRelocateFollowsARename(t *testing.T) {
	prev := []review.File{{Path: "old.go", Old: "base\n", New: "base\nkeep\n"}}
	next := []review.File{{Path: "new.go", OldPath: "old.go", Old: "base\n", New: "// moved\nbase\nkeep\n"}}
	s := review.NewState()
	s.AddComment(review.Comment{Path: "old.go", Start: 2, End: 2, Quote: "keep"})
	s.AddComment(review.Comment{Path: "old.go", Text: "file note"})
	s.Relocate(prev, next)
	got := s.Comments()
	if got[0].Path != "new.go" || got[0].Start != 3 || got[0].Orphaned {
		t.Errorf("line comment = %+v, want new.go:3", got[0])
	}
	if got[1].Path != "new.go" || got[1].Start != 0 || got[1].Orphaned {
		t.Errorf("file comment = %+v, want on new.go", got[1])
	}
}

// TestRelocateFollowsARenameOfANewFile: a file added in the review and
// then renamed shows up under a new path without a rename source; its
// comments follow the quote into the newly appeared file.
func TestRelocateFollowsARenameOfANewFile(t *testing.T) {
	prev := []review.File{
		{Path: "draft.go", New: "package x\n\nfunc F() {}\n"},
		{Path: "other.go", Old: "package x\n", New: "package x\n\nfunc F() {}\n"},
	}
	next := []review.File{
		{Path: "final.go", New: "package x\n\n// F does it.\nfunc F() {}\n"},
		{Path: "other.go", Old: "package x\n", New: "package x\n\nfunc F() {}\n"},
	}
	got := relocated(t, review.Comment{Path: "draft.go", Start: 3, End: 3, Quote: "func F() {}"}, prev, next)
	if got.Path != "final.go" || got.Start != 4 || got.Orphaned {
		t.Errorf("comment = %+v, want final.go:4", got)
	}
}

// TestRelocateOrphansCommentsOfADeletedFile: when a file is deleted,
// comments on its new lines are orphaned, comments on its deleted old
// lines and on the whole file stay anchored.
func TestRelocateOrphansCommentsOfADeletedFile(t *testing.T) {
	prev := []review.File{{Path: "a.go", Old: "old\n", New: "old\nnew\n"}}
	next := []review.File{{Path: "a.go", Old: "old\n"}}
	s := review.NewState()
	s.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "new"})
	s.AddComment(review.Comment{Path: "a.go", Start: 1, End: 1, Old: true, Quote: "old"})
	s.AddComment(review.Comment{Path: "a.go"})
	if n := s.Relocate(prev, next); n != 1 {
		t.Errorf("Relocate reported %d orphans, want 1", n)
	}
	got := s.Comments()
	if !got[0].Orphaned {
		t.Errorf("new-line comment = %+v, want orphaned", got[0])
	}
	if got[1].Orphaned || got[1].Start != 1 {
		t.Errorf("old-line comment = %+v, want still on old line 1", got[1])
	}
	if got[2].Orphaned {
		t.Errorf("file comment = %+v, want not orphaned", got[2])
	}
}

// TestRelocateOrphansCommentsOfAFileThatLeftTheReview: a file that is
// no longer part of the review orphans its comments, file comments too.
func TestRelocateOrphansCommentsOfAFileThatLeftTheReview(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "a\n"}, {Path: "b.go", New: "b\n"}}
	next := []review.File{{Path: "b.go", New: "b\n"}}
	s := review.NewState()
	s.AddComment(review.Comment{Path: "a.go", Start: 1, End: 1, Quote: "a"})
	s.AddComment(review.Comment{Path: "a.go"})
	if n := s.Relocate(prev, next); n != 2 {
		t.Errorf("Relocate reported %d orphans, want 2", n)
	}
	for _, c := range s.Comments() {
		if !c.Orphaned || c.Path != "a.go" {
			t.Errorf("comment = %+v, want orphaned on a.go", c)
		}
	}
}

// TestRelocateLeavesGlobalCommentsAlone: a comment on the whole review
// has no anchor to lose.
func TestRelocateLeavesGlobalCommentsAlone(t *testing.T) {
	got := relocated(t, review.Comment{Text: "overall"}, nil, nil)
	if got.Orphaned || got.Path != "" {
		t.Errorf("global comment = %+v, want untouched", got)
	}
}

// TestRelocateFollowsDeletedLinesInTheOldSide: comments on deleted lines
// are searched in the old content; a rebased base moves them too.
func TestRelocateFollowsDeletedLinesInTheOldSide(t *testing.T) {
	prev := []review.File{{Path: "a.go", Old: "a\ngone\n", New: "a\n"}}
	next := []review.File{{Path: "a.go", Old: "header\na\ngone\n", New: "header\na\n"}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 2, End: 2, Old: true, Quote: "gone"}, prev, next)
	if got.Start != 3 || !got.Old || got.Orphaned {
		t.Errorf("comment = %+v, want old line 3", got)
	}
}

// TestRelocateToleratesReindentation: code that only moved into another
// indentation level (e.g. wrapped into an if) keeps its comment, and the
// quote shows the code as it is now.
func TestRelocateToleratesReindentation(t *testing.T) {
	prev := []review.File{{Path: "a.go", New: "f()\ng()\n"}}
	next := []review.File{{Path: "a.go", New: "if ok {\n\tf()\n\tg()\n}\n"}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 1, End: 2, Quote: "f()\ng()"}, prev, next)
	if got.Orphaned || got.Start != 2 || got.End != 3 || got.Quote != "\tf()\n\tg()" {
		t.Errorf("comment = %+v, want 2-3 with the reindented quote", got)
	}
}

// TestOrphanedCommentsSurviveSaveAndLoadAndAreExported: an orphaned
// open comment is persisted and still exported to review.md, marked as
// orphaned, so nothing vanishes silently.
func TestOrphanedCommentsSurviveSaveAndLoadAndAreExported(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	s.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "x := 1", Category: review.Must, Text: "why 1?"})
	s.Relocate([]review.File{{Path: "a.go", New: "a\nx := 1\n"}}, []review.File{{Path: "a.go", New: "a\nx := 2\n"}})
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	restored, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.Comments(); len(got) != 1 || !got[0].Orphaned {
		t.Fatalf("restored comments = %+v, want one orphaned", got)
	}
	md, err := os.ReadFile(filepath.Join(dir, ".lgtm", "review.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "### a.go:2 (verwaist) (must)") || !strings.Contains(string(md), "why 1?") {
		t.Errorf("review.md misses the orphaned comment:\n%s", md)
	}
}

// TestLocateGuessesWhereAChangedLineMoved: for a line whose text
// changed, Locate reports not found but still names the line it moved
// to, so a cursor can stay in place.
func TestLocateGuessesWhereAChangedLineMoved(t *testing.T) {
	line, ok := review.Locate("a\nb\nc\n", "new\na\nB\nc\n", "b", 2)
	if ok || line != 3 {
		t.Errorf("Locate = %d, %v, want 3, false", line, ok)
	}
}

// TestLocateFindsTheLineInUnrelatedContent: without a usable previous
// version the quote is still searched, nearest to the old line number.
func TestLocateFindsTheLineInUnrelatedContent(t *testing.T) {
	line, ok := review.Locate("", "x\ny\nx\n", "x", 3)
	if !ok || line != 3 {
		t.Errorf("Locate = %d, %v, want 3, true", line, ok)
	}
}

// TestRelocateDoesNotJumpToOtherCodeWithTheSameText: when the commented
// line is deleted, an unrelated line with the same text that was there
// before does not take the comment; it is orphaned instead.
func TestRelocateDoesNotJumpToOtherCodeWithTheSameText(t *testing.T) {
	before := "func a() {\n\tx()\n}\n\nfunc b() {\n\ty()\n}\n"
	after := "func a() {\n\tx()\n}\n"
	prev := []review.File{{Path: "a.go", New: before}}
	next := []review.File{{Path: "a.go", New: after}}
	got := relocated(t, review.Comment{Path: "a.go", Start: 7, End: 7, Quote: "}"}, prev, next)
	if !got.Orphaned {
		t.Errorf("comment = %+v, want orphaned, not moved to the brace of a", got)
	}
}

// TestRelocateDoesNotMoveIntoAnUnrelatedNewFile: a comment of a file
// that left the review is only searched in a new file that looks like
// its renamed version, not in any new file with the same short line.
func TestRelocateDoesNotMoveIntoAnUnrelatedNewFile(t *testing.T) {
	prev := []review.File{{Path: "draft.go", New: "package x\n\nfunc F() {\n\tf()\n}\n"}}
	next := []review.File{{Path: "other.go", New: "package y\n\ntype T struct {\n\tn int\n}\n"}}
	got := relocated(t, review.Comment{Path: "draft.go", Start: 5, End: 5, Quote: "}"}, prev, next)
	if !got.Orphaned || got.Path != "draft.go" {
		t.Errorf("comment = %+v, want orphaned on draft.go", got)
	}
}

// TestReviewMarkdownRendersOrphanedComments pins the review.md contract
// for orphaned comments: they stay in the export with their last known
// anchor and quote, marked (verwaist), next to the anchored ones.
func TestReviewMarkdownRendersOrphanedComments(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	s.AddComment(review.Comment{Path: "main.go", Start: 3, End: 3, Category: review.Must, Quote: "x := 1", Text: "Why 1?"})
	s.AddComment(review.Comment{Path: "main.go", Start: 5, End: 5, Category: review.Should, Quote: "keep()", Text: "Fine."})
	s.AddComment(review.Comment{Path: "gone.go", Category: review.Nit, Text: "Split this file."})
	s.Relocate(
		[]review.File{
			{Path: "main.go", New: "package main\n\nx := 1\n\nkeep()\n"},
			{Path: "gone.go", New: "package main\n"},
		},
		[]review.File{{Path: "main.go", New: "package main\n\nx := 2\n\n// moved\nkeep()\n"}},
	)
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	assertGolden(t, "review-orphaned.md", readReviewMD(t, dir))
}
