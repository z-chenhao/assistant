#!/usr/bin/env python3
"""M3 stdio MCP, progressive Skills and actual controlled coding CLI proof."""
import argparse
import hashlib
import json
import shutil
import signal
import sqlite3
import subprocess
import tempfile
from pathlib import Path

from verify_main import MainHarness, require


class CapabilityHarness(MainHarness):
    def args(self, *args):
        return [str(self.binary), '-config', str(self.root / 'config.yaml'),
                '-test-bridge', 'capabilities', *args]

    def command(self, *args, text=None, ok=True):
        result = subprocess.run(self.args(*args), input=text, text=True, capture_output=True,
                                env=self.env, cwd=self.root, timeout=320)
        require(not ok or result.returncode == 0, f'{args}: {result.stderr}')
        return json.loads(result.stdout) if ok else result


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tree(root):
    return {str(p.relative_to(root)): digest(p) for p in sorted(root.rglob('*')) if p.is_file()}


def prepare(binary, root, mode='edit'):
    repo = root / 'repo'
    shutil.copytree(Path(__file__).resolve().parents[1] / 'evals/fixtures/coding-v1', repo)
    for args in [('init', '-q'), ('add', '.'),
                 ('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.test',
                  'commit', '-qm', 'fixture')]:
        subprocess.run(['git', *args], cwd=repo, check=True, capture_output=True,
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


def run(binary):
    report = {'version': 1, 'scope': 'M3 MCP/Skill/coding contracts; GC-19 prerequisites, GC-20 coding boundary only; M4 Child and M5 Durable Replay remain not_applicable',
              'checks': [], 'crashes': [], 'cleanup': False}
    with tempfile.TemporaryDirectory(prefix='erii-capabilities-') as temp:
        root = Path(temp)
        def scenario(name, fn, mode='edit'):
            folder = root / name
            folder.mkdir()
            h = prepare(binary, folder, mode)
            try:
                return fn(h)
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
            require(changed == [path], f'unrelated coding changes: {changed}')
            require(not (repo / 'hook-ran').exists() and original['.git/hooks/pre-commit'] == final['.git/hooks/pre-commit'],
                    'hook changed or executed')
            return {'name': 'real_red_edit_green_with_Worker_approval', 'changed': changed,
                    'before': observed, 'after': json.loads(content(tested)['text']),
                    'sentinel_hash': digest(repo / 'docs/notes.md'), 'hook_hash': digest(repo / '.git/hooks/pre-commit'), 'state': h.state()}
        report['checks'].append(scenario('coding', coding))

        def audit(h):
            repo = h.root / 'repo'
            before = tree(repo)
            searched = content(call(h, 'search', 'builtin__workspace__search', {'query': 'provider.Execute'}, worker=True))
            require('internal/retry/executor.go' in searched['text'], 'audit search missed actual source')
            read = content(call(h, 'read', 'builtin__workspace__read', {'path': 'internal/retry/executor.go'}))
            require('provider.Execute()' in read['text'] and 'ledger.Lookup()' in read['text'], 'audit lacks symbol evidence')
            forbidden = content(call(h, 'forged-edit', 'builtin__workspace__edit', {}, worker=True))
            require(forbidden['error']['code'] == 'NOT_FOUND', 'audit edit capability leaked')
            require(tree(repo) == before, 'audit changed workspace bytes or Git state')
            return {'name': 'audit_read_search_no_writes', 'tree_hashes': before, 'state': h.state()}
        report['checks'].append(scenario('audit', audit, mode='audit'))

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
            saved = content(call(h, 'report', 'builtin__artifact__save',
                                 {'name': 'quote_comparison.md', 'mediaType': 'text/markdown', 'text': report_text}))
            ref = saved['artifact_refs'][0]
            opened = subprocess.run(h.args('artifact', 'open', ref), capture_output=True, text=True,
                                    env=h.env, cwd=h.root, timeout=20)
            require(opened.returncode == 0 and opened.stdout == report_text, 'saved comparison bytes not reopenable')
            return {'name': 'GC-11_structured_quote_and_Artifact_fragment', 'source': data,
                    'conditional_net_cny': possible, 'without_cashback_cny': b['premium_cny'], 'artifact_ref': ref,
                    'artifact_sha256': hashlib.sha256(opened.stdout.encode()).hexdigest(), 'state': h.state()}
        report['checks'].append(scenario('quotes', quotes))

        def discovery(h):
            listed = content(call(h, 'skills', 'builtin__skill__list', {}, worker=True))
            summaries = json.loads(listed['text'])
            require(len(summaries) == 1 and 'PRIVATE_BODY_MARKER' not in listed['text'], 'Skill list exposed body')
            viewed = content(call(h, 'view', 'builtin__skill__view', {'name': 'review', 'version': summaries[0]['version']}))
            require('PRIVATE_BODY_MARKER' in viewed['text'], 'Skill view missing actual body')
            asset = content(call(h, 'asset', 'builtin__skill__view',
                                 {'name': 'review', 'version': summaries[0]['version'], 'path': 'assets/opaque.bin'}))
            opened = subprocess.run(h.args('artifact', 'open', asset['artifact_refs'][0]), capture_output=True,
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
        report['checks'].append(scenario('discovery', discovery))

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
        report['crashes'].append(scenario('crash', crash))
    report['cleanup'] = not root.exists()
    require(report['cleanup'], 'disposable roots remain')
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    report = run(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'checks': len(report['checks']), 'crashes': len(report['crashes']), 'cleanup': report['cleanup']}))
