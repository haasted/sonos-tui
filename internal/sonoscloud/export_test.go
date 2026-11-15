package sonoscloud

import "testing"

// SetControlAPI points the household question at address for the length of
// one test. Tests that call it cannot run in parallel.
func SetControlAPI(t *testing.T, address string) {
	t.Helper()
	previous := controlAPI
	controlAPI = address
	t.Cleanup(func() { controlAPI = previous })
}
