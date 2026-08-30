package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/haasted/sonos-sandbox/internal/ui/panes"
)

var (
	_ tea.Model = &sandboxRoot{}
	_ tea.Model = &sandboxBranch{}
	_ tea.Model = &sandboxLeaf{}
)

type tickMsg struct{ target string }

type sandboxLeaf struct {
	name      string
	remaining int
	ticks     int
}

func (l sandboxLeaf) Init() tea.Cmd {
	return l.tick()
}

func (l sandboxLeaf) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	tick, ok := msg.(tickMsg)
	if !ok || tick.target != l.name {
		return l, nil
	}

	l.ticks++
	l.remaining--
	if l.remaining <= 0 {
		return l, nil
	}
	return l, l.tick()
}

func (l sandboxLeaf) View() tea.View {
	return tea.NewView(fmt.Sprintf("%s:%d", l.name, l.ticks))
}

func (l sandboxLeaf) tick() tea.Cmd {
	println(" --- Creating tick")
	return func() tea.Msg {
		return tickMsg{target: l.name}
	}
}

func (l sandboxLeaf) done() bool {
	return l.remaining <= 0
}

type sandboxBranch struct {
	left  sandboxLeaf
	right sandboxLeaf
}

func (b sandboxBranch) Init() tea.Cmd {
	return tea.Batch(b.left.Init(), b.right.Init())
}

func (b sandboxBranch) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	left, leftCmd := b.left.Update(msg)
	b.left = left.(sandboxLeaf)

	right, rightCmd := b.right.Update(msg)
	b.right = right.(sandboxLeaf)

	return b, tea.Batch(leftCmd, rightCmd)
}

func (b sandboxBranch) View() tea.View {
	return tea.NewView(b.left.View().Content + " " + b.right.View().Content)
}

func (b sandboxBranch) done() bool {
	return b.left.done() && b.right.done()
}

func (b sandboxRoot) done() bool {
	return b.branch.done()
}

type sandboxRoot struct {
	branch sandboxBranch
}

func newSandboxRoot(ticks int) sandboxRoot {
	return sandboxRoot{branch: sandboxBranch{
		left:  sandboxLeaf{name: "left", remaining: ticks},
		right: sandboxLeaf{name: "right", remaining: ticks},
	}}
}

func (r sandboxRoot) Init() tea.Cmd {
	return r.branch.Init()
}

func (r sandboxRoot) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	branch, cmd := r.branch.Update(msg)
	r.branch = branch.(sandboxBranch)

	if r.branch.done() {
		return r, tea.Quit
	}
	return r, cmd
}

func (r sandboxRoot) View() tea.View {
	return tea.NewView("[" + r.branch.View().Content + "]")
}

func run(t *testing.T, root sandboxRoot) tea.Model {
	t.Helper()

	final, err := tea.NewProgram(root, tea.WithoutRenderer(), tea.WithInput(nil)).Run()
	require.NoError(t, err)
	return final
}

func TestLoop_ACommandsReturnValueBecomesTheNextMessage(t *testing.T) {
	var leaf tea.Model = sandboxLeaf{name: "left", remaining: 3}

	leaf, cmd := leaf.Update(tickMsg{target: "left"})
	require.NotNil(t, cmd, "a leaf with work left schedules itself again")
	assert.Equal(t, 1, leaf.(sandboxLeaf).ticks)

	msg := cmd()
	assert.Equal(t, tickMsg{target: "left"}, msg,
		"running a command yields a message; that message is what Update sees next")

	leaf, _ = leaf.Update(msg)
	assert.Equal(t, 2, leaf.(sandboxLeaf).ticks,
		"feeding it back is the whole cycle: Update -> Cmd -> Msg -> Update")
}

func TestLoop_NoCommandMeansTheModelStopsMoving(t *testing.T) {
	var leaf tea.Model = sandboxLeaf{name: "left", remaining: 1}

	leaf, cmd := leaf.Update(tickMsg{target: "left"})

	assert.Equal(t, 1, leaf.(sandboxLeaf).ticks)
	assert.Nil(t, cmd, "returning no command ends this chain; nothing else will ever call Update")
}

func TestLoop_ProgramDrivesTheWholeTreeToCompletion(t *testing.T) {
	final := run(t, newSandboxRoot(3))

	assert.Equal(t, "[left:3 right:3]", final.View().Content)
	assert.True(t, final.(sandboxRoot).done())
}

func TestLoop_ParentDecidesWhichChildrenSeeAMessage(t *testing.T) {
	var branch tea.Model = sandboxBranch{
		left:  sandboxLeaf{name: "left", remaining: 5},
		right: sandboxLeaf{name: "right", remaining: 5},
	}

	branch, _ = branch.Update(tickMsg{target: "left"})

	assert.Equal(t, 1, branch.(sandboxBranch).left.ticks)
	assert.Equal(t, 0, branch.(sandboxBranch).right.ticks,
		"the branch hands the message to both, but only the addressee acts on it")
}

func TestLoop_ViewIsDerivedOnlyFromModelState(t *testing.T) {
	root := newSandboxRoot(8)
	assert.Equal(t, "[left:0 right:0]", root.View().Content)

	assert.Equal(t, "[left:8 right:8]", run(t, root).View().Content)
}

func TestLoop_ContextCancellationTerminatesTheProgram(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	program :=
		tea.NewProgram(panes.StaticContent("idle"),
			tea.WithContext(ctx),
			tea.WithoutRenderer(),
			tea.WithInput(nil),
		)

	_, err := program.Run()

	require.ErrorIs(t, err, tea.ErrProgramKilled,
		"StaticContent issues no commands, so nothing but the context can end this program")
	require.ErrorIs(t, err, context.DeadlineExceeded,
		"the context's own error is wrapped too, so a deliberate shutdown is distinguishable from a crash")
}
