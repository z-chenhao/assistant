#!/usr/bin/env python3
"""M3 stdio MCP, progressive Skills and actual controlled coding CLI proof."""
import argparse
from contextlib import closing
import hashlib
import json
import os
import shutil
import signal
import sqlite3
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

from verify_main import MainHarness, require
from fixture_pool import run as fixture_run, run_cases


class CapabilityHarness(MainHarness):
    def args(self, *args):
        return [str(self.binary), '-config', str(self.root / 'config.yaml'),
                '-test-bridge', 'capabilities', *args]

    def command(self, *args, text=None, ok=True, timeout=320):
        run = self.processes.run if self.processes is not None else subprocess.run
        result = run(self.args(*args), input=text, text=True, capture_output=True,
                                env=self.env, cwd=self.root, timeout=timeout)
        if os.environ.get('ERII_CAPABILITY_DIAGNOSTICS') and (result.returncode or result.stderr):
            self.capture_diagnostic(args, result)
        require(not ok or result.returncode == 0, f'{args}: {result.stderr}')
        return json.loads(result.stdout) if ok else result

    def capture_diagnostic(self, args, result):
        # Opt-in disposable Fake fixtures only. Never copy credential stores,
        # configuration, stdin, or an arbitrary data-root directory.
        folder = Path(os.environ['ERII_CAPABILITY_DIAGNOSTICS']) / uuid.uuid4().hex
        folder.mkdir(parents=True)
        if not hasattr(self, 'diagnostic_binary_sha256'):
            self.diagnostic_binary_sha256 = digest(self.binary)
        record = dict(command=list(args), returncode=result.returncode,
                      stderr=result.stderr, binary_sha256=self.diagnostic_binary_sha256, databases={})
        if result.returncode:
            for name in ('state.db', 'conversation.db'):
                source = self.root / 'data' / name
                if source.is_file():
                    try:
                        deadline = time.monotonic() + 5
                        def progress(*_):
                            if time.monotonic() > deadline:
                                raise TimeoutError('fixture backup deadline')
                        with closing(sqlite3.connect(source.as_uri() + '?mode=ro', uri=True)) as db, \
                             closing(sqlite3.connect(folder / name)) as backup:
                            db.backup(backup, pages=128, progress=progress)
                        record['databases'][name] = digest(folder / name)
                    except (sqlite3.Error, TimeoutError) as error:
                        record['databases'][name] = {'capture_error': str(error)}
        (folder / 'command.json').write_text(json.dumps(record, indent=2) + '\n')


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree(root):
    return {str(p.relative_to(root)): digest(p) for p in sorted(root.rglob('*')) if p.is_file()}


def prepare(binary, root, mode='edit'):
    repo = root / 'repo'
    shutil.copytree(Path(__file__).resolve().parents[2] / 'tests/fixtures/coding-v1', repo)
    for args in [('init', '-q', '-b', 'feature/eval'), ('add', '.'),
                 ('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test',
                  'commit', '-qm', 'fixture')]:
        fixture_run(['git', '-c', 'maintenance.auto=false', '-c', 'gc.auto=0', *args], cwd=repo, check=True, capture_output=True,
                       env={'PATH': '/usr/bin:/bin', 'HOME': '/nonexistent',
                            'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'})
    repo.chmod(0o700)
    (repo / 'docs/notes.md').write_text('USER_UNCOMMITTED_SENTINEL\n')
    hook = repo / '.git/hooks/pre-commit'
    hook.write_text('#!/bin/sh\necho must-not-run > hook-ran\nexit 1\n')
    hook.chmod(0o700)
    skills = root / 'skills/review'
    skills.mkdir(parents=True)
    (skills / 'SKILL.md').write_text('---\nname: review\ndescription: Inspect code and preserve user files.\n---\nPRIVATE_BODY_MARKER: Read AGENTS.md and cite symbols.\n')
    (skills / 'assets').mkdir()
    (skills / 'assets/opaque.bin').write_bytes(b'\x00\xffSKILL_ASSET')
    (root / 'provider').mkdir()
    shutil.copyfile(Path(__file__).resolve().parents[2] / 'tests/fixtures/m3-capabilities-v1.json',
                    root / 'provider/mcp-data.json')
    config = ('version: 1\nmcp:\n  - name: fixture\n    command: ' + json.dumps(str(binary)) +
              '\n    directory: ' + json.dumps(str(root / 'provider')) +
              '\n    args: ["-test-mcp-server", ' + json.dumps(str(root / 'provider')) + ']\n' +
              '    tools: {read: read_only, send: external_effect}\n' +
              'skills:\n  root: ' + json.dumps(str(root / 'skills')) + '\n  scope: workspace\n' +
              'execution:\n  root: ' + json.dumps(str(repo)) + '\n  toolchain_roots: [' +
              json.dumps(str(Path(shutil.which('go')).resolve().parent.parent)) + ']\n')
    (root / 'config.yaml').write_text(config)
    h = CapabilityHarness(binary, root, fixture='capabilities')
    if os.environ.get('ERII_CAPABILITY_DIAGNOSTICS'):
        h.env['ERII_SQL_FIXTURE_DIAGNOSTIC'] = '1'
    h.setup()
    return h


