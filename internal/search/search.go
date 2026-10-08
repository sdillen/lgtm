// Package search finds context beyond the diff: words in the current
// file and the whole repository, and heuristic definitions of the symbol
// under the cursor. The git-aware half is the narrow Repo interface
// (implemented by source.Source); the matching half is pure.
package search

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Hit is one match of a search: a file and a 1-based line.
type Hit struct {
	Path string
	Line int    // 1-based
	Text string // the line's text, without a terminator
}

// Repo is the working tree the searcher asks about.
type Repo interface {
	// Files lists the files of the working tree: everything tracked
	// plus untracked files that are not ignored, sorted by path.
	Files() ([]string, error)
	// Read returns the current content of path, as it is in the
	// working tree.
	Read(path string) (string, error)
}

// Searcher answers context questions about the working tree: file
// listing, file content, word search and heuristic definitions.
type Searcher struct {
	repo Repo
}

// New returns a Searcher over repo.
func New(repo Repo) *Searcher {
	return &Searcher{repo: repo}
}

// Files lists the files of the working tree, as the context picker and
// the project search build on.
func (s *Searcher) Files() ([]string, error) {
	return s.repo.Files()
}

// Read returns the current content of path, as it is in the working
// tree.
func (s *Searcher) Read(path string) (string, error) {
	return s.repo.Read(path)
}

// FindWord finds the lines of the repository containing word as a whole
// word, in path order. Ignored files are excluded by the Repo's file
// listing, binary files by their content.
func (s *Searcher) FindWord(word string) ([]Hit, error) {
	if word == "" {
		return nil, nil
	}
	var hits []Hit
	err := s.forEachLine(func(path string, line int, text string) {
		if hasWord(text, word) {
			hits = append(hits, Hit{Path: path, Line: line, Text: text})
		}
	})
	return hits, err
}

// FindDefinitions finds the lines that heuristically define symbol:
// language-specific declaration patterns for Go, Python,
// TypeScript/JavaScript and Rust, matched as whole words. Call sites and
// comment lines do not match.
func (s *Searcher) FindDefinitions(symbol string) ([]Hit, error) {
	if symbol == "" {
		return nil, nil
	}
	files, err := s.Files()
	if err != nil {
		return nil, err
	}
	compiled := map[string][]*regexp.Regexp{}
	var hits []Hit
	for _, path := range files {
		lang, ok := defLang[extOf(path)]
		if !ok {
			continue
		}
		rs := compiled[lang]
		if rs == nil {
			rs = compilePatterns(defPatterns[lang], symbol)
			compiled[lang] = rs
		}
		content, err := s.Read(path)
		if err != nil || IsBinary(content) {
			continue
		}
		for i, line := range SplitLines(content) {
			if isCommentLine(line) {
				continue
			}
			for _, r := range rs {
				if r.MatchString(line) {
					hits = append(hits, Hit{Path: path, Line: i + 1, Text: line})
					break
				}
			}
		}
	}
	return hits, nil
}

// forEachLine calls fn for every line of every file of the repository,
// in path order, skipping binary files and files that cannot be read.
func (s *Searcher) forEachLine(fn func(path string, line int, text string)) error {
	files, err := s.Files()
	if err != nil {
		return err
	}
	for _, path := range files {
		content, err := s.Read(path)
		if err != nil || IsBinary(content) {
			continue
		}
		for i, line := range SplitLines(content) {
			fn(path, i+1, line)
		}
	}
	return nil
}

// FindInFile returns the indexes (0-based) of the lines containing
// pattern, in order.
func FindInFile(lines []string, pattern string) []int {
	if pattern == "" {
		return nil
	}
	var out []int
	for i, line := range lines {
		if strings.Contains(line, pattern) {
			out = append(out, i)
		}
	}
	return out
}

// FindWordInFile returns the indexes (0-based) of the lines containing
// word as a whole word, in order.
func FindWordInFile(lines []string, word string) []int {
	if word == "" {
		return nil
	}
	var out []int
	for i, line := range lines {
		if hasWord(line, word) {
			out = append(out, i)
		}
	}
	return out
}

