#!/usr/bin/env bash
set -Eeuo pipefail

vr_text() { if [[ "${VELORAY_INSTALL_LANG:-${VELORAY_LANGUAGE:-en}}" == fa ]];then printf '%s' "$2";else printf '%s' "$1";fi; }
vr_say() { vr_text "$1" "$2";printf '\n'; }
vr_color() { if [[ -t 1 && -z "${NO_COLOR:-}" ]];then printf '\033[%sm%s\033[0m' "$1" "$2";else printf '%s' "$2";fi; }
vr_step() { VR_STEP="$(vr_text "$1" "$2")";printf '\n  ';vr_color 36 '•';printf ' %s\n' "$VR_STEP"; }
vr_log_init() {
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
vr_run_visible() { if [[ "${VELORAY_VERBOSE:-0}" == 1 ]];then "$@" 2>&1 | tee -a "$VELORAY_INSTALL_LOG";else vr_run "$@";fi; }
vr_error() {
 local line="$1" status="${2:-1}"
 printf 'Failure: step=%s line=%s status=%s\n' "${VR_STEP:-operation}" "$line" "$status" >>"$VELORAY_INSTALL_LOG"
 vr_color 31 "$(vr_text "Failed: ${VR_STEP:-operation}." "مرحله ناموفق: ${VR_STEP:-عملیات}.")" >&2;printf '\n' >&2
 vr_say "Details: sudo tail -n 60 $VELORAY_INSTALL_LOG" "جزئیات: sudo tail -n 60 $VELORAY_INSTALL_LOG" >&2
 exit "$status"
}
fail() {
 printf 'VeloRay: %s\n' "$*" >&2
 [[ -z "${VELORAY_INSTALL_LOG:-}" ]]||vr_say "Details: $VELORAY_INSTALL_LOG" "جزئیات: $VELORAY_INSTALL_LOG" >&2
 exit 1
}
usage() {
 cat <<'HELP'
VeloRay installer
  install.sh [install|node] [--mode docker|native] [--database postgresql] [--lang en|fa]
             [--repo OWNER/REPOSITORY] [--ref BRANCH_OR_TAG_OR_COMMIT]
Docker is the default for panel installations. Existing native data is migrated with a recovery backup.
PostgreSQL is the supported database. Default repository: ParsaZali/VeloRay, ref: main.
HELP
}
version_at_least() { [[ "$(printf '%s\n%s\n' "$2" "$1" | sort -V | head -n1)" == "$2" ]]; }
fetch() {
 local status=0 timeout="${3:-900}" log="${VELORAY_INSTALL_LOG:-/dev/null}"
 printf '  Download: %s\n' "$1" | tee -a "$log"
 curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --location --silent --show-error --no-progress-meter \
  --retry 2 --retry-max-time 120 --connect-timeout 15 --max-time "$timeout" \
  --speed-time 30 --speed-limit 1024 --write-out '  Received %{size_download} bytes in %{time_total}s (%{speed_download} bytes/s)\n' \
  "$1" -o "$2" 2> >(tee -a "$log" >&2) || status=$?
 if [[ "$status" -ne 0 ]];then
  printf 'Download failed (curl %s): %s\n' "$status" "$1" | tee -a "$log" >&2
  return "$status"
 fi
}
download_source() {
 local url
 # All routes resolve the same immutable commit, including fallback downloads.
 for url in "https://codeload.github.com/$repository/tar.gz/$commit" \
            "https://api.github.com/repos/$repository/tarball/$commit" \
            "https://github.com/$repository/archive/$commit.tar.gz";do
  if fetch "$url" "$temporary/source.tar.gz" 3600;then return 0;fi
  rm -f -- "$temporary/source.tar.gz"
  vr_say 'Trying an alternate GitHub download route…' 'تلاش با مسیر جایگزین دانلود گیت‌هاب…'
 done
 fail "Cannot download source commit $commit from $repository. Check GitHub access from this server and retry."
}
verify() {
 [[ "$1" =~ ^[a-fA-F0-9]{64}$ ]] || fail 'Missing or invalid SHA-256 checksum.'
 printf '%s  %s\n' "$1" "$2" | sha256sum -c - >/dev/null || fail 'Download checksum mismatch.'
}
check_archive() {
 tar -tf "$1" >"$temporary/members" || fail 'Cannot read archive members.'
 if awk '/(^\/|(^|\/)\.\.($|\/))/ {bad=1} END {exit !bad}' "$temporary/members";then fail 'Unsafe archive paths.';fi
 tar -tvf "$1" >"$temporary/types" || fail 'Cannot read archive file types.'
 if awk 'substr($0,1,1)!="-" && substr($0,1,1)!="d" {bad=1} END {exit !bad}' "$temporary/types";then fail 'Archive contains links or special files.';fi
}

action=install
database=postgresql
language=en
mode="${VELORAY_INSTALL_MODE:-docker}"
repository="${VELORAY_REPO:-ParsaZali/VeloRay}"
reference="${VELORAY_REF:-main}"
if [[ "${1:-}" == install || "${1:-}" == node ]];then action="$1";shift;fi
while [[ $# -gt 0 ]];do
 case "$1" in
  --help|-h) usage;exit 0;;
  --database|--lang|--repo|--ref|--mode)
   [[ $# -ge 2 && -n "$2" && "$2" != --* ]]||fail "Missing value for $1."
   case "$1" in --database) database="$2";;--lang) language="$2";;--repo) repository="$2";;--ref) reference="$2";;--mode) mode="$2";;esac
   shift 2;;
  *) fail "Unknown argument: $1. Run install.sh --help.";;
 esac
