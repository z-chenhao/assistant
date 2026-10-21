#!/usr/bin/env python3
"""Memory/Conversation Launch, Doctor, Drive, Evidence and Cleanup through CLI."""
import argparse
from contextlib import closing
import json
from pathlib import Path
import signal
import sqlite3
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from verify_main import MainHarness, require


class Embeddings:
    """Explicit deterministic external embedding Fake; production HTTP adapter."""
    def __init__(self):
        self.fail = False
        self.requests = []
        self.before_response = None
        self.callback_errors = []
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def do_POST(self):
                value = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                owner.requests.append(value)
                if owner.fail:
                    self.send_response(503)
                    self.end_headers()
                    self.wfile.write(b'SYNTHETIC_EMBEDDING_SECRET')
                    return
                require(self.path == '/v1/embeddings', 'wrong embedding endpoint')
                require(value['model'] == 'semantic-fixture-v1', 'model drift')
                if owner.before_response is not None:
                    try:
                        owner.before_response(value)
                    except Exception as error:
                        owner.callback_errors.append(type(error).__name__)
                        self.send_response(503)
                        self.end_headers()
                        return
                if len(value['input'].encode('utf-8')) > 64 << 10:
                    self.send_response(400)
                    self.end_headers()
                    return
                text = value['input'].lower()
                vector = [float(any(term in text for term in terms)) for terms in
                          [('tea', 'chai'), ('train', 'railway'), ('wedding', 'marriage'),
                           ('sleep', 'rest')]] + [.01]
                raw = json.dumps(dict(model=value['model'], data=[dict(index=0, embedding=vector)])).encode()
                self.send_response(200)
                self.send_header('Content-Type', 'application/json')
                self.send_header('Content-Length', str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)

        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever)
        self.thread.start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=5)
        require(not self.thread.is_alive(), 'embedding Fake did not stop')


def main_completed(h, run_id):
    with closing(sqlite3.connect(Path(h.env['ERII_DATA_ROOT']) / 'state.db')) as db:
        return db.execute("SELECT status='completed' FROM run WHERE run_id=? AND kind='main'", (run_id,)).fetchone() == (1,)


def worker_requests(h):
    root = Path(h.env['ERII_DATA_ROOT'])
    with closing(sqlite3.connect(root / 'state.db')) as db:
        ids = {r[0] for r in db.execute("SELECT generation_id FROM model_generation JOIN run USING(run_id) WHERE kind='worker'")}
    audit = root / 'fake-provider-calls.jsonl'
    return sum(json.loads(line)['generation_id'] in ids for line in audit.read_text().splitlines()) if audit.exists() else 0


def drain_with_publication_barrier(h, main_run, *, prefix='', progress=None):
    """Freeze Root recall, publish Main, then dispatch Root; never retry errors.

    Existing forced-conflict coverage proves a pending concurrent append fails
    closed. Optional progress drives a control Main while a Root is held later.
    """
    main_release=h.root/(prefix+'publish.release')
    worker_release=h.root/(prefix+'worker.release')
    require(not main_release.exists() and not worker_release.exists(),'occupied recall barriers')
    baseline=None
    seen=dict(after_task_commit=0,before_worker_generation=0)
    evidence=dict(worker_requests_before_main_completion=None)
    def observe(name):
        nonlocal baseline
        if baseline is None:baseline=worker_requests(h)
        require(name in seen,'unexpected recall barrier')
        seen[name]+=1
        if name=='before_worker_generation' and not main_release.exists():
            main_release.touch(exist_ok=False)
    def advance():
        if main_release.exists() and not worker_release.exists() and main_completed(h,main_run):
            evidence['worker_requests_before_main_completion']=worker_requests(h)-baseline
            require(evidence['worker_requests_before_main_completion']==0,'Worker dispatched before the publication barrier')
            worker_release.touch(exist_ok=False)
        if progress:progress()
    result=h.drive_boundaries({'after_task_commit':main_release,'before_worker_generation':worker_release},observe,advance)
    require(result.returncode==0,"('chat', 'work'): "+result.stderr)
    require(all(seen.values()) and worker_release.exists(),'recall scheduling barriers were not exercised')
    return dict(**evidence,boundaries=seen,root_recall_frozen_before_main_publication=True,
                main_completed_before_worker_dispatch=True)


