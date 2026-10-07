package export_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/export"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// update rewrites the golden files: go test ./internal/export -update
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

// sampleData is a realistic prompt data set: two files, mixed
// categories, a multi-line quote, a quote containing a backtick run and
// a scrambled comment order, so the golden file pins the grouping and
// sorting of the template helpers.
func sampleData() export.Data {
	return export.Data{
		Branch: "feat/export",
		Base:   "main",
		Round:  2,
		Comments: []review.Comment{
			{Path: "internal/a.go", Start: 3, End: 5, Category: review.Question, Quote: "a\nb\nc", Text: "Warum drei Zeilen?"},
			{Path: "cmd/main.go", Start: 1, End: 1, Category: review.Must, Quote: "package main", Text: "Hier fehlt ein Fehlerfall."},
			{Path: "internal/a.go", Start: 9, End: 9, Category: review.Should, Quote: "y", Text: "Bitte benennen."},
			{Path: "internal/a.go", Start: 7, End: 7, Category: review.Must, Quote: "a`b", Text: "Backtick im Zitat."},
		},
	}
}

// TestRenderDefaultTemplateGolden pins the rendered default prompt: the
// header with branch, base and round, the files in path order with the
// must comments first, the question marker and the quoted code.
func TestRenderDefaultTemplateGolden(t *testing.T) {
	got, err := export.Render(export.DefaultTemplate, sampleData())
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "prompt.md", got)
}

// TestRenderWithoutRoundOmitsTheRound pins that the default template
// hides the round while rounds are not tracked yet.
func TestRenderWithoutRoundOmitsTheRound(t *testing.T) {
	data := sampleData()
	data.Round = 0
	got, err := export.Render(export.DefaultTemplate, data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Runde") {
		t.Errorf("prompt mentions the round although it is 0:\n%s", got)
	}
}

// TestRenderReportsTemplateErrors pins that a broken template or data
// yields an error instead of a half-rendered prompt.
func TestRenderReportsTemplateErrors(t *testing.T) {
	for _, text := range []string{"{{", "{{.Missing}}"} {
		if _, err := export.Render(text, sampleData()); err == nil {
			t.Errorf("Render(%q) succeeded, want an error", text)
		}
	}
}

// TestGroupByFileGroupsByPathWithMustFirst pins the ordering the prompt
// contract promises: files in path order, within each file the comments
// by category priority (must first), then by position.
func TestGroupByFileGroupsByPathWithMustFirst(t *testing.T) {
	groups := export.GroupByFile(sampleData().Comments)

	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2:\n%+v", len(groups), groups)
	}
	if groups[0].Path != "cmd/main.go" || groups[1].Path != "internal/a.go" {
		t.Errorf("groups ordered %q, %q, want path order", groups[0].Path, groups[1].Path)
	}
	a := groups[1].Comments
	want := []review.Category{review.Must, review.Should, review.Question}
	for i, c := range want {
		if a[i].Category != c {
			t.Errorf("a.go comment %d category = %s, want %s", i, a[i].Category, c)
		}
	}
	if a[0].Start != 7 || a[1].Start != 9 || a[2].Start != 3 {
		t.Errorf("a.go comments out of position order: %d, %d, %d", a[0].Start, a[1].Start, a[2].Start)
	}
}

// TestSortByCategoryPutsMustFirst pins the global sort: category
// priority, then path, then position, stable for equal keys.
func TestSortByCategoryPutsMustFirst(t *testing.T) {
	got := export.SortByCategory(sampleData().Comments)

	want := []review.Category{review.Must, review.Must, review.Should, review.Question}
	for i, c := range want {
		if got[i].Category != c {
			t.Errorf("comment %d category = %s, want %s", i, got[i].Category, c)
		}
	}
	if got[0].Path != "cmd/main.go" {
		t.Errorf("first comment path = %s, want cmd/main.go (path order after category)", got[0].Path)
	}
}

// TestSortByCategoryIsStable pins that comments with equal category,
// path and position keep their input order.
func TestSortByCategoryIsStable(t *testing.T) {
	comments := []review.Comment{
		{Path: "a.go", Start: 1, Category: review.Should, Text: "first"},
		{Path: "a.go", Start: 1, Category: review.Should, Text: "second"},
	}
	got := export.SortByCategory(comments)
	if got[0].Text != "first" || got[1].Text != "second" {
		t.Errorf("equal comments changed order: %q, %q", got[0].Text, got[1].Text)
	}
}

// TestQuoteFenceSurvivesBackticks pins that a quote containing backticks
// cannot close its fence early, like the fence of review.md.
func TestQuoteFenceSurvivesBackticks(t *testing.T) {
	got := export.Quote("a\n`b`\n``c``")
	// The longest run in the quote is two backticks, so a fence of
	// three is already longer and cannot be closed early.
	want := "```\na\n`b`\n``c``\n```"
	if got != want {
		t.Errorf("Quote = %q, want %q", got, want)
	}
}

// TestDefaultTemplateIsUsable pins that the built-in template parses and
// renders with empty data, so `lgtm --print-template` always yields a
// working start point.
func TestDefaultTemplateIsUsable(t *testing.T) {
	if _, err := export.Render(export.DefaultTemplate, export.Data{}); err != nil {
		t.Fatalf("default template does not render empty data: %v", err)
	}
	if !strings.Contains(export.DefaultTemplate, "groupByFile") {
		t.Error("default template does not use the grouping helper")
	}
}

// TestOpenCommentsSortsForThePrompt pins the order of the prompt data:
// only the open comments, by file, must first in each file.
func TestOpenCommentsSortsForThePrompt(t *testing.T) {
	s := review.NewState()
	s.AddComment(review.Comment{Path: "b.go", Start: 1, Category: review.Must, Text: "must b"})
	s.AddComment(review.Comment{Path: "a.go", Start: 5, Category: review.Should, Text: "should a"})
	s.AddComment(review.Comment{Path: "a.go", Start: 2, Category: review.Must, Text: "must a"})
	s.AddComment(review.Comment{Path: "b.go", Start: 9, Category: review.Nit, Text: "nit b"})
	s.CommentList[3].Status = review.Status("done")

	got := export.OpenComments(s)
	if len(got) != 3 {
		t.Fatalf("OpenComments returned %d comments, want 3", len(got))
	}
	want := []string{"a.go:must a", "a.go:should a", "b.go:must b"}
	for i, w := range want {
		if got[i].Path+":"+got[i].Text != w {
			t.Errorf("comment %d = %q, want %q", i, got[i].Path+":"+got[i].Text, w)
		}
	}
}
