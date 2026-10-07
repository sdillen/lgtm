package diff_test

import (
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

// TestBinaryContentIsClassifiedBinary pins that a change of a file with
// a NUL byte in either version is shown as a binary info entry: no
// lines, just the sizes of the two versions.
func TestBinaryContentIsClassifiedBinary(t *testing.T) {
	got := diff.New("img.png", "\x00\x01", "\x00\x01\x02")

	if got.Kind != diff.KindBinary {
		t.Errorf("Kind = %v, want KindBinary", got.Kind)
	}
	if got.OldSize != 2 || got.NewSize != 3 {
		t.Errorf("sizes = %d, %d; want 2, 3", got.OldSize, got.NewSize)
	}
	if len(got.Lines) != 0 || len(got.Hunks) != 0 || len(got.Folds) != 0 {
		t.Errorf("binary file carries lines: %+v", got.Lines)
	}
}

// TestDeletedBinaryKeepsTheOldSize pins that a deleted binary file still
// reports the size of the old version.
func TestDeletedBinaryKeepsTheOldSize(t *testing.T) {
	got := diff.New("gone.bin", "\x00\x01\x02", "")

	if got.Kind != diff.KindBinary || got.OldSize != 3 || got.NewSize != 0 {
		t.Errorf("Kind = %v, sizes = %d, %d; want KindBinary, 3, 0", got.Kind, got.OldSize, got.NewSize)
	}
}

// TestClassifyGeneratedPins the built-in generated file patterns: the
// vendor and dependency trees, lockfiles and generated artifacts.
func TestClassifyGenerated(t *testing.T) {
	for _, path := range []string{
		"vendor/github.com/foo/bar.go",
		"a/vendor/github.com/foo/bar.go",
		"node_modules/pkg/index.js",
		"a/node_modules/pkg/index.js",
		"go.sum",
		"package-lock.json",
		"npm-shrinkwrap.json",
		"yarn.lock",
		"pnpm-lock.yaml",
		"bun.lockb",
		"Cargo.lock",
		"Gemfile.lock",
		"composer.lock",
		"poetry.lock",
		"Pipfile.lock",
		"uv.lock",
		"mix.lock",
		"pubspec.lock",
		"Package.resolved",
		"assets/app.min.js",
		"assets/app.min.css",
		"assets/out.js.map",
		"api/api.pb.go",
	} {
		if got := diff.Classify(path, "a\n", "b\n", nil); got != diff.KindGenerated {
			t.Errorf("Classify(%q) = %v, want KindGenerated", path, got)
		}
	}
}

// TestClassifyTextPins that ordinary source files and near-lockfile
// names stay regular text diffs.
func TestClassifyText(t *testing.T) {
	for _, path := range []string{
		"main.go",
		"go.mod",
		"README.md",
		"assets/app.js",
		"assets/app.css",
		"api/api.go",
		"app.go.summary",
		"vendor.md",
	} {
		if got := diff.Classify(path, "a\n", "b\n", nil); got != diff.KindDiff {
			t.Errorf("Classify(%q) = %v, want KindDiff", path, got)
		}
	}
}

// TestClassifyUsesGitAttributes pins that the generated callback of the
// .gitattributes check marks a path generated even when the built-in
// patterns do not.
func TestClassifyUsesGitAttributes(t *testing.T) {
	got := diff.Classify("foo.txt", "a\n", "b\n", func(path string) bool {
		return path == "foo.txt"
	})
	if got != diff.KindGenerated {
		t.Errorf("Classify with attribute = %v, want KindGenerated", got)
	}
}

// TestNewWithAttrsClassifiesGenerated pins the wiring of the attribute
// callback into New: the file keeps its text diff but is marked
// generated.
func TestNewWithAttrsClassifiesGenerated(t *testing.T) {
	attrs := diff.ParseGitAttributes("*.txt linguist-generated\n")
	got := diff.NewWithAttrs("foo.txt", "a\n", "b\n", attrs.Generated)

	if got.Kind != diff.KindGenerated {
		t.Errorf("Kind = %v, want KindGenerated", got.Kind)
	}
	if len(got.Lines) != 2 {
		t.Errorf("generated file lost its diff: %d lines", len(got.Lines))
	}
}

// TestParseGitAttributesPins the minimal linguist-generated parsing:
// patterns with and without slashes, directory patterns, explicit true
// values; false values and negations are ignored.
func TestParseGitAttributes(t *testing.T) {
	attrs := diff.ParseGitAttributes(`
# generated artifacts
*.pb.go linguist-generated
docs/ linguist-generated=true
assets/*.js.map linguist-generated
*.go linguist-generated=false
*.md -linguist-generated
plain.txt other-attr

vendor/ linguist-generated
`)

	for path, want := range map[string]bool{
		"api/api.pb.go":        true,
		"nested/a.pb.go":       true,
		"docs/guide.md":        true,
		"docs":                 true,
		"assets/out.js.map":    true,
		"assets/deep/x.js.map": false, // slash pattern is anchored
		"main.go":              false,
		"readme.md":            false,
		"plain.txt":            false,
		"vendor/x/y/z.go":      true,
		"src/vendor":           false,
		"api.pb.go.txt":        false, // pattern without slash matches components only
	} {
		if got := attrs.Generated(path); got != want {
			t.Errorf("Generated(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestModeAndSubmoduleReviewFiles pin the info entries of mode-only and
// submodule changes: no lines, the modes and hashes carried as data.
func TestModeAndSubmoduleReviewFiles(t *testing.T) {
	mode := diff.Mode("script.sh", 0o100644, 0o100755)
	if mode.Kind != diff.KindMode || mode.OldMode != 0o100644 || mode.NewMode != 0o100755 {
		t.Errorf("Mode = %+v", mode)
	}
	if len(mode.Lines) != 0 {
		t.Errorf("mode file carries lines: %+v", mode.Lines)
	}

	sub := diff.Submodule("lib", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if sub.Kind != diff.KindSubmodule || sub.OldSub != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || sub.NewSub != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("Submodule = %+v", sub)
	}
	if len(sub.Lines) != 0 {
		t.Errorf("submodule file carries lines: %+v", sub.Lines)
	}
}

// TestAddedPins the new-file detection: only files whose whole view is
// added lines count, so the UI can drop the background wash.
func TestAdded(t *testing.T) {
	for _, tc := range []struct {
		name string
		file diff.ReviewFile
		want bool
	}{
		{"added file", diff.New("new.txt", "", "x\n"), true},
		{"modified file", diff.New("f.txt", "a\n", "b\n"), false},
		{"deleted file", diff.New("d.txt", "x\n", ""), false},
		{"binary file", diff.New("img.png", "\x00", "\x00"), false},
	} {
		if got := tc.file.Added(); got != tc.want {
			t.Errorf("%s: Added() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestCollapsedPins which files the UI collapses by default: deleted
// files and generated files. Binary files and regular changes stay open.
func TestCollapsed(t *testing.T) {
	for _, tc := range []struct {
		name string
		file diff.ReviewFile
		want bool
	}{
		{"deleted file", diff.New("gone.txt", "x\ny\n", ""), true},
		{"added file", diff.New("new.txt", "", "x\n"), false},
		{"modified file", diff.New("f.txt", "a\n", "b\n"), false},
		{"unchanged file", diff.New("f.txt", "a\n", "a\n"), false},
		{"generated file", diff.NewWithAttrs("f.txt", "a\n", "b\n", func(string) bool { return true }), true},
		{"binary file", diff.New("img.png", "\x00", "\x00"), false},
		{"mode change", diff.Mode("s.sh", 0o100644, 0o100755), false},
		{"submodule", diff.Submodule("lib", "a", "b"), false},
	} {
		if got := tc.file.Collapsed(); got != tc.want {
			t.Errorf("%s: Collapsed() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRenamedFileCarriesTheOldPath pins the rename display data: the
// ReviewFile keeps the old path alongside the new one.
func TestRenamedFileCarriesTheOldPath(t *testing.T) {
	got := diff.New("b.go", "package b\n", "package b\n")
	got.OldPath = "a.go"

	if got.Path != "b.go" || got.OldPath != "a.go" {
		t.Errorf("paths = %q, %q; want b.go, a.go", got.Path, got.OldPath)
	}
}

// TestGitAttributesIsEmptyWithoutLinguistGenerated pins that a
// .gitattributes without the attribute yields no generated paths.
func TestGitAttributesIsEmptyWithoutLinguistGenerated(t *testing.T) {
	attrs := diff.ParseGitAttributes("*.go text eol=lf\n")
	if attrs.Generated("x.go") {
		t.Error("Generated(x.go) = true, want false")
	}
}
