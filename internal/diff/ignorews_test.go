package diff_test

import (
	"reflect"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

// TestIgnoreWSHidesAWhitespaceOnlyChange pins that a pair whose lines
// differ only in whitespace becomes one context line, shown with the
// new text: the reader sees the new file.
func TestIgnoreWSHidesAWhitespaceOnlyChange(t *testing.T) {
	got := diff.New("f.txt", "a b\n", "a  b\n").WithIgnoreWS(true)

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a  b"},
	})
	if got.Hunks != nil {
		t.Errorf("whitespace-only change left hunks: %+v", got.Hunks)
	}
}

// TestIgnoreWSHidesATrailingNewlineChange pins that the missing-newline
// marker disappears: a newline is whitespace.
func TestIgnoreWSHidesATrailingNewlineChange(t *testing.T) {
	got := diff.New("f.txt", "a\nb", "a\nb\n").WithIgnoreWS(true)

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Context, Old: 2, New: 2, Text: "b"},
	})
}

// TestIgnoreWSKeepsRealChanges pins that non-whitespace edits stay
// visible as changed lines with their intra-line words.
func TestIgnoreWSKeepsRealChanges(t *testing.T) {
	got := diff.New("f.txt", "a\nx\n", "a\ny\n").WithIgnoreWS(true)

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "x"},
		{Kind: diff.Added, New: 2, Text: "y"},
	})
}

// TestIgnoreWSKeepsLineNumbersConsistent pins that the collapsed pair
// keeps the numbering of the following lines intact: one line is
// consumed on both sides.
func TestIgnoreWSKeepsLineNumbersConsistent(t *testing.T) {
	got := diff.New("f.txt", "a\nb c\nd\n", "a\nbc\nd\n").WithIgnoreWS(true)

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Context, Old: 2, New: 2, Text: "bc"},
		{Kind: diff.Context, Old: 3, New: 3, Text: "d"},
	})
}

// TestIgnoreWSAlignsChangesAroundWhitespaceShifts pins that the diff is
// computed over whitespace-insensitive equality, so a real change next
// to a whitespace-only line aligns correctly.
func TestIgnoreWSAlignsChangesAroundWhitespaceShifts(t *testing.T) {
	got := diff.New("f.txt", "a\n  \nb\n", "a\nb\nc\n").WithIgnoreWS(true)

	assertLines(t, got, []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "a"},
		{Kind: diff.Deleted, Old: 2, Text: "  "},
		{Kind: diff.Context, Old: 3, New: 2, Text: "b"},
		{Kind: diff.Added, New: 3, Text: "c"},
	})
}

// TestWithIgnoreWSFalseRestoresTheOriginalView pins that the option is
// a pure view switch: toggling it off yields exactly the plain view.
func TestWithIgnoreWSFalseRestoresTheOriginalView(t *testing.T) {
	before, after := "a b\nx\n", "a  b\ny\n"
	plain := diff.New("f.txt", before, after)
	got := plain.WithIgnoreWS(true).WithIgnoreWS(false)

	if !reflect.DeepEqual(got.Lines, plain.Lines) ||
		!reflect.DeepEqual(got.Hunks, plain.Hunks) ||
		!reflect.DeepEqual(got.Folds, plain.Folds) {
		t.Errorf("toggle back changed the view:\n got  %+v\n want %+v", got.Lines, plain.Lines)
	}
}

// TestIgnoreWSKeepsTheClassification pins that the view switch keeps
// the file kind and the rename path.
func TestIgnoreWSKeepsTheClassification(t *testing.T) {
	gen := diff.NewWithAttrs("gen.txt", "a \n", "a\n", func(string) bool { return true })
	got := gen.WithIgnoreWS(true)
	if got.Kind != diff.KindGenerated {
		t.Errorf("Kind = %v, want KindGenerated", got.Kind)
	}
	got.OldPath = "old.txt"
	if got := got.WithIgnoreWS(true); got.OldPath != "old.txt" {
		t.Errorf("OldPath lost after re-toggle: %q", got.OldPath)
	}

	bin := diff.New("img.png", "\x00 ", "\x00")
	if got := bin.WithIgnoreWS(true); got.Kind != diff.KindBinary || got.OldSize != 2 {
		t.Errorf("binary view changed: %+v", got)
	}
}

// TestIgnoreWSNewContentIsTheNewFile pins that the reconstructed new
// content of the ignored view is the real new file: the context lines
// show the new text.
func TestIgnoreWSNewContentIsTheNewFile(t *testing.T) {
	got := diff.New("f.txt", "a b\nc\n", "a  b\nc\n").WithIgnoreWS(true)
	if got, want := got.NewContent(), "a  b\nc"; got != want {
		t.Errorf("NewContent = %q, want %q", got, want)
	}
}
