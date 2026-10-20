"""Owner-specific archives and isolated SSH startup/hook regression fixtures."""
from pathlib import Path
import hashlib
import importlib.util
import json
import os
import re
import shutil
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("build_ssh", ROOT / "tools/build_ssh.py")
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)
BB = shutil.which("busybox")
assert BB, "Install BusyBox for shell fixtures"
cases = []


def passed(name):
    cases.append({"case": name})
    print("PASS", name, flush=True)


def run(argv, *, env=None):
    return subprocess.run(argv, env=env, capture_output=True, text=True, timeout=25)


def crypt_test():
    # Published-format interoperability checked against the host's openssl,
    # using disposable values passed through stdin rather than process argv.
    openssl = shutil.which("openssl")
    if openssl:
        for password in ("disposable-test-password", "unicode fixture 🦄"):
            check = subprocess.run(
                [openssl, "passwd", "-6", "-salt", "fixtureSalt", "-stdin"],
                input=password + "\n", text=True, capture_output=True, check=True,
            )
            assert builder.password_hash(password, "fixtureSalt") == check.stdout.strip()
    # Deterministic independently verified vector also covers openssl-free CI.
    assert builder.password_hash("fixture-password", "fixtureSalt") == (
        "$6$fixtureSalt$sQSjITREzEfMEXA9wpMkKTNkMHs1yQEs0cX6Kt8PuwtC4tmRgNSPG05og2XZJV3PDk4TqFPAkmRnhWHzo7dEU."
    )
    assert builder.password_hash("fixture-password") != builder.password_hash("fixture-password")
    passed("sha512crypt_interoperability_and_fresh_salts")


def archive_tests():
    with tempfile.TemporaryDirectory(prefix="jbl-public-ssh-build-") as tmp:
        folder = Path(tmp)
        password = "disposable-owner-password"
        paths = builder.build(folder / "one", "10.23.4.7", password)
        with zipfile.ZipFile(paths[0]) as archive:
            files = {name: archive.read(name) for name in archive.namelist()}
        assert files["AutoScript/AutoScript"].endswith(b"\r\n")
        assert files["JBLLocalSSH/settings.conf"] == b"ALLOWED_PC=10.23.4.7\nINTERFACE=wlan0\nAUTOSTART=1\n"
        assert not any("wifi.psk" in name or "wifi.ssid" in name for name in files)
        assert all(password.encode() not in data for data in files.values())
        assert all("host_ed25519" not in name for name in files)
        for line in files["JBLLocalSSH/checksums.txt"].decode().splitlines():
            expected, name = line.split("  ", 1)
            assert hashlib.sha256(files["JBLLocalSSH/" + name]).hexdigest() == expected
        assert all(path.stat().st_mode & 0o077 == 0 for path in paths)
        with zipfile.ZipFile(paths[1]) as archive:
            rollback = b"".join(archive.read(name) for name in archive.namelist())
            assert b"password.hash" not in rollback and b"wifi.psk" not in rollback
        passed("private_archive_integrity_no_plaintext_or_host_keys")
        ssid, passphrase = "fixture Wi-Fi", "disposable-wifi-passphrase"
        other = builder.build(folder / "two", "172.20.8.9", password, interface="ra0", autostart=False,
                              wifi=(ssid, passphrase))
        with zipfile.ZipFile(other[0]) as archive:
            settings = archive.read("JBLLocalSSH/settings.conf")
            assert b"172.20.8.9" in settings and b"INTERFACE=ra0" in settings and b"AUTOSTART=0" in settings
            encoded = archive.read("JBLLocalSSH/wifi.psk.hex").strip()
            expected = hashlib.pbkdf2_hmac("sha1", passphrase.encode(), ssid.encode(), 4096, 32).hex().encode()
            assert encoded == expected
            assert all(passphrase.encode() not in archive.read(n) for n in archive.namelist())
        passed("per_owner_lan_optional_wpa2_derivation_usb_only")
        for pc in ("8.8.8.8", "127.0.0.1", "0.0.0.0", "::1", "10.0.0.999", "10.0.0.1;echo"):
            try:
                builder.allowed_ipv4(pc)
            except ValueError:
                continue
            raise AssertionError(pc)
        try:
            builder.build(folder / "bad", "10.23.4.7", password, interface="wlan0;echo")
        except ValueError:
            pass
        else:
            raise AssertionError("interface injection accepted")
        passed("reject_public_or_malformed_clients_and_interface_injection")
        try:
            builder.build(folder / "one", "10.23.4.7", password)
        except ValueError:
            pass
        else:
            raise AssertionError("private archive silently overwritten")
        passed("never_overwrite_existing_private_archives")


