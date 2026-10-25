#!/usr/bin/env python3
"""Frozen Case inputs through the production CLI and an explicitly selected profile.

Private evaluation producer. No provider, loop, scoring, credential import or
automatic approval lives here. Current adapters are input fragments, not release
trials. A dedicated operator-prepared root is retained, including failed state.
"""
import argparse
from contextlib import closing
import fcntl
import json
import math
from pathlib import Path
import signal
import sqlite3
import subprocess
import time

from case_runner import digest, load_contract, variants
from fixture_pool import Scope
from release_receipts import CASES, PACKET_FIELDS, git, hashed, require, strict_json
from run_cases import timestamp
from verify_main import MainHarness

ROOT = Path(__file__).resolve().parents[1]
# Reuse locked, non-scripted inputs. Never import scripted execute()/plans.
INPUTS = {f'GC-{n:02}': 'm6-v1' for n in (1, 2, 3, 4, 5, 6, 7, 10, 13, 15, 18)}
INPUTS['GC-09'] = 'm1-v2'
GAPS = {
    'GC-01': 'health connector and durable-memory adapter/clinical quality; --gc01-journey supplies week or safety inputs only',
    'GC-02': 'memory, scheduled quarterly follow-up and logical-clock driver',
    'GC-03': 'asset connector and correction/forget journey',
    'GC-04': 'travel files, calendar approvals and recovery journey',
    'GC-05': 'historical recall and opportunity sources',
    'GC-06': 'durable learning memory and correction journey',
    'GC-07': 'device/network fixture and approved diagnostics',
    'GC-08': 'non-scripted full event fixture and wedding Task lifecycle',
    'GC-09': 'complete Inbox/calendar sources and interruption journey',
    'GC-10': 'fresh local sources and stale-source adversary',
    'GC-11': 'catalog/source fixture adapter (PR32 driver remains unmerged provenance)',
    'GC-12': 'Mail/Calendar frozen approvals and steering adapter',
    'GC-13': 'booking browser, approval and unknown-outcome adapter',
    'GC-14': 'scheduled source/threshold and notification adapter',
    'GC-15': 'native file ingress and multimodal Artifact adapter',
    'GC-16': 'research sources, Child work and Artifact adapter',
    'GC-17': 'exact 100-row attachments, assertions and kill/restart adapter',
    'GC-18': 'native files, concurrent Tasks and pause/append adapter',
    'GC-19': 'isolated coding workspace and protection adapter',
    'GC-20': 'engineering workspace, migration and recovery adapter',
}


def matrix():
    return {case: dict(adapter='conversation_fragment' if case in INPUTS else None,
                       fixture=INPUTS.get(case), eligible_for_grading=False,
                       missing=GAPS[case]) for case in sorted(CASES)}


def inputs(case_id, variant_id, journey=None):
    require(case_id in INPUTS, 'ineligible: missing adapter: ' + GAPS[case_id])
    dataset, _, lock = load_contract(ROOT, INPUTS[case_id])
    case = next(c for c in dataset['cases'] if c['case_id'] == case_id)
    variant = next((v for v in variants(dataset, case) if v['variant_id'] == variant_id), None)
    require(variant is not None, 'unknown variant')
    # Gold, assertions, required_text, max_questions, wait_for, script plans and
    # expected state never enter the model. Noise/order use the maintained code.
    value = dict(case_id=case_id, case_version=case['version'], variant_id=variant_id,
                seed=variant['seed'], clock=variant['clock'], context=variant['context'],
                events=[dict(at=e['at'], text=e['text']) for e in variant['events']],
                fixture_digest=variant['fixture_digest'], lock=lock)
    if journey is not None:
        require(case_id == 'GC-01' and journey in ('week', 'safety'), 'GC-01 journey only')
        from verify_gc01 import journey_inputs
        value['events'], value['journey_lock'] = journey_inputs(variant_id, journey == 'safety')
        value['journey'] = journey
    return value


