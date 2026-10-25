#!/usr/bin/env bash
# Use Docker's signed official apt repository; never pipe a remote script to root.
set -Eeuo pipefail
[[ ${EUID:-$(id -u)} -eq 0 ]] || { echo 'Run with sudo.' >&2; exit 1; }
if command -v docker >/dev/null && docker compose version >/dev/null 2>&1;then
 docker info >/dev/null 2>&1 || systemctl start docker
 docker info >/dev/null
 exit 0
fi
source /etc/os-release
[[ "$ID" == ubuntu && "$VERSION_ID" =~ ^(22.04|24.04)$ ]] || { echo 'Supported hosts: Ubuntu 22.04 and 24.04.' >&2; exit 1; }
# Preserve other applications and existing containerd deployments.
for package in docker.io docker-compose docker-compose-v2 podman-docker containerd runc;do
 if dpkg-query -W -f='${Status}' "$package" 2>/dev/null | grep -q 'install ok installed';then
  echo "Existing $package installation detected. Install Compose v2 for that Docker installation before continuing; no packages were removed." >&2
  exit 1
 fi
done
install -d -m 0755 /etc/apt/keyrings
temporary="$(mktemp)";trap 'rm -f -- "$temporary"' EXIT
curl --proto '=https' --tlsv1.2 -fsS --retry 2 --connect-timeout 15 --max-time 60 https://download.docker.com/linux/ubuntu/gpg -o "$temporary"
install -m 0644 "$temporary" /etc/apt/keyrings/docker.asc
cat >/etc/apt/sources.list.d/veloray-docker.sources <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${UBUNTU_CODENAME:-$VERSION_CODENAME}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF
apt-get -o Acquire::Retries=2 update
env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=l apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
docker info >/dev/null
docker compose version
