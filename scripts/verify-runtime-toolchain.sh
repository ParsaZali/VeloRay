#!/usr/bin/env bash
# Inspect Go build metadata for every engine and panel binary, including ARM64.
set -Eeuo pipefail
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
expected=go1.26.10
temporary="$(mktemp -d)";trap 'rm -rf -- "$temporary"' EXIT
for arch in amd64 arm64;do
 mkdir -p "$temporary/$arch"
 tar --no-same-owner --no-same-permissions -xJf "runtime/linux-$arch.tar.xz" -C "$temporary/$arch" \
  "bin/linux-$arch/veloray" "bin/linux-$arch/veloray-agent" xray/xray singbox/sing-box
 for relative in "bin/linux-$arch/veloray" "bin/linux-$arch/veloray-agent" xray/xray singbox/sing-box;do
  binary="$temporary/$arch/$relative"
  metadata="$(go version -m "$binary")"
  first="${metadata%%$'\n'*}"
  [[ "$first" == "$binary: $expected" ]]||{ echo "Unexpected Go toolchain for $arch/$relative: $first" >&2;exit 1; }
 done
 echo "Verified patched $expected toolchain for all $arch binaries."
done
