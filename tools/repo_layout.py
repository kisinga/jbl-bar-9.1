"""Canonical source locations and the reviewed Bar 9.1 build profile.

This is host-side build metadata. Device paths and payload layouts are defined
by their existing installers, not inferred from repository directory names.
"""
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
REMOTE = ROOT / "apps/remote"
BAR91 = ROOT / "models/bar91"
USB = BAR91 / "usb"
SSH = BAR91 / "ssh"
SMART_MODE = BAR91 / "smart-mode"
REMOTE_DEVICE = BAR91 / "remote"
FIXTURES = ROOT / "tests/bar91/fixtures"


def bar91_profile():
    """Load one supported profile; never accept an owner-supplied replacement."""
    profile = json.loads((BAR91 / "profile.json").read_text())
    if profile["schema_version"] != 1 or profile["model"] != "bar91":
        raise ValueError("Unsupported Bar 9.1 profile schema/model")
    groups = [profile["executables"], *profile["native_helpers"].values()]
    for group in groups:
        if not group or any(not re.fullmatch(r"[0-9a-f]{64}", value) for value in group.values()):
            raise ValueError("Invalid firmware SHA-256 profile")
    if set(profile["executables"]) != {"bdpprog", "cli"}:
        raise ValueError("Incomplete executable profile")
    return profile
