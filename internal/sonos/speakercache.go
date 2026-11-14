package sonos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/haasted/sonos-tui/internal/store"
)

// verifyTimeout bounds each verification call. Deliberately short: the whole
// point is to be quicker than discovery, so a dead IP must fail fast rather
// than stall startup the way a full Discover would.
const verifyTimeout = 2 * time.Second

// SpeakerCache is the speaker list from a previous run, plus the context
// needed to judge whether it still applies.
type SpeakerCache struct {
	SavedAt  time.Time
	Subnet   string // network this was recorded on, as a fast pre-filter
	Speakers []Speaker
}

// LoadCache reads the cache through the store. An empty store or an
// unreadable one reports ok=false rather than an error: the cache is only
// ever a shortcut, and every failure path falls through to a full Discover.
func LoadCache(ctx context.Context, cache *store.Cache) (SpeakerCache, bool) {
	snapshot, ok, err := cache.Load(ctx)
	if err != nil {
		slog.Warn("reading the speaker cache", "err", err)
		return SpeakerCache{}, false
	}
	if !ok || len(snapshot.Speakers) == 0 {
		return SpeakerCache{}, false
	}

	loaded := SpeakerCache{
		SavedAt:  snapshot.SavedAt,
		Subnet:   snapshot.Subnet,
		Speakers: make([]Speaker, 0, len(snapshot.Speakers)),
	}
	for _, s := range snapshot.Speakers {
		loaded.Speakers = append(loaded.Speakers, NewSpeaker(s.IP, s.Location, s.USN, DeviceDescription{
			UDN:         s.UDN,
			RoomName:    s.RoomName,
			DisplayName: s.DisplayName,
			ModelName:   s.ModelName,
			ModelNumber: s.ModelNumber,
		}))
	}
	return loaded, true
}

// SaveCache records the speaker list, replacing whatever the store held.
func SaveCache(ctx context.Context, cache *store.Cache, speakers []Speaker) error {
	subnet, err := LocalSubnet()
	if err != nil {
		return fmt.Errorf("reading local subnet: %w", err)
	}

	snapshot := store.Snapshot{
		SavedAt: time.Now(),
		Subnet:  subnet,
	}
	for _, s := range speakers {
		desc, _ := s.Description()
		snapshot.Speakers = append(snapshot.Speakers, store.CachedSpeaker{
			USN: s.USN, IP: s.IP, Location: s.Location,
			UDN:         desc.UDN,
			RoomName:    desc.RoomName,
			DisplayName: desc.DisplayName,
			ModelName:   desc.ModelName,
			ModelNumber: desc.ModelNumber,
		})
	}
	return cache.Save(ctx, snapshot)
}

// UsableOn reports whether the cache was recorded on the given subnet. A
// mismatch means a different network, so the cached IPs are meaningless.
func (c SpeakerCache) UsableOn(subnet string) bool {
	return c.Subnet != "" && c.Subnet == subnet
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
	if err := device.Refresh(ctx); err != nil {
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