def hook_tests():
    for case in ("coexist", "missing", "changed", "symlink", "unowned", "temporary_symlink", "legacy_upgrade"):
        with tempfile.TemporaryDirectory(prefix="jbl-ssh-hook-") as tmp:
            folder = Path(tmp)
            store, hook = folder / "store", folder / "AutoScript.TSS"
            store.mkdir()
            (store / "OWNER.txt").write_text("mine\n" if case == "unowned" else "JBL_LOCAL_SSH_REV1\n")
            original = b"CLI(CLI_exec sh /var/JBLStandard/run.sh &)\r\n# preserved without final newline"
            if case != "missing":
                hook.write_bytes(original)
                hook.chmod(0o640)
            if case == "symlink":
                hook.unlink()
                hook.symlink_to(folder / "other")
            if case == "temporary_symlink":
                (folder / "sentinel").write_text("unchanged")
                (folder / "AutoScript.TSS.ssh-new").symlink_to(folder / "sentinel")
            if case == "legacy_upgrade":
                (store / "hook.before-ssh").write_bytes(original)
                (store / "hook.with-ssh").write_bytes(original + b"\r\nCLI(CLI_exec sh /var/JBLLocalSSH/boot.sh &)\r\n")
                hook.write_bytes((store / "hook.with-ssh").read_bytes())
            env = dict(os.environ, JBL_BUSYBOX=BB, JBL_SSH_STORE=str(store), JBL_SSH_HOOK=str(hook))
            command = ["/bin/sh", str(ROOT / "ssh/hooks.sh")]
            installed = run(command + ["install"], env=env)
            if case in ("symlink", "unowned", "temporary_symlink"):
                assert installed.returncode != 0
                if case == "temporary_symlink":
                    assert (folder / "sentinel").read_text() == "unchanged"
                if case == "symlink":
                    assert hook.is_symlink()
                passed("hook_refuse_" + case)
                continue
            assert installed.returncode == 0, installed.stderr
            expected = (b"" if case == "missing" else original) + b"\r\nCLI(CLI_exec sh /var/JBLLocalSSH/boot.sh &)\r\n"
            assert hook.read_bytes() == expected
            assert run(command + ["install"], env=env).returncode == 0
            assert hook.read_bytes() == expected
            if case == "changed":
                hook.write_bytes(expected + b"# later user customization\r\n")
                changed = hook.read_bytes()
                assert run(command + ["rollback"], env=env).returncode != 0
                assert hook.read_bytes() == changed and (store / "BOOT_DISABLED").is_file()
                assert run(command + ["install"], env=env).returncode != 0
            else:
                assert run(command + ["rollback"], env=env).returncode == 0
                if case == "missing":
                    assert not hook.exists()
                else:
                    assert hook.read_bytes() == original
                    if case == "coexist":
                        assert hook.stat().st_mode & 0o777 == 0o640
                assert (store / "BOOT_DISABLED").is_file()
                assert run(command + ["rollback"], env=env).returncode == 0
            passed("hook_" + case)


def address_tests():
    with tempfile.TemporaryDirectory(prefix="jbl-ssh-lan-") as tmp:
        folder = Path(tmp)
        wrapper = folder / "busybox"
        wrapper.write_text("#!/bin/sh\nif [ \"$1\" = ip ]; then printf '%s\\n' \"$JBL_FIXTURE_IPS\"; else exec " + BB + " \"$@\"; fi\n")
        wrapper.chmod(0o700)
        tests = (
            ("10.23.4.7", "10.23.5.20/16", "10.23.5.20"),
            ("10.23.4.7", "10.23.5.20/24", ""),
            ("172.20.8.9", "172.20.8.11/24", "172.20.8.11"),
            ("192.168.44.7", "192.168.44.21/24", "192.168.44.21"),
            ("10.23.4.7", "10.23.4.7/24", ""),
            ("10.23.4.7", "8.8.8.8/8", ""),
        )
        for pc, cidr, expected in tests:
            env = dict(os.environ, JBL_BUSYBOX=str(wrapper), ALLOWED_PC=pc, INTERFACE="wlan0",
                       JBL_FIXTURE_IPS="7: wlan0 inet " + cidr + " scope global wlan0")
            result = run(["/bin/sh", "-c", '. "$1"; find_address', "fixture", str(ROOT / "ssh/common.sh")], env=env)
            assert result.returncode == 0 and result.stdout.strip() == expected, (pc, cidr, result.stdout, result.stderr)
        passed("actual_subnet_address_selection_excludes_public_other_lan_and_pc")


