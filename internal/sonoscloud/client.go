// Package sonoscloud talks to the Sonos cloud as the Sonos web app
// (play.sonos.com) does, with the reader's own session. Every request copies,
// as far as it can, what Google Chrome on a Mac sends: "the browser" below.
package sonoscloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the web app, whose backend passes requests on to
	// Sonos's own API.
	DefaultBaseURL = "https://play.sonos.com"

	// browserAgent and the client hints name the browser's version. Update
	// them together.
	browserAgent  = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
	browserHints  = `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`
	browserLocale = "en-US,en;q=0.9"

	// appPage is the page the web app's own requests come from.
	appPage = "/en-us/web-app"

	// pageAccept is what the browser accepts when it loads a page.
	pageAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8"

	// maxAnswer is the largest answer read, far above any measured one.
	maxAnswer = 32 << 20
)

// ErrSignedOut says there is no valid session: none was saved, it ran out,
// or the server no longer takes it. The reader has to sign in.
var ErrSignedOut = errors.New("sonoscloud: not signed in")

// Client makes requests to the Sonos cloud with the saved session, one
// at a time as the web app's page would, and saves the session again each
// time the server renews it. It is safe to use from several goroutines.
type Client struct {
	base       *url.URL
	http       browser
	jar        *sessionJar
	store      SessionStore
	households HouseholdStore // where the household is kept; in memory unless given
	accounts   map[int]int    // the household's account on each service, as last seen
	now        func() time.Time

	mu sync.Mutex // one request at a time, as a page does; guards accounts too
}

// browser is an HTTP client that sends requests as the browser does.
type browser struct{ *http.Client }

// option changes how New builds a Client. Only tests use one today.
type option func(*Client)

// StatusError is an answer that was not a success. It holds no part of the
// request's headers or the answer's body, so it is safe to log.
type StatusError struct {
	Method string
	Path   string
	Status int
}

var _ error = &StatusError{}

// SessionInfo is what the web app says about the session.
type SessionInfo struct {
	Expires time.Time // when the server ends the session unless it is used
}

// sessionJar is a cookie jar that also sees the session cookie every time
// the server sets it, with its expiry, which a cookiejar.Jar does not give
// back. It sees cookies set on a redirect too.
type sessionJar struct {
	inner   *cookiejar.Jar
	host    string
	now     func() time.Time
	session map[string]Cookie
	changed bool

	mu sync.Mutex // guards session and changed
}

var _ http.CookieJar = (*sessionJar)(nil)

// withBaseURL points the client somewhere other than the web app, such as a
// test server.
func withBaseURL(base string) option {
	return func(c *Client) {
		if u, err := url.Parse(base); err == nil {
			c.base = u
		}
	}
}

// withHTTPClient sends requests through hc, for its transport and timeout.
// The client puts its own cookie jar on a copy and leaves hc as it is.
func withHTTPClient(hc *http.Client) option {
	return func(c *Client) { c.http = browser{hc} }
}

// withSessionStore keeps the session in store instead of the default file.
func withSessionStore(store SessionStore) option {
	return func(c *Client) { c.store = store }
}

// withAccount says which account the household has on a service before an
// answer has said. The client learns the account itself.
func withAccount(service, account int) option {
	return func(c *Client) { c.accounts[service] = account }
}

// New makes a client with the saved session: by default the file that the
// sign-in writes, ~/.config/sonos-tui/web-session. A missing session is not
// an error: requests then answer ErrSignedOut.
func New(opts ...option) (*Client, error) {
	c := &Client{now: time.Now, http: browser{&http.Client{Timeout: 30 * time.Second}}, accounts: map[int]int{}}
	c.base, _ = url.Parse(DefaultBaseURL)
	for _, opt := range opts {
		opt(c)
	}
	if c.store == nil {
		path, err := defaultSessionPath()
		if err != nil {
			return nil, err
		}
		c.store = FileStore{Path: path}
	}
	store := c.store

	saved, err := store.Load()
	if err != nil {
		return nil, err
	}
	inner, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	if c.households == nil {
		c.households = &memoryHouseholds{}
	}
	c.jar = &sessionJar{inner: inner, host: c.base.Host, now: c.now, session: map[string]Cookie{}}
	c.jar.load(c.base, saved)

	hc := *c.http.Client
	hc.Jar = c.jar
	c.http = browser{&hc}
	return c, nil
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("sonoscloud: %s %s answered %d", e.Method, e.Path, e.Status)
}

// Session asks the web app about the session, as its page does when it
// opens. An answer with no session is ErrSignedOut.
func (c *Client) Session(ctx context.Context) (SessionInfo, error) {
	body, err := c.get(ctx, "/api/auth/session", nil)
	if err != nil {
		return SessionInfo{}, err
	}
	var answer struct {
		Expires time.Time `json:"expires"`
	}
	if err := json.Unmarshal(body, &answer); err != nil {
		return SessionInfo{}, fmt.Errorf("sonoscloud: reading the session: %w", err)
	}
	if answer.Expires.IsZero() {
		return SessionInfo{}, ErrSignedOut
	}
	return SessionInfo{Expires: answer.Expires}, nil
}

// get sends a GET that needs the session, and returns the answer's body. It
// answers ErrSignedOut without a request when no valid session is saved.
func (c *Client) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.jar.current().Valid(c.now()) {
		return nil, ErrSignedOut
	}
	target := c.base.JoinPath(path)
	target.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", c.base.JoinPath(appPage).String())

	resp, body, err := c.http.fetch(req)
	if saveErr := c.saveIfRenewed(); saveErr != nil {
		slog.Warn("sonoscloud: saving the renewed session", "err", saveErr)
	}
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrSignedOut
	case resp.StatusCode >= 300:
		return nil, &StatusError{Method: http.MethodGet, Path: path, Status: resp.StatusCode}
	}
	return body, nil
}

