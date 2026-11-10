// Command sonosweb signs in to the Sonos web app, play.sonos.com, as an
// ordinary browser would, to find out if a program can get a web session of
// its own. It asks for the username and password when the sign-in needs them.
//
// The session is kept in a file outside the repository (mode 0600), so the
// next run signs in only when the session has run out. No password, token or
// cookie value is printed. Nothing here is production code; its findings
// belong in SONOS-NOTES.md.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/gocolly/colly/v2"
	"golang.org/x/term"
)

const (
	webApp = "https://play.sonos.com"

	// browserAgent is what a current Chrome on a Mac sends.
	browserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
		"(KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

	// sessionCookie is NextAuth's session; a large one is split into
	// numbered parts, sessionCookie.0, .1 and on.
	sessionCookie = "__Secure-next-auth.session-token"

	// controlAPI is Sonos's Control API websocket, which the web app opens
	// with a ticket from its own backend.
	controlAPI = "wss://api.ws.sonos.com/websocket"
)

// stateTokenPattern finds the Okta state token in the sign-in page's script.
var stateTokenPattern = regexp.MustCompile(`"stateToken":"((?:[^"\\]|\\.)*)"`)

// browser is a Colly collector used one request at a time, as a script
// would: each call waits for its answer and returns it.
type browser struct {
	c    *colly.Collector
	last *colly.Response // the answer to the request in flight
}

// authnAnswer is the part of Okta's /api/v1/authn answer the sign-in reads.
type authnAnswer struct {
	Status       string `json:"status"`
	SessionToken string `json:"sessionToken"`
	ErrorCode    string `json:"errorCode"`
	ErrorSummary string `json:"errorSummary"`
	Links        struct {
		Next struct {
			Href string `json:"href"`
		} `json:"next"`
	} `json:"_links"`
	Embedded struct {
		Factors []struct {
			FactorType string `json:"factorType"`
			Provider   string `json:"provider"`
		} `json:"factors"`
	} `json:"_embedded"`
}

// household is one household the Control API lists for the account.
type household struct {
	ID        string `json:"id"`
	SwVersion string `json:"swVersion"`
}

// searchOptions say what to search for, and in which service and account.
type searchOptions struct {
	query     string
	household string
	services  string // service ids, comma separated
	account   int    // with one service, searches it alone; 0 searches across them
	count     int    // how many results the search of one service asks for
}

// resolved fills in the household from the Control API when none is given.
func (opts searchOptions) resolved() (searchOptions, error) {
	if opts.household != "" {
		return opts, nil
	}
	households, err := b.households()
	if err != nil {
		return opts, err
	}
	if len(households) != 1 {
		return opts, fmt.Errorf("the account has %d households; pass -household", len(households))
	}
	opts.household = households[0].ID
	return opts, nil
}

// savedCookie is one session cookie as the session file holds it.
type savedCookie struct {
	Name    string    `json:"name"`
	Value   string    `json:"value"`
	Expires time.Time `json:"expires"`
}

func main() {
	fresh := flag.Bool("fresh", false, "sign in again, ignoring a saved session")
	var search searchOptions
	flag.StringVar(&search.query, "search", "", "search for this once signed in")
	flag.StringVar(&search.household, "household", "", "the household id; asked of the Control API when empty")
	flag.StringVar(&search.services, "services", "52231", "service ids to search, comma separated; 52231 is Apple Music")
	flag.IntVar(&search.account, "account", 0, "the account (sn= in a service URI) to search one service alone; 0 searches across services")
	flag.IntVar(&search.count, "count", 50, "how many results to ask for when searching one service alone")
	interactive := flag.Bool("interactive", false, "ask for search queries in the terminal, one after the other; needs -account")
	flag.Parse()

	if err := run(*fresh, *interactive, search); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(fresh, interactive bool, search searchOptions) error {
	if err := signedIn(fresh); err != nil {
		return err
	}
	switch {
	case interactive:
		return console(search)
	case search.query != "":
		return b.search(search)
	}
	return nil
}

// b is the one browser of the run, shared by the sign-in and the search.
var b = newBrowser()

// signedIn makes sure the browser has a session: the saved one while it
// lasts, or a new sign-in. The session is saved again either way.
func signedIn(fresh bool) error {
	path, err := sessionPath()
	if err != nil {
		return err
	}

	if !fresh {
		if loaded, err := b.loadSession(path); err != nil {
			fmt.Println("no saved session:", err)
		} else if loaded {
			fmt.Println("trying the saved session from", path)
			if ok, err := b.checkSession(); err != nil {
				return err
			} else if ok {
				return b.saveSession(path)
			}
			fmt.Println("the saved session has run out; signing in again")
		}
	}

	if err := b.signIn(); err != nil {
		return err
	}
	ok, err := b.checkSession()
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("the sign-in finished, but the web app has no session")
	}
	return b.saveSession(path)
}

