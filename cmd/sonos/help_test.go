package main

import (
	"go/ast"
	"go/parser"
	"go/token"
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
	text := helpContent{}.View().Content

	for _, key := range boundKeys(t) {
		if why, omitted := helpOmissions[key]; omitted {
			t.Logf("%q deliberately undocumented: %s", key, why)
			continue
		}

		want := key
		if alias, ok := helpAliases[key]; ok {
			want = alias
		}
		if !strings.Contains(text, want) {
			t.Errorf("key %q is handled but not in the help popup (looked for %q)", key, want)
		}
	}
}

// Case: the popup's own bindings, which the app must actually honour. The
// reverse direction of the same drift: documenting a key nobody handles.
func TestHelp_PromisesNothingTheAppIgnores(t *testing.T) {
	bound := map[string]bool{}
	for _, key := range boundKeys(t) {
		bound[key] = true
	}

	for _, key := range []string{"?", "q", "p", "j", "l", "m", "enter"} {
		if !bound[key] {
			t.Errorf("the help popup documents %q, which nothing handles", key)
		}
	}
}

// helpAliases maps a binding to the text that documents it, where the two
// differ: uppercase variants are covered by their lowercase form, and the
// arrow keys are drawn as glyphs.
var helpAliases = map[string]string{
	"P": "p", "K": "k", "J": "j", "L": "l", "M": "m",
	"up": "↑", "down": "↓",
	"right": "→", "left": "←", "shift+tab": "⇧tab",
}

// helpOmissions are bindings deliberately left out, with the reason. Anything
// not listed here has to appear in the popup.
var helpOmissions = map[string]string{
	"ctrl+c": "documented as q, the more discoverable alias for the same thing",
	"Q":      "debug only — dumps the queue to the log until a queue view exists",
	"?":      "Not showing the button that opened the list of keyboard shortcuts",
}

// boundKeys reads every key literal the app switches on, from all three layers
// that own bindings. Parsing the source rather than maintaining a second list
// is the point: a second list would drift exactly as the help text would.
func boundKeys(t *testing.T) []string {
	t.Helper()

	var keys []string
	for _, dir := range []string{".", "../../internal/ui/panes"} {
		packages, err := parser.ParseDir(token.NewFileSet(), dir, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", dir, err)
		}
		for _, pkg := range packages {
			ast.Inspect(pkg, func(n ast.Node) bool {
				stmt, ok := n.(*ast.SwitchStmt)
				if !ok || !switchesOnAKey(stmt) {
					return true
				}
				for _, clause := range stmt.Body.List {
					keys = append(keys, stringLiterals(clause.(*ast.CaseClause).List)...)
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
	default:
		return tea.KeyPressMsg{Code: rune(key[0]), Text: key}
	}
}
