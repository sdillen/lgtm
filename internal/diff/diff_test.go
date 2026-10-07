package diff_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

func TestFullFileShowsWholeNewFileWithChangesInPlace(t *testing.T) {
	before := "a\nb\nc\nd\n"
	after := "a\nB\nc\nd\ne\n"

	got := diff.New("f.txt", before, after)

	want := []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "b"},
		{Kind: diff.Added, New: 2, Text: "B"},
		{Kind: diff.Context, Old: 3, New: 3, Text: "c"},
		{Kind: diff.Context, Old: 4, New: 4, Text: "d"},
		{Kind: diff.Added, New: 5, Text: "e"},
	}
	assertLines(t, got, want)
	if got.Path != "f.txt" {
		t.Errorf("Path = %q, want %q", got.Path, "f.txt")
	}
}

func TestDeletedLinesAppearAtTheirOriginalPosition(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\nd\n", "a\nd\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "b"},
		{Kind: diff.Deleted, Old: 3, Text: "c"},
		{Kind: diff.Context, Old: 4, New: 2, Text: "d"},
	})
}

func TestAddedFileConsistsOfAddedLines(t *testing.T) {
	got := diff.New("new.txt", "", "x\ny\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Added, New: 1, Text: "x"},
		{Kind: diff.Added, New: 2, Text: "y"},
	})
}

func TestDeletedFileConsistsOfDeletedLines(t *testing.T) {
	got := diff.New("gone.txt", "x\ny", "")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Deleted, Old: 1, Text: "x"},
		{Kind: diff.Deleted, Old: 2, Text: "y"},
	})
}

func TestMissingNewlineAtEndOfFileIsAChange(t *testing.T) {
	got := diff.New("f.txt", "a\nb", "a\nb\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "b"},
		{Kind: diff.Added, New: 2, Text: "b"},
	})
}

func TestHunksGroupAdjacentChanges(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\nd\ne\n", "a\nB\nc\nd\nE\n")

	assertHunks(t, got, []diff.Hunk{
		{Start: 1, End: 3}, // b -> B
		{Start: 5, End: 7}, // e -> E
	})
	assertFolds(t, got, []diff.Fold{
		{Start: 0, End: 1}, // a
		{Start: 3, End: 5}, // c, d
	})
}

func TestAdjacentChangesMergeIntoOneHunk(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\n", "a\nX\nY\nc\n")

	assertHunks(t, got, []diff.Hunk{{Start: 1, End: 4}})
	assertFolds(t, got, []diff.Fold{
		{Start: 0, End: 1},
		{Start: 4, End: 5},
	})
}

func TestAddedFileHasOneHunkAndNoFolds(t *testing.T) {
	got := diff.New("new.txt", "", "x\ny\n")

	assertHunks(t, got, []diff.Hunk{{Start: 0, End: 2}})
	assertFolds(t, got, nil)
}

func TestDeletedFileHasOneHunkAndNoFolds(t *testing.T) {
	got := diff.New("gone.txt", "x\ny\n", "")

	assertHunks(t, got, []diff.Hunk{{Start: 0, End: 2}})
	assertFolds(t, got, nil)
}

func TestUnchangedFileHasOneFoldAndNoHunks(t *testing.T) {
	got := diff.New("same.txt", "a\nb\n", "a\nb\n")

	assertHunks(t, got, nil)
	assertFolds(t, got, []diff.Fold{{Start: 0, End: 2}})
}

func TestFoldCountsTheCollapsedLines(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\n", "a\nB\nc\nd\ne\nf\ng\nh\ni\nJ\n")

	if len(got.Folds) != 2 {
		t.Fatalf("got %d folds, want 2", len(got.Folds))
	}
	if got.Folds[1].Count() != 7 {
		t.Errorf("middle fold Count() = %d, want 7 (c..i)", got.Folds[1].Count())
	}
}

func TestUnifiedShowsEachHunkWithContextAndHeader(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\n", "a\nB\nc\nd\ne\nf\ng\nh\ni\nJ\nk\n")

	sections := got.Unified(3)
	if len(sections) != 2 {
		t.Fatalf("got %d sections, want 2:\n%+v", len(sections), sections)
	}
	assertSection(t, sections[0], diff.Section{
		OldStart: 1, OldCount: 5,
		NewStart: 1, NewCount: 5,
		Start: 0, End: 6,
	})
	assertSection(t, sections[1], diff.Section{
		OldStart: 7, OldCount: 5,
		NewStart: 7, NewCount: 5,
		Start: 7, End: 13,
	})
}

