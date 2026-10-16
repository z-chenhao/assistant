#!/usr/bin/env python3
"""Unified Tool CLI launch/doctor/drive/evidence/cleanup and crash boundaries."""
import argparse
import json
from pathlib import Path
import signal
import sqlite3
import tempfile

from verify_main import MainHarness, require


class ToolHarness(MainHarness):
    def args(self, *args):
        config = self.root / 'tool-config.yaml'
        prefix = ['-config', str(config)] if config.exists() else []
        return [str(self.binary), *prefix, '-test-bridge', self.fixture, *args]


def state(h):
    value = h.state()
    root = Path(h.env['ERII_DATA_ROOT'])
    with sqlite3.connect(root / 'state.db') as db:
        db.row_factory = sqlite3.Row
        for name, query in {
            'approvals': 'SELECT * FROM approval ORDER BY created_at',
            'invocations': 'SELECT * FROM tool_invocation ORDER BY created_at,delivery_seq',
            'claims': 'SELECT * FROM run_claim',
            'tasks': 'SELECT * FROM task',
            'updates': 'SELECT * FROM task_update ORDER BY occurred_at',
            'policy_events': "SELECT type,payload_json FROM event_outbox WHERE type LIKE '%approval%' OR type LIKE '%tool_invocation%' ORDER BY occurred_at",
        }.items():
            value[name] = [dict(r) for r in db.execute(query)]
    value['effects'] = []
    if (root / 'effect-ledger.db').exists():
        with sqlite3.connect(root / 'effect-ledger.db') as db:
            db.row_factory = sqlite3.Row
            value['effects'] = [dict(r) for r in db.execute('SELECT * FROM effects ORDER BY key')]
    return value


def pending(h, text=None):
    h.send('effect', json.dumps(text or {'target': 'alex.chen@example.test', 'text': '2026-10-20 11:00 Erii review'}))
    h.command('chat', 'work')
    approval = h.command('approval', 'list')[-1]
    value = state(h)
    require(approval['status'] == 'pending' and not value['effects'], 'effect before approval')
    require(value['invocations'][0]['status'] == 'waiting' and value['invocations'][0]['attempt_count'] == 0,
            'approval acquired execution resources')
    require(all(c['token'] is None for c in value['claims']), 'waiting retained Run lease')
    return approval


def decide(h, approval, decision='approve'):
    return h.command('approval', decision, approval['approval_id'], approval['frozen_arguments_hash'])


