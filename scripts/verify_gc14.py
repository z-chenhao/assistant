#!/usr/bin/env python3
"""GC-14 six-day CLI journey; explicit scripted model, never semantic grades."""
import argparse
import copy
from datetime import datetime, timezone
import hashlib
import gzip
import io
import json
from pathlib import Path
import sqlite3
import subprocess
import sys
import tempfile

import monitor_sandbox as peer
from release_audit import inventory
from release_receipts import Receipts, strict_json
from unscripted_cases import source_identity
from verify_automation import Harness
from verify_gc03 import exact
from verify_gc11 import environment
from verify_main import require
from verify_suite import source_state

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = ROOT / 'evals/fixtures/gc14-v1.json'
BOUNDARY_LOCK = ROOT / 'evals/fixtures/gc14-boundary-v1.lock.json'
INPUT = '每天早上八点看这趟航班，低于 1800 才告诉我，没变化不要打扰。'
CANCEL = '取消这趟航班的价格监控。'


def sha(data):
    return hashlib.sha256(data).hexdigest()


def load():
    lock = strict_json(BOUNDARY_LOCK.read_bytes())
    for name, expected in lock['files'].items():
        require(sha((ROOT/name).read_bytes()) == expected, 'GC-14 lock drift: '+name)
    current = {(b['line'], b['sha256']) for b in inventory((ROOT/'evals/golden-cases.md').read_text())}
    blocks = lock['frozen_blocks']
    require(len(blocks) == len({b['line'] for b in blocks}) == 16 and all(
        b['source'] == 'evals/golden-cases.md' and (b['line'], b['sha256']) in current for b in blocks),
        'GC-14 frozen block drift')
    return lock


def variants():
    source = json.loads(FIXTURE.read_text())
    rows = [dict(id='base', source=copy.deepcopy(source), text=INPUT),
        dict(id='wording-1', source=copy.deepcopy(source), text='每天08:00查这趟航班，跌到1800元以下再提醒，没变化就别通知。'),
        dict(id='wording-2', source=copy.deepcopy(source), text='这趟航班每天早上八点查一次；价格低于1800才告诉我，持续低价不重复提醒。')]
    for name in ('source-order', 'unrelated-flight', 'unrelated-note'):
        data = copy.deepcopy(source)
        if name == 'source-order':
            data['daily_prices'] = dict(reversed(list(data['daily_prices'].items())))
        elif name == 'unrelated-flight':
            data['unrelated_flight'] = dict(route='HKG-NRT', date='2026-12-02', price=1200)
        else:
            data['unrelated_note'] = '酒店促销1599元，与本次航班价格监控无关。'
        rows.append(dict(id=name, source=data, text=INPUT))
    return rows


def sources(source):
    values = dict(monitor_source=source)
    for day, price in source['daily_prices'].items():
        for site in ('site-a', 'site-b'):
            values[day + ':' + site] = dict(value=price, route=source['route'], flight_date=source['date'],
                observed_on=day, currency='CNY', source=site)
    return values


class MonitorHarness(Harness):
    def args(self, *args):
        return [str(self.binary), '-test-bridge', 'monitor', '-test-automation-now', self.now,
                '-config', str(self.root / 'config.yaml'), *args]


def native(h):
    value = h.evidence()
    with sqlite3.connect(h.root / 'data/state.db') as db:
        db.row_factory = sqlite3.Row
        for table in ('approval', 'artifact'):
            value[table] = [dict(row) for row in db.execute('SELECT * FROM ' + table + ' ORDER BY rowid')]
    return value


def day_of(micros):
    return datetime.fromtimestamp(micros / 1e6, timezone.utc).strftime('%Y-%m-%d')


def frozen_generations(value):
    nodes = {r['run_id']:r for r in value['runs']}
    models = {s['snapshot_id']:s for s in value['snapshots']}
    capabilities = {s['snapshot_id']:s for s in value['capabilities']}
    for generation in value['generations']:
        request = strict_json(generation['request_json']); run = nodes[generation['run_id']]
        stored = models[run['model_snapshot_id']]
        model = {k.removesuffix('_json'):strict_json(v) if k.endswith('_json') else v
                 for k,v in stored.items() if v is not None and k != 'created_at'}
        actual = dict(request['model_snapshot']); created = actual.pop('created_at')
        specs = strict_json(capabilities[run['capability_snapshot_id']]['tools_json'])
        tools = [{k:s[k] for k in ('name','description','input_schema') if k in s} for s in specs]
        if (request['run_id'] != run['run_id'] or request['generation_id'] != generation['generation_id'] or
                not exact(actual,model) or int(datetime.fromisoformat(created.replace('Z','+00:00')).timestamp()*1e6) != stored['created_at'] or
                not exact(request['tools'],tools) or generation['status'] != 'completed' or generation['attempt_count'] != 1):
            return False
    return bool(value['generations'])