done
[[ "$database" == postgresql ]]||fail 'Only PostgreSQL is supported; SQLite, MySQL, MariaDB and TimescaleDB options are unavailable.'
[[ "$language" == en || "$language" == fa ]]||fail 'Language must be en or fa.'
[[ "$mode" == docker || "$mode" == native ]]||fail 'Mode must be docker or native.'
[[ "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ && "$repository" != *..* ]]||fail 'Invalid repository name.'
[[ "$reference" =~ ^[A-Za-z0-9_.-]+$ && "$reference" != *..* ]]||fail 'Invalid ref; use a simple branch, tag or full commit SHA.'
[[ ${EUID:-$(id -u)} -eq 0 ]]||fail 'Run the installer with sudo.'
[[ -r /etc/os-release ]]||fail 'Cannot identify the operating system.'
source /etc/os-release
[[ "${ID:-}" == ubuntu && ( "${VERSION_ID:-}" == 24.04 || ( "$mode" == docker && "$action" == install && "${VERSION_ID:-}" == 22.04 ) ) ]]||fail 'Docker supports Ubuntu 22.04/24.04; native and node installation require Ubuntu 24.04.'
case "$(uname -m)" in x86_64) arch=amd64;node_arch=x64;xray_asset=Xray-linux-64.zip;;aarch64|arm64) arch=arm64;node_arch=arm64;xray_asset=Xray-linux-arm64-v8a.zip;;*) fail 'Unsupported CPU architecture.';;esac

temporary="$(mktemp -d)"
trap 'rm -rf -- "$temporary"' EXIT
export VELORAY_INSTALL_LANG="$language"
vr_log_init install
exec 9>/run/lock/veloray-install.lock
flock -n 9||fail 'Another VeloRay installation is running. Wait for it to finish.'
export VELORAY_INSTALL_LOCKED=true
trap 'vr_error "$LINENO" "$?"' ERR
printf '\nVeloRay 1.0\n'
vr_say 'Server installation' 'نصب روی سرور'
vr_say "Log: $VELORAY_INSTALL_LOG" "گزارش: $VELORAY_INSTALL_LOG"
vr_step 'Prepare system packages' 'آماده‌سازی بسته‌های سیستم'
packages=(ca-certificates curl jq tar xz-utils unzip openssl iproute2 python3)
[[ "$action" != install || "$mode" == docker ]]||packages+=(postgresql postgresql-client nginx certbot python3)
apt_options=(-o Acquire::Retries=2 -o Acquire::http::Timeout=30 -o Acquire::https::Timeout=30)
vr_run_visible env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get "${apt_options[@]}" update
vr_run_visible env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get "${apt_options[@]}" install -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold "${packages[@]}"
export VELORAY_DEPENDENCIES_READY=true
vr_step 'Download and verify VeloRay' 'دریافت و بررسی فایل‌های VeloRay'

