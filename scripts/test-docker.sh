#!/usr/bin/env bash
# Isolated, disposable deployment. No existing named project/volume is reused.
set -Eeuo pipefail
cd "$(dirname -- "${BASH_SOURCE[0]}")/.."
command -v docker >/dev/null
test "${VELORAY_DOCKER_TEST:-}" = 1 || { echo 'Set VELORAY_DOCKER_TEST=1 to create a disposable local Docker deployment.' >&2; exit 2; }
temporary="$(mktemp -d)"
project="veloray-ci-$(date +%s)-$RANDOM"
export VELORAY_SECRET_DIRECTORY="$temporary/secrets"
export VELORAY_CERTIFICATE_DIRECTORY="$temporary/certificates"
python3 deploy/docker/generate.py --domain localhost --directory "$temporary"
compose=(docker compose --project-name "$project" --env-file "$temporary/.env" -f deploy/docker/compose.yaml)
cleanup() {
 status=$?
 if ((status));then "${compose[@]}" ps || true; "${compose[@]}" logs --tail 40 agent panel subscriptions proxy || true;fi
 "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
 python3 - "$temporary" <<'PY'
import shutil, sys
shutil.rmtree(sys.argv[1])
PY
 exit "$status"
}
trap cleanup EXIT
"${compose[@]}" config --quiet
"${compose[@]}" build panel proxy
"${compose[@]}" up -d --no-build --wait --wait-timeout 150
export VELORAY_TEST_COMPOSE_PROJECT="$project" VELORAY_TEST_COMPOSE_ENV="$temporary/.env"
python3 scripts/test-docker.py
echo 'Docker HTTPS, authentication, native traffic, restart and persistent-accounting checks passed.'
