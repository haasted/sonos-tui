# sonos-sandbox

A Go sandbox for exploring the Sonos local UPnP/SOAP API.

## Project Structure

```
sonos-sandbox/
├── cmd/
│   └── sonos/
│       └── main.go          # CLI entry point — discovers & queries a speaker
├── internal/
│   └── sonos/
│       ├── client.go        # Low-level SOAP HTTP client
│       ├── client_test.go
│       ├── discovery.go     # SSDP speaker discovery
│       ├── discovery_test.go
│       ├── transport.go     # AVTransport + RenderingControl commands
├── go.mod
├── go.sum
├── Makefile
└── sonos_discovery.py       # Original Python prototype
```

## Quickstart

```bash
# Discover speakers and query the first one found
go run ./cmd/sonos

# Query a specific speaker by IP
go run ./cmd/sonos 192.168.68.57

# Run all tests
make test

# Build binary
make build
```

## API Coverage

| Feature | Method |
|---|---|
| Play | `c.Play()` |
| Pause | `c.Pause()` |
| Stop | `c.Stop()` |
| Next track | `c.Next()` |
| Previous track | `c.Previous()` |
| Playback state | `c.GetTransportInfo()` |
| Current track + metadata | `c.GetPositionInfo()` |
| Get volume | `c.GetVolume()` |
| Set volume | `c.SetVolume(40)` |
| Get mute | `c.GetMute()` |
| Set mute | `c.SetMute(true)` |
| Discover speakers | `sonos.Discover(4 * time.Second)` |

## How It Works

Sonos speakers expose a local HTTP server on **port 1400** implementing the
UPnP/SOAP standard. Commands are `POST` requests with an XML body and a
`SOAPAction` header. No cloud or authentication required — everything runs
entirely on the local network.

Speaker discovery uses **SSDP** (Simple Service Discovery Protocol) — a UDP
multicast to `239.255.255.250:1900` with the Sonos-specific service type
`urn:schemas-upnp-org:device:ZonePlayer:1`.