class Harness(MainHarness):
    def __init__(self, binary, root, embeddings):
        super().__init__(binary, root, fixture='recall')
        self.config = root / 'config.yaml'
        self.config.write_text('version: 1\nembedding:\n  provider: openai_compatible\n'
                               f'  endpoint: http://127.0.0.1:{embeddings.server.server_port}/v1\n'
                               '  model: semantic-fixture-v1\n  dimensions: 5\n')

    def args(self, *args):
        return super().args('-config', str(self.config), *args)

    def main_completed(self, run_id):
        return main_completed(self,run_id)

    def worker_requests(self):
        return worker_requests(self)

    def drain(self, main_run):
        return drain_with_publication_barrier(self,main_run)

    def snapshot(self):
        result = self.state()
        result['agent_id'] = self.command('status')['agent_id']
        root = Path(self.env['ERII_DATA_ROOT'])
        with sqlite3.connect(root / 'state.db') as db:
            db.row_factory = sqlite3.Row
            result['recall'] = [dict(r) for r in db.execute('SELECT * FROM run_recall ORDER BY run_id,step_index')]
            result['children'] = [dict(r) for r in db.execute("SELECT run_id,parent_run_id,worker_position,checkpoint_json FROM run WHERE kind='worker' ORDER BY created_at")]
        for name in ('memory', 'conversation'):
            with sqlite3.connect(root / (name + '.db')) as db:
                db.row_factory = sqlite3.Row
                result[name + '_index'] = [dict(r) for r in db.execute('SELECT * FROM recall_document ORDER BY ref')]
                result[name + '_schema'] = db.execute('PRAGMA user_version').fetchone()[0]
                result[name + '_journal'] = db.execute('PRAGMA journal_mode').fetchone()[0]
                if name == 'memory':
                    result['memories'] = [dict(r) for r in db.execute('SELECT * FROM memory_record ORDER BY memory_id')]
        return result

    def seed(self, legacy=False):
        """Launch fixture only. Production mutations are tested via Memory.Apply."""
        agent = self.command('status')['agent_id']
        root = Path(self.env['ERII_DATA_ROOT'])
        with sqlite3.connect(root / 'memory.db') as db:
            for id_, user, agent_, text in [
                    ('lexical', 'local', agent, 'tea preference from explicit evidence'),
                    ('semantic', 'local', agent, 'chai without sugar'),
                    ('user', 'local', '', 'tea user-level fact'),
                    ('foreign-user', 'bob', agent, 'tea FOREIGN_USER_MARKER'),
                    ('foreign-agent', 'local', 'other', 'tea FOREIGN_AGENT_MARKER')]:
                kind, scope = ('agent', agent_) if agent_ else ('user', user)
                db.execute('INSERT INTO memory_record(memory_id,user_id,scope_type,scope_id,tier,content,source_refs,created_at) VALUES(?,?,?,?,?,?,?,?)',
                           (id_, user, kind, scope, 'recall', text,
                            json.dumps([dict(source_type='conversation', source_ref='conversation:original')]), 1000000))
                db.execute('INSERT INTO recall_document(ref,user_id,agent_id,content,occurred_at,author_role,core,active) VALUES(?,?,?,?,?,?,?,?)',
                           ('memory:' + id_, user, agent_, text, 1000000, '', 0, 1))
        with sqlite3.connect(root / 'conversation.db') as db:
            for id_, user, owner, text in [('original', 'local', agent, 'chai was my original preference'),
                                          ('foreign', 'bob', agent, 'tea FOREIGN_CONVERSATION_MARKER')]:
                conversation_id = agent if legacy and id_ == 'original' else id_
                db.execute('INSERT INTO conversation_scope VALUES(?,?,?)', (conversation_id, user, owner))
                db.execute('INSERT INTO conversation_message VALUES(?,?,?,?,?,?,?,?)',
                           (id_, conversation_id, 1, 'user', json.dumps([dict(type='text', text=text)]), 'cli', id_, 1000000))
                db.execute('INSERT INTO recall_document(ref,user_id,agent_id,content,occurred_at,author_role,core,active) VALUES(?,?,?,?,?,?,?,?)',
                           ('conversation:' + id_, user, owner, text, 1000000, 'user', 0, 1))
        return agent

    def seed_multipart(self, agent, prior=False):
        """Valid persisted original; the Go regression also uses public Append."""
        parts = [dict(type='text', text='a' * 40000),
                 dict(type='text', text='茶' * 13330 + ' tailmark!')]
        raw = json.dumps(parts, ensure_ascii=False)
        text = '\n'.join(p['text'] for p in parts)
        require(len(text.encode('utf-8')) == 80001, 'multipart fixture size drift')
        with sqlite3.connect(Path(self.env['ERII_DATA_ROOT']) / 'conversation.db') as db:
            db.execute('INSERT INTO conversation_scope SELECT ?,?,? WHERE NOT EXISTS(SELECT 1 FROM conversation_scope WHERE conversation_id=?)',
                       (agent, 'local', agent, agent))
            db.execute('INSERT INTO conversation_message VALUES(?,?,(SELECT coalesce(max(sequence),0)+1 FROM conversation_message WHERE conversation_id=?),?,?,?,?,?)',
                       ('multipart', agent, agent, 'user', raw, 'cli', 'multipart', 1000000))
            db.execute('INSERT INTO recall_document(ref,user_id,agent_id,content,occurred_at,author_role,core,active) VALUES(?,?,?,?,?,?,?,?)',
                       ('conversation:multipart', 'local', agent, text, 1000000, 'user', 0, 1))
            if prior:
                # Exact v1 originals, without derived v2 data; migration must
                # reconstruct all documents without rewriting the source JSON.
                db.executescript('DROP TABLE recall_fts; DROP TABLE recall_document; '
                                 'DROP TABLE recall_configuration; DROP TABLE conversation_scope; '
                                 'DROP TABLE compaction_summary; '
                                 'PRAGMA user_version=1;')
        return raw, text


