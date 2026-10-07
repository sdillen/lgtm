// Package keymap resolves key presses to actions. All bindings live in one
// prefix trie: single keys ("j"), sequences ("gg", "]c") and leader
// sequences ("<leader>u"), each optionally preceded by a count ("5j",
// "3]c"). Config overrides rebind actions and the leader key. The same
// trie drives the dispatch, the which-key popup and the help.
//
// Keys are written in vim notation ("j", "G", "<C-d>", "<space>",
// "<leader>u") in bindings and config, and arrive as Bubble Tea key
// strings ("j", "G", "ctrl+d", "space") at the dispatcher.
package keymap

import (
	"fmt"
	"slices"
	"strings"
)

// Action names what a binding does, e.g. "down" or "next-hunk".
type Action string

// Binding binds an action to its default key sequences.
type Binding struct {
	Action Action
	// Keys are the key sequences in vim notation, e.g. "j", "<C-d>",
	// "]c" or "<leader>u".
	Keys []string
	// Desc describes the action in which-key and the help.
	Desc string
}

// node is one node of the trie: a key sequence leading either to an action
// (leaf) or to further keys (prefix).
type node struct {
	action   Action
	desc     string
	children map[string]*node
	order    []string // child keys in binding order, for stable listings
}

// Keymap is the resolved trie of all bindings. It is immutable.
type Keymap struct {
	root *node
	help []HelpEntry
}

// Group names a key prefix for which-key, e.g. "]" as "next".
type Group struct {
	// Keys is the prefix in vim notation, e.g. "]" or "<leader>".
	Keys string
	Desc string
}

// Overrides are the user's changes to the default bindings.
type Overrides struct {
	// Leader is the leader key in vim notation, e.g. "," or "<space>".
	// Empty means the default <space>.
	Leader string
	// Keys rebinds actions: each listed action gets exactly these key
	// sequences instead of its defaults; an empty list unbinds it. A
	// rebound sequence wins over default bindings it collides with.
	Keys map[Action][]string
}

// New builds the keymap from the default bindings and the overrides. It
// fails on overrides for unknown actions, malformed keys, sequences that
// start with a count digit and overrides that collide with each other.
func New(defaults []Binding, groups []Group, ov Overrides) (*Keymap, error) {
	leader, err := parseLeader(ov.Leader)
	if err != nil {
		return nil, err
	}
	known := make(map[Action]bool, len(defaults))
	for _, b := range defaults {
		known[b.Action] = true
	}
	for a := range ov.Keys {
		if !known[a] {
			return nil, fmt.Errorf("keys: unknown action %q", a)
		}
	}

	km := &Keymap{root: &node{}}
	// The user's bindings go in first so that they win over colliding
	// defaults. Walking the defaults keeps the order deterministic.
	for _, b := range defaults {
		specs, ok := ov.Keys[b.Action]
		if !ok {
			continue
		}
		for _, spec := range specs {
			keys, err := parseBinding(spec, leader)
			if err != nil {
				return nil, fmt.Errorf("keys: %s: %w", b.Action, err)
			}
			if other, clash := km.root.collides(keys); clash {
				return nil, fmt.Errorf("keys: %s: %q collides with %s", b.Action, spec, other)
			}
			km.root.insert(keys, b)
		}
	}
	for _, b := range defaults {
		if _, ok := ov.Keys[b.Action]; ok {
			continue
		}
		for _, spec := range b.Keys {
			keys, err := parseBinding(spec, leader)
			if err != nil {
				return nil, fmt.Errorf("binding %s: %w", b.Action, err)
			}
			if other, clash := km.root.collides(keys); clash {
				if _, rebound := ov.Keys[other]; rebound {
					continue // shadowed by the user's binding
				}
				return nil, fmt.Errorf("binding %s: %q collides with %s", b.Action, spec, other)
			}
			km.root.insert(keys, b)
		}
	}
	km.nameGroups(groups, leader)
	km.collectHelp(defaults)
	return km, nil
}

// nameGroups attaches the group names to their prefix nodes. Groups whose
// keys are no prefix (any more) are ignored.
func (k *Keymap) nameGroups(groups []Group, leader string) {
	for _, g := range groups {
		keys, err := parseSeq(g.Keys, leader)
		if err != nil {
			continue
		}
		if n := k.root.find(keys); n != nil && n.action == "" {
			n.desc = g.Desc
		}
	}
}

// collectHelp lists every action in binding order with the sequences that
// reach it in the trie.
func (k *Keymap) collectHelp(defaults []Binding) {
	seqs := make(map[Action][]string)
	var walk func(n *node, path []string)
	walk = func(n *node, path []string) {
		if n.action != "" {
			seqs[n.action] = append(seqs[n.action], FormatSeq(path))
			return
		}
		for _, key := range n.order {
			walk(n.children[key], append(path[:len(path):len(path)], key))
		}
	}
	walk(k.root, nil)
	for _, b := range defaults {
		k.help = append(k.help, HelpEntry{Action: b.Action, Desc: b.Desc, Keys: seqs[b.Action]})
	}
}

// HelpEntry is one action of the help with all its key sequences in vim
// notation, the leader expanded.
type HelpEntry struct {
	Action Action
	Desc   string
	Keys   []string
}

// Help lists all actions in binding order with their current keys.
// Unbound actions have no keys.
func (k *Keymap) Help() []HelpEntry {
	return k.help
}

// find returns the node of the sequence, nil if there is none.
func (n *node) find(keys []string) *node {
	at := n
	for _, key := range keys {
		at = at.children[key]
		if at == nil {
			return nil
		}
	}
	return at
}

