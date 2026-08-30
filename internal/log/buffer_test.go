package log_test

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuffer_Flush_PreservesOrder(t *testing.T) {
	b := log.NewBuffer()

	fmt.Fprintln(b, "first")
	fmt.Fprintln(b, "second")
	fmt.Fprintln(b, "third")

	var out bytes.Buffer
	require.NoError(t, b.Flush(&out))

	assert.Equal(t, "first\nsecond\nthird\n", out.String())
}

func TestBuffer_Flush_EmptiesBuffer(t *testing.T) {
	b := log.NewBuffer()
	fmt.Fprintln(b, "only")

	var first bytes.Buffer
	require.NoError(t, b.Flush(&first))
	assert.Equal(t, "only\n", first.String())

	var second bytes.Buffer
	require.NoError(t, b.Flush(&second))
	assert.Empty(t, second.String())
}

func TestBuffer_ConcurrentWrites(t *testing.T) {
	b := log.NewBuffer()

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fmt.Fprintf(b, "line %d\n", i)
		}(i)
	}
	wg.Wait()

	var out bytes.Buffer
	require.NoError(t, b.Flush(&out))

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	require.Len(t, lines, n)

	seen := make(map[string]bool, n)
	for _, l := range lines {
		seen[l] = true
	}
	sorted := make([]string, 0, n)
	for l := range seen {
		sorted = append(sorted, l)
	}
	sort.Strings(sorted)
	assert.Len(t, sorted, n, "expected every concurrent write to survive with no duplicates or loss")
}
