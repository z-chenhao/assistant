#!/usr/bin/env python3
"""Drive author/reader and cold/warm evidence CLIs with a local gh Fake.

Reports actual Fake payload bytes, separately from API metadata sizes. It does
not claim live GitHub transport, billed traffic, or independent code review.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

from evidence import durable_root, save, sha256_file


def run(root, sha):
    repo = Path(__file__).resolve().parents[1]
    root = durable_root(root)
    root.mkdir(parents=True, exist_ok=False)
    with tempfile.TemporaryDirectory(prefix='erii-evidence-drive-') as directory:
        workspace = Path(directory)
        source = workspace / 'author-worktree'
        source.mkdir()
        save(source / 'result.json', dict(passed=True, source_sha=sha, assertions=['fixture-only']))
        record = source / 'command.json'
        save(record, dict(command=[sys.executable, '-c', 'print("fixture")'], returncode=0, source_sha=sha))
        (source / 'node_modules').mkdir()
        (source / 'node_modules/omitted').write_bytes(b'x' * 10000)
        (source / 'old.zip').write_bytes(b'old archive')
        env = os.environ.copy()
        logs = []
        def drive(*args, cwd=repo, success=True):
            command = [sys.executable, str(repo / 'scripts/evidence.py'), *map(str, args)]
            process = subprocess.run(command, cwd=cwd, env=env, capture_output=True, text=True)
            logs.append(dict(command=command, returncode=process.returncode))
            if (process.returncode == 0) != success:
                raise AssertionError(process.stderr)
            return json.loads(process.stdout) if success else None
        collected = drive('collect', '--root', root, '--source', source, '--issue', 'ERII-206',
                          '--repo', 'z-chenhao/erii', '--sha', sha, '--run', 'cli-harness', '--attempt', 1,
                          '--record', record, '--profile', 'offline-evidence-fake-v1')
        manifest = Path(collected['manifest'])
        repeated = drive('collect', '--root', root, '--source', source, '--issue', 'ERII-206',
                         '--repo', 'z-chenhao/erii', '--sha', sha, '--run', 'cli-harness', '--attempt', 1,
                         '--record', record, '--profile', 'offline-evidence-fake-v1')
        assert repeated == collected
        common = ['--manifest', manifest, '--manifest-sha256', collected['sha256']]
        receipt = root / 'handoff.json'
        planned = drive('receipt', *common, '--output', receipt)
        first = receipt.read_bytes()
        drive('receipt', *common, '--output', receipt)
        assert receipt.read_bytes() == first
        # A separate reader process, unrelated cwd, no author worktree or credentials.
        shutil.rmtree(source)
        reader = workspace / 'reviewer-checkout'
        reader.mkdir()
        verified = drive('verify', *common, cwd=reader)
        drive('verify', '--manifest', manifest, '--manifest-sha256', '0' * 64, success=False)
        exported = drive('export', *common, '--purpose', 'fixture consumer complete evidence')
        assert exported == drive('export', *common, '--purpose', 'fixture consumer complete evidence')
        with zipfile.ZipFile(manifest.parent / 'export/evidence.zip') as output:
            assert 'raw/result.json' in output.namelist() and 'raw/command.json' in output.namelist()
            assert not any('node_modules' in n or 'old.zip' in n for n in output.namelist())
        remote = workspace / 'remote.zip'
        with zipfile.ZipFile(remote, 'w') as output:
            output.writestr('suite-runtime.json', json.dumps(dict(passed=True, source_sha=sha)))
        payload = remote.read_bytes()
        item = dict(id=7, name=f'evidence-runtime-{sha}-1', size_in_bytes=len(payload), expired=False,
                    digest='sha256:' + hashlib.sha256(payload).hexdigest(),
                    workflow_run=dict(id=42, head_sha=sha))
        fake = workspace / 'fake'
        fake.mkdir()
        state = workspace / 'fake-state.json'
        save(state, dict(run=dict(id=42, run_attempt=1, head_sha=sha),
                        artifacts=[dict(artifacts=[item])]))
        ledger = workspace / 'network.jsonl'
        # Executable Fake at the same gh process boundary as the real collector.
        gh = fake / 'gh'
        gh.write_text('#!' + sys.executable + '\n' + '''import json, os, pathlib, sys
state = json.loads(pathlib.Path(os.environ['EVIDENCE_FAKE_STATE']).read_text())
endpoint = sys.argv[-1]
if endpoint.endswith('/zip'):
    data = pathlib.Path(os.environ['EVIDENCE_FAKE_ARCHIVE']).read_bytes()
    kind = 'artifact_payload'
elif '/attempts/' in endpoint:
    data = json.dumps(state['run']).encode(); kind = 'metadata'
else:
    data = json.dumps(state['artifacts']).encode(); kind = 'metadata'
with pathlib.Path(os.environ['EVIDENCE_FAKE_LEDGER']).open('a') as stream:
    stream.write(json.dumps(dict(endpoint=endpoint, kind=kind, bytes=len(data))) + '\\n')
sys.stdout.buffer.write(data)
''')
        gh.chmod(0o755)
        env.update(PATH=str(fake) + os.pathsep + env['PATH'], EVIDENCE_FAKE_STATE=str(state),
                   EVIDENCE_FAKE_ARCHIVE=str(remote), EVIDENCE_FAKE_LEDGER=str(ledger))
        metadata = root / 'ci-metadata.json'
        inventory = drive('ci-list', '--repo', 'z-chenhao/erii', '--run', 42, '--attempt', 1, '--output', metadata)
        cold = drive('ci-fetch', '--root', root, '--metadata', metadata, '--id', 7)
        warm = drive('ci-fetch', '--root', root, '--metadata', metadata, '--id', 7)
        network = [json.loads(line) for line in ledger.read_text().splitlines()]
        assert len([n for n in network if n['kind'] == 'artifact_payload']) == 1
        assert cold['downloaded_bytes'] == len(payload) and warm['downloaded_bytes'] == 0
        # Same ID cannot migrate into another attempt. No fallback download.
        inventory['attempt'] = 2
        save(metadata, inventory)
        drive('ci-fetch', '--root', root, '--metadata', metadata, '--id', 7, success=False)
        return dict(passed=True, source_sha=sha, author_manifest=collected, reader=verified,
                    commands=logs, network_objects=network, cold=cold, warm=warm,
                    local_handoff=planned, mandatory_export=exported,
                    actual_cloud_uploads=[], actual_cloud_upload_bytes=0,
                    limitations=['Network byte counts are deterministic Fake process payloads, not live HTTP traffic.',
                                 'Receipt is staged locally; no upload is inferred from file creation.',
                                 'Reader scenario tests transport reuse only; independent exact-SHA code review remains required.'],
                    cleanup='author worktree and Fake removed; durable raw/cache/export/report retained')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--sha', required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    result = run(args.root, args.sha)
    save(args.evidence, result)
    print(json.dumps(dict(passed=True, report=str(args.evidence), sha256=sha256_file(args.evidence),
                          cold_bytes=result['cold']['downloaded_bytes'], warm_bytes=0,
                          staged_receipt_bytes=result['local_handoff']['size'])))


if __name__ == '__main__':
    main()