def price_observe(value, source, count):
    """Recompute exact fires/observations/delivery from stored state, not flags."""
    days = sorted(source['daily_prices'])[:count]
    fires = sorted(value['automation_fire'], key=lambda f: f['fire_at'])
    roots = [r for r in value['runs'] if r['kind'] == 'worker']
    reads = [t for t in value['tools'] if t['tool_name'] == 'mcp__fixture__read']
    observations = [t for t in value['tools'] if t['tool_name'] == 'builtin__automation__observe']
    entered = False
    notices = []
    for day in days:
        below = source['daily_prices'][day] < source['threshold_cny']
        if below and not entered:
            notices.append(day)
        entered = below
    return dict(
        daily_fires=len(fires) == count and [day_of(f['fire_at']) for f in fires] == days and
            all(f['fire_at'] == int(datetime.fromisoformat(day + 'T08:00:00+08:00').timestamp()*1e6) and
                f['state'] == 'admitted' and f['below_value'] == source['threshold_cny'] and
                f['observed_value'] == source['daily_prices'][day] for day, f in zip(days, fires)),
        same_ongoing_task=len(value['task']) == 1 and value['task'][0]['status'] == 'active' and
            value['task'][0]['completion_policy'] == 'ongoing' and len(roots) == count and
            len({r['run_id'] for r in roots}) == count and all(r['status'] == 'completed' and r['depth'] == 0 and
                r['parent_run_id'] is None and r['task_id'] == value['task'][0]['task_id'] for r in roots) and
            {f['root_run_id'] for f in fires} == {r['run_id'] for r in roots},
        only_threshold_entries=[day_of(f['fire_at']) for f in fires if f['notify']] == notices and
            len(value['task_update']) == len(notices) and
            {u['root_run_id'] for u in value['task_update']} == {f['root_run_id'] for f in fires if f['notify']} and
            [u['content'] for u in value['task_update']] == [
                f"航班 {source['route']} {source['date']}：{day} 核验价格 {source['daily_prices'][day]} CNY；阈值 {source['threshold_cny']} CNY。" for day in notices],
        two_verified_sources=len(reads) == 1 + 2*count and all(
            [json.loads(t['arguments_json'])['text'] for t in reads if t['run_id'] == f['root_run_id']] ==
                [day + ':site-a', day + ':site-b'] and
            [json.loads(json.loads(t['result_json'])['text']) for t in reads if t['run_id'] == f['root_run_id']] ==
                [sources(source)[day + ':' + site] for site in ('site-a', 'site-b')] for day, f in zip(days, fires)),
        two_same_value_observations=len(observations) == 2*count and all(
            [json.loads(t['arguments_json']) for t in observations if t['run_id'] == f['root_run_id']] ==
                [dict(value=source['daily_prices'][day])]*2 for day, f in zip(days, fires)),
        notifications_delivered_once=len(value['deliveries']) == 1 + len(notices) and
            len({d['message_id'] for d in value['deliveries']}) == len(value['deliveries']) and
            all(d['status'] == 'sent' for d in value['deliveries']) and
            len([m for m in value['conversation'] if m['role'] == 'assistant']) == 1 + len(notices))


