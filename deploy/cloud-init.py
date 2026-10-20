#!/usr/bin/env python3
"""Create an Ubuntu 24.04 first-boot deployment without embedding credentials."""
import argparse
import json
import re
import shlex


def cloud_config(domain: str, revision: str = "main") -> dict:
    if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9.-]{0,251}[a-zA-Z0-9]", domain) or "." not in domain or ".." in domain:
        raise ValueError("A public DNS hostname is required")
    if revision != "main" and not re.fullmatch(r"[a-f0-9]{40}", revision):
        raise ValueError("Revision must be main or an exact 40-character commit")
    domain = domain.lower()
    script = r'''#!/bin/bash
set -Eeuo pipefail
umask 077
exec 9>/run/lock/veloray-firstboot.lock
flock -n 9 || exit 0
[[ ! -f /var/lib/veloray-cloud-init/complete ]] || exit 0
install -d -m 0700 /var/lib/veloray-cloud-init
if [[ ! -d /opt/veloray/.git ]];then
 [[ ! -e /opt/veloray ]] || { echo 'Existing /opt/veloray preserved.' >&2; exit 1; }
 git clone --depth 1 https://github.com/ParsaZali/VeloRay.git /opt/veloray
fi
cd /opt/veloray
[[ "$(git remote get-url origin)" == https://github.com/ParsaZali/VeloRay.git ]] || exit 1
[[ -z "$(git status --porcelain)" ]] || { echo 'Local changes preserved; review before resuming.' >&2; exit 1; }
REVISION=__REVISION__
if [[ "$REVISION" != main ]];then git fetch --depth 1 origin "$REVISION"; git checkout --detach "$REVISION";fi
source scripts/auto-update.sh
commit="$(git rev-parse HEAD)"
ready=false
for attempt in {1..20};do
 state="$(vr_auto_ci_state ParsaZali/VeloRay "$commit" /var/lib/veloray-cloud-init/ci.json)"
 if [[ "$state" == success ]];then ready=true;break;fi
 case "$state" in missing|queued|in_progress|requested|waiting|pending) sleep 15;;*) echo 'Selected revision has no successful main CI.' >&2;exit 1;;esac
done
[[ "$ready" == true ]] || { echo 'CI verification is not ready; rerun veloray-firstboot later.' >&2;exit 1; }
bash scripts/verify-runtime.sh
sha256sum --check --quiet SHA256SUMS
if [[ ! -f deploy/docker/.env ]];then python3 deploy/docker/generate.py --domain __DOMAIN__;fi
docker compose --env-file deploy/docker/.env -f deploy/docker/compose.yaml up -d --build --wait --wait-timeout 180
curl --fail --silent --show-error --retry 12 --retry-connrefused --retry-delay 10 --max-time 5 --resolve __RESOLVE__ __HEALTH__ >/dev/null
printf '%s\n' "$commit" >/var/lib/veloray-cloud-init/complete
echo 'VeloRay first-boot deployment verified. Administrator credentials remain in the private Docker secrets directory.'
'''
    script = script.replace("__REVISION__", shlex.quote(revision)).replace("__DOMAIN__", shlex.quote(domain)).replace("__RESOLVE__", shlex.quote(f"{domain}:443:127.0.0.1")).replace("__HEALTH__", shlex.quote(f"https://{domain}/api/health"))
    return {"package_update": True, "packages": ["ca-certificates", "curl", "git", "python3", "jq", "xz-utils", "docker.io", "docker-compose-v2"], "write_files": [{"path": "/usr/local/sbin/veloray-firstboot", "owner": "root:root", "permissions": "0700", "content": script}], "runcmd": [["systemctl", "enable", "--now", "docker"], ["/usr/local/sbin/veloray-firstboot"]]}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--domain", required=True)
    parser.add_argument("--revision", default="main")
    args = parser.parse_args()
    try:
        output = cloud_config(args.domain, args.revision)
    except ValueError as exc:
        parser.exit(2, str(exc) + "\n")
    # JSON is valid YAML; this avoids interpolating operator input into YAML.
    print("#cloud-config\n" + json.dumps(output, ensure_ascii=False, indent=2))
