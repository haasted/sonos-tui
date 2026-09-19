package main

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// Case: the width of every frame within one proposal. A spinner whose frames
// differ in width moves the whole panel sideways on every frame, which no
// amount of taste makes acceptable.
func TestSpinnerProposals_FramesKeepOneWidth(t *testing.T) {
	for _, family := range spinnerFamilies {
		for _, p := range family.items {
			want := lipgloss.Width(p.spin.Frames[0])
			for _, frame := range p.spin.Frames {
				if got := lipgloss.Width(frame); got != want {
					t.Errorf("%s: frame %q is %d cells where the first is %d",
						p.name, frame, got, want)
				}
			}
		}
	}
}

// Case: the width the panel has room for. The now-playing panel takes the
// indent of its artist and album rows from the glyph on screen, and the paused
// and muted glyphs are one cell, so anything wider shifts those rows whenever
// playback stops. This reports rather than fails: a wider frame is a choice the
// reader may still want, and the demo labels it.
func TestSpinnerProposals_ReportFramesWiderThanOneCell(t *testing.T) {
	for _, family := range spinnerFamilies {
		for _, p := range family.items {
			if w := lipgloss.Width(p.spin.Frames[0]); w != 1 {
				t.Logf("%-10s %d cells — would shift the panel when playback stops", p.name, w)
			}
		}
	}
}

// Case: the count the demo prints, against the proposals it holds.
func TestSpinnerProposals_CountMatchesTheFamilies(t *testing.T) {
	total := 0
	for _, family := range spinnerFamilies {
		if len(family.items) == 0 {
			t.Errorf("family %q holds nothing", family.title)
		}
		total += len(family.items)
	}
	if spinnerCount() != total {
		t.Errorf("the demo says %d candidates, the families hold %d", spinnerCount(), total)
	}
	if total < 10 {
		t.Errorf("only %d proposals, which is too few to choose from", total)
	}
}
