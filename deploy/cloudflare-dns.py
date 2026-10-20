#!/usr/bin/env python3
"""Explicit Cloudflare A/AAAA upsert; token comes from a private file, never argv."""
import argparse
import ipaddress
import json
import os
from pathlib import Path
import re
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def upsert(zone: str, hostname: str, address: str, token: str, proxied=False, opener=None):
    if not re.fullmatch(r"[a-fA-F0-9]{32}", zone):
        raise ValueError("Invalid Cloudflare zone ID")
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9.-]{0,251}[a-zA-Z0-9]", hostname) or "." not in hostname or ".." in hostname:
        raise ValueError("Use a full DNS hostname")
    if not token or len(token) > 4096 or any(char.isspace() for char in token):
        raise ValueError("Invalid Cloudflare token file")
    ip = ipaddress.ip_address(address)
    hostname = hostname.lower()
    kind = "A" if ip.version == 4 else "AAAA"
    opener = opener or urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())
    base = f"https://api.cloudflare.com/client/v4/zones/{zone}/dns_records"

    def call(method, endpoint, body=None):
        headers = {"Authorization": "Bearer " + token, "Content-Type": "application/json"}
        request = urllib.request.Request(endpoint, data=json.dumps(body).encode() if body is not None else None, headers=headers, method=method)
        try:
            with opener.open(request, timeout=15) as response:
                raw = response.read(1048577)
        except urllib.error.HTTPError as exc:
            raise RuntimeError(f"Cloudflare request failed (HTTP {exc.code})") from None
        except (OSError, urllib.error.URLError):
            raise RuntimeError("Cloudflare is unavailable") from None
        if len(raw) > 1048576:
            raise RuntimeError("Cloudflare response exceeds 1 MiB")
        try:
            result = json.loads(raw)
        except (ValueError, UnicodeError):
            raise RuntimeError("Invalid Cloudflare response") from None
        if not isinstance(result, dict) or result.get("success") is not True:
            raise RuntimeError("Cloudflare rejected the DNS operation")
        return result.get("result")

    records = call("GET", base + "?" + urllib.parse.urlencode({"type": kind, "name": hostname, "per_page": 100}))
    if not isinstance(records, list) or len(records) > 1:
        raise RuntimeError("Ambiguous existing DNS records; review them before retrying")
    payload = {"type": kind, "name": hostname, "content": str(ip), "ttl": 1, "proxied": bool(proxied)}
    if records:
        record = records[0]
        if not isinstance(record, dict) or record.get("name") != hostname or record.get("type") != kind or not isinstance(record.get("id"), str) or not re.fullmatch(r"[a-fA-F0-9]{32}", record["id"]):
            raise RuntimeError("Cloudflare returned an unexpected DNS record")
        call("PATCH", base + "/" + record["id"], payload)
        return "updated"
    call("POST", base, payload)
    return "created"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--zone-id", required=True)
    parser.add_argument("--hostname", required=True)
    parser.add_argument("--address", required=True)
    parser.add_argument("--token-file", type=Path, default=os.environ.get("CLOUDFLARE_API_TOKEN_FILE"))
    parser.add_argument("--proxied", action="store_true", help="Enable Cloudflare HTTP proxying; leave off for ordinary VPN/UDP endpoints")
    args = parser.parse_args()
    try:
        if args.token_file is None or not args.token_file.is_file() or args.token_file.stat().st_size > 4096:
            raise ValueError("Provide a token file of at most 4096 bytes")
        result = upsert(args.zone_id, args.hostname, args.address, args.token_file.read_text().strip(), args.proxied)
    except (OSError, ValueError, RuntimeError):
        parser.exit(1, "DNS operation failed. Check the token permissions, zone, hostname and existing records; no credential was logged.\n")
    print("Cloudflare DNS record " + result + ".")
