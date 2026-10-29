#!/usr/bin/env python3
"""Account spend: maintained CLI launch/doctor/drive/evidence/cleanup actions."""
import argparse
import hashlib
import json
from pathlib import Path
import signal
import sqlite3
import tempfile

from verify_artifacts import ArtifactHarness
from terminal_driver import Terminal
from verify_children import state as child_state
from verify_main import require
from verify_scheduling import setup
from verify_tools import decide, state as tool_state


FIXTURE = json.loads((Path(__file__).resolve().parents[2] / 'tests/fixtures/m4-spend-v1.json').read_text())


def state(h):
    value = child_state(h)
    with sqlite3.connect(Path(h.env['COLLIE_DATA_ROOT']) / 'state.db') as db:
        db.row_factory = sqlite3.Row
        for table in ('spend_snapshot', 'spend_reservation', 'spend_approval'):
            value[table] = [dict(row) for row in db.execute('SELECT * FROM ' + table + ' ORDER BY 1')]
        value['priced_usage'] = [dict(row) for row in db.execute('SELECT * FROM provider_usage ORDER BY generation_id')]
    value['spend_status'] = h.command('spend', 'status')
    return value


def policy(h, *, boundary=None, priced=True, model=None):
    # Matches llm.EndpointHash: canonical JSON of the endpoint, no secret/URL in
    # account status, approval arguments or trace metadata.
    endpoint_hash = hashlib.sha256(json.dumps({'endpoint': FIXTURE['endpoint']}, separators=(',', ':')).encode()).hexdigest()
    account = {'account': 'openai_compatible:' + endpoint_hash, 'currency': FIXTURE['currency'],
               'boundary_micros': FIXTURE['boundary_micros'] if boundary is None else boundary,
               'prices': [dict(FIXTURE['price'], model=model or FIXTURE['price']['model'])] if priced else []}
    # JSON is valid YAML, so no extra Python package is needed by CI.
    (h.root / 'tool-config.yaml').write_text(json.dumps({'version': 1, 'spend': {'accounts': [account]}}))


def priced_setup(h, model=None):
    h.setup(metadata={'name': 'spend fixture', 'provider_kind': 'openai_compatible',
                      'endpoint': FIXTURE['endpoint'], 'wire_api': 'openai_completions',
                      'model_id': model or FIXTURE['price']['model'], 'context_window': 131072,
                      'max_output_tokens': 8192, 'generation_defaults': {},
                      'compatibility': {'tool_result_format': 'openai'},
                      'capabilities': {'text_input': True}})
    status = h.command('spend', 'status')
    require(status and all(v['state'] == 'unpriced_unconfigured' for v in status), 'initial spend state guessed prices')
    require(any(v['unpriced_calls'] > 0 for v in status), 'Probe observations omitted from unknown account usage')
    require(FIXTURE['endpoint'] not in json.dumps(status), 'status exposed endpoint')


def monetary(h):
    return [a for a in h.command('approval', 'list') if a['tool'] == 'account_spend']


def finish_main(h):
    """A fresh synchronous Main exchange needs its own spend consent. It must
    never inherit a Worker grant. Bound this action to observed pending Mains."""
    for _ in range(8):
        value = state(h)
        runs = {r['run_id']: r for r in value['runs']}
        pending = [a for a in value['spend_approval'] if a['status'] == 'pending']
        if not pending:
            return
        require(all(runs[a['root_run_id']]['kind'] == 'main' for a in pending), 'unexpected second Root spend prompt')
        for a in pending:
            h.command('approval', 'approve', a['approval_id'], a['frozen_hash'])
        h.command('chat', 'work')
    raise RuntimeError('Main monetary prompt storm')