// fetch sends req as the browser sends a fetch from a page's script, and
// page as it loads a page. Both answer whatever the status, with the body
// read and closed; the response's Request holds the address after redirects.
func (b browser) fetch(req *http.Request) (*http.Response, []byte, error) {
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	return b.send(req)
}

func (b browser) page(req *http.Request) (*http.Response, []byte, error) {
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", pageAccept)
	}
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	return b.send(req)
}

// send adds the headers the browser puts on every request and sends req. Go
// writes headers in its own order and its TLS handshake is not the browser's;
// those differences remain. Errors name the path only, never the query.
func (b browser) send(req *http.Request) (*http.Response, []byte, error) {
	req.Header.Set("User-Agent", browserAgent)
	req.Header.Set("Accept-Language", browserLocale)
	req.Header.Set("Sec-Ch-Ua", browserHints)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"macOS"`)
	req.Header.Set("Sec-Fetch-Site", fetchSite(req))

	resp, err := b.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("sonoscloud: %s %s: %w", req.Method, req.URL.Path, unwrapURLError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	logged := []any{"method", req.Method, "host", req.URL.Host, "path", req.URL.Path, "status", resp.StatusCode}
	// A change of the content API's version is when it may stop answering as
	// measured, so each request's line shows it.
	if version := resp.Header.Get("X-Sonos-Api-Version"); version != "" {
		logged = append(logged, "apiVersion", version)
	}
	slog.Debug("sonoscloud", logged...)
	if err != nil {
		return nil, nil, fmt.Errorf("sonoscloud: reading %s %s: %w", req.Method, req.URL.Path, err)
	}
	return resp, body, nil
}

// fetchSite is the browser's Sec-Fetch-Site: none when the reader started
// the request, same-origin when it comes from a page of the same origin, and
// cross-site otherwise.
func fetchSite(req *http.Request) string {
	from, err := url.Parse(req.Header.Get("Referer"))
	switch {
	case req.Header.Get("Referer") == "" || err != nil:
		return "none"
	case from.Scheme == req.URL.Scheme && from.Host == req.URL.Host:
		return "same-origin"
	}
	return "cross-site"
}

// account is the household's account on service, as last seen, or 0, which
// the content API also takes.
func (c *Client) account(service int) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.accounts[service]
}

// learnAccount keeps the account an answer says for service, so the next
// request names the account the web app would.
func (c *Client) learnAccount(service int, accountID string) {
	account, err := strconv.Atoi(accountID)
	if err != nil || account <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accounts[service] = account
}

// wrongHousehold forgets the kept household, which the cloud has just said
// it does not know, and says so as ErrWrongHousehold.
func (c *Client) wrongHousehold() error {
	if err := c.households.ClearHousehold(); err != nil {
		slog.Warn("sonoscloud: forgetting the household", "err", err)
	}
	return ErrWrongHousehold
}

// saveIfRenewed saves the session when the server has set it since the last
// save, so a renewal is not lost when the program ends.
func (c *Client) saveIfRenewed() error {
	session, changed := c.jar.takeChange()
	if !changed {
		return nil
	}
	if len(session.Cookies) == 0 {
		return c.store.Clear()
	}
	return c.store.Save(session)
}

// unwrapURLError drops the *url.Error around a transport error: it repeats
// the whole address, query and all.
func unwrapURLError(err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return urlErr.Err
	}
	return err
}

// load puts a saved session in the jar for the web app's host.
func (j *sessionJar) load(base *url.URL, s Session) {
	var cookies []*http.Cookie
	for _, c := range s.Cookies {
		j.session[c.Name] = c
		cookies = append(cookies, &http.Cookie{
			Name: c.Name, Value: c.Value, Path: "/", Expires: c.Expires, Secure: true, HttpOnly: true,
		})
	}
	j.inner.SetCookies(base, cookies)
}

func (j *sessionJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.inner.SetCookies(u, cookies)
	if u.Host != j.host {
		return
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	now := j.now()
	for _, c := range cookies {
		if !isSessionCookie(c.Name) {
			continue
		}
		j.changed = true
		expires := c.Expires
		if c.MaxAge > 0 {
			expires = now.Add(time.Duration(c.MaxAge) * time.Second)
		}
		if c.MaxAge < 0 || c.Value == "" || (!expires.IsZero() && !expires.After(now)) {
			delete(j.session, c.Name)
			continue
		}
		j.session[c.Name] = Cookie{Name: c.Name, Value: c.Value, Expires: expires}
	}
}

func (j *sessionJar) Cookies(u *url.URL) []*http.Cookie { return j.inner.Cookies(u) }

// current is the session as the jar holds it now.
func (j *sessionJar) current() Session {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.snapshot()
}

// takeChange is the session, and whether the server set it since the last
// call.
func (j *sessionJar) takeChange() (Session, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	changed := j.changed
	j.changed = false
	return j.snapshot(), changed
}

func (j *sessionJar) snapshot() Session {
	var s Session
	for _, c := range j.session {
		s.Cookies = append(s.Cookies, c)
	}
	slices.SortFunc(s.Cookies, func(a, b Cookie) int { return strings.Compare(a.Name, b.Name) })
	return s
}
