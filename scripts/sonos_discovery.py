#!/usr/bin/env python3
"""
Sonos Device Discovery + Description Fetcher
---------------------------------------------
Discovers Sonos speakers on the local network via SSDP (port 1900),
then fetches and parses the UPnP device description XML from each one.

No dependencies beyond the standard library + requests:
    pip install requests
"""

import socket
import re
import xml.etree.ElementTree as ET
from typing import List, Optional
from urllib.parse import urlparse
import requests

# --- SSDP Discovery -----------------------------------------------------------

SSDP_ADDR = "239.255.255.250"
SSDP_PORT = 1900
SSDP_MX    = 3   # seconds to wait for responses
SSDP_ST    = "urn:schemas-upnp-org:device:ZonePlayer:1"  # Sonos-specific

SSDP_REQUEST = (
    "M-SEARCH * HTTP/1.1\r\n"
    f"HOST: {SSDP_ADDR}:{SSDP_PORT}\r\n"
    "MAN: \"ssdp:discover\"\r\n"
    f"MX: {SSDP_MX}\r\n"
    f"ST: {SSDP_ST}\r\n"
    "\r\n"
)


def _get_local_ip() -> str:
    """Find the local IP address of the active LAN interface."""
    try:
        # Connect to an external address (doesn't send data) to determine
        # which network interface the OS would use for LAN traffic.
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except OSError:
        return "0.0.0.0"


def discover_sonos_speakers(timeout: int = SSDP_MX + 1) -> List[dict]:
    """
    Sends an SSDP M-SEARCH multicast and collects Sonos speaker responses.
    Returns a list of dicts with keys: ip, location, usn.
    """
    local_ip = _get_local_ip()

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
    # Bind to the active LAN interface so multicast is sent on the right adapter
    sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_IF,
                    socket.inet_aton(local_ip))
    sock.setsockopt(socket.IPPROTO_IP, socket.IP_MULTICAST_TTL, 2)
    sock.settimeout(timeout)

    sock.sendto(SSDP_REQUEST.encode(), (SSDP_ADDR, SSDP_PORT))

    seen_ips = set()
    speakers = []

    try:
        while True:
            data, addr = sock.recvfrom(4096)
            ip = addr[0]
            if ip in seen_ips:
                continue
            seen_ips.add(ip)

            response = data.decode(errors="ignore")
            location = _parse_header(response, "LOCATION")
            usn      = _parse_header(response, "USN")

            if location:
                speakers.append({"ip": ip, "location": location, "usn": usn or ""})
    except socket.timeout:
        pass
    finally:
        sock.close()

    return speakers


def _parse_header(response: str, header: str) -> Optional[str]:
    """Extract a header value from an HTTP-style response string."""
    match = re.search(rf"^{header}:\s*(.+)$", response, re.MULTILINE | re.IGNORECASE)
    return match.group(1).strip() if match else None


# --- Device Description -------------------------------------------------------

# UPnP XML namespaces used in Sonos device descriptions
NS = {
    "upnp": "urn:schemas-upnp-org:device-1-0",
    "r":    "urn:schemas-rinconnetworks-com:device-1-0",
}


def fetch_device_description(location_url: str) -> dict:
    """
    GETs the device description XML at the given UPnP location URL and
    returns a structured dict of the most useful fields.
    """
    resp = requests.get(location_url, timeout=5)
    resp.raise_for_status()

    root = ET.fromstring(resp.text)
    device = root.find("upnp:device", NS)

    def text(tag: str) -> str:
        el = device.find(tag, NS) if device is not None else None
        return el.text.strip() if el is not None and el.text else ""

    # List all services the speaker exposes
    services = []
    for svc in root.findall(".//upnp:service", NS):
        service_type = svc.findtext("upnp:serviceType", namespaces=NS, default="")
        control_url  = svc.findtext("upnp:controlURL",  namespaces=NS, default="")
        services.append({"type": service_type, "controlURL": control_url})

    # Sonos-specific extra fields (if present)
    display_version = text("r:display-version") or text("r:softwareVersion")

    parsed = urlparse(location_url)
    base_url = f"{parsed.scheme}://{parsed.netloc}"

    return {
        "base_url":         base_url,
        "friendly_name":    text("upnp:friendlyName"),
        "model_name":       text("upnp:modelName"),
        "model_number":     text("upnp:modelNumber"),
        "manufacturer":     text("upnp:manufacturer"),
        "serial_number":    text("upnp:serialNum") or text("upnp:UDN"),
        "software_version": display_version,
        "room_name":        text("r:roomName"),
        "services":         services,
        "description_url":  location_url,
    }


# --- Pretty Printing ----------------------------------------------------------

def print_device(info: dict) -> None:
    width = 60
    print("=" * width)
    print(f"  {info['friendly_name']}")
    print("=" * width)
    print(f"  Model:          {info['model_name']} ({info['model_number']})")
    print(f"  Room:           {info['room_name']}")
    print(f"  Base URL:       {info['base_url']}")
    print(f"  Manufacturer:   {info['manufacturer']}")
    print(f"  Serial:         {info['serial_number']}")
    print(f"  SW Version:     {info['software_version']}")
    print(f"  Description:    {info['description_url']}")
    print(f"\n  Exposed services ({len(info['services'])}):")
    for svc in info["services"]:
        short_type = svc["type"].split(":")[-2] if ":" in svc["type"] else svc["type"]
        print(f"    • {short_type:<30} → {info['base_url']}{svc['controlURL']}")
    print()


# --- Main ---------------------------------------------------------------------

if __name__ == "__main__":
    print("Searching for Sonos speakers on the local network (SSDP)...")
    speakers = discover_sonos_speakers()

    if not speakers:
        print("No Sonos speakers found. Make sure you're on the same network.")
        raise SystemExit(1)

    print(f"Found {len(speakers)} speaker(s). Fetching device descriptions...\n")

    for speaker in speakers:
        try:
            info = fetch_device_description(speaker["location"])
            print_device(info)
        except Exception as e:
            print(f"Could not fetch description for {speaker['ip']}: {e}\n")