func TestUnifiedMergesHunksWhoseContextWindowsTouch(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\nd\ne\nf\n", "a\nB\nc\nd\nE\nf\n")

	sections := got.Unified(3)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1:\n%+v", len(sections), sections)
	}
	assertSection(t, sections[0], diff.Section{
		OldStart: 1, OldCount: 6,
		NewStart: 1, NewCount: 6,
		Start: 0, End: 8,
	})
}

func TestUnifiedHeaderOfAddedFileStartsAtZero(t *testing.T) {
	got := diff.New("new.txt", "", "x\ny\nz\n")

	sections := got.Unified(3)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	assertSection(t, sections[0], diff.Section{
		OldStart: 0, OldCount: 0,
		NewStart: 1, NewCount: 3,
		Start: 0, End: 3,
	})
}

func TestUnifiedHeaderOfDeletedFileEndsAtZero(t *testing.T) {
	got := diff.New("gone.txt", "x\ny\nz\n", "")

	sections := got.Unified(3)
	if len(sections) != 1 {
		t.Fatalf("got %d sections, want 1", len(sections))
	}
	assertSection(t, sections[0], diff.Section{
		OldStart: 1, OldCount: 3,
		NewStart: 0, NewCount: 0,
		Start: 0, End: 3,
	})
}

func TestSectionHeaderRendersClassicHunkHeader(t *testing.T) {
	s := diff.Section{OldStart: 1, OldCount: 5, NewStart: 1, NewCount: 5}
	if got, want := s.Header(), "@@ -1,5 +1,5 @@"; got != want {
		t.Errorf("Header() = %q, want %q", got, want)
	}
}

func TestIntraLineWordsHighlightTheChangedTokensOfAPair(t *testing.T) {
	got := diff.New("f.txt", "the quick brown fox\n", "the quick red fox\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Deleted, Old: 1, Text: "the quick brown fox", Words: []diff.Word{{Start: 10, End: 15}}},
		{Kind: diff.Added, New: 1, Text: "the quick red fox", Words: []diff.Word{{Start: 10, End: 13}}},
	})
}

func TestIntraLineWordsCoverSeveralChangedTokensPerLine(t *testing.T) {
	got := diff.New("f.txt", "the quick brown fox\n", "the slow brown cat\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Deleted, Old: 1, Text: "the quick brown fox", Words: []diff.Word{{Start: 4, End: 9}, {Start: 16, End: 19}}},
		{Kind: diff.Added, New: 1, Text: "the slow brown cat", Words: []diff.Word{{Start: 4, End: 8}, {Start: 15, End: 18}}},
	})
}

func TestIntraLineWordsOfACompletelyReplacedPairAreMeaningless(t *testing.T) {
	got := diff.New("f.txt", "aaa bbb\n", "ccc ddd\n")

	// No token survives the replacement, so the pairing carries no
	// information: neither line gets word ranges.
	assertLines(t, got, []diff.Line{
		{Kind: diff.Deleted, Old: 1, Text: "aaa bbb"},
		{Kind: diff.Added, New: 1, Text: "ccc ddd"},
	})
}

func TestIntraLineWordsPairABlockOfReplacedLinesInOrder(t *testing.T) {
	got := diff.New("f.txt", "a\nbb bb\ncc cc\nd\n", "a\nbb XX\ncc YY\nd\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "bb bb", Words: []diff.Word{{Start: 3, End: 5}}},
		{Kind: diff.Deleted, Old: 3, Text: "cc cc", Words: []diff.Word{{Start: 3, End: 5}}},
		{Kind: diff.Added, New: 2, Text: "bb XX", Words: []diff.Word{{Start: 3, End: 5}}},
		{Kind: diff.Added, New: 3, Text: "cc YY", Words: []diff.Word{{Start: 3, End: 5}}},
		{Kind: diff.Context, Old: 4, New: 4, Text: "d"},
	})
}

func TestIntraLineWordsSkipLinesWithoutAPartner(t *testing.T) {
	// Two lines collapse into one: only the first deleted line pairs
	// with the added line, the second stays without ranges.
	got := diff.New("f.txt", "a\nbb bb\ncc cc\nd\n", "a\nbb XX\nd\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "bb bb", Words: []diff.Word{{Start: 3, End: 5}}},
		{Kind: diff.Deleted, Old: 3, Text: "cc cc"},
		{Kind: diff.Added, New: 2, Text: "bb XX", Words: []diff.Word{{Start: 3, End: 5}}},
		{Kind: diff.Context, Old: 4, New: 3, Text: "d"},
	})
}

