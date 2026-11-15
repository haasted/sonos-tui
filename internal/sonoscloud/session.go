package sonoscloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

// FileStore keeps the session in a file that only the reader can read.
type FileStore struct {
	Path string
}

var (
	_ fmt.Stringer   = Cookie{}
	_ fmt.GoStringer = Cookie{}
	_ slog.LogValuer = Cookie{}
	_ SessionStore   = FileStore{}
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

func (f FileStore) Load() (Session, error) {
	data, err := os.ReadFile(f.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Session{}, nil
	}
	if err != nil {
		return Session{}, err
	}
	var cookies []Cookie
	if err := json.Unmarshal(data, &cookies); err != nil {
		return Session{}, fmt.Errorf("reading the session in %s: %w", f.Path, err)
	}
	return Session{Cookies: cookies}, nil
}

// Save replaces the file through a rename, so a crash cannot leave half a
// session behind.
func (f FileStore) Save(s Session) error {
	data, err := json.Marshal(s.Cookies)
	if err != nil {
		return err
	}
	dir := filepath.Dir(f.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // fails harmlessly once renamed
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.Path)
}

func (f FileStore) Clear() error {
	if err := os.Remove(f.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
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
