package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"
)

// Case: every key the app actually handles, which must be documented. The help
// table is hand-written rather than generated from declarative bindings, so
// this reads the bindings out of the source instead —
// adding one without documenting it fails here rather than silently rotting.
func TestHelp_DocumentsEveryKeyTheAppHandles(t *testing.T) {
	documented := map[string]map[string]bool{}
	for file, layer := range helpLayers {
		documented[file] = helpKeySet(layer.table)
	}

	for _, bound := range boundKeys(t) {
		if why, omitted := helpOmissions[bound.key]; omitted {
			t.Logf("%q deliberately undocumented: %s", bound.key, why)
			continue
		}
		if why, bare := helpLayersWithoutAPopup[bound.file]; bare {
			t.Logf("%s binds %q and no popup covers it: %s", bound.file, bound.key, why)
			continue
		}

		layer, named := helpLayers[bound.file]
		if !named {
			t.Errorf("%s binds %q, and no popup is named as its owner — add it to helpLayers "+
				"or to helpLayersWithoutAPopup with a reason", bound.file, bound.key)
			continue
		}

		want := bound.key
		if alias, ok := helpAliases[bound.key]; ok {
			want = alias
		}
		if !documented[bound.file][want] {
			t.Errorf("%s binds %q, which the %s popup does not document (looked for %q)",
				bound.file, bound.key, layer.name, want)
		}
	}
}

// Case: the popup's own bindings, which the app must actually honour. The
// reverse direction of the same drift: documenting a key nobody handles.
func TestHelp_PromisesNothingTheAppIgnores(t *testing.T) {
	bound := map[string]bool{}
	for _, key := range boundKeys(t) {
		bound[key.key] = true
	}

	for _, key := range []string{"?", "q", "p", "j", "l", "m", "enter"} {
		if !bound[key] {
			t.Errorf("the help popup documents %q, which nothing handles", key)
		}
	}
}

// Case: the rule that each popup earns its place -- it shows the keys that work
// where the reader is. A queue key in the player's popup promises something
// that does nothing until the queue is open.
func TestHelp_ThePlayerPopupHoldsNoQueueKeys(t *testing.T) {
	documented := helpKeySet(playerHelp)

	// k and j are left out: they are bound in both places, meaning play/pause
	// and previous track here, and one row up or down inside the list.
	for _, key := range []string{"pgup", "pgdn", "spc", "^u", "^d", "home", "end", "G", "F"} {
		if documented[key] {
			t.Errorf("the player popup shows %q, which only moves the queue", key)
		}
	}
}

// Case: the queue's own popup, which must say how to move the list and how to
// leave it.
func TestHelp_TheQueuePopupCoversTheList(t *testing.T) {
	documented := helpKeySet(queueHelp)

	for _, key := range []string{"pgup", "pgdn", "^u", "^d", "home", "end", "esc"} {
		if !documented[key] {
			t.Errorf("the queue popup leaves out %q", key)
		}
	}
}

// helpKeySet is every key the tables document, as whole words. Searching the
// rendered popup for a substring instead lets a one-letter binding pass on any
// word holding that letter: "b" matched both "Playback" and "tab".
func helpKeySet(tables ...helpTable) map[string]bool {
	documented := map[string]bool{}
	for _, table := range tables {
		for _, column := range table {
			for _, section := range column {
				for _, key := range section.keys {
					for _, word := range strings.Fields(key.keys) {
						documented[word] = true
					}
				}
			}
		}
	}
	return documented
}

// helpLayers names the popup that must document a binding, by the file it
// lives in. The key alone cannot say: k and j mean different things in the
// player and the queue.
var helpLayers = map[string]struct {
	name  string // what to call the popup when a key is missing from it
	table helpTable
}{
	"speaker.go":    {"player", playerHelp},
	"appModel.go":   {"player", playerHelp},
	"tabs.go":       {"player", playerHelp},
	"queue.go":      {"queue", queueHelp},
	"favourites.go": {"favourites", favouritesHelp},
}

// helpLayersWithoutAPopup are files whose bindings no popup documents yet,
// with the reason, so the gap is logged rather than hidden.
var helpLayersWithoutAPopup = map[string]string{
	"grouping.go":    "the grouping modal has no table of its own, and takes ? before the player's handler sees it",
	"markchooser.go": "the chooser is two rows, moved with the queue's keys and closed with enter or esc",
}

// helpAliases maps a binding to the text that documents it, where the two
// differ: uppercase variants are covered by their lowercase form, the arrow
// keys are drawn as glyphs, and the queue's keys are abbreviated to fit the
// key column.
var helpAliases = map[string]string{
	"P": "p", "K": "k", "J": "j", "L": "l", "M": "m",
	"up": "↑", "down": "↓",
	"right": "→", "left": "←", "shift+tab": "⇧tab",
	"pgdown": "pgdn", "space": "spc", "ctrl+u": "^u", "ctrl+d": "^d",
	"shift+up": "⇧↑", "shift+down": "⇧↓",
}

// helpOmissions are bindings deliberately left out, with the reason. Anything
// not listed here has to appear in the popup.
var helpOmissions = map[string]string{
	"ctrl+c": "documented as q, the more discoverable alias for the same thing",
	"?":      "Not showing the button that opened the list of keyboard shortcuts",
}

// boundKey is one key literal and the file that switches on it. The file is
// the only thing in the source that says which popup ought to document it.
type boundKey struct {
	key  string
	file string
}

