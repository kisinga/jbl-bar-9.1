#!/usr/bin/env python3
"""Unpack historical JBL One-family update archives without mounting/executing them.

Creates package/, images/, evidence/, and optionally rootfs/ in a NEW directory.
Firmware remains private. ZIP CRCs and the vendor update.zip MD5 are checked;
these checks detect corruption, not vendor authenticity or signature validity.
"""
import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import subprocess
import zipfile

MAX_MEMBER = 1024 * 1024 * 1024
MAX_TOTAL = 2 * MAX_MEMBER
IMAGES = {"tee.img", "boot.img", "system.ext4", "production_group_jblbar",
          "recovery.ext4", "scp.img", "dts.zip"}


def identity(path):
    hashes = {name: hashlib.new(name) for name in ("sha256", "md5")}
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            for digest in hashes.values():
                digest.update(block)
    return {"bytes": path.stat().st_size, **{n: h.hexdigest() for n, h in hashes.items()}}


def unpack(archive, destination, inner=False):
    destination.mkdir(exist_ok=False)
    rows, names, total = [], set(), 0
    with zipfile.ZipFile(archive) as source:
        for member in source.infolist():
            name = PurePosixPath(member.filename)
            if name.is_absolute() or ".." in name.parts or "\\" in member.filename:
                raise ValueError(f"Unsafe ZIP path: {member.filename}")
            if member.is_dir():
                continue
            if stat.S_ISLNK(member.external_attr >> 16) or member.flag_bits & 1:
                raise ValueError("Symlink/encrypted ZIP member is unsupported")
            allowed = name.name in IMAGES if inner else (
                name.name in {"checksum.txt", "update.zip", "update.zip.sign"}
                or name.suffix.lower() == ".pdf")
            if not allowed or name.name in names:
                raise ValueError(f"Unknown or duplicate update member: {member.filename}")
            total += member.file_size
            if member.file_size > MAX_MEMBER or total > MAX_TOTAL:
                raise ValueError("Archive exceeds extraction budget")
            names.add(name.name)
            output = destination / name.name
            with source.open(member) as stream, output.open("xb") as target:
                shutil.copyfileobj(stream, target, 1024 * 1024)
            rows.append({"member": member.filename, "file": name.name,
                         "crc32": f"{member.CRC:08x}", **identity(output)})
    return rows


def extract_rootfs(image, output, log):
    debugfs = shutil.which("debugfs") or next(
        (p for p in ("/usr/sbin/debugfs", "/sbin/debugfs") if Path(p).is_file()), None)
    if not debugfs or not shutil.which("bwrap"):
        raise RuntimeError("Filesystem extraction requires local debugfs and bwrap")
    debugfs = str(Path(debugfs).resolve())
    output.mkdir(exist_ok=False)
    command = ["bwrap", "--unshare-all", "--die-with-parent"]
    for path in ("/usr", "/lib", "/lib64"):
        if Path(path).exists():
            command += ["--ro-bind", path, path]
    command += ["--ro-bind", str(image), "/system.ext4", "--bind", str(output), "/out",
                "--proc", "/proc", "--dev", "/dev", "--", debugfs,
                "-R", "rdump / /out", "/system.ext4"]
    with log.open("w") as stream:
        subprocess.run(command, stdout=stream, stderr=subprocess.STDOUT,
                       check=True, timeout=180)
    # debugfs can return zero despite per-file errors; callers must inspect log.
    lines = log.read_text().splitlines()
    warnings = [line for line in lines if "while changing ownership" in line]
    other = [line for line in lines if "while changing ownership" not in line
             and not line.startswith("debugfs ")]
    return {"ownership_warnings": len(warnings), "other_diagnostics": other,
            "limitations": ["Ownership is not preserved in the unprivileged namespace.",
                            "Symlinks may refer to runtime /data; never resolve them on the host.",
                            "Extraction does not establish runtime or hardware behavior."]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("archive", type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--extract-filesystem", action="store_true")
    args = parser.parse_args()
    archive, out = args.archive.resolve(), args.output.resolve()
    out.mkdir(parents=True, exist_ok=False)
    evidence = out / "evidence"
    evidence.mkdir()
    report = {"archive": {"filename": archive.name, **identity(archive)}}
    report["package"] = unpack(archive, out / "package")
    update = out / "package/update.zip"
    checksum = out / "package/checksum.txt"
    expected = re.search(r"\b[0-9a-fA-F]{32}\b", checksum.read_text()).group().lower()
    actual = identity(update)["md5"]
    report["vendor_md5"] = {"expected": expected, "actual": actual, "matches": expected == actual}
    if expected != actual:
        raise ValueError("Vendor update.zip checksum mismatch")
    report["images"] = unpack(update, out / "images", inner=True)
    if args.extract_filesystem:
        report["rootfs"] = extract_rootfs(out / "images/system.ext4", out / "rootfs",
                                           evidence / "debugfs.log")
    (evidence / "extraction.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps({"output": str(out), "vendor_md5_matches": True,
                      "images": len(report["images"])}))


if __name__ == "__main__":
    main()