def verify(value):
    tools = [t for t in value['tools'] if t['tool_name'] == 'builtin__memory__search']
    runs = {r['run_id']: r for r in value['runs']}
    require(len(tools) == 3 and all(t['status'] == 'succeeded' for t in tools), 'missing real Main/Root/Child Tool search')
    roles = {runs[t['run_id']]['kind'] if runs[t['run_id']]['kind'] == 'main' else
             next(c['worker_position'] for c in value['children'] if c['run_id'] == t['run_id']) for t in tools}
    require(roles == {'main', 'root', 'child'}, 'search roles incorrect')
    for t in tools:
        hits = json.loads(json.loads(t['result_json'])['text'])
        require({'memory', 'conversation'} <= {h['source_type'] for h in hits}, 'source missing')
        require(any(h['source_ref'] == 'memory:semantic' for h in hits), 'vector-only synonym missing')
        require(any(h['source_ref'] == 'conversation:original' and h['author_role'] == 'user' for h in hits), 'original provenance missing')
        require(all(set(h) <= {'content', 'source_type', 'source_ref', 'occurred_at', 'author_role'} for h in hits), 'score or private field leaked')
        require('FOREIGN_' not in json.dumps(hits), 'cross-scope recall')
    require(all(r['status'] == 'completed' for r in value['runs']), 'nonterminal Run')
    require(len([c for c in value['children'] if c['worker_position'] == 'child']) == 1, 'Child count')
    for c in value['children']:
        cp = json.loads(c['checkpoint_json'])
        if c['worker_position'] == 'child':
            base = cp['messages'][:3]
            require(len(base) == 3 and base[-1]['content'][0]['text'].startswith('Retrieve tea context independently'), 'parent context copied')
            require(not any('Task anchor:' in json.dumps(m) or 'personal assistant' in json.dumps(m) for m in base), 'Child inherited identity/history')
    for key in ('memory', 'conversation'):
        require(value[key + '_journal'] == 'wal', 'missing WAL')
        authorized = [r for r in value[key + '_index'] if r['user_id'] == 'local' and r['agent_id'] in ('', value['agent_id'])]
        # A final assistant reply may be committed after the final model recall.
        require(any(r['vector_json'] is not None for r in authorized), 'no actual vector index')
    original = next(m for m in value['conversation_index'] if m['ref'] == 'conversation:original')
    require(original['content'] == 'chai was my original preference', 'original changed')
    require(all('FOREIGN_' not in r['hits_json'] for r in value['recall']), 'automatic recall scope leak')