def call(h, key, name, arguments, worker=False, approve=True):
    plan = json.dumps([{'name': name, 'arguments': arguments}])
    h.send(key, ('Delegate capabilities: ' if worker else 'Capabilities: ') + plan)
    h.command('chat', 'work')
    pending = [a for a in h.command('approval', 'list') if a['status'] == 'pending']
    if pending:
        require(len(pending) == 1, 'unexpected pending approvals')
        a = pending[0]
        before = h.state()
        require(all(t['attempt_count'] == 0 for t in before['tools'] if t['status'] == 'waiting'),
                'dispatch before exact approval')
        h.command('approval', 'approve' if approve else 'deny', a['approval_id'], a['frozen_arguments_hash'])
        h.command('chat', 'work')
    return h.state()['tools'][-1]


def content(invocation):
    return json.loads(invocation['result_json'])


def run(binary, selected=None, fixture_context=None, scenario_jobs=1):
    report = {'version': 1, 'scope': 'MCP/Skill and native file/command contracts; approvals, isolated persistent execution, CAS and unknown effects',
              'checks': [], 'crashes': [], 'cleanup': False, 'scenario_jobs': scenario_jobs}
    with tempfile.TemporaryDirectory(prefix='erii-capabilities-') as temp:
        root = Path(temp)
        cases = []
        def scenario(name, fn, mode='edit', kind='checks'):
            if not selected or selected == name or selected == 'quotes' and name.startswith('quotes-'):
                cases.append((name, fn, mode, kind))

        def drive(item):
            name, fn, mode, kind = item
            folder = root / name
            folder.mkdir()
            h = prepare(binary, folder, mode)
            try:
                if fixture_context:
                    h.send('fixture-context', fixture_context)
                return kind, fn(h)
            finally:
                h.cleanup()

        def coding(h):
            repo = h.root / 'repo'
            original = tree(repo)
            call(h, 'rules', 'builtin__read', {'path': 'AGENTS.md'})
            before = call(h, 'red', 'builtin__bash', {'command': 'go test -race ./...', 'timeout': 120})
            observed = json.loads(content(before)['text'])
            require(observed['exit_code'] != 0 and 'flag provided but not defined' in observed['output'], 'fixture did not reproduce missing --status: ' + str(observed))
            path = 'cmd/taskctl/main.go'
            source = json.loads(content(call(h, 'source', 'builtin__read', {'path': path}))['text'])
            replacement = source['text'].replace('status := "" // CLI_STATUS_FLAG',
                                                'status := ""\n\tflags.StringVar(&status, "status", "", "Filter Task status")')
            edited = call(h, 'edit', 'builtin__edit',
                          {'path': path, 'before_hash': source['hash'], 'old_text': 'status := "" // CLI_STATUS_FLAG', 'new_text': 'status := ""\n\tflags.StringVar(&status, "status", "", "Filter Task status")'}, worker=True)
            require(not content(edited).get('error') and (repo / path).read_text() == replacement, 'approved Worker edit failed')
            test_path = 'cmd/taskctl/main_test.go'
            source_test = json.loads(content(call(h, 'test-source', 'builtin__read', {'path': test_path}))['text'])
            edited_test = call(h, 'test-edit', 'builtin__edit', {'path': test_path, 'before_hash': source_test['hash'],
                               'old_text': 'package main', 'new_text': 'package main\n// verification retains existing tests'}, worker=True)
            require(not content(edited_test).get('error'), 'multi-file edit failed')
            tested = call(h, 'green', 'builtin__bash', {'command': 'go test -race ./...', 'timeout': 120})
            passed = json.loads(content(tested)['text'])
            require(passed['exit_code'] == 0 and not passed['timed_out'], 'real sandbox tests failed: ' + str(passed))
            # Git is a regular approved command; explicit no-refresh/index scope
            # avoids treating index maintenance as a source-file modification.
            diff = json.loads(content(call(h, 'diff', 'builtin__bash', {'command': 'git --no-optional-locks diff --no-ext-diff'}))['text'])
            require('flags.StringVar' in diff['output'], 'Git diff omitted edit')
            published = content(call(h, 'deliver', 'builtin__read', {'path': path, 'as_artifact': True}))
            artifact = fixture_run(h.args('artifact', 'open', published['artifact_refs'][0]), capture_output=True, env=h.env, cwd=h.root, timeout=20)
            require(artifact.returncode == 0 and artifact.stdout == replacement.encode(), 'native read did not retain file as Artifact')
            final = tree(repo)
            changed = [p for p in original if original[p] != final.get(p)]
            require(sorted(changed) == sorted([path, test_path]), 'unrelated source changes: ' + str(changed))
            require(not (repo / 'hook-ran').exists(), 'reading diff executed hook')
            return {'name': 'real_red_edit_green_with_Worker_approval', 'changed': changed, 'before': observed, 'after': passed, 'state': h.state()}
        scenario('coding', coding)

        def dirty_index(h):
            repo = h.root / 'repo'
            path = 'docs/notes.md'
            source = json.loads(content(call(h, 'read-dirty', 'builtin__read', {'path': path}))['text'])
            first = call(h, 'first-edit', 'builtin__edit', {'path': path, 'before_hash': source['hash'],
                         'old_text': 'SENTINEL', 'new_text': 'SENTINEL + FIRST'}, worker=True)
            require(not content(first).get('error'), 'dirty-file edit rejected')
            current = json.loads(content(call(h, 'read-first', 'builtin__read', {'path': path}))['text'])
            second = call(h, 'second-edit', 'builtin__edit', {'path': path, 'before_hash': current['hash'],
                          'old_text': 'FIRST', 'new_text': 'SECOND'}, worker=True)
            require(not content(second).get('error') and (repo / path).read_text() == 'USER_UNCOMMITTED_SENTINEL + SECOND\n', 'continuous edits lost user content')
            stale = call(h, 'stale-edit', 'builtin__edit', {'path': path, 'before_hash': source['hash'],
                         'old_text': 'SECOND', 'new_text': 'OVERWRITE'}, worker=True)
            require(content(stale)['error']['code'] == 'PERMISSION_DENIED', 'stale preimage overwrote changes')
            created = call(h, 'create', 'builtin__write', {'path': 'output/result.txt', 'content': 'created'}, worker=True)
            require(not content(created).get('error'), 'file creation failed')
            denied = call(h, 'overwrite', 'builtin__write', {'path': 'output/result.txt', 'content': 'blind'})
            require(content(denied)['error']['code'] == 'PERMISSION_DENIED', 'blind overwrite allowed')
            return {'name': 'continuous_dirty_edits_create_and_stale_hash_refusal', 'state': h.state()}
        scenario('dirty-index', dirty_index)

        def audit(h):
            repo = h.root / 'repo'
            before = tree(repo)
            read = content(call(h, 'read', 'builtin__read', {'path': 'internal/retry/executor.go'}))
            text = json.loads(read['text'])['text']
            require('provider.Execute()' in text and 'ledger.Lookup()' in text, 'read lacks call-order evidence')
            denied = call(h, 'denied-command', 'builtin__bash', {'command': 'touch forbidden'}, approve=False)
            require(content(denied)['error']['code'] == 'APPROVAL_DENIED' and tree(repo) == before, 'unapproved bash modified workspace')
            traversal = content(call(h, 'traversal', 'builtin__read', {'path': '../config.yaml'}))
            require(traversal['error']['code'] == 'PERMISSION_DENIED', 'file traversal escaped workspace')
            return {'name': 'reads_automatic_mutations_require_exact_approval', 'state': h.state()}
        scenario('audit', audit)

        def discovery(h):
            listed = content(call(h, 'skills', 'builtin__skill_list', {}, worker=True))
            summaries = json.loads(listed['text'])
            require(len(summaries) == 1 and 'PRIVATE_BODY_MARKER' not in listed['text'], 'Skill list exposed body')
            viewed = content(call(h, 'view', 'builtin__skill_view', {'name': 'review', 'version': summaries[0]['version']}))
            require('PRIVATE_BODY_MARKER' in viewed['text'], 'Skill view missing actual body')
            asset = content(call(h, 'asset', 'builtin__skill_view',
                                 {'name': 'review', 'version': summaries[0]['version'], 'path': 'assets/opaque.bin'}))
            opened = fixture_run(h.args('artifact', 'open', asset['artifact_refs'][0]), capture_output=True,
                                    env=h.env, cwd=h.root, timeout=20)
            require(opened.returncode == 0 and opened.stdout == b'\x00\xffSKILL_ASSET',
                    'binary Skill asset was corrupted or bypassed ArtifactStore')
            denied = content(call(h, 'manage', 'builtin__skill_manage', {}, worker=True))
            require(denied['error']['code'] == 'NOT_FOUND', 'ordinary Worker got Skill management')
            denied = content(call(h, 'task-control', 'builtin__task_manage', {'operation': 'cancel', 'taskRef': 'forged'}, worker=True))
            require(denied['error']['code'] == 'NOT_FOUND', 'Worker gained Task control')
            read = content(call(h, 'mcp-read', 'mcp__fixture__read', {'text': 'structured fixture response'}, worker=True))
            require(read['text'] == 'structured fixture response', 'stdio response lost')
            rejected = content(call(h, 'forged', 'mcp__fixture__read', {'text': 'x', 'call_key': 'forged'}))
            require(rejected['error']['code'] == 'INVALID_ARGUMENT', 'malformed MCP parameters dispatched')
            send = call(h, 'deny', 'mcp__fixture__send', {'text': 'must not send'}, approve=False)
            require(content(send)['error']['code'] == 'APPROVAL_DENIED' and not (h.root / 'provider/mcp-effects.jsonl').exists(), 'denied MCP dispatched')
            unknown = call(h, 'unknown', 'mcp__fixture__send', {'text': 'one order', 'behavior': 'unknown'})
            require(unknown['status'] == 'unknown' and len((h.root / 'provider/mcp-effects.jsonl').read_text().splitlines()) == 1,
                    'ambiguous MCP lost uncertainty')
            h.command('chat', 'work')
            require(len((h.root / 'provider/mcp-effects.jsonl').read_text().splitlines()) == 1, 'unknown MCP automatically retried')
            value = h.state()
            require(all(c['skill_set_hash'] != hashlib.sha256(b'[]').hexdigest() for c in value['capabilities']), 'Skill set not frozen')
            serialized = json.dumps(value['generations'][0])
            require('Inspect code and preserve user files.' in serialized and 'PRIVATE_BODY_MARKER' not in serialized,
                    'startup Skill disclosure is not progressive')
            return {'name': 'MCP_transport_Skill_scope_role_denial_unknown', 'state': value}
        scenario('discovery', discovery)

        def malformed_mcp(h):
            provider = h.root / 'provider'
            observations = []
            # The real stdio Fake records an effect before emitting each
            # malformed reply. Every CLI command opens a new Host process.
            for name, reply in [('null', None), ('missing-text', {'content': [{'type': 'text'}]})]:
                (provider / 'mcp-result.json').write_text(json.dumps(reply))
                read = call(h, 'read-' + name, 'mcp__fixture__read', {'text': 'read'})
                require(read['status'] == 'failed' and content(read)['error']['code'] == 'PROVIDER_UNAVAILABLE',
                        'malformed read reply accepted')
                invocation = call(h, 'send-' + name, 'mcp__fixture__send', {'text': name}, worker=True)
                require(invocation['status'] == 'unknown' and invocation['attempt_count'] == 1 and
                        invocation['result_json'] is None, 'malformed effect reply claimed completion')
                require('UNKNOWN_SIDE_EFFECT' in json.dumps(h.command('chat', 'history')[-1]),
                        'side-effect uncertainty was not delivered')
                effects = (provider / 'mcp-effects.jsonl').read_bytes()
                require(len(effects.splitlines()) == len(observations) + 1, 'effect missing or repeated')
                h.command('approval', 'reconcile')
                h.command('chat', 'work')
                current = next(t for t in h.state()['tools'] if t['invocation_id'] == invocation['invocation_id'])
                require(current['status'] == 'unknown' and current['attempt_count'] == 1 and
                        (provider / 'mcp-effects.jsonl').read_bytes() == effects,
                        'malformed effect lost uncertainty or retried after restart/reconcile')
                observations.append({'reply': reply, 'invocation': current, 'effect_count': len(effects.splitlines())})
            return {'name': 'malformed_stdio_effect_replies_stay_unknown', 'cases': observations, 'state': h.state()}
        scenario('mcp-reply', malformed_mcp)

        def crash(h):
            repo = h.root / 'repo'
            path = 'cmd/taskctl/main.go'
            source = json.loads(content(call(h, 'source', 'builtin__read', {'path': path}))['text'])
            replacement = source['text'].replace('package main', 'package main\n// accepted edit', 1)
            plan = [{'name': 'builtin__edit', 'arguments': {'path': path, 'before_hash': source['hash'],
                     'old_text': 'package main', 'new_text': 'package main\n// accepted edit'}}]
            h.send('crash-edit', 'Capabilities: ' + json.dumps(plan))
            h.command('chat', 'work')
            a = [a for a in h.command('approval', 'list') if a['status'] == 'pending'][0]
            h.command('approval', 'approve', a['approval_id'], a['frozen_arguments_hash'])
            child = h.hold('before_tool_commit')
            child.send_signal(signal.SIGKILL)
            child.communicate(timeout=10)
            require((repo / path).read_text() == replacement, 'crash not after actual edit')
            h.command('chat', 'work')
            value = h.state()
            inv = [t for t in value['tools'] if t['tool_name'] == 'builtin__edit'][0]
            require(inv['status'] == 'unknown' and inv['attempt_count'] == 1 and (repo / path).read_text() == replacement,
                    'crashed edit retried or falsely completed')
            return {'name': 'edit_after_effect_before_result_commit', 'signal': 'SIGKILL', 'state': value}
        scenario('crash', crash, kind='crashes')
        for kind, result in run_cases(cases, drive, scenario_jobs):
            report[kind].append(result)
        if not selected:
            expected = {'real_red_edit_green_with_Worker_approval',
                        'continuous_dirty_edits_create_and_stale_hash_refusal',
                        'reads_automatic_mutations_require_exact_approval',
                        'MCP_transport_Skill_scope_role_denial_unknown',
                        'malformed_stdio_effect_replies_stay_unknown'}
            require(len(report['checks']) == len(expected) and
                    {row['name'] for row in report['checks']} == expected and
                    len(report['crashes']) == 1 and
                    report['crashes'][0]['name'] == 'edit_after_effect_before_result_commit',
                    'complete software capability scenario inventory required')
    report['cleanup'] = not root.exists()
    require(report['cleanup'], 'disposable roots remain')
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--scenario', choices=['coding', 'dirty-index', 'audit', 'quotes', 'discovery', 'mcp-reply', 'crash'],
                        help='Run one maintained scenario; omit for the complete gate')
    parser.add_argument("--scenario-jobs",type=int,choices=[1,2,4],default=int(os.environ.get("ERII_VERIFY_CASE_JOBS","1")))
    args = parser.parse_args()
    report = run(args.binary.resolve(), args.scenario, scenario_jobs=args.scenario_jobs)
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'checks': len(report['checks']), 'crashes': len(report['crashes']), 'cleanup': report['cleanup']}))
