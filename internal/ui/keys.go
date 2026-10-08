package ui

import "gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"

// The actions of the review view. Their names are the keys of the [keys]
// table in the config.
const (
	actDown           keymap.Action = "down"
	actUp             keymap.Action = "up"
	actLeft           keymap.Action = "left"
	actRight          keymap.Action = "right"
	actScrollLeft     keymap.Action = "scroll-left"
	actScrollRight    keymap.Action = "scroll-right"
	actHalfDown       keymap.Action = "half-page-down"
	actHalfUp         keymap.Action = "half-page-up"
	actTop            keymap.Action = "first-line"
	actBottom         keymap.Action = "last-line"
	actCenter         keymap.Action = "center-cursor"
	actCursorTop      keymap.Action = "cursor-to-top"
	actFoldOpen       keymap.Action = "open-fold"
	actFoldClose      keymap.Action = "close-fold"
	actFoldsOpenAll   keymap.Action = "open-all-folds"
	actFoldsCloseAll  keymap.Action = "close-all-folds"
	actNextHunk       keymap.Action = "next-hunk"
	actPrevHunk       keymap.Action = "prev-hunk"
	actNextFile       keymap.Action = "next-file"
	actPrevFile       keymap.Action = "prev-file"
	actNextComment    keymap.Action = "next-comment"
	actPrevComment    keymap.Action = "prev-comment"
	actSearch         keymap.Action = "search"
	actSearchNext     keymap.Action = "search-next"
	actSearchPrev     keymap.Action = "search-prev"
	actSearchWordNext keymap.Action = "search-word-next"
	actSearchWordPrev keymap.Action = "search-word-prev"
	actGrep           keymap.Action = "search-repo"
	actGotoDef        keymap.Action = "go-to-definition"
	actJumpBack       keymap.Action = "jump-back"
	actJumpForward    keymap.Action = "jump-forward"
	actUnified        keymap.Action = "toggle-unified"
	actMinimap        keymap.Action = "toggle-minimap"
	actDrawer         keymap.Action = "toggle-file-drawer"
	actPicker         keymap.Action = "pick-file"
	actPickFileAll    keymap.Action = "pick-any-file"
	actToggleRead     keymap.Action = "toggle-read"
	actVisual         keymap.Action = "select-lines"
	actComment        keymap.Action = "comment"
	actCommentFile    keymap.Action = "comment-file"
	actCommentGlobal  keymap.Action = "comment-global"
	actDeleteComment  keymap.Action = "delete-comment"
	actMarkDone       keymap.Action = "mark-done"
	actCopyPrompt     keymap.Action = "copy-prompt"
	actCopyComment    keymap.Action = "copy-comment"
	actThemeCmd       keymap.Action = "choose-theme"
	actHelp           keymap.Action = "help"
	actCmdline        keymap.Action = "command-line"
	actReload         keymap.Action = "reload"
	actInterdiff      keymap.Action = "toggle-interdiff"
	actQuit           keymap.Action = "quit"
)