func newBrowser() *browser {
	b := &browser{c: colly.NewCollector(colly.UserAgent(browserAgent), colly.AllowURLRevisit())}
	b.c.ParseHTTPErrorResponse = true // a 4xx answer is read, not dropped
	b.c.SetRedirectHandler(func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("more than 10 redirects")
		}
		fmt.Println("    ↪", where(req.URL))
		return nil
	})
	b.c.OnRequest(func(r *colly.Request) {
		if r.Headers.Get("Accept") == "" {
			r.Headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		}
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9")
		fmt.Println("  →", r.Method, where(r.URL))
	})
	b.c.OnResponse(func(r *colly.Response) {
		b.last = r
		fmt.Println("  ←", r.StatusCode, where(r.Request.URL))
	})
	b.c.OnError(func(r *colly.Response, err error) { b.last = r })
	return b
}

// signIn follows the web app's sign-in: NextAuth hands over to Sonos's Okta,
// Okta takes the username and password, and the redirects carry the code
// back to the web app, which sets its session cookie.
func (b *browser) signIn() error {
	fmt.Println("\n1. the web app, as a browser opens it")
	page, err := b.get(webApp+"/", "")
	if err != nil {
		return err
	}
	if !strings.Contains(page.Request.URL.Path, "login") {
		fmt.Println("   no sign-in asked for; landed on", where(page.Request.URL))
		return nil
	}
	fmt.Println("   the web app asks for a sign-in")

	fmt.Println("\n2. NextAuth: the CSRF token and the Okta sign-in address")
	var csrf struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := b.getJSON(webApp+"/api/auth/csrf", &csrf); err != nil {
		return err
	}
	form := url.Values{"csrfToken": {csrf.CSRFToken}, "callbackUrl": {webApp + "/"}, "json": {"true"}}
	hdr := http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Accept": {"application/json"}}
	answer, err := b.send(http.MethodPost, webApp+"/api/auth/signin/okta", []byte(form.Encode()), hdr)
	if err != nil {
		return err
	}
	var start struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(answer.Body, &start); err != nil || start.URL == "" {
		return fmt.Errorf("NextAuth gave no sign-in address (status %d)", answer.StatusCode)
	}

	fmt.Println("\n3. Okta's sign-in page")
	oktaPage, err := b.get(start.URL, "")
	if err != nil {
		return err
	}
	match := stateTokenPattern.FindSubmatch(oktaPage.Body)
	if match == nil {
		return errors.New("the Okta page has no state token; its sign-in is not the one expected")
	}
	stateToken := unescapeJS(string(match[1]))
	fmt.Println("   found the state token")

	fmt.Println("\n4. the Sonos account")
	username, password, err := askCredentials()
	if err != nil {
		return err
	}
	okta := oktaPage.Request.URL
	body, _ := json.Marshal(map[string]any{
		"username":   username,
		"password":   password,
		"stateToken": stateToken,
		"options":    map[string]bool{"warnBeforePasswordExpired": true, "multiOptionalFactorEnroll": true},
	})
	hdr = http.Header{
		"Content-Type": {"application/json"},
		"Accept":       {"application/json"},
		"Origin":       {okta.Scheme + "://" + okta.Host},
		"Referer":      {okta.String()},
	}
	answer, err = b.send(http.MethodPost, okta.Scheme+"://"+okta.Host+"/api/v1/authn", body, hdr)
	if err != nil {
		return err
	}
	var authn authnAnswer
	if err := json.Unmarshal(answer.Body, &authn); err != nil {
		return fmt.Errorf("reading Okta's answer (status %d): %w", answer.StatusCode, err)
	}
	next, err := afterAuthn(authn, okta, start.URL)
	if err != nil {
		return err
	}

	fmt.Println("\n5. back to the web app with the code")
	landed, err := b.get(next, "")
	if err != nil {
		return err
	}
	if landed.Request.URL.Host != "play.sonos.com" {
		return fmt.Errorf("the redirects ended at %s, not at the web app", where(landed.Request.URL))
	}
	return nil
}