source_directory=""
if [[ -n "${BASH_SOURCE[0]:-}" && -f "${BASH_SOURCE[0]}" ]];then
 source_directory="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
fi
if [[ -z "$source_directory" || ! -f "$source_directory/go.mod" || ! -f "$source_directory/scripts/install-panel.sh" ]];then
 vr_step 'Resolve GitHub revision' 'دریافت نسخهٔ گیت‌هاب'
 fetch "https://api.github.com/repos/$repository/commits/$reference" "$temporary/commit.json" 45
 commit="$(jq -er '.sha' "$temporary/commit.json")"
 export VELORAY_INSTALL_COMMIT="$commit"
 [[ "$commit" =~ ^[a-f0-9]{40}$ ]]||fail 'GitHub did not return a valid source commit.'
 printf 'Repository=%s commit=%s\n' "$repository" "$commit" >>"$VELORAY_INSTALL_LOG"
 vr_step 'Download source and runtimes' 'دریافت سورس و بسته‌های آماده'
 download_source
 vr_step 'Check and extract project archive' 'بررسی و استخراج آرشیو پروژه'
 # GitHub archives contain regular files/directories; reject links and traversal before extracting.
 check_archive "$temporary/source.tar.gz"
 source_directory="$temporary/source"
 mkdir -p "$source_directory"
 tar --no-same-owner --no-same-permissions -xzf "$temporary/source.tar.gz" -C "$source_directory" --strip-components=1
fi
[[ -f "$source_directory/go.mod" && -f "$source_directory/frontend/package-lock.json" && -f "$source_directory/scripts/install-panel.sh" ]]||fail 'The selected repository does not contain a complete VeloRay project.'

if [[ "$action" == install && "$mode" == docker ]];then
 export VELORAY_INSTALL_REPO="$repository" VELORAY_INSTALL_REF="$reference"
 [[ -f "$source_directory/deploy/docker/manage.py" ]]||fail 'This revision does not include the Docker installer.'
 vr_step 'Verify container runtime bundles' 'بررسی بسته‌های اجرای کانتینری'
 vr_run bash "$source_directory/scripts/verify-runtime.sh" "$arch"
 vr_step 'Check existing installation' 'بررسی نصب قبلی'
 python3 "$source_directory/deploy/docker/manage.py" preflight
 vr_step 'Install Docker Engine and Compose' 'نصب Docker و Compose'
 vr_run_visible bash "$source_directory/deploy/docker/install-engine.sh"
 vr_step 'Install or migrate VeloRay' 'نصب یا انتقال VeloRay'
 if { true </dev/tty; } 2>/dev/null;then
  python3 "$source_directory/deploy/docker/manage.py" install --source "$source_directory" </dev/tty
 else
  python3 "$source_directory/deploy/docker/manage.py" install --source "$source_directory"
 fi
 exit 0
fi

