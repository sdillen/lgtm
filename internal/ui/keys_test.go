package ui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// fast shows which-key without the human-scale delay.
var fast = ui.Options{WhichKeyDelay: time.Nanosecond}

func withKeys(t *testing.T, ov keymap.Overrides) ui.Options {
	t.Helper()
	km, err := ui.Keymap(ov)
	if err != nil {
		t.Fatalf("Keymap: %v", err)
	}
	return ui.Options{Keymap: km, WhichKeyDelay: time.Nanosecond}
}

// pressWait presses the keys and runs the command of the last one (the
// which-key timer), feeding its message back like the runtime would.
func pressWait(m tea.Model, keys ...string) tea.Model {
	var cmd tea.Cmd
	for _, k := range keys {
		m, cmd = m.Update(key(k))
	}
	if cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
}

// typeText types a command line text key by key.
func typeText(m tea.Model, text string) tea.Model {
	for _, r := range text {
		if r == ' ' {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
			continue
		}
		m = press(m, string(r))
	}
	return m
}

func enter(m tea.Model) (tea.Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// The cursor row is padded to the full width with its background, so
// trailing blanks are not part of what a test compares.
func screen(m tea.Model) string {
	lines := strings.Split(withoutRule(ansi.Strip(m.View().Content)), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

// withoutRule drops the rule above the status line from a stripped
// screen. The row tests were written before the rule existed; start
// adds the row it takes, so their expectations stay valid. The rule
// itself is pinned by TestStatusLineIsSetOffByARule.
func withoutRule(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if t := strings.TrimRight(l, " "); len(t) >= 9 && strings.Trim(t, "─") == "" {
			lines = append(lines[:i], lines[i+1:]...)
			break
		}
	}
	return strings.Join(lines, "\n")
}

func TestDefaultKeymapIsValid(t *testing.T) {
	if _, err := ui.Keymap(keymap.Overrides{}); err != nil {
		t.Fatal(err)
	}
}

func TestReboundKeyMovesTheCursor(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor,
		withKeys(t, keymap.Overrides{Keys: map[keymap.Action][]string{"down": {"n"}}})), 30, 3)

	m = press(m, "j", "n", "n", "n")
	assertScreen(t, m, []string{
		"        2 + new",
		"   3    3   end",
		"main.go  1/1  +1 −1",
	})
}

func TestLeaderIsConfigurable(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor,
		withKeys(t, keymap.Overrides{Leader: ","})), 40, 3)

	m = press(m, ",", "u")
	if !strings.Contains(screen(m), "[unified]") {
		t.Errorf(",u did not toggle the unified view:\n%s", screen(m))
	}
}

func TestWhichKeyShowsTheFollowUpKeysAfterTheDelay(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, fast), 40, 12)

	m, cmd := m.Update(key("]"))
	if strings.Contains(screen(m), "next hunk") {
		t.Fatalf("which-key shown before the delay:\n%s", screen(m))
	}
	if cmd == nil {
		t.Fatal("prefix key returned no which-key timer")
	}
	m, _ = m.Update(cmd())
	for _, want := range []string{"next", "c", "next hunk", "f", "next file"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("which-key lacks %q:\n%s", want, screen(m))
		}
	}

	// The second key runs the binding and closes the popup.
	m = press(m, "c")
	if strings.Contains(screen(m), "next hunk") {
		t.Errorf("which-key still shown after ]c:\n%s", screen(m))
	}
}

func TestWhichKeyStaysHiddenWhenTheSequenceIsDoneInTime(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, fast), 40, 12)

	m, cmd := m.Update(key("g"))
	m = press(m, "g", "]")
	// The timer of the first g fires late, during the next prefix.
	m, _ = m.Update(cmd())

	if strings.Contains(screen(m), "next hunk") {
		t.Errorf("stale timer opened which-key:\n%s", screen(m))
	}
}

func TestWhichKeyShowsLeaderBindingsWithGroups(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, fast), 50, 12)

	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m, _ = m.Update(cmd())

	for _, want := range []string{"<space>", "u", "toggle unified view"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("leader which-key lacks %q:\n%s", want, screen(m))
		}
	}
}

func TestWhichKeyShowsReboundKeys(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor,
		withKeys(t, keymap.Overrides{Keys: map[keymap.Action][]string{"next-hunk": {"]h"}}})), 40, 12)

	m = pressWait(m, "]")

	if !strings.Contains(screen(m), "h  next hunk") {
		t.Errorf("which-key lacks the rebound ]h:\n%s", screen(m))
	}
}

