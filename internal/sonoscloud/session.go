package sonoscloud

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// sessionCookie is the web app's NextAuth session. A large session is split
// into numbered parts: sessionCookie.0, .1 and on.
const sessionCookie = "__Secure-next-auth.session-token"

// Session is the saved sign-in to the reader's Sonos account: the web app's
// session cookie, in one or more parts. Printing or logging it never shows
// a value.
type Session struct {
	Cookies []Cookie
}

// Cookie is one part of the session cookie.
type Cookie struct {
	Name    string    `json:"name"`
	Value   string    `json:"value"`
	Expires time.Time `json:"expires"`
}

// SessionStore keeps the session between runs. An empty Session from Load
// means there is none.
type SessionStore interface {
	Load() (Session, error)
	Save(Session) error
	Clear() error
}

// memorySession keeps the session for the client's life only.
type memorySession struct {
	session Session

	mu sync.Mutex // guards session
}

var (
	_ fmt.Stringer   = Cookie{}
	_ fmt.GoStringer = Cookie{}
	_ slog.LogValuer = Cookie{}
	_ SessionStore   = &memorySession{}
)

// Expires is when the first part of the session runs out, and zero when no
// part says. The server moves it on as the session is used.
func (s Session) Expires() time.Time {
	var first time.Time
	for _, c := range s.Cookies {
		if !c.Expires.IsZero() && (first.IsZero() || c.Expires.Before(first)) {
			first = c.Expires
		}
	}
	return first
}

// Valid reports whether the session holds a cookie and none of its parts has
// run out at now. A session with no expiry is left to the server to judge.
func (s Session) Valid(now time.Time) bool {
	expires := s.Expires()
	return len(s.Cookies) > 0 && (expires.IsZero() || now.Before(expires))
}

func (c Cookie) String() string { return c.Name + "=…" }

func (c Cookie) GoString() string {
	return fmt.Sprintf("sonoscloud.Cookie{Name: %q, Value: …}", c.Name)
}

func (c Cookie) LogValue() slog.Value { return slog.StringValue(c.String()) }

func (m *memorySession) Load() (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.session, nil
}

func (m *memorySession) Save(s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = s
	return nil
}

func (m *memorySession) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.session = Session{}
	return nil
}

// isSessionCookie reports whether a cookie name is the session or one of its
// numbered parts.
func isSessionCookie(name string) bool {
	if name == sessionCookie {
		return true
	}
	rest, ok := strings.CutPrefix(name, sessionCookie+".")
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
