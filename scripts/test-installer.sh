#!/usr/bin/env bash
# Offline bootstrap tests: all host commands and downloads use temporary fixtures.
set -Eeuo pipefail
project="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d)";export TEST_ROOT
trap 'rm -rf -- "$TEST_ROOT"' EXIT
trap 'printf "Installer test failed at line %s.\n" "$LINENO" >&2; for output in "$TEST_ROOT/result" "$TEST_ROOT/host-result";do [[ ! -f "$output" ]]||tail -n 20 "$output" >&2;done' ERR
mkdir -p "$TEST_ROOT/commands" "$TEST_ROOT/fixture/scripts" "$TEST_ROOT/fixture/frontend" "$TEST_ROOT/fixture/deploy/node"
printf 'ID=ubuntu\nVERSION_ID=24.04\n' >"$TEST_ROOT/os-release"
sed -e 's|source /etc/os-release|source "$TEST_ROOT/os-release"|' \
 -e '/^\[\[ ${EUID:/d' \
 -e 's|/opt/veloray/toolchains|$TEST_ROOT/toolchains|g' \
 -e 's|/usr/local/bin/xray|$TEST_ROOT/installed-xray|g' \
 -e 's|/usr/local/bin/sing-box|$TEST_ROOT/installed-singbox|g' \
 -e 's|/var/log/veloray|$TEST_ROOT/logs|g' \
 -e 's|/run/lock/veloray-install.lock|$TEST_ROOT/install.lock|g' \
 "$project/install.sh" >"$TEST_ROOT/bootstrap.sh"
printf 'module test\ngo 1.26.0\n' >"$TEST_ROOT/fixture/go.mod"
printf '{}\n' >"$TEST_ROOT/fixture/frontend/package-lock.json"
manifest_root="$TEST_ROOT/manifest-fixture"
mkdir -p "$manifest_root/scripts" "$manifest_root/cmd" "$manifest_root/internal/panel" "$manifest_root/frontend/src"
for script in build-runtime.sh build.sh build-singbox.sh build-xray.sh verify-runtime-toolchain.sh;do cp "$project/scripts/$script" "$manifest_root/scripts/";done
printf 'module manifest-test\ngo 1.26.0\n' >"$manifest_root/go.mod"
touch "$manifest_root/go.sum"
printf '0.1.0\n' >"$manifest_root/VERSION"
printf 'print("embedded source")\n' >"$manifest_root/internal/panel/import_reader.py"
bash "$manifest_root/scripts/build-runtime.sh" --manifest >"$TEST_ROOT/manifest-before"
mkdir -p "$manifest_root/internal/panel/__pycache__"
printf 'local cache' >"$manifest_root/internal/panel/__pycache__/import_reader.cpython-312.pyc"
bash "$manifest_root/scripts/build-runtime.sh" --manifest >"$TEST_ROOT/manifest-with-cache"
cmp "$TEST_ROOT/manifest-before" "$TEST_ROOT/manifest-with-cache"
printf '# embedded source changed\n' >>"$manifest_root/internal/panel/import_reader.py"
bash "$manifest_root/scripts/build-runtime.sh" --manifest >"$TEST_ROOT/manifest-changed"
if cmp -s "$TEST_ROOT/manifest-before" "$TEST_ROOT/manifest-changed";then echo 'Embedded Python source was not verified.' >&2;exit 1;fi
cat >"$TEST_ROOT/fixture/scripts/build.sh" <<'BUILD'
#!/usr/bin/env bash
set -e
cd "$(dirname "$0")/.."
mkdir -p "bin/linux-$1" frontend/dist
printf '#!/usr/bin/env bash\nexit 0\n' >"bin/linux-$1/veloray"
cp "bin/linux-$1/veloray" "bin/linux-$1/veloray-agent"
chmod +x "bin/linux-$1/"*
touch frontend/dist/index.html
(cd "bin/linux-$1" && sha256sum veloray veloray-agent >SHA256SUMS)
echo "build=$1" >>"$TEST_ROOT/calls"
BUILD
cat >"$TEST_ROOT/fixture/scripts/install-panel.sh" <<'PANEL'
#!/usr/bin/env bash
if [[ -n "${VELORAY_XRAY_DIRECTORY:-}" ]];then
 (cd "$VELORAY_XRAY_DIRECTORY" && sha256sum -c SHA256SUMS >/dev/null)
else
 test -f "$VELORAY_XRAY_ARCHIVE"
 printf '%s  %s\n' "$VELORAY_XRAY_SHA256" "$VELORAY_XRAY_ARCHIVE" | sha256sum -c - >/dev/null
