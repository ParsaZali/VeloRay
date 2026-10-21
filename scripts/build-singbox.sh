#!/usr/bin/env bash
set -Eeuo pipefail
export GOTOOLCHAIN=go1.26.10
arch="${1:?architecture required}";output="${2:?output directory required}"
case "$arch" in amd64|arm64) ;;*) exit 2;;esac
version=1.14.3
source_digest=9a8e3712a5f611ad95f1d16874bbdcdffbd3b8eed16dae2d71ce7042a33c3386
temporary="$(mktemp -d)";trap 'rm -rf -- "$temporary"' EXIT
if [[ -n "${VELORAY_SINGBOX_SOURCE:-}" ]];then cp "$VELORAY_SINGBOX_SOURCE" "$temporary/source.tar.gz"
else curl --proto '=https' --proto-redir '=https' -fsSL --retry 3 "https://codeload.github.com/SagerNet/sing-box/tar.gz/refs/tags/v$version" -o "$temporary/source.tar.gz";fi
printf '%s  %s\n' "$source_digest" "$temporary/source.tar.gz" | sha256sum -c -
mkdir -p "$temporary/source" "$output"
output="$(cd "$output" && pwd)"
tar -xzf "$temporary/source.tar.gz" --strip-components=1 -C "$temporary/source"
python3 - "$temporary/source/experimental/clashapi/connections.go" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]);s=p.read_text();needle='"network":         c.Metadata.Network,'
if s.count(needle)!=1:raise SystemExit('Unexpected upstream connection metadata format')
p.write_text(s.replace(needle,needle+'\n            "user": c.Metadata.User,'))
PY
(cd "$temporary/source" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -buildvcs=false -trimpath -tags=with_quic,with_utls,with_v2ray_api,with_clash_api,with_wireguard -ldflags="-s -w -X github.com/sagernet/sing-box/constant.Version=$version" -o "$output/sing-box" ./cmd/sing-box)
cp "$temporary/source/LICENSE" "$output/LICENSE"
cp /usr/share/common-licenses/GPL-3 "$output/COPYING"
printf '%s\n' "$version" >"$output/VERSION"
XZ_OPT='-T2 -6' tar -cJf "$output/source.tar.xz" -C "$temporary/source" .
(cd "$output" && sha256sum sing-box source.tar.xz >SHA256SUMS)
