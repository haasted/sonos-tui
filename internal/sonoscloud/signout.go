package sonoscloud

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

const (
	// logoutPath is the web app's sign-out. It answers with a redirect to the
	// OAuth server's sign-out, which ends the session there too.
	logoutPath = "/api/auth/logout"

	// rscAlphabet and rscLength give the _rsc value the web app's router
	// adds to its fetches, as measured.
	rscAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rscLength   = 16
)

var (
	// ErrServerNotTold says Sonos could not be told to end the session.
	ErrServerNotTold = errors.New("sonoscloud: Sonos was not told about the sign-out")

	// ErrNotDeleted says the saved session or household could not be deleted.
	ErrNotDeleted = errors.New("sonoscloud: what was kept for the session was not deleted")
)

// SignOut deletes the session and all that is kept for it, then asks Sonos
// to end the session, so a copy of the cookie stops working. Unless the
// error holds ErrNotDeleted, the local data is gone.
func (c *Client) SignOut(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	signedIn := c.jar.current().Valid(c.now())
	forgot := c.forget()
	var told error
	if signedIn {
		if err := c.tellSignOut(ctx); err != nil {
			told = fmt.Errorf("%w: %w", ErrServerNotTold, err)
		}
	}
	c.jar.reset()
	return errors.Join(forgot, told)
}

// forget deletes what is kept for the session: the saved session, the
// household, the accounts and the opened pages. The jar keeps the cookie
// until Sonos is told.
func (c *Client) forget() error {
	c.accounts = map[int]int{}
	c.artists.clear()
	c.albums.clear()
	if err := errors.Join(c.store.Clear(), c.households.ClearHousehold()); err != nil {
		return fmt.Errorf("%w: %w", ErrNotDeleted, err)
	}
	return nil
}

// tellSignOut does what the web app's sign-out does: the web app's own
// sign-out, then the OAuth server's, which the first redirects to.
func (c *Client) tellSignOut(ctx context.Context) error {
	target := c.base.JoinPath(logoutPath)
	target.RawQuery = url.Values{"_rsc": {rscValue()}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("RSC", "1")
	req.Header.Set("Referer", c.base.JoinPath(appPage).String())
	resp, _, err := c.http.withoutRedirects().fetch(req)
	if err != nil {
		return err
	}
	logSignOutStep("web app", resp)
	oauth, err := c.oauthSignOut(resp)
	if err != nil {
		return err
	}

	// The router loads the OAuth server's page in place of the app. The
	// browser sends only the origin as Referer to another host.
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, oauth.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Referer", origin(c.base)+"/")
	resp, _, err = c.http.loggingRedirects(c.signOutHost).page(req)
	if err != nil {
		return err
	}
	logSignOutStep("last", resp)
	if resp.StatusCode >= 400 {
		return &StatusError{Method: http.MethodGet, Path: oauth.Path, Status: resp.StatusCode}
	}
	return nil
}

// oauthSignOut is where the web app's sign-out redirects to. Only a Sonos
// host, or the web app's own, is followed: the address carries the token.
func (c *Client) oauthSignOut(resp *http.Response) (*url.URL, error) {
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return nil, &StatusError{Method: http.MethodGet, Path: logoutPath, Status: resp.StatusCode}
	}
	next, err := resp.Location()
	if err != nil {
		return nil, errors.New("sonoscloud: the sign-out did not say where to go next")
	}
	if !c.signOutHost(next) {
		return nil, fmt.Errorf("sonoscloud: the sign-out went to %s, not to Sonos", next.Host)
	}
	return next, nil
}

// signOutHost reports whether the sign-out may go to u: over HTTPS, to a
// Sonos host or the web app's own.
func (c *Client) signOutHost(u *url.URL) bool {
	return u.Scheme == "https" && (strings.HasSuffix(u.Hostname(), ".sonos.com") || u.Host == c.base.Host)
}

// withoutRedirects is the browser, giving a redirect back instead of
// following it.
func (b browser) withoutRedirects() browser {
	hc := *b.Client
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return browser{&hc}
}

// loggingRedirects is the browser, logging each redirect it follows as a
// step of the sign-out, and following it only where allowed says.
func (b browser) loggingRedirects(allowed func(*url.URL) bool) browser {
	hc := *b.Client
	hc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		logSignOutStep("redirected", req.Response)
		switch {
		case len(via) >= 10:
			return errors.New("sonoscloud: the sign-out redirected 10 times")
		case !allowed(req.URL):
			return fmt.Errorf("sonoscloud: the sign-out went to %s, not to Sonos", req.URL.Host)
		}
		return nil
	}
	return browser{&hc}
}

// logSignOutStep logs one request of the sign-out and its answer, to compare
// with the browser's. Cookie values and query values, which hold the token,
// are not shown.
func logSignOutStep(step string, resp *http.Response) {
	var cookies []string
	for _, c := range resp.Cookies() {
		cookies = append(cookies, fmt.Sprintf("%s (max-age %d, expires %s)", c.Name, c.MaxAge, c.Expires.Format(time.RFC3339)))
	}
	slog.Debug("sonoscloud: sign-out", "step", step,
		"request", safeURL(resp.Request.URL), "sent", safeHeaders(resp.Request.Header),
		"status", resp.StatusCode, "answered", safeHeaders(resp.Header), "setCookies", cookies)
}

// safeHeaders is headers as text, with the cookies' names only and an
// address's query names only.
func safeHeaders(headers http.Header) string {
	var lines []string
	for name, values := range headers {
		value := strings.Join(values, ", ")
		switch name {
		case "Cookie":
			value = strings.Join(cookieNames(values), ", ")
		case "Set-Cookie":
			continue // logged by name with its expiry
		case "Location", "Referer":
			if u, err := url.Parse(value); err == nil {
				value = safeURL(u)
			}
		}
		lines = append(lines, name+": "+value)
	}
	slices.Sort(lines)
	return strings.Join(lines, " | ")
}

// cookieNames is the names in Cookie header values.
func cookieNames(values []string) []string {
	var names []string
	for _, v := range values {
		for part := range strings.SplitSeq(v, ";") {
			name, _, _ := strings.Cut(strings.TrimSpace(part), "=")
			names = append(names, name+"=…")
		}
	}
	return names
}

// safeURL is u with each query value replaced by an ellipsis.
func safeURL(u *url.URL) string {
	safe := *u
	query := u.Query()
	for name := range query {
		query[name] = []string{"…"}
	}
	safe.RawQuery = query.Encode()
	return safe.String()
}

// rscValue is a new _rsc value. It only has to differ between requests.
func rscValue() string {
	b := make([]byte, rscLength)
	for i := range b {
		b[i] = rscAlphabet[rand.IntN(len(rscAlphabet))]
	}
	return string(b)
}