def source_identity():
    # Include actual working bytes: a SHA alone cannot identify an edited runner.
    names = git(ROOT, 'ls-files', '-z', 'cmd', 'internal', 'scripts', 'evals',
                'provider-bridge', 'runtime/javascript', 'go.mod', 'go.sum').decode().split('\0')
    files = {name: hashed((ROOT / name).read_bytes()) for name in names if name}
    # New producer files must also participate before the first commit.
    files['scripts/unscripted_cases.py'] = hashed(Path(__file__).read_bytes())
    return dict(sha=git(ROOT, 'rev-parse', 'HEAD').decode().strip(), files=files)


class ProductionHarness(MainHarness):
    def __init__(self, binary, root, config, seconds):
        super().__init__(binary, root)
        self.config = config
        self.processes = Scope()
        self.deadline = time.monotonic() + seconds
        self.commands = []

    def args(self, *args):
        return [str(self.binary), '-config', str(self.config), *args]

    def command(self, *args, text=None, ok=True, timeout=None):
        remaining = self.deadline - time.monotonic()
        require(remaining > 0, 'wall budget exhausted')
        start = time.monotonic()
        row = dict(args=list(args), exit_code=None)
        self.commands.append(row)
        try:
            result = self.processes.run(self.args(*args), input=text, text=True,
                capture_output=True, cwd=self.root, env=self.env, timeout=remaining)
            row['exit_code'] = result.returncode
            # Provider stderr is untrusted and may contain secrets. Keep only
            # status here; normalized native error_code is read from state.
            require(not ok or result.returncode == 0, 'CLI failure: ' + ' '.join(args[:2]))
            return strict_json(result.stdout) if ok else result
        finally:
            row['wall_ms'] = round((time.monotonic() - start) * 1000)

    def cleanup(self):
        self.processes.cancel()
        self.processes.finish()


def rows(root, table):
    path = root / 'data/state.db'
    if not path.exists():
        return []
    with closing(sqlite3.connect(f'file:{path}?mode=ro', uri=True)) as db:
        db.row_factory = sqlite3.Row
        # Table names are internal constants, never model/operator text.
        return [dict(row) for row in db.execute('SELECT * FROM ' + table + ' ORDER BY rowid')]


def selected(h):
    status = h.command('status')
    if status['readiness'] != 'ready':
        return None
    profiles = h.command('model', 'list')
    return next(p for p in profiles if p['profile_id'] == status['active_model_profile_id'])


def configuration(path):
    value = strict_json(path.read_bytes())
    # This input adapter has no external-effect or mutable external fixtures.
    # Broader adapters must explicitly own and freeze those lifecycle boundaries.
    require(set(value) <= {'version', 'llm', 'context', 'children', 'scheduler', 'tools',
                            'artifact', 'spend', 'javascript'},
            'conversation adapter requires JSON config without external adapters/data-root overrides')
    require(value.get('version') == 1 and 'llm' in value, 'explicit production bridge required')
    llm = value['llm']
    require(set(llm) == {'provider_bridge_command', 'provider_bridge_script', 'provider_bridge_version'},
            'explicit pinned production bridge required')
    require(Path(llm['provider_bridge_script']).resolve() == ROOT / 'provider-bridge/bridge.mjs'
            and llm['provider_bridge_version'] == 'pi-ai-1.3.0-v1', 'production bridge identity mismatch')
    return value


def freeze(h, case_id, variant_id, kind, arm, repetition, seconds, journey=None):
    require(not rows(h.root, 'run') and not rows(h.root, 'main_input'), 'dedicated empty evaluation root required')
    config = configuration(h.config)
    profile = selected(h)
    if profile is None:
        return dict(status='setup_required', eligible_for_grading=False, support=matrix())
    if case_id not in INPUTS:
        return dict(status='ineligible', case_id=case_id, missing=GAPS[case_id],
                    eligible_for_grading=False, support=matrix())
    require((arm == 'account') == (profile['provider_kind'] == 'openai_account'), 'arm/profile mismatch')
    return dict(version=1, status='frozen', source=source_identity(),
        binary_sha256=hashed(h.binary.read_bytes()), configuration_sha256=hashed(h.config.read_bytes()),
        configuration=config, profile=profile, input=inputs(case_id, variant_id, journey),
        execution_kind=kind, arm=arm, repetition=repetition, wall_budget_seconds=seconds,
        support=matrix(), scope='input_fragment', eligible_for_grading=False,
        seed_policy='fixture seed is recorded; provider RNG seed is unsupported, never claimed deterministic')