// bindings are the default key bindings, in help order.
var bindings = []keymap.Binding{
	{Action: actDown, Keys: []string{"j", "<down>"}, Desc: "line down"},
	{Action: actUp, Keys: []string{"k", "<up>"}, Desc: "line up"},
	{Action: actLeft, Keys: []string{"h"}, Desc: "left"},
	{Action: actRight, Keys: []string{"l"}, Desc: "right"},
	{Action: actScrollLeft, Keys: []string{"zh"}, Desc: "scroll left"},
	{Action: actScrollRight, Keys: []string{"zl"}, Desc: "scroll right"},
	{Action: actHalfDown, Keys: []string{"<C-d>"}, Desc: "half page down"},
	{Action: actHalfUp, Keys: []string{"<C-u>"}, Desc: "half page up"},
	{Action: actTop, Keys: []string{"gg"}, Desc: "first line (count: line N)"},
	{Action: actBottom, Keys: []string{"G"}, Desc: "last line (count: line N)"},
	{Action: actCenter, Keys: []string{"zz"}, Desc: "center the cursor line"},
	{Action: actCursorTop, Keys: []string{"zt"}, Desc: "cursor line to the top"},
	{Action: actFoldOpen, Keys: []string{"zo"}, Desc: "open fold"},
	{Action: actFoldClose, Keys: []string{"zc"}, Desc: "close fold"},
	{Action: actFoldsOpenAll, Keys: []string{"zR"}, Desc: "open all folds"},
	{Action: actFoldsCloseAll, Keys: []string{"zM"}, Desc: "close all folds"},
	{Action: actNextHunk, Keys: []string{"]c"}, Desc: "next hunk"},
	{Action: actPrevHunk, Keys: []string{"[c"}, Desc: "previous hunk"},
	{Action: actNextFile, Keys: []string{"]f"}, Desc: "next file"},
	{Action: actPrevFile, Keys: []string{"[f"}, Desc: "previous file"},
	{Action: actNextComment, Keys: []string{"]k"}, Desc: "next comment"},
	{Action: actPrevComment, Keys: []string{"[k"}, Desc: "previous comment"},
	{Action: actSearch, Keys: []string{"/"}, Desc: "search the file"},
	{Action: actSearchNext, Keys: []string{"n"}, Desc: "next match"},
	{Action: actSearchPrev, Keys: []string{"N"}, Desc: "previous match"},
	{Action: actSearchWordNext, Keys: []string{"*"}, Desc: "next word under the cursor"},
	{Action: actSearchWordPrev, Keys: []string{"#"}, Desc: "previous word under the cursor"},
	{Action: actGrep, Keys: []string{"gr"}, Desc: "search the word in the repo"},
	{Action: actGotoDef, Keys: []string{"gd"}, Desc: "go to the definition"},
	{Action: actJumpBack, Keys: []string{"<C-o>"}, Desc: "jump back"},
	// tab is bound alongside <C-i>: terminals send tab for ctrl+i.
	{Action: actJumpForward, Keys: []string{"<C-i>", "<tab>"}, Desc: "jump forward"},
	// u toggles the unified view. It deliberately is not vim's undo:
	// lgtm only reads, and the classic diff view is one key away.
	{Action: actUnified, Keys: []string{"u", "<leader>u"}, Desc: "toggle unified view"},
	{Action: actDrawer, Keys: []string{"<leader>d"}, Desc: "toggle file drawer"},
	{Action: actPicker, Keys: []string{"<leader>p"}, Desc: "pick a changed file"},
	{Action: actPickFileAll, Keys: []string{"<leader>f"}, Desc: "pick any file of the repo"},
	{Action: actToggleRead, Keys: []string{"<leader>r"}, Desc: "toggle read status"},
	{Action: actInterdiff, Keys: []string{"<leader>i"}, Desc: "toggle interdiff since the last round"},
	{Action: actMinimap, Keys: []string{"m"}, Desc: "toggle minimap"},
	{Action: actVisual, Keys: []string{"V"}, Desc: "select lines"},
	{Action: actComment, Keys: []string{"c"}, Desc: "comment the line or selection (edit on a comment)"},
	{Action: actCommentFile, Keys: []string{"C"}, Desc: "comment the whole file"},
	{Action: actCommentGlobal, Keys: []string{"<leader>c"}, Desc: "comment the whole review"},
	{Action: actDeleteComment, Keys: []string{"dd"}, Desc: "delete the comment (dd again confirms)"},
	{Action: actMarkDone, Keys: []string{"x"}, Desc: "mark the comment as done (again: open)"},
	{Action: actCopyPrompt, Keys: []string{"Y"}, Desc: "copy the review prompt to the clipboard"},
	{Action: actCopyComment, Keys: []string{"yc"}, Desc: "copy the comment under the cursor"},
	{Action: actThemeCmd, Keys: []string{"<leader>t"}, Desc: "choose theme"},
	{Action: actHelp, Keys: []string{"?"}, Desc: "help"},
	{Action: actCmdline, Keys: []string{":"}, Desc: "command line"},
	{Action: actReload, Keys: []string{"R"}, Desc: "reload the changed review"},
	{Action: actQuit, Keys: []string{"q"}, Desc: "quit"},
}

// groups name the prefixes in which-key.
var groups = []keymap.Group{
	{Keys: "<leader>", Desc: "leader"},
	{Keys: "]", Desc: "next"},
	{Keys: "[", Desc: "previous"},
	{Keys: "g", Desc: "go to"},
	{Keys: "z", Desc: "folds and scrolling"},
}

// Keymap builds the keymap of the review view: the default bindings with
// the user's overrides applied.
func Keymap(ov keymap.Overrides) (*keymap.Keymap, error) {
	return keymap.New(bindings, groups, ov)
}
