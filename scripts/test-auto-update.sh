#!/usr/bin/env bash
# Unit checks for the side-effect-free GitHub update-check path.
set -Eeuo pipefail
cd "$(dirname "$0")/.."
source scripts/auto-update.sh

TEST_HOME="$(mktemp -d)"
trap 'rm -rf -- "$TEST_HOME"' EXIT
TEST_COMMIT=1111111111111111111111111111111111111111
installed="$TEST_HOME/installed"
mode=success
ci_status=completed
calls=0

vr_auto_download() {
 local url="$1" output="$2"
 calls=$((calls + 1))
 case "$url" in
  */commits/main)
   if [[ "$mode" == outage ]];then return 22;fi
   printf '{"sha":"%s"}\n' "$TEST_COMMIT" >"$output"
   ;;
  */actions/workflows/ci.yml/runs*)
   printf '{"workflow_runs":[{"head_sha":"%s","head_branch":"main","event":"push","status":"%s","conclusion":"%s"}]}\n' "$TEST_COMMIT" "$ci_status" "$mode" >"$output"
   ;;
  *) echo "Unexpected GitHub endpoint: $url" >&2;return 1;;
 esac
}

pass=0
expect() {
 local phrase="$1" output="$2"
 [[ "$output" == *"$phrase"* ]] || { echo "Expected '$phrase' in $output" >&2;exit 1; }
 pass=$((pass + 1))
}

printf '%s\n' "$TEST_COMMIT" >"$installed"
output="$(vr_auto_update_check check ParsaZali/VeloRay "$installed")"
expect 'Already up to date' "$output"

printf '%s\n' 0000000000000000000000000000000000000000 >"$installed"
output="$(vr_auto_update_check check ParsaZali/VeloRay "$installed")"
expect 'Verified GitHub update available' "$output"

if VELORAY_EXPECTED_UPDATE_COMMIT=2222222222222222222222222222222222222222 vr_auto_update_check check ParsaZali/VeloRay "$installed" >"$TEST_HOME/pinned-output" 2>&1;then
 echo 'A stale UI update target was accepted.' >&2;exit 1
fi
expect 'branch head changed' "$(cat "$TEST_HOME/pinned-output")"

mode=failure
output="$(vr_auto_update_check check ParsaZali/VeloRay "$installed")"
expect 'skipping unsafe update' "$output"

mode=success
ci_status=in_progress
output="$(vr_auto_update_check check ParsaZali/VeloRay "$installed")"
expect 'CI is in_progress' "$output"
expect 'update is not ready yet' "$output"
ci_status=completed

mode=workflow_dispatch
# A green manual run must never authorize deployment of a commit.
vr_auto_download() {
 local url="$1" output="$2"
 case "$url" in
  */commits/main) printf '{"sha":"%s"}\n' "$TEST_COMMIT" >"$output";;
  */actions/workflows/ci.yml/runs*) printf '{"workflow_runs":[{"head_sha":"%s","head_branch":"main","event":"workflow_dispatch","status":"completed","conclusion":"success"}]}\n' "$TEST_COMMIT" >"$output";;
  *) return 1;;
 esac
}
output="$(vr_auto_update_check check ParsaZali/VeloRay "$installed")"
expect 'update is not ready yet' "$output"

mode=outage
vr_auto_download() {
 local url="$1" output="$2"
 if [[ "$url" == */commits/main ]];then return 22;fi
 return 1
}
if vr_auto_update_check check ParsaZali/VeloRay "$installed" >"$TEST_HOME/output" 2>&1;then
 echo 'Network outage was incorrectly treated as a safe update.' >&2
 exit 1
fi
expect 'Could not reach GitHub' "$(cat "$TEST_HOME/output")"

echo "$pass automatic update checks passed"
