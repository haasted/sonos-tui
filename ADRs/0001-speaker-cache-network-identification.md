# 1. Identifying the network for a speaker cache

## Status

Proposed

## Context

`Discover` currently runs a full SSDP discovery burst (up to a few seconds) on every program invocation. We'd like to cache the speaker list between runs so the program can start up quickly, using the cache instead of a full discovery when it's likely still valid.

The open question: how does the program know its cached speaker list still applies? The instinctive approach is to identify "which network am I on" (e.g. by Wi-Fi SSID) and key the cache to that. Investigating this surfaced a real gap in Go's standard library and a better-fitting alternative, both captured here.

## Options considered

### 1. Read the Wi-Fi SSID

The most intuitive network identifier — but Go's standard library has no cross-platform way to read it. It requires OS-specific mechanisms:

- macOS: shell out to `networksetup -getairportnetwork en0`
- Linux: shell out to `iwgetid` / `nmcli`, or parse `/proc/net/wireless`
- Windows: shell out to `netsh wlan show interfaces`

**Rejected.** Every option is a subprocess dependency or CGo-adjacent, which conflicts with this project's established preference for pure Go, no CGo (the deciding factor in every library evaluation so far — `go-termimg`, `koron/go-ssdp`).

Surveyed the library landscape too, in case something solved this more cleanly than hand-rolling it: [mdlayher/wifi](https://pkg.go.dev/github.com/mdlayher/wifi) is the best-built option (MIT, pure Go via netlink, 74 importers) but Linux-only — doesn't run on macOS. [wifiscan](https://github.com/shadowypress/wifiscan) is cross-platform but shells out to the same OS utilities (`airport`/`iwlist`/`netsh.exe`) this option already rejected, and scans for *nearby* SSIDs rather than reporting the current connection cleanly. No library changes this option's conclusion.

### 2. Fingerprint the local subnet

`net.InterfaceAddrs()` gives the local IP and subnet mask with no CGo and no subprocess — `discovery.go`'s existing `getLocalIP()` already computes half of this. The subnet (e.g. `192.168.68.0/24`) could serve as an approximate cache key.

**Weakness:** many routers default to the same private ranges (`192.168.1.0/24` is extremely common), so two genuinely different networks can collide on this key. Cheap and pure Go, but only an approximate signal.

### 3. Fingerprint the default gateway's MAC address

A router's MAC address (read via the ARP table) is a stronger, more stable per-network identifier than the subnet. Hits the same wall as option 1, though: no stdlib API for reading the ARP table, so it also requires shelling out (`arp -a`) or OS-specific parsing.

**Rejected for the same reason as option 1**, unless the subnet fingerprint proves too collision-prone in practice. [jackpal/gateway](https://github.com/jackpal/gateway) (BSD-3-Clause, pure Go, no CGo, actively maintained, broad platform support) would remove the hand-rolling problem for the gateway *IP* specifically — but the MAC address, the stronger fingerprint, still has no clean library answer, so this option's rejection stands.

### 4. Validate the cache by reachability, not network identity

Reframes the question: instead of asking "which network am I on," ask "are the cached speakers still there." On startup, probe each cached speaker directly — a quick TCP dial to port 1400, or a short-timeout `GetTransportInfo()` call — instead of fingerprinting the network at all.

This is arguably more correct than any network-identity check: being on the same Wi-Fi as last time doesn't guarantee a speaker wasn't unplugged or given a new DHCP lease, and a reachability check catches that where a network fingerprint wouldn't. Uses only tools already in the codebase.

### 5. Combine subnet fingerprint + reachability check

Use the cheap subnet fingerprint (option 2) as a fast pre-filter — a different subnet than last time means don't even bother probing, go straight to full discovery. If the subnet matches, confirm with a reachability check (option 4) before trusting the cache. Falls back to full `Discover` if either check fails.

## Decision

Refining option 5, but reframed: the subnet fingerprint is a **speed optimization only**, not a correctness mechanism. A real call to a cached speaker is the only thing that actually proves the cache entry is still good — the network-identity options (1–3) were all just proxies for that question, so once we're willing to ask the speaker directly, none of them are needed as the source of truth. Option 3's library finding (`jackpal/gateway`) is noted above but isn't used by this decision.

```mermaid
flowchart TD
    Start([Program start]) --> LocalIP[Get local IP / subnet]
    LocalIP --> HasCache{Cache file exists?}
    HasCache -- No --> Discover[Full Discover]
    HasCache -- Yes --> SubnetMatch{Cached subnet ==<br/>current subnet?}
    SubnetMatch -- No --> Discover
    SubnetMatch -- Yes --> Verify[Call real endpoint on target<br/>cached speaker, e.g. GetTransportInfo]
    Verify --> VerifyOK{Responded OK?}
    VerifyOK -- Yes --> UseCache[Use cached speaker + fresh data;<br/>trust rest of cache lazily]
    VerifyOK -- No --> Discover
    Discover --> WriteCache[Write new cache: subnet + speakers]
    WriteCache --> UseFresh[Use freshly discovered speakers]
    UseCache --> Done([Ready])
    UseFresh --> Done
```

The subnet mismatch and verification-failure paths both fall through to the same full `Discover` — the cache is never a hard dependency, only ever a shortcut around it.

### Open questions before implementation

- **Eager vs. lazy verification**: verify every cached speaker on startup, or only the one about to be used (e.g. `cmd/sonos/main.go` today only ever acts on `speakers[0]`)? Leaning lazy — matches the "quick startup" goal better, and re-verifying N entries when only one is needed defeats some of the point.
- **Which endpoint verifies a speaker**: a lightweight GET of the cached `Location` URL (`device_description.xml` — minimal, and its UDN can be cross-checked against the cached USN to catch a DHCP lease reassigned to a *different* device), or a real SOAP call like `GetTransportInfo()` that verifies and fetches useful data in the same round trip? Leaning toward the real call for whichever speaker is actually being used, lightweight GET for any others.

### Related, smaller optimization (no caching required)

Independent of this whole cache design: live measurements show `Discover` finds all speakers in well under 400ms, but its channel doesn't close until the full context deadline, since a straggler could still reply. A caller that only needs one speaker (e.g. `cmd/sonos/main.go`) could act on the *first* value received instead of draining the channel — a smaller, already-unlocked win from the streaming `Discover` design, worth doing regardless of whether the cache below is built.

## Consequences

The cache is a *fast path*, never a source of truth — `Discover` remains the fallback whenever the subnet check or the direct verification call fails. The cache file needs to store the subnet recorded at write time alongside the speaker list, so there's something to compare against on the next run. No decision here blocks other work; this document exists to record the reasoning so it doesn't need to be re-derived later.
