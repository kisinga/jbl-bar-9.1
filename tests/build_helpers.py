"""Invoke the production Go builder from independent Python contract fixtures."""
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
CONFIG = b'''# Run the portable JBL setup executable to configure your USB.
format=1
action=check
ssh=false
web_remote=true
auto_standard=false
allowed_cidr=192.168.1.0/24
web_port=80
'''


def build(output, model='auto'):
    try:
        result = subprocess.run(['go', 'run', './apps/build', '--repo', str(ROOT),
                                 '--output-dir', str(output), '--model', model, '--json'],
                                cwd=ROOT, text=True, capture_output=True, check=True)
    except subprocess.CalledProcessError as error:
        raise ValueError(error.stderr.strip() or 'Go builder failed') from error
    return [Path(name) for name in json.loads(result.stdout)]