prepare_runtime() {
 local archive="$source_directory/runtime/linux-$arch.tar.xz" checksum bundle="$temporary/runtime"
 [[ -f "$archive" ]]||return 1
 vr_step "Verify $arch runtime" "بررسی بستهٔ آمادهٔ $arch"
 [[ -f "$source_directory/runtime/SHA256SUMS" && -f "$source_directory/scripts/build-runtime.sh" ]]||fail 'Runtime checksum or build manifest helper is missing.'
 checksum="$(awk -v name="linux-$arch.tar.xz" '$2==name {print $1}' "$source_directory/runtime/SHA256SUMS")"
 verify "$checksum" "$archive"
 check_archive "$archive"
 mkdir -p "$bundle"
 tar --no-same-owner --no-same-permissions -xf "$archive" -C "$bundle" || fail 'Cannot extract runtime archive.'
 [[ -f "$bundle/source.SHA256SUMS" && -x "$bundle/bin/linux-$arch/veloray" && -x "$bundle/bin/linux-$arch/veloray-agent" && -f "$bundle/frontend/dist/index.html" ]]||fail 'Runtime archive is incomplete.'
 if [[ -z "${VELORAY_XRAY_ARCHIVE:-}" && -x "$bundle/xray/xray" && -f "$bundle/xray/SHA256SUMS" ]];then
  (cd "$bundle/xray" && sha256sum -c SHA256SUMS) >>"$VELORAY_INSTALL_LOG" 2>&1 || fail 'Bundled Xray checksum mismatch.'
  export VELORAY_XRAY_DIRECTORY="$bundle/xray"
 fi
 if [[ -x "$bundle/singbox/sing-box" && -f "$bundle/singbox/SHA256SUMS" ]];then (cd "$bundle/singbox" && sha256sum -c SHA256SUMS) >>"$VELORAY_INSTALL_LOG" 2>&1||fail 'Bundled sing-box checksum mismatch.';export VELORAY_SINGBOX_DIRECTORY="$bundle/singbox";fi
 bash "$source_directory/scripts/build-runtime.sh" --manifest >"$temporary/current-source.SHA256SUMS" || fail 'Cannot verify runtime source files.'
 if ! cmp -s "$temporary/current-source.SHA256SUMS" "$bundle/source.SHA256SUMS";then
  printf 'Runtime source manifest mismatch:\n' >>"$VELORAY_INSTALL_LOG"
  diff -u "$bundle/source.SHA256SUMS" "$temporary/current-source.SHA256SUMS" >>"$VELORAY_INSTALL_LOG" || true
  printf 'Source files changed; building the current source instead of using the bundled runtime.\n'
  vr_say "Changed file details: sudo tail -n 80 $VELORAY_INSTALL_LOG" "فایل‌های ناهماهنگ: sudo tail -n 80 $VELORAY_INSTALL_LOG"
  return 1
 fi
 (cd "$bundle/bin/linux-$arch" && sha256sum -c SHA256SUMS) >>"$VELORAY_INSTALL_LOG" 2>&1 || fail 'Runtime binary checksum mismatch.'
 mkdir -p "$source_directory/bin" "$source_directory/frontend"
 rm -rf -- "$source_directory/bin/linux-$arch" "$source_directory/frontend/dist"
 cp -a "$bundle/bin/linux-$arch" "$source_directory/bin/" || fail 'Cannot copy runtime binaries.'
 cp -a "$bundle/frontend/dist" "$source_directory/frontend/" || fail 'Cannot copy runtime frontend.'
 vr_say "Using verified $arch runtime." "بستهٔ آمادهٔ $arch تأیید شد."
}

