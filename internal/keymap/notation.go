package keymap

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// named maps vim key names (lower case, inside <...>) to Bubble Tea key
// strings.
var named = map[string]string{
	"space":     "space",
	"cr":        "enter",
	"enter":     "enter",
	"return":    "enter",
	"esc":       "esc",
	"tab":       "tab",
	"bs":        "backspace",
	"backspace": "backspace",
	"del":       "delete",
	"delete":    "delete",
	"up":        "up",
	"down":      "down",
	"left":      "left",
	"right":     "right",
	"home":      "home",
	"end":       "end",
	"pageup":    "pgup",
	"pgup":      "pgup",
	"pagedown":  "pgdown",
	"pgdown":    "pgdown",
	"lt":        "<",
}

// display maps Bubble Tea key strings back to their vim names.
var display = map[string]string{
	"space":     "space",
	"enter":     "cr",
	"esc":       "esc",
	"tab":       "tab",
	"backspace": "bs",
	"delete":    "del",
	"up":        "up",
	"down":      "down",
	"left":      "left",
	"right":     "right",
	"home":      "home",
	"end":       "end",
	"pgup":      "pageup",
	"pgdown":    "pagedown",
	"<":         "lt",
}

// modifiers maps vim modifier prefixes to Bubble Tea modifier prefixes, in
// Bubble Tea's canonical order.
var modifiers = []struct{ vim, tea string }{
	{"C-", "ctrl+"},
	{"A-", "alt+"},
	{"M-", "alt+"},
	{"S-", "shift+"},
}

// parseSeq turns a key sequence in vim notation into Bubble Tea key
// strings. <leader> expands to the leader key; with an empty leader (when
// parsing the leader itself) it is an error. A literal < is written <lt>.
func parseSeq(spec, leader string) ([]string, error) {
	var keys []string
	rest := spec
	for rest != "" {
		if rest[0] == '<' {
			end := strings.IndexByte(rest, '>')
			if end < 0 {
				return nil, fmt.Errorf("key %q: unterminated <", spec)
			}
			name := rest[1:end]
			rest = rest[end+1:]
			if strings.EqualFold(name, "leader") {
				if leader == "" {
					return nil, fmt.Errorf("key %q: <leader> cannot be used here", spec)
				}
				keys = append(keys, leader)
				continue
			}
			k, err := parseNamed(name)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", spec, err)
			}
			keys = append(keys, k)
			continue
		}
		r, size := utf8.DecodeRuneInString(rest)
		rest = rest[size:]
		if r == ' ' {
			keys = append(keys, "space")
		} else {
			keys = append(keys, string(r))
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty key sequence")
	}
	return keys, nil
}

// parseNamed parses the inside of <...>: an optional chain of modifiers
// (C-, A-, M-, S-) and a key name or a single character.
func parseNamed(name string) (string, error) {
	var mods []string
	for {
		found := false
		for _, m := range modifiers {
			if len(name) > len(m.vim) && strings.EqualFold(name[:len(m.vim)], m.vim) {
				mods = append(mods, m.tea)
				name = name[len(m.vim):]
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	key, ok := named[strings.ToLower(name)]
	if !ok {
		if utf8.RuneCountInString(name) != 1 {
			return "", fmt.Errorf("unknown key <%s>", name)
		}
		key = name
		if len(mods) > 0 {
			key = strings.ToLower(key)
		}
	}
	return canonicalMods(mods) + key, nil
}

// canonicalMods joins modifier prefixes in Bubble Tea's order: ctrl, alt,
// shift.
func canonicalMods(mods []string) string {
	var sb strings.Builder
	for _, want := range []string{"ctrl+", "alt+", "shift+"} {
		for _, m := range mods {
			if m == want {
				sb.WriteString(want)
				break
			}
		}
	}
	return sb.String()
}

// Format renders a Bubble Tea key string in vim notation, e.g. "ctrl+d"
// as "<C-d>" and "space" as "<space>".
func Format(key string) string {
	var mods strings.Builder
	rest := key
	for _, m := range []struct{ tea, vim string }{{"ctrl+", "C-"}, {"alt+", "A-"}, {"shift+", "S-"}} {
		if after, ok := strings.CutPrefix(rest, m.tea); ok && after != "" {
			mods.WriteString(m.vim)
			rest = after
		}
	}
	if name, ok := display[rest]; ok {
		return "<" + mods.String() + name + ">"
	}
	if mods.Len() > 0 {
		return "<" + mods.String() + rest + ">"
	}
	return rest
}

// FormatSeq renders a key sequence in vim notation.
func FormatSeq(keys []string) string {
	var sb strings.Builder
	for _, k := range keys {
		sb.WriteString(Format(k))
	}
	return sb.String()
}
