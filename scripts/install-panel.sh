#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${EUID:-$(id -u)} -eq 0 ]]||{ echo 'Run with sudo.' >&2;exit 1; }
cd "$(dirname "${BASH_SOURCE[0]}")/.."
source_dir="$PWD"
source scripts/common.sh
vr_log_init install
vr_install_lock
trap 'failure_status=$?;vr_diagnostics;vr_error "$LINENO" "$failure_status"' ERR
vr_step 'Configure database and local services' 'آماده‌سازی پایگاه داده و سرویس‌ها' 
prompt() { if [[ "${VELORAY_INSTALL_LANG:-en}" == fa ]];then printf '%s' "$2";else printf '%s' "$1";fi; }
ensure_local_database() {
 [[ -z "${DATABASE_URL:-}" && "${POSTGRES_HOST:-127.0.0.1}" =~ ^(127\.0\.0\.1|localhost)$ && "${POSTGRES_USER:-veloray}" == veloray && "${POSTGRES_DB:-veloray}" == veloray ]]||return 0
 local present
 present="$(runuser -u postgres -- psql -tAc "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='veloray')")"
 if [[ "${present//[[:space:]]/}" != t ]];then
  vr_run runuser -u postgres -- psql -v ON_ERROR_STOP=1 --set=password="$POSTGRES_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE veloray WITH LOGIN PASSWORD %L', :'password') \gexec
SQL
 fi
 present="$(runuser -u postgres -- psql -tAc "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname='veloray')")"
 [[ "${present//[[:space:]]/}" == t ]]||vr_run runuser -u postgres -- createdb -O veloray veloray
}
arch="$(uname -m)";case "$arch" in x86_64) arch=amd64;;aarch64|arm64) arch=arm64;;*) echo 'Unsupported CPU' >&2;exit 1;;esac
[[ -f go.mod && -f frontend/package-lock.json ]]||{ echo 'Extract a complete VeloRay release first.' >&2;exit 1; }
if [[ ! -x "bin/linux-$arch/veloray" || ! -f frontend/dist/index.html ]];then vr_run bash scripts/build.sh "$arch";fi
(cd "bin/linux-$arch" && sha256sum -c SHA256SUMS) >>"$VELORAY_INSTALL_LOG" 2>&1
vr_packages postgresql postgresql-client nginx openssl curl jq unzip certbot ca-certificates iproute2 python3
id veloray >/dev/null 2>&1||useradd --system --home /var/lib/veloray --shell /usr/sbin/nologin veloray
install -d -o veloray -g veloray -m 0750 /var/lib/veloray
install -d -m 0750 /etc/veloray /etc/veloray-node
chown root:veloray /etc/veloray
install -d -o root -g veloray -m 0750 /etc/veloray/templates
if [[ -d examples/subscription ]];then
 install -d -o root -g veloray -m 0750 /etc/veloray/templates/example /etc/veloray/templates/example/assets
 for item in examples/subscription/index.html examples/subscription/assets/*;do
  [[ -f "$item" ]]||continue
  target="/etc/veloray/templates/example/${item#examples/subscription/}"
  [[ -e "$target" ]]||install -o root -g veloray -m 0640 "$item" "$target"
 done
fi
# Seed owner-supplied compatibility examples without overwriting local edits.
if [[ -d examples/subscription/legacy ]];then
 install -d -o root -g veloray -m 0750 /etc/veloray/templates/legacy
 for item in examples/subscription/legacy/*.html;do
  [[ -f "$item" ]]||continue
  target="/etc/veloray/templates/legacy/${item##*/}"
  [[ -e "$target" ]]||install -o root -g veloray -m 0640 "$item" "$target"
 done
fi
install -d -m 0700 /var/lib/veloray-agent
install -d -m 0755 /opt/veloray/releases /usr/local/etc/xray /etc/nginx/sites-available /etc/nginx/sites-enabled
version="$(bin/linux-$arch/veloray version)"
release="$(mktemp -d "/opt/veloray/releases/$version-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")"
chmod 0755 "$release"
mkdir -p "$release/frontend"
cp -a frontend/dist "$release/frontend/"
if [[ -d /opt/veloray/current/frontend/dist/assets ]];then cp -an /opt/veloray/current/frontend/dist/assets/. "$release/frontend/dist/assets/";fi
cp -a frontend/package-lock.json frontend/package.json "$release/frontend/"
find "$release/frontend" -type d -exec chmod 0755 {} +
find "$release/frontend" -type f -exec chmod 0644 {} +
cp -a deploy scripts install.sh go.mod "$release/"
mkdir -p "$release/bin/linux-$arch"
cp -a "bin/linux-$arch/"* "$release/bin/linux-$arch/"