func TestEscCancelsAPendingSequence(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, fast), 40, 12)

	m = pressWait(m, "]")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = press(m, "c")

	if strings.Contains(screen(m), "next hunk") {
		t.Errorf("which-key still shown after <esc>:\n%s", screen(m))
	}
	// c acts on the first row, a fold marker: the cursor did not move,
	// so the toast says it is not on a code line.
	if !strings.Contains(screen(m), "move the cursor to a code line") {
		t.Errorf("view moved:\n%s", screen(m))
	}
}

func TestStatusLineShowsPendingKeys(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 40, 3)

	m = press(m, "3", "]")

	if !strings.Contains(screen(m), "3]") {
		t.Errorf("status line lacks the pending keys:\n%s", screen(m))
	}
}

func TestQuestionMarkShowsTheFullHelp(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 80, 60)

	m = press(m, "?")
	for _, want := range []string{"j, <down>", "line down", "]c", "next hunk", "<C-d>", "?", ":", "q"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("help lacks %q:\n%s", want, screen(m))
		}
	}

	m = press(m, "?")
	if strings.Contains(screen(m), "next hunk") {
		t.Errorf("? did not close the help:\n%s", screen(m))
	}
}

func TestHelpListsTheCommands(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 80, 60)

	m = press(m, "?")
	for _, want := range []string{":q", ":theme [name]", ":set [no]unified"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("help lacks %q:\n%s", want, screen(m))
		}
	}
}

func TestHelpShowsReboundKeysAndLeader(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, withKeys(t, keymap.Overrides{
		Leader: ",",
		Keys:   map[keymap.Action][]string{"next-hunk": {"]h"}},
	})), 80, 40)

	m = press(m, "?")
	for _, want := range []string{"]h", ",u"} {
		if !strings.Contains(screen(m), want) {
			t.Errorf("help lacks %q:\n%s", want, screen(m))
		}
	}
	if strings.Contains(screen(m), "]c") {
		t.Errorf("help still shows the replaced ]c:\n%s", screen(m))
	}
}

func TestHelpScrollsAndQClosesItWithoutQuitting(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 60, 8)

	m = press(m, "?")
	first := screen(m)
	m = press(m, "j")
	if screen(m) == first {
		t.Errorf("j did not scroll the help:\n%s", screen(m))
	}
	m, cmd := m.Update(key("q"))
	if cmd != nil {
		t.Fatalf("q in the help returned %T, want no command", cmd())
	}
	if !strings.HasPrefix(screen(m), "⋯ 1 unveränderte Zeile ⋯") {
		t.Errorf("q did not close the help:\n%s", screen(m))
	}
}

func TestColonOpensTheCommandLine(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 30, 3)

	m = typeText(press(m, ":"), "se")
	lines := strings.Split(screen(m), "\n")
	if got := strings.TrimRight(lines[len(lines)-1], " "); got != ":se" {
		t.Errorf("bottom line = %q, want :se", got)
	}

	// Typing does not move the view: j is text, not a motion.
	m = typeText(m, "j")
	if !strings.HasPrefix(screen(m), "   1    1   package main") {
		t.Errorf("typing moved the view:\n%s", screen(m))
	}
}

func TestEscAndBackspaceLeaveTheCommandLine(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 30, 3)

	m = typeText(press(m, ":"), "x")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if !strings.HasSuffix(screen(m), "main.go  1/1  +1 −1") {
		t.Errorf("<esc> did not close the command line:\n%s", screen(m))
	}

	m = typeText(press(m, ":"), "x")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if !strings.HasSuffix(screen(m), "main.go  1/1  +1 −1") {
		t.Errorf("<bs> on an empty line did not close the command line:\n%s", screen(m))
	}
}

func TestColonQQuits(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 30, 3)

	m = typeText(press(m, ":"), "q")
	_, cmd := enter(m)

	if cmd == nil {
		t.Fatal(":q returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf(":q returned %T, want tea.QuitMsg", cmd())
	}
}

func TestColonThemeSwitchesTheTheme(t *testing.T) {
	var asked string
	opts := ui.Options{LoadTheme: func(name string) (theme.Theme, error) {
		asked = name
		return theme.Default(theme.KindLight), nil
	}}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 30, 5)

	m = typeText(press(m, ":"), "theme tokyo-night-day")
	m, _ = enter(m)

	if asked != "tokyo-night-day" {
		t.Errorf("LoadTheme(%q), want tokyo-night-day", asked)
	}
	// The status line uses the day accent #2e7de9.
	if !strings.Contains(m.View().Content, "38;2;46;125;233") {
		t.Errorf("view lacks the day accent after :theme:\n%s", m.View().Content)
	}
}

func TestColonThemeWithoutNameShowsTheCurrentTheme(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 3)

	m = typeText(press(m, ":"), "theme")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "tokyo-night-night") {
		t.Errorf(":theme did not name the theme:\n%s", screen(m))
	}
}

