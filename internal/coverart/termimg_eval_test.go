// Package coverart_test evaluates go-termimg for rendering Sonos cover art (sonos-thx); no live speaker calls.
package coverart_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blacktop/go-termimg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultFixture is used by tests that only need one representative image (library-mechanics checks, not content-sensitive ones).
const defaultFixture = "hellwalkers-cover.jpg"

// fixtureNames are all cover art files in data/, used to parameterize content-sensitive tests.
var fixtureNames = []string{
	"hellwalkers-cover.jpg",
	"bojack-horseman-cover.jpg",
	"jeff-buckley-cover.jpg",
	"bowie-heroes-cover.jpg",
}

// fixturePath returns the path to a named cover art file in data/.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "data", name)
	_, err := os.Stat(path)
	require.NoError(t, err, "test fixture missing: %s", path)
	return path
}

// squareCellSize returns the cols/rows needed to render an image square, since terminal cells aren't (~2x taller than wide).
func squareCellSize(t *testing.T, targetRows int) (cols, rows int, feat *termimg.TerminalFeatures) {
	t.Helper()
	feat = termimg.QueryTerminalFeatures()
	require.NotZero(t, feat.FontHeight, "expected a non-zero cell height")
	cellAspect := float64(feat.FontWidth) / float64(feat.FontHeight)
	return int(float64(targetRows) / cellAspect), targetRows, feat
}

// allProtocols are the terminal graphics protocols go-termimg supports (excluding Auto/Unsupported).
var allProtocols = []termimg.Protocol{
	termimg.ITerm2,
	termimg.Kitty,
	termimg.Sixel,
	termimg.Halfblocks,
}

// TestRenderFromFile_AllProtocols renders every fixture under every protocol, forced explicitly so it doesn't depend on the terminal.
func TestRenderFromFile_AllProtocols(t *testing.T) {
	cols, rows, _ := squareCellSize(t, 20)

	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			path := fixturePath(t, name)

			for _, proto := range allProtocols {
				t.Run(proto.String(), func(t *testing.T) {
					img, err := termimg.Open(path)
					require.NoError(t, err)

					out, err := img.
						Protocol(proto).
						Width(cols).
						Height(rows).
						Scale(termimg.ScaleFit).
						Render()

					require.NoError(t, err, "protocol %s should render without error", proto)
					assert.NotEmpty(t, out, "protocol %s produced empty output", proto)
					printPreview(t, proto.String(), out)
				})
			}
		})
	}
}

// printPreview writes rendered output to stdout when -v is passed, so a human can see the image; t.Log would corrupt escape sequences.
func printPreview(t *testing.T, label, out string) {
	t.Helper()
	if !testing.Verbose() {
		return
	}
	fmt.Printf("\n--- %s: %s ---\n%s\n", t.Name(), label, out)
}

// TestRenderFromFile_VisualPreview renders with Protocol(Auto), the same detection path production code would use.
func TestRenderFromFile_VisualPreview(t *testing.T) {
	path := fixturePath(t, defaultFixture)

	img, err := termimg.Open(path)
	require.NoError(t, err)

	out, err := img.Protocol(termimg.Auto).Width(20).Height(20).Scale(termimg.ScaleFit).Render()
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	printPreview(t, "Auto ("+termimg.DetectProtocol().String()+")", out)
}

// TestRenderFromFile_SquareAspect renders the fixture at an aspect-corrected size so it appears square on screen.
func TestRenderFromFile_SquareAspect(t *testing.T) {
	path := fixturePath(t, defaultFixture)
	cols, rows, feat := squareCellSize(t, 20)
	cellAspect := float64(feat.FontWidth) / float64(feat.FontHeight)

	img, err := termimg.Open(path)
	require.NoError(t, err)

	out, err := img.Protocol(termimg.Auto).Width(cols).Height(rows).Scale(termimg.ScaleFit).Render()
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	label := fmt.Sprintf("Auto, cell %dx%dpx (aspect %.2f) -> %dx%d cells",
		feat.FontWidth, feat.FontHeight, cellAspect, cols, rows)
	printPreview(t, label, out)
}

// TestRenderFromReader_HTTPStylePath checks termimg.From (the io.Reader path a fetched AlbumArtURI would use) matches Open(), for every fixture.
func TestRenderFromReader_HTTPStylePath(t *testing.T) {
	for _, name := range fixtureNames {
		t.Run(name, func(t *testing.T) {
			path := fixturePath(t, name)

			fileImg, err := termimg.Open(path)
			require.NoError(t, err)
			wantOut, err := fileImg.Protocol(termimg.Halfblocks).Width(20).Height(20).Scale(termimg.ScaleFit).Render()
			require.NoError(t, err)

			data, err := os.ReadFile(path)
			require.NoError(t, err)

			readerImg, err := termimg.From(bytes.NewReader(data))
			require.NoError(t, err)
			gotOut, err := readerImg.Protocol(termimg.Halfblocks).Width(20).Height(20).Scale(termimg.ScaleFit).Render()
			require.NoError(t, err)

			assert.Equal(t, wantOut, gotOut, "rendering via io.Reader should match rendering via Open() for the same bytes")
		})
	}
}

// TestSmallSizeOutput_HasReasonableShape sanity-checks a 20x20 render produces a multi-line grid, not a degenerate output.
func TestSmallSizeOutput_HasReasonableShape(t *testing.T) {
	path := fixturePath(t, defaultFixture)

	img, err := termimg.Open(path)
	require.NoError(t, err)

	out, err := img.Protocol(termimg.Halfblocks).Width(20).Height(20).Scale(termimg.ScaleFit).Render()
	require.NoError(t, err)

	lines := strings.Count(out, "\n")
	assert.GreaterOrEqual(t, lines, 5, "expected multiple output lines for a 20x20 render, got %d", lines)
	assert.LessOrEqual(t, lines, 25, "expected roughly height-scaled output lines, got %d", lines)
}

// TestInvalidImageData_ReturnsError confirms malformed bytes error rather than panic.
func TestInvalidImageData_ReturnsError(t *testing.T) {
	_, err := termimg.From(strings.NewReader("not an image"))
	assert.Error(t, err)
}

// TestUnsupportedProtocolValue confirms Protocol(Unsupported) fails rather than silently falling back.
func TestUnsupportedProtocolValue(t *testing.T) {
	path := fixturePath(t, defaultFixture)

	img, err := termimg.Open(path)
	require.NoError(t, err)

	_, err = img.Protocol(termimg.Unsupported).Render()
	assert.Error(t, err, "Protocol(Unsupported) is expected to fail rather than silently render")
}
