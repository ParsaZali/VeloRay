#!/usr/bin/env python3
"""Package the verified source and both Linux runtimes for GitHub upload."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import tempfile
import zipfile


def main():
    project = Path(__file__).resolve().parents[1]
    version = (project / "VERSION").read_text().strip()
    if not re.fullmatch(r"[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[A-Za-z0-9.-]+)?", version):
        raise SystemExit("Invalid release version.")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, default=project.parent / f"VeloRay-v{version}-GitHub.zip")
    output = parser.parse_args().output.resolve()
    if output.is_relative_to(project):
        raise SystemExit("Choose an output path outside the source directory.")
    subprocess.run(["bash", "scripts/verify-runtime.sh"], cwd=project, check=True)
    excluded = {"node_modules", "dist", "bin", "test-results", "playwright-report", "__pycache__", ".git", "release"}
    files = []
    for directory, directories, names in os.walk(project):
        directories[:] = sorted(name for name in directories if name not in excluded)
        for name in sorted(names):
            path = Path(directory) / name
            relative = path.relative_to(project)
            if name == ".git" or name.endswith((".tsbuildinfo", ".pyc", ".pyo")) or (name.startswith(".env") and name != ".env.example"):
                continue
            if path.is_symlink():
                raise SystemExit(f"Source package cannot contain symlinks: {relative}")
            if path.suffix in {".db", ".sqlite", ".sqlite3", ".dump", ".log", ".key", ".pem"}:
                raise SystemExit(f"Remove private/generated files before packaging: {relative}")
            if relative.parts[0] == "runtime" and name not in {"SHA256SUMS", "linux-amd64.tar.xz", "linux-arm64.tar.xz"}:
                continue
            files.append(path)
    checksum_file = project / "SHA256SUMS"
    files = sorted(set(files + [checksum_file]), key=lambda path: path.relative_to(project).as_posix())
    manifest = "".join(
        hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.relative_to(project).as_posix() + "\n"
        for path in files if path != checksum_file
    )
    checksum_file.write_text(manifest)
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="veloray-release-", dir=output.parent) as temporary:
        archive = Path(temporary) / output.name
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=6) as packed:
            for path in files:
                packed.write(path, "VeloRay/" + path.relative_to(project).as_posix(),
                             compress_type=zipfile.ZIP_STORED if path.suffix == ".xz" else zipfile.ZIP_DEFLATED)
        with zipfile.ZipFile(archive) as packed:
            if packed.testzip() is not None:
                raise SystemExit("Release ZIP failed its integrity check.")
            for line in manifest.splitlines():
                digest, name = line.split("  ", 1)
                if hashlib.sha256(packed.read("VeloRay/" + name)).hexdigest() != digest:
                    raise SystemExit(f"Release checksum mismatch: {name}")
            extracted = Path(temporary) / "extracted"
            packed.extractall(extracted)
            subprocess.run(["bash", "scripts/verify-runtime.sh"], cwd=extracted / "VeloRay", check=True)
        os.replace(archive, output)
    print(f"Complete release: {output} ({len(files)} files, {output.stat().st_size} bytes)")
    print(f"SHA256: {hashlib.sha256(output.read_bytes()).hexdigest()}")


if __name__ == "__main__":
    main()
