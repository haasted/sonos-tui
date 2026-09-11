//go:build !debug

package main

// debugUISize is a no-op in release builds — -ui-size doesn't exist at all
// outside a `-tags debug` build (see uisize_debug.go).
func debugUISize() (width, height int, ok bool) {
	return 0, 0, false
}
