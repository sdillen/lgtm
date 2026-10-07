package keymap_test

import (
	"reflect"
	"testing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
)

var defaults = []keymap.Binding{
	{Action: "down", Keys: []string{"j", "<down>"}, Desc: "line down"},
	{Action: "up", Keys: []string{"k"}, Desc: "line up"},
	{Action: "top", Keys: []string{"gg"}, Desc: "first line"},
	{Action: "next-hunk", Keys: []string{"]c"}, Desc: "next hunk"},
	{Action: "half-down", Keys: []string{"<C-d>"}, Desc: "half page down"},
	{Action: "unified", Keys: []string{"<leader>u"}, Desc: "toggle unified view"},
	{Action: "prev-file", Keys: []string{"<leader>fp"}, Desc: "previous file"},
}

var groups = []keymap.Group{
	{Keys: "]", Desc: "next"},
	{Keys: "<leader>", Desc: "leader"},
	{Keys: "<leader>f", Desc: "files"},
}

func mustNew(t *testing.T, ov keymap.Overrides) *keymap.Keymap {
	t.Helper()
	km, err := keymap.New(defaults, groups, ov)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return km
}

// feed presses the keys in order and returns the last result.
func feed(d keymap.Dispatcher, keys ...string) (keymap.Dispatcher, keymap.Result) {
	var r keymap.Result
	for _, k := range keys {
		d, r = d.Press(k)
	}
	return d, r
}

func TestSingleKeyDispatchesItsAction(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	_, r := feed(km.Dispatcher(), "j")

	if r != (keymap.Result{Status: keymap.Matched, Action: "down"}) {
		t.Errorf("j = %+v, want down", r)
	}
}

func TestPrefixKeyIsPendingUntilTheSequenceCompletes(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	d, r := feed(km.Dispatcher(), "]")
	if r.Status != keymap.Pending {
		t.Fatalf("] = %+v, want pending", r)
	}
	_, r = feed(d, "c")
	if r != (keymap.Result{Status: keymap.Matched, Action: "next-hunk"}) {
		t.Errorf("]c = %+v, want next-hunk", r)
	}
}

func TestDispatcherStartsOverAfterAMatch(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	_, r := feed(km.Dispatcher(), "g", "g", "g", "g")

	if r.Action != "top" {
		t.Errorf("gggg = %+v, want top", r)
	}
}

func TestUnboundKeyInsideASequenceDropsIt(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	d, r := feed(km.Dispatcher(), "]", "x")
	if r.Status != keymap.Unbound {
		t.Fatalf("]x = %+v, want unbound", r)
	}
	// The next key starts a fresh sequence.
	_, r = feed(d, "j")
	if r.Action != "down" {
		t.Errorf("j after ]x = %+v, want down", r)
	}
}

func TestSpecialKeysUseVimNotation(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	for key, want := range map[string]keymap.Action{
		"ctrl+d": "half-down",
		"down":   "down",
	} {
		if _, r := feed(km.Dispatcher(), key); r.Action != want {
			t.Errorf("%s = %+v, want %s", key, r, want)
		}
	}
}

func TestLeaderDefaultsToSpace(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	_, r := feed(km.Dispatcher(), "space", "u")

	if r.Action != "unified" {
		t.Errorf("<space>u = %+v, want unified", r)
	}
}

func TestCountBeforeAKeyIsPassedWithTheAction(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	_, r := feed(km.Dispatcher(), "1", "2", "j")

	if r != (keymap.Result{Status: keymap.Matched, Action: "down", Count: 12}) {
		t.Errorf("12j = %+v, want down with count 12", r)
	}
}

func TestCountBeforeASequence(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	d, r := feed(km.Dispatcher(), "3")
	if r.Status != keymap.Pending {
		t.Fatalf("3 = %+v, want pending", r)
	}
	_, r = feed(d, "]", "c")
	if r != (keymap.Result{Status: keymap.Matched, Action: "next-hunk", Count: 3}) {
		t.Errorf("3]c = %+v, want next-hunk with count 3", r)
	}
}