vr_run systemctl enable --now postgresql nginx
subscriptions_active=false
if systemctl is-active --quiet veloray-subscriptions.service;then subscriptions_active=true;fi
existing=false
if [[ -f /etc/veloray/veloray.env ]];then
 existing=true
 set -a;source /etc/veloray/veloray.env;set +a
 [[ "${POSTGRES_HOST:-}" != 127.0.1.0 ]]||export POSTGRES_HOST=127.0.0.1
 # Database and encryption keys are preserved. No account reset during upgrades.
 [[ -n "${VELORAY_FIELD_KEY:-}" && -n "${VELORAY_SECRET_KEY:-}" ]]||{ echo 'Existing secrets are missing.' >&2;exit 1; }
 ensure_local_database
 if [[ -x /usr/local/bin/veloray && ! -f /opt/veloray/src/backend/manage.py ]];then /usr/local/bin/veloray backup "/var/lib/veloray/backups-pre-upgrade-$(date -u +%Y%m%dT%H%M%SZ).tar.gz";else
  backup="/var/lib/veloray/legacy-pre-upgrade-$(date -u +%Y%m%dT%H%M%SZ)";install -d -m 0700 "$backup"
  PGHOST="${POSTGRES_HOST:-127.0.0.1}" PGPORT="${POSTGRES_PORT:-5432}" PGUSER="${POSTGRES_USER:-veloray}" PGPASSWORD="$POSTGRES_PASSWORD" pg_dump -Fc --no-owner --no-acl "${POSTGRES_DB:-veloray}" >"$backup/database.dump"
  cp -a /etc/veloray "$backup/";[[ ! -d /etc/veloray-node ]]||cp -a /etc/veloray-node "$backup/";cp -a /usr/local/etc/xray "$backup/";chmod -R go-rwx "$backup"
 fi
 systemctl stop veloray-subscription-endpoint.timer veloray-subscription-endpoint.service 2>/dev/null||true
 if $subscriptions_active;then vr_run systemctl stop veloray-subscriptions.service;fi
 systemctl stop veloray-web.service veloray-reconcile.timer veloray-reconcile.service 2>/dev/null||true
else
 vr_setup_initial
 public_url_host="$VELORAY_PUBLIC_HOST";[[ "$public_url_host" != *:* ]]||public_url_host="[$public_url_host]"
 dbpass="$(openssl rand -hex 32)"
 preserved="$(runuser -u postgres -- psql -tAc "SELECT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='veloray') OR EXISTS(SELECT 1 FROM pg_database WHERE datname='veloray')")"
 [[ "${preserved//[[:space:]]/}" != t ]]||{ echo 'PostgreSQL data exists but /etc/veloray/veloray.env is missing. Restore this environment file from your backup before reinstalling; its encryption keys and database credentials are required.' >&2;exit 1; }
 # Persist credentials before creating the database so interrupted installs can resume.
 env_temporary="$(mktemp /etc/veloray/.veloray.env.XXXXXX)"
 cat >"$env_temporary" <<ENV
VELORAY_SECRET_KEY=$(openssl rand -hex 48)
VELORAY_FIELD_KEY=$(openssl rand -hex 32)
VELORAY_PUBLIC_HOST=$VELORAY_PUBLIC_HOST
VELORAY_PANEL_PORT=$VELORAY_PANEL_PORT
VELORAY_SUBSCRIPTION_PORT=$VELORAY_SUBSCRIPTION_PORT
VELORAY_TLS_MODE=${VELORAY_TLS_MODE:-local}
VELORAY_PUBLIC_URL=https://$public_url_host:$VELORAY_PANEL_PORT
VELORAY_LISTEN=127.0.0.1:8610
VELORAY_COOKIE_SECURE=true
VELORAY_LOCAL_NODE_TOKEN=$(openssl rand -hex 32)
VELORAY_FRONTEND_DIR=/opt/veloray/current/frontend/dist
POSTGRES_HOST=127.0.0.1
POSTGRES_PORT=5432
POSTGRES_DB=veloray
POSTGRES_USER=veloray
POSTGRES_PASSWORD=$dbpass
VELORAY_LANGUAGE=${VELORAY_INSTALL_LANG:-en}
VELORAY_REPO=${VELORAY_REPO:-ParsaZali/VeloRay}
VELORAY_REF=${VELORAY_REF:-main}
ENV
 mv "$env_temporary" /etc/veloray/veloray.env
 set -a;source /etc/veloray/veloray.env;set +a
 [[ "${POSTGRES_HOST:-}" != 127.0.1.0 ]]||export POSTGRES_HOST=127.0.0.1
 ensure_local_database