def observe(row):
    value = row['daily'][-1]['state']
    source = row['variant']['source']
    first = row['initial']; initial = first['state']
    actions = [json.loads(t['arguments_json']) for t in row['cancelled']['tools']
               if t['tool_name'] == 'builtin__automation__manage']
    after = row['cancelled']
    roots = [r for r in value['runs'] if r['kind'] == 'worker']
    snapshots = {s['snapshot_id']:s for s in value['snapshots']}
    capabilities = {s['snapshot_id']:s for s in value['capabilities']}
    reads = ['monitor_source'] + [day + ':' + site for day in sorted(source['daily_prices']) for site in ('site-a','site-b')]
    checks = price_observe(value, source, 6)
    checks.update(
        generation_snapshots=frozen_generations(after),
        fire_ownership=len(value['automation']) == 1 and value['automation'][0]['task_id'] == value['task'][0]['task_id'] and
            all(f['automation_id'] == initial['automation'][0]['automation_id'] and
                f['schedule_version'] == initial['automation'][0]['schedule_version'] and
                f['instruction'] == initial['automation'][0]['instruction'] for f in value['automation_fire']),
        cancel_epoch=all(r['task_epoch'] == value['task'][0]['task_epoch'] for r in roots) and
            after['task'][0]['task_epoch'] == value['task'][0]['task_epoch']+1 and
            [p['text'] for m in after['conversation'] if m['role'] == 'user' for p in m['content']] == [row['variant']['text'],CANCEL],
        exact_daily_inventory=[d['day'] for d in row['daily']] == sorted(source['daily_prices']),
        original_input=[p['text'] for m in initial['conversation'] if m['role'] == 'user' for p in m['content']] == [row['variant']['text']],
        one_automation_no_early_task=len(initial['automation']) == 1 and not initial['task'] and not initial['automation_fire'] and
            len(initial['runs']) == 1 and initial['runs'][0]['kind'] == 'main' and initial['runs'][0]['status'] == 'completed' and
            initial == row['before_due'],
        exact_schedule=len(first['views']) == 1 and first['views'][0]['nextFire'] == '2026-10-16T00:00:00Z' and
            all(initial['automation'][0][key] == expected for key, expected in dict(trigger_type='cron', trigger_spec=source['schedule'],
                timezone=source['timezone'], below_value=source['threshold_cny'], enabled=1).items()),
        source_from_tool=peer_value(initial['tools'][0]) == source,
        all_daily_checks=all(all(price_observe(d['state'], source, i+1).values()) and d['state'] == d['replay_state']
            for i, d in enumerate(row['daily'])),
        independent_source_log=row['remote'] == reads == row['remote_replay'] == row['remote_after_cancel'],
        no_unrequested_work=not after['approval'] and not after['artifact'] and all(
            t['tool_name'] in {'mcp__fixture__read', 'builtin__automation__observe', 'builtin__automation__manage'} and
            t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in after['tools']),
        create_list_cancel=[a['operation'] for a in actions] == ['create','list','cancel'] and
            actions[-1]['automationRef'] == initial['automation'][0]['automation_id'],
        cancelled_no_new_run=len(after['automation']) == 1 and not after['automation'][0]['enabled'] and
            after['task'][0]['status'] == 'cancelled' and value['automation_fire'] == after['automation_fire'] and
            len(after['runs']) == len(value['runs'])+1 and row['cancelled'] == row['after_cancel'],
        history_retained=all(r in after['runs'] for r in value['runs']) and
            after['task_update'] == value['task_update'] and after['conversation'][:len(value['conversation'])] == value['conversation'] and
            all(item in after[table] for table in ('tools','generations','snapshots','capabilities','deliveries') for item in value[table]),
        restart_and_duplicate_equal=row['restart']['returncode'] == -9 and row['restart']['boundary'] == 'before_next_scan' and
            value == row['restart_state'],
        admitted_restart=row['admission_restart']['returncode'] == -9 and row['admission_restart']['boundary'] == 'after_automation_admit_commit' and
            [r['status'] for r in row['admission_restart']['state']['runs'] if r['kind'] == 'worker'] == ['completed','completed','queued'] and
            row['admission_restart']['reads'] == reads[:5],
        current_profile_at_admission=len(roots) == 6 and len(set(row['admission_profiles'])) == 2 and
            all(snapshots[r['model_snapshot_id']]['profile_id'] == profile for r, profile in zip(roots, row['admission_profiles'])),
        frozen_snapshots=all(r['model_snapshot_id'] in snapshots and r['capability_snapshot_id'] in capabilities for r in value['runs']) and
            all(d['state']['snapshots'] == d['replay_state']['snapshots'] and d['state']['capabilities'] == d['replay_state']['capabilities'] and
                all(s in value['snapshots'] for s in d['state']['snapshots']) and
                all(c in value['capabilities'] for c in d['state']['capabilities']) for d in row['daily']) and
            len({json.dumps({k:v for k,v in capabilities[r['capability_snapshot_id']].items()
                if k not in ('snapshot_id','agent_config_version','created_at')}, sort_keys=True) for r in roots}) == 1 and
            all(c in value['capabilities'] for c in row['admission_restart']['state']['capabilities']) and
            all(s in value['snapshots'] for s in row['admission_restart']['state']['snapshots']))
    return checks