// WordAt returns the word of line containing the byte offset col, with
// its byte span in line. A word is a maximal run of letters, digits and
// underscores. col that does not sit on a word rune yields no word.
func WordAt(line string, col int) (string, int, int) {
	if col < 0 || col > len(line) {
		return "", 0, 0
	}
	r, _ := utf8.DecodeRuneInString(line[col:])
	if !wordRune(r) {
		return "", 0, 0
	}
	start, end := col, col
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(line[:start])
		if !wordRune(r) {
			break
		}
		start -= size
	}
	for end < len(line) {
		r, size := utf8.DecodeRuneInString(line[end:])
		if !wordRune(r) {
			break
		}
		end += size
	}
	return line[start:end], start, end
}

// SplitLines splits content into lines without terminators. A trailing
// newline does not add an empty line.
func SplitLines(content string) []string {
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// IsBinary reports whether content looks binary: it contains a NUL
// byte.
func IsBinary(content string) bool {
	return strings.IndexByte(content, 0) >= 0
}

// hasWord reports whether line contains word as a whole word: not
// preceded or followed by a word rune.
func hasWord(line, word string) bool {
	for off := 0; ; {
		idx := strings.Index(line[off:], word)
		if idx < 0 {
			return false
		}
		pos := off + idx
		before := pos == 0 || !wordRuneAt(line, pos-1)
		after := pos+len(word) == len(line) || !wordRuneAt(line, pos+len(word))
		if before && after {
			return true
		}
		off = pos + 1
	}
}

// wordRuneAt reports whether the rune starting at the byte offset i of
// line is a word rune. i inside a multi-byte rune yields false.
func wordRuneAt(line string, i int) bool {
	r, _ := utf8.DecodeRuneInString(line[i:])
	return wordRune(r)
}

// wordRune reports whether r is a word rune: a letter, a digit or an
// underscore, like the tokenizer of the diff module.
func wordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// defLang maps file extensions to the language of the definition
// patterns.
var defLang = map[string]string{
	".go":  "go",
	".py":  "python",
	".ts":  "ts",
	".tsx": "ts",
	".js":  "ts",
	".jsx": "ts",
	".mjs": "ts",
	".cjs": "ts",
	".rs":  "rust",
}

// defPatterns are the declaration patterns of each language, as fmt
// templates with %s for the symbol.
var defPatterns = map[string][]string{
	"go": {
		`\bfunc\s*(\([^)]*\)\s*)?%s\b`,
		`\btype\s+%s\b`,
		`\bvar\s+%s\b`,
		`\bconst\s+%s\b`,
	},
	"python": {
		`\b(?:async\s+)?def\s+%s\b`,
		`\b(?:async\s+)?class\s+%s\b`,
	},
	"ts": {
		`\b(?:async\s+)?function\s+%s\b`,
		`\bclass\s+%s\b`,
		`\b(?:const|let|var)\s+%s\s*=`,
		`\binterface\s+%s\b`,
		`\btype\s+%s\b`,
		`\benum\s+%s\b`,
	},
	"rust": {
		`\bfn\s+%s\b`,
		`\bstruct\s+%s\b`,
		`\benum\s+%s\b`,
		`\btrait\s+%s\b`,
		`\btype\s+%s\b`,
		`\bconst\s+%s\b`,
		`\bstatic\s+%s\b`,
		`\bmod\s+%s\b`,
	},
}

// compilePatterns compiles the patterns of a language for the symbol.
func compilePatterns(patterns []string, symbol string) []*regexp.Regexp {
	sym := regexp.QuoteMeta(symbol)
	rs := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		rs[i] = regexp.MustCompile(fmt.Sprintf(p, sym))
	}
	return rs
}

// extOf returns the extension of path, "" if it has none.
func extOf(path string) string {
	i := strings.LastIndexByte(path, '.')
	if i < 0 {
		return ""
	}
	return path[i:]
}

// isCommentLine reports whether the line is a comment: it starts with a
// comment marker after leading whitespace. Keeping comment lines out of
// the definition patterns avoids matching comment text like
// "// type Fake struct".
func isCommentLine(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	for _, marker := range []string{"//", "/*", "*/", "*", "#", "<!--"} {
		if strings.HasPrefix(trimmed, marker) {
			return true
		}
	}
	return false
}
