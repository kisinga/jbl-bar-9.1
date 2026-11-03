"""Profile changes must not silently expand existing firmware eligibility."""
import copy
import json
from pathlib import Path
import sys
import tempfile
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))
import check_profiles
import repo_layout

cases = []
check_profiles.check()
cases.append("generated_runtime_and_existing_shell_guards_agree")

profile = repo_layout.bar91_profile()
changed = copy.deepcopy(profile)
changed["executables"]["bdpprog"] = "0" * 64
try:
    check_profiles.check_shell(changed)
except ValueError:
    pass
else:
    raise AssertionError("A profile edit bypassed pinned shell identities")
cases.append("profile_only_firmware_change_rejected")

with tempfile.TemporaryDirectory(prefix="jbl-profile-") as temporary:
    target = Path(temporary)
    (target / "profile.json").write_text(json.dumps({**profile, "model": "bar1300"}))
    with patch.object(repo_layout, "BAR91", target):
        try:
            repo_layout.bar91_profile()
        except ValueError:
            pass
        else:
            raise AssertionError("Unqualified model accepted as Bar 9.1")
    cases.append("another_model_cannot_reuse_supported_profile")
    changed = copy.deepcopy(profile)
    changed["native_helpers"]["bluetooth"]["/system/bdplib/libc.so.6"] = "not-a-sha256"
    (target / "profile.json").write_text(json.dumps(changed))
    with patch.object(repo_layout, "BAR91", target):
        try:
            repo_layout.bar91_profile()
        except ValueError:
            pass
        else:
            raise AssertionError("Malformed helper identity accepted")
    cases.append("malformed_library_identity_rejected")

out = ROOT / "build/test-results"
out.mkdir(parents=True, exist_ok=True)
(out / "profile-tests.json").write_text(json.dumps({
    "scope": "Host profile consistency and refusal checks; no device validation.",
    "cases": cases,
}, indent=2) + "\n")
print("Profile checks:", len(cases))