def execution_ids(row):
    result = {}
    for table, key in (('runs','run_id'), ('tools','invocation_id'), ('generations','generation_id'),
                       ('deliveries','delivery_id'), ('conversation','message_id')):
        ids = [item[key] for item in row['cancelled'][table]]
        require(bool(ids) and all(type(i) is str and i for i in ids) and len(set(ids)) == len(ids),
                'invalid GC-14 execution identities: '+table)
        result[table] = ids
    return result


def rejection_probes(row):
    """Mutate real native receipts; stored green flags cannot rescue corruption."""
    mutations = dict(
        day=lambda r:r['daily'][0].update(day='2026-10-15'),
        schedule=lambda r:r['initial']['state']['automation'][0].update(timezone='UTC'),
        price=lambda r:r['daily'][-1]['state']['automation_fire'][0].update(observed_value=1799),
        threshold=lambda r:r['daily'][-1]['state']['automation_fire'][0].update(below_value=1801),
        currency=lambda r:r['daily'][-1]['state']['tools'][0].update(result_json='{"text":"{}"}'),
        duplicate_fire=lambda r:r['daily'][-1]['state']['automation_fire'].append(r['daily'][-1]['state']['automation_fire'][0]),
        duplicate_notice=lambda r:r['daily'][-1]['state']['deliveries'].append(r['daily'][-1]['state']['deliveries'][0]),
        missing_day=lambda r:r['daily'].pop(0),
        reversed_days=lambda r:r['daily'].reverse(),
        source_read=lambda r:r['remote'].append('unrequested-monitor'),
        replay=lambda r:r['daily'][0]['replay_state']['task'][0].update(status='completed'),
        stop=lambda r:r['after_cancel']['automation'][0].update(enabled=1),
        late_run=lambda r:r['after_cancel']['runs'].append(r['after_cancel']['runs'][0]),
        admission_crash=lambda r:r['admission_restart'].update(returncode=0),
        final_crash=lambda r:r['restart'].update(returncode=0),
        profile=lambda r:r['admission_profiles'].__setitem__(0,'forged-profile'),
        snapshot=lambda r:r['daily'][-1]['state']['snapshots'].pop(),
        generation=lambda r:r['cancelled']['generations'][0].update(request_json='{}'),
        approval=lambda r:r['cancelled']['approval'].append(dict(status='approved')),
        artifact=lambda r:r['cancelled']['artifact'].append(dict(artifact_ref='unexpected')),
        prior_tool=lambda r:r['cancelled']['tools'][0].update(arguments_json='{}'))
    mutations.update(
        fire_owner=lambda r:r['daily'][-1]['state']['automation_fire'][0].update(automation_id='other-automation'),
        old_epoch=lambda r:r['cancelled']['task'][0].update(task_epoch=1))
    result = {}
    for name, mutate in mutations.items():
        changed = copy.deepcopy(row); mutate(changed)
        try:
            result[name] = not all(observe(changed).values())
        except (KeyError, IndexError, TypeError, ValueError):
            result[name] = True
    require(all(result.values()), 'GC-14 corruption accepted: '+repr(result))
    return result


