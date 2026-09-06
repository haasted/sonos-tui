package sonos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheVersion guards against reading a file written by an older layout.
// A mismatch is treated as no cache at all, never as an error worth surfacing.
const cacheVersion = 1

const cacheDirName = "sonos-sandbox"
const cacheFileName = "speakers.json"

// verifyTimeout bounds each verification call. Deliberately short: the whole
// point is to be quicker than discovery, so a dead IP must fail fast rather
// than stall startup the way a full Discover would.
const verifyTimeout = 2 * time.Second

// CachedSpeaker is one speaker as it was last seen, enough to rebuild a
// Speaker without asking the network anything.
type CachedSpeaker struct {
	IP          string
	Location    string
	USN         string
	Description DeviceDescription
}

// Cache is the speaker list from a previous run, plus the context needed to
// judge whether it still applies.
type Cache struct {
	Version  int
	SavedAt  time.Time
	Subnet   string // network this was recorded on, as a fast pre-filter
	Selected string // USN of the speaker whose tab was active at exit
	Speakers []CachedSpeaker
}

// CachePath is the file the cache lives in, under the user's cache directory.
func CachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locating cache directory: %w", err)
	}
	return filepath.Join(dir, cacheDirName, cacheFileName), nil
}

// LoadCache reads the cache. A missing file, an unreadable one, or a version
// mismatch all report ok=false rather than an error: the cache is only ever a
// shortcut, and every failure path falls through to a full Discover.
func LoadCache() (cache Cache, ok bool) {
	path, err := CachePath()
	if err != nil {
		return Cache{}, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Cache{}, false
	}
	if err := json.Unmarshal(data, &cache); err != nil {
		return Cache{}, false
	}
	if cache.Version != cacheVersion || len(cache.Speakers) == 0 {
		return Cache{}, false
	}
	return cache, true
}

// SaveCache writes the speaker list and the selected speaker, creating the
// cache directory if needed.
func SaveCache(speakers []Speaker, selectedUSN string) error {
	path, err := CachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}

	subnet, err := LocalSubnet()
	if err != nil {
		return fmt.Errorf("reading local subnet: %w", err)
	}

	cache := Cache{
		Version:  cacheVersion,
		SavedAt:  time.Now(),
		Subnet:   subnet,
		Selected: selectedUSN,
	}
	for _, s := range speakers {
		desc, _ := s.Description()
		cache.Speakers = append(cache.Speakers, CachedSpeaker{
			IP: s.IP, Location: s.Location, USN: s.USN, Description: desc,
		})
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// UsableOn reports whether the cache was recorded on the given subnet. A
// mismatch means a different network, so the cached IPs are meaningless.
func (c Cache) UsableOn(subnet string) bool {
	return c.Subnet != "" && c.Subnet == subnet
}

// Speakers rebuilt from the cache, descriptions already populated so tabs can
// show real room names without waiting for a fetch.
func (c Cache) SpeakerList() []Speaker {
	speakers := make([]Speaker, 0, len(c.Speakers))
	for _, s := range c.Speakers {
		speakers = append(speakers, NewSpeaker(s.IP, s.Location, s.USN, s.Description))
	}
	return speakers
}

// VerifySpeaker proves a cached speaker is still the device it claims to be,
// still at that address, and still answering. The cheap GET runs first so a
// dead or reassigned IP fails without paying for a SOAP round trip.
func VerifySpeaker(ctx context.Context, speaker Speaker) error {
	client := &http.Client{Timeout: verifyTimeout}

	desc, err := FetchDeviceDescription(ctx, client, speaker.Location)
	if err != nil {
		return fmt.Errorf("fetching description: %w", err)
	}
	if !identityMatches(speaker.USN, desc.UDN) {
		return fmt.Errorf("address now belongs to %s, not %s", desc.UDN, speaker.USN)
	}
	speaker.storeDescription(desc)

	device := NewDeviceWithClient(speaker, &SonosClient{
		BaseURL:    "http://" + speaker.IP + ":1400",
		HTTPClient: client,
	})
	if err := device.Refresh(); err != nil {
		return fmt.Errorf("transport check: %w", err)
	}
	return nil
}

// identityMatches compares an SSDP USN against a description's UDN. The USN
// is the UDN plus "::<service>", so a prefix match is the whole test.
func identityMatches(usn, udn string) bool {
	if usn == "" || udn == "" {
		return false
	}
	return strings.HasPrefix(usn, udn)
}
