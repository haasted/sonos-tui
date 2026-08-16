// Package coverart_test evaluates BourgeoisBear/rasterm as an alternative to go-termimg (sonos-9yf); test names mirror termimg_eval_test.go 1:1 for comparison.
package coverart_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"os"
	"strings"
	"testing"

	"github.com/BourgeoisBear/rasterm"
	"github.com/nfnt/resize"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeFixture decodes the shared fixture; rasterm has no Open/From helper, so callers always decode themselves.
func decodeFixture(t *testing.T) image.Image {
	t.Helper()
	f, err := os.Open(fixturePath(t))
	require.NoError(t, err)
	defer f.Close()
	img, _, err := image.Decode(f)
	require.NoError(t, err)
	return img
}

// toPaletted resizes and palettizes an image for Sixel, which has no cell-size option and needs pre-sized paletted pixels.
func toPaletted(img image.Image, w, h int) *image.Paletted {
	resized := resize.Resize(uint(w), uint(h), img, resize.Lanczos3)
	dst := image.NewPaletted(resized.Bounds(), palette.Plan9)
	draw.FloydSteinberg.Draw(dst, resized.Bounds(), resized, image.Point{})
	return dst
}

// TestRasterm_AllProtocols mirrors TestRenderFromFile_AllProtocols.
func TestRasterm_AllProtocols(t *testing.T) {
	img := decodeFixture(t)
	cols, rows, feat := squareCellSize(t, 20)

	t.Run("ITerm2", func(t *testing.T) {
		var buf bytes.Buffer
		err := rasterm.ItermWriteImageWithOptions(&buf, img, rasterm.ItermImgOpts{
			Width:  fmt.Sprintf("%d", cols),
			Height: fmt.Sprintf("%d", rows),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String())
		printPreview(t, "ITerm2", buf.String())
	})

	t.Run("Kitty", func(t *testing.T) {
		var buf bytes.Buffer
		err := rasterm.KittyWriteImage(&buf, img, rasterm.KittyImgOpts{
			DstCols: uint32(cols),
			DstRows: uint32(rows),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String())
		printPreview(t, "Kitty", buf.String())
	})

	t.Run("Sixel", func(t *testing.T) {
		// Sixel has no cell-size option — pre-resize to pixels using the detected cell size instead.
		paletted := toPaletted(img, cols*feat.FontWidth, rows*feat.FontHeight)
		var buf bytes.Buffer
		err := rasterm.SixelWriteImage(&buf, paletted)
		require.NoError(t, err)
		assert.NotEmpty(t, buf.String())
		printPreview(t, "Sixel", buf.String())
	})

	t.Run("Halfblocks", func(t *testing.T) {
		t.Skip("rasterm has no halfblock renderer — no universal fallback like go-termimg's")
	})
}

// TestRasterm_ProtocolDetection mirrors TestRenderFromFile_VisualPreview; rasterm has no single Auto entrypoint, so detection is a manual cascade.
func TestRasterm_ProtocolDetection(t *testing.T) {
	img := decodeFixture(t)
	cols, rows, feat := squareCellSize(t, 20)

	var buf bytes.Buffer
	var label string
	var err error

	switch {
	case rasterm.IsKittyCapable():
		label = "Kitty"
		err = rasterm.KittyWriteImage(&buf, img, rasterm.KittyImgOpts{DstCols: uint32(cols), DstRows: uint32(rows)})
	case rasterm.IsItermCapable():
		label = "ITerm2"
		err = rasterm.ItermWriteImageWithOptions(&buf, img, rasterm.ItermImgOpts{Width: fmt.Sprintf("%d", cols), Height: fmt.Sprintf("%d", rows)})
	default:
		if ok, _ := rasterm.IsSixelCapable(); ok {
			label = "Sixel"
			paletted := toPaletted(img, cols*feat.FontWidth, rows*feat.FontHeight)
			err = rasterm.SixelWriteImage(&buf, paletted)
		} else {
			t.Skip("no protocol detected and rasterm has no universal fallback (unlike go-termimg's Halfblocks)")
		}
	}

	require.NoError(t, err)
	assert.NotEmpty(t, buf.String())
	printPreview(t, "Detected: "+label, buf.String())
}

// TestRasterm_SquareAspect mirrors TestRenderFromFile_SquareAspect, reusing squareCellSize; Kitty is used since DstCols/DstRows map directly onto cell counts.
func TestRasterm_SquareAspect(t *testing.T) {
	img := decodeFixture(t)
	cols, rows, feat := squareCellSize(t, 20)

	var buf bytes.Buffer
	err := rasterm.KittyWriteImage(&buf, img, rasterm.KittyImgOpts{DstCols: uint32(cols), DstRows: uint32(rows)})
	require.NoError(t, err)
	assert.NotEmpty(t, buf.String())

	label := fmt.Sprintf("Kitty, cell %dx%dpx -> %dx%d cells", feat.FontWidth, feat.FontHeight, cols, rows)
	printPreview(t, label, buf.String())
}

// TestRasterm_ReaderVsFile mirrors TestRenderFromReader_HTTPStylePath: decoding fixture bytes via a file vs an in-memory reader should encode identically.
func TestRasterm_ReaderVsFile(t *testing.T) {
	cols, rows, _ := squareCellSize(t, 20)

	f, err := os.Open(fixturePath(t))
	require.NoError(t, err)
	defer f.Close()
	fileImg, _, err := image.Decode(f)
	require.NoError(t, err)

	data, err := os.ReadFile(fixturePath(t))
	require.NoError(t, err)
	readerImg, _, err := image.Decode(bytes.NewReader(data))
	require.NoError(t, err)

	opts := rasterm.KittyImgOpts{DstCols: uint32(cols), DstRows: uint32(rows)}
	var wantBuf, gotBuf bytes.Buffer
	require.NoError(t, rasterm.KittyWriteImage(&wantBuf, fileImg, opts))
	require.NoError(t, rasterm.KittyWriteImage(&gotBuf, readerImg, opts))

	assert.Equal(t, wantBuf.String(), gotBuf.String())
}

// TestRasterm_SmallSizeShape mirrors TestSmallSizeOutput_HasReasonableShape; rasterm output is one escape sequence, not a text grid, so shape means header/footer framing.
func TestRasterm_SmallSizeShape(t *testing.T) {
	img := decodeFixture(t)
	cols, rows, _ := squareCellSize(t, 20)

	var buf bytes.Buffer
	err := rasterm.KittyWriteImage(&buf, img, rasterm.KittyImgOpts{DstCols: uint32(cols), DstRows: uint32(rows)})
	require.NoError(t, err)

	out := buf.String()
	assert.True(t, strings.HasPrefix(out, rasterm.KITTY_IMG_HDR), "expected Kitty header prefix")
	assert.True(t, strings.HasSuffix(out, rasterm.KITTY_IMG_FTR), "expected Kitty footer suffix")
	assert.Greater(t, len(out), 100, "expected a non-trivial base64 payload")
}

// TestRasterm_InvalidImageData mirrors TestInvalidImageData_ReturnsError; unlike go-termimg's From(), rasterm has no decode step of its own, so this is the stdlib decode every caller must guard.
func TestRasterm_InvalidImageData(t *testing.T) {
	_, _, err := image.Decode(strings.NewReader("not an image"))
	assert.Error(t, err)
}

// TestRasterm_DegenerateInput mirrors TestUnsupportedProtocolValue; unlike go-termimg's explicit error, rasterm's Sixel writer silently no-ops on an empty palette.
func TestRasterm_DegenerateInput(t *testing.T) {
	degenerate := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{})
	var buf bytes.Buffer
	err := rasterm.SixelWriteImage(&buf, degenerate)
	assert.NoError(t, err, "rasterm silently no-ops rather than erroring")
	assert.Empty(t, buf.String())
}