fi
if $existing;then vr_run systemctl stop veloray-agent.service;fi
if ! vr_core_is_official xray && [[ -n "${VELORAY_XRAY_DIRECTORY:-}" || ! -x /usr/local/bin/xray ]];then vr_run bash scripts/xray-install.sh;fi
if vr_core_is_official singbox;then :;elif [[ -n "${VELORAY_SINGBOX_DIRECTORY:-}" ]];then vr_run bash scripts/singbox-install.sh;elif [[ ! -x /usr/local/bin/sing-box ]];then echo 'Use the complete VeloRay runtime package to install sing-box.' >&2;exit 1;fi
vr_prepare_managed_cores
chown root:veloray /etc/veloray/veloray.env;chmod 0640 /etc/veloray/veloray.env
# All changes below use the candidate binary. Keep the previous release for rollback.
install -m 0755 "bin/linux-$arch/veloray" /usr/local/bin/veloray
install -m 0755 "bin/linux-$arch/veloray-agent" /usr/local/bin/veloray-agent
install -m 0755 scripts/velorayctl /usr/local/bin/velorayctl
export VELORAY_PUBLIC_HOST="${VELORAY_PUBLIC_HOST:-localhost}"
export VELORAY_PANEL_PORT="${VELORAY_PANEL_PORT:-8443}"
VELORAY_LOCAL_NODE_TOKEN="${VELORAY_LOCAL_NODE_TOKEN:-$(openssl rand -hex 32)}"
public_url_host="$VELORAY_PUBLIC_HOST";[[ "$public_url_host" != *:* ]]||public_url_host="[$public_url_host]"
export VELORAY_PUBLIC_URL="https://$public_url_host:$VELORAY_PANEL_PORT"
vr_step 'Configure panel and administrator' 'تنظیم پنل و حساب مدیر'
vr_run veloray set-env VELORAY_LANGUAGE "${VELORAY_INSTALL_LANG:-${VELORAY_LANGUAGE:-en}}"
vr_run veloray set-env VELORAY_REPO "${VELORAY_INSTALL_REPO:-${VELORAY_REPO:-ParsaZali/VeloRay}}"
vr_run veloray set-env VELORAY_REF "${VELORAY_TRACKING_REF:-${VELORAY_INSTALL_REF:-${VELORAY_REF:-main}}}"
veloray set-env VELORAY_PUBLIC_URL "$VELORAY_PUBLIC_URL"
veloray set-env VELORAY_FRONTEND_DIR /opt/veloray/current/frontend/dist
veloray set-env VELORAY_LOCAL_NODE_TOKEN "$VELORAY_LOCAL_NODE_TOKEN"
veloray set-env VELORAY_LISTEN 127.0.0.1:8610
veloray set-env VELORAY_COOKIE_SECURE true
veloray set-env VELORAY_SUBSCRIPTION_DRIVER native
veloray set-env POSTGRES_HOST "${POSTGRES_HOST:-127.0.0.1}"
vr_run veloray migrate
if $existing;then
 legacy="$(PGHOST="${POSTGRES_HOST:-127.0.0.1}" PGPORT="${POSTGRES_PORT:-5432}" PGUSER="${POSTGRES_USER:-veloray}" PGPASSWORD="$POSTGRES_PASSWORD" psql -tAc "SELECT to_regclass('core_node') IS NOT NULL AND NOT EXISTS(SELECT 1 FROM vr_users)" "${POSTGRES_DB:-veloray}")"
 if [[ "${legacy//[[:space:]]/}" == t ]];then vr_run veloray import-legacy --dry-run;vr_run veloray import-legacy;fi
