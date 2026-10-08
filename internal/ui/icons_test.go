package ui_test

import (
	"strings"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// iconFiles cover the four file statuses of the tree icons: added,
// deleted, renamed and modified.
var iconFiles = []diff.ReviewFile{
	{Path: "added.go", Lines: []diff.Line{{Kind: diff.Added, New: 1, Text: "x"}}},
	{Path: "deleted.go", Lines: []diff.Line{{Kind: diff.Deleted, Old: 1, Text: "x"}}},
	{Path: "renamed.go", OldPath: "old.go", Lines: []diff.Line{{Kind: diff.Context, Old: 1, New: 1, Text: "x"}}},
	{Path: "modified.go", Lines: []diff.Line{
		{Kind: diff.Deleted, Old: 1, Text: "x"},
		{Kind: diff.Added, New: 1, Text: "y"},
	}},
}

// TestIconsMarkTheFileKinds pins the acceptance criterion "Icons
// optional": with the option on, the tree carries the Nerd Font icons
// of the file statuses — plus circle for added, minus circle for
// deleted, exchange for renames, pencil for modified.
func TestIconsMarkTheFileKinds(t *testing.T) {
	m := start(ui.New(iconFiles, "main", themeFor, ui.Options{Icons: true}), 60, 12)
	m = press(leader(m), "d")

	for _, icon := range []string{"\uf055", "\uf056", "\uf0ec", "\uf040"} {
		if !strings.Contains(screen(m), icon) {
			t.Errorf("drawer lacks the icon %q:\n%s", icon, screen(m))
		}
	}
}

// TestWithoutIconsTheTreeStaysClean pins that the icons are off by
// default: no Nerd Font glyphs appear without the option.
func TestWithoutIconsTheTreeStaysClean(t *testing.T) {
	m := start(ui.New(iconFiles, "main", themeFor, ui.Options{}), 60, 12)
	m = press(leader(m), "d")

	for _, icon := range []string{"\uf055", "\uf056", "\uf0ec", "\uf040"} {
		if strings.Contains(screen(m), icon) {
			t.Errorf("drawer shows the icon %q without the option:\n%s", icon, screen(m))
		}
	}
}

// TestIconsRenderInTheTree pins the icon file list as a golden frame.
func TestIconsRenderInTheTree(t *testing.T) {
	m := start(ui.New(iconFiles, "main", themeFor, ui.Options{Icons: true}), 60, 12)
	m = press(leader(m), "d")

	assertGolden(t, "icons.txt", screen(m))
}
