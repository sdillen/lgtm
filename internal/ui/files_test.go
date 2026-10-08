package ui

import (
	"strings"
	"testing"
)

// TestFileTreeGroupsPathsByDirectory pins the tree shape: nested
// directories with their files indented, directories before files,
// both alphabetically, and the file index pointing into the file list.
func TestFileTreeGroupsPathsByDirectory(t *testing.T) {
	paths := []string{"cmd/main.go", "internal/ui/ui.go", "internal/ui/overlay.go", "README.md"}
	rows := fileTree(paths)

	var names []string
	for _, r := range rows {
		names = append(names, strings.Repeat("  ", r.indent)+r.name)
	}
	want := []string{
		"cmd",
		"  main.go",
		"internal",
		"  ui",
		"    overlay.go",
		"    ui.go",
		"README.md",
	}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Errorf("tree:\n%s\nwant:\n%s", strings.Join(names, "\n"), strings.Join(want, "\n"))
	}

	// overlay.go and ui.go point at the right files.
	var overlay, uigo, maingo int
	for _, r := range rows {
		switch r.name {
		case "overlay.go":
			overlay = r.file
		case "ui.go":
			uigo = r.file
		case "main.go":
			maingo = r.file
		}
	}
	if paths[overlay] != "internal/ui/overlay.go" || paths[uigo] != "internal/ui/ui.go" || paths[maingo] != "cmd/main.go" {
		t.Errorf("file rows point at the wrong files: overlay=%d ui.go=%d main.go=%d", overlay, uigo, maingo)
	}
}

// TestFileTreeMarksDirectoryRows pins that the dir flag tells headings
// apart from files, so the drawer cursor skips headings.
func TestFileTreeMarksDirectoryRows(t *testing.T) {
	rows := fileTree([]string{"a/b.txt", "c.txt"})
	if !rows[0].dir || rows[0].name != "a" {
		t.Errorf("row 0 = %+v, want the a/ heading", rows[0])
	}
	if rows[1].dir || rows[1].name != "b.txt" {
		t.Errorf("row 1 = %+v, want the b.txt file", rows[1])
	}
	// Bare root files come after all directories, without an indent.
	if rows[2].dir || rows[2].name != "c.txt" || rows[2].indent != 0 {
		t.Errorf("row 2 = %+v, want the bare root file c.txt", rows[2])
	}
}

// TestFuzzyScoreMatchesASubsequence pins the acceptance criterion "jump
// directly to a file": the query needs no contiguous match, case does
// not matter and a non-subsequence does not match.
func TestFuzzyScoreMatchesASubsequence(t *testing.T) {
	for _, tc := range []struct {
		query, path string
		want        bool
	}{
		{"ui", "internal/ui/ui.go", true},
		{"uigo", "internal/ui/ui.go", true},
		{"UI", "internal/ui/ui.go", true},
		{"main", "cmd/main.go", true},
		{"zz", "cmd/main.go", false},
		{"gogo", "cmd/main.go", false},
	} {
		_, ok := fuzzyScore(tc.query, tc.path)
		if ok != tc.want {
			t.Errorf("fuzzyScore(%q, %q) ok = %v, want %v", tc.query, tc.path, ok, tc.want)
		}
	}
}

// TestFuzzyScorePrefersEarlySegmentMatches pins the ordering: a match at
// a path segment start scores better than the same characters buried in
// a name, so typing "ui.go" surfaces internal/ui/ui.go before a file
// that merely contains the letters.
func TestFuzzyScorePrefersEarlySegmentMatches(t *testing.T) {
	boundary, ok1 := fuzzyScore("ui.go", "a/ui.go")
	buried, ok2 := fuzzyScore("ui.go", "xuiy.gox.txt")
	if !ok1 || !ok2 {
		t.Fatalf("both must match: ok1=%v ok2=%v", ok1, ok2)
	}
	if boundary >= buried {
		t.Errorf("segment-start match scores %d, buried match %d; want segment-start to win", boundary, buried)
	}
}

// TestFuzzyScorePrefersEarlyMatches pins the ordering: with the same
// match shape, a match near the start of the path wins over one that
// starts later.
func TestFuzzyScorePrefersEarlyMatches(t *testing.T) {
	early, ok1 := fuzzyScore("uigo", "uigo.txt")
	late, ok2 := fuzzyScore("uigo", "ab/uigo.txt")
	if !ok1 || !ok2 {
		t.Fatalf("both must match: ok1=%v ok2=%v", ok1, ok2)
	}
	if early >= late {
		t.Errorf("early match scores %d, late match %d; want early to win", early, late)
	}
}

// TestFuzzyScoreEmptyQueryMatchesEverything pins that an empty query
// matches all files with score 0.
func TestFuzzyScoreEmptyQueryMatchesEverything(t *testing.T) {
	score, ok := fuzzyScore("", "anything/at/all.go")
	if !ok || score != 0 {
		t.Errorf("empty query: score = %d, ok = %v; want 0, true", score, ok)
	}
}

// TestFuzzyScoreExactMatchIsBest pins that an exact match is better than
// any subsequence with gaps; a plain exact match scores 0.
func TestFuzzyScoreExactMatchIsBest(t *testing.T) {
	exact, ok1 := fuzzyScore("uigo", "uigo")
	gapped, ok2 := fuzzyScore("uigo", "xuiygxo.txt")
	if !ok1 || !ok2 {
		t.Fatalf("both must match: ok1=%v ok2=%v", ok1, ok2)
	}
	if exact != 0 {
		t.Errorf("exact match scores %d, want 0", exact)
	}
	if gapped <= exact {
		t.Errorf("gapped match scores %d, want worse than the exact match", gapped)
	}
}
