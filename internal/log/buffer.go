package log

import (
	"io"
	"sync"
)

type Buffer struct {
	mu    sync.Mutex
	lines []string
}

func NewBuffer() *Buffer {
	return &Buffer{}
}

func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.lines = append(b.lines, string(p))
	b.mu.Unlock()
	return len(p), nil
}

func (b *Buffer) Flush(w io.Writer) error {
	b.mu.Lock()
	lines := b.lines
	b.lines = nil
	b.mu.Unlock()

	for _, line := range lines {
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	return nil
}
