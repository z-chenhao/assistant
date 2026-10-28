#!/usr/bin/env python3
"""Offline operations Launch/Doctor/Drive/Evidence/Cleanup through the real CLI."""
import argparse
import hashlib
import json
from pathlib import Path
import sqlite3
import tempfile
import time

from verify_artifacts import ArtifactHarness
from verify_main import require
from verify_memory_maintenance import manage, work, readback
from verify_tools import pending, decide, state
from verify_scheduling import setup as batch_setup, state as batch_state, concurrency


def run(binary):
    report = dict(version=1, scope='M7 operations contracts; no semantic score or exactly-once external-effect claim',
                  checks=[], passed=False)
    with tempfile.TemporaryDirectory(prefix='erii-operations-') as folder:
        root = Path(folder)
        h = ArtifactHarness(binary, root, fixture='approval')
        h.env.update(ERII_BACKUP_KEY='B'*32, ERII_MEMORY_KEY='M'*32,
                     ERII_AUTH_FIXTURE='PRIVATE_CREDENTIAL_SENTINEL_29')
        (root / 'tool-config.yaml').write_text('version: 1\nmemory:\n  encryption_key_ref: env:ERII_MEMORY_KEY\n')
        try:
            h.setup()
            auth = h.command('model', 'login', 'ERII_AUTH_FIXTURE')
            require('credential_ref' not in auth, 'credential ref exposed on public login response')
            with sqlite3.connect(Path(h.env['ERII_DATA_ROOT']) / 'state.db') as db:
                credential_before = db.execute('SELECT credential_ref FROM auth_connection WHERE connection_id=?', (auth['connection_id'],)).fetchone()[0]
            require(credential_before == 'env:ERII_AUTH_FIXTURE', 'credential ref did not use SecretStore')
            saved = h.save('operations.txt', 'text/plain', b'PRIVATE_ARTIFACT_29\n')
            ref = saved['artifact_ref']
            manage(h, 'durable-memory', operation='remember', content='PRIVATE_MEMORY_29 I prefer tea')
            work(h)
            memory = readback(h)['memory']
            require(memory['records'], 'doctor: no durable Memory')
            approval = pending(h)
            before = state(h)
            health = h.command('ops', 'health')
            require(health['counts']['approvals_pending'] == 1, 'health lost frozen approval')
            run_id = before['runs'][0]['run_id']
            trace = h.command('ops', 'trace', run_id)
            require(trace['run_id'] == run_id and trace['points'], 'trace has no correlated runtime events')
            bug = h.command('ops', 'report')
            encoded = json.dumps([bug, trace])
            require(not any(v in encoded for v in ('alex.chen', 'PRIVATE_', 'credential_ref', 'arguments_json', 'content_json', 'endpoint')), 'bug report exposed domain/private data')
            archive = root / 'backup.erii'
            # A real held process proves offline exclusion; kill releases the
            # kernel lock, then an archive can be made without stale owners.
            held = h.hold('before_next_scan')
            result = h.raw('ops', 'backup', str(archive), 'env:ERII_BACKUP_KEY', ok=False)
            require(result.returncode != 0 and b'data root is in use' in result.stderr and not archive.exists(), 'backup copied an active instance')
            held.kill()
            held.communicate(timeout=10)
            memory = readback(h)['memory']  # Freeze the actual backup boundary, after Curator joined.
            manifest = h.command('ops', 'backup', str(archive), 'env:ERII_BACKUP_KEY')
            raw = archive.read_bytes()
            require(not any(v in raw for v in (b'SQLite format', b'PRIVATE_', b'B'*32, b'M'*32)), 'unencrypted archive material')
            require(h.command('ops', 'backup', str(archive), 'env:ERII_BACKUP_KEY', ok=False).returncode != 0, 'existing backup overwritten')
            target = root / 'restored'
            h.env['ERII_DATA_ROOT'] = str(target)
            restored = h.command('ops', 'restore', str(archive), 'env:ERII_BACKUP_KEY')
            require(restored == manifest, 'restore manifest changed')
            require(h.command('chat', 'work', ok=False).returncode != 0, 'restored identity dispatched before cutover')
            for entry in manifest['files']:
                data = (target / entry['path']).read_bytes()
                require(len(data) == entry['bytes'] and hashlib.sha256(data).hexdigest() == entry['sha256'], 'restored file hash differs')
            h.command('ops', 'activate', 'source-retired-and-outcomes-reconciled')
            require(h.command('status')['agent_ingress_enabled'], 'restored profile lost readiness')
            require(h.output(ref) == b'PRIVATE_ARTIFACT_29\n', 'Artifact bytes/reference lost')
            require(readback(h)['memory'] == memory, 'encrypted Memory or receipt lost')
            with sqlite3.connect(target / 'state.db') as db:
                credential = db.execute('SELECT credential_ref FROM auth_connection WHERE connection_id=?', (auth['connection_id'],)).fetchone()[0]
            require(credential == credential_before, 'credential reference changed')
            current = h.command('approval', 'list')[-1]
            require(current['approval_id'] == approval['approval_id'] and current['frozen_arguments_hash'] == approval['frozen_arguments_hash'], 'restored approval identity/hash drift')
            decide(h, current, 'deny')
            h.command('chat', 'work')
            require(not state(h)['effects'] and not h.command('ops', 'health')['counts']['runs_active'], 'denied restored work dispatched or orphaned')
            h.command('chat', 'work')
            report.update(manifest=manifest, archive_sha256=hashlib.sha256(raw).hexdigest(),
                          health=health, trace=trace, bug_report=bug,
                          after=h.command('ops', 'health'), credential_reference_preserved=True,
                          restored_artifact_sha256=hashlib.sha256(h.output(ref)).hexdigest())
            for path in target.rglob('*'):
                if path.is_file():
                    content = path.read_bytes()
                    require(not any(value.encode() in content for value in (h.env['ERII_BACKUP_KEY'], h.env['ERII_MEMORY_KEY'], h.env['ERII_AUTH_FIXTURE'])), 'Secret leaked into restored data/logs')
            report['checks'] = ['CLI/SecretStore setup Doctor', 'operator-only health and correlated content-free trace/report',
                                'SIGKILL releases shared lock; live backup refused',
                                'encrypted consistent state/conversation/memory/Artifact archive',
                                'new-root restore hash inventory and cutover fence',
                                'preserved credentials refs and frozen approval; denied active work recovers with no effect',
                                'secret scan and duplicate drain stability']
            report['passed'] = True
        finally:
            h.cleanup()
    report['cleanup'] = 'all owned processes joined; disposable roots and archive removed'
    return report