// boundKeys reads every key literal the app switches on, from all three layers
// that own bindings. Parsing the source rather than maintaining a second list
// is the point: a second list would drift exactly as the help text would.
func boundKeys(t *testing.T) []boundKey {
	t.Helper()

	var keys []boundKey
	for _, dir := range []string{".", "../../internal/ui/panes"} {
		// The file set outlives the parse: it is what turns a node position
		// back into the file name that says which layer owns the binding.
		fset := token.NewFileSet()
		packages, err := parser.ParseDir(fset, dir, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", dir, err)
		}
		for _, pkg := range packages {
			ast.Inspect(pkg, func(n ast.Node) bool {
				stmt, ok := n.(*ast.SwitchStmt)
				if !ok || !switchesOnAKey(stmt) {
					return true
				}
				file := filepath.Base(fset.Position(stmt.Pos()).Filename)
				for _, clause := range stmt.Body.List {
					for _, key := range stringLiterals(clause.(*ast.CaseClause).List) {
						keys = append(keys, boundKey{key: key, file: file})
					}
				}
				return true
			})
		}
	}

	if len(keys) < 10 {
		t.Fatalf("found only %d bound keys, so the scan is broken rather than the help", len(keys))
	}
	return keys
}

// switchesOnAKey reports whether a switch dispatches on a key name. Both forms
// appear: "switch msg.String()" in cmd/sonos, and "switch key := keyMsg.String();
// key" in panes.Tabs, where the call is in the init statement instead.
func switchesOnAKey(stmt *ast.SwitchStmt) bool {
	if callsString(stmt.Tag) {
		return true
	}
	assign, ok := stmt.Init.(*ast.AssignStmt)
	if !ok {
		return false
	}
	for _, rhs := range assign.Rhs {
		if callsString(rhs) {
			return true
		}
	}
	return false
}

func callsString(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "String"
}

func stringLiterals(exprs []ast.Expr) []string {
	var out []string
	for _, expr := range exprs {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		if value, err := strconv.Unquote(lit.Value); err == nil {
			out = append(out, value)
		}
	}
	return out
}

// Case: "?" with the player running, which must open the popup over it.
func TestHelp_QuestionMarkOpensAndCloses(t *testing.T) {
	m := sizedModel(t)

	m = pressKey(m, "?")
	if !m.help.IsOpen() {
		t.Fatal("? did not open the help popup")
	}
	if !strings.Contains(m.View().Content, "play / pause") {
		t.Fatal("the popup is open but its content is not rendered")
	}

	m = pressKey(m, "?")
	if m.help.IsOpen() {
		t.Fatal("? did not close the help popup again")
	}
}

// Case: keys pressed while help is open, which must not reach the player or
// the tabs. Without this the arrow keys keep changing volume behind it, unseen
// — which the modal component itself could not test, having no consumer.
func TestHelp_OwnsTheKeyboardWhileOpen(t *testing.T) {
	m := sizedModel(t)
	active := m.tabs.Active

	m = pressKey(m, "?")
	for _, key := range []string{"right", "down", "m", "p"} {
		m = pressKey(m, key)
	}

	if m.tabs.Active != active {
		t.Fatal("tab navigation reached the tabs behind an open help popup")
	}
	if !m.help.IsOpen() {
		t.Fatal("a playback key closed the popup instead of being swallowed")
	}
	if m.quitting {
		t.Fatal("the program began quitting from behind the popup")
	}
}

// Case: closing the popup, after which the frame must NOT shrink back. The
// inline renderer works from a relative cursor, so once the terminal has
// scrolled to make room it cannot reach above its own frame to erase rows it
// gives back — each open/close cycle would strand a copy of the popup.
func TestHelp_FrameNeverShrinksBack(t *testing.T) {
	m := sizedModel(t)
	player := lipgloss.Height(m.View().Content)

	open := pressKey(m, "?")
	opened := lipgloss.Height(open.View().Content)
	if opened <= player {
		t.Fatal("the popup is taller than the player, so the frame must grow")
	}

	closed := pressKey(open, "esc")
	if got := lipgloss.Height(closed.View().Content); got != opened {
		t.Fatalf("frame shrank from %d rows to %d on close, which strands the difference",
			opened, got)
	}
}

// Case: repeated open/close cycles, which must all render at one height. This
// is the reported symptom directly: each cycle used to leave another copy of
// the popup behind in the scrollback.
func TestHelp_RepeatedCyclesKeepOneHeight(t *testing.T) {
	m := pressKey(sizedModel(t), "?")
	height := lipgloss.Height(m.View().Content)

	for range 3 {
		m = pressKey(pressKey(m, "esc"), "?")
		if got := lipgloss.Height(m.View().Content); got != height {
			t.Fatalf("frame height moved to %d during repeated cycles, want a steady %d", got, height)
		}
	}
	if got := lipgloss.Height(pressKey(m, "esc").View().Content); got != height {
		t.Fatalf("frame height moved to %d after the final close, want %d", got, height)
	}
}

func sizedModel(t *testing.T) appModel {
	t.Helper()

	model, _ := loadedModel(t).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return model.(appModel)
}

func pressKey(m appModel, key string) appModel {
	model, _ := m.Update(keyMsgFor(key))
	return model.(appModel)
}

func keyMsgFor(key string) tea.KeyMsg {
	switch key {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "pgup":
		return tea.KeyPressMsg{Code: tea.KeyPgUp}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "ctrl+u", "ctrl+d":
		return tea.KeyPressMsg{Code: rune(key[len(key)-1]), Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
}
