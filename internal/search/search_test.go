package search_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-billy/v6/util"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/go-git/go-git/v6/storage/filesystem"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/search"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/source"
)

// stubRepo is a search.Repo over fixed contents, for the pure searcher
// tests.
type stubRepo struct {
	files    []string
	contents map[string]string
}

func (r stubRepo) Files() ([]string, error) { return r.files, nil }

func (r stubRepo) Read(path string) (string, error) {
	c, ok := r.contents[path]
	if !ok {
		return "", fmt.Errorf("%s: not in the repo", path)
	}
	return c, nil
}

func TestWordAt(t *testing.T) {
	cases := []struct {
		line       string
		col        int
		word       string
		start, end int
	}{
		{"var foo = 1", 4, "foo", 4, 7},
		{"var foo = 1", 5, "foo", 4, 7},
		{"var foo = 1", 6, "foo", 4, 7},
		{"var foo = 1", 0, "var", 0, 3},
		{"var foo = 1", 2, "var", 0, 3},
		{"var foo = 1", 3, "", 0, 0}, // on the space
		{"var foo = 1", 7, "", 0, 0}, // on the "="
		{"foo", 1, "foo", 0, 3},
		{"foo_bar", 5, "foo_bar", 0, 7},
		{"héllo", 1, "héllo", 0, 6},
		{"", 0, "", 0, 0},
		{"foo", 3, "", 0, 0}, // past the end
		{"foo", -1, "", 0, 0},
	}
	for _, c := range cases {
		word, start, end := search.WordAt(c.line, c.col)
		if word != c.word || start != c.start || end != c.end {
			t.Errorf("WordAt(%q, %d) = (%q, %d, %d), want (%q, %d, %d)",
				c.line, c.col, word, start, end, c.word, c.start, c.end)
		}
	}
}

func TestFindInFile(t *testing.T) {
	lines := []string{"func main() {", "return foo()", "}", "// foo"}
	if got := search.FindInFile(lines, "foo"); fmt.Sprint(got) != "[1 3]" {
		t.Errorf("FindInFile = %v, want [1 3]", got)
	}
	if got := search.FindInFile(lines, "bar"); got != nil {
		t.Errorf("FindInFile(bar) = %v, want no matches", got)
	}
	if got := search.FindInFile(lines, ""); got != nil {
		t.Errorf("FindInFile(\"\") = %v, want no matches", got)
	}
}

func TestFindWordInFile(t *testing.T) {
	lines := []string{"foo", "foobar", "foo bar", "xfoo", "foo(", "FOO", "foo_bar"}
	got := search.FindWordInFile(lines, "foo")
	if fmt.Sprint(got) != "[0 2 4]" {
		t.Errorf("FindWordInFile = %v, want [0 2 4]", got)
	}
	// The search is case-sensitive: FOO matches only its own line.
	if got := search.FindWordInFile(lines, "FOO"); fmt.Sprint(got) != "[5]" {
		t.Errorf("FindWordInFile(FOO) = %v, want [5]", got)
	}
	if got := search.FindWordInFile(lines, ""); got != nil {
		t.Errorf("FindWordInFile(\"\") = %v, want no matches", got)
	}
}

func TestSplitLines(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a\nb\n", []string{"a", "b"}},
		{"a\nb", []string{"a", "b"}},
		{"", []string{""}},
		{"a", []string{"a"}},
	}
	for _, c := range cases {
		got := search.SplitLines(c.in)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("SplitLines(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsBinary(t *testing.T) {
	if !search.IsBinary("a\x00b") {
		t.Error("content with a NUL byte is not binary")
	}
	if search.IsBinary("hello world") {
		t.Error("plain text is binary")
	}
}

// TestFindDefinitionsPatterns pins the definition patterns of the
// heuristic go-to-definition: one hit per defining line, in Go, Python,
// TypeScript/JavaScript and Rust, and no hits for call sites, comments
// and look-alikes.
func TestFindDefinitionsPatterns(t *testing.T) {
	cases := []struct {
		ext     string
		content string
		symbol  string
		want    int
	}{
		{".go", "func main() {}\n", "main", 1},
		{".go", "func (r *Receiver) Method() {}\n", "Method", 1},
		{".go", "func (r *Receiver) Method() {}\n", "Receiver", 0},
		{".go", "type Point struct{}\n", "Point", 1},
		{".go", "var count int\n", "count", 1},
		{".go", "const limit = 10\n", "limit", 1},
		{".go", "result := foo()\n", "foo", 0},
		{".go", "// type Fake struct\n", "Fake", 0},
		{".py", "def hello():\n", "hello", 1},
		{".py", "async def fetch():\n", "fetch", 1},
		{".py", "class Animal:\n", "Animal", 1},
		{".py", "hello()\n", "hello", 0},
		{".py", "# class Fake\n", "Fake", 0},
		{".ts", "function render() {}\n", "render", 1},
		{".ts", "class Button extends X {}\n", "Button", 1},
		{".ts", "const MAX = 10;\n", "MAX", 1},
		{".ts", "let state = 0;\n", "state", 1},
		{".ts", "interface Props {}\n", "Props", 1},
		{".ts", "type ID = string;\n", "ID", 1},
		{".ts", "enum Color {}\n", "Color", 1},
		{".ts", "const handler = () => {};\n", "handler", 1},
		{".js", "export function util() {}\n", "util", 1},
		{".rs", "fn main() {}\n", "main", 1},
		{".rs", "struct Config {}\n", "Config", 1},
		{".rs", "enum State {}\n", "State", 1},
		{".rs", "trait Display {}\n", "Display", 1},
		{".rs", "type Id = u64;\n", "Id", 1},
		{".rs", "const MAX: u32 = 10;\n", "MAX", 1},
		{".rs", "static NAME: &str = \"x\";\n", "NAME", 1},
		{".rs", "mod utils;\n", "utils", 1},
		{".rs", "impl Config {}\n", "Config", 0},
		{".txt", "fn main() {}\n", "main", 0},
	}
	for _, c := range cases {
		r := stubRepo{
			files:    []string{"f" + c.ext},
			contents: map[string]string{"f" + c.ext: c.content},
		}
		hits, err := search.New(r).FindDefinitions(c.symbol)
		if err != nil {
			t.Fatalf("%s %q: %v", c.ext, c.symbol, err)
		}
		if len(hits) != c.want {
			t.Errorf("%s %q in %q: %d hits, want %d (hits: %+v)",
				c.ext, c.symbol, c.content, len(hits), c.want, hits)
		}
	}
}

// fixture is an in-memory git repository with a working tree, like the
// source module builds them. The searcher is exercised against it
// through the production source implementation.
type fixture struct {
	t    *testing.T
	repo *git.Repository
	wt   billy.Filesystem
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dot, wt := memfs.New(), memfs.New()
	storage := filesystem.NewStorage(dot, cache.NewObjectLRUDefault())
	repo, err := git.Init(storage,
		git.WithWorkTree(wt),
		git.WithDefaultBranch(plumbing.NewBranchReferenceName("main")))
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, repo: repo, wt: wt}
}