func TestZeroContinuesACountButDoesNotStartOne(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	if _, r := feed(km.Dispatcher(), "1", "0", "j"); r.Count != 10 {
		t.Errorf("10j = %+v, want count 10", r)
	}
	if _, r := feed(km.Dispatcher(), "0"); r.Status != keymap.Unbound {
		t.Errorf("0 = %+v, want unbound", r)
	}
}

func TestCountIsDroppedWithAnUnboundKey(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	_, r := feed(km.Dispatcher(), "5", "x", "j")

	if r != (keymap.Result{Status: keymap.Matched, Action: "down"}) {
		t.Errorf("5xj = %+v, want down without count", r)
	}
}

func TestOverrideReplacesTheDefaultKeysOfAnAction(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Keys: map[keymap.Action][]string{
		"top": {"<home>"},
	}})

	if _, r := feed(km.Dispatcher(), "home"); r.Action != "top" {
		t.Errorf("<home> = %+v, want top", r)
	}
	if _, r := feed(km.Dispatcher(), "g"); r.Status != keymap.Unbound {
		t.Errorf("g = %+v, want unbound after rebinding gg", r)
	}
}

func TestOverrideTakesAKeyAwayFromAnotherAction(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Keys: map[keymap.Action][]string{
		"up": {"j"},
	}})

	if _, r := feed(km.Dispatcher(), "j"); r.Action != "up" {
		t.Errorf("j = %+v, want up", r)
	}
	// down keeps its other key.
	if _, r := feed(km.Dispatcher(), "down"); r.Action != "down" {
		t.Errorf("<down> = %+v, want down", r)
	}
}

func TestOverrideShadowsDefaultsItIsAPrefixOf(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Keys: map[keymap.Action][]string{
		"unified": {"g"},
	}})

	if _, r := feed(km.Dispatcher(), "g"); r.Action != "unified" {
		t.Errorf("g = %+v, want unified", r)
	}
}

func TestOverrideShadowsDefaultsThatArePrefixesOfIt(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Keys: map[keymap.Action][]string{
		"top": {"jj"},
	}})

	d, r := feed(km.Dispatcher(), "j")
	if r.Status != keymap.Pending {
		t.Fatalf("j = %+v, want pending", r)
	}
	if _, r = feed(d, "j"); r.Action != "top" {
		t.Errorf("jj = %+v, want top", r)
	}
}

func TestEmptyOverrideUnbindsTheAction(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Keys: map[keymap.Action][]string{
		"up": {},
	}})

	if _, r := feed(km.Dispatcher(), "k"); r.Status != keymap.Unbound {
		t.Errorf("k = %+v, want unbound", r)
	}
}

func TestLeaderOverride(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Leader: ","})

	if _, r := feed(km.Dispatcher(), ",", "u"); r.Action != "unified" {
		t.Errorf(",u = %+v, want unified", r)
	}
	if _, r := feed(km.Dispatcher(), "space"); r.Status != keymap.Unbound {
		t.Errorf("<space> = %+v, want unbound with leader ,", r)
	}
}

func TestLeaderInOverrides(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Leader: "<C-x>", Keys: map[keymap.Action][]string{
		"top": {"<leader>t"},
	}})

	if _, r := feed(km.Dispatcher(), "ctrl+x", "t"); r.Action != "top" {
		t.Errorf("<C-x>t = %+v, want top", r)
	}
}

func TestInvalidOverridesAreErrors(t *testing.T) {
	for name, ov := range map[string]keymap.Overrides{
		"unknown action":  {Keys: map[keymap.Action][]string{"fly": {"f"}}},
		"bad notation":    {Keys: map[keymap.Action][]string{"top": {"<nope>"}}},
		"unterminated":    {Keys: map[keymap.Action][]string{"top": {"<C-d"}}},
		"count conflict":  {Keys: map[keymap.Action][]string{"top": {"1"}}},
		"same keys twice": {Keys: map[keymap.Action][]string{"top": {"t"}, "up": {"t"}}},
		"prefix of other": {Keys: map[keymap.Action][]string{"top": {"t"}, "up": {"tt"}}},
		"long leader":     {Leader: "ab"},
		"digit leader":    {Leader: "1"},
		"leader leader":   {Leader: "<leader>"},
	} {
		if _, err := keymap.New(defaults, groups, ov); err == nil {
			t.Errorf("%s: New succeeded, want error", name)
		}
	}
}

