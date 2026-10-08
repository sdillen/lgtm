// Package ui renders ReviewFile models in the terminal. It knows nothing
// about git or diffing: the file view is a full-file view with folded
// unchanged runs, a toggleable classic unified view, and vim-like
// navigation. All fold and hunk regions come from the ReviewFile model.
package ui

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/search"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// contextWindow is how many unchanged lines surround each hunk in the
// classic unified diff view.
const contextWindow = 3

// defaultWhichKeyDelay is how long a prefix waits before which-key shows
// its follow-up keys.
const defaultWhichKeyDelay = 300 * time.Millisecond

// Options configure the review view. The zero value is the default.
type Options struct {
	// Keymap resolves keys to actions; nil means the default bindings.
	Keymap *keymap.Keymap
	// LoadTheme loads a theme by name for :theme; nil disables
	// switching.
	LoadTheme func(name string) (theme.Theme, error)
	// WhichKeyDelay is the wait before which-key appears; 0 means the
	// default of 300ms.
	WhichKeyDelay time.Duration
	// State is the review state the read status lives in; nil means a
	// session-only state.
	State *review.State
	// Dir is the worktree root the state is saved to; empty disables
	// saving.
	Dir string
	// CfgDir is the config directory holding the global prompt
	// template; empty disables it.
	CfgDir string
	// Branch, Base and Round are the prompt template data of the
	// review; Round is the current review round, 0 without rounds.
	Branch string
	Base   string
	Round  int
	// Copy copies the prompt to the clipboard; nil means the default
	// OSC 52/tool chain.
	Copy func(string) error
	// ApprovalDelay is the confirmation moment of :lgtm; 0 means the
	// default of 1.5s.
	ApprovalDelay time.Duration
	// ToastTTL is how long a toast stays; 0 means the default.
	ToastTTL time.Duration
	// Search answers context questions beyond the review: the repo
	// file listing, reading files in their new state, project word
	// search and heuristic definitions. nil disables the all-files
	// picker, gr and gd.
	Search *search.Searcher
	// Changes signals that the source changed while the review runs;
	// nil means the review is not watched. A signal only shows a hint,
	// the view changes on R.
	Changes <-chan struct{}
	// Reload resolves the review subject again: for the change hint
	// and for R. nil disables reloading.
	Reload func() ([]diff.ReviewFile, error)
	// Interdiff resolves the changes since the snapshot of the
	// previous round, shown instead of the whole review on <leader>i
	// and marking the files changed since then in the drawer. nil
	// means there is no previous round.
	Interdiff func() ([]diff.ReviewFile, error)
	// Snapshot records the reviewed state again after R, so the
	// snapshot of the round is what the reviewer saw; nil disables it.
	Snapshot func() error
	// StartScreen shows the review overview on startup (branch, base,
	// round, size and directory distribution) until the first key
	// press. Off by default; the command enables it.
	StartScreen bool
	// Icons enables Nerd Font icons in the file list. Off by default;
	// without a Nerd Font installed the list stays clean.
	Icons bool
	// Animate turns the smooth scroll of half-page moves and jumps and
	// the slide-in of the overlays on; nil means off. The command
	// enables it unless the config turns it off.
	Animate *bool
}

// mode is what key presses go to.
type mode int

const (
	modeNormal  mode = iota // the review view
	modeHelp                // the ? help overlay
	modeCmdline             // the : command line
	modeDrawer              // the file drawer
	modePicker              // the fuzzy file picker
	modeSearch              // the / search line
	modeResults             // the gr/gd result list
	modeComment             // the comment float
)

// Model is the review view: one file at a time, with folds for the
// unchanged runs of the full-file view and a toggleable unified view.
// label names the review subject and appears in the hint when there is
// nothing to review. theme provides the semantic styles and the syntax
// highlighter.
type Model struct {
	files     []diff.ReviewFile
	label     string
	theme     theme.Theme
	opts      Options
	width     int
	height    int
	fileIdx   int
	cursor    int    // index of the cursor row in rows()
	offset    int    // index of the first visible row
	foldsOpen []bool // per fold of the current file: open?
	unified   bool   // show the classic unified diff instead of the full file

	mode       mode
	keys       keymap.Dispatcher // the key sequence typed so far
	keyGen     int               // counts key presses, to drop stale which-key timers
	whichKey   bool              // the which-key popup is shown
	helpOffset int               // first visible line of the help
	cmdline    string            // text of the command line
	message    string            // command result, shown as a toast until the next key or its expiry
	messageErr bool              // message is an error
	messageAt  time.Time         // when the toast was shown, for its decay

	// state holds the read status per file; dir is where it is saved,
	// "" disables saving.
	state *review.State
	dir   string

	// tree is the file tree of the drawer and the sidebar, built once;
	// drawerCursor is the selected row while the drawer is open.
	tree         []treeRow
	drawerCursor int
	sidebar      bool // the permanent sidebar is shown
	// sidebarW is sidebarWidth for one frame, 0 when unknown. View sets
	// it on its own copy of the model: every row asks for the content
	// width, and measuring the tree each time cost a third of a frame.
	sidebarW int

	// pickerQuery is the fuzzy query, pickerMatches the matching file
	// indexes and pickerSel the selected match. pickerAll tells whether
	// the picker runs over all repo files (pickerFiles) instead of the
	// review files.
	pickerQuery   string
	pickerMatches []int
	pickerSel     int
	pickerAll     bool
	pickerFiles   []string

	// pathIndex maps the paths of the review files to their index, so
	// context jumps can tell review files from files beyond it.
	pathIndex map[string]int

	// context is the file beyond the review that is currently open
	// read-only; nil shows the review. It replaces the review file in
	// the view until the reviewer jumps away.
	context *contextFile

	// jumps is the jumplist across files and positions, jumpPos the
	// current entry. A jump appends the destination and drops the
	// forward history; <C-o>/<C-i> move within the list.
	jumps   []jumpEntry
	jumpPos int

	// col is the cursor column as a byte offset into the raw line
	// text, hscroll the horizontal scroll offset in cells.
	col     int
	hscroll int

	// wrap soft-wraps long lines instead of truncating them, ignoreWS
	// hides whitespace-only changes; both are :set options. fileCollapsed
	// remembers per file whether its whole content is collapsed (deleted
	// and generated files start collapsed).
	wrap          bool
	minimap       bool // the minimap strip is wanted; minimapOn also needs room
	ignoreWS      bool
	fileCollapsed []bool

	// searchQuery is the search pattern, searchMatches the matching
	// lines of the current view and searchIdx the current match.
	searchQuery   string
	searchMatches []int
	searchIdx     int

	// results is the open gr/gd result list, resultSel the selected
	// hit and resultsTitle the list title.
	results      []search.Hit
	resultSel    int
	resultsTitle string

	// visual is the line selection of V, started at visualLine (a line
	// index of the current file).
	visual     bool
	visualLine int

	// quitArmed remembers that the quit warning was shown; the next q
	// quits. Any other action disarms it.
	quitArmed bool
	// deleteArmed remembers that the delete confirmation was shown; the
	// next dd deletes. Any other action disarms it.
	deleteArmed bool
	// quitOpen reports that open comments remained when the review
	// ended; it becomes the exit code 1.
	quitOpen bool
	// approved is true after :lgtm; the review ends with exit code 0.
	approved bool

	// draft is the comment being written while the float is open;
	// fileComments caches the comments of the current file,
	// globalComments the comments on the whole review.
	draft          draft
	fileComments   []review.Comment
	globalComments []review.Comment

	// endVisible tells whether the end of the current file was visible
	// after the last update; the read status is only set when it
	// becomes visible, so the manual toggle is not undone by itself.
	endVisible bool

	// hl are the syntax highlights of the current file, computed lazily
	// for the visible area plus buffer: the new-file lines of the token
	// stream and the dimmed deleted lines.
	hl *fileHighlights

	// rowsCache is the cached row list of rows(), rebuilt when the file
	// view state changes (folds, mode, comments, files). rows() runs
	// several times per keystroke and once per frame, so a 10k-line
	// file must not be rebuilt every time. It lives behind a pointer so
	// View, which is called on a copy, can refill it.
	rowsCache *rowsCache

	// changed is the number of files that changed in the source since
	// the files were loaded; the status line hints at R. loadGen counts
	// the reloads, so checks against older files are dropped.
	changed int
	loadGen int

	// full are the files of the whole review, inter the files changed
	// since the previous round (nil without one); files is one of
	// them, inter while interdiff is on. sinceRound holds the paths of
	// inter for the drawer marker.
	full       []diff.ReviewFile
	inter      []diff.ReviewFile
	interdiff  bool
	sinceRound map[string]bool

	// resuming tells that the position where the review was left is
	// still to be restored; that needs the terminal size.
	resuming bool

	// startScreen shows the review overview until the first key press.
	startScreen bool

	// smooth is the active smooth scroll (nil when idle), slide the
	// active overlay slide-in; both tick on frameMsg.
	smooth *scrollTween
	slide  *slideTween

	// flash is the highlight of freshly changed lines after a reload,
	// fade the cross-fade of :theme, shimmer the frame counter of the
	// approval moment; all tick on frameMsg.
	flash   *flashTween
	fade    *themeFade
	shimmer int
}