func TestColonThemeReportsLoadErrors(t *testing.T) {
	opts := ui.Options{LoadTheme: func(name string) (theme.Theme, error) {
		return theme.Theme{}, errors.New("theme \"nope\" not found")
	}}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 3)

	m = typeText(press(m, ":"), "theme nope")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "not found") {
		t.Errorf(":theme nope did not report the error:\n%s", screen(m))
	}
	// The message disappears with the next key.
	m = press(m, "j")
	if strings.Contains(screen(m), "not found") {
		t.Errorf("message still shown after a key:\n%s", screen(m))
	}
}

func TestColonSetTogglesTheUnifiedView(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 40, 4)

	m = typeText(press(m, ":"), "set unified")
	m, _ = enter(m)
	if !strings.Contains(screen(m), "[unified]") {
		t.Errorf(":set unified did not switch:\n%s", screen(m))
	}

	m = typeText(press(m, ":"), "set nounified")
	m, _ = enter(m)
	if strings.Contains(screen(m), "[unified]") {
		t.Errorf(":set nounified did not switch back:\n%s", screen(m))
	}

	m = typeText(press(m, ":"), "set unified!")
	m, _ = enter(m)
	if !strings.Contains(screen(m), "[unified]") {
		t.Errorf(":set unified! did not toggle:\n%s", screen(m))
	}
}

func TestColonSetQueriesAnOption(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 40, 4)

	m = typeText(press(m, ":"), "set unified?")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "nounified") {
		t.Errorf(":set unified? did not report the option:\n%s", screen(m))
	}
}

func TestColonSetRejectsUnknownOptions(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{big}, "main", themeFor, ui.Options{}), 40, 4)

	m = typeText(press(m, ":"), "set bogus")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "unknown option: bogus") {
		t.Errorf(":set bogus did not report the error:\n%s", screen(m))
	}
}

func TestUnknownCommandIsReported(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 3)

	m = typeText(press(m, ":"), "frobnicate")
	m, _ = enter(m)

	if !strings.Contains(screen(m), "unknown command: frobnicate") {
		t.Errorf("unknown command not reported:\n%s", screen(m))
	}
}

func TestCommandLineWorksWithoutChanges(t *testing.T) {
	m := start(ui.New(nil, "main", themeFor, ui.Options{}), 30, 5)

	m = typeText(press(m, ":"), "q")
	_, cmd := enter(m)

	if cmd == nil {
		t.Fatal(":q returned no command without changes")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf(":q returned %T, want tea.QuitMsg", cmd())
	}
}

func TestTabCompletesAUniqueCommand(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 3)

	m = tab(typeText(press(m, ":"), "lg"))

	if !strings.Contains(screen(m), ":lgtm") {
		t.Errorf("Tab did not complete lg to lgtm:\n%s", screen(m))
	}
}

func TestTabCompletesASetOption(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 3)

	m = tab(typeText(press(m, ":"), "set igno"))
	if !strings.Contains(screen(m), ":set ignorews") {
		t.Errorf("Tab did not complete igno to ignorews:\n%s", screen(m))
	}
}

func TestTabCompletesAThemeName(t *testing.T) {
	opts := ui.Options{ThemeNames: func() []string { return []string{"solarized", "tokyo-night-day"} }}
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, opts), 40, 3)

	m = tab(typeText(press(m, ":"), "theme tok"))

	if !strings.Contains(screen(m), ":theme tokyo-night-day") {
		t.Errorf("Tab did not complete tok to tokyo-night-day:\n%s", screen(m))
	}
}

func TestTabCyclesThroughSeveralCompletions(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 3)

	m = tab(typeText(press(m, ":"), "set no"))
	if !strings.Contains(screen(m), ":set nounified") {
		t.Fatalf("first Tab should offer nounified:\n%s", screen(m))
	}
	m = tab(m)
	if !strings.Contains(screen(m), ":set nosidebar") {
		t.Errorf("second Tab should offer nosidebar:\n%s", screen(m))
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if !strings.Contains(screen(m), ":set nounified") {
		t.Errorf("Shift+Tab should go back to nounified:\n%s", screen(m))
	}
}

func TestTypingAfterTabStartsANewCompletion(t *testing.T) {
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, ui.Options{}), 40, 3)

	m = tab(typeText(press(m, ":"), "set no")) // nounified
	m = tab(typeText(m, "w"))                  // set nounifiedw: no match

	if !strings.Contains(screen(m), ":set nounifiedw") {
		t.Errorf("typing must discard the cycle:\n%s", screen(m))
	}
}
