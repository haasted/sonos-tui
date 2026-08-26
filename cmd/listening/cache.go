package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

const speakerCacheVersion = 1

type cachedSpeaker struct {
	IP          string
	Location    string
	USN         string
	Description sonos.DeviceDescription
}

type speakerCache struct {
	Version  int
	SavedAt  time.Time
	Speakers []cachedSpeaker
}

func loadSpeakerCache(path string) (cache speakerCache, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return speakerCache{}, false, nil
	}
	if err != nil {
		return speakerCache{}, false, err
	}

	if err := json.Unmarshal(data, &cache); err != nil {
		return speakerCache{}, false, fmt.Errorf("parsing %s: %w", path, err)
	}
	if cache.Version != speakerCacheVersion {
		return speakerCache{}, false, fmt.Errorf("cache %s has version %d, want %d", path, cache.Version, speakerCacheVersion)
	}
	return cache, true, nil
}

func writeSpeakerCache(path string, speakers []sonos.Speaker) error {
	cache := speakerCache{Version: speakerCacheVersion, SavedAt: time.Now()}
	for _, sp := range speakers {
		desc, _ := sp.Description()
		cache.Speakers = append(cache.Speakers, cachedSpeaker{
			IP:          sp.IP,
			Location:    sp.Location,
			USN:         sp.USN,
			Description: desc,
		})
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
