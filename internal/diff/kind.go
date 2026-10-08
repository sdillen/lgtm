package diff

import (
	"path/filepath"
	"strings"
)

// FileKind classifies how the change of a file is presented.
type FileKind int

const (
	// KindDiff is a regular text change, shown as the full-file diff.
	KindDiff FileKind = iota
	// KindBinary is a binary change: only the sizes of the two
	// versions are shown.
	KindBinary
	// KindMode is a mode-only change: a compact info entry with the
	// old and the new file mode.
	KindMode
	// KindSubmodule is a gitlink change: a compact info entry with the
	// old and the new commit hash.
	KindSubmodule
	// KindGenerated is a text change of a generated file, a lockfile
	// or a vendor tree: the diff is kept, but the UI collapses the
	// file and sorts it to the end of the file list.
	KindGenerated
)

// Classify determines how the change of a file is presented: binary
// files, generated files, lockfiles and vendor trees get their own
// kind, everything else is a regular text diff. generated reports
// whether .gitattributes marks path as linguist-generated; nil skips
// the attribute check.
func Classify(path, old, new string, generated func(string) bool) FileKind {
	if IsBinary(old) || IsBinary(new) {
		return KindBinary
	}
	if generatedPath(path) || (generated != nil && generated(path)) {
		return KindGenerated
	}
	return KindDiff
}

// IsBinary reports whether content looks binary: it contains a NUL
// byte, the same heuristic as git.
func IsBinary(content string) bool {
	return strings.IndexByte(content, 0) >= 0
}

// Mode returns the ReviewFile of a mode-only change: an info entry with
// the old and the new file mode, no lines.
func Mode(path string, oldMode, newMode uint32) ReviewFile {
	return ReviewFile{Path: path, Kind: KindMode, OldMode: oldMode, NewMode: newMode}
}

// Submodule returns the ReviewFile of a submodule change: an info entry
// with the old and the new commit hash, no lines.
func Submodule(path, oldSub, newSub string) ReviewFile {
	return ReviewFile{Path: path, Kind: KindSubmodule, OldSub: oldSub, NewSub: newSub}
}

// generatedPath reports whether the built-in patterns mark path as
// generated: a vendor or node_modules tree, a lockfile or a generated
// artifact. Repo-specific marks come from .gitattributes via
// GitAttributes.
func generatedPath(path string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == "vendor" || seg == "node_modules" {
			return true
		}
	}
	base := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		base = path[i+1:]
	}
	switch base {
	case "go.sum", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock",
		"pnpm-lock.yaml", "bun.lockb", "Cargo.lock", "Gemfile.lock",
		"composer.lock", "poetry.lock", "Pipfile.lock", "uv.lock",
		"mix.lock", "pubspec.lock", "Package.resolved":
		return true
	}
	for _, ext := range []string{".min.js", ".min.css", ".map", ".pb.go", ".pb.h", ".pb.cc"} {
		if strings.HasSuffix(base, ext) {
			return true
		}
	}
	return false
}

// GitAttributes is a parsed .gitattributes: the patterns that carry the
// linguist-generated attribute.
type GitAttributes struct {
	patterns []string
}

// ParseGitAttributes parses the linguist-generated patterns of the
// content of a .gitattributes file. Other attributes are ignored; a
// negated or false-valued linguist-generated attribute is ignored too.
// The parser is deliberately minimal: no **, no directory recursion
// magic beyond a trailing slash.
func ParseGitAttributes(content string) GitAttributes {
	var g GitAttributes
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		for _, attr := range fields[1:] {
			if strings.HasPrefix(attr, "-") {
				continue
			}
			name, value, hasValue := strings.Cut(attr, "=")
			if name != "linguist-generated" {
				continue
			}
			if hasValue && value != "true" && value != "1" {
				continue
			}
			g.patterns = append(g.patterns, fields[0])
		}
	}
	return g
}

// Generated reports whether path matches one of the parsed patterns.
func (g GitAttributes) Generated(path string) bool {
	for _, pattern := range g.patterns {
		if attrMatch(pattern, path) {
			return true
		}
	}
	return false
}

// attrMatch matches a .gitattributes pattern against path, following
// git's rules for the simple cases: a trailing slash matches the whole
// directory, a pattern with a slash is anchored at the root, and a
// pattern without a slash matches any path component.
func attrMatch(pattern, path string) bool {
	if strings.HasSuffix(pattern, "/") {
		dir := strings.TrimSuffix(pattern, "/")
		return path == dir || strings.HasPrefix(path, dir+"/")
	}
	if strings.Contains(pattern, "/") {
		ok, err := filepath.Match(pattern, path)
		return err == nil && ok
	}
	for _, part := range strings.Split(path, "/") {
		ok, err := filepath.Match(pattern, part)
		if err == nil && ok {
			return true
		}
	}
	return false
}
