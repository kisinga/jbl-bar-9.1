"""Reviewed executable package targets. Descriptive catalog entries grant no code."""
import re

from repo_layout import ROOT, bar91_profile

INSTALLERS = {"bar91": bar91_profile}
USB_ENTRY = b"CLI(CLI_exec sh /mnt/sda1/JBLToolkit/dispatch.sh &)\r\n"
LEGACY_USB_ENTRY = b"CLI(CLI_exec sh /mnt/sda1/JBLToolkit/launch.sh &)\r\n"


def profile(model):
    if model not in INSTALLERS:
        raise ValueError(f"{model}: installation is not qualified; run ./jbl models")
    return INSTALLERS[model]()


def resolve(model):
    if model != "auto":
        profile(model)
        return model
    # A second bootstrap/payload implementation needs explicit bundle layout
    # review. Never silently choose the first target or mix incompatible stores.
    if len(INSTALLERS) != 1:
        raise ValueError("Automatic bundle composition needs review for this installer set")
    return next(iter(INSTALLERS))


def dispatcher(model):
    item = profile(model)
    replacements = {"MODEL": model, "PROFILE": item["id"],
                    "BDP_HASH": item["executables"]["bdpprog"],
                    "CLI_HASH": item["executables"]["cli"]}
    text = (ROOT / "packaging/usb/dispatch.sh.in").read_text()
    for key, value in replacements.items():
        if not re.fullmatch(r"[a-zA-Z0-9_.-]+", value):
            raise ValueError("Unsafe reviewed profile token")
        text = text.replace("@" + key + "@", value)
    if re.search(r"@[A-Z_]+@", text):
        raise ValueError("Incomplete USB dispatcher template")
    return text.encode()


def matrix():
    return {"include": [{"model": "auto", "artifact": "jbl-bar-toolkit-usb-development"},
                        *[{"model": model, "artifact": f"jbl-{model}-usb-development"}
                          for model in sorted(INSTALLERS)]]}
