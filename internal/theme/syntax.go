package theme

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/colorprofile"
)

// Segment is one styled run of text within a highlighted line.
type Segment struct {
	Text  string
	Style lipgloss.Style
}

// Line is one highlighted line: its styled segments in reading order.
type Line []Segment

// Highlighter colourises code with a theme's syntax style: a chroma style
// (base plus overrides) mapped onto lipgloss styles. Highlighting is
// lexed per path and split back into lines, so multi-line tokens (block
// comments, raw strings) stay correct across the file.
type Highlighter struct {
	derived *chroma.Style
	profile colorprofile.Profile

	lexers map[string]chroma.Lexer
	cache  map[chroma.StyleEntry]lipgloss.Style
}

// newHighlighter builds the syntax style: the registered chroma style named
// by the spec plus its token overrides.
func newHighlighter(syntax SyntaxSpec, profile colorprofile.Profile) (*Highlighter, error) {
	if _, ok := styles.Registry[strings.ToLower(syntax.Base)]; !ok {
		return nil, fmt.Errorf("unknown syntax base %q (want a chroma style like tokyonight-night)", syntax.Base)
	}
	base := styles.Get(syntax.Base)
	derived := base
	if len(syntax.Overrides) > 0 {
		entries := chroma.StyleEntries{}
		for name, o := range syntax.Overrides {
			ttype, err := chroma.TokenTypeString(name)
			if err != nil {
				return nil, fmt.Errorf("unknown token type %q in syntax overrides", name)
			}
			entry, err := o.entry()
			if err != nil {
				return nil, fmt.Errorf("syntax override %q: %w", name, err)
			}
			entries[ttype] = entry.String()
		}
		b := base.Builder()
		b.AddAll(entries)
		var err error
		if derived, err = b.Build(); err != nil {
			return nil, fmt.Errorf("building syntax style: %w", err)
		}
	}
	return &Highlighter{
		derived: derived,
		profile: profile,
		lexers:  map[string]chroma.Lexer{},
		cache:   map[chroma.StyleEntry]lipgloss.Style{},
	}, nil
}

// Highlight tokenises the whole text with the lexer for path and returns
// one Line per line of text. Files without a lexer fall back to plain text.
func (h *Highlighter) Highlight(path, text string) []Line {
	return h.highlight(path, text, false)
}

// HighlightDim is Highlight for deleted lines: the tokens keep their
// colours, dimmed so deletions recede behind the added code.
func (h *Highlighter) HighlightDim(path, text string) []Line {
	return h.highlight(path, text, true)
}

func (h *Highlighter) highlight(path, text string, dim bool) []Line {
	if text == "" {
		return nil
	}
	it, err := h.lexerFor(path).Tokenise(nil, text)
	if err != nil {
		return nil
	}
	var lines []Line
	cur := 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		if tok.Value == "" {
			continue
		}
		entry := h.derived.Get(tok.Type)
		if dim {
			entry = dimEntry(entry)
		}
		style := h.styleOf(entry)
		for _, part := range strings.SplitAfter(tok.Value, "\n") {
			newline := strings.HasSuffix(part, "\n")
			if newline {
				part = strings.TrimSuffix(part, "\n")
			}
			if part != "" {
				for len(lines) <= cur {
					lines = append(lines, nil)
				}
				lines[cur] = append(lines[cur], Segment{Text: expandTabs(part), Style: style})
			}
			if newline {
				cur++
			}
		}
	}
	return lines
}

// lexerFor returns the coalesced lexer for a path, falling back to plain
// text.
func (h *Highlighter) lexerFor(path string) chroma.Lexer {
	if l, ok := h.lexers[path]; ok {
		return l
	}
	l := lexers.Match(path)
	if l == nil {
		l = lexers.Fallback
	}
	l = chroma.Coalesce(l)
	h.lexers[path] = l
	return l
}

// styleOf converts a chroma style entry into a lipgloss style, adapted to
// the colour profile.
func (h *Highlighter) styleOf(entry chroma.StyleEntry) lipgloss.Style {
	if s, ok := h.cache[entry]; ok {
		return s
	}
	var st lipgloss.Style
	if entry.Colour.IsSet() {
		if c := h.convert(entry.Colour); c != nil {
			st = st.Foreground(c)
		}
	}
	if entry.Background.IsSet() {
		if c := h.convert(entry.Background); c != nil {
			st = st.Background(c)
		}
	}
	if entry.Bold != chroma.Pass {
		st = st.Bold(entry.Bold == chroma.Yes)
	}
	if entry.Italic != chroma.Pass {
		st = st.Italic(entry.Italic == chroma.Yes)
	}
	if entry.Underline != chroma.Pass {
		st = st.Underline(entry.Underline == chroma.Yes)
	}
	h.cache[entry] = st
	return st
}

// convert adapts a chroma colour to the profile; nil means the profile
// cannot show colours.
func (h *Highlighter) convert(c chroma.Colour) color.Color {
	return h.profile.Convert(color.RGBA{R: c.Red(), G: c.Green(), B: c.Blue(), A: 0xff})
}

// dimEntry dims a style entry: all colours are darkened by the dim factor.
func dimEntry(entry chroma.StyleEntry) chroma.StyleEntry {
	if entry.Colour.IsSet() {
		entry.Colour = entry.Colour.Brighten(-dimFactor)
	}
	if entry.Background.IsSet() {
		entry.Background = entry.Background.Brighten(-dimFactor)
	}
	return entry
}

// dimFactor is how much deleted lines are darkened relative to normal text.
const dimFactor = 0.45

func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}