def failure_state(h, embeddings, scenario, stage):
    """Read only structural evidence; never copy corpus, prompts or HTTP bodies."""
    value = dict(scenario=scenario, stage=stage, embedding_requests=len(embeddings.requests),
                 embedding_failure_enabled=embeddings.fail, databases={})
    root = Path(h.env['ERII_DATA_ROOT'])
    agents = []
    for name in ('state', 'memory', 'conversation'):
        path = root / (name + '.db')
        if not path.exists():
            continue
        try:
            with closing(sqlite3.connect(path.as_uri() + '?mode=ro', uri=True)) as db:
                if name == 'state':
                    rows = db.execute('SELECT kind,status,error_code,count(*) FROM run GROUP BY kind,status,error_code').fetchall()
                    value['databases'][name] = dict(run_groups=rows,
                        frozen_recalls=db.execute('SELECT count(*) FROM run_recall').fetchone()[0])
                    agents = [r[0] for r in db.execute('SELECT DISTINCT agent_id FROM run')]
                else:
                    rows = db.execute('SELECT active,vector_json IS NULL,count(*) FROM recall_document GROUP BY active,vector_json IS NULL').fetchall()
                    value['databases'][name] = dict(index_groups=rows)
                    if agents:
                        parameters = ','.join('?' for _ in agents)
                        pending = db.execute("SELECT count(*) FROM recall_document WHERE active=1 AND vector_json IS NULL AND user_id='local' AND (agent_id='' OR agent_id IN (" + parameters + '))', agents).fetchone()[0]
                        value['databases'][name]['authorized_pending_vectors'] = pending
        except sqlite3.Error:
            value['databases'][name] = dict(diagnostic_unavailable=True)
    return value


