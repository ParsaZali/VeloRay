#!/usr/bin/env bash
# Rebuild the pinned engine with the patched compiler, preserving its protocols.
set -Eeuo pipefail
arch="${1:?architecture required}";output="${2:?output directory required}"
case "$arch" in amd64|arm64) ;;*) exit 2;;esac
export GOTOOLCHAIN=go1.26.10
version="${VELORAY_XRAY_VERSION:-v26.3.27}"
source_digest="${VELORAY_XRAY_SOURCE_SHA256:-992a4997e6bb846d11469435d687f99ef812fcde1e0a009bb8e95189ea20331d}"
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ && "$source_digest" =~ ^[a-f0-9]{64}$ ]]||{ echo 'Invalid Xray source version or digest' >&2;exit 2; }
temporary="$(mktemp -d)";trap 'rm -rf -- "$temporary"' EXIT
if [[ -n "${VELORAY_XRAY_SOURCE:-}" ]];then cp "$VELORAY_XRAY_SOURCE" "$temporary/source.tar.gz"
else curl --proto '=https' --proto-redir '=https' -fsSL --retry 3 "https://codeload.github.com/XTLS/Xray-core/tar.gz/refs/tags/$version" -o "$temporary/source.tar.gz";fi
printf '%s  %s\n' "$source_digest" "$temporary/source.tar.gz" | sha256sum -c -
mkdir -p "$temporary/source" "$output"
output="$(cd "$output" && pwd)"
tar --no-same-owner --no-same-permissions -xzf "$temporary/source.tar.gz" --strip-components=1 -C "$temporary/source"
(cd "$temporary/source" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -buildvcs=false -trimpath -ldflags='-s -w -buildid=' -o "$output/xray" ./main)
mkdir -p "$output/licenses/go"
cp "$(go env GOROOT)/LICENSE" "$output/licenses/Go-LICENSE"
(cd "$temporary/source" && go list -m -json all) | jq -sr '.[]|select(.Dir!=null)|[.Path,.Dir]|@tsv' >"$temporary/modules"
while IFS=$'\t' read -r module directory;do
 name="${module//\//_}";mkdir -p "$output/licenses/go/$name"
 for file in "$directory"/LICENSE* "$directory"/COPYING*;do [[ ! -f "$file" ]]||cp "$file" "$output/licenses/go/$name/";done
done <"$temporary/modules"
cp "$temporary/source/LICENSE" "$output/LICENSE"
printf '%s\n' "$version" >"$output/VERSION"
XZ_OPT='-T2 -6' tar -cJf "$output/source.tar.xz" -C "$temporary/source" .
# Checked by the release verifier; upstream release assets supply the geo files.
(cd "$output" && sha256sum xray source.tar.xz >SHA256SUMS)
