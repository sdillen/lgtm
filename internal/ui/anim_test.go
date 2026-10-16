package ui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/ui"
)

// TestSmoothScrollJumpToTopNeverRendersNegativeRows pins that the
// spring overshoot of gg after G does not push the offset below zero:
// every frame of the glide renders without panicking.
func TestSmoothScrollJumpToTopNeverRendersNegativeRows(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{long()}, "main", themeFor, animated()), 40, 5)

	m, cmd := m.Update(key("G"))
	m = settle(m, cmd)
	m, _ = m.Update(key("g"))
	m, cmd = m.Update(key("g"))
	for i := 0; cmd != nil && i < 200; i++ {
		_ = m.View()
		m, cmd = m.Update(cmd())
	}
	_ = m.View()
}

// long is a file whose changes every few lines keep the rows unfolded,
// so a jump between top and bottom spans many rows.
func long() diff.ReviewFile {
	var a, b strings.Builder
	for i := range 300 {
		fmt.Fprintf(&a, "line %d\n", i)
		if i%3 == 0 {
			fmt.Fprintf(&b, "LINE %d\n", i)
		} else {
			fmt.Fprintf(&b, "line %d\n", i)
		}
	}
	return diff.New("long.go", a.String(), b.String())
}