// search asks the web app to search the services given, and prints the shape of
// the answer with short values.
func (b *browser) search(opts searchOptions) error {
	opts, err := opts.resolved()
	if err != nil {
		return err
	}
	body, err := b.fetchSearch(opts)
	if err != nil {
		return err
	}
	var answer any
	if err := json.Unmarshal(body, &answer); err != nil {
		fmt.Printf("   not JSON: %.300s\n", body)
		return nil
	}
	printShape(answer, "   ", 0)
	return nil
}

// fetchSearch sends one search and returns the answer. The whole answer also
// goes to a file outside the repository, to read at leisure.
func (b *browser) fetchSearch(opts searchOptions) ([]byte, error) {
	fmt.Println("\nsearch")
	base := fmt.Sprintf("%s/api/content/v1/households/%s", webApp, url.PathEscape(opts.household))
	q := url.Values{"query": {opts.query}, "services": {opts.services}}
	target := base + "/search?" + q.Encode()
	if opts.account != 0 {
		q = url.Values{"query": {opts.query}, "count": {strconv.Itoa(opts.count)}}
		target = fmt.Sprintf("%s/services/%s/accounts/%d/search?%s", base, opts.services, opts.account, q.Encode())
	}
	r, err := b.send(http.MethodGet, target, nil, http.Header{"Accept": {"application/json"}})
	if err != nil {
		return nil, err
	}
	out := filepath.Join(os.TempDir(), "sonosweb-search.json")
	if err := os.WriteFile(out, r.Body, 0o600); err != nil {
		return nil, err
	}
	fmt.Printf("   status %d, %d bytes, saved to %s\n", r.StatusCode, len(r.Body), out)
	if r.StatusCode >= 400 {
		return nil, fmt.Errorf("the search answered %d: %.300s", r.StatusCode, r.Body)
	}
	return r.Body, nil
}

// households asks Sonos's Control API for the account's households, as the
// web app does: a ticket from /api/mfe opens the websocket, and getHouseholds
// lists them.
func (b *browser) households() ([]household, error) {
	fmt.Println("\nthe households")
	var mfe struct {
		Ticket string `json:"ticket"`
	}
	if err := b.getJSON(webApp+"/api/mfe", &mfe); err != nil {
		return nil, err
	}
	if mfe.Ticket == "" {
		return nil, errors.New("the web app gave no ticket for the Control API")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	fmt.Println("  → WS", controlAPI)
	conn, _, err := websocket.Dial(ctx, controlAPI+"?ticket="+url.QueryEscape(mfe.Ticket), &websocket.DialOptions{
		HTTPHeader:   http.Header{"Origin": {webApp}, "User-Agent": {browserAgent}},
		Subprotocols: []string{"v1.api.smartspeaker.audio"},
	})
	if err != nil {
		return nil, fmt.Errorf("opening the Control API websocket: %w", err)
	}
	defer conn.CloseNow()

	corrID := newUUID()
	ask := []any{
		map[string]string{"command": "getHouseholds", "namespace": "households", "corrId": corrID},
		map[string]string{"connectedOnly": "true"},
	}
	if err := wsjson.Write(ctx, conn, ask); err != nil {
		return nil, fmt.Errorf("sending getHouseholds: %w", err)
	}
	for {
		var message []json.RawMessage
		if err := wsjson.Read(ctx, conn, &message); err != nil {
			return nil, fmt.Errorf("waiting for the households: %w", err)
		}
		var header struct {
			CorrID    string `json:"corrId"`
			Namespace string `json:"namespace"`
			Type      string `json:"type"`
			Success   bool   `json:"success"`
		}
		if len(message) == 0 || json.Unmarshal(message[0], &header) != nil {
			continue
		}
		fmt.Printf("  ← WS %s/%s success=%t\n", header.Namespace, header.Type, header.Success)
		if header.CorrID != corrID {
			continue
		}
		var body struct {
			Households []household `json:"households"`
		}
		if !header.Success || len(message) < 2 || json.Unmarshal(message[1], &body) != nil {
			return nil, fmt.Errorf("getHouseholds failed: %.300s", message)
		}
		for _, h := range body.Households {
			fmt.Printf("   household %s (software %s)\n", h.ID, h.SwVersion)
		}
		conn.Close(websocket.StatusNormalClosure, "")
		return body.Households, nil
	}
}

// newUUID is a random UUID, as the web app's correlation ids are.
func newUUID() string {
	var u [16]byte
	_, _ = rand.Read(u[:])
	u[6], u[8] = u[6]&0x0f|0x40, u[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:])
}