def validate_report(report, binary=None):
    require(report.get('case_id') == 'GC-14' and report.get('status') == 'mechanical_pass' and
        report.get('scope') == 'scripted_whole_case_mechanics' and report.get('cleanup') is True and
        report.get('semantic_score','missing') is None and report.get('eligible_for_grading') is False and
        report.get('release_ready') is False and report.get('boundary_lock') == load() and
        report.get('fixture_lock') == strict_json(FIXTURE.with_suffix('.lock.json').read_bytes()),
        'complete locked ungraded GC-14 receipts required')
    identity = report['identity']; binary = Path(binary or identity['binary_path'])
    require(identity['source_before'] == identity['source_after'] == source_identity() and
        identity['binary_sha256'] == sha(binary.read_bytes()) and exact(identity['environment'], environment(binary)),
        'GC-14 source/binary/environment identity drift')
    proof = dict(**source_state(), binary_sha256=identity['binary_sha256'])
    require(exact(report.get('source_provenance'), dict(before=proof,after=proof)), 'GC-14 source provenance drift')
    rows = report['variants']
    require([r['variant'] for r in rows] == variants(), 'GC-14 complete ordered six-variant inventory required')
    seen = {}
    for row in rows:
        require('failure' not in row and row.get('scope') == 'scripted_whole_case_mechanics' and
            row.get('semantic_score','missing') is None and row.get('owned_processes_joined') is True and
            exact(row.get('cleanup'),dict(owned_processes_joined=True,disposable_root_removed=True)),
            'GC-14 failed, graded or unclean native receipt')
        checks = observe(row)
        require(exact(row['assertions'],checks) and all(v is True for v in checks.values()), 'GC-14 failed or forged native assertions')
        for table, ids in execution_ids(row).items():
            require(seen.setdefault(table,set()).isdisjoint(ids), 'GC-14 reused execution identities: '+table)
            seen[table].update(ids)
    require(exact(report.get('rejection_probes'),rejection_probes(rows[0])), 'GC-14 corruption probes missing or drifted')
    return summary(report)


def summary(report):
    return dict(case_id='GC-14', status=report['status'], receipts=len(report['variants']), frozen_blocks=16,
        semantic_score=None, eligible_for_grading=False, release_ready=False, cleanup=report['cleanup'],
        source_sha=report['identity']['source_before']['sha'], binary_sha256=report['identity']['binary_sha256'])


def write_report(path, report):
    raw = path.parent / ('.'+path.stem+'-native.json.gz')
    data = json.dumps(report,ensure_ascii=False,separators=(',',':')).encode()+b'\n'
    raw.write_bytes(gzip.compress(data,compresslevel=1,mtime=0))
    manifest = dict(kind='gc14_native_manifest',version=1,summary=summary(report),
        native=dict(path=raw.name,sha256=sha(raw.read_bytes())),native_bytes=raw.stat().st_size,
        native_uncompressed_bytes=len(data))
    path.write_text(json.dumps(manifest,ensure_ascii=False,indent=2)+'\n')


def read_report(path):
    manifest = strict_json(path.read_bytes())
    require(set(manifest) == {'kind','version','summary','native','native_bytes','native_uncompressed_bytes'} and
        manifest['kind'] == 'gc14_native_manifest' and type(manifest['version']) is int and manifest['version'] == 1,
        'GC-14 native manifest required')
    raw = Receipts(path.parent,None).bytes(manifest['native'])
    require(type(manifest['native_bytes']) is int and len(raw) == manifest['native_bytes'], 'GC-14 native size mismatch')
    size = manifest['native_uncompressed_bytes']
    require(type(size) is int and 0 < size <= 512<<20, 'GC-14 bounded native receipt required')
    with gzip.GzipFile(fileobj=io.BytesIO(raw)) as archive:
        data = archive.read(size+1)
    require(len(data) == size, 'GC-14 decompressed size mismatch')
    report = strict_json(data)
    require(exact(manifest['summary'],summary(report)), 'GC-14 manifest/native summary drift')
    return report


def peer_value(tool):
    return json.loads(json.loads(tool['result_json'])['text'])


