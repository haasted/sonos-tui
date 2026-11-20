package sonoscloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// oktaBadCredentials is Okta's error code for a wrong username or password.
const oktaBadCredentials = "E0000004"

var (
	// ErrBadCredentials says Okta did not accept the username and password.
	ErrBadCredentials = errors.New("sonoscloud: wrong username or password")

	// ErrFlowChanged says a step of the sign-in did not answer as measured,
	// so the web app's sign-in has probably changed.
	ErrFlowChanged = errors.New("sonoscloud: the sign-in is not the one this program knows")
)

// stateTokenPattern finds Okta's state token in its sign-in page's script.
var stateTokenPattern = regexp.MustCompile(`"stateToken":"((?:[^"\\]|\\.)*)"`)

// signIn is one sign-in, made with a cookie jar of its own so a failed
// attempt leaves the client's session as it was.
type signIn struct {
	c    *Client
	http browser
}

// oktaAnswer is the part of Okta's /api/v1/authn answer the sign-in reads.
type oktaAnswer struct {
	Status       string `json:"status"`
	SessionToken string `json:"sessionToken"`
	ErrorCode    string `json:"errorCode"`
	Links        struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
}

// SignIn signs in to the Sonos web app with the account's username and
// password, as its Okta sign-in page does, and saves the new session in
// place of the old one. Neither the username nor the password is kept.
func (c *Client) SignIn(ctx context.Context, username, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	inner, err := cookiejar.New(nil)
	if err != nil {
		return err
	}
	jar := &sessionJar{inner: inner, host: c.base.Host, now: c.now, session: map[string]Cookie{}}
	hc := *c.http.Client
	hc.Jar = jar

	if err := (signIn{c: c, http: browser{&hc}}).run(ctx, username, password); err != nil {
		return err
	}
	session, _ := jar.takeChange()
	if !session.Valid(c.now()) {
		return fmt.Errorf("%w: the web app set no session", ErrFlowChanged)
	}
	if err := c.store.Save(session); err != nil {
		return err
	}
	c.jar, c.http = jar, browser{&hc}
	return nil
}

// run goes through the steps the web app's page and Okta's take, in their
// order. SONOS-CLOUD-NOTES.md describes each.
func (s signIn) run(ctx context.Context, username, password string) error {
	base := s.c.base
	landing, err := s.loadPage(ctx, base.JoinPath("/"), "")
	if err != nil {
		return err
	}
	loginPage := landing.String()

	var csrf struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := s.fetchJSON(ctx, http.MethodGet, base.JoinPath("/api/auth/csrf"), loginPage, nil, &csrf); err != nil {
		return err
	}
	if csrf.CSRFToken == "" {
		return fmt.Errorf("%w: no CSRF token", ErrFlowChanged)
	}

	form := url.Values{"csrfToken": {csrf.CSRFToken}, "callbackUrl": {base.JoinPath("/").String()}, "json": {"true"}}
	var start struct {
		URL string `json:"url"`
	}
	if err := s.fetchJSON(ctx, http.MethodPost, base.JoinPath("/api/auth/signin/okta"), loginPage, form, &start); err != nil {
		return err
	}
	authorize, err := url.Parse(start.URL)
	if err != nil || authorize.Host == "" {
		return fmt.Errorf("%w: no Okta address", ErrFlowChanged)
	}

	oktaPage, body, err := s.loadPageBody(ctx, authorize, loginPage)
	if err != nil {
		return err
	}
	stateToken, ok := findStateToken(body)
	if !ok {
		return fmt.Errorf("%w: no state token on Okta's page", ErrFlowChanged)
	}

	next, err := s.authn(ctx, oktaPage, authorize, username, password, stateToken)
	if err != nil {
		return err
	}
	landed, err := s.loadPage(ctx, next, oktaPage.String())
	if err != nil {
		return err
	}
	if landed.Host != base.Host {
		return fmt.Errorf("%w: the sign-in ended away from the web app", ErrFlowChanged)
	}
	return nil
}

