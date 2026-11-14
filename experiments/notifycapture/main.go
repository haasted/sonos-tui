// Command notifycapture records the NOTIFY bodies a speaker sends for one
// event service, so a parser can be written against what arrives rather than
// against what the documentation says arrives.
//
// It subscribes itself rather than going through EventManager: the point is
// the raw body, and EventManager hands out parsed events for the services it
// already understands. A service nobody has written a parser for yet is
// exactly the one worth capturing, and there is nothing to parse it with.
//
// Nothing here is production code. Its findings belong in SONOS-NOTES.md,
// and this should go once they are there.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/haasted/sonos-tui/internal/sonos"
)

const (
	// discoverFor is how long to look for speakers. Long enough for a slow
	// household, short enough to keep the harness usable.
	discoverFor = 5 * time.Second

	// describeFor bounds the wait for the room names. Discover streams a
	// speaker the moment SSDP sees it and fetches its description after, so
	// until that lands a speaker calls itself by its IP address.
	describeFor = 3 * time.Second

	// subscribeSeconds is how long a speaker keeps sending. Not renewed: a
	// capture session is minutes, and a renewal loop is machinery this does
	// not need.
	subscribeSeconds = 900
)

var (
	roomFlag    = flag.String("room", "", "subscribe only this room; every room by default")
	serviceFlag = flag.String("service", "/ZoneGroupTopology/Event", "the event service to subscribe to")
	outFlag     = flag.String("out", "notify-captures", "directory to write the bodies to")
)

// subscription is one speaker listening, and what it takes to stop.
type subscription struct {
	speaker sonos.Speaker
	sid     string
}

func main() {
	flag.Parse()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	speakers, err := findSpeakers(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "opening a port to be notified on:", err)
		os.Exit(1)
	}
	defer listener.Close()

	local, err := addressReachableBy(speakers[0], listener.Addr().(*net.TCPAddr).Port)
	if err != nil {
		fmt.Fprintln(os.Stderr, "working out what address a speaker can reach us on:", err)
		os.Exit(1)
	}

	var count int
	var mu sync.Mutex
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)

		mu.Lock()
		count++
		n := count
		mu.Unlock()
		record(n, strings.TrimPrefix(r.URL.Path, "/"), body)
	})}
	go server.Serve(listener)
	defer server.Close()

	subs := subscribeAll(ctx, speakers, local)
	if len(subs) == 0 {
		fmt.Fprintln(os.Stderr, "no speaker accepted a subscription to", *serviceFlag)
		os.Exit(1)
	}
	defer unsubscribeAll(subs)

	fmt.Printf("\nListening for %s on %s.\nChange something in the Sonos app. Ctrl-C to stop.\n\n",
		*serviceFlag, local)
	<-ctx.Done()
	fmt.Printf("\nCaptured %d notifications into %s\n", count, *outFlag)
}

// record writes one body and says what arrived, so a session can be watched
// as well as read back afterwards.
func record(n int, from string, body []byte) {
	name := fmt.Sprintf("%s-%03d-%s.xml", time.Now().Format("20060102-150405"), n, from)
	path := filepath.Join(*outFlag, name)

	if err := os.MkdirAll(*outFlag, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "making the output directory:", err)
		return
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "writing", path+":", err)
		return
	}
	fmt.Printf("%3d  %-22s %6d bytes  %s\n", n, from, len(body), path)
}

// subscribeAll asks every speaker to notify us, and reports which agreed.
func subscribeAll(ctx context.Context, speakers []sonos.Speaker, local string) []subscription {
	var subs []subscription
	for i, speaker := range speakers {
		callback := fmt.Sprintf("<http://%s/%s>", local, speaker.RoomName())

		sid, err := send(ctx, "SUBSCRIBE", speaker, map[string]string{
			"CALLBACK": callback,
			"NT":       "upnp:event",
			"TIMEOUT":  fmt.Sprintf("Second-%d", subscribeSeconds),
		})
		if err != nil {
			fmt.Printf("%-14s refused: %v\n", speaker.RoomName(), err)
			continue
		}

		fmt.Printf("%-14s subscribed as %s\n", speaker.RoomName(), sid)
		subs = append(subs, subscription{speaker: speakers[i], sid: sid})
	}
	return subs
}

// unsubscribeAll lets the speakers stop, rather than leaving them notifying
// a port that has gone.
func unsubscribeAll(subs []subscription) {
	for _, sub := range subs {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if _, err := send(ctx, "UNSUBSCRIBE", sub.speaker, map[string]string{"SID": sub.sid}); err != nil {
			fmt.Fprintf(os.Stderr, "%s did not release its subscription: %v\n", sub.speaker.RoomName(), err)
		}
		cancel()
	}
}

func send(ctx context.Context, method string, speaker sonos.Speaker, headers map[string]string) (string, error) {
	url := fmt.Sprintf("http://%s:1400%s", speaker.IP, *serviceFlag)
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return "", err
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s said %s", url, resp.Status)
	}
	return resp.Header.Get("SID"), nil
}

// addressReachableBy is the address a speaker can call us back on. Taken
// from a connection to the speaker itself rather than guessed: a machine has
// several addresses and only one of them faces that speaker.
func addressReachableBy(speaker sonos.Speaker, port int) (string, error) {
	conn, err := net.DialTimeout("tcp", speaker.IP+":1400", 3*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	host, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%d", host, port), nil
}

// awaitDescriptions waits until every speaker knows its own name, or until
// describeFor runs out. Without it a room filter is matched against the IP
// address a speaker answers to before its description has been fetched.
func awaitDescriptions(ctx context.Context, speakers []sonos.Speaker) {
	deadline := time.Now().Add(describeFor)

	for time.Now().Before(deadline) {
		named := true
		for _, speaker := range speakers {
			if _, ok := speaker.Description(); !ok {
				named = false
				break
			}
		}
		if named {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// roomsOf names the speakers found, so a filter that matched nothing can say
// what it could have matched.
func roomsOf(speakers []sonos.Speaker) []string {
	rooms := make([]string, 0, len(speakers))
	for _, speaker := range speakers {
		rooms = append(rooms, speaker.RoomName())
	}
	sort.Strings(rooms)
	return rooms
}

func findSpeakers(ctx context.Context) ([]sonos.Speaker, error) {
	deadline, cancel := context.WithTimeout(ctx, discoverFor)
	defer cancel()

	found, err := sonos.Discover(deadline)
	if err != nil {
		return nil, err
	}

	var discovered []sonos.Speaker
	for speaker := range found {
		discovered = append(discovered, speaker)
	}
	awaitDescriptions(ctx, discovered)

	var speakers []sonos.Speaker
	for _, speaker := range discovered {
		if *roomFlag == "" || strings.EqualFold(speaker.RoomName(), *roomFlag) {
			speakers = append(speakers, speaker)
		}
	}
	if len(speakers) == 0 {
		if *roomFlag != "" {
			return nil, fmt.Errorf("no speaker called %q, out of %d found: %s",
				*roomFlag, len(discovered), strings.Join(roomsOf(discovered), ", "))
		}
		return nil, fmt.Errorf("no speaker found")
	}

	sort.Slice(speakers, func(i, j int) bool { return speakers[i].IP < speakers[j].IP })
	return speakers, nil
}
