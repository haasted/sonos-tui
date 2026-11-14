package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// startRowWait is the longest the program waits for the terminal to say
// where its cursor is. A terminal that answers does so at once; one that
// never answers must not hold up the start for long.
const startRowWait = 250 * time.Millisecond

// startRow asks the terminal which row the cursor is on, counted from 0. The
// inline frame is drawn from that row, which a click needs to know: the
// terminal counts a click from its own top. It asks before Bubble Tea starts,
// so no frame has moved the cursor yet. It reports false when it cannot tell.
func startRow() (int, bool) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return 0, false
	}
	defer tty.Close()

	// Without a deadline a terminal that never answers would block here.
	if err := tty.SetReadDeadline(time.Now().Add(startRowWait)); err != nil {
		return 0, false
	}
	state, err := term.MakeRaw(tty.Fd())
	if err != nil {
		return 0, false
	}
	defer func() { _ = term.Restore(tty.Fd(), state) }()

	if _, err := tty.WriteString(ansi.RequestCursorPositionReport); err != nil {
		return 0, false
	}
	var reply []byte
	buf := make([]byte, 32)
	for !bytes.ContainsRune(reply, 'R') {
		n, err := tty.Read(buf)
		if err != nil {
			return 0, false
		}
		reply = append(reply, buf[:n]...)
	}
	return parseCursorReport(reply)
}

// parseCursorReport reads the row from a cursor position report, ESC [ row ;
// column R, ignoring anything typed before it.
func parseCursorReport(reply []byte) (int, bool) {
	at := bytes.LastIndex(reply, []byte("\x1b["))
	if at < 0 {
		return 0, false
	}
	var row, column int
	if _, err := fmt.Sscanf(string(reply[at:]), "\x1b[%d;%dR", &row, &column); err != nil || row < 1 {
		return 0, false
	}
	return row - 1, true
}
