package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/haasted/sonos-sandbox/internal/sonos"
)

func TestSpeakerCacheRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "speaker-cache.json")

	speakers := []sonos.Speaker{
		sonos.NewSpeaker("192.168.68.51", "http://192.168.68.51:1400/xml/device_description.xml", "uuid:RINCON_1",
			sonos.DeviceDescription{RoomName: "Kitchen", DisplayName: "Amp", ModelName: "Sonos Amp", ModelNumber: "S38"}),
		sonos.NewSpeaker("192.168.68.55", "http://192.168.68.55:1400/xml/device_description.xml", "uuid:RINCON_2",
			sonos.DeviceDescription{}),
	}

	if err := writeSpeakerCache(path, speakers); err != nil {
		t.Fatalf("writeSpeakerCache: %v", err)
	}

	cache, ok, err := loadSpeakerCache(path)
	if err != nil {
		t.Fatalf("loadSpeakerCache: %v", err)
	}
	if !ok {
		t.Fatal("loadSpeakerCache: ok = false, want true")
	}
	if cache.Version != speakerCacheVersion {
		t.Errorf("Version = %d, want %d", cache.Version, speakerCacheVersion)
	}
	if cache.SavedAt.IsZero() {
		t.Error("SavedAt is zero, want a real timestamp")
	}
	if len(cache.Speakers) != len(speakers) {
		t.Fatalf("got %d speakers, want %d", len(cache.Speakers), len(speakers))
	}

	got := cache.Speakers[0]
	want := cachedSpeaker{IP: "192.168.68.51", Location: "http://192.168.68.51:1400/xml/device_description.xml", USN: "uuid:RINCON_1",
		Description: sonos.DeviceDescription{RoomName: "Kitchen", DisplayName: "Amp", ModelName: "Sonos Amp", ModelNumber: "S38"}}
	if got != want {
		t.Errorf("Speakers[0] = %+v, want %+v", got, want)
	}
}

func TestLoadSpeakerCache_MissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	_, ok, err := loadSpeakerCache(path)
	if err != nil {
		t.Fatalf("expected no error for a missing file, got: %v", err)
	}
	if ok {
		t.Fatal("expected ok = false for a missing file")
	}
}

func TestLoadSpeakerCache_VersionMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "speaker-cache.json")

	data, err := json.Marshal(speakerCache{Version: speakerCacheVersion + 1})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	_, ok, err := loadSpeakerCache(path)
	if err == nil {
		t.Fatal("expected an error for a version mismatch")
	}
	if ok {
		t.Fatal("expected ok = false for a version mismatch")
	}
}
