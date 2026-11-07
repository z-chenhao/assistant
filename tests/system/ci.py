#!/usr/bin/env python3
"""The private PR check partition; each lane owns its checkout and processes."""
import argparse
import json
import os
import sys

from runner import CORE
from verify_channels import CHANNEL_GROUPS

# Keep the public local Make targets intact. CI partitions their existing work
# across isolated runners rather than rebuilding a shared, mutable worktree.
SOFTWARE = {
    'go': 'make test-prompts test-go',
    'connected-apps': 'make test-connected-apps',
    'provider': 'npm test --prefix provider-bridge',
    'frontend': 'npm run check --prefix web\nnpm test --prefix web\nnpm test --prefix runtime/javascript',
    'browser-infra': 'make test-browser test-infra',
    'lint': 'make lint',
}
SYSTEM = {
    'main-tasks': ('main', 'tasks', 'children'),
    'tools-provider': ('tools', 'artifacts', 'code-recovery', 'local-provider', 'account'),
    'startup-ui': ('startup', 'intelligent-ui', 'ui-interactions'),
}
MEMORY_GATES = ('memory-maintenance', 'recall')
CAPABILITIES = ('task-code-environments', 'task-code-recovery', 'tool-loading', 'main-business')
RACE = "go test -race ./internal/agent ./internal/agent/sqlite ./internal/interaction/web ./internal/llm/bridge ./internal/app ./internal/llm/secret -run 'Test(MainQueuesSecondRequest|SerialMain|Recall.*|ConversationalRecall.*|.*UI.*|ReplyStream.*|InboundAdmission|Files|AccountFiles|.*Refresh.*)' -count=1"


def plan(kind):
    names = SOFTWARE if kind == 'software' else (*SYSTEM, *('channels-' + group for group in CHANNEL_GROUPS), 'capabilities-race', 'memory-context')
    return dict(include=[dict(lane=name,
        frontend=str(name in ('frontend', 'browser-infra', 'lint')).lower(),
        browser=str(name in ('browser-infra', 'lint', 'tools-provider', 'startup-ui') or name.startswith('channels-')).lower())
        for name in names])


def command(lane):
    if lane in SOFTWARE:
        return SOFTWARE[lane]
    if lane in SYSTEM:
        gates = ' '.join('--gate ' + gate for gate in SYSTEM[lane])
        return f'make build\npython3 tests/system/runner.py {gates} --binary bin/steward --evidence evidence/e2e-{lane}'
    if lane.startswith('channels-') and lane.removeprefix('channels-') in CHANNEL_GROUPS:
        group = lane.removeprefix('channels-')
        return f'make build\npython3 tests/system/verify_channels.py --group {group} --binary bin/steward --evidence evidence/{lane}/report.json'
    if lane == 'memory-context':
        gates = ' '.join('--gate ' + gate for gate in MEMORY_GATES)
        return f'make test-memory build\npython3 tests/system/runner.py {gates} --binary bin/steward --evidence evidence/e2e-{lane}'
    if lane == 'capabilities-race':
        commands = ['make build', 'make test-web-access', RACE]
        commands.extend(f'python3 tests/system/verify_capabilities.py --binary bin/steward --scenario {scenario} --evidence evidence/{scenario}.json'
                        for scenario in CAPABILITIES)
        return '\n'.join(commands)
    raise ValueError('unknown CI lane: ' + lane)


def passed(needs):
    # A missing/cancelled matrix is never green. Only the explicit docs path may
    # skip code, and scope must itself have completed successfully.
    if set(needs) != {'scope', 'software', 'system'} or needs['scope'].get('result') != 'success':
        return False
    docs = needs['scope'].get('outputs', {}).get('docs_only')
    if docs not in ('true', 'false'):
        return False
    expected = 'skipped' if docs == 'true' else 'success'
    return all(needs[name].get('result') == expected for name in ('software', 'system'))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    action = parser.add_mutually_exclusive_group(required=True)
    action.add_argument('--plan', choices=('software', 'system'))
    action.add_argument('--lane')
    action.add_argument('--results', help='GitHub needs JSON for the required aggregate check')
    args = parser.parse_args()
    gates = [gate for group in SYSTEM.values() for gate in group]
    if len(gates) != len(set(gates)) or set(gates) != set(CORE) - {'channels'}:
        parser.error('CI must run every core gate exactly once')
    if args.plan:
        print(json.dumps(plan(args.plan)))
        return 0
    if args.results is not None:
        return 0 if passed(json.loads(args.results)) else 1
    shell = command(args.lane)
    # Reuse the existing cancellation owner and source-stability evidence.
    os.execv(sys.executable, [sys.executable, 'tests/system/process_step.py',
        '--name', args.lane, '--phase', 'test', '--', 'bash', '-eu', '-c', shell])


if __name__ == '__main__':
    raise SystemExit(main())