fi
echo "panel=$VELORAY_INSTALL_LANG" >>"$TEST_ROOT/calls"
PANEL
sed 's/panel=/node=/' "$TEST_ROOT/fixture/scripts/install-panel.sh" >"$TEST_ROOT/fixture/deploy/node/install.sh"
cat >"$TEST_ROOT/fixture/scripts/build-singbox.sh" <<'SECONDARY'
#!/usr/bin/env bash
mkdir -p "$2"
printf '#!/usr/bin/env bash\nexit 0\n' >"$2/sing-box"
chmod +x "$2/sing-box"
(cd "$2" && sha256sum sing-box >SHA256SUMS)
SECONDARY
tar -czf "$TEST_ROOT/source.tar.gz" -C "$TEST_ROOT" fixture
printf 'fixture-Xray-archive' >"$TEST_ROOT/xray.zip"
export TEST_XRAY_SHA="$(sha256sum "$TEST_ROOT/xray.zip" | cut -d' ' -f1)"
mkdir -p "$TEST_ROOT/go/bin" "$TEST_ROOT/node-v24.21.0-linux-arm64/bin"
printf '#!/usr/bin/env bash\necho "go version go1.27.2 linux/arm64"\n' >"$TEST_ROOT/go/bin/go"
printf '#!/usr/bin/env bash\necho v24.21.0\n' >"$TEST_ROOT/node-v24.21.0-linux-arm64/bin/node"
printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/node-v24.21.0-linux-arm64/bin/npm"
chmod +x "$TEST_ROOT/go/bin/go" "$TEST_ROOT/node-v24.21.0-linux-arm64/bin/"*
tar -czf "$TEST_ROOT/go.tar.gz" -C "$TEST_ROOT" go
sed -i 's/go1.27.2/go1.26.9/' "$TEST_ROOT/go/bin/go"
tar -czf "$TEST_ROOT/go-fallback.tar.gz" -C "$TEST_ROOT" go
tar -cJf "$TEST_ROOT/node.tar.xz" -C "$TEST_ROOT" node-v24.21.0-linux-arm64
export TEST_GO_SHA="$(sha256sum "$TEST_ROOT/go.tar.gz" | cut -d' ' -f1)" TEST_NODE_SHA="$(sha256sum "$TEST_ROOT/node.tar.xz" | cut -d' ' -f1)"
export TEST_GO_FALLBACK_SHA="$(sha256sum "$TEST_ROOT/go-fallback.tar.gz" | cut -d' ' -f1)"
cat >"$TEST_ROOT/commands/curl" <<'CURL'
#!/usr/bin/env bash
set -e
url='';destination=''
printf '%s\n' "$*" >>"$TEST_ROOT/curl-options"
while [[ $# -gt 0 ]];do case "$1" in -o) destination="$2";shift 2;;https://*) url="$1";shift;;*) shift;;esac;done
echo "$url" >>"$TEST_ROOT/requests"
case "$url" in
 https://go.dev/dl/\?mode=json) printf '[{"stable":true,"version":"go1.27.2","files":[{"os":"linux","arch":"arm64","kind":"archive","filename":"go1.27.2.linux-arm64.tar.gz","sha256":"%s"}]},{"stable":true,"version":"go1.26.9","files":[{"os":"linux","arch":"arm64","kind":"archive","filename":"go1.26.9.linux-arm64.tar.gz","sha256":"%s"}]}]' "$TEST_GO_SHA" "$TEST_GO_FALLBACK_SHA" >"$destination";;
 https://*/go1.27.2.linux-arm64.tar.gz)
  [[ "${TEST_GO_ROUTE:-}" != unavailable && "${TEST_GO_ROUTE:-}" != previous ]]||exit 22
  [[ "${TEST_GO_ROUTE:-}" != mirror || "$url" != https://dl.google.com/* ]]||exit 22
  if [[ "${TEST_BAD_GO_DIGEST:-}" == 1 ]];then printf broken >"$destination";else cp "$TEST_ROOT/go.tar.gz" "$destination";fi;;
 https://*/go1.26.9.linux-arm64.tar.gz)
  [[ "${TEST_GO_ROUTE:-}" != unavailable ]]||exit 22
  cp "$TEST_ROOT/go-fallback.tar.gz" "$destination";;
 https://nodejs.org/dist/index.json) printf '[{"version":"v24.21.0","lts":"Krypton","files":["linux-arm64"]}]' >"$destination";;
 */SHASUMS256.txt)
  [[ "${TEST_NODE_ROUTE:-}" != mirror || "$url" != https://nodejs.org/dist/* ]]||exit 22
  printf '%s  node-v24.21.0-linux-arm64.tar.xz\n' "$TEST_NODE_SHA" >"$destination";;
 */node-v24.21.0-linux-arm64.tar.xz)
  if [[ "${TEST_BAD_NODE_DIGEST:-}" == 1 ]];then printf broken >"$destination";else cp "$TEST_ROOT/node.tar.xz" "$destination";fi;;
 */commits/*) printf '{"sha":"1111111111111111111111111111111111111111"}' >"$destination";;
 https://codeload.github.com/ParsaZali/VeloRay/tar.gz/*)
  if [[ "${TEST_SOURCE_ROUTE:-}" == stalled ]];then echo 'curl: (28) Operation too slow' >&2;exit 28;fi
  [[ "${TEST_SOURCE_ROUTE:-direct}" == direct ]]||exit 22
  cp "$TEST_ROOT/source.tar.gz" "$destination";;
 https://api.github.com/repos/ParsaZali/VeloRay/tarball/*)
  [[ "${TEST_SOURCE_ROUTE:-direct}" != web && "${TEST_SOURCE_ROUTE:-direct}" != unavailable ]]||exit 22
  cp "$TEST_ROOT/source.tar.gz" "$destination";;
 https://github.com/ParsaZali/VeloRay/archive/*.tar.gz)
  [[ "${TEST_SOURCE_ROUTE:-direct}" != unavailable ]]||exit 22
  cp "$TEST_ROOT/source.tar.gz" "$destination";;
 */XTLS/Xray-core/releases/latest) printf '{"tag_name":"v26.3.27","assets":[{"name":"Xray-linux-64.zip","digest":"sha256:%s"},{"name":"Xray-linux-arm64-v8a.zip","digest":"sha256:%s"}]}' "$TEST_XRAY_SHA" "$TEST_XRAY_SHA" >"$destination";;
 */releases/download/*) if [[ "${TEST_BAD_DIGEST:-}" == 1 ]];then printf broken >"$destination";else cp "$TEST_ROOT/xray.zip" "$destination";fi;;
 *) echo "Unexpected network request: $url" >&2;exit 2;;
esac
CURL
cat >"$TEST_ROOT/commands/apt-get" <<'APT'
#!/usr/bin/env bash
echo "apt=$*" >>"$TEST_ROOT/host-commands"
echo 'Checking system package repositories'
if [[ "${TEST_APT_FAILURE:-}" == 1 ]];then echo 'Package repository unavailable' >&2;exit 100;fi
APT
cat >"$TEST_ROOT/commands/uname" <<'UNAME'
#!/usr/bin/env bash
echo "${TEST_MACHINE:-x86_64}"
UNAME
cat >"$TEST_ROOT/commands/go" <<'GO'
#!/usr/bin/env bash
if [[ "${TEST_OLD_TOOLS:-}" == 1 ]];then echo 'go version go1.25.0 linux/amd64';else echo 'go version go1.27.2 linux/amd64';fi
GO
cat >"$TEST_ROOT/commands/node" <<'NODE'
#!/usr/bin/env bash
if [[ "${TEST_OLD_TOOLS:-}" == 1 ]];then echo v20.0.0;else echo v24.21.0;fi
NODE
printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/commands/npm"
chmod +x "$TEST_ROOT/commands/"*
export PATH="$TEST_ROOT/commands:$PATH" VELORAY_PUBLIC_HOST=panel.example.com VELORAY_ADMIN_PASSWORD=test-only-password
unset VELORAY_XRAY_ARCHIVE VELORAY_XRAY_DIRECTORY VELORAY_XRAY_SHA256 VELORAY_REPO VELORAY_REF TEST_SOURCE_ROUTE TEST_GO_ROUTE TEST_NODE_ROUTE || true
passed=0
pass() { passed=$((passed+1));printf 'PASS %s\n' "$1"; }
reject() {
 local phrase="$1";shift
 if bash "$project/install.sh" "$@" >"$TEST_ROOT/result" 2>&1;then echo 'Invalid arguments accepted' >&2;exit 1;fi
 grep -Fq "$phrase" "$TEST_ROOT/result";pass "$phrase"
}
bash "$project/install.sh" --help >"$TEST_ROOT/result"
grep -Fq -- '--database postgresql' "$TEST_ROOT/result";pass help
reject 'Only PostgreSQL' install --database mysql
reject 'Missing value' --database
reject 'Language must' --lang xx
reject 'Invalid repository' --repo ../bad
reject 'Unknown argument' --unavailable
: >"$TEST_ROOT/calls"
if flock "$TEST_ROOT/install.lock" bash "$TEST_ROOT/bootstrap.sh" install >"$TEST_ROOT/result" 2>&1;then echo 'Concurrent installation accepted' >&2;exit 1;fi
grep -Fq 'Another VeloRay installation is running' "$TEST_ROOT/result"
test ! -s "$TEST_ROOT/calls";pass concurrent-installation-blocked
export TEST_APT_FAILURE=1
: >"$TEST_ROOT/requests"
if bash "$TEST_ROOT/bootstrap.sh" install >"$TEST_ROOT/result" 2>&1;then echo 'Failed packages accepted' >&2;exit 1;fi
grep -Fq 'Package repository unavailable' "$TEST_ROOT/result"
grep -Fq 'Failed: Prepare system packages' "$TEST_ROOT/result"
test ! -s "$TEST_ROOT/requests";test ! -s "$TEST_ROOT/calls"
pass visible-package-failure-stops-installation
unset TEST_APT_FAILURE
for machine in x86_64 aarch64;do
 export TEST_MACHINE="$machine"
 for mode in install node;do
  : >"$TEST_ROOT/calls"
  # bash -c matches the documented curl command; @ supplies $0.
  bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ "$mode" --database postgresql --lang fa >"$TEST_ROOT/result" 2>&1
  expected=panel;[[ "$mode" != node ]]||expected=node
  grep -Fq "$expected=fa" "$TEST_ROOT/calls"
  grep -Fq '/tar.gz/1111111111111111111111111111111111111111' "$TEST_ROOT/requests"
  pass "$mode/$machine"
 done
done
for route in api web stalled;do
 export TEST_SOURCE_ROUTE="$route"
 : >"$TEST_ROOT/requests";: >"$TEST_ROOT/calls"
 bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1
 grep -Fq 'panel=en' "$TEST_ROOT/calls"
 if [[ "$route" == api || "$route" == stalled ]];then grep -Fq '/tarball/1111111111111111111111111111111111111111' "$TEST_ROOT/requests"
 else grep -Fq '/archive/1111111111111111111111111111111111111111.tar.gz' "$TEST_ROOT/requests";fi
 if grep -Fq 'installation failed at line' "$TEST_ROOT/result";then echo 'Recovered download reported as installation failure' >&2;exit 1;fi
 pass "source-$route-fallback"
done
grep -Fq 'curl: (28) Operation too slow' "$TEST_ROOT/result"
grep -Fq 'Download failed (curl 28)' "$TEST_ROOT/result"
grep -Fq 'Download: https://api.github.com/repos/ParsaZali/VeloRay/commits/main' "$TEST_ROOT/result"
grep -Fq 'Resolve GitHub revision' "$TEST_ROOT/result"
grep -Fq 'Check and extract project archive' "$TEST_ROOT/result"
grep -Fq 'Log:' "$TEST_ROOT/result"
grep -Fq -- '--max-time 45' "$TEST_ROOT/curl-options"
grep -Fq -- '--max-time 900' "$TEST_ROOT/curl-options"
grep -F 'codeload.github.com/' "$TEST_ROOT/curl-options" | grep -Fq -- '--max-time 3600'
if grep -vF -- '--speed-time 30 --speed-limit 1024' "$TEST_ROOT/curl-options" | grep -q .;then echo 'Download lacks a stall timeout' >&2;exit 1;fi
if grep -vF -- '--no-silent --no-progress-bar --progress-meter' "$TEST_ROOT/curl-options" | grep -q .;then echo 'Download lacks a byte and speed meter' >&2;exit 1;fi
pass visible-downloads-and-stall-timeouts
export TEST_SOURCE_ROUTE=unavailable
: >"$TEST_ROOT/calls"
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Unavailable source accepted' >&2;exit 1;fi
grep -Fq 'Cannot download source commit 1111111111111111111111111111111111111111' "$TEST_ROOT/result"
grep -RFq 'Download failed (curl 22): https://' "$TEST_ROOT/logs"
test ! -s "$TEST_ROOT/calls"
pass source-download-failure
unset TEST_SOURCE_ROUTE
cat "$TEST_ROOT/bootstrap.sh" | bash -s -- install --lang en >"$TEST_ROOT/result" 2>&1
grep -Fq 'panel=en' "$TEST_ROOT/calls";pass piped-install
export TEST_OLD_TOOLS=1
bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1
test -x "$TEST_ROOT/toolchains/go1.27.2/go/bin/go"
test -x "$TEST_ROOT/toolchains/node-v24.21.0-linux-arm64/bin/node"
pass build-tool-downloads
: >"$TEST_ROOT/requests"
bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1
if grep -Eq 'go.dev|dl.google.com|nodejs.org' "$TEST_ROOT/requests";then echo 'Cached tools were downloaded again' >&2;exit 1;fi
pass cached-build-tools
for route in mirror previous;do
 rm -rf -- "$TEST_ROOT/toolchains"
 export TEST_GO_ROUTE="$route"
 bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1
 version=go1.27.2;[[ "$route" != previous ]]||version=go1.26.9
 test -x "$TEST_ROOT/toolchains/$version/go/bin/go"
 pass "go-$route-fallback"
done
rm -rf -- "$TEST_ROOT/toolchains"
export TEST_GO_ROUTE=unavailable
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Unavailable Go accepted' >&2;exit 1;fi
grep -Fq 'Cannot download a verified Go compiler' "$TEST_ROOT/result";pass go-download-failure
unset TEST_GO_ROUTE
export TEST_NODE_ROUTE=mirror
bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1
grep -Fq 'nodejs.org/download/release/' "$TEST_ROOT/requests";pass node-mirror-fallback
unset TEST_NODE_ROUTE
rm -rf -- "$TEST_ROOT/toolchains"
export TEST_BAD_NODE_DIGEST=1
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Bad Node checksum accepted' >&2;exit 1;fi
grep -Fq 'checksum mismatch' "$TEST_ROOT/result";pass node-checksum-rejection
unset TEST_BAD_NODE_DIGEST
rm -rf -- "$TEST_ROOT/toolchains"
export TEST_BAD_GO_DIGEST=1
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Bad Go checksum accepted' >&2;exit 1;fi
grep -Fq 'checksum mismatch' "$TEST_ROOT/result";pass build-tool-checksum-rejection
unset TEST_OLD_TOOLS TEST_BAD_GO_DIGEST
export TEST_BAD_DIGEST=1
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Bad checksum accepted' >&2;exit 1;fi
grep -Fq 'checksum mismatch' "$TEST_ROOT/result";pass checksum-rejection
unset TEST_BAD_DIGEST
cat >"$TEST_ROOT/fixture/scripts/build-runtime.sh" <<'MANIFEST'
#!/usr/bin/env bash
cd "$(dirname "$0")/.."
sha256sum go.mod
MANIFEST
mkdir -p "$TEST_ROOT/fixture/runtime"
for arch in amd64 arm64;do
 bash "$TEST_ROOT/fixture/scripts/build.sh" "$arch"
 bundle="$TEST_ROOT/bundle-$arch"
 mkdir -p "$bundle/bin" "$bundle/frontend" "$bundle/xray"
 bash "$TEST_ROOT/fixture/scripts/build-singbox.sh" "$arch" "$bundle/singbox"
 cp -a "$TEST_ROOT/fixture/bin/linux-$arch" "$bundle/bin/"
 cp -a "$TEST_ROOT/fixture/frontend/dist" "$bundle/frontend/"
 printf '#!/usr/bin/env bash\nexit 0\n' >"$bundle/xray/xray";chmod +x "$bundle/xray/xray"
 printf fixture >"$bundle/xray/geoip.dat";cp "$bundle/xray/geoip.dat" "$bundle/xray/geosite.dat"
 (cd "$bundle/xray" && sha256sum xray geoip.dat geosite.dat >SHA256SUMS)
 bash "$TEST_ROOT/fixture/scripts/build-runtime.sh" --manifest >"$bundle/source.SHA256SUMS"
 tar -cJf "$TEST_ROOT/fixture/runtime/linux-$arch.tar.xz" -C "$bundle" bin frontend xray singbox source.SHA256SUMS
done
(cd "$TEST_ROOT/fixture/runtime" && sha256sum linux-*.tar.xz >SHA256SUMS)
rm -rf -- "$TEST_ROOT/fixture/bin" "$TEST_ROOT/fixture/frontend/dist"
tar -czf "$TEST_ROOT/source.tar.gz" -C "$TEST_ROOT" fixture
export TEST_OLD_TOOLS=1 TEST_GO_ROUTE=unavailable
for machine in x86_64 aarch64;do
 export TEST_MACHINE="$machine"
 for mode in install node;do
  : >"$TEST_ROOT/calls";: >"$TEST_ROOT/requests"
  bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ "$mode" >"$TEST_ROOT/result" 2>&1
  grep -Fq 'Using verified' "$TEST_ROOT/result"
  if grep -Eq 'go.dev|dl.google.com|nodejs.org|XTLS' "$TEST_ROOT/requests" || grep -Fq build= "$TEST_ROOT/calls";then echo 'Bundled runtime unexpectedly required external tools' >&2;exit 1;fi
  pass "bundled-$mode/$machine"
 done
done
unset TEST_OLD_TOOLS TEST_GO_ROUTE
pass source-manifest-excludes-cache-and-tracks-embedded-python
printf '# source changed\n' >>"$TEST_ROOT/fixture/go.mod"
tar -czf "$TEST_ROOT/source.tar.gz" -C "$TEST_ROOT" fixture
: >"$TEST_ROOT/calls"
bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1
grep -Fq 'Source files changed' "$TEST_ROOT/result"
grep -Fq 'build=arm64' "$TEST_ROOT/calls";pass stale-runtime-rebuilt
sed -i '$d' "$TEST_ROOT/fixture/go.mod"
cp "$TEST_ROOT/fixture/runtime/linux-arm64.tar.xz" "$TEST_ROOT/good-runtime.tar.xz"
printf corrupt >>"$TEST_ROOT/fixture/runtime/linux-arm64.tar.xz"
tar -czf "$TEST_ROOT/source.tar.gz" -C "$TEST_ROOT" fixture
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Corrupt runtime accepted' >&2;exit 1;fi
grep -Fq 'checksum mismatch' "$TEST_ROOT/result";pass runtime-checksum-rejection
cp "$TEST_ROOT/good-runtime.tar.xz" "$TEST_ROOT/fixture/runtime/linux-arm64.tar.xz"
ln -s /etc/passwd "$TEST_ROOT/fixture/unsafe-link"
tar -czf "$TEST_ROOT/source.tar.gz" -C "$TEST_ROOT" fixture
if bash -c "$(cat "$TEST_ROOT/bootstrap.sh")" @ install >"$TEST_ROOT/result" 2>&1;then echo 'Unsafe archive accepted' >&2;exit 1;fi
grep -Fq 'links or special files' "$TEST_ROOT/result";pass archive-link-rejection

# Execute the real host installer and controller against a private filesystem.
# Service management and PostgreSQL CLI commands are simulated; no host files change.
HOST_ROOT="$TEST_ROOT/host";export HOST_ROOT
host_project="$TEST_ROOT/host-project";mkdir -p "$host_project/scripts" "$host_project/bin/linux-amd64" "$host_project/frontend/dist" "$TEST_ROOT/host-tools"
cp -a "$project/deploy" "$host_project/"
cp -a "$project/examples" "$host_project/"
cp "$project/go.mod" "$host_project/";cp "$project/install.sh" "$host_project/"
cp "$project/frontend/"package{,-lock}.json "$host_project/frontend/"
touch "$host_project/frontend/dist/index.html"
for script in scripts/install-panel.sh scripts/velorayctl scripts/auto-update.sh scripts/xray-install.sh scripts/common.sh;do
 sed -e '/^\[\[ ${EUID:/d' -e "s|/etc/|$HOST_ROOT/etc/|g" -e "s|/opt/|$HOST_ROOT/opt/|g" -e "s|/usr/local/|$HOST_ROOT/usr/local/|g" -e "s|/var/lib/|$HOST_ROOT/var/lib/|g" -e "s|/var/log/|$HOST_ROOT/var/log/|g" -e "s|/run/lock/veloray-install.lock|$HOST_ROOT/install.lock|g" "$project/$script" >"$host_project/$script"
done
cat >"$host_project/bin/linux-amd64/veloray" <<'APP'
#!/usr/bin/env bash
set -e
echo "$1" >>"$HOST_ROOT/app-calls"
case "$1" in
 version) echo 1.0;;
 bootstrap) [[ "${TEST_BOOTSTRAP_FAIL:-}" != 1 ]]||exit 1;touch "$HOST_ROOT/admin";;
 backup) printf fixture-backup >"$2";;
 reset-password) [[ "${#VELORAY_ADMIN_PASSWORD}" -ge 12 ]]||exit 1;touch "$HOST_ROOT/password-reset";;
 host-inbounds) printf '[{"id":1,"name":"main","listen":"0.0.0.0","port":443,"protocol":"vless","enabled":true}]\n';;
 host-inbound-port) [[ "$2" == 1 && "$3" == 2053 ]]||exit 2;touch "$HOST_ROOT/ports-resolved";;
esac
APP
cat >"$host_project/bin/linux-amd64/veloray-agent" <<'AGENT'
#!/usr/bin/env bash
if [[ "${1:-}" == check-ports ]];then
 if [[ "${TEST_PORTS_CONFLICT:-}" == 1 && ! -f "$HOST_ROOT/ports-resolved" ]];then
  printf '%s\n' '{"ports":[{"tag":"in-1-main","listen":"0.0.0.0","port":443,"network":"tcp","state":"conflict","owner":"nginx pid=456"}]}'
  exit 1
 fi
 printf '%s\n' '{"ports":[]}'
fi
AGENT
chmod +x "$host_project/bin/linux-amd64/"*
(cd "$host_project/bin/linux-amd64" && sha256sum veloray veloray-agent >SHA256SUMS)
cat >"$TEST_ROOT/host-tools/psql" <<'PG'
#!/usr/bin/env bash
set -e
args="$*"
if [[ "$args" == *'pg_roles'* && "$args" == *'pg_database'* ]];then
 if [[ -f "$HOST_ROOT/role" || -f "$HOST_ROOT/database" ]];then echo t;else echo f;fi
elif [[ "$args" == *'pg_roles'* ]];then
 if [[ -f "$HOST_ROOT/role" ]];then echo t;else echo f;fi
elif [[ "$args" == *'pg_database'* ]];then
 if [[ -f "$HOST_ROOT/database" ]];then echo t;else echo f;fi
elif [[ "$args" == *'vr_users'* ]];then
 if [[ -f "$HOST_ROOT/admin" ]];then echo t;else echo f;fi
elif [[ "$args" == *'core_node'* ]];then echo f
else
 sql="$(cat)"
 [[ "$sql" == *'CREATE ROLE veloray'* ]]
 test -f "$HOST_ROOT/etc/veloray/veloray.env"
 touch "$HOST_ROOT/role"
fi
PG
cat >"$TEST_ROOT/host-tools/createdb" <<'DB'
#!/usr/bin/env bash
test -f "$HOST_ROOT/etc/veloray/veloray.env"
touch "$HOST_ROOT/database"
DB
cat >"$TEST_ROOT/host-tools/runuser" <<'RUNUSER'
#!/usr/bin/env bash
shift 3;exec "$@"
RUNUSER
cat >"$TEST_ROOT/host-tools/install" <<'INSTALL'
#!/usr/bin/env bash
args=()
while [[ $# -gt 0 ]];do case "$1" in -o|-g) shift 2;;*) args+=("$1");shift;;esac;done
exec /usr/bin/install "${args[@]}"
INSTALL
cat >"$TEST_ROOT/host-tools/curl" <<'HEALTH'
#!/usr/bin/env bash
[[ "${TEST_HEALTH_FAIL:-}" != 1 ]]||exit 22
case "$*" in
 */xray/stats*) echo '{"accounting":"durable-v1","ledger_id":"fixture","stat":[]}' ;;
 *:9191/health*) if [[ "${TEST_DEGRADED:-}" == 1 ]];then echo '{"status":"degraded","xray":{"installed":true,"running":false}}';else echo '{"status":"ok","xray":{"installed":true,"running":true}}';fi;;
 *) echo '{"status":"ok"}';;
