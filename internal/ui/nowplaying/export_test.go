package nowplaying

import "time"

const ScrollDelayTicks = scrollDelayTicks

// SweepLabel starts the stream label's glint immediately, so a test does not
// have to wait out the idle gap before one begins.
func SweepLabel(m Model) Model {
	m.labelSweep = time.Now()
	return m
}