def concurrent_append_case(binary, root, failure_evidence=None):
    """Force Main's real append into Root's catch-up/query gap without sleeps."""
    root.mkdir()
    embeddings = Embeddings()
    h = Harness(binary, root, embeddings)
    release = root / 'task.release'
    stage = 'setup'
    barrier = None
    try:
        h.setup()
        h.seed()
        receipt = h.send('recall-request', 'tea work with independent child recall')
        observed = dict(query_requests=0, pending_assistant_vectors=None)
        def append_after_catch_up():
            release.touch(exist_ok=False)
            h.wait_state(bool, 'Main publication during Root query',
                         reader=lambda _: h.main_completed(receipt['run_id']))
            with closing(sqlite3.connect(Path(h.env['ERII_DATA_ROOT']) / 'conversation.db')) as db:
                observed['pending_assistant_vectors'] = db.execute("SELECT count(*) FROM recall_document WHERE active=1 AND vector_json IS NULL AND author_role='assistant' AND user_id='local'").fetchone()[0]
            require(observed['pending_assistant_vectors'] == 1, 'concurrent append boundary was not forced')
        barrier = threading.Barrier(2, action=append_after_catch_up)
        def hold_query(value):
            if 'Retrieve tea context independently and delegate a separate recall.' in value['input']:
                observed['query_requests'] += 1
                barrier.wait(timeout=10)
        embeddings.before_response = hold_query
        stage = 'forced-conflict'
        child = h.hold('after_task_commit', extra_flags=('-test-main-release', str(release)))
        _, error = child.communicate(timeout=20)
        require(child.returncode == 1 and error.strip() == 'erii: run storage unavailable',
                'concurrent pending append did not fail the CLI closed')
        require(not embeddings.callback_errors and observed['query_requests'] == 2,
                'embedding barrier failed instead of exercising recall')
        before = h.snapshot()
        workers = {r['run_id'] for r in before['runs'] if r['kind'] == 'worker'}
        require(len(workers) == 1 and h.worker_requests() == 0 and
                not any(r['run_id'] in workers for r in before['recall']) and
                not any(g['run_id'] in workers for g in before['generations']),
                'failed recall admitted a partial Worker context/model request')
        embeddings.before_response = None
        stage = 'explicit-later-caller'
        h.command('chat', 'work')
        value = h.snapshot()
        verify(value)
        require(len(value['generations']) == 9 and len(value['runs']) == 4,
                'explicit recovery changed the complete recall fixture')
        require(all(r in value['recall'] for r in before['recall']) and
                value['conversation'][:len(before['conversation'])] == before['conversation'],
                'explicit recovery rewrote frozen recall or original messages')
        h.command('chat', 'work')
        require(h.snapshot() == value, 'repeated recovery drain changed terminal state')
        return dict(name='concurrent-append', first_drive_rejected=True,
                    worker_model_requests_before_recovery=0, explicit_later_caller_recovered=True,
                    boundary=observed, state=value, embedding_requests=embeddings.requests)
    except Exception:
        if failure_evidence is not None:
            failure_evidence.parent.mkdir(parents=True, exist_ok=True)
            failure_evidence.write_text(json.dumps(dict(version=1, passed=False,
                failure=failure_state(h, embeddings, 'concurrent-append', stage)), indent=2) + '\n')
        raise
    finally:
        if barrier is not None:
            barrier.abort()
        try:
            h.cleanup()
        finally:
            embeddings.close()