fi
has_admin="$(PGHOST="${POSTGRES_HOST:-127.0.0.1}" PGPORT="${POSTGRES_PORT:-5432}" PGUSER="${POSTGRES_USER:-veloray}" PGPASSWORD="$POSTGRES_PASSWORD" psql -tAc "SELECT EXISTS(SELECT 1 FROM vr_users WHERE data->>'is_superuser'='true')" "${POSTGRES_DB:-veloray}")"
if [[ "${has_admin//[[:space:]]/}" != t ]];then
 export VELORAY_ADMIN_USERNAME="${VELORAY_ADMIN_USERNAME:-vr_$(openssl rand -hex 4)}"
 export VELORAY_ADMIN_PASSWORD="${VELORAY_ADMIN_PASSWORD:-$(openssl rand -hex 16)}"
 vr_run veloray set-env VELORAY_ADMIN_USERNAME "$VELORAY_ADMIN_USERNAME"
 credentials_file=/etc/veloray/install-result.env
 (umask 077;printf 'PANEL_URL=%q\nUSERNAME=%q\nPASSWORD=%q\n' "$VELORAY_PUBLIC_URL" "$VELORAY_ADMIN_USERNAME" "$VELORAY_ADMIN_PASSWORD" >"$credentials_file")
 chmod 0600 "$credentials_file"
 vr_run veloray bootstrap
 unset VELORAY_ADMIN_PASSWORD
