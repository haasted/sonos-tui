package panes

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// Case: a notice with an entrance, walked across its whole life. The rule must
// be unbroken at both ends — an entrance that started or finished mid-text
// would leave letters stranded in the divider.
func TestNoticeEntrance_OpensOutOfTheRuleAndClosesBackIntoIt(t *testing.T) {
	n := notice{text: "press ? for keyboard shortcuts", level: NoticeInfo, entrance: NoticeBloom}
	run := strings.Repeat(string(noticeRule), 60)

	n.shownAt = time.Now()
	if got := stripANSI(n.render(run)); got != run {
		t.Fatalf("at t=0 the divider is %q; a notice must open out of an unbroken rule", got)
	}

	n.shownAt = time.Now().Add(-n.openDuration())
	if got := stripANSI(n.render(run)); !strings.Contains(got, "press ? for keyboard shortcuts") {
		t.Fatalf("once open the divider is %q; the text should be readable", got)
	}

	n.shownAt = time.Now().Add(-(n.openDuration() + noticeHold + noticeFade))
	if got := stripANSI(n.render(run)); got != run {
		t.Fatalf("after collapsing the divider is %q; it must be whole again", got)
	}
}

// Case: the hold. An entrance that ate into it would leave less time to read
// the text than a plain notice gets, which is backwards for a longer message.
func TestNoticeEntrance_HoldsForAsLongAsAPlainNotice(t *testing.T) {
	plain := notice{text: "hello", level: NoticeInfo}
	opened := notice{text: "hello", level: NoticeInfo, entrance: NoticeBloom}

	if opened.openDuration() <= 0 {
		t.Fatal("an entrance should take time; without one there is nothing to see")
	}
	if plain.openDuration() != 0 {
		t.Fatal("a notice with no entrance should not delay its own hold")
	}

	plain.shownAt = time.Now().Add(-(noticeHold - 10*time.Millisecond))
	opened.shownAt = time.Now().Add(-(opened.openDuration() + noticeHold - 10*time.Millisecond))
	if plain.strength() != 1 || opened.strength() != 1 {
		t.Fatal("both should still be fully lit just before their hold ends")
	}

	opened.shownAt = time.Now().Add(-(opened.openDuration() + noticeHold + noticeFade))
	if opened.strength() != 0 {
		t.Fatal("an opened notice should be spent once its collapse has run")
	}
}

// Case: an entrance repaints per frame where a fade only eases a colour, so
// the two cannot share a tick rate without one of them looking wrong.
func TestNoticeEntrance_RepaintsFasterThanAPlainFade(t *testing.T) {
	plain := notice{text: "hello"}
	opened := notice{text: "hello", entrance: NoticeBloom}

	if opened.interval() >= plain.interval() {
		t.Fatalf("an opening notice repaints every %s against a fade's %s; it needs to be quicker",
			opened.interval(), plain.interval())
	}
}

// Case: the default. Every existing caller sends no entrance, and their
// notices must keep simply appearing.
func TestNoticeEntrance_ZeroValueIsNoAnimation(t *testing.T) {
	if (NoticeMsg{}).Entrance != NoticeAppear {
		t.Fatal("a NoticeMsg with no Entrance set must not animate")
	}

	n := notice{text: "Køkken did not answer", level: NoticeWarn, shownAt: time.Now()}
	run := strings.Repeat(string(noticeRule), 60)
	if got := stripANSI(n.render(run)); !strings.Contains(got, "Køkken did not answer") {
		t.Fatalf("a plain notice should be readable immediately, got %q", got)
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string { return ansiPattern.ReplaceAllString(s, "") }

// Case: a notice narrow enough to centre, which is the common one — the label
// should sit on the divider's own axis rather than near its left edge.
func TestNotice_IsCentredInTheRule(t *testing.T) {
	n := notice{text: "press ? for keyboard shortcuts", level: NoticeInfo, shownAt: time.Now()}
	line := stripANSI(n.render(strings.Repeat(string(noticeRule), 60)))

	lead := len([]rune(line)) - len([]rune(strings.TrimLeft(line, string(noticeRule))))
	trail := len([]rune(line)) - len([]rune(strings.TrimRight(line, string(noticeRule))))

	if lead-trail > 1 || trail-lead > 1 {
		t.Fatalf("rule runs %d left and %d right; a centred notice should sit between equal runs", lead, trail)
	}
}

// Case: a notice too long to centre. Symmetric margins would have to come out
// of the text, and the message matters more than the symmetry.
func TestNotice_LongNoticeKeepsItsTextRatherThanItsMargins(t *testing.T) {
	const text = "Soveværelse: no live updates"

	n := notice{text: text, level: NoticeWarn, shownAt: time.Now()}
	line := stripANSI(n.render(strings.Repeat(string(noticeRule), 38)))

	if !strings.Contains(line, text) {
		t.Fatalf("centring cost the notice its text: %q", line)
	}
	if lead := len([]rune(line)) - len([]rune(strings.TrimLeft(line, string(noticeRule)))); lead < noticeLead {
		t.Fatalf("only %d rule runes before the label; the line stops reading as a divider below %d", lead, noticeLead)
	}
}