// authn sends the username and password to Okta from its own page, and
// answers the address that carries the sign-in back to the web app.
func (s signIn) authn(ctx context.Context, oktaPage, authorize *url.URL, username, password, stateToken string) (*url.URL, error) {
	body, err := json.Marshal(map[string]any{
		"username":   username,
		"password":   password,
		"stateToken": stateToken,
		"options":    map[string]bool{"warnBeforePasswordExpired": true, "multiOptionalFactorEnroll": true},
	})
	if err != nil {
		return nil, err
	}
	okta := &url.URL{Scheme: oktaPage.Scheme, Host: oktaPage.Host}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, okta.JoinPath("/api/v1/authn").String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", oktaPage.String())
	req.Header.Set("Origin", origin(okta))
	resp, answer, err := s.http.fetch(req)
	if err != nil {
		return nil, err
	}

	var got oktaAnswer
	if err := json.Unmarshal(answer, &got); err != nil {
		return nil, fmt.Errorf("%w: Okta's answer is not JSON (%d)", ErrFlowChanged, resp.StatusCode)
	}
	switch {
	case got.ErrorCode == oktaBadCredentials:
		return nil, ErrBadCredentials
	case got.ErrorCode != "":
		return nil, fmt.Errorf("sonoscloud: Okta refused the sign-in (%s)", got.ErrorCode)
	case got.Status != "SUCCESS":
		return nil, fmt.Errorf("sonoscloud: Okta did not finish the sign-in (%s)", got.Status)
	case got.Links.Next.Href != "":
		return url.Parse(got.Links.Next.Href)
	case got.SessionToken != "":
		redirect := okta.JoinPath("/login/sessionCookieRedirect")
		redirect.RawQuery = url.Values{"token": {got.SessionToken}, "redirectUrl": {authorize.String()}}.Encode()
		return redirect, nil
	}
	return nil, fmt.Errorf("%w: Okta's success has nowhere to go", ErrFlowChanged)
}

// loadPage loads a page as the browser does when the reader follows a link from
// referer, and answers the address it ended at after redirects.
func (s signIn) loadPage(ctx context.Context, target *url.URL, referer string) (*url.URL, error) {
	final, _, err := s.loadPageBody(ctx, target, referer)
	return final, err
}

// loadPageBody is loadPage with the page itself. An answer that is not a
// success is an error.
func (s signIn) loadPageBody(ctx context.Context, target *url.URL, referer string) (*url.URL, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, nil, err
	}
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, body, err := s.http.page(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, nil, &StatusError{Method: http.MethodGet, Path: target.Path, Status: resp.StatusCode}
	}
	return resp.Request.URL, body, nil
}

// fetchJSON sends a fetch from the web app's page at referer, with form as
// its body when there is one, and reads the JSON answer into into.
func (s signIn) fetchJSON(ctx context.Context, method string, target *url.URL, referer string, form url.Values, into any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", referer)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", origin(target))
	}
	resp, answer, err := s.http.fetch(req)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return &StatusError{Method: method, Path: target.Path, Status: resp.StatusCode}
	}
	if err := json.Unmarshal(answer, into); err != nil {
		return fmt.Errorf("%w: %s %s is not JSON", ErrFlowChanged, method, target.Path)
	}
	return nil
}

// findStateToken reads the state token from Okta's page, where its script
// writes it with JavaScript's \xNN and \uNNNN escapes.
func findStateToken(page []byte) (string, bool) {
	match := stateTokenPattern.FindSubmatch(page)
	if match == nil {
		return "", false
	}
	token := unescapeJS(string(match[1]))
	return token, token != ""
}

// unescapeJS undoes the escapes of a JavaScript string literal's body.
func unescapeJS(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		width := map[byte]int{'x': 2, 'u': 4}[s[i+1]]
		if width > 0 && i+2+width <= len(s) {
			if n, err := strconv.ParseUint(s[i+2:i+2+width], 16, 32); err == nil {
				out.WriteRune(rune(n))
				i += 1 + width
				continue
			}
		}
		out.WriteByte(s[i+1])
		i++
	}
	return out.String()
}

// origin is the scheme and host of an address, as an Origin header names it.
func origin(u *url.URL) string { return u.Scheme + "://" + u.Host }
