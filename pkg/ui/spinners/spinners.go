// Package spinners holds extra charm.land/bubbles/v2/spinner
// styles built from Braille Pattern characters, since bubbles/spinner only
// ships a fixed set. Exported the same way bubbles itself exports spinner.Dot,
// spinner.Line, etc., so they drop straight into spinner.Model.Spinner.
package spinners

import (
	"time"

	"charm.land/bubbles/v2/spinner"
)

// Braille reference for hand-building bar-style spinner frames (used by
// Equalizer, Bars and Jitter below). Each cell is 2 columns x 4 rows of
// dots that fill bottom-up, so a cell can represent two independent bar
// heights from 0 (empty) to 4 (full):
//
//	left-only,  height 0-4: ⠀ ⡀ ⡄ ⡆ ⡇
//	right-only, height 0-4: ⠀ ⢀ ⢠ ⢰ ⢸
//
//	combined grid, rows = left height, columns = right height:
//	⠀ ⢀ ⢠ ⢰ ⢸
//	⡀ ⣀ ⣠ ⣰ ⣸
//	⡄ ⣄ ⣤ ⣴ ⣼
//	⡆ ⣆ ⣦ ⣶ ⣾
//	⡇ ⣇ ⣧ ⣷ ⣿

// Full Braille Patterns Unicode block, U+2800-U+28FF (256 chars, every
// possible combination of the 8 dots — the bar-height table above is just
// the 25 of these where each column fills contiguously from the bottom).
// Row = codepoint high nibble of the low byte, column = low nibble, e.g.
// row E0s column 6 is U+28E6 = ⣦.
//
//	     0 1 2 3 4 5 6 7 8 9 A B C D E F
//	00s  ⠀ ⠁ ⠂ ⠃ ⠄ ⠅ ⠆ ⠇ ⠈ ⠉ ⠊ ⠋ ⠌ ⠍ ⠎ ⠏
//	10s  ⠐ ⠑ ⠒ ⠓ ⠔ ⠕ ⠖ ⠗ ⠘ ⠙ ⠚ ⠛ ⠜ ⠝ ⠞ ⠟
//	20s  ⠠ ⠡ ⠢ ⠣ ⠤ ⠥ ⠦ ⠧ ⠨ ⠩ ⠪ ⠫ ⠬ ⠭ ⠮ ⠯
//	30s  ⠰ ⠱ ⠲ ⠳ ⠴ ⠵ ⠶ ⠷ ⠸ ⠹ ⠺ ⠻ ⠼ ⠽ ⠾ ⠿
//	40s  ⡀ ⡁ ⡂ ⡃ ⡄ ⡅ ⡆ ⡇ ⡈ ⡉ ⡊ ⡋ ⡌ ⡍ ⡎ ⡏
//	50s  ⡐ ⡑ ⡒ ⡓ ⡔ ⡕ ⡖ ⡗ ⡘ ⡙ ⡚ ⡛ ⡜ ⡝ ⡞ ⡟
//	60s  ⡠ ⡡ ⡢ ⡣ ⡤ ⡥ ⡦ ⡧ ⡨ ⡩ ⡪ ⡫ ⡬ ⡭ ⡮ ⡯
//	70s  ⡰ ⡱ ⡲ ⡳ ⡴ ⡵ ⡶ ⡷ ⡸ ⡹ ⡺ ⡻ ⡼ ⡽ ⡾ ⡿
//	80s  ⢀ ⢁ ⢂ ⢃ ⢄ ⢅ ⢆ ⢇ ⢈ ⢉ ⢊ ⢋ ⢌ ⢍ ⢎ ⢏
//	90s  ⢐ ⢑ ⢒ ⢓ ⢔ ⢕ ⢖ ⢗ ⢘ ⢙ ⢚ ⢛ ⢜ ⢝ ⢞ ⢟
//	A0s  ⢠ ⢡ ⢢ ⢣ ⢤ ⢥ ⢦ ⢧ ⢨ ⢩ ⢪ ⢫ ⢬ ⢭ ⢮ ⢯
//	B0s  ⢰ ⢱ ⢲ ⢳ ⢴ ⢵ ⢶ ⢷ ⢸ ⢹ ⢺ ⢻ ⢼ ⢽ ⢾ ⢿
//	C0s  ⣀ ⣁ ⣂ ⣃ ⣄ ⣅ ⣆ ⣇ ⣈ ⣉ ⣊ ⣋ ⣌ ⣍ ⣎ ⣏
//	D0s  ⣐ ⣑ ⣒ ⣓ ⣔ ⣕ ⣖ ⣗ ⣘ ⣙ ⣚ ⣛ ⣜ ⣝ ⣞ ⣟
//	E0s  ⣠ ⣡ ⣢ ⣣ ⣤ ⣥ ⣦ ⣧ ⣨ ⣩ ⣪ ⣫ ⣬ ⣭ ⣮ ⣯
//	F0s  ⣰ ⣱ ⣲ ⣳ ⣴ ⣵ ⣶ ⣷ ⣸ ⣹ ⣺ ⣻ ⣼ ⣽ ⣾ ⣿

// Equalizer is a single braille cell pulsing through a few fill levels.
var Equalizer = spinner.Spinner{
	Frames: []string{"⣠", "⣶", "⣧"},
	FPS:    200 * time.Millisecond,
}

// Bars renders 3 braille cells side by side (6 half-width columns), each
// column's dot count standing in for a bar height (1-4), phase-shifted so
// they bounce like an equalizer rather than blinking together.
var Bars = spinner.Spinner{
	Frames: []string{"⣠⣾⣦", "⣴⣷⣄", "⣾⣦⣠", "⣷⣄⣴", "⣦⣠⣾", "⣄⣴⣷"},
	FPS:    time.Second / 8,
}

// Jitter is the same 6-bar layout as Bars, but each bar jumps to an
// unrelated height every frame instead of rippling through a wave — closer
// to how a real audio level meter looks, all bars moving independently
// rather than in a traveling pattern.
var Jitter = spinner.Spinner{
	Frames: []string{"⣦⣾⣤", "⣇⣴⣸", "⣠⣠⣆", "⣄⣷⣸", "⣠⣾⣇", "⣿⣄⣠", "⣆⣶⣼", "⣸⣼⣇"},
	FPS:    time.Second / 6,
}