// rowsCache is the cached row list of rows(), plus what the frame
// derives from the current file and its rows: each of them walks the
// whole file, so they are computed once, not once per frame.
type rowsCache struct {
	rows []row

	statsKnown     bool
	added, deleted int // Stats() of the current file
	newKnown       bool
	isNew          bool  // Added() of the current file
	marked         []int // minimapMarked of rows, for the strip height
	frameLen       int   // bytes of the last frame of fileView
}

// invalidateRows drops the cached row list; call it whenever the file
// view state changes: files, mode, folds, comments.
func (m *Model) invalidateRows() {
	if m.rowsCache != nil {
		*m.rowsCache = rowsCache{}
	}
}

// fileStats is Stats() of the current file, from the row cache.
func (m Model) fileStats() (added, deleted int) {
	c := m.rowsCache
	if c == nil {
		return m.files[m.fileIdx].Stats()
	}
	if !c.statsKnown {
		c.added, c.deleted = m.files[m.fileIdx].Stats()
		c.statsKnown = true
	}
	return c.added, c.deleted
}

// whichKeyMsg fires when a prefix has waited long enough for which-key.
// gen is the key press it belongs to.
type whichKeyMsg struct{ gen int }

// approveMsg ends the program after the :lgtm confirmation moment.
type approveMsg struct{}

// toastTickMsg checks whether the toast has decayed.
type toastTickMsg struct{}

// New returns a Model showing files of the review subject label with the
// given theme.
func New(files []diff.ReviewFile, label string, th theme.Theme, opts Options) Model {
	if opts.Keymap == nil {
		km, err := Keymap(keymap.Overrides{})
		if err != nil {
			panic(fmt.Sprintf("default keymap: %v", err))
		}
		opts.Keymap = km
	}
	if opts.WhichKeyDelay == 0 {
		opts.WhichKeyDelay = defaultWhichKeyDelay
	}
	if opts.State == nil {
		opts.State = review.NewState()
	}
	m := Model{
		files:       files,
		label:       label,
		theme:       th,
		opts:        opts,
		keys:        opts.Keymap.Dispatcher(),
		state:       opts.State,
		dir:         opts.Dir,
		rowsCache:   &rowsCache{},
		minimap:     true,
		startScreen: opts.StartScreen,
	}
	m.full = files
	m.loadInterdiff()
	m.setFiles(files, nil)
	if len(files) > 0 {
		m.jumps = []jumpEntry{{file: 0}}
	}
	m.resetFile()
	m.resuming = true
	return m
}

// setFiles makes files the files of the review and builds what depends
// on the file list: the path index, the collapse state per file, the
// drawer tree and the picker. collapsed keeps the collapse state of
// files by path; other files start in their default.
func (m *Model) setFiles(files []diff.ReviewFile, collapsed map[string]bool) {
	m.files = files
	m.invalidateRows()
	paths := make([]string, len(files))
	m.pathIndex = make(map[string]int, len(files))
	m.fileCollapsed = make([]bool, len(files))
	for i, f := range files {
		paths[i] = f.Path
		m.pathIndex[f.Path] = i
		c, ok := collapsed[f.Path]
		if !ok {
			c = f.Collapsed()
		}
		m.fileCollapsed[i] = c
	}
	m.tree = fileTree(paths)
	m.drawerCursor = m.firstFileRow()
	m.pickerMatches = m.allFileIndexes()
}

// firstFileRow is the tree row of the first file, -1 if there is none.
func (m Model) firstFileRow() int {
	for i, r := range m.tree {
		if !r.dir {
			return i
		}
	}
	return -1
}

// allFileIndexes lists every file index, for the picker before any query.
func (m Model) allFileIndexes() []int {
	all := make([]int, len(m.files))
	for i := range m.files {
		all[i] = i
	}
	return all
}

// Init starts waiting for the watcher's change signals and the toast
// decay ticks.
func (m Model) Init() tea.Cmd { return tea.Batch(m.waitForChange(), toastTick()) }

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Comment rows wrap at the width.
		m.invalidateRows()
		m.finishAnims()
		if m.resuming {
			m.resuming = false
			m.resumePosition()
		}
		m.clamp()
		m.clampHelp()
		m.maybeMarkRead()
	case frameMsg:
		m.stepFrames()
		return m, m.animCmd()
	case whichKeyMsg:
		if msg.gen == m.keyGen && m.keys.InPrefix() {
			m.whichKey = true
			m.startSlideOverlay(m.bodyHeight())
			return m, m.animCmd()
		}
	case approveMsg:
		return m, tea.Quit
	case toastTickMsg:
		if m.message != "" && time.Since(m.messageAt) >= m.toastTTL() {
			m.message, m.messageErr = "", false
		}
		return m, toastTick()
	case changedMsg:
		return m, m.checkChanges()
	case checkedMsg:
		return m.takeCheck(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.savePosition()
			m.quitOpen = len(m.state.OpenComments()) > 0
			return m, tea.Quit
		}
		if m.startScreen {
			// The first key dismisses the overview and acts
			// normally.
			m.startScreen = false
		}
		m.finishAnims()
		m.keyGen++
		m.whichKey = false
		m.message, m.messageErr = "", false
		switch m.mode {
		case modeCmdline:
			return m.cmdlineKey(msg)
		case modeDrawer:
			m = m.drawerKey(msg)
			m.maybeMarkRead()
			return m, nil
		case modePicker:
			m, cmd := m.pickerKey(msg)
			m.maybeMarkRead()
			return m, cmd
		case modeSearch:
			return m.searchKey(msg), nil
		case modeResults:
			return m.resultsKey(msg), nil
		case modeComment:
			return m.commentKey(msg), nil
		}
		m, cmd := m.dispatch(msg.String())
		m.maybeMarkRead()
		return m, cmd
	}
	return m, nil
}

