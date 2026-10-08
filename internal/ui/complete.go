package ui

import "strings"

// commandNames are the commands :execute knows, for Tab completion.
var commandNames = []string{"q", "q!", "quit", "quit!", "lgtm", "theme", "set"}

// optionNames are the :set options, for Tab completion.
var optionNames = []string{"unified", "sidebar", "minimap", "wrap", "ignorews"}

// completions returns the full command lines that Tab can turn line into:
// command names for the first word, then the arguments of :set and
// :theme (themes are the available theme names).
func completions(line string, themes []string) []string {
	head, word := "", line
	if i := strings.LastIndexByte(line, ' '); i >= 0 {
		head, word = line[:i+1], line[i+1:]
	}
	var candidates []string
	switch strings.TrimSpace(head) {
	case "":
		candidates = commandNames
	case "set":
		candidates = optionNames
		for _, prefix := range []string{"no", "inv"} {
			for _, name := range optionNames {
				candidates = append(candidates[:len(candidates):len(candidates)], prefix+name)
			}
		}
	case "theme":
		candidates = themes
	}
	var out []string
	for _, c := range candidates {
		if strings.HasPrefix(c, word) {
			out = append(out, head+c)
		}
	}
	return out
}
