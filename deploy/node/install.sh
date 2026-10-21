#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${EUID:-$(id -u)} -eq 0 ]]||{ echo 'Run with sudo.' >&2;exit 1; }
cd "$(dirname "$0")/../.."
source scripts/common.sh
vr_log_init install
vr_install_lock
trap 'failure_status=$?;vr_diagnostics;vr_error "$LINENO" "$failure_status"' ERR
vr_step 'Prepare node services' 'آماده‌سازی سرویس‌های نود'
arch="$(uname -m)";case "$arch" in x86_64) arch=amd64;;aarch64|arm64) arch=arm64;;*) exit 1;;esac
[[ -x "bin/linux-$arch/veloray-agent" ]]||{ echo 'Build the agent first or use a release archive.' >&2;exit 1; }
(cd "bin/linux-$arch" && sha256sum -c SHA256SUMS) >>"$VELORAY_INSTALL_LOG" 2>&1
vr_packages openssl curl jq unzip ca-certificates iproute2
if [[ -n "${VELORAY_XRAY_DIRECTORY:-}" || ! -x /usr/local/bin/xray ]];then vr_run bash scripts/xray-install.sh;fi
if [[ -n "${VELORAY_SINGBOX_DIRECTORY:-}" ]];then vr_run bash scripts/singbox-install.sh;elif [[ ! -x /usr/local/bin/sing-box ]];then echo 'Use the complete VeloRay runtime package to install sing-box.' >&2;exit 1;fi
install -d -m 0750 /etc/veloray-node
install -d -m 0700 /var/lib/veloray-agent
install -d -m 0755 /usr/local/etc/xray
if [[ -f /etc/veloray-node/agent.env ]];then
 set -a;source /etc/veloray-node/agent.env;set +a
 # Upgrade in place; preserve TLS keys, bearer token, accounting ledger and Xray config.
 cp -a /etc/veloray-node "/etc/veloray-node.backup-$(date -u +%Y%m%dT%H%M%SZ)"
else
 [[ -n "${VELORAY_PUBLIC_HOST:-}" ]]||read -rp 'Node public host: ' VELORAY_PUBLIC_HOST
 [[ "$VELORAY_PUBLIC_HOST" =~ ^[a-zA-Z0-9.:-]+$ ]]||exit 2
 VELORAY_AGENT_TOKEN="${VELORAY_AGENT_TOKEN:-$(openssl rand -hex 32)}"
 [[ ${#VELORAY_AGENT_TOKEN} -ge 32 ]]||exit 2
 port="${VELORAY_AGENT_PORT:-9191}"
 [[ "$port" =~ ^[0-9]+$ ]]&&((port>0&&port<65536&&port!=10085))||exit 2
 install -d -m 0700 /etc/veloray-node/tls
 san="DNS:$VELORAY_PUBLIC_HOST";[[ "$VELORAY_PUBLIC_HOST" != *:* && ! "$VELORAY_PUBLIC_HOST" =~ ^[0-9.]+$ ]]||san="IP:$VELORAY_PUBLIC_HOST"
 openssl req -x509 -newkey rsa:3072 -sha256 -nodes -days 365 -keyout /etc/veloray-node/tls/agent.key -out /etc/veloray-node/tls/agent.crt -subj "/CN=$VELORAY_PUBLIC_HOST" -addext "subjectAltName=$san" >/dev/null 2>&1
 chmod 0600 /etc/veloray-node/tls/agent.key
 cat >/etc/veloray-node/agent.env <<ENV
VELORAY_NODE_ID=$VELORAY_PUBLIC_HOST
VELORAY_AGENT_LISTEN=0.0.0.0:$port
VELORAY_AGENT_TOKEN=$VELORAY_AGENT_TOKEN
VELORAY_NODE_CERT=/etc/veloray-node/tls/agent.crt
VELORAY_NODE_KEY=/etc/veloray-node/tls/agent.key
VELORAY_XRAY_BINARY=/usr/local/bin/xray
VELORAY_XRAY_CONFIG=/usr/local/etc/xray/config.json
VELORAY_XRAY_SERVICE=xray.service
VELORAY_AGENT_STATE=/var/lib/veloray-agent/state.json
ENV
 chmod 0600 /etc/veloray-node/agent.env
fi
# Bootstrap with no public listeners; the panel deploys the requested inbounds.
if [[ ! -f /usr/local/etc/xray/config.json ]];then
 cat >/usr/local/etc/xray/config.json <<'JSON'
{"log":{"loglevel":"warning"},"api":{"tag":"api","services":["StatsService"]},"stats":{},"policy":{"levels":{"0":{"statsUserUplink":true,"statsUserDownlink":true}},"system":{"statsInboundUplink":true,"statsInboundDownlink":true}},"inbounds":[{"tag":"api","listen":"127.0.0.1","port":10085,"protocol":"dokodemo-door","settings":{"address":"127.0.0.1"}}],"outbounds":[{"protocol":"freedom","tag":"direct"}],"routing":{"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}}
JSON
fi
vr_step 'Validate Xray configuration' 'بررسی پیکربندی Xray'
vr_xray_prepare
vr_xray_check
install -m 0755 "bin/linux-$arch/veloray-agent" /usr/local/bin/veloray-agent
install -m 0644 deploy/systemd/{veloray-agent,xray}.service /etc/systemd/system/
vr_step 'Start and verify the node' 'راه‌اندازی و بررسی سلامت نود'
vr_run systemctl daemon-reload
vr_run systemctl enable sing-box.service
vr_run systemctl restart sing-box.service
vr_run systemctl enable xray.service veloray-agent.service
vr_ports_check
vr_run systemctl reset-failed xray.service
vr_run systemctl restart xray.service veloray-agent.service
for unit in veloray-agent.service xray.service;do vr_run systemctl is-active --quiet "$unit";done
set -a;source /etc/veloray-node/agent.env;set +a
VELORAY_PUBLIC_HOST="${VELORAY_PUBLIC_HOST:-$VELORAY_NODE_ID}"
node_port="${VELORAY_AGENT_LISTEN##*:}"
node_url_host="$VELORAY_PUBLIC_HOST";[[ "$node_url_host" != *:* ]]||node_url_host="[$node_url_host]"
healthy=false
for attempt in {1..45};do
 if body="$(curl -fsS --max-time 3 --cacert "$VELORAY_NODE_CERT" --connect-to ":$node_port:127.0.0.1:$node_port" -H "Authorization: Bearer $VELORAY_AGENT_TOKEN" "https://$node_url_host:$node_port/health" 2>>"$VELORAY_INSTALL_LOG")" && jq -e '.status=="ok" and .xray.running==true' <<<"$body" >/dev/null 2>>"$VELORAY_INSTALL_LOG";then healthy=true;break;fi
 sleep 1
done
$healthy||{ vr_diagnostics;vr_error "$LINENO" 1; }
vr_say 'Node installed and verified. Restrict the agent port to the panel server.' 'نود نصب شد و بررسی سلامت موفق بود؛ دسترسی پورت نود را به سرور پنل محدود کنید.'
printf 'Token: %s\nCertificate SHA-256: ' "$VELORAY_AGENT_TOKEN"
openssl x509 -in "${VELORAY_NODE_CERT:-/etc/veloray-node/tls/agent.crt}" -noout -fingerprint -sha256
printf 'For verified TLS, use a trusted certificate or add this certificate to the panel CA bundle.\n'