// printShape prints a JSON value as an indented tree: each key with its
// value cut short, and only the first two items of a list.
func printShape(v any, indent string, depth int) {
	if depth > 6 {
		fmt.Println(indent + "…")
		return
	}
	switch v := v.(type) {
	case map[string]any:
		for key, value := range v {
			switch value.(type) {
			case map[string]any, []any:
				fmt.Printf("%s%s:\n", indent, key)
				printShape(value, indent+"  ", depth+1)
			default:
				fmt.Printf("%s%s: %s\n", indent, key, short(value))
			}
		}
	case []any:
		fmt.Printf("%s[%d items]\n", indent, len(v))
		for i, item := range v[:min(len(v), 2)] {
			fmt.Printf("%s- %d\n", indent, i)
			printShape(item, indent+"  ", depth+1)
		}
	default:
		fmt.Println(indent + short(v))
	}
}

func short(v any) string {
	text := fmt.Sprint(v)
	if s, ok := v.(string); ok {
		text = strconv.Quote(s)
	}
	if len(text) > 70 {
		text = text[:67] + "…"
	}
	return text
}

// afterAuthn is where to go once Okta has answered the username and
// password, or why the sign-in stops there.
func afterAuthn(authn authnAnswer, okta *url.URL, authorize string) (string, error) {
	switch {
	case authn.ErrorCode != "":
		return "", fmt.Errorf("Okta refused the sign-in: %s (%s)", authn.ErrorSummary, authn.ErrorCode)
	case authn.Status == "SUCCESS" && authn.Links.Next.Href != "":
		fmt.Println("   Okta: SUCCESS, with a next step")
		return authn.Links.Next.Href, nil
	case authn.Status == "SUCCESS" && authn.SessionToken != "":
		fmt.Println("   Okta: SUCCESS, with a session token")
		q := url.Values{"token": {authn.SessionToken}, "redirectUrl": {authorize}}
		return okta.Scheme + "://" + okta.Host + "/login/sessionCookieRedirect?" + q.Encode(), nil
	case strings.HasPrefix(authn.Status, "MFA_"):
		var factors []string
		for _, f := range authn.Embedded.Factors {
			factors = append(factors, f.FactorType+"/"+f.Provider)
		}
		return "", fmt.Errorf("Okta asks for a second factor (%s): %s; not handled yet",
			authn.Status, strings.Join(factors, ", "))
	default:
		return "", fmt.Errorf("Okta answered with status %q; not handled yet", authn.Status)
	}
}

// checkSession asks the web app for its session, and says what it holds:
// when it runs out and which fields its user has, never their values.
func (b *browser) checkSession() (bool, error) {
	fmt.Println("\nthe web app's session")
	var session map[string]json.RawMessage
	if err := b.getJSON(webApp+"/api/auth/session", &session); err != nil {
		return false, err
	}
	expires, ok := session["expires"]
	if !ok {
		fmt.Println("   none")
		return false, nil
	}
	var user map[string]json.RawMessage
	_ = json.Unmarshal(session["user"], &user)
	var fields []string
	for field := range user {
		fields = append(fields, field)
	}
	fmt.Printf("   signed in; expires %s; the user has %s\n", expires, strings.Join(fields, ", "))
	var others []string
	for key := range session {
		if key != "expires" && key != "user" {
			others = append(others, key)
		}
	}
	if len(others) > 0 {
		fmt.Println("   the session also holds:", strings.Join(others, ", "))
	}
	// Times and status words, not secrets: they show how the session renews.
	for _, key := range []string{"sessionExp", "accessTokenStatus", "version"} {
		if value, ok := session[key]; ok {
			fmt.Printf("   %s: %s\n", key, value)
		}
	}
	return true, nil
}

