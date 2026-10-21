#!/usr/bin/env bash
set -Eeuo pipefail
export GOTOOLCHAIN=go1.26.10
cd "$(dirname "$0")/.."
command -v go >/dev/null || { echo 'Go 1.26.10+ is required' >&2; exit 1; }
command -v npm >/dev/null || { echo 'Node.js 22.12+ and npm are required' >&2; exit 1; }
(cd frontend && npm ci --no-audit --no-fund && npm run build)
arch="${1:-$(go env GOARCH)}"
case "$arch" in amd64|arm64) ;; *) echo 'Supported architectures: amd64, arm64' >&2; exit 2;; esac
mkdir -p "bin/linux-$arch"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -buildvcs=false -trimpath -ldflags='-s -w' -o "bin/linux-$arch/veloray" ./cmd/veloray
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -buildvcs=false -trimpath -ldflags='-s -w' -o "bin/linux-$arch/veloray-agent" ./cmd/veloray-agent
(cd "bin/linux-$arch" && sha256sum veloray veloray-agent > SHA256SUMS)
