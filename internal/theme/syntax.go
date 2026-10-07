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

// Stream lexes text lazily with the lexer for path: the caller pulls
// one Line at a time, so highlighting a huge file stops after the
// lines that are actually needed. One stream covers the whole text, so
// multi-line tokens (block comments, raw strings) stay consistent
// across pulls.
type Stream struct {
	it   chroma.Iterator
	h    *Highlighter
	dim  bool
	done bool

	// pending holds the parts of tokens that span a line boundary:
	// everything after the newline that ended the previous line.
	pending []part
}

// part is one piece of a token value, possibly ending in a newline.
type part struct {
	text  string
	style lipgloss.Style
}

// Stream returns a lazy token stream of text, highlighted like
// Highlight. Next yields one Line per text line in order; lines the
// lexer produced no tokens for come back without segments.
func (h *Highlighter) Stream(path, text string) *Stream {
	return h.stream(path, text, false)
}

func (h *Highlighter) stream(path, text string, dim bool) *Stream {
	it, err := h.lexerFor(path).Tokenise(nil, text)
	if err != nil {
		it = func() chroma.Token { return chroma.EOF }
	}
	return &Stream{it: it, h: h, dim: dim}
}

// Next returns the next line of the text, or ok=false when the stream
// is exhausted. It is safe to stop pulling early: nothing beyond the
// pulled lines is lexed.
func (s *Stream) Next() (Line, bool) {
	if s.done {
		return nil, false
	}
	var cur Line
	for {
		if len(s.pending) == 0 && !s.pull() {
			return cur, cur != nil
		}
		p := s.pending[0]
		s.pending = s.pending[1:]
		text := strings.TrimSuffix(p.text, "\n")
		if text != "" {
			cur = append(cur, Segment{Text: expandTabs(text), Style: p.style})
		}
		if strings.HasSuffix(p.text, "\n") {
			return cur, true
		}
	}
}

// pull lexes the next token and queues its parts; ok reports whether
// the stream has more input.
func (s *Stream) pull() bool {
	for {
		tok := s.it()
		if tok == chroma.EOF {
			s.done = true
			return false
		}
		if tok.Value == "" {
			continue
		}
		entry := s.h.derived.Get(tok.Type)
		if s.dim {
			entry = dimEntry(entry)
		}
		style := s.h.styleOf(entry)
		for _, p := range strings.SplitAfter(tok.Value, "\n") {
			s.pending = append(s.pending, part{text: p, style: style})
		}
		return true
	}
}

func (h *Highlighter) highlight(path, text string, dim bool) []Line {
	if text == "" {
		return nil
	}
	s := h.stream(path, text, dim)
	var lines []Line
	for {
		line, ok := s.Next()
		if !ok {
			return lines
		}
		lines = append(lines, line)
	}
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
