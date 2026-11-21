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

// Option and the With functions give tests the options New takes, which
// only tests use: fake servers, stores in memory, a known account.
type Option = option

var (
	WithBaseURL        = withBaseURL
	WithHTTPClient     = withHTTPClient
	WithSessionStore   = withSessionStore
	WithHouseholdStore = withHouseholdStore
	WithAccount        = withAccount
	WithClock          = withClock
)

// DefaultSessionPath is the file New keeps the session in by default.
var DefaultSessionPath = defaultSessionPath
