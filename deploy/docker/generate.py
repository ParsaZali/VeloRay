#!/usr/bin/env python3
"""Generate private deployment credentials without printing their contents."""
import argparse
import ipaddress
import os
from pathlib import Path
import re
import secrets


def generate(directory: Path, domain: str) -> None:
    if len(domain) > 253 or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]|[A-Za-z0-9]", domain):
        raise ValueError("Use a hostname without scheme, path or port")
    try:
        ipaddress.ip_address(domain)
    except ValueError:
        pass
    else:
        raise ValueError("Use a DNS hostname for automatic public TLS")
    if any(not part or len(part) > 63 or part.startswith("-") or part.endswith("-") for part in domain.split(".")):
        raise ValueError("Invalid DNS hostname")
    secret_dir = directory / "secrets"
    if secret_dir.exists() or (directory / ".env").exists():
        raise ValueError("Existing deployment preserved; do not regenerate encryption keys")
    directory.mkdir(parents=True, exist_ok=True)
    secret_dir.mkdir(mode=0o700)
    try:
        for name in ("database_password", "secret_key", "field_key", "admin_password", "agent_token"):
            # The parent is operator-only. File mounts must be readable by each
            # container's dedicated non-root identity (Compose local secrets).
            path = secret_dir / name
            with path.open("x", encoding="utf-8") as stream:
                stream.write(("Vr!" + secrets.token_hex(32) if name == "admin_password" else secrets.token_urlsafe(48)) + "\n")
            path.chmod(0o444)
        env = directory / ".env"
        with env.open("x", encoding="utf-8") as stream:
            stream.write(f"VELORAY_DOMAIN={domain.lower()}\n")
        env.chmod(0o600)
        (directory / "certificates").mkdir(mode=0o755)
    except BaseException:
        # Leave partial state for explicit operator recovery; never replace keys.
        raise


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--domain", required=True)
    parser.add_argument("--directory", type=Path, default=Path(__file__).resolve().parent)
    args = parser.parse_args()
    os.umask(0o077)
    try:
        generate(args.directory, args.domain)
    except ValueError as exc:
        parser.exit(2, str(exc) + "\n")
    print("Deployment files generated. Credentials are in the private secrets directory.")