def native(h):
    # Retain native state including failed/incomplete calls. No synthetic ledger.
    value = h.state()
    for table in ('task', 'task_instruction', 'approval', 'artifact', 'artifact_reference',
                  'artifact_publication', 'spend_snapshot', 'spend_reservation', 'spend_approval'):
        value[table] = rows(h.root, table)
    value['provider_usage'] = rows(h.root, 'provider_usage')
    value['tool_invocation'] = rows(h.root, 'tool_invocation')
    return value


def frozen_requests(state, profile):
    """Compare observed native requests, not a fabricated capability receipt."""
    requests = [json.loads(g['request_json']) for g in state['generations']]
    if not requests:
        return False
    for request in requests:
        model = request['model_snapshot']
        for key in ('profile_id', 'model_id', 'provider_kind', 'wire_api', 'context_window',
                    'max_output_tokens', 'generation_defaults', 'compatibility', 'capabilities',
                    'auth_connection_id', 'reasoning_effort'):
            if model.get(key) != profile.get(key):
                return False
        if model['profile_version'] != profile['version']:
            return False
        if profile.get('endpoint') and model.get('endpoint_hash') != digest({'endpoint': profile['endpoint']}):
            return False
        peers = [r for r in requests if r['run_id'] == request['run_id']]
        if any(r['model_snapshot'] != model or r.get('tools') != request.get('tools') for r in peers):
            return False
    return True


def file_receipt(folder, name, value):
    data = (json.dumps(value, ensure_ascii=False, sort_keys=True, indent=2) + '\n').encode()
    (folder / name).write_bytes(data)
    return dict(path=name, sha256=hashed(data))


def artifact_receipts(root, output, state):
    exported = []
    folder = root / 'data/artifacts'
    for artifact in state['artifact']:
        if artifact['deleted_at'] is not None:
            continue
        source = folder / artifact['storage_key']
        require(source.resolve().is_relative_to(folder.resolve()) and not source.is_symlink(), 'unsafe Artifact storage key')
        data = source.read_bytes()
        require(hashed(data) == artifact['content_hash'] and len(data) == artifact['byte_size'], 'Artifact bytes drift')
        name = 'artifact-' + str(len(exported)) + '.bin'
        (output/name).write_bytes(data)
        exported.append(dict(artifact_ref=artifact['artifact_ref'], path=name, sha256=hashed(data)))
    return exported


