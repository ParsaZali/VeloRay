#!/usr/bin/env bash
set -euo pipefail
umask 077
case "${1:-panel}" in
 panel)
  veloray migrate
  veloray bootstrap-if-empty </dev/null
  veloray bootstrap-local
  exec veloray serve
  ;;
 subscriptions) exec veloray serve ;;
 agent) exec veloray-agent ;;
 health-panel) exec curl --fail --silent --show-error --max-time 3 "http://${VELORAY_LISTEN:-127.0.0.1:8610}/api/health" ;;
 health-agent)
  token="$(cat "${VELORAY_AGENT_TOKEN_FILE:?}")"
  [[ "$token" =~ ^[A-Za-z0-9_-]{32,128}$ ]] || exit 1
  printf 'header = "Authorization: Bearer %s"\n' "$token" | curl --config - --fail --silent --show-error --max-time 3 "http://${VELORAY_AGENT_LISTEN:-127.0.0.1:9191}/health" | jq -e '.status=="ok" and .xray.running==true and .singbox.running==true' >/dev/null
  ;;
 *) exec "$@" ;;
esac
