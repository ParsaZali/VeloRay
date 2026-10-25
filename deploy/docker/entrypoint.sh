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
 agent)
  # Writable, durable executable slots preserve official updates across recreation.
  for core in xray singbox;do
   binary=/usr/local/bin/xray;[[ "$core" != singbox ]]||binary=/usr/local/bin/sing-box
   slot="/var/lib/veloray-agent/cores/$core"
   mkdir -p "$slot"
   if [[ ! -e "$slot/current" && ! -L "$slot/current" ]];then
    digest="$(sha256sum "$binary" | cut -d' ' -f1)"
    cp "$binary" "$slot/binary-$digest";chmod 0755 "$slot/binary-$digest"
    ln -s "$slot/binary-$digest" "$slot/current"
    printf '{"tag":"bundled","source":"bundled","sha256":"%s"}\n' "$digest" >"$slot/active.json"
   fi
   [[ -L "$slot/entry" ]]||ln -s "$slot/current" "$slot/entry"
  done
  mkdir -p /var/lib/veloray-agent/assets/xray
  for asset in geoip.dat geosite.dat;do
   [[ -f "/var/lib/veloray-agent/assets/xray/$asset" ]]||cp "/opt/veloray/xray/$asset" "/var/lib/veloray-agent/assets/xray/$asset"
  done
  exec veloray-agent
  ;;
 health-panel) exec curl --fail --silent --show-error --max-time 3 "http://${VELORAY_LISTEN:-127.0.0.1:8610}/api/health" ;;
 health-agent)
  token="$(cat "${VELORAY_AGENT_TOKEN_FILE:?}")"
  [[ "$token" =~ ^[A-Za-z0-9_-]{32,128}$ ]] || exit 1
  printf 'header = "Authorization: Bearer %s"\n' "$token" | curl --config - --fail --silent --show-error --max-time 3 "http://${VELORAY_AGENT_LISTEN:-127.0.0.1:9191}/health" | jq -e '.status=="ok" and .xray.running==true and .singbox.running==true' >/dev/null
  ;;
 *) exec "$@" ;;
esac
