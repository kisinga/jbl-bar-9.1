# Changelog

User-facing changes go here. Before publishing another release, change `VERSION`
in `tools/build_usb.py` and add a matching `## <version>` section. Release notes
are generated from that section plus the installation and recovery instructions.

## 0.3.0-dev.8

- Install from one USB package with an offline setup page and automatic hardware identification. Bar 9.1 is the only included model.
- Use the browser remote for volume, sources, playback and sound settings without a cloud account.
- Choose bass, center, rear speaker and Atmos levels directly; the remote confirms each step with the bar.
- Save an experimental bass offset from -3 to +3 dB, in 0.5 dB steps, across all five bass levels. Supported Standard playback is required for application.
- Optionally remember the Smart/Standard mode chosen before standby and restore it after wake.
- Find audio delay, rear speaker calibration, subwoofer pairing under Advanced, plus a separate experimental Front EQ panel. Availability depends on the device and build.
- Download public USB and independent recovery packages with checksums and source-commit metadata. Maintainers choose when to publish a prerelease through GitHub Actions.

The combined public USB installation, restart and recovery flow still needs
hardware qualification. Keep an existing installation's original recovery files;
cross-version updates are not implemented.