func (f *fixture) write(path, content string) {
	f.t.Helper()
	if err := util.WriteFile(f.wt, path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) commit() {
	f.t.Helper()
	w, err := f.repo.Worktree()
	if err != nil {
		f.t.Fatal(err)
	}
	if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		f.t.Fatal(err)
	}
	if _, err := w.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Unix(0, 0)},
	}); err != nil {
		f.t.Fatal(err)
	}
}

// searcher returns a Searcher over the fixture, wired to the production
// source implementation.
func (f *fixture) searcher() *search.Searcher {
	return search.New(source.New(f.repo))
}

func TestSearcherFilesListsTheWorkingTree(t *testing.T) {
	f := newFixture(t)
	f.write(".gitignore", "ignored.txt\nvendor/\n")
	f.write("a.txt", "a\n")
	f.write("dir/b.txt", "b\n")
	f.write("gone.txt", "x\n")
	f.commit()
	f.remove("gone.txt")
	f.write("u.txt", "u\n")
	f.write("ignored.txt", "i\n")
	f.write("vendor/v.txt", "v\n")

	files, err := f.searcher().Files()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".gitignore", "a.txt", "dir/b.txt", "u.txt"}
	if fmt.Sprint(files) != fmt.Sprint(want) {
		t.Errorf("Files() = %v, want %v", files, want)
	}
}

func (f *fixture) remove(path string) {
	f.t.Helper()
	if err := f.wt.Remove(path); err != nil {
		f.t.Fatal(err)
	}
}

func TestSearcherReadReturnsTheWorkingTreeVersion(t *testing.T) {
	f := newFixture(t)
	f.write("a.txt", "old\n")
	f.commit()
	f.write("a.txt", "new\n")

	s := f.searcher()
	got, err := s.Read("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got != "new\n" {
		t.Errorf("Read(a.txt) = %q, want the working tree version", got)
	}
	if _, err := s.Read("missing.txt"); err == nil {
		t.Error("Read(missing.txt) did not fail")
	}
}

func TestFindWordSearchesTheRepository(t *testing.T) {
	f := newFixture(t)
	f.write(".gitignore", "ignored.txt\n")
	f.write("a.txt", "hello world\nworld of go\n")
	f.write("sub/b.txt", "nothing here\n")
	f.commit()
	f.write("ignored.txt", "hello from the ignored file\n")
	f.write("sub/c.txt", "hello there\n")

	hits, err := f.searcher().FindWord("hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("FindWord = %+v, want 2 hits", hits)
	}
	if hits[0].Path != "a.txt" || hits[0].Line != 1 || hits[0].Text != "hello world" {
		t.Errorf("first hit = %+v, want a.txt:1", hits[0])
	}
	if hits[1].Path != "sub/c.txt" || hits[1].Line != 1 {
		t.Errorf("second hit = %+v, want sub/c.txt:1", hits[1])
	}
}

func TestFindWordSkipsBinaryFiles(t *testing.T) {
	f := newFixture(t)
	f.write("bin.dat", "hello\x00world\n")
	f.commit()

	hits, err := f.searcher().FindWord("hello")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 0 {
		t.Errorf("FindWord found %+v in a binary file", hits)
	}
}

func TestFindWordMatchesWholeWords(t *testing.T) {
	f := newFixture(t)
	f.write("w.txt", "foobar\nfoo bar\n")
	f.commit()

	hits, err := f.searcher().FindWord("foo")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Line != 2 {
		t.Errorf("FindWord(foo) = %+v, want the whole-word line only", hits)
	}
}

func TestFindDefinitionsFindsAcrossFiles(t *testing.T) {
	f := newFixture(t)
	f.write("main.go", "package main\n\nfunc helper() {}\n")
	f.write("other.go", "package main\n\nvar helper = 1\n")
	f.commit()

	hits, err := f.searcher().FindDefinitions("helper")
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("FindDefinitions(helper) = %+v, want 2 hits", hits)
	}
	if hits[0].Path != "main.go" || hits[0].Line != 3 {
		t.Errorf("first hit = %+v, want main.go:3", hits[0])
	}
	if hits[1].Path != "other.go" || hits[1].Line != 3 {
		t.Errorf("second hit = %+v, want other.go:3", hits[1])
	}
}
