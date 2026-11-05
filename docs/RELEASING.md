# Releases

Merging and publishing are separate choices. Every PR and merge to `main` runs
checks and builds downloadable USB/recovery packages. Publishing happens only
when a maintainer requests it. There is no timer, release bot or extra secret.

## Publish when ready

1. Edit the root `VERSION` file and add a matching `## <version>`
   section to [CHANGELOG.md](../CHANGELOG.md), with plain-language bullet points.
   Use a new version for each release, for example `0.3.0-dev.10`.
2. Merge the changes to `main` after CI passes.
3. Open [Test and package toolkit](https://github.com/kisinga/jbl-bar-toolkit/actions/workflows/package.yml)
   → **Run workflow** → choose **main** → check **Publish this version** → **Run workflow**.
4. The pipeline validates release intent, runs host, Go race/vet and browser
   checks, builds all reviewed targets, verifies assets and publishes a
   **prerelease** at [Releases](https://github.com/kisinga/jbl-bar-toolkit/releases).
   Its release notes combine that version's changelog with setup/recovery guidance.

The manual run is the approval to release. Leaving the box unchecked runs tests
and produces CI downloads only. Publication from another branch is refused
before tests start. A pushed `v<VERSION>` tag retains the existing **draft
prerelease** path; the tag must match the version and belong to `main` history.
Tags created by the manual pipeline need no second workflow run.

All automated publications remain prereleases while combined installation,
USB-free boot, audio/wake and recovery qualification is pending. CI checks do
not establish hardware qualification. After qualification, a maintainer can
review and promote an appropriate release in GitHub.

## What users download

- `jbl-setup-OS-ARCH` (`.exe` on Windows): one portable executable for Linux,
  macOS or Windows, Intel/AMD or ARM64. Downloads verified toolkit/recovery files
  and supports explicit self-update. No installer or runtime is required.
- `jbl-bar-toolkit-v<VERSION>-setup-OS-ARCH.zip`: offline bundle containing that
  executable plus `toolkit-usb.zip` and `toolkit-recovery.zip`.
- `jbl-bar-toolkit-v<VERSION>-usb.zip` and `…-recovery.zip`: raw installation and
  independent recovery packages for advanced/manual use. `SETUP.html` directs
  users to the portable app; it no longer generates configuration downloads.
- `SHA256SUMS` and `release.json`: all public asset checksums and package details,
  including source commit, version and supported hardware.

The setup app selects the newest published release containing automatic toolkit
assets, including development prereleases, then checks the manifest, metadata,
package contents and recovery match. Self-update accepts only a higher semantic
version and the matching OS/architecture binary. Downloads use this repository's
public GitHub endpoints; checksums verify integrity, not an independent signature.
Self-update and first-run downloads require publishing these assets; CI artifacts
alone are not an update source. macOS/Windows binaries are unsigned.

Model-specific packages have distinct filenames, metadata and checksum files.
The default automatic package is the normal user download. Bar 9.1 is currently
the only included implementation. Public archives contain no owner credentials
or vendor firmware; locally configured drives are private.

CI artifacts expire after 30 days and require a GitHub sign-in. GitHub wraps
those files in another ZIP; extract that wrapper first. Published release assets
are direct downloads. Release notes explain which ZIP to use.

## Failures and retries

Missing/empty changelog entries, tag/version mismatches, and already published
versions fail before expensive tests on a manual publication. Build/test errors
stop publication. Missing target packages, corrupt checksums, extra files or
source/version mismatches stop the release before uploading.

A release stays a draft until its assets are uploaded successfully. Retry a
failed run from the **same commit** to repair an unfinished draft. Published
releases are never overwritten; make a new version and changelog entry instead.
Publishing is serialized per ref, and tags pointing at another commit are
refused. Repository Actions must allow `GITHUB_TOKEN` to write contents for the
release job; all other jobs use read-only permissions.

## Build locally

```sh
go run ./apps/build
go run ./apps/build --model bar91
python3 tools/release.py check
go run ./apps/build --matrix
```

The shared Go builder in `internal/buildtool/` creates USB/recovery ZIPs,
cross-compiles six binaries without CGO, and produces their offline bundles.
CI and local builds invoke `go run ./apps/build`; obsolete Python packaging
entry points have been removed. Independent legacy payload fixtures remain.
Dependency licenses are embedded (`--licenses` or the interface footer); after dependency changes,
regenerate them with `go run ./apps/build --write-licenses`. Linux raw
formatting uses go-diskfs; keep its version pinned unless the disk-image/fsck
regression passes. Windows and macOS use native formatters. Windows limits its
FAT32 partition to 30 GiB, leaving any remaining space unallocated. OS privilege
prompts and physical USB installation still need qualification on each platform.

Default packages include both project-owned ARM helpers from
`models/bar91/remote/native/bin/`. Go verifies their source, builder, dependency
and binary hashes before embedding them; vendor libraries are never bundled.
After native source changes, rebuild and review with
`python3 tools/check_native.py --write --cc <ARM-GCC> --rootfs <private-rootfs>`.
The existing native build instructions describe those maintainer-only inputs.
`python3 tools/check_native.py` checks the recorded inventory without compiling.

`internal/buildtool/` defines public package targets and guards;
`tools/hardware.py` retains the independent legacy implementation used by fixtures.
`models/catalog.json` only describes support and research. Adding a model requires a reviewed profile,
bootstrap/detector, payload builder, recovery and tests. Auto mode refuses to
silently choose among multiple implementations without bundle composition review.
Historical v0.1.0 standalone bytes, filenames and recovery remain unchanged.
