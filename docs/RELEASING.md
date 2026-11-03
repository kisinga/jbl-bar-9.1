# Release packaging

Owners normally download **jbl-bar-toolkit-v<VERSION>-usb.zip**, extract it to
FAT32 and open **SETUP.html**. It contains the precompiled app and automatically
selects its included hardware implementation. Keep its matching recovery ZIP.
Defaults are check-only, web enabled and SSH disabled. Public archives contain
no owner credentials or vendor firmware; locally configured drives are private.

```sh
./jbl build
./jbl build --model bar91
python3 tools/build_distribution.py --matrix
```

`tools/hardware.py` lists implemented installers; `models/catalog.json` only
describes research/support status. CI derives its matrix from the implementation
registry. Currently it builds the default automatic ZIP and the Bar 9.1 ZIP,
each with recovery, metadata and SHA-256 checksums. Model-specific metadata and
checksum filenames are distinct so downloads can be combined without collisions.

The workflow runs host fixtures, the pinned standalone baseline, Go race/vet
and browser checks before packaging. It uploads only explicit public assets.
A `v*` tag must match `tools/build_usb.py`'s VERSION and creates a **draft
prerelease**, verifying all downloaded asset checksums first. Full installation,
USB-free boot, audio/wake and recovery qualification is required before a stable
release. CI does not establish hardware qualification.

A new installer needs its own reviewed profile, bootstrap/detector, payload
builder, recovery and tests before joining the registry. The current flat
payload contract serves one model. Adding a second implementation must review
combined bundle composition explicitly; auto mode refuses to silently pick a
model or mix payloads. Per-model jobs are ready for separate implemented builders.
A research entry never causes a release job or an installation route to appear.

Historical v0.1.0 standalone bytes, filenames and recovery remain unchanged.
No publication happens merely by editing this repository.
