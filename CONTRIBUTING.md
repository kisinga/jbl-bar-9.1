# Development

On `work/faster-wake`, the builder pins an **untested revision 6 candidate** and emits `v0.2.0-dev.1` archives. The original revision 5 baseline remains in the fixtures and on `main`/`v0.1.0`. See [candidate notes below](#faster-wake-candidate--revision-6); descriptions of the release I tested below refer to revision 5.

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

## Faster wake candidate — revision 6

**Working branch `work/faster-wake`; hardware untested.** The public v0.1.0 download remains the confirmed revision 5. Do not describe the faster candidate as device-validated.

The confirmed trace took 34.55 seconds from first awake observation to `STANDARD_THIS_WAKE`. Around ten seconds were spent accumulating awake samples at five-second intervals; `once.sh` then included a fixed ten-second delay and repeated readiness checks before invoking the controller.

| Setting | Confirmed revision 5 | Candidate revision 6 |
| --- | --- | --- |
| Watcher poll interval | 5 seconds | 1 second |
| Required identical awake samples | 3 | 3 |
| Bounded runner's initial settling delay | 10 seconds | 1 second |
| Runner readiness sample interval/count | 2 seconds / 3 samples | Unchanged |
| Mute hold, key-release/completion checks | Confirmed sequence | Byte-for-byte unchanged controller/helper |
| Standby deadline | None | None |

The candidate removes approximately 17 seconds of scheduled waiting, suggesting **about 18 seconds** from first awake detection on a similar run. That is an estimate, not a measurement. Initial detection also polls sooner. Read-only state sampling now occurs more frequently; hardware CPU/load and wake behavior must be observed. The change does not shorten mute duration, skip mode checks or weaken restoration.

Revision 6 has distinct ownership and checksums. Its dispatcher only upgrades the exact revision 5 package, backing it up first. It retains refusal for unknown payloads, conflicting hooks or unsupported executables. Use its **matching uninstall ZIP**; the original v0.1.0 uninstaller does not recognize revision 6 ownership.

### Build and hardware test

On this branch run `python3 tools/build_release.py` and `python3 tools/test.py`. The generated candidate archives are:

- `dist/jbl-bar9-standard-mode-v0.2.0-dev.1-install.zip`
- `dist/jbl-bar9-standard-mode-v0.2.0-dev.1-uninstall.zip`

These local archives are deliberately separate from the public confirmed release. Use the same install/test procedure, recording actual seconds after power-on. Confirm normal LOAD, delayed power-on, Smart-to-Standard readback, key release, console restoration and repeated standby/wake. Test standby of at least 20 seconds first. Return the USB logs. No hardware result for revision 6 exists yet.

The pinned revision 6 manifest is an integrity record for this candidate, not a claim of hardware qualification. `tests/fixtures/revision5-sha256.json` retains the confirmed baseline for comparison.