def run(binary, failure_evidence=None):
    report = dict(version=1, scope='ERII-22 read-only hybrid recall and frozen context; Curator/manage/compaction and full SC-08 remain ERII-23', scenarios=[])
    with tempfile.TemporaryDirectory(prefix='erii-recall-') as directory:
        for name in ('ordinary', 'kill-restart', 'embedding-failure', 'multipart', 'multipart-prior'):
            root = Path(directory) / name
            root.mkdir()
            embeddings = Embeddings()
            h = Harness(binary, root, embeddings)
            stage = 'setup'
            try:
                multipart = name.startswith('multipart')
                if multipart:
                    h.setup(metadata=dict(name='multipart fixture', provider_kind='openai_compatible',
                                          endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions',
                                          model_id='fixture-model', context_window=131072, max_output_tokens=8192,
                                          generation_defaults={}, compatibility=dict(tool_result_format='openai'),
                                          capabilities=dict(text_input=True)))
                else:
                    h.setup()
                agent = h.seed(legacy=name == 'multipart-prior')
                if multipart:
                    original_json, joined_text = h.seed_multipart(agent, prior=name.endswith('prior'))
                receipt = h.send('recall-request', 'tea work with independent child recall')
                stage = 'fault-boundary'
                if name == 'embedding-failure':
                    embeddings.fail = True
                    failure = h.command('chat', 'work', ok=False)
                    require(failure.returncode != 0 and 'SYNTHETIC_EMBEDDING_SECRET' not in failure.stderr, 'provider failure not sanitized')
                    before = h.snapshot()
                    require(not before['recall'] and not before['generations'], 'failed recall partially accepted')
                    embeddings.fail = False
                elif name == 'kill-restart':
                    child = h.hold('after_recall_commit')
                    child.send_signal(signal.SIGKILL)
                    child.communicate(timeout=10)
                    before = h.snapshot()
                    require(len(before['recall']) == 1 and not before['generations'], 'wrong kill boundary')
                    # Change corpus after the turn was frozen; restart must reuse it.
                    frozen = before['recall'][0]
                    with sqlite3.connect(Path(h.env['ERII_DATA_ROOT']) / 'memory.db') as db:
                        db.execute('INSERT INTO memory_record(memory_id,user_id,scope_type,scope_id,tier,content,source_refs,created_at) VALUES(?,?,?,?,?,?,?,?)',
                                   ('after-freeze', 'local', 'agent', agent, 'recall',
                                    'tea NEW_AFTER_FREEZE', '[{"source_type":"conversation","source_ref":"conversation:original"}]', 2000000))
                        db.execute('INSERT INTO recall_document(ref,user_id,agent_id,content,occurred_at,author_role,core,active) VALUES(?,?,?,?,?,?,?,?)',
                                   ('memory:after-freeze', 'local', agent, 'tea NEW_AFTER_FREEZE', 2000000, '', 0, 1))
                stage = 'drain'
                scheduling = h.drain(receipt['run_id'])
                stage = 'assertions'
                value = h.snapshot()
                verify(value)
                multipart_evidence = None
                if multipart:
                    document = next(d for d in value['conversation_index'] if d['ref'] == 'conversation:multipart')
                    require(document['active'] == 1 and document['vector_json'] is not None and
                            document['content'] == joined_text, 'multipart document missing or pending')
                    chunks = [r['input'] for r in embeddings.requests if len(r['input'].encode('utf-8')) > 4096]
                    require(len(chunks) == 2 and ''.join(chunks) == joined_text, 'multipart truncated or reindexed')
                    with sqlite3.connect(Path(h.env['ERII_DATA_ROOT']) / 'conversation.db') as db:
                        stored = db.execute("SELECT content_json FROM conversation_message WHERE message_id='multipart'").fetchone()[0]
                    require(stored == original_json and value['conversation_schema'] == 3, 'migration rewrote original')
                    require(len(value['generations']) == 9 and len(value['runs']) == 4, 'multipart blocked later model admission')
                    multipart_evidence = dict(original_json_unchanged=True, original_bytes=80001,
                                              chunk_bytes=[len(c.encode('utf-8')) for c in chunks],
                                              generations=9, completed_runs=4)
                if name == 'kill-restart':
                    same = next(r for r in value['recall'] if r['run_id'] == receipt['run_id'] and r['step_index'] == 0)
                    require(same == frozen, 'restart changed frozen recall')
                    require('NEW_AFTER_FREEZE' not in same['hits_json'] and any('NEW_AFTER_FREEZE' in r['hits_json'] for r in value['recall'] if r != same), 'per-turn snapshot did not advance')
                stage = 'idempotent-drain'
                h.command('chat', 'work')
                require(h.snapshot() == value, 'repeated drain changed terminal state')
                require(all(len(r['input'].encode('utf-8')) <= 64 << 10 for r in embeddings.requests), 'oversized embedding request')
                report['scenarios'].append(dict(name=name, state=value, embedding_requests=embeddings.requests,
                                                scheduling=scheduling,
                                                multipart=multipart_evidence,
                                                before=before if name in ('kill-restart', 'embedding-failure') else None))
            except Exception:
                if failure_evidence is not None:
                    failure_evidence.parent.mkdir(parents=True, exist_ok=True)
                    failure_evidence.write_text(json.dumps(dict(version=1, passed=False,
                        failure=failure_state(h, embeddings, name, stage)), indent=2) + '\n')
                raise
            finally:
                try:
                    h.cleanup()
                finally:
                    embeddings.close()
        report['scenarios'].append(concurrent_append_case(binary, Path(directory) / 'concurrent-append', failure_evidence))
    report['cleanup'] = 'CLI processes reaped, HTTP Fake stopped and joined, temporary roots removed'
    report['passed'] = True
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.unlink(missing_ok=True)
    report = run(args.binary.resolve(), args.evidence)
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(dict(passed=True, scenarios=len(report['scenarios']), cleanup=report['cleanup'])))


if __name__ == '__main__':
    main()