def run(binary):
    report = {'version': 1, 'scope': FIXTURE['scope'], 'checks': [],
              'binary_sha256': hashlib.sha256(binary.read_bytes()).hexdigest()}
    with tempfile.TemporaryDirectory(prefix='collie-spend-') as folder:
        root = Path(folder)

        def scenario(name, action):
            path = root / name
            path.mkdir()
            h = ArtifactHarness(binary, path, fixture='children')
            try:
                priced_setup(h)
                return action(h)
            finally:
                h.cleanup()

        def batch(h):
            h.send('batch', 'Delegate: Batch priced 100')  # Main freezes unconfigured metadata
            policy(h)
            h.command('chat', 'work')
            before = state(h)
            roots = [r for r in before['runs'] if r['kind'] == 'worker' and not r['parent_run_id']]
            require(len(roots) == 1 and len(before['spend_approval']) == 1, 'Root approval not coalesced')
            approval = monetary(h)[0]
            require(before['spend_approval'][0]['root_run_id'] == roots[0]['run_id'], 'Child prompted directly')
            require(approval['status'] == 'pending' and all(c['token'] is None for c in before['claims']), 'hold retained resources')
            args = json.loads(approval['arguments'])
            require(args['next_call_max_micros'] == FIXTURE['expected_projection_micros'], 'projection ignored frozen model bounds')
            require(h.command('approval', 'approve', approval['approval_id'], 'wrong', ok=False).returncode != 0, 'hash drift accepted')
            # Every command is a fresh process: approval survives process exit;
            # the restart preserves pending Child/generation identities.
            pending_ids = {g['generation_id'] for g in before['generations']}
            decide(h, approval)
            decide(h, approval)
            h.command('chat', 'work')
            finish_main(h)
            after = state(h)
            children = [r for r in after['runs'] if r['parent_run_id']]
            require(len(children) == 100 and sum(r['status'] == 'completed' for r in children) == 98, 'batch did not complete')
            root_approvals = [a for a in after['spend_approval'] if a['root_run_id'] == roots[0]['run_id']]
            require(len(root_approvals) == 1 and root_approvals[0]['status'] == 'approved', 'Child prompt storm after grant')
            require(pending_ids <= {g['generation_id'] for g in after['generations']}, 'restart replaced generation')
            priced = [v for v in after['spend_reservation'] if v['status'] == 'priced']
            require(priced and all(v['actual_micros'] == FIXTURE['expected_call_micros'] for v in priced), 'currency conversion mismatch')
            require(len({v['generation_id'] for v in after['spend_reservation']}) == len(after['spend_reservation']), 'duplicate reservation')
            require(after['updates'][0]['content'] == 'Batch aggregate: 98 results, 2 failed. Needs input: confirm region', 'wrong aggregate')
            decide(h, approval)  # delayed duplicate after terminal Root
            h.command('chat', 'work')
            require(state(h) == after, 'terminal monetary replay changed state')
            return {'name': '100_item_root_coalescing_restart_conversion', 'before': before, 'after': after}
        report['checks'].append(scenario('batch', batch))

        def actual_final(h):
            priced_setup(h, model='overrun-fixture-model')
            h.send('actual-final', 'Delegate: Spend final')
            policy(h, model='overrun-fixture-model')
            h.command('chat', 'work')
            before = state(h)
            workers = [r for r in before['runs'] if r['kind'] == 'worker']
            require(len(workers) == 1 and workers[0]['status'] == 'waiting', 'actual final response escaped consent')
            worker = workers[0]
            require(json.loads(worker['checkpoint_json'])['phase'] == 'reply', 'final result was not checkpointed')
            require(len(before['spend_approval']) == 1 and before['spend_reservation'][0]['actual_micros'] == FIXTURE['expected_overrun_micros'], 'actual crossing not durably aggregated')
            decide(h, monetary(h)[0])
            h.command('chat', 'work')
            finish_main(h)
            after = state(h)
            require(sum(g['run_id'] == worker['run_id'] for g in after['generations']) == 1, 'final response was regenerated after approval')
            require(next(r for r in after['runs'] if r['run_id'] == worker['run_id'])['status'] == 'completed', 'frozen final result did not resume')
            return {'name': 'actual_spend_holds_final_reply_and_reuses_result', 'before': before, 'after': after}
        report['checks'].append(scenario('actual-final', actual_final))

        def effect(h):
            h.send('effect', 'Delegate: Child approval')
            policy(h, boundary=1)
            h.command('chat', 'work')
            spend = monetary(h)[0]
            with Terminal(h.args('tui'), h.env, h.root) as terminal:
                terminal.expect_all(('You> ', 'Account spend approval required:'))
                terminal.send('/approve ' + spend['approval_id'] + ' ' + spend['frozen_arguments_hash'])
                terminal.expect('Approval decision saved.')
                terminal.expect('Approval required for mcp__fixture__send:')
                require(terminal.stop() == 0, 'monetary TUI cleanup')
                transcript = terminal.transcript()
            approvals = h.command('approval', 'list')
            effects = [a for a in approvals if a['tool'] != 'account_spend' and a['status'] == 'pending']
            require(len(effects) == 1 and not tool_state(h)['effects'], 'batch spend grant authorized Child effect')
            require(effects[0]['approval_id'] != spend['approval_id'], 'effect reused spend approval')
            before = state(h)
            owner = next(r for r in before['runs'] if r['run_id'] == before['spend_approval'][0]['root_run_id'])
            require(owner['run_id'] not in transcript and owner['model_snapshot_id'] not in transcript, 'ordinary spend notice exposed Run/snapshot identity')
            # Real kill at the existing shared pipeline's pre-effect boundary.
            decide(h, effects[0])
            child = h.hold('after_provider_effect')
            child.send_signal(signal.SIGKILL)
            child.communicate(timeout=10)
            h.command('chat', 'work')
            finish_main(h)
            after = state(h)
            require(len(tool_state(h)['effects']) == 1, 'approved Child effect not exactly once')
            return {'name': 'monetary_grant_excludes_effect_kill_restart', 'transcript': transcript, 'before': before, 'after': after}
        report['checks'].append(scenario('effect', effect))

        def denied(h):
            h.send('denied', 'Delegate: Child approval')
            policy(h, boundary=1)
            h.command('chat', 'work')
            approval = monetary(h)[0]
            before = state(h)
            decide(h, approval, 'deny')
            h.command('chat', 'work')
            finish_main(h)
            after = state(h)
            workers = [r for r in after['runs'] if r['kind'] == 'worker']
            require(len(workers) == 1 and workers[0]['status'] == 'failed' and workers[0]['error_code'] == 'SPEND_AUTHORIZATION_DENIED', 'denial dispatch or status')
            worker_ids = {r['run_id'] for r in workers}
            worker_gens = {g['generation_id'] for g in after['generations'] if g['run_id'] in worker_ids}
            require(all(g['status'] == 'failed' and g['error_code'] == 'SPEND_AUTHORIZATION_DENIED' for g in after['generations'] if g['run_id'] in worker_ids), 'denied model ledger stayed pending')
            require(not tool_state(h)['effects'] and not any(v['generation_id'] in worker_gens for v in after['spend_reservation']), 'denied model/tool dispatched')
            return {'name': 'deny_no_model_or_effect_dispatch', 'before': before, 'after': after}
        report['checks'].append(scenario('denied', denied))

        def cancelled(h):
            h.send('pending-cancel', 'Delegate: Child approval')
            policy(h, boundary=1)
            h.command('chat', 'work')
            approval = monetary(h)[0]
            before = state(h)
            h.send('cancel', 'Cancel: Child approval')
            for _ in range(5):
                h.command('chat', 'work')
                value = state(h)
                if value['tasks'][0]['status'] == 'cancelled':
                    break
                runs = {r['run_id']: r for r in value['runs']}
                mains = [a for a in value['spend_approval'] if a['status'] == 'pending' and runs[a['root_run_id']]['kind'] == 'main']
                require(len(mains) == 1, 'cancel Main missing spend consent')
                h.command('approval', 'approve', mains[0]['approval_id'], mains[0]['frozen_hash'])
            else:
                raise RuntimeError('Main did not cancel pending batch')
            finish_main(h)
            after = state(h)
            old = next(a for a in after['spend_approval'] if a['approval_id'] == approval['approval_id'])
            require(old['status'] == 'invalidated', 'cancel retained spend approval')
            require(h.command('approval', 'approve', approval['approval_id'], approval['frozen_arguments_hash'], ok=False).returncode != 0, 'old epoch approval accepted')
            require(not tool_state(h)['effects'], 'cancel dispatched effect')
            require(all(r['status'] == 'cancelled' for r in after['runs'] if r['kind'] == 'worker'), 'cancel left Worker live')
            h.command('chat', 'work')
            require(state(h) == after, 'cancel replay changed settled state')
            return {'name': 'main_cancel_invalidates_pending_root_approval', 'before': before, 'after': after}
        report['checks'].append(scenario('cancelled', cancelled))

        def reserved_restart(h):
            h.send('reserve-restart', 'Delegate: Child regular')
            policy(h, boundary=1000000000)
            child = h.hold('before_worker_generation')
            before = state(h)
            require(len(before['spend_reservation']) == 1 and before['spend_reservation'][0]['status'] == 'reserved', 'dispatch lacked durable reservation')
            reservation = before['spend_reservation'][0]
            require(not any(u['generation_id'] == reservation['generation_id'] for u in before['priced_usage']), 'pre-dispatch usage appeared')
            child.send_signal(signal.SIGKILL)
            child.communicate(timeout=10)
            h.command('chat', 'work')
            after = state(h)
            recovered = [v for v in after['spend_reservation'] if v['generation_id'] == reservation['generation_id']]
            require(len(recovered) == 1 and recovered[0]['status'] == 'priced' and recovered[0]['actual_micros'] == FIXTURE['expected_call_micros'], 'restart duplicated or lost reservation')
            require(sum(u['generation_id'] == reservation['generation_id'] for u in after['priced_usage']) == 1, 'restart duplicated usage')
            require(after['tasks'][0]['status'] == 'completed', 'reserved model did not recover')
            return {'name': 'pre_dispatch_reservation_kill_restart', 'before': before, 'after': after}
        report['checks'].append(scenario('reserved-restart', reserved_restart))

        for mode in ('unpriced', 'unconfigured', 'missing_usage'):
            def unknown(h, mode=mode):
                h.send(mode, 'Delegate: Child regular')
                policy(h, boundary=0 if mode == 'unconfigured' else 1000000000, priced=mode != 'unpriced')
                if mode == 'missing_usage':
                    # Existing unpriced-model Fake deliberately omits usage.
                    # A separate explicitly priced entry is added before Root
                    # creation; no real provider values are inferred.
                    config = json.loads((h.root / 'tool-config.yaml').read_text())
                    config['spend']['accounts'][0]['prices'][0]['model'] = 'fixture-model'
                    (h.root / 'tool-config.yaml').write_text(json.dumps(config))
                    setup(h)
                h.command('chat', 'work')
                value = state(h)
                require(not monetary(h), 'unknown configuration guessed approval')
                if mode == 'missing_usage':
                    require(any(v['status'] == 'usage_missing' and v['actual_micros'] is None and v['reserved_micros'] > 0 for v in value['spend_reservation']), 'missing usage became zero')
                else:
                    require(any(v['state'] == mode for v in value['spend_status']), 'missing explicit sanitized state')
                return {'name': mode, 'state': value}
            report['checks'].append(scenario(mode, unknown))
    report['cleanup'] = 'all child processes joined; isolated state roots removed'
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    result = run(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'checks': len(result['checks']), 'cleanup': result['cleanup'], 'evidence': str(args.evidence)}))