// get fetches a page, and fails on an answer that is not a success.
func (b *browser) get(target, accept string) (*colly.Response, error) {
	hdr := http.Header{}
	if accept != "" {
		hdr.Set("Accept", accept)
	}
	r, err := b.send(http.MethodGet, target, nil, hdr)
	if err == nil && r.StatusCode >= 400 {
		err = fmt.Errorf("%s answered %d", where(r.Request.URL), r.StatusCode)
	}
	return r, err
}

func (b *browser) getJSON(target string, into any) error {
	r, err := b.get(target, "application/json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(r.Body, into); err != nil {
		return fmt.Errorf("reading %s: %w", where(r.Request.URL), err)
	}
	return nil
}

// send makes one request and returns its answer, whatever its status.
func (b *browser) send(method, target string, body []byte, hdr http.Header) (*colly.Response, error) {
	b.last = nil
	err := b.c.Request(method, target, bytes.NewReader(body), nil, hdr)
	if b.last == nil {
		if err == nil {
			err = errors.New("no answer")
		}
		return nil, fmt.Errorf("%s %s: %w", method, target, err)
	}
	return b.last, nil
}

// loadSession puts the saved session cookies in the browser's jar. It
// reports false when there is no file to load.
func (b *browser) loadSession(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var saved []savedCookie
	if err := json.Unmarshal(data, &saved); err != nil {
		return false, fmt.Errorf("reading %s: %w", path, err)
	}
	var cookies []*http.Cookie
	for _, s := range saved {
		cookies = append(cookies, &http.Cookie{
			Name: s.Name, Value: s.Value, Path: "/", Expires: s.Expires, Secure: true, HttpOnly: true,
		})
	}
	return true, b.c.SetCookies(webApp, cookies)
}

// saveSession writes the web app's session cookies to path, readable by the
// reader alone.
func (b *browser) saveSession(path string) error {
	var saved []savedCookie
	for _, c := range b.c.Cookies(webApp) {
		if c.Name == sessionCookie || strings.HasPrefix(c.Name, sessionCookie+".") {
			saved = append(saved, savedCookie{Name: c.Name, Value: c.Value, Expires: c.Expires})
		}
	}
	if len(saved) == 0 {
		return errors.New("the web app set no session cookie")
	}
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	fmt.Printf("\nsaved the session (%d cookie parts) to %s\n", len(saved), path)
	return nil
}

func sessionPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "sonos-sandbox", "web-session"), nil
}

// askCredentials asks for the Sonos account in the terminal. The password
// is not echoed.
func askCredentials() (string, string, error) {
	fmt.Print("   email: ")
	username, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", "", err
	}
	fmt.Print("   password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", "", fmt.Errorf("reading the password: %w", err)
	}
	return strings.TrimSpace(username), string(password), nil
}

// where is a URL without its query, which can hold tokens and codes.
func where(u *url.URL) string { return u.Host + u.Path }

// unescapeJS decodes the \xNN and \uNNNN escapes of a JavaScript string,
// which is how Okta writes values into its page.
func unescapeJS(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			out.WriteByte(s[i])
			continue
		}
		switch width := map[byte]int{'x': 2, 'u': 4}[s[i+1]]; {
		case width > 0 && i+2+width <= len(s):
			if n, err := strconv.ParseUint(s[i+2:i+2+width], 16, 32); err == nil {
				out.WriteRune(rune(n))
				i += 1 + width
				continue
			}
			out.WriteByte(s[i])
		default:
			out.WriteByte(s[i+1])
			i++
		}
	}
	return out.String()
}