esac
HEALTH
for command in chown id nginx journalctl sleep;do printf '#!/usr/bin/env bash\nexit 0\n' >"$TEST_ROOT/host-tools/$command";done
cat >"$TEST_ROOT/host-tools/systemctl" <<'SYSTEMD'
#!/usr/bin/env bash
echo "$*" >>"$HOST_ROOT/service-calls"
if [[ "$1" == is-enabled && "$*" == *veloray-subscriptions.service* && "${TEST_SUBSCRIPTIONS_ACTIVE:-}" != 1 ]];then exit 1;fi
if [[ "$1" == is-active ]];then
 [[ "$*" != *veloray-subscriptions.service* || "${TEST_SUBSCRIPTIONS_ACTIVE:-}" == 1 ]]||exit 3
 [[ "$*" != *"${TEST_FAILED_UNIT:-__none__}"* ]]||exit 3
 [[ "$*" == *--quiet* ]]||echo active
fi
SYSTEMD
cat >"$TEST_ROOT/host-tools/certbot" <<'CERTBOT'
#!/usr/bin/env bash
printf '%s\n' "$*" >"$HOST_ROOT/certbot-call"
CERTBOT
printf '#!/usr/bin/env bash\nprintf fixture-dump\n' >"$TEST_ROOT/host-tools/pg_dump"
chmod +x "$TEST_ROOT/host-tools/"*
bootstrap_test_path="$PATH"
export PATH="$HOST_ROOT/usr/local/bin:$TEST_ROOT/host-tools:$PATH" TEST_MACHINE=x86_64
reset_host() {
 rm -rf -- "$HOST_ROOT"
 mkdir -p "$HOST_ROOT/usr/local/bin" "$HOST_ROOT/opt/veloray" "$HOST_ROOT/etc/systemd/system"
 printf '#!/usr/bin/env bash\n[[ "${TEST_XRAY_CONFIG_FAIL:-}" != 1 ]]\n' >"$HOST_ROOT/usr/local/bin/xray";chmod +x "$HOST_ROOT/usr/local/bin/xray"
 printf '#!/usr/bin/env bash\nexit 0\n' >"$HOST_ROOT/usr/local/bin/sing-box";chmod +x "$HOST_ROOT/usr/local/bin/sing-box"
}
host_install() { bash "$host_project/scripts/install-panel.sh" >"$TEST_ROOT/host-result" 2>&1; }
reset_host
chmod 0600 "$host_project/frontend/dist/index.html"
host_install
[[ "$(stat -c '%a' "$HOST_ROOT/opt/veloray/current/frontend/dist/index.html")" == 644 ]]
pass web-assets-readable-after-private-umask
test -f "$HOST_ROOT/admin";test -f "$HOST_ROOT/database";test -f "$HOST_ROOT/role"
original_env="$(sha256sum "$HOST_ROOT/etc/veloray/veloray.env")"
pass host-fresh-install
# A same-version engine rebuild can contain a critical compiler security fix.
# Updating a complete runtime must replace an already installed Xray binary.
patched_xray="$TEST_ROOT/patched-xray";mkdir -p "$patched_xray"
printf '#!/usr/bin/env bash\n# patched-release\n[[ "${TEST_XRAY_CONFIG_FAIL:-}" != 1 ]]\n' >"$patched_xray/xray"
printf geoip >"$patched_xray/geoip.dat";printf geosite >"$patched_xray/geosite.dat"
chmod +x "$patched_xray/xray"
(cd "$patched_xray" && sha256sum xray geoip.dat geosite.dat >SHA256SUMS)
config_path="$(source "$HOST_ROOT/etc/veloray-node/agent.env";printf '%s' "$VELORAY_XRAY_CONFIG")"
prior_config="$(sha256sum "$config_path")"
mkdir -p "$HOST_ROOT/usr/local/share/xray"
printf operator-geoip >"$HOST_ROOT/usr/local/share/xray/geoip.dat"
prior_geo="$(sha256sum "$HOST_ROOT/usr/local/share/xray/geoip.dat")"
export VELORAY_XRAY_DIRECTORY="$patched_xray"
host_install
unset VELORAY_XRAY_DIRECTORY
grep -Fq '# patched-release' "$HOST_ROOT/usr/local/bin/xray"
[[ "$(sha256sum "$config_path")" == "$prior_config" ]]
[[ "$(sha256sum "$HOST_ROOT/usr/local/share/xray/geoip.dat")" == "$prior_geo" ]]
pass runtime-upgrade-replaces-existing-xray-and-keeps-config
for name in modernX.html sub-dashboard.html sub-fish.html;do
 cmp "$project/examples/subscription/legacy/$name" "$HOST_ROOT/etc/veloray/templates/legacy/$name"
 [[ "$(stat -c '%a' "$HOST_ROOT/etc/veloray/templates/legacy/$name")" == 640 ]]
