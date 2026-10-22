#!/usr/bin/env bash
# Shared host operations. Sourcing this file never changes services or data.
vr_text() { if [[ "${VELORAY_INSTALL_LANG:-${VELORAY_LANGUAGE:-en}}" == fa ]];then printf '%s' "$2";else printf '%s' "$1";fi; }
vr_say() { vr_text "$1" "$2";printf '\n'; }
vr_color() {
 local color="$1";shift
 if [[ -t 1 && "${TERM:-dumb}" != dumb && -z "${NO_COLOR:-}" ]];then printf '\033[%sm%s\033[0m' "$color" "$*";else printf '%s' "$*";fi
}
vr_success() { vr_color 32 "  ✓ $(vr_text "$1" "$2")";printf '\n'; }
vr_warn() { vr_color 33 "  ! $(vr_text "$1" "$2")";printf '\n'; }
vr_failure() { vr_color 31 "  ✗ $(vr_text "$1" "$2")";printf '\n'; }
vr_random_port() {
 python3 - "$@" <<'PY'
import secrets, socket, sys
reserved = {80, 5432, 8610, 8611, 8612, 9191, 10085, 10086, 10087, *map(int, sys.argv[1:])}
for _ in range(300):
    port = 1024 + secrets.randbelow(64512)
    if port in reserved:
        continue
    tcp, udp = socket.socket(), socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        tcp.bind(('0.0.0.0', port)); udp.bind(('0.0.0.0', port))
        print(port)
        break
    except OSError:
        pass
    finally:
        tcp.close(); udp.close()
else:
    raise SystemExit('Cannot find a free port')
PY
}
vr_setup_initial() {
 local answer host_kind
 if [[ -z "${VELORAY_PUBLIC_HOST:-}" ]];then
  if [[ -t 0 ]];then
   read -rp "$(vr_text 'Do you have a domain pointing to this server? [Y/n]: ' 'دامنه‌ای که به این سرور وصل باشد دارید؟ [Y/n]: ')" answer
   case "${answer,,}" in n|no) read -rp "$(vr_text 'Server public IP: ' 'آی‌پی عمومی سرور: ')" VELORAY_PUBLIC_HOST;;*) read -rp "$(vr_text 'Domain: ' 'دامنه: ')" VELORAY_PUBLIC_HOST;;esac
  else
   vr_failure 'Set VELORAY_PUBLIC_HOST for unattended installation.' 'برای نصب خودکار VELORAY_PUBLIC_HOST را مشخص کنید.' >&2;return 2
  fi
 fi
 [[ "$VELORAY_PUBLIC_HOST" =~ ^[a-zA-Z0-9.:-]+$ ]]||{ vr_failure 'Invalid host.' 'دامنه یا آی‌پی نامعتبر است.' >&2;return 2; }
 if [[ -z "${VELORAY_PANEL_PORT:-}" && -t 0 ]];then read -rp "$(vr_text 'Panel port [Enter = random]: ' 'پورت پنل [Enter = تصادفی]: ')" VELORAY_PANEL_PORT;fi
 VELORAY_PANEL_PORT="${VELORAY_PANEL_PORT:-$(vr_random_port)}"
 [[ "$VELORAY_PANEL_PORT" =~ ^[0-9]+$ ]]&&((VELORAY_PANEL_PORT>0&&VELORAY_PANEL_PORT<65536))&&[[ ! " 80 5432 8610 8611 8612 9191 10085 10086 10087 " =~ " $VELORAY_PANEL_PORT " ]]||{ echo 'Invalid or reserved panel port.' >&2;return 2; }
 if [[ -z "${VELORAY_SUBSCRIPTION_PORT:-}" && -t 0 ]];then read -rp "$(vr_text 'Subscription port [Enter = random]: ' 'پورت اشتراک [Enter = تصادفی]: ')" VELORAY_SUBSCRIPTION_PORT;fi
 VELORAY_SUBSCRIPTION_PORT="${VELORAY_SUBSCRIPTION_PORT:-$(vr_random_port "$VELORAY_PANEL_PORT")}"
 [[ "$VELORAY_SUBSCRIPTION_PORT" =~ ^[0-9]+$ ]]&&((VELORAY_SUBSCRIPTION_PORT>0&&VELORAY_SUBSCRIPTION_PORT<65536))&&[[ ! " 80 5432 8610 8611 8612 9191 10085 10086 10087 $VELORAY_PANEL_PORT " =~ " $VELORAY_SUBSCRIPTION_PORT " ]]||{ echo 'Invalid or reserved subscription port.' >&2;return 2; }
 host_kind="$(python3 - "$VELORAY_PUBLIC_HOST" <<'PY'