func TestIntraLineWordsOfAPureInsertionHaveNoPair(t *testing.T) {
	got := diff.New("f.txt", "a\n", "a\nnew line\n")

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Added, New: 2, Text: "new line"},
	})
}

func TestIntraLineWordsUseRuneOffsetsIntoTheLineText(t *testing.T) {
	before, after := "foo(bar) — grüß\n", "foo(baz) — grüß\n"
	got := diff.New("f.txt", before, after)

	// Byte offsets into the raw line, so multi-byte runes before the
	// change do not shift the ranges.
	want := strings.Index(before, "bar")
	assertLines(t, got, []diff.Line{
		{Kind: diff.Deleted, Old: 1, Text: "foo(bar) — grüß", Words: []diff.Word{{Start: want, End: want + 3}}},
		{Kind: diff.Added, New: 1, Text: "foo(baz) — grüß", Words: []diff.Word{{Start: strings.Index(after, "baz"), End: strings.Index(after, "baz") + 3}}},
	})
}

func TestStatsCountAddedAndDeletedLines(t *testing.T) {
	got := diff.New("f.txt", "a\nb\nc\n", "a\nB\nc\nd\n")

	added, deleted := got.Stats()
	if added != 2 || deleted != 1 {
		t.Errorf("Stats() = %d, %d; want 2, 1", added, deleted)
	}
}

func assertHunks(t *testing.T, got diff.ReviewFile, want []diff.Hunk) {
	t.Helper()
	if len(got.Hunks) != len(want) {
		t.Fatalf("got %d hunks, want %d:\n got  %+v\n want %+v", len(got.Hunks), len(want), got.Hunks, want)
	}
	for i := range want {
		if got.Hunks[i] != want[i] {
			t.Errorf("hunk %d = %+v, want %+v", i, got.Hunks[i], want[i])
		}
	}
}

func assertFolds(t *testing.T, got diff.ReviewFile, want []diff.Fold) {
	t.Helper()
	if len(got.Folds) != len(want) {
		t.Fatalf("got %d folds, want %d:\n got  %+v\n want %+v", len(got.Folds), len(want), got.Folds, want)
	}
	for i := range want {
		if got.Folds[i] != want[i] {
			t.Errorf("fold %d = %+v, want %+v", i, got.Folds[i], want[i])
		}
	}
}

func assertSection(t *testing.T, got, want diff.Section) {
	t.Helper()
	if got != want {
		t.Errorf("section = %+v, want %+v", got, want)
	}
}

func BenchmarkNewLargeFile(b *testing.B) {
	var before, after strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&before, "line %d with some content\n", i)
		if i%7 == 0 {
			fmt.Fprintf(&after, "line %d with some changed content\n", i)
		} else {
			fmt.Fprintf(&after, "line %d with some content\n", i)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		diff.New("big.txt", before.String(), after.String())
	}
}

func assertLines(t *testing.T, got diff.ReviewFile, want []diff.Line) {
	t.Helper()
	if len(got.Lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n got  %+v\n want %+v", len(got.Lines), len(want), got.Lines, want)
	}
	for i := range want {
		if !reflect.DeepEqual(got.Lines[i], want[i]) {
			t.Errorf("line %d = %+v, want %+v", i, got.Lines[i], want[i])
		}
	}
}

// TestNewContentReconstructsTheNewFile pins the content the reader sees:
// the lines of the full-file view that exist in the new version, deleted
// lines skipped, without a trailing newline. It is what the review module
// hashes for the read status.
func TestNewContentReconstructsTheNewFile(t *testing.T) {
	f := diff.New("f.txt", "one\ntwo\n", "one\ntwo\nthree\n")

	if got, want := f.NewContent(), "one\ntwo\nthree"; got != want {
		t.Errorf("NewContent = %q, want %q", got, want)
	}
}

// TestNewContentOfAnAddedFile pins that an added file yields its whole
// content and a deleted file yields nothing.
func TestNewContentOfAnAddedFile(t *testing.T) {
	added := diff.New("a.txt", "", "x\ny\n")
	if got, want := added.NewContent(), "x\ny"; got != want {
		t.Errorf("added file: NewContent = %q, want %q", got, want)
	}
	deleted := diff.New("d.txt", "x\n", "")
	if got := deleted.NewContent(); got != "" {
		t.Errorf("deleted file: NewContent = %q, want empty", got)
	}
}
