package ui

import (
	"slices"
	"sort"
	"strings"
)

// treeRow is one row of the file tree: a directory heading or a file.
type treeRow struct {
	// dir is true for a directory heading; headings carry no file.
	dir bool
	// name is the directory or file base name, without the path.
	name string
	// indent is the tree depth, 0 at the root.
	indent int
	// file is the index into the review's file list, for file rows.
	file int
}

// treeNode is one node of the directory tree the file list builds.
type treeNode struct {
	dirs  map[string]*treeNode
	files []string
}

// fileTree turns the file paths of the review into tree rows: nested
// directories with the files indented under them, directories before
// files, both alphabetically. Bare root files come last, also
// alphabetically.
func fileTree(paths []string) []treeRow {
	root := &treeNode{dirs: map[string]*treeNode{}}
	for _, p := range paths {
		parts := strings.Split(p, "/")
		n := root
		for _, part := range parts[:len(parts)-1] {
			child, ok := n.dirs[part]
			if !ok {
				child = &treeNode{dirs: map[string]*treeNode{}}
				n.dirs[part] = child
			}
			n = child
		}
		n.files = append(n.files, parts[len(parts)-1])
	}
	index := map[string]int{}
	for i, p := range paths {
		index[p] = i
	}
	var rows []treeRow
	var walk func(n *treeNode, indent int, prefix string)
	walk = func(n *treeNode, indent int, prefix string) {
		names := make([]string, 0, len(n.dirs))
		for name := range n.dirs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			rows = append(rows, treeRow{dir: true, name: name, indent: indent})
			walk(n.dirs[name], indent+1, prefix+name+"/")
		}
		files := slices.Clone(n.files)
		slices.Sort(files)
		for _, name := range files {
			rows = append(rows, treeRow{name: name, indent: indent, file: index[prefix+name]})
		}
	}
	walk(root, 0, "")
	return rows
}

// fuzzyScore scores query against path as a case-insensitive subsequence
// match; ok is false when query is not a subsequence of path. Lower
// scores are better: 0 is an exact match, every skipped character adds
// cost, and a match at the start of a path segment (after a separator)
// is a bonus.
func fuzzyScore(query, path string) (score int, ok bool) {
	q := strings.ToLower(query)
	if q == "" {
		return 0, true
	}
	t := strings.ToLower(path)
	pos := 0
	for _, r := range q {
		idx := strings.IndexRune(t[pos:], r)
		if idx < 0 {
			return 0, false
		}
		idx += pos
		if idx > pos {
			score += (idx - pos) * 2
		}
		if idx > 0 && isSegmentStart(t[idx-1]) {
			score -= 4
		}
		pos = idx + 1
	}
	return score, true
}

// isSegmentStart reports whether b is a path separator or a name
// separator, where a match is worth a bonus.
func isSegmentStart(b byte) bool {
	return b == '/' || b == '.' || b == '-' || b == '_'
}