// Next sorts by key so that which-key reads the same whatever the
// overrides.
func TestNextListsTheFollowUpKeysOfAPrefix(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	d, _ := feed(km.Dispatcher(), "space")

	want := []keymap.Entry{
		{Key: "f", Desc: "files", Group: true},
		{Key: "u", Desc: "toggle unified view"},
	}
	if got := d.Next(); !reflect.DeepEqual(got, want) {
		t.Errorf("Next after <space> = %+v, want %+v", got, want)
	}
	if got := d.Group(); got != "leader" {
		t.Errorf("Group after <space> = %q, want leader", got)
	}
}

func TestTypedShowsCountAndPrefixInVimNotation(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	d, _ := feed(km.Dispatcher(), "3", "space", "f")

	if got := d.Typed(); got != "3<space>f" {
		t.Errorf("Typed = %q, want 3<space>f", got)
	}
	if !d.InPrefix() {
		t.Error("InPrefix = false inside <space>f")
	}
	d, _ = feed(d, "p")
	if d.Typed() != "" || d.InPrefix() {
		t.Errorf("after a match Typed = %q, InPrefix = %v; want a fresh start", d.Typed(), d.InPrefix())
	}
}

func TestCountAloneIsNotAPrefix(t *testing.T) {
	km := mustNew(t, keymap.Overrides{})

	d, _ := feed(km.Dispatcher(), "5")

	if d.InPrefix() {
		t.Error("InPrefix = true after a bare count")
	}
	if d.Typed() != "5" {
		t.Errorf("Typed = %q, want 5", d.Typed())
	}
}

func TestNextShowsRebindings(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Leader: ",", Keys: map[keymap.Action][]string{
		"unified": {"<leader><C-u>"},
	}})

	d, _ := feed(km.Dispatcher(), ",")

	want := []keymap.Entry{
		{Key: "<C-u>", Desc: "toggle unified view"},
		{Key: "f", Desc: "files", Group: true},
	}
	if got := d.Next(); !reflect.DeepEqual(got, want) {
		t.Errorf("Next after , = %+v, want %+v", got, want)
	}
}

func TestHelpListsEveryActionWithItsKeys(t *testing.T) {
	km := mustNew(t, keymap.Overrides{Leader: ",", Keys: map[keymap.Action][]string{
		"up": {"j"},
	}})

	want := []keymap.HelpEntry{
		{Action: "down", Desc: "line down", Keys: []string{"<down>"}},
		{Action: "up", Desc: "line up", Keys: []string{"j"}},
		{Action: "top", Desc: "first line", Keys: []string{"gg"}},
		{Action: "next-hunk", Desc: "next hunk", Keys: []string{"]c"}},
		{Action: "half-down", Desc: "half page down", Keys: []string{"<C-d>"}},
		{Action: "unified", Desc: "toggle unified view", Keys: []string{",u"}},
		{Action: "prev-file", Desc: "previous file", Keys: []string{",fp"}},
	}
	if got := km.Help(); !reflect.DeepEqual(got, want) {
		t.Errorf("Help = %+v\nwant %+v", got, want)
	}
}

func TestFormatRendersVimNotation(t *testing.T) {
	for key, want := range map[string]string{
		"j":          "j",
		"G":          "G",
		"space":      "<space>",
		"ctrl+d":     "<C-d>",
		"alt+x":      "<A-x>",
		"shift+tab":  "<S-tab>",
		"enter":      "<cr>",
		"<":          "<lt>",
		"ctrl+space": "<C-space>",
	} {
		if got := keymap.Format(key); got != want {
			t.Errorf("Format(%q) = %q, want %q", key, got, want)
		}
	}
}