// parseLeader parses the leader key; it must be a single key that cannot
// start a count.
func parseLeader(spec string) (string, error) {
	if spec == "" {
		return "space", nil
	}
	keys, err := parseSeq(spec, "")
	if err != nil {
		return "", fmt.Errorf("leader: %w", err)
	}
	if len(keys) != 1 || isCountDigit(keys[0], true) {
		return "", fmt.Errorf("leader: %q is not a single non-digit key", spec)
	}
	return keys[0], nil
}

// parseBinding parses a bound key sequence. It must not start with a count
// digit, or the binding could never be reached.
func parseBinding(spec, leader string) ([]string, error) {
	keys, err := parseSeq(spec, leader)
	if err != nil {
		return nil, err
	}
	if isCountDigit(keys[0], false) {
		return nil, fmt.Errorf("%q starts with a count digit", spec)
	}
	return keys, nil
}

// collides reports the action of a binding that the sequence collides
// with: the same sequence, a binding that is a prefix of it, or a binding
// it is a prefix of.
func (n *node) collides(keys []string) (Action, bool) {
	at := n
	for _, k := range keys {
		c, ok := at.children[k]
		if !ok {
			return "", false
		}
		if c.action != "" {
			return c.action, true
		}
		at = c
	}
	return at.firstAction(), true
}

// firstAction returns the first action bound below the node.
func (n *node) firstAction() Action {
	if n.action != "" {
		return n.action
	}
	for _, k := range n.order {
		if a := n.children[k].firstAction(); a != "" {
			return a
		}
	}
	return ""
}

// insert binds the sequence to the binding's action.
func (n *node) insert(keys []string, b Binding) {
	at := n
	for _, k := range keys {
		at = at.child(k)
	}
	at.action, at.desc = b.Action, b.Desc
}

func (n *node) child(key string) *node {
	if n.children == nil {
		n.children = make(map[string]*node)
	}
	c, ok := n.children[key]
	if !ok {
		c = &node{}
		n.children[key] = c
		n.order = append(n.order, key)
	}
	return c
}

// Status tells what a key press did.
type Status int

const (
	// Unbound means the keys bound nothing; the sequence was dropped.
	Unbound Status = iota
	// Pending means the keys so far are a count or a prefix of a binding.
	Pending
	// Matched means the keys completed a binding.
	Matched
)

// Result is the outcome of one key press.
type Result struct {
	Status Status
	// Action is the matched action.
	Action Action
	// Count is the count typed before the sequence, 0 if none.
	Count int
}

// Dispatcher feeds key presses through the trie. It is a value: Press
// returns the next state.
type Dispatcher struct {
	km    *Keymap
	node  *node    // current trie node; nil at the start of a sequence
	count int      // count typed before the sequence, 0 if none
	typed []string // keys typed so far, count digits included
}

// Dispatcher returns a dispatcher at the start of a key sequence.
func (k *Keymap) Dispatcher() Dispatcher {
	return Dispatcher{km: k}
}

// Press feeds one key (a Bubble Tea key string) and returns the next state
// and the result.
func (d Dispatcher) Press(key string) (Dispatcher, Result) {
	if d.node == nil && isCountDigit(key, d.count > 0) {
		d.count = min(d.count*10+int(key[0]-'0'), maxCount)
		d.typed = append(d.typed[:len(d.typed):len(d.typed)], key)
		return d, Result{Status: Pending}
	}
	at := d.node
	if at == nil {
		at = d.km.root
	}
	start := Dispatcher{km: d.km}
	c, ok := at.children[key]
	switch {
	case !ok:
		return start, Result{Status: Unbound}
	case c.action != "":
		return start, Result{Status: Matched, Action: c.action, Count: d.count}
	}
	d.node = c
	d.typed = append(d.typed[:len(d.typed):len(d.typed)], key)
	return d, Result{Status: Pending}
}

// maxCount caps counts so that runaway digits cannot overflow.
const maxCount = 99999

// InPrefix reports whether the typed keys are a proper prefix of bindings,
// i.e. whether which-key has something to show. A bare count is not.
func (d Dispatcher) InPrefix() bool {
	return d.node != nil
}

// Typed returns the keys typed so far in vim notation, count included,
// e.g. "3]".
func (d Dispatcher) Typed() string {
	return FormatSeq(d.typed)
}

// Group returns the name of the current prefix, "" if it has none.
func (d Dispatcher) Group() string {
	if d.node == nil {
		return ""
	}
	return d.node.desc
}

// Entry is one possible follow-up key in which-key.
type Entry struct {
	// Key is the key in vim notation.
	Key string
	// Desc describes the bound action or names the group.
	Desc string
	// Group marks a key that leads to further keys.
	Group bool
}

// Next lists the keys that can follow the typed prefix, sorted by key
// (case-insensitive, lower case first).
func (d Dispatcher) Next() []Entry {
	if d.node == nil {
		return nil
	}
	entries := make([]Entry, 0, len(d.node.order))
	for _, key := range d.node.order {
		c := d.node.children[key]
		entries = append(entries, Entry{Key: Format(key), Desc: c.desc, Group: c.action == ""})
	}
	slices.SortFunc(entries, func(a, b Entry) int {
		if c := strings.Compare(strings.ToLower(a.Key), strings.ToLower(b.Key)); c != 0 {
			return c
		}
		return -strings.Compare(a.Key, b.Key)
	})
	return entries
}

// isCountDigit reports whether the key extends a count: 1-9 start one, 0
// only continues one, like in vim.
func isCountDigit(key string, counting bool) bool {
	if len(key) != 1 {
		return false
	}
	return key[0] >= '1' && key[0] <= '9' || counting && key[0] == '0'
}