// dispatch feeds a key through the keymap and runs the bound action. A
// prefix starts the which-key timer.
func (m Model) dispatch(key string) (Model, tea.Cmd) {
	var r keymap.Result
	m.keys, r = m.keys.Press(key)
	switch r.Status {
	case keymap.Pending:
		if m.keys.InPrefix() {
			gen := m.keyGen
			return m, tea.Tick(m.opts.WhichKeyDelay, func(time.Time) tea.Msg {
				return whichKeyMsg{gen: gen}
			})
		}
	case keymap.Matched:
		if m.mode == modeHelp {
			return m.helpAction(r.Action, r.Count), nil
		}
		return m.do(r.Action, r.Count)
	case keymap.Unbound:
		switch {
		case m.mode == modeHelp && key == "esc":
			m.mode = modeNormal
		case key == "esc":
			m.visual = false
		}
	}
	return m, nil
}

// do runs an action of the review view. count is the count typed before
// the keys, 0 if none: motions repeat, line jumps go to line count.
func (m Model) do(a keymap.Action, count int) (Model, tea.Cmd) {
	// Any action except q cancels the quit warning, so the warning is
	// for the next q only; any action except dd cancels the delete
	// confirmation.
	if a != actQuit {
		m.quitArmed = false
	}
	if a != actDeleteComment {
		m.deleteArmed = false
	}
	switch a {
	case actQuit:
		return m, m.quitAction()
	case actReload:
		m.reload()
		return m, m.animCmd()
	case actInterdiff:
		m.toggleInterdiff()
		return m, nil
	case actCopyPrompt:
		m.copyPrompt()
		return m, nil
	case actCopyComment:
		m.copyComment()
		return m, nil
	case actHelp:
		m.mode, m.helpOffset = modeHelp, 0
		m.startSlideOverlay(m.bodyHeight())
		return m, m.animCmd()
	case actCmdline:
		m.mode, m.cmdline = modeCmdline, ""
		return m, nil
	case actThemeCmd:
		m.mode, m.cmdline = modeCmdline, "theme "
		return m, nil
	case actDrawer:
		if len(m.files) > 0 {
			m.openDrawer()
		}
		return m, m.animCmd()
	case actPicker:
		if len(m.files) > 0 {
			m.openPicker()
		}
		return m, m.animCmd()
	case actPickFileAll:
		m.openPickerAll()
		return m, m.animCmd()
	case actToggleRead:
		if m.context == nil && len(m.files) > 0 {
			m.toggleRead()
		}
		return m, nil
	case actSearch:
		m.openSearch()
		return m, nil
	case actSearchNext:
		m.searchJump(1)
		return m, nil
	case actSearchPrev:
		m.searchJump(-1)
		return m, nil
	case actSearchWordNext:
		m.searchWord(1)
		return m, nil
	case actSearchWordPrev:
		m.searchWord(-1)
		return m, nil
	case actGrep:
		m.grep()
		return m, nil
	case actGotoDef:
		m.gotoDef()
		return m, nil
	case actJumpBack:
		m.jumpHistory(-1)
		return m, nil
	case actJumpForward:
		m.jumpHistory(1)
		return m, nil
	case actLeft:
		m.moveCol(-1)
		return m, nil
	case actRight:
		m.moveCol(1)
		return m, nil
	case actScrollLeft:
		if !m.wrap {
			m.hscroll = max(0, m.hscroll-m.contentWidth()/2)
		}
		return m, nil
	case actScrollRight:
		if !m.wrap {
			m.hscroll += m.contentWidth() / 2
		}
		return m, nil
	}
	if len(m.files) == 0 && m.context == nil {
		return m, nil
	}
	if m.context != nil {
		return m.doContext(a, count), nil
	}
	n := max(1, count)
	lastLine := len(m.files[m.fileIdx].Lines) - 1
	switch a {
	case actDown:
		m.cursor = min(m.cursor+n, max(0, len(m.rows())-1))
	case actUp:
		m.cursor = max(m.cursor-n, 0)
	case actHalfDown:
		half := max(1, m.bodyHeight()/2)
		rows := m.rows()
		m.animate(func() {
			m.offset = min(m.offset+n*half, max(0, len(rows)-m.bodyHeight()))
			m.cursor = min(m.cursor+n*half, max(0, len(rows)-1))
		})
	case actHalfUp:
		half := max(1, m.bodyHeight()/2)
		m.animate(func() {
			m.offset = max(m.offset-n*half, 0)
			m.cursor = max(m.cursor-n*half, 0)
		})
	case actTop:
		m.animate(func() {
			if count > 0 {
				m.jumpToLine(min(count-1, lastLine))
			} else {
				m.jumpToLine(0)
			}
		})
	case actBottom:
		m.animate(func() {
			if count > 0 {
				m.jumpToLine(min(count-1, lastLine))
			} else {
				m.jumpToLine(lastLine)
			}
		})
	case actCenter:
		m.animate(func() {
			if count > 0 {
				m.cursor = m.rowOfLine(min(count-1, lastLine))
			}
			m.offset = m.cursor - m.bodyHeight()/2
		})
	case actCursorTop:
		m.animate(func() {
			if count > 0 {
				m.cursor = m.rowOfLine(min(count-1, lastLine))
			}
			m.offset = m.cursor
		})
	case actFoldOpen:
		if m.fileCollapsed[m.fileIdx] {
			m.fileCollapsed[m.fileIdx] = false
			m.invalidateRows()
			m.cursor = m.rowOfLine(0)
		} else if fi := m.foldAtCursor(); fi >= 0 && !m.foldsOpen[fi] {
			m.foldsOpen[fi] = true
			m.invalidateRows()
			m.cursor = m.rowOfLine(m.files[m.fileIdx].Folds[fi].Start)
		}
	case actFoldClose:
		if fi := m.foldAtCursor(); fi >= 0 && m.foldsOpen[fi] {
			m.foldsOpen[fi] = false
			m.invalidateRows()
			m.cursor = m.rowOfFold(fi)
		} else if m.files[m.fileIdx].Collapsed() && !m.fileCollapsed[m.fileIdx] && m.cursorLine() == 0 {
			// The whole file is one fold for deleted and generated
			// files: zc at the first line collapses it again.
			m.fileCollapsed[m.fileIdx] = true
			m.invalidateRows()
			m.cursor, m.offset = 0, 0
		}
	case actFoldsOpenAll, actFoldsCloseAll:
		old := m.cursorLine()
		if m.files[m.fileIdx].Collapsed() {
			m.fileCollapsed[m.fileIdx] = a == actFoldsCloseAll
		}
		for i := range m.foldsOpen {
			m.foldsOpen[i] = a == actFoldsOpenAll
		}
		m.invalidateRows()
		m.cursor = m.rowOfLine(old)
	case actNextHunk, actPrevHunk:
		dir := 1
		if a == actPrevHunk {
			dir = -1
		}
		m.animate(func() {
			for range n {
				m.jumpToHunk(dir)
			}
		})
	case actNextFile, actPrevFile:
		dir := 1
		if a == actPrevFile {
			dir = -1
		}
		for range n {
			if !m.jumpFile(dir) {
				break
			}
		}
	case actUnified:
		m.setUnified(!m.unified)
	case actMinimap:
		m.minimap = !m.minimap
	case actVisual:
		m.toggleVisual()
	case actComment:
		m.openComment()
	case actCommentFile:
		m.openCommentFile()
	case actCommentGlobal:
		m.openCommentGlobal()
	case actDeleteComment:
		m.deleteComment()
	case actMarkDone:
		m.markDone()
	case actNextComment, actPrevComment:
		dir := 1
		if a == actPrevComment {
			dir = -1
		}
		m.animate(func() {
			for range n {
				m.jumpToComment(dir)
			}
		})
	}
	if m.smooth == nil {
		m.scrollCursorIntoView()
	}
	return m, m.animCmd()
}

