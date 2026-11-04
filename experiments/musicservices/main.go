// Command musicservices asks one speaker which music services the system
// offers, through MusicServices ListAvailableServices, and lists them. A
// service counts as available when it streams for this household: it needs
// no account, or the household has one, seen in its favourites or a queue.
//
// The speaker is the first one discovery finds, or the one -ip names. Console
// output only. Nothing here is production code; its findings belong in
// SONOS-NOTES.md.
package main

import (
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const (
	musicServicesPath    = "/MusicServices/Control"
	musicServicesURN     = "urn:schemas-upnp-org:service:MusicServices:1"
	contentDirectoryPath = "/MediaServer/ContentDirectory/Control"
	contentDirectoryURN  = "urn:schemas-upnp-org:service:ContentDirectory:1"
	favouritesObjectID   = "FV:2" // the household's Sonos favourites
	anonymousAuth        = "Anonymous"
	discoverFor          = 3 * time.Second
	askFor               = 10 * time.Second

	// serviceTypeOffset is what a service type adds to sid*256, as seen in
	// the desc of every item: 52231 = 204*256 + 7 for Apple Music.
	serviceTypeOffset = 7
)

var ipFlag = flag.String("ip", "", "the speaker to ask; the first one discovery finds by default")

// listAvailableServices is the request. It takes no arguments.
type listAvailableServices struct {
	XMLName   xml.Name
	Namespace string `xml:"xmlns:u,attr"`
}

// response is the SOAP answer. The descriptor list arrives as escaped XML
// inside one element, so it is parsed a second time.
type response struct {
	Body struct {
		Answer struct {
			Descriptors string `xml:"AvailableServiceDescriptorList"`
			Types       string `xml:"AvailableServiceTypeList"`
			Version     string `xml:"AvailableServiceListVersion"`
		} `xml:"ListAvailableServicesResponse"`
	} `xml:"Body"`
}

// browse asks ContentDirectory for the children of one object.
type browse struct {
	XMLName        xml.Name
	Namespace      string `xml:"xmlns:u,attr"`
	ObjectID       string `xml:"ObjectID"`
	BrowseFlag     string `xml:"BrowseFlag"`
	Filter         string `xml:"Filter"`
	StartingIndex  int    `xml:"StartingIndex"`
	RequestedCount int    `xml:"RequestedCount"`
	SortCriteria   string `xml:"SortCriteria"`
}

// browseResult is a Browse answer; Result is escaped DIDL, parsed again.
type browseResult struct {
	Body struct {
		Answer struct {
			Result string `xml:"Result"`
		} `xml:"BrowseResponse"`
	} `xml:"Body"`
}

// accounts are the account numbers (sn) seen per service id (sid).
type accounts map[int]map[int]bool

// service is one entry of the descriptor list.
type service struct {
	ID            int    `xml:"Id,attr"`
	Name          string `xml:"Name,attr"`
	Version       string `xml:"Version,attr"`
	URI           string `xml:"Uri,attr"`
	SecureURI     string `xml:"SecureUri,attr"`
	ContainerType string `xml:"ContainerType,attr"`
	Capabilities  string `xml:"Capabilities,attr"`
	Policy        struct {
		Auth         string `xml:"Auth,attr"`
		PollInterval string `xml:"PollInterval,attr"`
	} `xml:"Policy"`
	Manifest struct {
		Version string `xml:"Version,attr"`
		URI     string `xml:"Uri,attr"`
	} `xml:"Manifest"`
}

func main() {
	flag.Parse()
	ctx := context.Background()

	// Discovery logs each description it reads; only its warnings matter here.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))

	ip := *ipFlag
	if ip == "" {
		found, err := firstSpeaker(ctx)
		if err != nil {
			fail("finding a speaker: %v", err)
		}
		ip = found
	}
	fmt.Printf("asking %s\n\n", ip)

	client := &sonos.SonosClient{BaseURL: "http://" + ip + ":1400", HTTPClient: &http.Client{Timeout: askFor}}
	body, err := client.Do(ctx, musicServicesPath, musicServicesURN, "ListAvailableServices", listAvailableServices{
		XMLName:   xml.Name{Local: "u:ListAvailableServices"},
		Namespace: musicServicesURN,
	})
	if err != nil {
		fail("ListAvailableServices: %v", err)
	}

	println(string(body))
	println(" --- ")

	var answer response
	if err := xml.Unmarshal(body, &answer); err != nil {
		fail("reading the answer: %v", err)
	}
	var list struct {
		Services []service `xml:"Service"`
	}
	if err := xml.Unmarshal([]byte(answer.Body.Answer.Descriptors), &list); err != nil {
		fail("reading the service list: %v", err)
	}

	seen, err := accountsInUse(ctx, client)
	if err != nil {
		fmt.Printf("could not read which accounts are in use: %v\n\n", err)
	}

	printServices(list.Services, seen)
	printAvailable(list.Services, seen)
	printTypes(answer.Body.Answer.Types, list.Services)
	fmt.Printf("\nlist version: %s\n", answer.Body.Answer.Version)
}

// firstSpeaker is the address of the first speaker discovery finds.
func firstSpeaker(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, discoverFor)
	defer cancel()

	found, err := sonos.Discover(ctx)
	if err != nil {
		return "", err
	}
	for speaker := range found {
		return speaker.IP, nil
	}
	return "", fmt.Errorf("none answered within %v", discoverFor)
}