fi
vr_run veloray bootstrap-local
# Preserve existing Xray configuration. The upgraded agent can harvest the old API address.
if [[ ! -f /usr/local/etc/xray/config.json ]];then
 cat >/usr/local/etc/xray/config.json <<'JSON'
{"log":{"loglevel":"warning"},"api":{"tag":"api","services":["StatsService"]},"stats":{},"policy":{"levels":{"0":{"statsUserUplink":true,"statsUserDownlink":true}},"system":{"statsInboundUplink":true,"statsInboundDownlink":true}},"inbounds":[{"tag":"api","listen":"127.0.0.1","port":10085,"protocol":"dokodemo-door","settings":{"address":"127.0.0.1"}}],"outbounds":[{"protocol":"freedom","tag":"direct"}],"routing":{"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}}
JSON
fi
vr_step 'Validate Xray configuration and file access' 'بررسی پیکربندی و دسترسی فایل‌های Xray'
vr_xray_prepare
if ! vr_xray_check;then vr_say 'Xray rejected the existing configuration. Inspect the log before retrying.' 'پیکربندی فعلی Xray رد شد؛ گزارش خطا را بررسی کنید.' >&2;vr_error "$LINENO" 1;fi
cat >/etc/veloray-node/agent.env <<ENV
VELORAY_NODE_ID=local
VELORAY_AGENT_LISTEN=127.0.0.1:9191
VELORAY_AGENT_TOKEN=$VELORAY_LOCAL_NODE_TOKEN
VELORAY_XRAY_BINARY=/usr/local/bin/xray
VELORAY_XRAY_CONFIG=/usr/local/etc/xray/config.json
VELORAY_XRAY_SERVICE=xray.service
VELORAY_AGENT_STATE=/var/lib/veloray-agent/state.json
VELORAY_CORE_DIRECTORY=/var/lib/veloray-cores
ENV
chmod 0600 /etc/veloray-node/agent.env
install -m 0644 deploy/systemd/{veloray-web,veloray-agent,veloray-subscriptions,veloray-subscription-endpoint,xray}.service /etc/systemd/system/
install -m 0644 deploy/systemd/veloray-subscription-endpoint.timer /etc/systemd/system/
ln -sfn "$release" /opt/veloray/current
systemctl disable --now veloray-reconcile.timer 2>/dev/null||true
vr_step 'Start HTTPS and panel services' 'راه‌اندازی HTTPS و سرویس‌های پنل'
vr_run systemctl daemon-reload
vr_run systemctl enable sing-box.service
vr_run systemctl restart sing-box.service
vr_run systemctl enable xray.service veloray-agent.service veloray-web.service
vr_run systemctl restart veloray-agent.service
vr_run systemctl enable veloray-subscriptions.service
vr_run systemctl restart veloray-subscriptions.service
vr_wait http://127.0.0.1:8611/api/health
vr_run veloray render-nginx
vr_run systemctl restart veloray-web.service
vr_wait http://127.0.0.1:8610/api/health
vr_run systemctl enable --now veloray-subscription-endpoint.timer
vr_step 'Check VPN ports and resolve conflicts' 'بررسی پورت‌های VPN و رفع تداخل'
if ! vr_resolve_ports;then
 # Stop an already failing Xray loop; never stop the process occupying its port.
 if ! systemctl is-active --quiet xray.service;then systemctl stop xray.service >>"$VELORAY_INSTALL_LOG" 2>&1||true;fi
 vr_say "The panel is available at $VELORAY_PUBLIC_URL; VPN runtime needs a free port." "پنل در $VELORAY_PUBLIC_URL در دسترس است؛ سرویس VPN به پورت آزاد نیاز دارد." >&2
 vr_say 'After changing the inbound port, run sudo veloray repair.' 'پس از تغییر پورت اتصال، sudo veloray repair را اجرا کنید.' >&2
 vr_error "$LINENO" 1
fi
vr_run systemctl reset-failed xray.service
vr_run systemctl restart xray.service
install -d -m 0755 /etc/letsencrypt/renewal-hooks/deploy
cat >/etc/letsencrypt/renewal-hooks/deploy/veloray-nginx <<'HOOK'
#!/usr/bin/env bash
systemctl reload nginx
HOOK
chmod 0755 /etc/letsencrypt/renewal-hooks/deploy/veloray-nginx

vr_step 'Verify panel, Xray and traffic accounting' 'بررسی سلامت پنل، Xray و ثبت ترافیک'
vr_wait http://127.0.0.1:8610/api/health
vr_wait http://127.0.0.1:9191/health "$VELORAY_LOCAL_NODE_TOKEN"
vr_run velorayctl doctor
# Record the immutable source revision only after all post-install health checks pass.
if [[ "${VELORAY_INSTALL_COMMIT:-}" =~ ^[a-f0-9]{40}$ ]];then
 printf '%s\n' "$VELORAY_INSTALL_COMMIT" >/var/lib/veloray/installed-commit
 chmod 0644 /var/lib/veloray/installed-commit
fi
if [[ "${VELORAY_TLS_MODE:-local}" == auto && ! -f "/etc/letsencrypt/live/$VELORAY_PUBLIC_HOST/fullchain.pem" ]];then
 vr_step 'Issue domain certificate' 'دریافت گواهی دامنه'
 certificate_args=(--register-unsafely-without-email)
 [[ -z "${VELORAY_ACME_EMAIL:-}" ]]||certificate_args=(--email "$VELORAY_ACME_EMAIL")
 if vr_run certbot certonly --webroot --webroot-path /var/lib/veloray-acme --preferred-challenges http --agree-tos --non-interactive "${certificate_args[@]}" -d "$VELORAY_PUBLIC_HOST";then
  vr_run veloray render-nginx
  vr_success 'Trusted HTTPS certificate installed.' 'گواهی معتبر HTTPS نصب شد.'
 else
  vr_warn 'Certificate issuance failed. Check domain DNS and port 80, then run veloray ssl EMAIL. Local HTTPS remains available.' 'صدور SSL ناموفق بود؛ DNS دامنه و پورت ۸۰ را بررسی کنید و veloray ssl EMAIL را اجرا کنید. HTTPS محلی فعال است.'
 fi
fi
vr_success 'Installation completed. All health checks passed.' 'نصب کامل شد و تمام بررسی‌های سلامت موفق بود.'
if [[ -n "${credentials_file:-}" ]];then
 source "$credentials_file"
 printf '\n  Username: %s\n  Password: %s\n' "$USERNAME" "$PASSWORD"
 vr_say "Saved privately: $credentials_file" "ذخیره خصوصی: $credentials_file"
 unset PASSWORD
fi
printf '\n  %s\n' "$VELORAY_PUBLIC_URL"
vr_say 'Management menu: sudo veloray' 'منوی مدیریت: sudo veloray'
vr_say 'Administrator password: sudo veloray reset' 'تغییر رمز مدیر: sudo veloray reset'
if [[ ! -f "/etc/letsencrypt/live/$VELORAY_PUBLIC_HOST/fullchain.pem" ]];then
 vr_say 'HTTPS uses a local certificate. After configuring a domain, run: veloray ssl EMAIL' 'HTTPS با گواهی محلی فعال است؛ پس از تنظیم دامنه: veloray ssl EMAIL'
fi