prepare_build_tools() {
 local current minimum descriptor filename checksum target version cached downloaded url
 minimum="$(awk '$1=="go" {print $2;exit}' "$source_directory/go.mod")"
 [[ "$minimum" =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]]||fail 'Invalid Go requirement in go.mod.'
 current="$(go version 2>/dev/null | awk '{sub(/^go/,"",$3);print $3}')"||current=""
 if [[ -z "$current" ]]||! version_at_least "$current" "$minimum";then
  for cached in /opt/veloray/toolchains/go*/go/bin/go;do
   [[ -x "$cached" ]]||continue
   current="$("$cached" version 2>/dev/null | awk '{sub(/^go/,"",$3);print $3}')"||current=""
   if [[ "$current" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] && version_at_least "$current" "$minimum";then export PATH="$(dirname "$cached"):$PATH";break;fi
  done
 fi
 if [[ -z "$current" ]]||! version_at_least "$current" "$minimum";then
  fetch 'https://go.dev/dl/?mode=json' "$temporary/go.json"
  jq -ce --arg arch "$arch" '.[]|select(.stable==true)|.version as $version|.files[]|select(.os=="linux" and .arch==$arch and .kind=="archive")|. + {version:$version}' "$temporary/go.json" >"$temporary/go-candidates"
  downloaded=false
  while IFS= read -r descriptor;do
   version="$(jq -er '.version' <<<"$descriptor")";filename="$(jq -er '.filename' <<<"$descriptor")";checksum="$(jq -er '.sha256' <<<"$descriptor")"
   [[ "$version" =~ ^go[0-9]+\.[0-9]+\.[0-9]+$ && "$filename" == "$version.linux-$arch.tar.gz" ]]||fail 'Unexpected Go download metadata.'
   version_at_least "${version#go}" "$minimum"||continue
   # Alternate transport only; the archive is still verified against the
   # SHA-256 checksum supplied by the official go.dev release metadata.
   for url in "https://dl.google.com/go/$filename" "https://go.dev/dl/$filename";do
    if fetch "$url" "$temporary/go.tar.gz";then downloaded=true;break;fi
   done
   if $downloaded;then verify "$checksum" "$temporary/go.tar.gz";break;fi
   printf 'Go %s is unavailable; trying another supported stable release.\n' "${version#go}"
  done <"$temporary/go-candidates"
  $downloaded||fail "Cannot download a verified Go compiler >= $minimum from the official endpoints or verified mirror."
  target="/opt/veloray/toolchains/$version";mkdir -p "$target"
  mkdir -p "$temporary/go-unpack"
  tar --no-same-owner -xzf "$temporary/go.tar.gz" -C "$temporary/go-unpack"
  [[ "$("$temporary/go-unpack/go/bin/go" version | awk '{print $3}')" == "$version" ]]||fail 'Downloaded Go compiler has an unexpected version.'
  rm -rf -- "$target/go"
  mv "$temporary/go-unpack/go" "$target/go"
  export PATH="$target/go/bin:$PATH"
 fi
 current="$(node --version 2>/dev/null)"||current=""
 if [[ -z "$current" ]]||! version_at_least "${current#v}" 22.12.0||! command -v npm >/dev/null;then
  for cached in /opt/veloray/toolchains/node-v*-linux-$node_arch/bin;do
   [[ -x "$cached/node" && -x "$cached/npm" ]]||continue
   current="$("$cached/node" --version 2>/dev/null)"||current=""
   if [[ "$current" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] && version_at_least "${current#v}" 22.12.0;then export PATH="$cached:$PATH";break;fi
  done
 fi
 if [[ -z "$current" ]]||! version_at_least "${current#v}" 22.12.0||! command -v npm >/dev/null;then
  fetch 'https://nodejs.org/dist/index.json' "$temporary/node.json"
  jq -er --arg arch "linux-$node_arch" '[.[]|select(.lts!=false and (.files|index($arch))!=null)|select((.version|ltrimstr("v")|split(".")[0]|tonumber)>=22)][0:8][].version' "$temporary/node.json" >"$temporary/node-candidates"
  downloaded=false
  while IFS= read -r version;do
   [[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]||fail 'Unexpected Node.js download metadata.'
   version_at_least "${version#v}" 22.12.0||continue
   filename="node-$version-linux-$node_arch.tar.xz"
   for url in "https://nodejs.org/dist/$version" "https://nodejs.org/download/release/$version";do
    if fetch "$url/SHASUMS256.txt" "$temporary/node.sums" && fetch "$url/$filename" "$temporary/node.tar.xz";then
     checksum="$(awk -v name="$filename" '$2==name {print $1}' "$temporary/node.sums")"
     verify "$checksum" "$temporary/node.tar.xz";downloaded=true;break
    fi
   done
   $downloaded&&break
  done <"$temporary/node-candidates"
  $downloaded||fail 'Cannot download verified Node.js >=22.12. Use a package with bundled runtimes or check nodejs.org access.'
  target=/opt/veloray/toolchains;mkdir -p "$target"
  mkdir -p "$temporary/node-unpack"
  tar --no-same-owner -xJf "$temporary/node.tar.xz" -C "$temporary/node-unpack"
  [[ "$("$temporary/node-unpack/node-$version-linux-$node_arch/bin/node" --version)" == "$version" ]]||fail 'Downloaded Node.js has an unexpected version.'
  rm -rf -- "$target/node-$version-linux-$node_arch"
  mv "$temporary/node-unpack/node-$version-linux-$node_arch" "$target/"
  export PATH="$target/node-$version-linux-$node_arch/bin:$PATH"
 fi
}

vr_step 'Prepare application runtime' 'آماده‌سازی برنامه'
if [[ -f "$source_directory/runtime/linux-$arch.tar.xz" ]];then
 if ! prepare_runtime;then prepare_build_tools;vr_run bash "$source_directory/scripts/build.sh" "$arch";fi
elif [[ ! -x "$source_directory/bin/linux-$arch/veloray-agent" || ! -x "$source_directory/bin/linux-$arch/veloray" || ! -f "$source_directory/frontend/dist/index.html" ]];then
 prepare_build_tools
 vr_run bash "$source_directory/scripts/build.sh" "$arch"
fi

if [[ ! -x /usr/local/bin/xray && -z "${VELORAY_XRAY_ARCHIVE:-}" && -z "${VELORAY_XRAY_DIRECTORY:-}" ]];then
 xray_version="${VELORAY_XRAY_VERSION:-latest}"
 if [[ "$xray_version" == latest ]];then release_url=https://api.github.com/repos/XTLS/Xray-core/releases/latest
 else [[ "$xray_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]||fail 'Invalid Xray version.';release_url="https://api.github.com/repos/XTLS/Xray-core/releases/tags/$xray_version";fi
 fetch "$release_url" "$temporary/xray.json"
 xray_version="$(jq -er '.tag_name' "$temporary/xray.json")"
 [[ "$xray_version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]||fail 'Unexpected Xray release metadata.'
 digest="$(jq -er --arg name "$xray_asset" '.assets[]|select(.name==$name)|.digest' "$temporary/xray.json")"
 [[ "$digest" == sha256:* ]]||fail 'The Xray release is missing a SHA-256 asset digest.'
 export VELORAY_XRAY_ARCHIVE="$temporary/$xray_asset" VELORAY_XRAY_SHA256="${digest#sha256:}"
 fetch "https://github.com/XTLS/Xray-core/releases/download/$xray_version/$xray_asset" "$VELORAY_XRAY_ARCHIVE"
 verify "$VELORAY_XRAY_SHA256" "$VELORAY_XRAY_ARCHIVE"
fi
if [[ ! -x /usr/local/bin/sing-box && -z "${VELORAY_SINGBOX_DIRECTORY:-}" ]];then
 prepare_build_tools
 vr_run bash "$source_directory/scripts/build-singbox.sh" "$arch" "$temporary/singbox"
 export VELORAY_SINGBOX_DIRECTORY="$temporary/singbox"
fi
export VELORAY_INSTALL_REPO="$repository" VELORAY_INSTALL_REF="$reference"
installer="$source_directory/scripts/install-panel.sh"
[[ "$action" != node ]]||installer="$source_directory/deploy/node/install.sh"
# Piped invocations reserve stdin for the script; prompts must use the terminal.
if { true </dev/tty; } 2>/dev/null;then
 if bash "$installer" </dev/tty;then exit 0;else exit $?;fi
else
 [[ -n "${VELORAY_PUBLIC_HOST:-}" ]]||fail 'No terminal: set VELORAY_PUBLIC_HOST for unattended installation.'
 if [[ "$action" == install && ! -f /etc/veloray/veloray.env && -z "${VELORAY_ADMIN_PASSWORD:-}" ]];then fail 'No terminal: set VELORAY_ADMIN_PASSWORD.';fi
 if bash "$installer";then exit 0;else exit $?;fi
fi