def installer_tests():
    # Actual control flow and hooks, adapted to isolated temp paths. Only root
    # metadata, native-tool checks and live-daemon detection are fixture stubs.
    for case in ("live_upgrade", "live_other_pc", "bad_integrity", "wrong_native_build", "boot_wait_bounded"):
        with tempfile.TemporaryDirectory(prefix="jbl-ssh-install-") as tmp:
            folder = Path(tmp)
            usb, store, ram = folder / "usb", folder / "store", folder / "ram"
            for directory in (usb, store, ram):
                directory.mkdir()
            hook = folder / "AutoScript.TSS"
            hook.write_bytes(b"CLI(CLI_exec sh /var/JBLStandard/run.sh &)\r\n")
            (store / "OWNER.txt").write_text("JBL_LOCAL_SSH_REV1\n")
            (store / "sshd_config").write_text("AllowUsers root@" + ("10.23.4.8" if case == "live_other_pc" else "10.23.4.7") + "\n")
            (store / "password.hash").write_text("preserve-existing-private-hash\n")
            wrapper = folder / "busybox"
            wrapper.write_text("#!/bin/sh\ncase \"$1\" in id) echo 0;; stat) if [ \"$3\" = %u ]; then echo 0; else exec "+ BB + " \"$@\"; fi;; sleep) exit 0;; *) exec " + BB + " \"$@\";; esac\n")
            wrapper.chmod(0o700)
            replacements = {"/mnt/sda1/JBLLocalSSH": str(usb), "/var/JBLLocalSSH": str(store),
                            "/var/AutoScript.TSS": str(hook), "/tmp/jbl-local-ssh": str(ram / "lock")}

            def adapt(source):
                for index, old in enumerate(replacements):
                    source = source.replace(old, "@@SSH_PATH_" + str(index) + "@@")
                for index, new in enumerate(replacements.values()):
                    source = source.replace("@@SSH_PATH_" + str(index) + "@@", new)
                return source

            for path in (ROOT / "ssh").glob("*.sh"):
                (usb / path.name).write_text(adapt(path.read_text()))
            common = (usb / "common.sh").read_text()
            common += "\nverify_tools() { return " + ("1" if case == "wrong_native_build" else "0") + "; }\n"
            common += "daemon_running() { return " + ("1" if case == "boot_wait_bounded" else "0") + "; }\n"
            if case == "boot_wait_bounded":
                common += "find_address() { return 0; }\n"
            (usb / "common.sh").write_text(common)
            (usb / "settings.conf").write_text("ALLOWED_PC=10.23.4.7\nINTERFACE=wlan0\nAUTOSTART=1\n")
            payload = {p.name: p.read_bytes() for p in usb.iterdir()}
            (usb / "checksums.txt").write_bytes(builder.manifest(payload))
            if case == "bad_integrity":
                (usb / "session.sh").write_text("changed\n")
            env = dict(os.environ, JBL_BUSYBOX=str(wrapper), JBL_SSH_STORE=str(store), JBL_SSH_HOOK=str(hook))
            if case == "boot_wait_bounded":
                for path in usb.iterdir():
                    shutil.copyfile(path, store / path.name)
                for name in ("passwd", "nsswitch.conf", "ld.so.preload", "host_ed25519", "host_ed25519.pub"):
                    (store / name).write_text("disposable-fixture\n")
                result = run(["/bin/sh", str(store / "boot.sh")], env=env)
                status = (store / "BOOT_STATUS.txt").read_text().strip()
                assert status == "NO_ALLOWED_LAN_WITHIN_180_SECONDS", (status, result.stderr)
            else:
                result = run(["/bin/sh", str(usb / "start.sh")], env=env)
                status = (usb / "STATUS.txt").read_text().strip()
                expected = {"live_upgrade": "ALREADY_RUNNING_PASSWORD_UNCHANGED", "live_other_pc": "RUNNING_ALLOWED_PC_DIFFERS",
                            "bad_integrity": "USB_INTEGRITY_FAILED", "wrong_native_build": "UNSUPPORTED_NATIVE_SSH_BUILD"}[case]
                assert status == expected, (status, result.stderr, (usb / "setup.log").read_text())
                assert (store / "password.hash").read_text() == "preserve-existing-private-hash\n"
                if case == "live_upgrade":
                    assert (store / "boot.sh").exists() and b"/boot.sh &)" in hook.read_bytes()
                else:
                    assert b"/JBLLocalSSH/boot.sh" not in hook.read_bytes()
            passed("installer_" + case)


crypt_test()
archive_tests()
hook_tests()
address_tests()
installer_tests()
for script in (ROOT / "ssh").glob("*.sh"):
    result = run([BB, "sh", "-n", str(script)])
    assert result.returncode == 0, (script.name, result.stderr)
source = b"\n".join(p.read_bytes() for p in (ROOT / "ssh").glob("*.sh"))
assert not re.search(rb"\$6\$[./0-9A-Za-z]{1,16}\$[./0-9A-Za-z]{86}", source)
assert b"BEGIN OPENSSH PRIVATE KEY" not in source and b"\x00" not in source
passed("shell_syntax_and_no_embedded_password_hashes_keys_or_binaries")
out = ROOT / "build/test-results"
out.mkdir(parents=True, exist_ok=True)
(out / "ssh-tests.json").write_text(json.dumps({
    "scope": "Host fixtures: per-owner builder, integrity, CIDR selection, actual hook transaction/rollback, active installation upgrade and bounded boot failure. Native binaries and physical reboot not exercised.",
    "cases": cases,
}, indent=2) + "\n")
