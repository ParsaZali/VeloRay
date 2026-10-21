#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
manifest() {
 {
  printf '%s\n' go.mod go.sum VERSION scripts/build.sh scripts/build-runtime.sh scripts/build-singbox.sh scripts/build-xray.sh scripts/verify-runtime-toolchain.sh
  # Python bytecode is local build state, never an input to the Go binaries.
  find cmd internal -type f ! -name '*_test.go' ! -name '*.pyc' ! -path '*/__pycache__/*'
  find frontend -maxdepth 1 -type f ! -name '*.tsbuildinfo'
  find frontend/src -type f
  [[ ! -d frontend/public ]]||find frontend/public -type f
 } | LC_ALL=C sort -u | while IFS= read -r file;do sha256sum "$file";done
}
if [[ "${1:-}" == --manifest ]];then manifest;exit 0;fi
[[ $# -eq 0 ]]||{ echo 'Usage: scripts/build-runtime.sh [--manifest]' >&2;exit 2; }
export GOTOOLCHAIN=go1.26.10
temporary="$(mktemp -d)";trap 'rm -rf -- "$temporary"' EXIT
mkdir -p runtime
manifest >"$temporary/source.SHA256SUMS"
mkdir -p "$temporary/licenses/go" "$temporary/licenses/npm"
cp LICENSE "$temporary/licenses/VeloRay-LICENSE"
cp "$(go env GOROOT)/LICENSE" "$temporary/licenses/Go-LICENSE"
go list -m -json all | jq -sr '.[]|select(.Dir!=null)|[.Path,.Dir]|@tsv' >"$temporary/modules"
while IFS=$'\t' read -r module directory;do
 name="${module//\//_}";mkdir -p "$temporary/licenses/go/$name"
 for file in "$directory"/LICENSE* "$directory"/COPYING*;do [[ ! -f "$file" ]]||cp "$file" "$temporary/licenses/go/$name/";done
done <"$temporary/modules"
xray_version="${VELORAY_XRAY_VERSION:-v26.3.27}"
[[ "$xray_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]||{ echo 'Invalid Xray version' >&2;exit 2; }
github_headers=()
if [[ -n "${GITHUB_TOKEN:-}" ]];then github_headers=(-H "Authorization: Bearer $GITHUB_TOKEN");fi
curl --proto '=https' --proto-redir '=https' -fsSL --retry 3 "${github_headers[@]}" "https://api.github.com/repos/XTLS/Xray-core/releases/tags/$xray_version" -o "$temporary/xray-release.json"
for arch in amd64 arm64;do
 bash scripts/build.sh "$arch"
 mkdir -p "$temporary/$arch/bin" "$temporary/$arch/frontend"
 cp -a "bin/linux-$arch" "$temporary/$arch/bin/"
 cp -a frontend/dist "$temporary/$arch/frontend/"
 mkdir -p "$temporary/$arch/xray"
 asset=Xray-linux-64.zip;[[ "$arch" != arm64 ]]||asset=Xray-linux-arm64-v8a.zip
 checksum="$(jq -er --arg name "$asset" '.assets[]|select(.name==$name)|.digest' "$temporary/xray-release.json")"
 [[ "$checksum" =~ ^sha256:[a-f0-9]{64}$ ]]||{ echo 'Missing Xray asset checksum' >&2;exit 1; }
 if [[ -n "${VELORAY_XRAY_CACHE:-}" && -f "$VELORAY_XRAY_CACHE/$asset" ]];then cp "$VELORAY_XRAY_CACHE/$asset" "$temporary/$asset"
 else curl --proto '=https' --proto-redir '=https' -fsSL --retry 3 "https://github.com/XTLS/Xray-core/releases/download/$xray_version/$asset" -o "$temporary/$asset";fi
 printf '%s  %s\n' "${checksum#sha256:}" "$temporary/$asset" | sha256sum -c -
 unzip -q "$temporary/$asset" geoip.dat geosite.dat LICENSE README.md -d "$temporary/$arch/xray"
 bash scripts/build-xray.sh "$arch" "$temporary/$arch/xray"
 printf '%s\n' "$xray_version" >"$temporary/$arch/xray/VERSION"
 (cd "$temporary/$arch/xray" && sha256sum xray geoip.dat geosite.dat source.tar.xz >SHA256SUMS)
 bash scripts/build-singbox.sh "$arch" "$temporary/$arch/singbox"
 find frontend/node_modules -type f \( -iname 'license*' -o -iname 'copying*' \) | while IFS= read -r file;do
  name="${file#frontend/node_modules/}";name="${name//\//_}";cp "$file" "$temporary/licenses/npm/$name"
 done
 cp -a "$temporary/licenses" "$temporary/$arch/"
 cp "$temporary/source.SHA256SUMS" "$temporary/$arch/source.SHA256SUMS"
 XZ_OPT='-T2 -6' tar -cJf "runtime/linux-$arch.tar.xz" -C "$temporary/$arch" bin frontend xray singbox licenses source.SHA256SUMS
done
# A build must not modify its source inputs after recording the manifest.
manifest >"$temporary/current.SHA256SUMS"
cmp "$temporary/source.SHA256SUMS" "$temporary/current.SHA256SUMS"
(cd runtime && sha256sum linux-amd64.tar.xz linux-arm64.tar.xz >SHA256SUMS)
bash scripts/verify-runtime-toolchain.sh
echo 'Verified runtime packages built for amd64 and arm64.'