done
pass legacy-templates-installed-privately
printf 'operator-owned template' >"$HOST_ROOT/etc/veloray/templates/legacy/modernX.html"
test -f "$HOST_ROOT/etc/systemd/system/veloray-subscriptions.service"
test -f "$HOST_ROOT/etc/systemd/system/veloray-subscription-endpoint.service"
test -f "$HOST_ROOT/etc/systemd/system/veloray-subscription-endpoint.timer"
grep -Fxq 'enable veloray-subscriptions.service' "$HOST_ROOT/service-calls"
grep -Fxq 'restart veloray-subscriptions.service' "$HOST_ROOT/service-calls"
grep -Fxq 'enable --now veloray-subscription-endpoint.timer' "$HOST_ROOT/service-calls"
pass independent-subscriptions-and-port-manager-enabled
: >"$HOST_ROOT/app-calls"
host_install
[[ "$(sha256sum "$HOST_ROOT/etc/veloray/veloray.env")" == "$original_env" ]]
grep -Fxq backup "$HOST_ROOT/app-calls"
if grep -Fxq bootstrap "$HOST_ROOT/app-calls";then echo 'Upgrade reset the administrator' >&2;exit 1;fi
pass host-repeat-keeps-secrets
[[ "$(cat "$HOST_ROOT/etc/veloray/templates/legacy/modernX.html")" == 'operator-owned template' ]]
pass legacy-template-edits-preserved-on-upgrade
: >"$HOST_ROOT/service-calls"
export TEST_SUBSCRIPTIONS_ACTIVE=1
host_install
grep -Fxq 'stop veloray-subscriptions.service' "$HOST_ROOT/service-calls"
grep -Fxq 'restart veloray-subscriptions.service' "$HOST_ROOT/service-calls"
pass independent-subscriptions-preserved-on-update
: >"$HOST_ROOT/service-calls"
bash "$HOST_ROOT/usr/local/bin/velorayctl" restart >"$TEST_ROOT/host-result" 2>&1
grep -Eq '^restart .*veloray-subscriptions.service' "$HOST_ROOT/service-calls"
grep -Fxq 'stop veloray-subscription-endpoint.timer veloray-subscription-endpoint.service' "$HOST_ROOT/service-calls"
grep -Fxq 'start veloray-subscription-endpoint.timer' "$HOST_ROOT/service-calls"
bash "$HOST_ROOT/usr/local/bin/velorayctl" stop >"$TEST_ROOT/host-result" 2>&1
grep -Eq '^stop .*veloray-subscriptions.service' "$HOST_ROOT/service-calls"
unset TEST_SUBSCRIPTIONS_ACTIVE
pass independent-subscriptions-controller-lifecycle
export TEST_SUBSCRIPTIONS_ACTIVE=1
: >"$HOST_ROOT/service-calls"
printf 'RESTORE\n' | bash "$HOST_ROOT/usr/local/bin/velorayctl" restore "$HOST_ROOT/recovery.tar.gz" >"$TEST_ROOT/host-result" 2>&1
grep -Fxq 'stop veloray-subscriptions.service' "$HOST_ROOT/service-calls"
grep -Fxq 'start veloray-subscriptions.service' "$HOST_ROOT/service-calls"
grep -Fxq 'start veloray-subscription-endpoint.timer' "$HOST_ROOT/service-calls"
unset TEST_SUBSCRIPTIONS_ACTIVE
pass independent-subscriptions-paused-during-restore
reset_host
export TEST_BOOTSTRAP_FAIL=1
if host_install;then echo 'Interrupted bootstrap accepted' >&2;exit 1;fi
test -f "$HOST_ROOT/etc/veloray/veloray.env";test -f "$HOST_ROOT/database";test ! -f "$HOST_ROOT/admin"
original_env="$(sha256sum "$HOST_ROOT/etc/veloray/veloray.env")"
unset TEST_BOOTSTRAP_FAIL
host_install
test -f "$HOST_ROOT/admin"
[[ "$(sha256sum "$HOST_ROOT/etc/veloray/veloray.env")" == "$original_env" ]]
pass host-interrupted-install-resumes
mkdir -p "$HOST_ROOT/var/lib/veloray-agent"
printf ledger >"$HOST_ROOT/var/lib/veloray-agent/state.json"
printf 'CANCEL\n' | bash "$HOST_ROOT/usr/local/bin/velorayctl" uninstall >"$TEST_ROOT/host-result" 2>&1
test -d "$HOST_ROOT/opt/veloray";pass uninstall-cancel
printf 'REMOVE\n' | bash "$HOST_ROOT/usr/local/bin/velorayctl" uninstall >"$TEST_ROOT/host-result" 2>&1
test ! -d "$HOST_ROOT/opt/veloray"
test ! -f "$HOST_ROOT/etc/systemd/system/veloray-subscriptions.service"
test ! -f "$HOST_ROOT/etc/systemd/system/veloray-subscription-endpoint.timer"
grep -Eq '^disable --now .*veloray-subscription-endpoint.timer' "$HOST_ROOT/service-calls"
grep -Eq '^disable --now .*veloray-auto-update.timer' "$HOST_ROOT/service-calls"
grep -Eq '^disable --now .*veloray-subscriptions.service' "$HOST_ROOT/service-calls"
pass uninstall-stops-subscriptions-and-updater
test -f "$HOST_ROOT/database";test -f "$HOST_ROOT/var/lib/veloray-agent/state.json"
[[ "$(sha256sum "$HOST_ROOT/etc/veloray/veloray.env")" == "$original_env" ]]
pass uninstall-keeps-data-and-keys
host_install
[[ "$(sha256sum "$HOST_ROOT/etc/veloray/veloray.env")" == "$original_env" ]]
test -f "$HOST_ROOT/admin";pass reinstall-after-uninstall
bash "$HOST_ROOT/usr/local/bin/velorayctl" >"$TEST_ROOT/host-result" 2>&1
grep -Fq 'Reset administrator password' "$TEST_ROOT/host-result"
grep -Fq 'Panel address' "$TEST_ROOT/host-result";pass default-management-menu
export TEST_DEGRADED=1
if bash "$HOST_ROOT/usr/local/bin/velorayctl" doctor >"$TEST_ROOT/host-result" 2>&1;then echo 'Degraded Xray accepted by doctor' >&2;exit 1;fi
grep -Fq 'Xray is stopped' "$TEST_ROOT/host-result";pass doctor-rejects-degraded-agent
if host_install;then echo 'Install announced success with stopped Xray' >&2;exit 1;fi
if grep -Fq 'Installation completed.' "$TEST_ROOT/host-result";then echo 'False installation success' >&2;exit 1;fi
pass install-rejects-stopped-xray
unset TEST_DEGRADED
export TEST_FAILED_UNIT=xray.service
if bash "$HOST_ROOT/usr/local/bin/velorayctl" doctor >"$TEST_ROOT/host-result" 2>&1;then echo 'Inactive Xray service accepted' >&2;exit 1;fi
pass doctor-checks-each-service
unset TEST_FAILED_UNIT
export TEST_XRAY_CONFIG_FAIL=1
if host_install;then echo 'Invalid Xray config accepted' >&2;exit 1;fi
grep -Fq 'Xray rejected' "$TEST_ROOT/host-result";pass install-rejects-invalid-xray
unset TEST_XRAY_CONFIG_FAIL
bash "$HOST_ROOT/usr/local/bin/velorayctl" reset >"$TEST_ROOT/host-result" 2>&1
test -f "$HOST_ROOT/password-reset"
grep -Fq 'Password changed' "$TEST_ROOT/host-result";pass administrator-reset-alias
rm -f "$HOST_ROOT/password-reset"
if printf 'Valid-long-password\nDifferent-password\n' | env -u VELORAY_ADMIN_PASSWORD bash "$HOST_ROOT/usr/local/bin/velorayctl" reset >"$TEST_ROOT/host-result" 2>&1;then echo 'Mismatched password confirmation accepted' >&2;exit 1;fi
test ! -f "$HOST_ROOT/password-reset";pass reset-rejects-password-mismatch
printf 'Valid-long-password\nValid-long-password\n' | env -u VELORAY_ADMIN_PASSWORD bash "$HOST_ROOT/usr/local/bin/velorayctl" reset >"$TEST_ROOT/host-result" 2>&1
test -f "$HOST_ROOT/password-reset"
if grep -Fq 'Valid-long-password' "$TEST_ROOT/host-result";then echo 'Password leaked into terminal output' >&2;exit 1;fi
pass reset-reads-one-password-with-confirmation
bash "$HOST_ROOT/usr/local/bin/velorayctl" backup-schedule daily >"$TEST_ROOT/host-result" 2>&1
test -f "$HOST_ROOT/etc/systemd/system/veloray-backup.timer"
grep -Fq 'enable --now veloray-backup.timer' "$HOST_ROOT/service-calls";pass daily-backup-schedule
bash "$HOST_ROOT/usr/local/bin/velorayctl" ssl admin@example.com >"$TEST_ROOT/host-result" 2>&1
grep -Fq -- '--webroot' "$HOST_ROOT/certbot-call"
if grep -Fq 'stop nginx' "$HOST_ROOT/service-calls";then echo 'Certificate request stopped nginx' >&2;exit 1;fi
pass certificate-keeps-nginx-running
VELORAY_LANGUAGE=fa bash "$HOST_ROOT/usr/local/bin/velorayctl" menu >"$TEST_ROOT/host-result" 2>&1
# The persistent setting is authoritative; explicitly save it before checking localization.
bash "$HOST_ROOT/usr/local/bin/velorayctl" help >"$TEST_ROOT/host-result" 2>&1
grep -Fq 'veloray reset' "$TEST_ROOT/host-result";pass management-help
printf '\nVELORAY_LANGUAGE=fa\n' >>"$HOST_ROOT/etc/veloray/veloray.env"
bash "$HOST_ROOT/usr/local/bin/velorayctl" menu >"$TEST_ROOT/host-result" 2>&1
grep -Fq 'Reset administrator password' "$TEST_ROOT/host-result";pass management-menu-stays-english
bash "$HOST_ROOT/usr/local/bin/velorayctl" status >"$TEST_ROOT/host-result" 2>&1
grep -Fq 'آدرس پنل:' "$TEST_ROOT/host-result";pass configured-cli-language-preserved
printf '\nVELORAY_LANGUAGE=en\n' >>"$HOST_ROOT/etc/veloray/veloray.env"
[[ "$(stat -c '%a' "$HOST_ROOT/var/log/veloray")" == 700 ]]
if find "$HOST_ROOT/var/log/veloray" -type f ! -perm 0600 | grep -q .;then echo 'Operation log is not private' >&2;exit 1;fi
pass private-operation-logs
if [[ -n "${VELORAY_TEST_PANEL_BINARY:-}" ]];then
 VELORAY_ENV_FILE="$HOST_ROOT/etc/veloray/veloray.env" "$VELORAY_TEST_PANEL_BINARY" >"$TEST_ROOT/host-result" 2>&1
 grep -Fq 'Reset administrator password' "$TEST_ROOT/host-result";pass compiled-default-opens-menu
 VELORAY_ENV_FILE="$HOST_ROOT/etc/veloray/veloray.env" "$VELORAY_TEST_PANEL_BINARY" auto-update status >"$TEST_ROOT/host-result" 2>&1
 grep -Fq 'Repository: ParsaZali/VeloRay (main)' "$TEST_ROOT/host-result"
 pass compiled-auto-update-dispatch
 VELORAY_ENV_FILE="$HOST_ROOT/etc/veloray/veloray.env" "$VELORAY_TEST_PANEL_BINARY" reset >"$TEST_ROOT/host-result" 2>&1
 grep -Fq 'Existing sessions were revoked' "$TEST_ROOT/host-result";pass compiled-reset-alias
