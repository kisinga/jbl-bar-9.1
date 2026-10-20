#!/usr/bin/env python3
"""Build the small native EQ parser against an owner's extracted vendor libraries.

Requires an ARM hard-float GNU/Linux GCC toolchain (validated with 14.2.rel1).
Only the MIT helper is emitted; firmware libraries stay in the supplied rootfs.
The helper exports defaults/registry and parses JSON. It never applies tuning.
"""
from pathlib import Path
import argparse
import hashlib
import os
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
# Runtime dependencies, verified against the supported Bar 9.1 firmware.
# Kept absolute so the server can use the same list for runtime guards.
DEPENDENCIES = {
    "/usr/lib/libhdsp.so": "1dcb71b2cd28df3fd8d36a16cd8cf71e47344ecb063cfab9efc55fcd94104258",
    "/system/bdplib/ld-linux.so.3": "49343df4311cb08d47dce9705eebcd06b666509d797ea018ef7c3142cb1f8af9",
    "/system/bdplib/libc.so.6": "2f3952341c498325cc9423acadcf7c2ad94ee9196af071c661ffd16057552dc0",
    "/system/bdplib/libdl.so.2": "a90018084bd508ce2a1a8d4a581cb3caebb0dfcd357f60b9e16a137850a064c6",
    "/system/bdplib/libgcc_s.so.1": "8cefc5883637998effb2b60cf89115692fcfea32845892e4e3b3570b036190d1",
    "/system/bdplib/libm.so.6": "8a5316e5d611f775819f77509014278d06335b03d8569cf2ff9dafd2e1e104c5",
    "/system/bdplib/libstdc++.so.6": "59a59e75b7ad46d45b239411ada16818b56f48803c6e25195cdcd266ed86a67d",
}


def validate_rootfs(rootfs):
    """Accept extracted rootfs/{bdplib,system/bdplib}; never patch its contents."""
    rootfs = Path(rootfs).resolve()
    roots = [rootfs, rootfs / "system"]
    firmware = next((p for p in roots if (p / "bdplib/libc.so.6").is_file()), None)
    if firmware is None:
        raise ValueError("Rootfs must contain bdplib/libc.so.6 or system/bdplib/libc.so.6")
    for device_path, expected in DEPENDENCIES.items():
        local = firmware / device_path.removeprefix("/system/").lstrip("/")
        if not local.is_file() or hashlib.sha256(local.read_bytes()).hexdigest() != expected:
            raise ValueError(f"Unsupported or missing firmware dependency: {device_path}")
    return firmware


def build(cc, rootfs, output):
    firmware = validate_rootfs(rootfs)
    compiler = shutil.which(str(cc))
    if compiler is None:
        raise ValueError("ARM Linux GCC not found; pass its path with --cc")
    compiler = str(Path(compiler).resolve())
    output = Path(output).resolve()
    if output.is_relative_to(firmware):
        raise ValueError("Output must be outside the extracted firmware")
    output.parent.mkdir(parents=True, exist_ok=True)
    vendor = firmware / "bdplib"

    def compiler_file(name):
        value = subprocess.check_output([compiler, "-print-file-name=" + name], text=True).strip()
        if not Path(value).is_file():
            raise ValueError(f"Toolchain is missing {name}")
        return value

    with tempfile.TemporaryDirectory(prefix="jbl-eq-build-") as folder:
        obj = Path(folder) / "eq_parser.o"
        subprocess.run([
            compiler, "-Os", "-Wall", "-Wextra", "-Werror", "-fno-pie", "-fno-stack-protector",
            "-D_FORTIFY_SOURCE=0", "-c", str(ROOT / "webremote/native/eq_parser.c"), "-o", str(obj),
        ], check=True)
        # Use modern toolchain CRT objects with the pinned old libc. The result
        # imports __libc_start_main@GLIBC_2.4, without bundling a replacement libc.
        executable = Path(folder) / "eq-parser"
        subprocess.run([
            compiler, "-nostdlib", "-no-pie", "-Wl,--build-id=none",
            "-Wl,--dynamic-linker=/system/bdplib/ld-linux.so.3", "-Wl,-z,relro,-z,now",
            compiler_file("crt1.o"), compiler_file("crti.o"), compiler_file("crtbegin.o"), str(obj),
            "-L" + str(vendor), "-Wl,-rpath-link=" + str(vendor),
            "-Wl,-rpath-link=" + str(firmware / "usr/lib"),
            "-l:libdl.so.2", "-l:libc.so.6", compiler_file("libgcc.a"),
            compiler_file("crtend.o"), compiler_file("crtn.o"), "-o", str(executable),
        ], check=True)
        # Preserve any previous output if compile/link fails.
        fd, temporary = tempfile.mkstemp(prefix=".eq-parser-", dir=output.parent)
        try:
            with os.fdopen(fd, "wb") as stream:
                stream.write(executable.read_bytes())
            Path(temporary).chmod(0o700)
            os.replace(temporary, output)
        finally:
            Path(temporary).unlink(missing_ok=True)
    return output


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cc", required=True, help="Path to arm-none-linux-gnueabihf-gcc")
    parser.add_argument("--rootfs", type=Path, required=True, help="Owner's extracted supported firmware rootfs")
    parser.add_argument("--output", type=Path, required=True, help="Output helper executable")
    args = parser.parse_args()
    try:
        print(build(args.cc, args.rootfs, args.output))
    except (ValueError, OSError, subprocess.CalledProcessError) as exc:
        parser.exit(1, f"EQ parser helper build failed: {exc}\n")