// quitAction ends the review: with open comments the first q only warns
// and the second q quits, with the exit code reporting the open
// comments.
func (m *Model) quitAction() tea.Cmd {
	n := len(m.state.OpenComments())
	if n > 0 && !m.quitArmed {
		m.quitArmed = true
		m.message = fmt.Sprintf("%d offene Kommentare – Y kopieren? Nochmal q beendet", n)
		return nil
	}
	m.quitOpen = n > 0
	m.savePosition()
	return tea.Quit
}

// ExitCode is the process exit code of the finished review: 0 when the
// review was approved with :lgtm or no open comments remained, 1 when
// open comments remained, so scripts and agents can react.
func (m Model) ExitCode() int {
	if m.approved {
		return 0
	}
	if m.quitOpen {
		return 1
	}
	return 0
}

// setUnified switches between the full-file and the unified view.
func (m *Model) setUnified(on bool) {
	if on == m.unified {
		return
	}
	m.unified = on
	m.invalidateRows()
	m.cursor, m.offset = 0, 0
	m.visual = false
}

// jumpToHunk moves the cursor to the next (dir 1) or previous (dir -1)
// hunk: the first hunk after the cursor line, or the last hunk before it,
// so a cursor inside a hunk lands on that hunk's start. In the full-file
// view hunks are the changed lines of the file, in the unified view the
// section headers.
func (m *Model) jumpToHunk(dir int) {
	if m.unified {
		line := m.cursorLine()
		hit := -1
		for _, s := range m.files[m.fileIdx].Unified(contextWindow) {
			if dir > 0 && s.Start > line {
				hit = s.Start
				break
			}
			if dir < 0 && s.Start < line {
				hit = s.Start // keep the last match for backward jumps
			}
		}
		if hit >= 0 {
			m.cursor = m.rowOfHeader(hit)
		}
		return
	}
	line := m.cursorLine()
	hit := -1
	for _, h := range m.files[m.fileIdx].Hunks {
		if dir > 0 && h.Start > line {
			hit = h.Start
			break
		}
		if dir < 0 && h.Start < line {
			hit = h.Start // keep the last match for backward jumps
		}
	}
	if hit >= 0 {
		m.cursor = m.rowOfLine(hit)
	}
}

// jumpFile moves to the next (dir 1) or previous (dir -1) file and reports
// whether the cursor moved.
func (m *Model) jumpFile(dir int) bool {
	next := m.fileIdx + dir
	if next < 0 || next >= len(m.files) {
		return false
	}
	m.jumpToFile(next, 0)
	return true
}

// jumpToLine moves the cursor to the file line and records the jump.
func (m *Model) jumpToLine(line int) {
	m.recordJump(jumpEntry{file: m.fileIdx, line: line})
	m.cursor = m.rowOfLine(line)
}

// rowOfHeader returns the row of the section header starting at the line,
// -1 if there is none.
func (m Model) rowOfHeader(line int) int {
	for i, r := range m.rows() {
		if r.header != "" && r.line == line {
			return i
		}
	}
	return -1
}

// resetFile resets the per-file view state for the current file.
func (m *Model) resetFile() {
	m.cursor, m.offset = 0, 0
	m.col = 0
	m.endVisible = false
	m.visual = false
	m.searchMatches = nil
	m.loadFileComments()
	m.invalidateRows()
	if len(m.files) == 0 {
		m.foldsOpen = nil
		m.hl = nil
		return
	}
	f := m.files[m.fileIdx]
	m.foldsOpen = make([]bool, len(f.Folds))
	m.hl = newFileHighlights(m.theme.Highlighter, f)
}

// clamp keeps cursor and offset inside the current rows.
func (m *Model) clamp() {
	if len(m.files) == 0 && m.context == nil {
		m.cursor, m.offset = 0, 0
		return
	}
	n := m.viewRows()
	m.cursor = min(m.cursor, max(0, n-1))
	m.offset = max(0, min(m.offset, max(0, n-m.bodyHeight())))
}

// scrollCursorIntoView scrolls the minimum amount so the cursor row is
// visible, and keeps the column inside the line.
func (m *Model) scrollCursorIntoView() {
	if len(m.files) == 0 && m.context == nil {
		return
	}
	n := m.viewRows()
	m.offset = max(0, min(m.offset, max(0, n-m.bodyHeight())))
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.bodyHeight() {
		m.offset = m.cursor - m.bodyHeight() + 1
	}
	m.clamp()
	m.col = min(m.col, len(m.viewLine(m.cursorViewLine())))
}

// row is one row of the view: a line of the file, a fold marker, a
// section header, a whole-file collapse marker, a meta info entry or a
// line of a comment.
type row struct {
	line      int    // index into the file's Lines; header rows carry the section start, comment rows their anchor line
	fold      int    // fold index of a marker row, -1 otherwise
	header    string // rendered section header of a header row, "" otherwise
	note      *note  // comment line of a comment row, nil otherwise
	collapsed bool   // whole-file collapse marker row of a deleted or generated file
	meta      string // rendered info entry of a binary, mode or submodule change
}

// rows are the rows of the current view mode for the current file:
// info entries for binary, mode and submodule changes, a single marker
// for collapsed files, otherwise the full-file or unified view. The
// result is cached until the file view state changes, because it is
// needed several times per keystroke and once per frame.
func (m Model) rows() []row {
	if m.rowsCache != nil && m.rowsCache.rows != nil {
		return m.rowsCache.rows
	}
	rows := m.buildRows()
	if m.rowsCache != nil {
		m.rowsCache.rows = rows
	}
	return rows
}

func (m Model) buildRows() []row {
	f := m.files[m.fileIdx]
	switch f.Kind {
	case diff.KindBinary, diff.KindMode, diff.KindSubmodule:
		return []row{{line: -1, fold: -1, meta: m.metaText(f)}}
	}
	if m.fileCollapsed[m.fileIdx] {
		return m.collapsedRows(f)
	}
	if m.unified {
		return m.unifiedRows(f)
	}
	return m.fullRows(f)
}

