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
    shutil.copytree(Path(__file__).resolve().parents[1] / 'evals/fixtures/coding-v1', repo)
    for args in [('init', '-q', '-b', 'feature/eval'), ('add', '.'),
                 ('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test',
                  'commit', '-qm', 'fixture')]:
        fixture_run(['git', '-c', 'maintenance.auto=false', '-c', 'gc.auto=0', *args], cwd=repo, check=True, capture_output=True,
                       env={'PATH': '/usr/bin:/bin', 'HOME': '/nonexistent',
                            'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'})
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
    shutil.copyfile(Path(__file__).resolve().parents[1] / 'evals/fixtures/m3-capabilities-v1.json',
                    root / 'provider/mcp-data.json')
    config = ('version: 1\nmcp:\n  - name: fixture\n    command: ' + json.dumps(str(binary)) +
              '\n    directory: ' + json.dumps(str(root / 'provider')) +
              '\n    args: ["-test-mcp-server", ' + json.dumps(str(root / 'provider')) + ']\n' +
              '    tools: {read: read_only, send: external_effect}\n' +
              'skills:\n  root: ' + json.dumps(str(root / 'skills')) + '\n  scope: workspace\n' +
              'workspace:\n  root: ' + json.dumps(str(repo)) + '\n  mode: ' + mode +
              '\n  protected: [docs/notes.md]\n')
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
    report = {'version': 1, 'scope': 'M3 MCP/Skill/coding contracts; GC-19 prerequisites, GC-20 coding boundary only; M4 Child and M5 Durable Replay remain not_applicable',
              'checks': [], 'crashes': [], 'cleanup': False, 'scenario_jobs': scenario_jobs}
    with tempfile.TemporaryDirectory(prefix='erii-capabilities-') as temp:
        root = Path(temp)
        cases = []
        def scenario(name, fn, mode='edit', kind='checks'):
            if not selected or selected == name:
                cases.append((name, fn, mode, kind))

        def drive(item):
            name, fn, mode, kind = item
            if selected and selected != name:
                return
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
            rules = call(h, 'rules', 'builtin__workspace__read', {'path': 'AGENTS.md'})
            document = json.loads(content(rules)['text'])
            before = call(h, 'red', 'builtin__workspace__test', {})
            observed = json.loads(content(before)['text'])
            require(not observed['passed'] and 'flag provided but not defined' in observed['output'],
                    'fixture did not reproduce missing --status')
            path = 'cmd/taskctl/main.go'
            source = json.loads(content(call(h, 'source', 'builtin__workspace__read', {'path': path}))['text'])
            replacement = source['text'].replace('status := "" // CLI_STATUS_FLAG',
                                                'status := ""\n\tflags.StringVar(&status, "status", "", "Filter Task status")')
            require(replacement != source['text'], 'fixture patch did not match')
            edited = call(h, 'edit', 'builtin__workspace__edit',
                          {'path': path, 'before_hash': source['hash'], 'rules_hash': document['hash'], 'text': replacement}, worker=True)
            require(not content(edited).get('error'), 'approved Worker edit failed')
            require((repo / path).read_text() == replacement, 'edit did not reach actual workspace file')
            test_path = 'cmd/taskctl/main_test.go'
            test_source = json.loads(content(call(h, 'test-source', 'builtin__workspace__read', {'path': test_path}))['text'])
            tests = test_source['text'] + '\nfunc TestSearchActiveCompatibility(t *testing.T) {\n var out bytes.Buffer\n if err := run([]string{"--status", "active"}, &out); err != nil { t.Fatal(err) }\n if out.String() != "[{\\\"Title\\\":\\\"Build\\\",\\\"Status\\\":\\\"active\\\"}]\\n" { t.Fatal(out.String()) }\n}\n'
            test_edit = call(h, 'test-edit', 'builtin__workspace__edit', {'path': test_path, 'before_hash': test_source['hash'], 'rules_hash': document['hash'], 'text': tests}, worker=True)
            require(not content(test_edit).get('error'), 'test edit failed')
            tested = call(h, 'green', 'builtin__workspace__test', {})
            require(json.loads(content(tested)['text'])['passed'], 'real sandbox tests did not pass: ' + content(tested)['text'])
            diff = content(call(h, 'diff', 'builtin__workspace__git', {'operation': 'diff'}))['text']
            require('flags.StringVar' in diff, 'Git diff omitted real edit')
            protected = content(call(h, 'protected', 'builtin__workspace__edit',
                                {'path': 'docs/notes.md', 'before_hash': digest(repo / 'docs/notes.md'),
                                 'rules_hash': document['hash'], 'text': 'overwrite'}))
            require(protected['error']['code'] == 'PERMISSION_DENIED', 'protected dirty file edited')
            final = tree(repo)
            changed = [p for p in original if original[p] != final.get(p)]
            require(sorted(changed) == sorted([path, test_path]), f'unrelated coding changes: {changed}')
            require(not (repo / 'hook-ran').exists() and original['.git/hooks/pre-commit'] == final['.git/hooks/pre-commit'],
                    'hook changed or executed')
            return {'name': 'real_red_edit_green_with_Worker_approval', 'changed': changed,
                    'before': observed, 'after': json.loads(content(tested)['text']),
                    'sentinel_hash': digest(repo / 'docs/notes.md'), 'hook_hash': digest(repo / '.git/hooks/pre-commit'), 'state': h.state()}
        scenario('coding', coding)

        def dirty_index(h):
            # Exercise unprotected files through approved Worker calls. Git's
            # hidden-index flags must never turn user bytes into editable input.
            repo = h.root / 'repo'
            rules = json.loads(content(call(h, 'rules', 'builtin__workspace__read', {'path': 'AGENTS.md'}))['text'])
            observations = []
            paths = ['cmd/taskctl/main.go', 'internal/retry/executor.go', 'internal/task/search.go']
            for flag, path in zip([None, '--assume-unchanged', '--skip-worktree'], paths):
                if flag:
                    fixture_run(['git', 'update-index', flag, '--', path], cwd=repo, check=True,
                                   capture_output=True, env={'PATH': '/usr/bin:/bin', 'HOME': '/nonexistent',
                                                            'GIT_CONFIG_NOSYSTEM': '1', 'GIT_CONFIG_GLOBAL': '/dev/null'})
                source = repo / path
                source.write_text(source.read_text() + '\n// USER_UNCOMMITTED_CHANGE\n')
                before = tree(repo)
                observed = json.loads(content(call(h, 'read-' + path, 'builtin__workspace__read', {'path': path}))['text'])
                invocation = call(h, 'edit-' + path, 'builtin__workspace__edit',
                                  {'path': path, 'before_hash': observed['hash'], 'rules_hash': rules['hash'],
                                   'text': '// AGENT_REPLACEMENT\n'}, worker=True)
                require(invocation['status'] == 'failed' and content(invocation)['error']['code'] == 'PERMISSION_DENIED',
                        f'{flag or "ordinary dirty"}: approved Worker overwrote user content')
                require(tree(repo) == before, f'{flag or "ordinary dirty"}: workspace/index/hooks changed')
                observations.append({'flag': flag, 'path': path, 'content_preserved': digest(source) == before[path],
                                     'index_unchanged': digest(repo / '.git/index') == before['.git/index'],
                                     'invocation': invocation})
            return {'name': 'approved_Worker_dirty_index_flags_refused', 'cases': observations, 'state': h.state()}
        scenario('dirty-index', dirty_index)

        def audit(h):
            repo = h.root / 'repo'
            before = tree(repo)
            call(h, 'audit-rules', 'builtin__workspace__read', {'path': 'AGENTS.md'})
            searched = content(call(h, 'search', 'builtin__workspace__search', {'query': 'provider.Execute'}, worker=True))
            require('internal/retry/executor.go' in searched['text'], 'audit search missed actual source')
            read = content(call(h, 'read', 'builtin__workspace__read', {'path': 'internal/retry/executor.go'}))
            require('provider.Execute()' in read['text'] and 'ledger.Lookup()' in read['text'] and read['text'].index('provider.Execute()') < read['text'].index('ledger.Lookup()'), 'audit lacks call-order evidence')
            forbidden = content(call(h, 'forged-edit', 'builtin__workspace__edit', {}, worker=True))
            require(forbidden['error']['code'] == 'NOT_FOUND', 'audit edit capability leaked')
            require(tree(repo) == before, 'audit changed workspace bytes or Git state')
            return {'name': 'audit_read_search_no_writes', 'tree_hashes': before, 'finding': {'file': 'internal/retry/executor.go', 'symbols': ['retry.Execute', 'Provider.Execute', 'Ledger.Lookup'], 'trigger': 'retry after a previously completed call', 'impact': 'Provider runs before the completed Ledger result is checked, so a retry can repeat an effect', 'basis': 'observed source order; no claim of a production failure'}, 'state': h.state()}
        scenario('audit', audit, mode='audit')

        def quotes(h):
            fetched = content(call(h, 'quotes', 'mcp__fixture__read', {'text': 'insurance_quotes'}))
            data = json.loads(fetched['text'])
            b = next(q for q in data['quotes'] if q['id'] == 'B')
            require(data['coverage_equal'] and not b['cashback_written'], 'lost quote source uncertainty')
            possible = round(b['premium_cny'] - b['cashback_cny'], 2)
            require(possible == 5038.49 and b['premium_cny'] == 5738.49, 'incorrect cashback scenarios')
            report_text = (f"Coverage is equal. B costs CNY {possible:.2f} if its oral cashback is paid, "
                           f"or CNY {b['premium_cny']:.2f} otherwise. Cashback is unconfirmed. "
                           "Request written terms, eligibility/payment conditions and complaint channels. "
                           "Recommend B only if cashback becomes written; C is an option for certainty. "
                           "Source: MCP fixture insurance_quotes. This is the M3 transport/artifact fragment.")
            saved = content(call(h, 'report', 'builtin__artifact__manage',
                                 {'operation': 'save', 'name': 'quote_comparison.md', 'mediaType': 'text/markdown', 'text': report_text}))
            ref = saved['artifact_refs'][0]
            opened = fixture_run(h.args('artifact', 'open', ref), capture_output=True, text=True,
                                    env=h.env, cwd=h.root, timeout=20)
            require(opened.returncode == 0 and opened.stdout == report_text, 'saved comparison bytes not reopenable')
            return {'name': 'GC-11_structured_quote_and_Artifact_fragment', 'source': data,
                    'conditional_net_cny': possible, 'without_cashback_cny': b['premium_cny'], 'artifact_ref': ref,
                    'artifact_sha256': hashlib.sha256(opened.stdout.encode()).hexdigest(), 'state': h.state()}
        scenario('quotes', quotes)

        def discovery(h):
            listed = content(call(h, 'skills', 'builtin__skill__list', {}, worker=True))
            summaries = json.loads(listed['text'])
            require(len(summaries) == 1 and 'PRIVATE_BODY_MARKER' not in listed['text'], 'Skill list exposed body')
            viewed = content(call(h, 'view', 'builtin__skill__view', {'name': 'review', 'version': summaries[0]['version']}))
            require('PRIVATE_BODY_MARKER' in viewed['text'], 'Skill view missing actual body')
            asset = content(call(h, 'asset', 'builtin__skill__view',
                                 {'name': 'review', 'version': summaries[0]['version'], 'path': 'assets/opaque.bin'}))
            opened = fixture_run(h.args('artifact', 'open', asset['artifact_refs'][0]), capture_output=True,
                                    env=h.env, cwd=h.root, timeout=20)
            require(opened.returncode == 0 and opened.stdout == b'\x00\xffSKILL_ASSET',
                    'binary Skill asset was corrupted or bypassed ArtifactStore')
            denied = content(call(h, 'manage', 'builtin__skill__manage', {}, worker=True))
            require(denied['error']['code'] == 'NOT_FOUND', 'ordinary Worker got Skill management')
            denied = content(call(h, 'task-control', 'builtin__task__manage', {'operation': 'cancel', 'taskRef': 'forged'}, worker=True))
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
            rules = json.loads(content(call(h, 'rules', 'builtin__workspace__read', {'path': 'AGENTS.md'}))['text'])
            path = 'cmd/taskctl/main.go'
            source = json.loads(content(call(h, 'source', 'builtin__workspace__read', {'path': path}))['text'])
            replacement = source['text'] + '\n// accepted edit\n'
            plan = [{'name': 'builtin__workspace__edit', 'arguments': {'path': path, 'before_hash': source['hash'],
                     'rules_hash': rules['hash'], 'text': replacement}}]
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
            inv = [t for t in value['tools'] if t['tool_name'] == 'builtin__workspace__edit'][0]
            require(inv['status'] == 'unknown' and inv['attempt_count'] == 1 and (repo / path).read_text() == replacement,
                    'crashed edit retried or falsely completed')
            return {'name': 'edit_after_effect_before_result_commit', 'signal': 'SIGKILL', 'state': value}
        scenario('crash', crash, kind='crashes')
        for kind, result in run_cases(cases, drive, scenario_jobs):
            report[kind].append(result)
        if not selected:
            require(len(report['checks']) == 6 and len(report['crashes']) == 1, 'complete capability scenario inventory required')
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
