#!/usr/bin/env bash
# Automatic updates are hosted by the root-only systemd service.
# The repository, branch, CI gate, installer and data directory are intentionally fixed.
# This file is sourced by velorayctl after /etc/veloray/veloray.env is loaded.

vr_autoupdate() {
 local action="${1:-status}" repo="ParsaZali/VeloRay"
 local installed=/var/lib/veloray/installed-commit
 local timer=veloray-auto-update.timer
 case "$action" in
  enable)
   [[ -r /opt/veloray/current/deploy/systemd/veloray-auto-update.timer ]] || { echo 'Auto-update units are missing. Install the latest VeloRay first.' >&2;return 1; }
   install -m 0644 /opt/veloray/current/deploy/systemd/veloray-auto-update.{service,timer} /etc/systemd/system/
   systemctl daemon-reload
   systemctl enable --now "$timer"
   vr_say 'VeloRay GitHub auto-update enabled. It checks main daily after a randomized delay.' 'به‌روزرسانی خودکار VeloRay فعال شد؛ شاخه اصلی روزانه بررسی می‌شود.'
   ;;
  disable)
   systemctl disable --now "$timer" 2>/dev/null || true
   vr_say 'Automatic updates disabled; installed files and backups are unchanged.' 'به‌روزرسانی خودکار غیرفعال شد؛ فایل‌ها و بکاپ‌ها حفظ شدند.'
   ;;
  status)
   local current=unknown
   [[ ! -f "$installed" ]] || current="$(cat "$installed")"
   printf 'Repository: %s (main)\nInstalled commit: %s\n' "$repo" "$current"
   if systemctl is-enabled --quiet "$timer" 2>/dev/null;then
    printf 'Automatic updates: enabled\n'
    systemctl list-timers "$timer" --no-pager 2>/dev/null || true
   else
    printf 'Automatic updates: disabled\n'
   fi
   ;;
  check|run)
   vr_auto_update_check "$action" "$repo" "$installed"
   ;;
  *)
   echo 'Usage: sudo veloray auto-update enable|disable|status|check|run' >&2
   return 2
   ;;
 esac
}

vr_auto_download() {
 curl --proto '=https' --proto-redir '=https' --tlsv1.2 \
  --fail --silent --show-error --location --retry 2 \
  --connect-timeout 10 --max-time 30 -H 'Accept: application/vnd.github+json' \
  "$1" -o "$2"
}

vr_auto_restore() {
 local previous="$1" original_env="$2" architecture="$3" subscriptions_active="${4:-false}"
 if [[ ! -d "$previous" || ! -x "$previous/bin/linux-$architecture/veloray" || ! -x "$previous/bin/linux-$architecture/veloray-agent" ]];then
  echo 'Unable to automatically restore: previous release binaries unavailable. Manual recovery required.' >&2
  return 1
 fi
 ln -sfn "$previous" /opt/veloray/current
 install -m 0755 "$previous/bin/linux-$architecture/veloray" /usr/local/bin/veloray
 install -m 0755 "$previous/bin/linux-$architecture/veloray-agent" /usr/local/bin/veloray-agent
 install -m 0755 "$previous/scripts/velorayctl" /usr/local/bin/velorayctl
 install -m 0644 "$original_env" /etc/veloray/veloray.env
 chown root:veloray /etc/veloray/veloray.env
 chmod 0640 /etc/veloray/veloray.env
 for unit in veloray-web veloray-agent veloray-subscriptions xray sing-box;do
  [[ ! -f "$previous/deploy/systemd/$unit.service" ]] || install -m 0644 "$previous/deploy/systemd/$unit.service" "/etc/systemd/system/$unit.service"
 done
 systemctl daemon-reload
 systemctl restart veloray-agent.service veloray-web.service xray.service sing-box.service
 if [[ "$subscriptions_active" == true ]];then systemctl restart veloray-subscriptions.service;fi
 /usr/local/bin/velorayctl doctor
}

