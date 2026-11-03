#!/usr/bin/env python3
"""Replay the exact OTA1 product-selection script in an offline fake device.

Executes vendor shell source under host bash, with synthetic product archives,
fake /proc files and isolated /data. Does not boot firmware or emulate audio.
Requires bwrap. The pinned SHA-256 prevents executing a different script.
"""
import argparse
import hashlib
import io
import json
from pathlib import Path
import subprocess
import tarfile

SCRIPT_SHA256 = "911877ad250b46c86af43a725f2379492268c1083413aabeb06a286337d2cd3c"
CASES = [
    ("bar1000", "0xD001", False, True, "jblbar_1000"),
    ("bar1300", "0xE001", False, True, "jblbar_1300"),
    ("bar1300-lowercase", "0xe001", False, True, "jblbar_1300"),
    ("bar800", "0xC001", False, True, "jblbar_800"),
    ("unknown-id", "0xFFFF", False, True, "jblbar_500"),
    ("cached-bar1300", "0xE001", True, True, "cached"),
    ("separate-image", "0xE001", False, False, None),
]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("rootfs", type=Path)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    script = (args.rootfs / "sbin/load_product_config_4_oneimage.sh").resolve()
    actual = hashlib.sha256(script.read_bytes()).hexdigest()
    if actual != SCRIPT_SHA256:
        raise ValueError("Unqualified product-loader script identity")
    out = args.output.resolve()
    out.mkdir(parents=True, exist_ok=False)
    packages = out / "synthetic-packages"
    packages.mkdir()
    for model in ("300", "500", "700", "800", "1000", "1300"):
        marker = ("jblbar_" + model).encode()
        with tarfile.open(packages / f"jblbar_{model}_conf.tar.gz", "w:gz") as archive:
            member = tarfile.TarInfo("selected-model")
            member.size = len(marker)
            archive.addfile(member, io.BytesIO(marker))
    fake_bin = out / "bin"
    fake_bin.mkdir()
    (fake_bin / "bash").symlink_to("/usr/bin/bash")
    (fake_bin / "sh").symlink_to("/usr/bin/bash")
    (fake_bin / "awk").symlink_to(str(Path("/usr/bin/awk").resolve()))
    stub = fake_bin / "logcat"
    stub.write_text("#!/bin/bash\nexit 0\n")
    stub.chmod(0o755)
    results = []
    for label, product_id, cached, grouped, expected in CASES:
        case = out / label
        proc, data = case / "proc", case / "data"
        vm = proc / "sys/vm"
        vm.mkdir(parents=True)
        for name in ("extfrag_threshold", "min_free_kbytes", "vfs_cache_pressure",
                     "watermark_scale_factor", "dirtytime_expire_seconds",
                     "oom_kill_allocating_task"):
            (vm / name).touch()
        cmdline = f"hw_id=0x0003 production_id={product_id}"
        if grouped:
            cmdline += " GROUP_ID=group_jblbar"
        (proc / "cmdline").write_text(cmdline + "\n")
        (proc / "meminfo").write_text("MemTotal: 512000 kB\n")
        mirror = data / "product_group/sys_mirror"
        mirror.mkdir(parents=True)
        (data / "product_group/dts/res").mkdir(parents=True)
        if cached:
            (data / "product_group/hw_flag_detect_done").touch()
            (mirror / "selected-model").write_text("cached")
        command = ["bwrap", "--unshare-all", "--die-with-parent"]
        for path in ("/usr", "/lib", "/lib64"):
            if Path(path).exists():
                command += ["--ro-bind", path, path]
        command += ["--ro-bind", str(fake_bin), "/bin",
                    "--ro-bind", str(script), "/product-loader.sh",
                    "--ro-bind", str(packages), "/etc/product_group",
                    "--bind", str(proc), "/proc", "--bind", str(data), "/data",
                    "--dev", "/dev", "--tmpfs", "/tmp", "--clearenv",
                    "--setenv", "PATH", "/bin:/usr/bin", "--", "/bin/bash", "/product-loader.sh"]
        run = subprocess.run(command, capture_output=True, text=True, timeout=15)
        (case / "stdout.log").write_text(run.stdout)
        (case / "stderr.log").write_text(run.stderr)
        selected = (mirror / "selected-model").read_text() if (mirror / "selected-model").exists() else None
        diagnostics = [line for line in run.stderr.splitlines()
                       if not line.startswith("cp: cannot stat '/data/product_group/sys_mirror/avs/PRODUCT_CONF/")]
        row = {"case": label, "selected": selected, "expected": expected,
               "exit_code": run.returncode,
               "passed": selected == expected and run.returncode == 0 and not diagnostics,
               "unexpected_diagnostics": diagnostics,
               "cache_marker": (data / "product_group/hw_flag_detect_done").exists()}
        results.append(row)
    report = {"script_sha256": actual, "authority": "controlled-replay",
              "scope": "Vendor shell under host bash; synthetic packages and procfs; network isolated",
              "limitations": ["No vendor ELF execution, firmware boot, audio DSP or soundbar hardware.",
                              "Missing AVS fixture files produce expected cp diagnostics.",
                              "Does not establish startup reachability or behavior on current firmware."],
              "cases": results}
    (out / "results.json").write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(results, indent=2))
    if not all(row["passed"] for row in results):
        raise SystemExit(1)


if __name__ == "__main__":
    main()
