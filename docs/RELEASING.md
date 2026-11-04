# Releases

Merging and publishing are separate choices. Every PR and merge to `main` runs
checks and builds downloadable USB/recovery packages. Publishing happens only
when a maintainer requests it. There is no timer, release bot or extra secret.

## Publish when ready

1. Edit `VERSION` in `tools/build_usb.py` and add a matching `## <version>`
   section to [CHANGELOG.md](../CHANGELOG.md), with plain-language bullet points.
   Use a new version for each release, for example `0.3.0-dev.9`.
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

- `jbl-bar-toolkit-v<VERSION>-usb.zip`: the normal installation package; extract
  to a clean FAT32 USB and open `SETUP.html`.
- `jbl-bar-toolkit-v<VERSION>-recovery.zip`: independent emergency recovery;
  keep this with the original installation package.
- `SHA256SUMS` and `release.json`: checksums and package details, including the
  source commit, version and supported hardware.

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
./jbl build
./jbl build --model bar91
python3 tools/release.py check
python3 tools/build_distribution.py --matrix
```

`tools/hardware.py` defines reviewed executable targets; `models/catalog.json`
only describes support and research. Adding a model requires a reviewed profile,
bootstrap/detector, payload builder, recovery and tests. Auto mode refuses to
silently choose among multiple implementations without bundle composition review.
Historical v0.1.0 standalone bytes, filenames and recovery remain unchanged.
