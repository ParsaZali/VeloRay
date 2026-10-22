#!/usr/bin/env bash
set -Eeuo pipefail
# Supply a locally downloaded Xray release and its expected SHA-256 digest.
# This avoids executing an unpinned installer fetched from the network.
tmp="$(mktemp -d)";trap 'rm -rf "$tmp"' EXIT
if [[ -n "${VELORAY_XRAY_ARCHIVE:-}" ]];then
 [[ "${VELORAY_XRAY_SHA256:-}" =~ ^[a-fA-F0-9]{64}$ ]]||{ echo 'A SHA-256 checksum is required for the Xray archive.' >&2;exit 1; }
 printf '%s  %s\n' "$VELORAY_XRAY_SHA256" "$VELORAY_XRAY_ARCHIVE" | sha256sum -c -
 unzip -q "$VELORAY_XRAY_ARCHIVE" xray geoip.dat geosite.dat -d "$tmp"
elif [[ -n "${VELORAY_XRAY_DIRECTORY:-}" && -f "$VELORAY_XRAY_DIRECTORY/SHA256SUMS" ]];then
 (cd "$VELORAY_XRAY_DIRECTORY" && sha256sum -c SHA256SUMS)
 cp "$VELORAY_XRAY_DIRECTORY/"{xray,geoip.dat,geosite.dat} "$tmp/"
else
 echo 'Use a complete runtime package or supply VELORAY_XRAY_ARCHIVE and VELORAY_XRAY_SHA256.' >&2;exit 1
fi
candidate_binary="$(mktemp /usr/local/bin/.veloray-xray.XXXXXX)"
install -m 0755 "$tmp/xray" "$candidate_binary"
mv -Tf "$candidate_binary" /usr/local/bin/xray
install -d -m 0755 /usr/local/share/xray
# Existing geo assets can contain operator-managed routing policy.
for asset in geoip.dat geosite.dat;do
 [[ -f "/usr/local/share/xray/$asset" ]]||install -m 0644 "$tmp/$asset" "/usr/local/share/xray/$asset"
done
/usr/local/bin/xray version
