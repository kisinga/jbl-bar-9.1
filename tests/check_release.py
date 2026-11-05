"""Release completeness, provenance and publication failure regressions; no GitHub writes."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tools'))
import build_usb
import hardware
import release

cases = []
commit = 'a' * 40


def rejects(callback):
    try:
        callback()
    except (ValueError, OSError):
        return
    raise AssertionError('Unsafe release was accepted')


with tempfile.TemporaryDirectory() as temp:
    root = Path(temp)
    changelog = root / 'CHANGELOG.md'
    for text in ('# Empty', f'## {build_usb.VERSION}\nTODO',
                 f'## {build_usb.VERSION}\n- One\n## {build_usb.VERSION}\n- Two'):
        changelog.write_text(text)
        rejects(lambda: release.release_notes(changelog))
    changelog.write_text(f'## {build_usb.VERSION}\n- A useful change.\n\n## 0.0.1\n- Old change.\n')
    notes = release.release_notes(changelog)
    assert 'A useful change.' in notes and 'Old change.' not in notes and 'jbl-setup' in notes
cases.append('changelog_requires_unique_current_version_and_real_notes')

with tempfile.TemporaryDirectory() as temp:
    root = Path(temp)
    for target in hardware.matrix()['include']:
        model = target['model']
        prefix = '' if model == 'auto' else model + '-'
        stem = f'jbl-{"bar-toolkit" if model == "auto" else model}-v{build_usb.VERSION}'
        assets = [stem + suffix for suffix in ('-usb.zip', '-recovery.zip')]
        if model == 'auto': assets += release.setup_asset_names(build_usb.VERSION)
        for asset in assets:
            (root / asset).write_bytes(b'fixture archive')
        report = prefix + 'release.json'
        (root / report).write_text(json.dumps(dict(version=build_usb.VERSION, target=model,
                                                   source_commit=commit, assets=assets)))
        (root / (prefix + 'SHA256SUMS')).write_text(''.join(
            hashlib.sha256((root / name).read_bytes()).hexdigest() + '  ' + name + '\n'
            for name in [*assets, report]))
    assert len(release.verify_assets(root, commit)) == 8+len(release.setup_asset_names(build_usb.VERSION))
    rejects(lambda: release.verify_assets(root, 'b' * 40))
    cases.append('all_reviewed_targets_and_exact_source_commit_required')
    manifest = root / 'SHA256SUMS'
    original = manifest.read_bytes()
    for text in ('', original.decode().splitlines()[0] + '\n',
                 original.decode() + original.decode().splitlines()[0] + '\n',
                 '0' * 64 + '  ../secret\n'):
        manifest.write_text(text)
        rejects(lambda: release.verify_assets(root, commit))
    manifest.write_bytes(original)
    archive = next(root.glob('*-usb.zip'))
    archive.write_bytes(b'corrupt archive')
    rejects(lambda: release.verify_assets(root, commit))
    archive.write_bytes(b'fixture archive')
    (root / 'private-config.txt').write_text('private')
    rejects(lambda: release.verify_assets(root, commit))
    (root / 'private-config.txt').unlink()
    (root / 'bar91-release.json').unlink()
    rejects(lambda: release.verify_assets(root, commit))
    cases.append('missing_duplicate_traversal_corrupt_and_extra_assets_refused')

with patch.object(release, 'source_commit', return_value=commit), \
     patch.object(release, 'verify_assets', return_value=[]):
    with patch.object(release.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, 'b' * 40 + '\n')) as run:
        rejects(lambda: release.publish(Path('unused')))
        assert run.call_count == 1
    with patch.object(release.subprocess, 'run', side_effect=[
        subprocess.CompletedProcess([], 0, commit + '\n'),
        subprocess.CompletedProcess([], 0, '{"isDraft":false}')]) as run:
        rejects(lambda: release.publish(Path('unused')))
        assert run.call_count == 2
    with patch.object(release.subprocess, 'run', side_effect=[
        subprocess.CompletedProcess([], 1, ''),
        subprocess.CompletedProcess([], 0, json.dumps({'isDraft':True,'targetCommitish':'b' * 40}))]) as run:
        rejects(lambda: release.publish(Path('unused')))
        assert run.call_count == 2
    # Creation and repairs stay draft until all upload commands have succeeded.
    for draft in (False, True):
        commands = []
        def gh_run(command, **kwargs):
            commands.append(command)
            if command[:3] == ['git', 'rev-parse', '--verify']:
                return subprocess.CompletedProcess(command, 1, '')
            if command[:3] == ['gh', 'release', 'view']:
                return subprocess.CompletedProcess(command, 1, '')
            if '--notes-file' in command:
                assert 'jbl-setup' in Path(command[command.index('--notes-file') + 1]).read_text()
            return subprocess.CompletedProcess(command, 0, '')
        with patch.object(release.subprocess, 'run', side_effect=gh_run):
            release.publish(Path('unused'), draft=draft)
        assert '--draft' in commands[2] and commands[2][commands[2].index('--target') + 1] == commit
        assert '--draft=' + str(draft).lower() in commands[3]
    with patch.object(release.subprocess, 'run', side_effect=[
        subprocess.CompletedProcess([], 1, ''),
        subprocess.CompletedProcess([], 0, json.dumps({'isDraft':True,'targetCommitish':commit})),
        subprocess.CalledProcessError(1, ['gh', 'release', 'upload'])]) as run:
        try:
            release.publish(Path('unused'))
        except subprocess.CalledProcessError:
            pass
        else:
            raise AssertionError('Upload error did not stop publication')
        assert run.call_count == 3
cases.append('moved_tags_published_releases_and_foreign_drafts_never_modified')
cases.append('release_stays_draft_until_uploads_succeed_and_notes_are_attached')

out = ROOT / 'build/test-results'
out.mkdir(parents=True, exist_ok=True)
(out / 'release-tests.json').write_text(json.dumps({'scope': 'Release validation without network writes.', 'cases': cases}, indent=2) + '\n')
print('Release checks:', len(cases))
