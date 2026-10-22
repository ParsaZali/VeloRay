#!/usr/bin/env bash
set -Eeuo pipefail
[[ -n "${VELORAY_SINGBOX_DIRECTORY:-}" && -f "$VELORAY_SINGBOX_DIRECTORY/SHA256SUMS" ]]||{ echo 'A verified sing-box runtime directory is required.' >&2;exit 1; }
(cd "$VELORAY_SINGBOX_DIRECTORY" && sha256sum -c SHA256SUMS)
candidate_binary="$(mktemp /usr/local/bin/.veloray-sing-box.XXXXXX)"
install -m 0755 "$VELORAY_SINGBOX_DIRECTORY/sing-box" "$candidate_binary"
mv -Tf "$candidate_binary" /usr/local/bin/sing-box
install -d -m 0755 /usr/local/etc/sing-box
if [[ ! -f /usr/local/etc/sing-box/config.json ]];then
 cat >/usr/local/etc/sing-box/config.json <<'JSON'
{"log":{"level":"info","timestamp":true},"inbounds":[],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"},"experimental":{"v2ray_api":{"listen":"127.0.0.1:10086","stats":{"enabled":true,"users":[],"inbounds":[],"outbounds":[]}},"clash_api":{"external_controller":"127.0.0.1:10087"}}}
JSON
fi
id veloray-singbox >/dev/null 2>&1||useradd --system --gid nogroup --home /var/lib/veloray-agent --shell /usr/sbin/nologin veloray-singbox
chown root:nogroup /usr/local/etc/sing-box/config.json
chmod 0640 /usr/local/etc/sing-box/config.json
/usr/local/bin/sing-box check -c /usr/local/etc/sing-box/config.json
install -m 0644 deploy/systemd/sing-box.service /etc/systemd/system/