def run(binary):
    report = {'version': 1, 'scope': 'SC-06/07 and GC-11/12/13 Tool approval/recovery fragments; full product cases remain due with their integrations',
              'checks': [], 'crashes': []}
    with tempfile.TemporaryDirectory(prefix='erii-tools-') as temp:
        root = Path(temp)

        def scenario(name, action, fixture='approval'):
            folder = root / name
            folder.mkdir()
            h = ToolHarness(binary, folder, fixture=fixture)
            if name == "expired":
                (folder / "tool-config.yaml").write_text("version: 1\ntools:\n  approval_ttl_seconds: 1\n")
            try:
                h.setup()
                return action(h)
            finally:
                h.cleanup()

        def approved(h):
            approval = pending(h)
            before = state(h)
            require(h.command('approval', 'approve', approval['approval_id'], 'wrong', ok=False).returncode != 0,
                    'changed frozen hash accepted')
            decide(h, approval)
            decide(h, approval)
            h.command('chat', 'work')
            decide(h, approval)  # delayed duplicate after terminal Run
            h.command('chat', 'work')
            after = state(h)
            require(len(after['effects']) == 1 and after['invocations'][0]['status'] == 'succeeded', 'approved effect count/status')
            require(after['invocations'][0]['arguments_json'] == before['invocations'][0]['arguments_json'] and
                    after['invocations'][0]['idempotency_key'] == before['invocations'][0]['idempotency_key'], 'frozen admission drift')
            return {'name': 'approve_restart_duplicate_and_wrong_hash', 'before': before, 'after': after}
        report['checks'].append(scenario('approved', approved))

        def denied(h):
            approval = pending(h)
            decide(h, approval, 'deny')
            h.command('chat', 'work')
            value = state(h)
            require(not value['effects'] and value['invocations'][0]['status'] == 'failed', 'denied effect')
            require(json.loads(value['invocations'][0]['error_json'])['code'] == 'APPROVAL_DENIED', 'denial error')
            return {'name': 'denied_no_dispatch', 'state': value}
        report['checks'].append(scenario('denied', denied))

        def expired(h):
            approval = pending(h)
            h.wait_state(lambda views: views[-1]['status'] == 'expired', 'approval expiry',
                         reader=lambda h: h.command('approval', 'list'))
            require(h.command('approval', 'approve', approval['approval_id'], approval['frozen_arguments_hash'], ok=False).returncode != 0,
                    'expired callback accepted')
            h.command('chat', 'work')
            value = state(h)
            require(not value['effects'] and value['invocations'][0]['status'] == 'failed' and
                    value['invocations'][0]['attempt_count'] == 0, 'expired approval dispatched')
            return {'name': 'expired_no_dispatch', 'state': value}
        report['checks'].append(scenario('expired', expired))

        def ambiguous(h):
            approval = pending(h, {'target': 'R1', 'text': 'Saturday 19:00 party 2 no deposit', 'behavior': 'ambiguous'})
            decide(h, approval)
            h.command('chat', 'work')
            observations = h.command('approval', 'reconcile')
            require(observations[-1]['status'] == 'succeeded' and observations[-1]['result']['text'], 'CLI reconciliation omitted receipt')
            h.command('chat', 'work')
            value = state(h)
            require(len(value['effects']) == 1 and value['invocations'][0]['status'] == 'succeeded' and
                    value['invocations'][0]['reconciliation_json'] is not None and value['invocations'][0]['attempt_count'] == 1,
                    'ambiguous accepted order retried or not reconciled')
            return {'name': 'GC-13_ambiguous_accepted_effect_reconciled_without_retry', 'state': value}
        report['checks'].append(scenario('ambiguous', ambiguous))

        def steering(h):
            approval = pending(h, {'target': 'alex.chen@example.test', 'text': 'Monday'})
            decide(h, approval)
            h.send('steering', json.dumps({'target': 'alex.chen@example.test', 'text': 'Tuesday'}))
            require(h.command('approval', 'approve', approval['approval_id'], approval['frozen_arguments_hash'], ok=False).returncode != 0,
                    'stale approval accepted')
            h.command('chat', 'work')
            newer = h.command('approval', 'list')[-1]
            require(newer['status'] == 'pending' and newer['approval_id'] != approval['approval_id'] and 'Tuesday' in newer['arguments'],
                    'new arguments lack distinct approval')
            require(not state(h)['effects'], 'invalidated approved invocation executed')
            decide(h, newer)
            h.command('chat', 'work')
            value = state(h)
            require(len(value['effects']) == 1 and 'Tuesday' in value['effects'][0]['args'], 'wrong frozen effect')
            return {'name': 'GC-12_steering_invalidates_approved_parameters', 'state': value}
        report['checks'].append(scenario('steering', steering))

        def worker(h):
            receipt = h.send('delegate', 'Delegate effect: ' + json.dumps({'target': 'R1', 'text': 'reservation', 'behavior': 'unknown'}))
            h.command('chat', 'work')
            approval = h.command('approval', 'list')[-1]
            before = state(h)
            inv = next(i for i in before['invocations'] if i['tool_name'] == 'mcp__fixture__send')
            require(inv['run_id'] != receipt['run_id'] and inv['status'] == 'waiting' and not before['updates'], 'Worker approval leaked to Main or ran without independent decision')
            decide(h, approval)
            h.command('chat', 'work')
            after = state(h)
            require(after['tasks'][0]['status'] == 'active' and len(after['updates']) == 1 and
                    after['updates'][0]['kind'] == 'unknown' and not after['effects'], 'unknown Worker falsely completed Task')
            return {'name': 'Worker_approval_and_unknown_report_to_Main', 'before': before, 'after': after}
        report['checks'].append(scenario('worker', worker))

        def cancel_waiter(h):
            h.send('delegate', 'Delegate effect: ' + json.dumps({'target': 'Alex', 'text': 'draft'}))
            h.command('chat', 'work')
            approval = h.command('approval', 'list')[-1]
            h.send('cancel', 'Cancel: Effect fixture')
            h.command('chat', 'work')
            value = state(h)
            inv = next(i for i in value['invocations'] if i['tool_name'] == 'mcp__fixture__send')
            require(inv['status'] == 'cancelled' and inv['attempt_count'] == 0 and not value['effects'], 'cancelled approval waiter dispatched')
            require(h.command('approval', 'approve', approval['approval_id'], approval['frozen_arguments_hash'], ok=False).returncode != 0, 'cancelled approval accepted')
            return {'name': 'Worker_cancel_during_approval_no_dispatch', 'state': value}
        report['checks'].append(scenario('cancel-waiter', cancel_waiter))

        def uncertain(h):
            approval = pending(h, {'target': 'R1', 'text': 'Saturday 19:00 party 2 no deposit', 'behavior': 'unknown'})
            decide(h, approval)
            h.command('chat', 'work')
            observations = h.command('approval', 'reconcile')
            require(observations[-1]['status'] == 'unknown' and observations[-1]['result']['error']['code'] == 'UNKNOWN_SIDE_EFFECT', 'CLI hid unknown outcome')
            h.command('chat', 'work')
            value = state(h)
            require(value['invocations'][0]['status'] == 'unknown' and value['invocations'][0]['attempt_count'] == 1 and not value['effects'],
                    'unknown blindly retried or lost')
            return {'name': 'unknown_without_evidence_retained', 'state': value}
        report['checks'].append(scenario('unknown', uncertain))

        def transient(h):
            approval = pending(h, {'target': 'Alex', 'text': 'draft', 'behavior': 'transient'})
            decide(h, approval)
            h.command('chat', 'work')
            value = state(h)
            inv = value['invocations'][0]
            require(inv['attempt_count'] == 3 and inv['status'] == 'failed' and not value['effects'], 'retry bound')
            require(json.loads(inv['error_json']) == {'code': 'PROVIDER_UNAVAILABLE', 'message': 'PROVIDER_UNAVAILABLE', 'retryable': True}, 'raw provider error escaped')
            return {'name': 'bounded_transient_retries_and_safe_errors', 'state': value}
        report['checks'].append(scenario('transient', transient))

        for policy in ('query_before_retry', 'never'):
            for boundary in ('before_provider_effect', 'after_provider_effect', 'before_tool_commit', 'after_tool_commit'):
                def crash(h, boundary=boundary, policy=policy):
                    approval = pending(h)
                    decide(h, approval)
                    child = h.hold(boundary)
                    child.send_signal(signal.SIGKILL)
                    child.communicate(timeout=10)
                    before = state(h)
                    h.command('chat', 'work')
                    after = state(h)
                    expected = 0 if policy == 'never' and boundary == 'before_provider_effect' else 1
                    require(len(after['effects']) == expected, 'crash duplicated or lost a permitted effect')
                    inv = after['invocations'][0]
                    want = 'unknown' if expected == 0 else 'succeeded'
                    require(inv['status'] == want, 'crash outcome policy')
                    require(inv['idempotency_key'] == before['invocations'][0]['idempotency_key'], 'restart key drift')
                    require(inv['attempt_count'] <= 2, 'unbounded replay')
                    return {'policy': policy, 'boundary': boundary, 'signal': 'SIGKILL', 'before': before, 'after': after}
                report['crashes'].append(scenario(policy + '-' + boundary, crash,
                                                  'approval-never' if policy == 'never' else 'approval'))
    report['cleanup'] = 'all child processes joined; isolated temporary roots removed'
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    report = run(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print('unified Tool approvals/recovery verified:', args.evidence)