// fullRows are the lines of the full-file view; each closed fold collapses
// to one marker row.
func (m Model) fullRows(f diff.ReviewFile) []row {
	var rows []row
	i, fi := 0, 0
	for i < len(f.Lines) {
		if fi < len(f.Folds) && i == f.Folds[fi].Start {
			if m.foldsOpen[fi] {
				for ; i < f.Folds[fi].End; i++ {
					rows = append(rows, row{line: i, fold: -1})
					rows = append(rows, m.noteRows(m.fileIdx, i)...)
				}
			} else {
				// The comments inside a closed fold stay visible
				// below its marker.
				rows = append(rows, row{line: -1, fold: fi})
				for ; i < f.Folds[fi].End; i++ {
					rows = append(rows, m.noteRows(m.fileIdx, i)...)
				}
			}
			fi++
		} else {
			rows = append(rows, row{line: i, fold: -1})
			rows = append(rows, m.noteRows(m.fileIdx, i)...)
			i++
		}
	}
	return rows
}

// collapsedRows is the whole-file collapse of a deleted or generated
// file: one marker row, with the comments on the hidden lines below it.
func (m Model) collapsedRows(f diff.ReviewFile) []row {
	rows := []row{{line: -1, fold: -1, collapsed: true}}
	for i := range f.Lines {
		rows = append(rows, m.noteRows(m.fileIdx, i)...)
	}
	return rows
}

// metaText is the info entry of a binary, mode or submodule change:
// the sizes, the modes or the commit hashes.
func (m Model) metaText(f diff.ReviewFile) string {
	switch f.Kind {
	case diff.KindBinary:
		switch {
		case f.OldSize > 0 && f.NewSize > 0:
			return fmt.Sprintf("binär: %d → %d Bytes", f.OldSize, f.NewSize)
		case f.NewSize > 0:
			return fmt.Sprintf("binär: %d Bytes", f.NewSize)
		default:
			return fmt.Sprintf("binär: %d Bytes (gelöscht)", f.OldSize)
		}
	case diff.KindMode:
		return fmt.Sprintf("modus: %o → %o", f.OldMode, f.NewMode)
	case diff.KindSubmodule:
		switch {
		case f.OldSub != "" && f.NewSub != "":
			return fmt.Sprintf("submodul: %s → %s", shortHash(f.OldSub), shortHash(f.NewSub))
		case f.NewSub != "":
			return fmt.Sprintf("submodul: %s", shortHash(f.NewSub))
		default:
			return fmt.Sprintf("submodul: %s (gelöscht)", shortHash(f.OldSub))
		}
	}
	return ""
}

// shortHash shortens a commit hash for the info entry.
func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// unifiedRows are the hunks of the classic unified diff view: section
// headers plus the hunks expanded by the context window. The file and
// global comments sit at the top of the view.
func (m Model) unifiedRows(f diff.ReviewFile) []row {
	var rows []row
	rows = append(rows, m.fileNotes()...)
	for _, s := range f.Unified(contextWindow) {
		rows = append(rows, row{line: s.Start, fold: -1, header: s.Header()})
		for i := s.Start; i < s.End; i++ {
			rows = append(rows, row{line: i, fold: -1})
			rows = append(rows, m.noteRows(m.fileIdx, i)...)
		}
	}
	return rows
}

// cursorLine returns the file line the cursor is on: the fold's first
// line for marker rows, the section's first line for header rows, and
// the first line for info and collapse marker rows.
func (m Model) cursorLine() int {
	r := m.rows()[m.cursor]
	if r.fold >= 0 {
		return m.files[m.fileIdx].Folds[r.fold].Start
	}
	if r.line < 0 {
		return 0
	}
	return r.line
}

// foldAtCursor returns the fold the cursor points at: the fold of a marker
// row or the open fold whose lines the cursor is on. -1 if there is none.
func (m Model) foldAtCursor() int {
	r := m.rows()[m.cursor]
	if r.fold >= 0 {
		return r.fold
	}
	if r.line < 0 {
		return -1
	}
	if r.note != nil {
		// A comment row belongs to the fold of its line, open or
		// closed.
		return m.foldOfLine(r.line)
	}
	f := m.files[m.fileIdx]
	for fi, fold := range f.Folds {
		if m.foldsOpen[fi] && fold.Start <= r.line && r.line < fold.End {
			return fi
		}
	}
	return -1
}

// rowOfLine returns the row showing the file line: its line row, or the
// marker row of the closed fold containing it. In the unified view lines
// outside the sections are not shown; the cursor snaps to the next visible
// line instead.
func (m Model) rowOfLine(line int) int {
	rows := m.rows()
	for i, r := range rows {
		if r.line == line && r.note == nil {
			return i
		}
	}
	if fi := m.foldOfLine(line); fi >= 0 {
		if i := m.rowOfFold(fi); i >= 0 {
			return i
		}
	}
	for i, r := range rows {
		if r.line > line {
			return i
		}
	}
	return max(0, len(rows)-1)
}

// rowOfFold returns the marker row of the fold, -1 if it is open.
func (m Model) rowOfFold(fi int) int {
	for i, r := range m.rows() {
		if r.fold == fi {
			return i
		}
	}
	return -1
}

// foldOfLine returns the index of the fold containing the line, -1 if none.
func (m Model) foldOfLine(line int) int {
	for fi, fold := range m.files[m.fileIdx].Folds {
		if fold.Start <= line && line < fold.End {
			return fi
		}
	}
	return -1
}

func (m Model) View() tea.View {
	if m.sidebar {
		m.sidebarW = m.sidebarWidth()
	}
	var content string
	switch {
	case m.context != nil:
		content = m.contextView()
	case len(m.files) == 0:
		content = m.emptyView()
	default:
		content = m.fileView()
	}
	v := tea.NewView(m.withOverlay(content))
	v.AltScreen = true
	return v
}