vr_auto_ci_state() {
 local repo="$1" commit="$2" output="$3"
 vr_auto_download "https://api.github.com/repos/$repo/actions/workflows/ci.yml/runs?branch=main&per_page=30" "$output" || return 2
 jq -r --arg sha "$commit" '
  [.workflow_runs[] | select(.head_sha==$sha and .head_branch=="main" and .event=="push")] | first
  | if . == null then "missing"
    elif .status != "completed" then .status
    else (.conclusion // "missing")
    end
 ' "$output"
}

vr_auto_update_check() (
 local action="$1" repo="$2" installed="$3"
 local tmp commit previous architecture current backup status attempts delay attempt subscriptions_active=false
 if [[ "$action" == run ]];then
  exec 8>/run/lock/veloray-auto-update.lock
  flock -n 8 || { echo 'An automatic update is already running; skipping.';return 0; }
 fi
 tmp="$(mktemp -d /var/tmp/veloray-update.XXXXXXXX)"
 chmod 0700 "$tmp"
 trap 'rm -rf -- "$tmp"' EXIT
 vr_auto_download "https://api.github.com/repos/$repo/commits/main" "$tmp/commit.json" || { echo 'Could not reach GitHub; current version remains active.' >&2;return 1; }
 commit="$(jq -er '.sha' "$tmp/commit.json")" || return 1
 [[ "$commit" =~ ^[a-f0-9]{40}$ ]] || { echo 'GitHub supplied an invalid commit ID.' >&2;return 1; }
 current=unknown
 if [[ -f "$installed" ]];then current="$(cat "$installed")";fi
 if [[ "$commit" == "$current" ]];then echo "Already up to date: $commit";return 0;fi

 # Do not deploy a branch head until that exact revision has passed the main CI.
 status="$(vr_auto_ci_state "$repo" "$commit" "$tmp/ci.json")" || {
  echo 'CI verification unavailable; not updating.' >&2
  return 1
 }
 if [[ "$action" == run ]];then
  attempts="${VELORAY_CI_WAIT_ATTEMPTS:-20}"
  delay="${VELORAY_CI_WAIT_SECONDS:-15}"
  [[ "$attempts" =~ ^[0-9]+$ ]] || attempts=20
  [[ "$delay" =~ ^[0-9]+$ ]] || delay=15
  attempt=0
  while [[ "$status" == missing || "$status" == queued || "$status" == in_progress || "$status" == requested || "$status" == waiting || "$status" == pending ]];do
   ((attempt >= attempts)) && break
   attempt=$((attempt + 1))
   echo "GitHub main at $commit; CI is $status. Waiting for verification ($attempt/$attempts)..."
   sleep "$delay"
   status="$(vr_auto_ci_state "$repo" "$commit" "$tmp/ci.json")" || {
    echo 'CI verification unavailable; not updating.' >&2
    return 1
   }
  done
 fi
 case "$status" in
  success) ;;
  missing|queued|in_progress|requested|waiting|pending)
   echo "GitHub main at $commit, but CI is $status; update is not ready yet."
   return 0
   ;;
  *)
   echo "GitHub main at $commit, but CI is $status; skipping unsafe update."
   return 0
   ;;
 esac
 echo "Verified GitHub update available: $current -> $commit"
 [[ "$action" == run ]] || return 0
 previous="$(readlink -f /opt/veloray/current)"
 [[ -d "$previous" ]] || { echo 'Previous release cannot be located; refusing unattended update.' >&2;return 1; }
 case "$(uname -m)" in x86_64) architecture=amd64;;aarch64|arm64) architecture=arm64;;*) echo 'Unsupported architecture';return 1;;esac
 [[ -x "$previous/bin/linux-$architecture/veloray" ]] || { echo 'Cannot roll back this installation safely.' >&2;return 1; }

 vr_auto_download "https://api.github.com/repos/$repo/contents/install.sh?ref=$commit" "$tmp/installer.json"
 jq -er '.content' "$tmp/installer.json" | base64 -d >"$tmp/install.sh"
 [[ -s "$tmp/install.sh" ]] || { echo 'The GitHub installer is empty.' >&2;return 1; }
 # Compare with GitHub's immutable git-blob hash, not the mutable branch URL.
 local blob_hash
 blob_hash="$( (printf 'blob %s\0' "$(wc -c <"$tmp/install.sh")";cat "$tmp/install.sh") | sha1sum | cut -d' ' -f1)"
 [[ "$blob_hash" == "$(jq -r '.sha' "$tmp/installer.json")" ]] || { echo 'Pinned installer failed integrity validation.' >&2;return 1; }
 bash -n "$tmp/install.sh" || return 1

 # Backups contain encrypted credentials and database state; do not publish or
 # transfer them outside the root-only local backup directory.
 install -d -m 0700 /var/lib/veloray/backups
 backup="/var/lib/veloray/backups/pre-update-$(date -u +%Y%m%dT%H%M%SZ).tar.gz"
 /usr/local/bin/velorayctl backup "$backup" || { echo 'Backup failed; refusing to update.' >&2;return 1; }
 cp -p /etc/veloray/veloray.env "$tmp/original.env"
 chmod 0600 "$tmp/original.env"
 if [[ -f "$installed" ]];then cp -p "$installed" "$tmp/original-commit";fi
 if systemctl is-active --quiet veloray-subscriptions.service;then subscriptions_active=true;fi
 echo "Starting pinned update $commit (backup: $backup)"
 if VELORAY_INSTALL_REPO="$repo" VELORAY_TRACKING_REF=main \
     bash "$tmp/install.sh" install --repo "$repo" --ref "$commit" --lang en \
     && /usr/local/bin/velorayctl doctor;then
  install -d -m 0750 /var/lib/veloray
  printf '%s\n' "$commit" >"$installed"
  chmod 0644 "$installed"
  echo "Automatic update succeeded: $commit"
 else
  echo "Update failed. Attempting application/service rollback to $previous." >&2
  # The installer may have already recorded the candidate commit. Restore the
  # original marker or future checks could mistake a rolled-back build as new.
  if [[ -f "$tmp/original-commit" ]];then
   install -m 0644 "$tmp/original-commit" "$installed"
  else
   rm -f -- "$installed"
  fi
  if vr_auto_restore "$previous" "$tmp/original.env" "$architecture" "$subscriptions_active";then
   echo 'Previous binaries and services were restored. Review the PostgreSQL backup before any database rollback.' >&2
  else
   echo "Recovery needs manual intervention. Recovery backup: $backup" >&2
  fi
  return 1
 fi
)