import ipaddress,sys
try: ipaddress.ip_address(sys.argv[1]);print('ip')
except ValueError: print('domain')
PY
)"
 if [[ "$host_kind" == domain ]];then
  VELORAY_TLS_MODE="${VELORAY_TLS_MODE:-auto}"
  if [[ -t 0 && -z "${VELORAY_ACME_EMAIL:-}" ]];then read -rp "$(vr_text 'Certificate email [optional]: ' 'ایمیل گواهی SSL [اختیاری]: ')" VELORAY_ACME_EMAIL;fi
 else VELORAY_TLS_MODE=local;fi
 [[ "$VELORAY_TLS_MODE" == auto || "$VELORAY_TLS_MODE" == local ]]||{ echo 'TLS mode must be auto or local.' >&2;return 2; }
 export VELORAY_PUBLIC_HOST VELORAY_PANEL_PORT VELORAY_SUBSCRIPTION_PORT VELORAY_TLS_MODE
}
vr_core_is_official() { [[ -x "/var/lib/veloray-cores/$1/current" ]]&&jq -e '.source=="official"' "/var/lib/veloray-cores/$1/active.json" >/dev/null 2>&1; }
vr_prepare_managed_cores() {
 local core binary slot checksum target previous
 install -d -m 0755 /var/lib/veloray-cores
 for core in xray singbox;do
  binary=/usr/local/bin/xray;[[ "$core" != singbox ]]||binary=/usr/local/bin/sing-box
  [[ -x "$binary" ]]||continue
  slot="/var/lib/veloray-cores/$core";install -d -m 0755 "$slot"
  if [[ -L "$binary" && "$(readlink "$binary")" == "$slot/current" ]]&&vr_core_is_official "$core";then continue;fi
  checksum="$(sha256sum "$binary" | cut -d' ' -f1)";target="$slot/binary-$checksum"
  [[ -f "$target" ]]||install -m 0755 "$binary" "$target"
  previous="$(readlink "$slot/current" 2>/dev/null||true)"
  if [[ "$previous" == "$target" && -f "$slot/active.json" ]];then ln -sfn "$slot/current" "$binary";continue;fi
  ln -sfn "$target" "$slot/current"
  ln -sfn "$slot/current" "$binary"
  jq -n --arg sha "$checksum" --arg previous "$previous" '{tag:"bundled",source:"bundled",sha256:$sha,previous_path:$previous,previous_tag:"bundled",previous_source:"bundled"}' >"$slot/active.json"
  chmod 0644 "$slot/active.json"
 done
}
vr_step() { VR_STEP="$(vr_text "$1" "$2")";printf '\n  • %s\n' "$VR_STEP"; }
vr_log_init() {
 VR_OPERATION_STARTED="$(date --iso-8601=seconds)"
 local previous_mask="$(umask)"
 umask 077
 if [[ -z "${VELORAY_INSTALL_LOG:-}" ]];then
  install -d -m 0700 /var/log/veloray
  VELORAY_INSTALL_LOG="$(mktemp "/var/log/veloray/${1:-operation}-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX.log")"
 fi
 [[ -f "$VELORAY_INSTALL_LOG" && ! -L "$VELORAY_INSTALL_LOG" ]]||{ echo 'Invalid operation log.' >&2;return 1; }
 chmod 0600 "$VELORAY_INSTALL_LOG"
 export VELORAY_INSTALL_LOG
 umask "$previous_mask"
}
vr_run() { "$@" >>"$VELORAY_INSTALL_LOG" 2>&1; }
vr_error() {
 local line="$1" status="${2:-1}"
 printf 'Failure: step=%s line=%s status=%s\n' "${VR_STEP:-operation}" "$line" "$status" >>"$VELORAY_INSTALL_LOG"
 vr_failure "Failed: ${VR_STEP:-operation}." "مرحله ناموفق: ${VR_STEP:-عملیات}." >&2
 vr_say "Details: sudo tail -n 60 $VELORAY_INSTALL_LOG" "جزئیات: sudo tail -n 60 $VELORAY_INSTALL_LOG" >&2
 exit "$status"
}
vr_install_lock() {
 [[ "${VELORAY_INSTALL_LOCKED:-}" != true ]]||return 0
 exec 9>/run/lock/veloray-install.lock
 flock -n 9||{ vr_say 'Another VeloRay installation is running.' 'نصب دیگری از VeloRay در حال اجراست.' >&2;exit 1; }
 export VELORAY_INSTALL_LOCKED=true
}
vr_packages() {
 [[ "${VELORAY_DEPENDENCIES_READY:-}" != true ]]||return 0
 vr_run env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get update
 vr_run env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold "$@"
}
vr_xray_prepare() {
 # The service can traverse its config and write its standard logs under systemd protection.
 id veloray-xray >/dev/null 2>&1||useradd --system --gid nogroup --home /var/log/xray --shell /usr/sbin/nologin veloray-xray
 chmod 0755 /usr/local/etc/xray
 install -d -o veloray-xray -g nogroup -m 0750 /var/log/xray
 find /var/log/xray -maxdepth 1 -type f -exec chown veloray-xray:nogroup {} + -exec chmod 0640 {} +
 if [[ -n "${VELORAY_XRAY_DIRECTORY:-}" && ( ! -f /usr/local/share/xray/geoip.dat || ! -f /usr/local/share/xray/geosite.dat ) ]];then
  (cd "$VELORAY_XRAY_DIRECTORY" && sha256sum -c SHA256SUMS) >>"$VELORAY_INSTALL_LOG" 2>&1
  install -d -m 0755 /usr/local/share/xray
  install -m 0644 "$VELORAY_XRAY_DIRECTORY/geoip.dat" "$VELORAY_XRAY_DIRECTORY/geosite.dat" /usr/local/share/xray/
 fi
}
vr_xray_check() {
 local config=/usr/local/etc/xray/config.json
 chown root:nogroup "$config";chmod 0640 "$config"
 # TUN creation requires capabilities supplied by the actual service unit.
 if jq -e 'any(.inbounds[]?; .protocol == "tun")' "$config" >/dev/null 2>>"$VELORAY_INSTALL_LOG";then
  vr_run env XRAY_LOCATION_ASSET=/usr/local/share/xray /usr/local/bin/xray run -test -config "$config"
 else
  vr_run runuser -u veloray-xray -- env XRAY_LOCATION_ASSET=/usr/local/share/xray /usr/local/bin/xray run -test -config "$config"
 fi
}
vr_ports_check() {
 local status=0 report
 report="$(/usr/local/bin/veloray-agent check-ports /usr/local/etc/xray/config.json 2>>"$VELORAY_INSTALL_LOG")"||status=$?
 if ! jq -e '.ports|type=="array"' <<<"$report" >/dev/null 2>&1;then
  vr_say 'Could not inspect ports. Check the operation log and iproute2 installation.' 'بررسی پورت‌ها ناموفق بود؛ گزارش عملیات و نصب iproute2 را بررسی کنید.' >&2
  return 1
 fi
 VR_PORTS_REPORT="$report"
 if ((status!=0));then
  vr_say 'Xray port conflict:' 'تداخل پورت Xray:' >&2
  jq -r '.ports[]|select(.state=="conflict")|"  \(.network) \(.listen):\(.port) [\(.tag)] — \(.owner)"' <<<"$report" >&2
  return "$status"
 fi
}
vr_resolve_ports() {
 local inbound port
 while ! vr_ports_check;do
  vr_say 'Keep the existing service running. Choose another VPN port; client links will update together.' 'سرویس فعلی حفظ می‌شود؛ پورت دیگری برای VPN انتخاب کنید تا لینک کاربران هم‌زمان به‌روزرسانی شود.' >&2
  vr_say 'Commands: sudo veloray inbounds / sudo veloray inbound-port ID PORT' 'دستورها: sudo veloray inbounds / sudo veloray inbound-port ID PORT' >&2
  [[ -t 0 ]]||return 1
  veloray host-inbounds | jq -r '.[]|"  ID=\(.id)  \(.name)  \(.listen):\(.port)  \(.protocol)"'
  read -rp "$(vr_text 'Inbound ID to move (Enter to leave): ' 'شناسه اتصال برای تغییر پورت (Enter برای خروج): ')" inbound||return 1
  [[ -n "$inbound" ]]||return 1
  [[ "$inbound" =~ ^[1-9][0-9]*$ ]]||{ vr_say 'Enter a numeric inbound ID.' 'شناسهٔ عددی اتصال را وارد کنید.';continue; }
  read -rp "$(vr_text 'New VPN port: ' 'پورت جدید VPN: ')" port||return 1
  [[ "$port" =~ ^[0-9]+$ ]]&&((port>0&&port<65536))||{ vr_say 'Port must be 1–65535.' 'پورت باید بین ۱ و ۶۵۵۳۵ باشد.';continue; }
  if ! veloray host-inbound-port "$inbound" "$port";then vr_say 'Port change failed; review the error before retrying.' 'تغییر پورت ناموفق بود؛ خطا را بررسی کنید.' >&2;return 1;fi
 done
}
vr_diagnostics() {
 for unit in veloray-web.service veloray-agent.service xray.service;do
  systemctl --no-pager --full status "$unit" >>"$VELORAY_INSTALL_LOG" 2>&1||true
  journalctl -u "$unit" -b --since "${VR_OPERATION_STARTED:-10 minutes ago}" -n 40 --no-pager >>"$VELORAY_INSTALL_LOG" 2>&1||true
 done
}
vr_wait() {
 local path="$1" token="${2:-}" attempt body
 local -a headers=()
 [[ -z "$token" ]]||headers=(-H "Authorization: Bearer $token")
 for attempt in {1..45};do
  if body="$(curl -fsS --max-time 3 "${headers[@]}" "$path" 2>>"$VELORAY_INSTALL_LOG")";then
   if jq -e '.status=="ok" and (if has("xray") then .xray.installed==true and .xray.running==true else true end)' <<<"$body" >/dev/null 2>&1;then return 0;fi
  fi
  sleep 1
 done
 vr_diagnostics
 return 1
}
