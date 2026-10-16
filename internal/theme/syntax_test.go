package theme_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/sdillen/lgtm/internal/theme"
)

const goSrc = "package main\n\n// comment\nfunc main() {\n\treturn\n}\n"

// highlight returns the highlighted lines of goSrc with the built-in night
// theme.
func highlight(t *testing.T, text string) []theme.Line {
	t.Helper()
	th := theme.Default(theme.KindDark)
	return th.Highlighter.Highlight("main.go", text)
}

// lineText joins the segments of one highlighted line.
func lineText(l theme.Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestStreamYieldsOneLineAtATime(t *testing.T) {
	th := theme.Default(theme.KindDark)
	s := th.Highlighter.Stream("main.go", goSrc)

	var got []string
	for {
		line, ok := s.Next()
		if !ok {
			break
		}
		got = append(got, lineText(line))
	}
	want := []string{
		"package main", "", "// comment", "func main() {", "    return", "}",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stream lines = %q, want %q", got, want)
	}
	// The stream is exhausted: further pulls stay empty.
	if line, ok := s.Next(); ok {
		t.Errorf("stream after EOF returned %q", lineText(line))
	}
}

// TestStreamKeepsMultilineTokensAcrossPulls pins that one stream lexes
// the whole text: a block comment opened before the first pull stays
// comment-coloured on the lines pulled later. A per-line lexer would
// break this.
func TestStreamKeepsMultilineTokensAcrossPulls(t *testing.T) {
	th := theme.Default(theme.KindDark)
	src := "func f() {\n/* block\ncomment */\nreturn\n}\n"
	s := th.Highlighter.Stream("main.go", src)

	// Pull only the lines of the comment; the stream must still know
	// that the comment opened on line 2.
	if _, ok := s.Next(); !ok {
		t.Fatal("stream ended early")
	}
	comment := []theme.Line{}
	for len(comment) < 2 {
		line, ok := s.Next()
		if !ok {
			t.Fatal("stream ended inside the comment")
		}
		comment = append(comment, line)
	}
	// Both pulled lines are comment-coloured, like the whole-text
	// highlight of the same source.
	want := "81;89;125" // CommentSingle #51597d in the night theme
	for i, line := range comment {
		if got := renderLine(line); !strings.Contains(got, want) {
			t.Errorf("comment line %d renders %q, want the comment colour", i+1, got)
		}
	}
}

func TestHighlightSplitsTokensIntoSegmentsPerLine(t *testing.T) {
	lines := highlight(t, goSrc)

	wantLines := []string{
		"package main", "", "// comment", "func main() {", "    return", "}",
	}
	if len(lines) != len(wantLines) {
		t.Fatalf("got %d lines, want %d", len(lines), len(wantLines))
	}
	for i, want := range wantLines {
		if got := lineText(lines[i]); got != want {
			t.Errorf("line %d text = %q, want %q", i, got, want)
		}
	}
}

func TestHighlightColoursTokensByType(t *testing.T) {
	lines := highlight(t, goSrc)

	checks := []struct {
		line int
		want string // rgb triplet of the token colour
	}{
		{0, "125;207;255"}, // package: KeywordNamespace #7dcfff
		{2, "81;89;125"},   // comment: CommentSingle #51597d
		{3, "157;124;216"}, // func: KeywordDeclaration #9d7cd8
		{4, "187;154;247"}, // return: Keyword #bb9af7
	}
	for _, c := range checks {
		rendered := renderLine(lines[c.line])
		if !strings.Contains(rendered, c.want) {
			t.Errorf("line %d renders %q, want token colour %s", c.line, rendered, c.want)
		}
	}
}

func TestHighlightExpandsTabs(t *testing.T) {
	lines := highlight(t, goSrc)

	if got := lineText(lines[4]); got != "    return" {
		t.Errorf("tabbed line = %q, want 4-space expansion", got)
	}
}

func TestHighlightMultiLineTokensSpanLines(t *testing.T) {
	lines := highlight(t, "/* one\ntwo */\nx\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3", len(lines))
	}
	// Both lines of the block comment are comment-coloured.
	for i := 0; i < 2; i++ {
		if got := renderLine(lines[i]); !strings.Contains(got, "81;89;125") {
			t.Errorf("line %d renders %q, want the comment colour", i, got)
		}
	}
}

func TestHighlightUnknownLanguageFallsBackToPlainText(t *testing.T) {
	th := theme.Default(theme.KindDark)
	lines := th.Highlighter.Highlight("notes.xyz", "just text\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(lines))
	}
	if got := lineText(lines[0]); got != "just text" {
		t.Errorf("line text = %q, want the plain text", got)
	}
}

func TestHighlightEmptyTextHasNoLines(t *testing.T) {
	if lines := highlight(t, ""); lines != nil {
		t.Errorf("empty text gave %d lines, want none", len(lines))
	}
}

func TestHighlightDimDimsDeletedLines(t *testing.T) {
	th := theme.Default(theme.KindDark)
	lines := th.Highlighter.HighlightDim("main.go", goSrc)

	// The comment is dimmed: #51597d with 55% brightness.
	if got := renderLine(lines[2]); !strings.Contains(got, "38;2;44;48;68") {
		t.Errorf("dimmed comment renders %q, want the dimmed #2c3044", got)
	}
	// The keyword is dimmed: #bb9af7 with 55% brightness.
	if got := renderLine(lines[4]); !strings.Contains(got, "38;2;102;84;135") {
		t.Errorf("dimmed keyword renders %q, want the dimmed #665487", got)
	}
	// The line text stays intact.
	if got := lineText(lines[0]); got != "package main" {
		t.Errorf("dimmed line text = %q, want the source line", got)
	}
}

func TestHighlightSyntaxOverrideChangesTokenColour(t *testing.T) {
	th := theme.Default(theme.KindDark)
	spec := th.Spec()
	spec.Syntax.Overrides = map[string]theme.Override{
		"CommentSingle": {FG: "#ff0000"},
	}
	overridden, err := theme.Resolve(spec, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	lines := overridden.Highlighter.Highlight("main.go", goSrc)
	if got := renderLine(lines[2]); !strings.Contains(got, "255;0;0") {
		t.Errorf("comment renders %q, want the overridden #ff0000", got)
	}
}

func TestHighlightSyntaxOverrideAcceptsTableForm(t *testing.T) {
	th := theme.Default(theme.KindDark)
	spec := th.Spec()
	spec.Syntax.Overrides = map[string]theme.Override{
		"CommentSingle": {FG: "#00ff00", Italic: boolPtr(true)},
	}
	overridden, err := theme.Resolve(spec, colorprofile.TrueColor)
	if err != nil {
		t.Fatal(err)
	}
	lines := overridden.Highlighter.Highlight("main.go", goSrc)
	rendered := renderLine(lines[2])
	if !strings.Contains(rendered, "0;255;0") || !strings.Contains(rendered, "3;") {
		t.Errorf("comment renders %q, want the overridden green italic", rendered)
	}
}

// renderLine renders the segments of a highlighted line.
func renderLine(l theme.Line) string {
	var b strings.Builder
	for _, s := range l {
		b.WriteString(s.Style.Render(s.Text))
	}
	return b.String()
}

func boolPtr(b bool) *bool { return &b }
