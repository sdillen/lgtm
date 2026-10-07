package ui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

var file = diff.ReviewFile{
	Path: "main.go",
	Lines: []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "package main"},
		{Kind: diff.Deleted, Old: 2, Text: "old"},
		{Kind: diff.Added, New: 2, Text: "new"},
		{Kind: diff.Context, Old: 3, New: 3, Text: "end"},
	},
}

func TestShowsFirstFileWithOldAndNewLineNumbers(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}), 30, 5)

	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"        2 + new",
		"   3    3   end",
		"main.go",
	})
}

func TestJAndKScrollWithinTheFile(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}), 30, 3)

	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"main.go",
	})

	m = press(m, "j", "j", "j", "j")
	assertScreen(t, m, []string{
		"        2 + new",
		"   3    3   end",
		"main.go",
	})

	m = press(m, "k")
	assertScreen(t, m, []string{
		"   2      - old",
		"        2 + new",
		"main.go",
	})

	m = press(m, "k", "k", "k")
	assertScreen(t, m, []string{
		"   1    1   package main",
		"   2      - old",
		"main.go",
	})
}

func TestQQuits(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}), 30, 5)

	_, cmd := m.Update(key("q"))

	if cmd == nil {
		t.Fatal("q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("q returned %T, want tea.QuitMsg", cmd())
	}
}

func TestLongLinesAreCutAtTheTerminalWidth(t *testing.T) {
	long := diff.ReviewFile{Path: "long.txt", Lines: []diff.Line{
		{Kind: diff.Added, New: 1, Text: "0123456789abcdef"},
	}}
	m := start(ui.New([]diff.ReviewFile{long}), 20, 2)

	assertScreen(t, m, []string{
		"        1 + 01234567",
		"long.txt",
	})
}

func TestTabsAreExpandedToSpaces(t *testing.T) {
	tabbed := diff.ReviewFile{Path: "t.go", Lines: []diff.Line{
		{Kind: diff.Context, Old: 1, New: 1, Text: "\tx"},
	}}
	m := start(ui.New([]diff.ReviewFile{tabbed}), 30, 2)

	assertScreen(t, m, []string{
		"   1    1       x",
		"t.go",
	})
}

func TestWithoutChangesShowsHint(t *testing.T) {
	m := start(ui.New(nil), 30, 3)

	m = press(m, "j", "k")
	assertScreen(t, m, []string{"No changes to review. Press q to quit."})
}

func start(m tea.Model, width, height int) tea.Model {
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func press(m tea.Model, keys ...string) tea.Model {
	for _, k := range keys {
		m, _ = m.Update(key(k))
	}
	return m
}

func key(k string) tea.KeyPressMsg {
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func assertScreen(t *testing.T, m tea.Model, want []string) {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View().Content), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	got := strings.Join(lines, "\n")
	if got != strings.Join(want, "\n") {
		t.Errorf("screen:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
}
