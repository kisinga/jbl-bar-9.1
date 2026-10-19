# Development

Keep the release small: readable shell scripts and two USB ZIPs. Preserve the working behavior and firmware identity guards. Do not add support for a build by merely changing its hash: its fields, event ABI and restoration behavior need validation.

## Build and test

On Linux, install Python 3.10+ and BusyBox (`/usr/bin/busybox`). On Debian/Ubuntu the packages are `python3` and `busybox`. These are developer requirements; users only need the ZIP.

```sh
python3 tools/build_release.py
python3 tools/test.py
```

Archives and `SHA256SUMS` appear in `dist/`. Rebuilding the same inputs produces identical ZIP bytes. The build verifies payload hashes, embedded expected hashes, exact tested device files and shell syntax. It refuses stale or modified device code against the pinned baseline. No firmware image or external Python package is needed.

The tests execute adapted temporary copies with fake process files and CLI behavior. They never connect to a soundbar or modify the real USB. The synthetic identity replacement is confined to those copies. Results go in ignored `build/test-results/`.

Optional descriptor validation against a legally obtained extracted ARM BusyBox uses `JBL_ARM_BUSYBOX=/absolute/path/to/busybox python3 tests/check_descriptors.py`, with `qemu-arm` and registered ARM binfmt available for direct applet execution. That binary is not bundled.

## Layout

- `usb/`: exact installable device source, with CRLF-preserved launcher/hook.
- `README.md`: installation, removal, compatibility, troubleshooting, evidence and credits.
- `COMMUNITY.md`: discussion links and a first-person draft.
- `USB-README.txt` / `ROLLBACK.txt`: short guides included in the ZIPs.
- `tools/`: release builder and test runner.
- `tests/`: portable fixture suite and pinned revision 5 file hashes.
- `research/`: public index plus ignored local-only artifacts; see its README.

`main` and tag `v0.1.0` preserve the revision 5 I first confirmed on my bar. Experimental latency work belongs on a separate branch and must not replace that tested download until hardware validation. A new payload requires updated checksums, installer expectations, ownership/upgrade handling, tests and a fresh hardware result. The baseline verifier must not be casually bypassed to publish an untested change.

Report results with the issue template. Keep proprietary images, full device logs, credentials, personal paths and tool caches out of commits and releases.
