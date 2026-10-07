package diff_test

import (
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

func TestFullFileShowsWholeNewFileWithChangesInPlace(t *testing.T) {
	old := "a\nb\nc\nd\n"
	new := "a\nB\nc\nd\ne\n"

	got := diff.New("f.txt", old, new)

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

func assertLines(t *testing.T, got diff.ReviewFile, want []diff.Line) {
	t.Helper()
	if len(got.Lines) != len(want) {
		t.Fatalf("got %d lines, want %d:\n got  %+v\n want %+v", len(got.Lines), len(want), got.Lines, want)
	}
	for i := range want {
		if got.Lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, got.Lines[i], want[i])
		}
	}
}