// accountsInUse reads the accounts this household uses, from the sid and sn
// in the URIs of its favourites and of the speaker's queue. The speaker has
// no call that lists accounts, so this is the only evidence there is.
func accountsInUse(ctx context.Context, client *sonos.SonosClient) (accounts, error) {
	seen := accounts{}
	note := func(uri string) {
		if ref, ok := sonos.ParseServiceURI(uri); ok {
			if seen[ref.Service] == nil {
				seen[ref.Service] = map[int]bool{}
			}
			seen[ref.Service][ref.Account] = true
		}
	}

	body, err := client.Do(ctx, contentDirectoryPath, contentDirectoryURN, "Browse", browse{
		XMLName: xml.Name{Local: "u:Browse"}, Namespace: contentDirectoryURN,
		ObjectID: favouritesObjectID, BrowseFlag: "BrowseDirectChildren", Filter: "*", RequestedCount: 1000,
	})
	if err != nil {
		return seen, fmt.Errorf("browsing the favourites: %w", err)
	}
	var answer browseResult
	if err := xml.Unmarshal(body, &answer); err != nil {
		return seen, fmt.Errorf("reading the favourites: %w", err)
	}
	var didl struct {
		Items []struct {
			Res string `xml:"res"`
		} `xml:"item"`
	}
	if err := xml.Unmarshal([]byte(answer.Body.Answer.Result), &didl); err != nil {
		return seen, fmt.Errorf("reading the favourites' DIDL: %w", err)
	}
	for _, item := range didl.Items {
		note(item.Res)
	}

	queue, err := client.GetQueue(ctx)
	if err != nil {
		return seen, fmt.Errorf("reading the queue: %w", err)
	}
	for _, item := range queue.Items {
		note(item.URI)
	}
	fmt.Printf("accounts read from %d favourites and %d queued tracks\n\n", len(didl.Items), len(queue.Items))
	return seen, nil
}

// availability says why a service streams here, or "-" when nothing shows
// that it does.
func availability(s service, seen accounts) string {
	var why []string
	if s.Policy.Auth == anonymousAuth {
		why = append(why, "no account needed")
	}
	if numbers := seen[s.ID]; len(numbers) > 0 {
		var sns []int
		for sn := range numbers {
			sns = append(sns, sn)
		}
		sort.Ints(sns)
		why = append(why, fmt.Sprintf("in use (sn %s)", strings.Trim(fmt.Sprint(sns), "[]")))
	}
	if len(why) == 0 {
		return "-"
	}
	return strings.Join(why, ", ")
}

// printAvailable lists the services that stream here, the ones in use first.
func printAvailable(services []service, seen accounts) {
	var inUse, open []string
	for _, s := range services {
		switch {
		case len(seen[s.ID]) > 0:
			inUse = append(inUse, fmt.Sprintf("%s (%d)", s.Name, s.ID))
		case s.Policy.Auth == anonymousAuth:
			open = append(open, fmt.Sprintf("%s (%d)", s.Name, s.ID))
		}
	}
	fmt.Printf("\n%d available: %d in use by this household, %d need no account\n",
		len(inUse)+len(open), len(inUse), len(open))
	fmt.Printf("  in use:            %s\n", strings.Join(inUse, ", "))
	fmt.Printf("  no account needed: %s\n", strings.Join(open, ", "))
}

// printServices lists every service, sorted by name.
func printServices(services []service, seen accounts) {
	sort.Slice(services, func(i, j int) bool {
		return strings.ToLower(services[i].Name) < strings.ToLower(services[j].Name)
	})
	fmt.Printf("%d services offered\n\n", len(services))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tAVAILABLE\tAUTH\tPOLL\tCAPABILITIES\tTYPE\tVERSION\tURI")
	for _, s := range services {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", s.ID, s.Name, availability(s, seen), s.Policy.Auth,
			s.Policy.PollInterval, s.Capabilities, s.ContainerType, s.Version, s.SecureURI)
	}
	w.Flush()
}

// printTypes turns the service type list back into service ids, by undoing
// sid*256+7, and names each one. A type that does not undo cleanly, or that
// names no listed service, is shown as it came.
func printTypes(raw string, services []service) {
	byID := map[int]string{}
	for _, s := range services {
		byID[s.ID] = s.Name
	}

	var types []string
	for t := range strings.SplitSeq(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			types = append(types, t)
		}
	}
	fmt.Printf("\n%d service types listed (AvailableServiceTypeList)\n\n", len(types))

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tSID\tNAME")
	for _, t := range types {
		n, err := strconv.Atoi(t)
		switch {
		case err != nil:
			fmt.Fprintf(w, "%s\t?\tnot a number\n", t)
		case (n-serviceTypeOffset)%256 != 0:
			fmt.Fprintf(w, "%s\t?\tnot sid*256+%d\n", t, serviceTypeOffset)
		default:
			sid := (n - serviceTypeOffset) / 256
			name, listed := byID[sid]
			if !listed {
				name = "(not in the service list)"
			}
			fmt.Fprintf(w, "%s\t%d\t%s\n", t, sid, name)
		}
	}
	w.Flush()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