def execute(h, plan, output):
    started, tick = timestamp(), time.monotonic()
    report = dict(version=1, kind='unscripted_observation', status='failed', started_at=started,
                  eligible_for_grading=False, scope='input_fragment', final_score=None,
                  source=plan['source'], plan_sha256=digest(plan), support=matrix(),
                  execution_kind=plan['execution_kind'], identity=dict(case_id=plan['input']['case_id'],
                      variant_id=plan['input']['variant_id'], arm=plan['arm'], repetition=plan['repetition']))
    state = None
    admissions = []
    try:
        require(plan == freeze(h, plan['input']['case_id'], plan['input']['variant_id'],
            plan['execution_kind'], plan['arm'], plan['repetition'], plan['wall_budget_seconds'],
            plan['input'].get('journey')), 'frozen conditions drift')
        item = plan['input']
        # Same ingress mechanics as run_cases: background is explicit user data.
        h.send('fixture-context', item['context'])
        for index, event in enumerate(item['events']):
            require(selected(h) == plan['profile'], 'active profile drift')
            require(hashed(h.config.read_bytes()) == plan['configuration_sha256'], 'configuration drift')
            h.send('event-' + str(index), '当前时间：' + event['at'] + '。' + event['text'])
            admissions.append(file_receipt(output, 'admission-' + str(index) + '.json',
                dict(runs=rows(h.root, 'run'), model_snapshots=rows(h.root, 'model_snapshot'),
                     capability_snapshots=rows(h.root, 'capability_snapshot'),
                     inputs=rows(h.root, 'main_input'))))
            h.command('chat', 'work')
            state = native(h)
            require(all(r['status'] == 'completed' for r in state['runs']), 'Run failed or waiting; no automatic approvals/retries')
            require(all(g['status'] == 'completed' for g in state['generations']), 'generation did not complete')
            require(frozen_requests(state, plan['profile']), 'native model/Tool snapshot drift')
        require(source_identity() == plan['source'] and hashed(h.binary.read_bytes()) == plan['binary_sha256'],
                'source/binary drift during execution')
        report['status'] = 'observed_fragment'
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired, StopIteration) as error:
        report['error'] = type(error).__name__ if isinstance(error, (OSError, StopIteration)) else str(error)
    finally:
        h.cleanup()
        report['cleanup'] = dict(owned_processes_joined=True, root_retained=True,
                                 external_effect_adapters='disabled', operator_root=str(h.root))
        # Read evidence with a new bounded CLI lifecycle, even after timeout.
        reader = ProductionHarness(h.binary, h.root, h.config, 20)
        try:
            state = native(reader)
        finally:
            reader.cleanup()
        raw = file_receipt(output, 'native.json', state)
        artifacts = artifact_receipts(h.root, output, state)
        item = plan['input']
        assertions = dict(cli_completed=report['status'] == 'observed_fragment',
            terminal_runs=bool(state['runs']) and all(r['status'] == 'completed' for r in state['runs']),
            completed_generations=bool(state['generations']) and all(g['status'] == 'completed' for g in state['generations']),
            atomic_usage={g['generation_id'] for g in state['generations']} <=
                         {u['generation_id'] for u in state['provider_usage']},
            frozen_requests=frozen_requests(state, plan['profile']))
        published = {a['artifact_ref'] for a in state['artifact_publication']}
        packet = dict(goal=item['events'][0]['text'],
            allowed_fixture=dict(context=item['context'], events=item['events']),
            conversation=state['conversation'],
            final_artifact_summary=[{k: a[k] for k in ('artifact_ref', 'name', 'media_type', 'byte_size', 'content_hash')}
                                    for a in state['artifact'] if a['artifact_ref'] in published],
            programmatic_assertions=assertions)
        require(set(packet) == PACKET_FIELDS, 'Judge packet field drift')
        report.update(raw_evidence=[raw, *admissions], exported_artifacts=artifacts, judge_input=file_receipt(output, 'judge-input.json', packet),
            assertions=assertions, model_snapshot=state['snapshots'], capability_snapshot=state['capabilities'],
            execution_ids=[r['run_id'] for r in state['runs']],
            observed_conditions=dict(context_requests_sha256=digest([g['request_json'] for g in state['generations']]),
                                     capability_sha256=digest(state['capabilities']),
                                     tools_sha256=digest([json.loads(g['request_json']).get('tools') for g in state['generations']])),
            costs=dict(pricing='native spend snapshots; missing price is unpriced',
                       snapshots=state['spend_snapshot'], settlements=state['spend_reservation'],
                       usage=state['provider_usage']),
            metrics=dict(wall_time_ms=round((time.monotonic()-tick)*1000),
                         model_calls=len(state['generations']), tool_calls=len(state['tools']),
                         resources=None, wall_budget_seconds=plan['wall_budget_seconds']),
            commands=h.commands, finished_at=timestamp(),
            missing=GAPS[item['case_id']],
            grading='ineligible: full Case adapters/assertions/hard-failure coverage and independent Judge are absent')
        if not all(assertions.values()):
            report['status'] = 'failed'
        file_receipt(output, 'observation.json', report)
    return report


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=('matrix', 'freeze', 'run'))
    parser.add_argument('--binary', type=Path, default=ROOT / 'bin/erii')
    parser.add_argument('--root', type=Path, help='Dedicated prepared root; state is never deleted')
    parser.add_argument('--config', type=Path, help='Explicit JSON subset of production YAML config')
    parser.add_argument('--output', type=Path, required=True, help='New directory, outside the prepared root')
    parser.add_argument('--case', choices=sorted(CASES), default='GC-09')
    parser.add_argument('--variant', default='wording-1')
    parser.add_argument('--gc01-journey', choices=('week', 'safety'), help='GC-01 user inputs only; remains ineligible')
    parser.add_argument('--arm', choices=('local', 'account'), default='local')
    parser.add_argument('--repetition', type=int, choices=range(1, 6), default=1)
    parser.add_argument('--execution-kind', choices=('live_model', 'loopback_mechanism'), default='live_model')
    parser.add_argument('--seconds', type=float, default=120)
    parser.add_argument('--plan', type=Path)
    parser.add_argument('--plan-sha256')
    args = parser.parse_args(argv)
    require(math.isfinite(args.seconds) and 0 < args.seconds <= 3600, 'wall budget must be in (0,3600]')
    args.output = args.output.resolve()
    if args.action == 'matrix':
        args.output.mkdir(parents=True, exist_ok=False)
        file_receipt(args.output, 'support.json', matrix())
        return 0
    require(args.root and args.config, '--root and --config required')
    root, config = args.root.resolve(strict=True), args.config.resolve(strict=True)
    require(not args.output.is_relative_to(root) and not root.is_relative_to(args.output), 'output/root overlap')
    require(root != Path.home() and root != ROOT, 'dedicated root required')
    args.output.mkdir(parents=True, exist_ok=False)
    def terminate(signum, _):
        raise RuntimeError('interrupted by signal ' + str(signum))
    previous = {s: signal.signal(s, terminate) for s in (signal.SIGINT, signal.SIGTERM)}
    try:
        with (root / '.unscripted.lock').open('a') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            plan = None
            if args.action == 'run':
                require(args.plan and args.plan_sha256, 'run requires pinned --plan and --plan-sha256')
                data = args.plan.read_bytes()
                require(hashed(data) == args.plan_sha256, 'plan bytes drift')
                plan = strict_json(data)
                require(plan.get('status') == 'frozen', 'setup_required or invalid plan')
            h = ProductionHarness(args.binary.resolve(strict=True), root, config,
                                  plan['wall_budget_seconds'] if plan else args.seconds)
            try:
                if args.action == 'freeze':
                    plan = freeze(h, args.case, args.variant, args.execution_kind, args.arm, args.repetition, args.seconds,
                                  args.gc01_journey)
                    file_receipt(args.output, 'plan.json', plan)
                    print(json.dumps(dict(status=plan['status'], eligible_for_grading=False)))
                    return 2 if plan['status'] != 'frozen' else 0
                report = execute(h, plan, args.output)
                print(json.dumps(dict(status=report['status'], eligible_for_grading=False)))
                return 0 if report['status'] == 'observed_fragment' else 1
            finally:
                h.cleanup()
    except (ValueError, RuntimeError, OSError, subprocess.TimeoutExpired) as error:
        # Rejected plans never leave an earlier successful report at this path.
        file_receipt(args.output, 'rejected.json', dict(status='rejected', eligible_for_grading=False,
            error=type(error).__name__ if isinstance(error, OSError) else str(error)))
        print(json.dumps(dict(status='rejected', eligible_for_grading=False)))
        return 1
    finally:
        for s, handler in previous.items():
            signal.signal(s, handler)


if __name__ == '__main__':
    raise SystemExit(main())