def capacity(binary):
    """Three default-limit 1000-Child batches reuse one durable state root."""
    with tempfile.TemporaryDirectory(prefix='erii-capacity-') as folder:
        h = ArtifactHarness(binary, Path(folder), fixture='children')
        try:
            batch_setup(h)
            cycles = []
            for cycle in range(3):
                h.send('thousand-' + str(cycle), 'Delegate: Batch 1000')
                started = time.monotonic()
                h.command('chat', 'work', timeout=180)
                elapsed = time.monotonic() - started
                value = batch_state(h)
                children = [r for r in value['runs'] if r['parent_run_id']]
                require(len(children) == 1000*(cycle+1) and sum(r['status'] == 'completed' for r in children) == 998*(cycle+1),
                        'normal 1000-item batch failed admission/completion')
                require(len({r['run_id'] for r in children}) == 1000*(cycle+1) and len(value['tasks']) == cycle+1,
                        'capacity soak duplicated identities or Tasks')
                require(len(value['updates']) == cycle+1 and all(u['content'] == 'Batch aggregate: 998 results, 2 failed. Needs input: confirm region' for u in value['updates']),
                        '1000-item aggregate or blocker adoption failed')
                observed = concurrency(value)
                require(not observed['remaining_worker_leases'], 'capacity run orphaned a lease')
                require(all('RESOURCE_LIMIT' not in r.get('error_code', '') for r in value['runs']), 'normal batch hit a safety fuse')
                health = h.command('ops', 'health')
                require(health['scheduler_limits']['child_concurrency'] == 16 and health['counts']['runs_active'] == 0,
                        'capacity changed defaults or left active work')
                require(not health['trees'], 'completed trees crowd active fairness diagnostics')
                cycles.append(dict(cycle=cycle+1, seconds=elapsed, observed=observed, health=health))
            return dict(items=1000, cycles=cycles, total_items=3000, completed=2994, business_failures=6,
                        seconds=sum(v['seconds'] for v in cycles), passed=True,
                        cleanup='all owned processes joined; reused durable state and disposable roots removed')
        finally:
            h.cleanup()


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--binary', type=Path, required=True)
    p.add_argument('--evidence', type=Path, required=True)
    a = p.parse_args()
    result = run(a.binary.resolve())
    result['binary_sha256'] = hashlib.sha256(a.binary.read_bytes()).hexdigest()
    a.evidence.parent.mkdir(parents=True, exist_ok=True)
    a.evidence.write_text(json.dumps(result, indent=2)+'\n')
    print(json.dumps(dict(passed=result['passed'], checks=len(result.get('checks', [])), cleanup=result['cleanup'])))