def drive(binary, evidence, variant):
    row = dict(variant=variant, scope='scripted_whole_case_mechanics', semantic_score=None, daily=[], admission_profiles=[])
    failure = None
    with tempfile.TemporaryDirectory(prefix='erii-gc14-') as temp:
        root = Path(temp); provider = root/'provider'; provider.mkdir()
        (provider/'monitor_sandbox.py').write_bytes((ROOT/'scripts/monitor_sandbox.py').read_bytes())
        (provider/'source.json').write_text(json.dumps(sources(variant['source']), ensure_ascii=False))
        interpreter = subprocess.run(['xcrun','--find','python3'], capture_output=True, text=True, check=True).stdout.strip() if sys.platform == 'darwin' else '/usr/bin/python3'
        (root/'config.yaml').write_text('version: 1\nmcp:\n  - name: fixture\n    command: '+json.dumps(interpreter)+
            '\n    directory: '+json.dumps(str(provider))+'\n    args: ['+json.dumps(str(provider/'monitor_sandbox.py'))+', '+json.dumps(str(provider))+']\n    tools: {read: read_only}\n')
        h = MonitorHarness(binary, root)
        try:
            profile = h.setup()
            h.send('monitor', variant['text']); h.command('chat','work')
            row['initial'] = dict(state=native(h), views=h.automations())
            h.now = '2026-10-16T07:59:59+08:00'
            h.command('automation','tick'); h.command('chat','work'); row['before_due'] = native(h)
            for i, day in enumerate(sorted(variant['source']['daily_prices'])):
                if i == 3:
                    profile = h.setup(model='fixture-model-updated')
                row['admission_profiles'].append(profile)
                h.now = day+'T08:00:00+08:00'
                h.command('automation','tick'); h.command('automation','tick')
                if i == 2:
                    child = h.hold('after_automation_admit_commit'); child.kill(); child.communicate(timeout=10)
                    row['admission_restart'] = dict(boundary='after_automation_admit_commit', returncode=child.returncode,
                        state=native(h), reads=peer.state(provider))
                h.command('chat','work'); value = native(h)
                h.command('automation','tick'); h.command('chat','work')
                row['daily'].append(dict(day=day, state=value, replay_state=native(h)))
            row['remote'] = peer.state(provider)
            child = h.hold('before_next_scan'); child.kill(); child.communicate(timeout=10)
            row['restart'] = dict(boundary='before_next_scan', returncode=child.returncode)
            h.send('monitor', variant['text']); h.command('chat','work')
            row['restart_state'] = native(h); row['remote_replay'] = peer.state(provider)
            h.send('cancel', CANCEL); h.command('chat','work'); row['cancelled'] = native(h)
            h.now = '2026-11-01T08:00:00+08:00'
            h.send('cancel', CANCEL); h.command('automation','tick'); h.command('chat','work')
            row['after_cancel'] = native(h); row['remote_after_cancel'] = peer.state(provider)
            row['assertions'] = observe(row)
            require(all(row['assertions'].values()), 'GC-14 assertions: '+json.dumps(row['assertions']))
        except BaseException as error:
            row.update(failure=str(error), failure_state=h.failure_state())
            failure = error
        finally:
            h.cleanup()
            row['owned_processes_joined'] = True
            (evidence/(variant['id']+'.json')).write_text(json.dumps(row, ensure_ascii=False, indent=2)+'\n')
    row['cleanup'] = dict(owned_processes_joined=True, disposable_root_removed=not root.exists())
    (evidence/(variant['id']+'.json')).write_text(json.dumps(row, ensure_ascii=False, indent=2)+'\n')
    if failure:
        raise failure
    return row


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--variant', choices=[v['id'] for v in variants()])
    parser.add_argument('--validate', action='store_true')
    args = parser.parse_args()
    if args.validate:
        print(json.dumps(validate_report(read_report(args.evidence),args.binary)))
        return
    args.evidence.unlink(missing_ok=True)
    folder = args.evidence.with_suffix(''); folder.mkdir(parents=True, exist_ok=True)
    initial = dict(**source_state(), binary_sha256=sha(args.binary.read_bytes()))
    identity = dict(source_before=source_identity(),binary_sha256=initial['binary_sha256'],
        binary_path=str(args.binary.resolve()),environment=environment(args.binary))
    boundary = load()
    lock = json.loads(FIXTURE.with_suffix('.lock.json').read_text())
    require(lock == dict(fixture_sha256=sha(FIXTURE.read_bytes()), golden_sha256=sha((ROOT/'evals/golden-cases.md').read_bytes())), 'GC-14 fixture lock drift')
    rows = [drive(args.binary.resolve(), folder, v) for v in variants() if not args.variant or v['id'] == args.variant]
    final = dict(**source_state(), binary_sha256=sha(args.binary.read_bytes()))
    identity['source_after'] = source_identity()
    require(initial == final, 'source or binary changed during GC-14 execution')
    report = dict(case_id='GC-14', status='mechanical_pass', scope='scripted_whole_case_mechanics', semantic_score=None,
        source_provenance=dict(before=initial, after=final), identity=identity, boundary_lock=boundary,
        eligible_for_grading=False, release_ready=False, fixture_lock=lock, variants=rows,
        cleanup=all(all(r['cleanup'].values()) for r in rows))
    report['rejection_probes'] = rejection_probes(rows[0])
    if not args.variant:
        validate_report(report,args.binary)
    write_report(args.evidence,report)
    print(json.dumps(dict(status=report['status'], variants=len(rows), cleanup=report['cleanup'])))


if __name__ == '__main__':
    main()
