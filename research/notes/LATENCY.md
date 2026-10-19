# Faster wake candidate — revision 6

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

## Build and owner test

On this branch run `python3 tools/build_release.py` and `python3 tools/test.py`. The generated candidate archives are:

- `dist/jbl-bar9-standard-mode-v0.2.0-dev.1-install.zip`
- `dist/jbl-bar9-standard-mode-v0.2.0-dev.1-uninstall.zip`

These local archives are deliberately separate from the public confirmed release. Use the same install/test procedure, recording actual seconds after power-on. Confirm normal LOAD, delayed power-on, Smart-to-Standard readback, key release, console restoration and repeated standby/wake. Test standby of at least 20 seconds first. Return the USB logs. No hardware result for revision 6 exists yet.

The pinned revision 6 manifest is an integrity record for this candidate, not a claim of hardware qualification. `tests/fixtures/revision5-sha256.json` retains the confirmed baseline for comparison.