fi
reset_host;touch "$HOST_ROOT/database" "$HOST_ROOT/role"
if host_install;then echo 'Preserved database with lost keys accepted' >&2;exit 1;fi
grep -Fq 'Restore this environment file from your backup' "$TEST_ROOT/host-result"
test ! -f "$HOST_ROOT/etc/veloray/veloray.env";pass orphan-database-protected
reset_host
export VELORAY_PANEL_PORT=8610
if host_install;then echo 'Reserved panel port accepted' >&2;exit 1;fi
grep -Fq 'Invalid or reserved panel port' "$TEST_ROOT/host-result";pass reserved-panel-port
unset VELORAY_PANEL_PORT
reset_host
export TEST_PORTS_CONFLICT=1 TEST_FAILED_UNIT=xray.service
if host_install;then echo 'Occupied Xray port accepted' >&2;exit 1;fi
grep -Fq 'nginx pid=456' "$TEST_ROOT/host-result"
grep -Fq 'The panel is available' "$TEST_ROOT/host-result"
grep -Fq 'inbound-port ID PORT' "$TEST_ROOT/host-result"
grep -Fq 'restart veloray-web.service' "$HOST_ROOT/service-calls"
if grep -Eq '(restart xray|stop nginx)' "$HOST_ROOT/service-calls";then echo 'Port preflight disrupted a runtime or another service' >&2;exit 1;fi
pass occupied-port-keeps-panel-and-other-services
if bash "$HOST_ROOT/usr/local/bin/velorayctl" ports >"$TEST_ROOT/host-result" 2>&1;then echo 'Occupied port inspection returned success' >&2;exit 1;fi
grep -Fq 'nginx pid=456' "$TEST_ROOT/host-result";pass port-command-shows-owner
if bash "$HOST_ROOT/usr/local/bin/velorayctl" repair >"$TEST_ROOT/host-result" 2>&1;then echo 'Repair restarted into an occupied port' >&2;exit 1;fi
pass repair-refuses-occupied-port
bash "$HOST_ROOT/usr/local/bin/velorayctl" inbound-port 1 2053 >"$TEST_ROOT/host-result" 2>&1
test -f "$HOST_ROOT/ports-resolved";pass inbound-port-command
unset TEST_FAILED_UNIT
bash "$HOST_ROOT/usr/local/bin/velorayctl" repair >"$TEST_ROOT/host-result" 2>&1
grep -Fq 'reset-failed xray.service' "$HOST_ROOT/service-calls"
pass recovery-after-port-change
grep -Fq 'User=veloray-xray' "$HOST_ROOT/etc/systemd/system/xray.service"
grep -Fq 'StartLimitBurst=3' "$HOST_ROOT/etc/systemd/system/xray.service";pass dedicated-runtime-account-and-restart-limit
unset TEST_PORTS_CONFLICT
export PATH="$bootstrap_test_path"
printf '%s installer checks passed.\n' "$passed"