// emptyView is the friendly hint shown instead of an empty review.
func (m Model) emptyView() string {
	hint := []string{"Nothing to review."}
	if m.label != "" {
		hint = append(hint, "", "No changes in "+m.label+".")
	}
	hint = append(hint, "", "Press q to quit.")
	if h := max(0, m.height); len(hint) > h {
		hint = hint[:h]
	}
	if bottom := m.bottomLine(); bottom != "" && m.height > 0 {
		for len(hint) < m.height {
			hint = append(hint, "")
		}
		hint[m.height-1] = bottom
	}
	lines := make([]string, len(hint))
	for i, line := range hint {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}

// fileView renders the file rows and the bottom line, with the sidebar
// beside them when it is on. With wrap on the rows are soft-wrapped
// instead of truncated. The minimap strip sits at the right edge of
// the file area.
func (m Model) fileView() string {
	width := m.contentWidth()
	var b strings.Builder
	// Frames of one file are about the same size; growing to the last
	// one spares the builder its doubling copies.
	if m.rowsCache != nil {
		b.Grow(m.rowsCache.frameLen)
	}
	rows := m.rows()
	m.hl.ensureRows(rows, m.offset-highlightBuffer, m.offset+m.bodyHeight()+highlightBuffer)
	marks := m.minimapMarks(rows)
	strip := m.minimapStrip(len(rows), m.cachedMinimapMarked(len(rows), marks), marks)
	if m.minimapOn() {
		width--
	}
	if m.wrap {
		m.renderWrapped(&b, width, len(rows), func(j int) (string, bool) {
			r := rows[j]
			hl := j == m.cursor || (r.note == nil && r.header == "" && m.selected(r.line))
			return m.renderRow(r, hl), hl
		}, strip)
	} else {
		for i := range m.bodyHeight() {
			if j := m.offset + i; j < len(rows) {
				r := rows[j]
				hl := j == m.cursor || (r.note == nil && r.header == "" && m.selected(r.line))
				row := ansi.Truncate(ansi.TruncateLeft(m.renderRow(r, hl), m.hscroll, ""), width, "")
				m.writePadded(&b, row, width, hl)
			} else {
				m.writePadded(&b, "", width, false)
			}
			b.WriteString(m.minimapCell(strip, i))
			b.WriteByte('\n')
		}
	}
	bottom := m.bottomLine()
	if bottom == "" {
		bottom = m.statusLine(width)
	}
	b.WriteString(m.footerRule())
	b.WriteString(ansi.Truncate(bottom, width, ""))
	if m.rowsCache != nil {
		m.rowsCache.frameLen = b.Len()
	}
	if !m.sidebar {
		return b.String()
	}

	// The sidebar box spans the file area; the bottom line spans the
	// whole terminal below it.
	side := strings.Split(m.sidebarBox(), "\n")
	content := strings.Split(b.String(), "\n")
	var out strings.Builder
	for i := 0; i < m.bodyHeight(); i++ {
		if i < len(side) {
			out.WriteString(side[i])
		}
		out.WriteByte(' ')
		if i < len(content) {
			out.WriteString(content[i])
		}
		out.WriteByte('\n')
	}
	if len(content) > m.bodyHeight() {
		// The rule and the bottom line span the whole terminal.
		out.WriteString(strings.Join(content[m.bodyHeight():], "\n"))
	}
	return out.String()
}

// writePadded writes the row and fills a highlighted row up to width
// with the cursor background, so the cursor reads as a bar and stays
// visible on short and empty lines. With the minimap on every row is filled up to width,
// with plain spaces when it is not highlighted, so the strip cell
// behind it sits in the last column.
func (m Model) writePadded(b *strings.Builder, row string, width int, hl bool) {
	b.WriteString(row)
	if !hl && !m.minimapOn() {
		return
	}
	pad := width - ansi.StringWidth(row)
	if pad <= 0 {
		return
	}
	if hl {
		b.WriteString(lipgloss.NewStyle().Background(m.theme.Styles.Cursor).Render(strings.Repeat(" ", pad)))
		return
	}
	for range pad {
		b.WriteByte(' ')
	}
}

// wrapLimit is how many cells of one line are rendered with wrap on: a
// screen height. Longer lines are cut with "…", so minified or
// generated content cannot flood the view.
func (m Model) wrapLimit() int {
	width := m.contentWidth()
	if m.minimapOn() {
		width--
	}
	return max(1, m.bodyHeight()*width-1)
}

// renderWrapped renders the rows with soft wrap: every row fills width
// cells and continues on the next line. A row longer than the wrap
// limit is cut with "…". cells are the minimap strip cells of the
// screen rows, nil without the strip.
func (m Model) renderWrapped(b *strings.Builder, width, rows int, render func(int) (string, bool), cells []string) {
	out := 0
	for j := m.offset; j < rows && out < m.bodyHeight(); j++ {
		text, hl := render(j)
		if ansi.StringWidth(text) > m.wrapLimit() {
			text = ansi.Truncate(text, m.wrapLimit(), "") + "…"
		}
		for _, piece := range strings.Split(ansi.Hardwrap(text, width, true), "\n") {
			if out >= m.bodyHeight() {
				break
			}
			m.writePadded(b, piece, width, hl)
			if cells != nil {
				b.WriteString(m.minimapCell(cells, out))
			}
			b.WriteByte('\n')
			out++
		}
	}
	// The strip runs down the whole file area, also beside the empty
	// rows below the last line.
	for ; cells != nil && m.minimapOn() && out < m.bodyHeight(); out++ {
		m.writePadded(b, "", width, false)
		b.WriteString(m.minimapCell(cells, out))
		b.WriteByte('\n')
	}
}

// contentWidth is the width of the file view: the terminal width, minus
// the sidebar when it is on.
func (m Model) contentWidth() int {
	if !m.sidebar {
		return m.width
	}
	return max(1, m.width-m.sidebarWidth()-1)
}

// statusLine shows file path, position in the file list, the +/−
// statistics, the review progress and the comment badges, plus the
// markers of the active options and the keys of a pending sequence. A
// rename names both paths as "old → new". On a narrow terminal the
// line gives way step by step: the progress label goes, then the
// review range and the progress bar, and finally the path is
// shortened in the middle, so the state markers stay readable.
func (m Model) statusLine(width int) string {
	for level := 0; level <= 3; level++ {
		if s := m.statusAt(level, -1); width <= 0 || ansi.StringWidth(s) <= width {
			return s
		}
	}
	// The rest of the line without the path is what the path may not
	// take away.
	rest := ansi.StringWidth(m.statusAt(3, 0))
	return m.statusAt(3, max(8, width-rest))
}

// statusAt renders the status line at a squeeze level: 0 is the full
// line, 1 drops the label of the progress bar, 2 the review range, 3
// the bar. pathW limits the width of the path, -1 leaves it alone.
func (m Model) statusAt(level, pathW int) string {
	f := m.files[m.fileIdx]
	added, deleted := m.fileStats()
	path := f.Path
	if f.OldPath != "" {
		path = f.OldPath + " → " + f.Path
	}
	if pathW >= 0 {
		path = shortenPath(path, pathW, f.OldPath != "")
	}
	// The review-wide elements come first, the file elements after.
	var head []string
	if m.opts.Round > 0 {
		head = append(head, fmt.Sprintf("Runde %d", m.opts.Round))
	}
	if m.interdiff {
		head = append(head, "[interdiff]")
	}
	head = append(head,
		path,
		fmt.Sprintf("%d/%d", m.fileIdx+1, len(m.files)),
		fmt.Sprintf("+%d −%d", added, deleted),
	)
	// The gradient head is followed by the review progress and the
	// comment badges, which carry their own colours, and the plain
	// rest of the markers.
	var out []string
	if level < 2 {
		if r := m.reviewRange(); r != "" {
			out = append(out, r)
		}
	}
	out = append(out, m.statusSegments(head))
	if p := m.progressBar(level == 0); p != "" && level < 3 {
		out = append(out, p)
	}
	if b := m.commentBadges(); b != "" {
		out = append(out, b)
	}
	var tail []string
	if n := m.orphanCount(); n > 0 {
		tail = append(tail, fmt.Sprintf("%d verwaist", n))
	}
	if m.unified {
		tail = append(tail, "[unified]")
	}
	if m.wrap {
		tail = append(tail, "[wrap]")
	}
	if m.ignoreWS {
		tail = append(tail, "[ignorews]")
	}
	if m.visual {
		tail = append(tail, "-- VISUAL --")
	}
	if typed := m.keys.Typed(); typed != "" {
		tail = append(tail, typed)
	}
	if hint := m.changeHint(); hint != "" {
		tail = append(tail, hint)
	}
	if len(tail) > 0 {
		out = append(out, m.theme.Styles.Status.Render(strings.Join(tail, "  ")))
	}
	return strings.Join(out, m.theme.Styles.Status.Render("  "))
}

// reviewRange names where the review runs from and to as "base → branch";
// with only one of the two known it shows just that one. Every name has
// its own colour gradient, picked from the theme by the name itself, so
// the same branch looks the same every time.
func (m Model) reviewRange() string {
	base, branch := m.opts.Base, m.opts.Branch
	if base == "" && branch == "" {
		return ""
	}
	st := m.theme.Styles
	var pal []color.Color
	for _, s := range []lipgloss.Style{st.Status, st.Purple, st.CommentMust, st.Orange, st.CommentQuestion, st.CommentNit} {
		if c := s.GetForeground(); !isUnset(c) {
			pal = append(pal, c)
		}
	}
	if len(pal) < 3 {
		return st.Status.Render(strings.TrimSpace(strings.Join([]string{base, "→", branch}, " ")))
	}
	n := len(pal)
	bi, ri := nameHue(base)%n, nameHue(branch)%n
	// Base and branch start at least two palette steps apart; the palette
	// is ordered by hue, so neighbours look alike.
	if d := (ri - bi + n) % n; d < 2 || d > n-2 {
		ri = (bi + 2) % n
	}
	colour := func(name string, i int) string {
		return gradientText(name, 0, false, []color.Color{pal[i], pal[(i+2)%n]})
	}
	var parts []string
	if base != "" {
		parts = append(parts, colour(base, bi))
	}
	parts = append(parts, st.Status.Render("→"))
	if branch != "" {
		parts = append(parts, colour(branch, ri))
	}
	return strings.Join(parts, st.Status.Render(" "))
}

// nameHue hashes a ref name to a stable number.
func nameHue(name string) int {
	h := fnv.New32a()
	h.Write([]byte(name))
	return int(h.Sum32() >> 1)
}

// statusSegments colours each element of the status line head in its
// own colour, spread over a gradient from the accent through the
// question blue to the cyan of the nit category. Without a colour for
// one of the stops the plain accent style stays.
func (m Model) statusSegments(segs []string) string {
	st := m.theme.Styles
	stops := []color.Color{st.Status.GetForeground(), st.CommentQuestion.GetForeground(), st.CommentNit.GetForeground()}
	for _, c := range stops {
		if isUnset(c) {
			return st.Status.Render(strings.Join(segs, "  "))
		}
	}
	cols := lipgloss.Blend1D(max(len(segs), 2), stops...)
	out := make([]string, len(segs))
	for i, seg := range segs {
		out[i] = lipgloss.NewStyle().Foreground(cols[i]).Render(seg)
	}
	return strings.Join(out, st.Status.Render("  "))
}

// renderRow renders one row of the current file, highlighting the cursor.
func (m Model) renderRow(r row, cursor bool) string {
	if r.note != nil {
		return m.renderNote(r.note, cursor)
	}
	if r.meta != "" {
		style := m.theme.Styles.Hunk
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		return style.Render(r.meta)
	}
	if r.collapsed {
		style := m.theme.Styles.Fold
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		return style.Render(collapseText(m.files[m.fileIdx]))
	}
	if r.header != "" {
		style := m.theme.Styles.Hunk
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		return style.Render(r.header)
	}
	if r.fold >= 0 {
		style := m.theme.Styles.Fold
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		return style.Render(foldText(m.files[m.fileIdx].Folds[r.fold].Count()))
	}
	return m.renderLine(m.files[m.fileIdx].Lines[r.line], r.line, cursor)
}

// collapseText is the marker of a collapsed file: the deleted lines of
// a deleted file, the diff of a generated file.
func collapseText(f diff.ReviewFile) string {
	n := len(f.Lines)
	if f.Deleted() {
		if n == 1 {
			return "⋯ 1 gelöschte Zeile ⋯"
		}
		return fmt.Sprintf("⋯ %d gelöschte Zeilen ⋯", n)
	}
	if n == 1 {
		return "⋯ generierte Datei: 1 Zeile ⋯"
	}
	return fmt.Sprintf("⋯ generierte Datei: %d Zeilen ⋯", n)
}

// foldText is the marker of a collapsed unchanged run.
func foldText(n int) string {
	if n == 1 {
		return "⋯ 1 unveränderte Zeile ⋯"
	}
	return fmt.Sprintf("⋯ %d unveränderte Zeilen ⋯", n)
}

// ruleOn reports whether a rule separates the file area from the
// status line; very small terminals keep the row for the code.
func (m Model) ruleOn() bool {
	return m.height >= 4
}

// bodyHeight is the number of file rows that fit above the status line
// and the rule that sets it off.
func (m Model) bodyHeight() int {
	if m.ruleOn() {
		return max(0, m.height-2)
	}
	return max(0, m.height-1)
}

// footerRule is the thin line above the status line, ended by a
// newline; empty when the terminal is too small for it.
func (m Model) footerRule() string {
	if !m.ruleOn() {
		return ""
	}
	return m.theme.Styles.Gutter.Render(strings.Repeat("─", max(0, m.width))) + "\n"
}

// fileAdded reports whether the current file is new, from the row
// cache: renderLine asks once per row.
func (m Model) fileAdded() bool {
	c := m.rowsCache
	if c == nil {
		return m.files[m.fileIdx].Added()
	}
	if !c.newKnown {
		c.isNew = m.files[m.fileIdx].Added()
		c.newKnown = true
	}
	return c.isNew
}

// renderLine renders one line of the current file: gutter, marker and the
// syntax-highlighted body. Added and context lines come from the file's
// highlight cache, deleted lines are the dimmed per-line highlight. The
// wash backgrounds of changed lines and the cursor background are applied
// on top of the token styles.
func (m Model) renderLine(l diff.Line, idx int, cursor bool) string {
	mark := -1
	if cursor && idx == m.cursorViewLine() && m.rows()[m.cursor].line == idx {
		mark = colMark(l.Text, m.col)
	}
	// Flashes and the theme fade change the colours from frame to frame.
	if m.hl == nil || m.flash != nil || m.fade != nil {
		return m.drawLine(l, idx, cursor, mark)
	}
	segs := m.segments(l, idx)
	if len(segs) == 0 {
		return m.drawLine(l, idx, cursor, mark)
	}
	// The window is only a cache key: the prefix of drawLine (line
	// numbers and marker) is fixed per line, so an approximation is
	// as unique as the exact width.
	skip, need := m.bodyWindow(12)
	key := lineKey{idx: idx, cursor: cursor, mark: mark, skip: skip, need: need, seg0: &segs[0]}
	if len(l.Words) > 0 {
		key.word0 = &l.Words[0]
	}
	if out, ok := m.hl.cachedLine(key, l); ok {
		return out
	}
	out := m.drawLine(l, idx, cursor, mark)
	m.hl.storeLine(key, l, out)
	return out
}

// drawLine renders one line of the current file without the cache.
func (m Model) drawLine(l diff.Line, idx int, cursor bool, mark int) string {
	st := m.theme.Styles
	gutter := st.Gutter
	if cursor {
		gutter = gutter.Background(st.Cursor).Foreground(st.Status.GetForeground()).Bold(true)
	}

	marker, markerStyle, wash := "   ", lipgloss.Style{}, color.Color(nil)
	switch l.Kind {
	case diff.Added:
		marker, markerStyle, wash = " + ", st.Added, st.AddedBG
	case diff.Deleted:
		marker, markerStyle, wash = " - ", st.Deleted, st.DeletedBG
	}
	// New files carry no wash: the whole file is added, so a flat
	// background would drown the syntax colours. The gutter markers
	// mark the lines.
	if wash != nil && m.fileAdded() {
		wash = nil
	}
	wash = m.flashWash(m.files[m.fileIdx].Path, l, wash)
	if wash != nil {
		markerStyle = markerStyle.Background(wash)
	}
	if cursor {
		markerStyle = markerStyle.Background(st.Cursor)
	}

	segs := washSegments(m.segments(l, idx), wash)
	if len(l.Words) > 0 {
		bg := st.WordAddedBG
		if l.Kind == diff.Deleted {
			bg = st.WordDeletedBG
		}
		segs = wordSegments(segs, l.Text, l.Words, bg)
	}

	num := fmt.Sprintf("%4s %4s", lineNum(l.Old), lineNum(l.New))
	var b strings.Builder
	m.renderBody(&b, segs, len(num)+len(marker), cursor, mark)
	return gutter.Render(num) + markerStyle.Render(marker) + b.String()
}

// bodyWindow is the horizontal window renderBody covers: the cells to
// skip and the cells needed after them.
func (m Model) bodyWindow(prefixCells int) (skip, need int) {
	skip = max(m.hscroll-prefixCells, 0)
	need = m.width
	if m.wrap {
		need = m.wrapLimit()
	}
	return skip, need
}

// renderBody writes the styled segments of a line body into b, covering
// only the horizontal window the terminal shows: whole segments left of
// the horizontal scroll are skipped and the render stops once the
// window is covered. The caller's ansi.Truncate and TruncateLeft cut
// the exact cell boundary afterwards, so the output is identical to
// rendering every segment, but minified lines only cost one window.
// prefixCells are the cells the body starts at (gutter and marker). mark
// is the expanded byte offset of the column cursor, or -1 for none: the
// character there is drawn in reverse video, or a blank cell at the end
// of the line. Splitting at render time keeps the cached segments
// untouched and avoids copying them for every frame.
func (m Model) renderBody(b *strings.Builder, segs []theme.Segment, prefixCells int, cursor bool, mark int) {
	skip, need := m.bodyWindow(prefixCells)
	end := skip + need
	acc, padded, pos := 0, false, 0
	var last lipgloss.Style
	emit := func(text string, style lipgloss.Style) {
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		b.WriteString(style.Render(text))
	}
	for _, s := range segs {
		start := pos
		pos += len(s.Text)
		last = s.Style
		w := ansi.StringWidth(s.Text)
		if acc+w <= skip {
			acc += w
			continue
		}
		if acc >= end {
			return
		}
		if acc > 0 && !padded {
			// Stand in for the skipped segments, so the cut at
			// the scroll offset lands on the right cell.
			b.WriteString(strings.Repeat(" ", acc))
		}
		padded = true
		if mark >= start && mark < pos {
			at := mark - start
			_, size := utf8.DecodeRuneInString(s.Text[at:])
			emit(s.Text[:at], s.Style)
			emit(s.Text[at:at+size], s.Style.Reverse(true))
			emit(s.Text[at+size:], s.Style)
		} else {
			emit(s.Text, s.Style)
		}
		acc += w
	}
	if mark >= pos && acc < end {
		if acc > 0 && !padded {
			b.WriteString(strings.Repeat(" ", acc))
		}
		emit(" ", last.Reverse(true))
	}
}

// washSegments applies the background wash of a changed line to the
// syntax segments, without touching the highlight cache the segments came
// from.
func washSegments(segs []theme.Segment, wash color.Color) []theme.Segment {
	if wash == nil {
		return segs
	}
	washed := make([]theme.Segment, len(segs))
	for i, s := range segs {
		washed[i] = theme.Segment{Text: s.Text, Style: s.Style.Background(wash)}
	}
	return washed
}

// colMark returns the offset in the tab-expanded text of the character at
// byte offset col of raw: the column cursor that go-to-definition and the
// word searches work from. The segments carry the expanded text, so col is
// translated first.
func colMark(raw string, col int) int {
	col = min(max(col, 0), len(raw))
	return col + 3*strings.Count(raw[:col], "\t")
}

// wordSegments splits the syntax segments of a line at the intra-line word
// range boundaries and applies the word highlight background to the
// changed pieces. The ranges are byte offsets into the raw line text; the
// segments carry the tab-expanded text, so the ranges are translated to
// expanded offsets first.
func wordSegments(segs []theme.Segment, raw string, words []diff.Word, bg color.Color) []theme.Segment {
	if len(words) == 0 {
		return segs
	}
	exp := make([]diff.Word, len(words))
	for i, w := range words {
		exp[i] = diff.Word{
			Start: w.Start + 3*strings.Count(raw[:w.Start], "\t"),
			End:   w.End + 3*strings.Count(raw[:w.End], "\t"),
		}
	}
	var out []theme.Segment
	pos := 0 // expanded offset of the current segment's start
	for _, s := range segs {
		start, end := pos, pos+len(s.Text)
		pos = end
		prev := start
		for _, w := range exp {
			if w.End <= start || w.Start >= end {
				continue
			}
			if w.Start > prev {
				out = append(out, theme.Segment{Text: s.Text[prev-start : w.Start-start], Style: s.Style})
				prev = w.Start
			}
			next := min(w.End, end)
			out = append(out, theme.Segment{Text: s.Text[prev-start : next-start], Style: s.Style.Background(bg)})
			prev = next
		}
		if prev < end {
			out = append(out, theme.Segment{Text: s.Text[prev-start:], Style: s.Style})
		}
	}
	return out
}

// segments returns the syntax segments of a line: from the lazy
// highlight cache for added and context lines, from the dimmed cache
// for deleted lines. Lines without a highlight fall back to plain text.
func (m Model) segments(l diff.Line, idx int) []theme.Segment {
	var segs []theme.Segment
	switch l.Kind {
	case diff.Added, diff.Context:
		if l.New > 0 {
			segs = m.hl.newLine(l.New)
		}
	case diff.Deleted:
		segs = m.hl.dimLine(idx)
	}
	if len(segs) == 0 && l.Text != "" {
		segs = []theme.Segment{{Text: strings.ReplaceAll(l.Text, "\t", "    ")}}
	}
	return segs
}

func lineNum(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

// shortenPath cuts a path to w cells in the middle, keeping the file
// name: "internal/ui/window.go" becomes "internal/…/window.go". A
// rename is cut at the end, because it has two names.
func shortenPath(path string, w int, rename bool) string {
	if ansi.StringWidth(path) <= w {
		return path
	}
	if rename || w < 4 {
		return ansi.Truncate(path, w, "…")
	}
	base := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		base = path[i+1:]
	}
	avail := w - ansi.StringWidth(base) - 2 // "…/"
	if avail < 1 {
		return "…" + ansi.TruncateLeft(base, ansi.StringWidth(base)-(w-1), "")
	}
	return ansi.Truncate(path, avail, "") + "…/" + base
}
