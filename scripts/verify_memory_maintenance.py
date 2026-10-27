#!/usr/bin/env python3
"""M6 Curator, forget revocation, compaction and Reviewer through the real CLI."""
import argparse
from contextlib import closing
import json
from pathlib import Path
import sqlite3
import tempfile

from verify_main import require
from verify_capabilities import prepare, call, content
from verify_recall import Embeddings, drain_with_publication_barrier


def readback(h):
    root = Path(h.env['ERII_DATA_ROOT'])
    out = {}
    for name, queries in {
        'state': {'events': 'SELECT * FROM memory_mutation ORDER BY sequence',
                  'privacy': 'SELECT * FROM memory_privacy',
                  'compaction': 'SELECT * FROM context_compaction ORDER BY created_at',
                  'candidates': 'SELECT * FROM skill_candidate ORDER BY created_at'},
        'memory': {'records': 'SELECT * FROM memory_record ORDER BY created_at,memory_id',
                   'evidence': 'SELECT * FROM memory_evidence ORDER BY source_type,source_ref',
                   'receipts': 'SELECT * FROM memory_receipt ORDER BY event_id'},
        'conversation': {'summaries': 'SELECT * FROM compaction_summary ORDER BY created_at'},
    }.items():
        with closing(sqlite3.connect(root / (name + '.db'))) as db:
            db.row_factory = sqlite3.Row
            out[name] = {key: [dict(r) for r in db.execute(sql)] for key, sql in queries.items()}
    return out


def work(h):
    for _ in range(16):
        status = h.command('memory', 'work')
        require(not any(item.get('rejected') for item in status), 'memory mutation rejected: ' + str(status))
        if all(item['applied'] for item in status):
            return
    raise RuntimeError('memory maintenance did not drain its bounded batches')


def manage(h, key, **value):
    receipt = h.command('memory', 'manage', key, text=json.dumps(value))
    require(receipt['accepted'], 'mutation was not durably accepted')
    return receipt


def latest_memory(h):
    return readback(h)['memory']['records'][-1]['memory_id']


def skill_list(h, key):
    result = content(call(h, key, 'builtin__skill__list', {}))
    require(not result.get('error'), 'Skill list Tool failed')
    refs = result.get('artifact_refs', [])
    if refs:
        require(len(refs) == 1, 'unexpected catalog Artifact count')
        return h.command('artifact', 'open', refs[0])
    return json.loads(result['text'])


def run(binary):
    report = {'scope': 'M6 memory/context/Skill contracts and GC-01/02/03/06 fragments; not full product scores',
              'scenarios': [], 'delegation_ordering': [], 'passed': False}

    def delegated_drain(h, main_run):
        # This success fixture needs a complete Root recall before Main appends
        # its reply. The recall gate separately forces and rejects the opposite
        # interleaving. Never retry a failed drive or admit partial evidence.
        ordering = drain_with_publication_barrier(h, main_run, prefix=main_run + '-')
        report['delegation_ordering'].append(dict(main_run_id=main_run, **ordering))

    with tempfile.TemporaryDirectory(prefix='erii-memory-maintenance-') as temp:
        root = Path(temp)
        embeddings = Embeddings()
        h = None
        try:
            h = prepare(binary, root)
            h.env['ERII_MEMORY_KEY'] = 'S' * 32
            require(len(h.env['ERII_MEMORY_KEY']) == 32, 'synthetic key length')
            with (root / 'config.yaml').open('a') as cfg:
                cfg.write('memory:\n  encryption_key_ref: env:ERII_MEMORY_KEY\n'
                          f'embedding:\n  provider: openai_compatible\n  endpoint: http://127.0.0.1:{embeddings.server.server_port}/v1\n'
                          '  model: semantic-fixture-v1\n  dimensions: 5\n')
            # Doctor: same activated profile, explicit local Fakes, no online account.
            require(h.command('status')['agent_ingress_enabled'], 'doctor: no selected model')
            scenarios = [('health', 'My health report: I slept late and feel tired today'),
                         ('goals', 'I might explore independent development, but have not decided'),
                         ('learning', 'I am practicing English for travel today')]
            for name, text in scenarios:
                before = len(h.command('delivery', 'list'))
                h.send(name, text)
                h.command('chat', 'work')
                work(h)
                snapshot = readback(h)
                records = snapshot['memory']['records']
                require(records and records[-1]['tier'] == 'recall', name + ': noisy report promoted to Core')
                deliveries = len(h.command('delivery', 'list'))
                work(h)
                require(len(h.command('delivery', 'list')) == deliveries, 'maintenance notified user')
                report['scenarios'].append(dict(name=name, source=text, record=records[-1], deliveries=deliveries - before))

            accepted = call(h, 'main-remember', 'builtin__memory__manage',
                            {'operation': 'remember', 'content': 'I prefer concise answers'})
            require(accepted['status'] == 'succeeded' and not content(accepted).get('error'), 'Main memory control failed')
            work(h)
            before = len(readback(h)['state']['events'])
            denied = call(h, 'worker-remember', 'builtin__memory__manage',
                          {'operation': 'remember', 'content': 'Worker may not overwrite preferences'},
                          worker=True, drain=delegated_drain)
            require(content(denied).get('error', {}).get('code') in ('NOT_FOUND', 'PERMISSION_DENIED'),
                    'Worker gained Memory control')
            require(not any(row['operation'] == 'remember' and row['source_ref'] == denied['invocation_id']
                            for row in readback(h)['state']['events']), 'Worker submitted a long-term mutation')
            report['scenarios'].append(dict(name='Main-control-Worker-read-only', accepted=accepted, denied=denied))

            # Explicit finance correction travels through the durable control,
            # Curator and read-only Tool boundary. Originals remain audit records.
            finance = 'My employer options: buyback USD 240; vested 200; illiquid'
            corrected = 'My employer options: buyback USD 250; vested 220; illiquid'
            manage(h, 'finance-v1', operation='remember', content=finance)
            work(h)
            old = latest_memory(h)
            manage(h, 'finance-v2', operation='remember', content=corrected, supersedes='memory:' + old)
            work(h)
            current = latest_memory(h)
            found = call(h, 'finance-search', 'builtin__memory__search', {'query': 'employer options'})
            hits = json.loads(content(found)['text'])
            require(any(hit['content'] == corrected for hit in hits), 'finance correction not recalled')
            require(not any(hit['content'] == finance for hit in hits), 'superseded finance fact recalled')
            for path in Path(h.env['ERII_DATA_ROOT']).glob('memory.db*'):
                require(finance.encode() not in path.read_bytes() and corrected.encode() not in path.read_bytes(),
                        'sensitive Memory plaintext at rest')
            require(any(corrected in req['input'] for req in embeddings.requests),
                    'encrypted Memory did not use the selected local vector provider')
            report['scenarios'].append(dict(name='finance-correction-encryption', old=old, current=current, hits=hits))

            # Kill between memory.db domain commit and state.db acknowledgement.
            work(h)
            restart = manage(h, 'restart-event', operation='remember', content='I prefer unsweetened tea')
            event_id = restart['receipt'].removeprefix('memory-event:')
            held = h.hold('after_memory_commit:' + event_id, command=('memory', 'work'))
            at_hold = readback(h)
            require(any(row['event_id'] == event_id for row in at_hold['memory']['receipts']), 'commit boundary fired before durable Memory receipt')
            held.kill()
            held.communicate(timeout=10)
            before = readback(h)
            work(h)
            after = readback(h)
            original = [r for r in before['memory']['records']
                        if 'feedback:control:restart-event' in r['source_refs']]
            recovered = [r for r in after['memory']['records']
                         if 'feedback:control:restart-event' in r['source_refs']]
            require(len(original) == 1 and recovered == original,
                    'target event did not commit exactly once across restart')
            require(sum(r['event_id'] == event_id for r in after['memory']['receipts']) == 1,
                    'restart duplicated the target event receipt')
            report['scenarios'].append(dict(name='curator-kill-restart', before=before['state']['events'],
                                            after=after['state']['events']))

            # Freeze the finance fact before provider dispatch; revoke it from
            # another CLI process, kill the old owner and recover its stop.
            before_delivery = len(h.command('delivery', 'list'))
            receipt = h.send('frozen-before-forget', 'employer options')
            held = h.hold('before_generation_dispatch')
            frozen = h.state()
            pending = next(g for g in frozen['generations'] if g['run_id'] == receipt['run_id'])
            require(corrected in pending['request_json'], 'fixture did not freeze target fact')
            manage(h, 'forget-finance', operation='forget', memory_ref='memory:' + current)
            fresh = h.send('fresh-before-owner-joins', 'FRESH_LIVE_INPUT_MARKER')
            require(fresh['run_id'] != receipt['run_id'], 'fresh input reused revoked live Main')
            require(h.send('fresh-before-owner-joins', 'FRESH_LIVE_INPUT_MARKER') == fresh,
                    'fresh live-input receipt changed on retry')
            waiting = h.state()
            require(next(r for r in waiting['runs'] if r['run_id'] == fresh['run_id'])['status'] == 'queued'
                    and not any(g['run_id'] == fresh['run_id'] for g in waiting['generations']),
                    'fresh successor executed before the held owner joined')
            held.kill()
            held.communicate(timeout=10)
            h.command('chat', 'work')
            work(h)
            state = h.state()
            old_run = next(r for r in state['runs'] if r['run_id'] == receipt['run_id'])
            require(old_run['status'] == 'cancelled' and old_run['error_code'] == 'memory_forgotten', 'revocation not recovered')
            require(next(r for r in state['runs'] if r['run_id'] == fresh['run_id'])['status'] == 'completed',
                    'fresh input stranded after owner crash/restart')
            fresh_requests = [g['request_json'] for g in state['generations'] if g['run_id'] == fresh['run_id']]
            require(fresh_requests and all(corrected not in value and finance not in value for value in fresh_requests),
                    'live successor imported revoked context')
            require(any('FRESH_LIVE_INPUT_MARKER' in value for value in fresh_requests)
                    and len(h.command('delivery', 'list')) == before_delivery + 1,
                    'fresh live input did not produce exactly one answer')
            later = h.send('after-forget', 'What context is available now?')
            h.command('chat', 'work')
            state = h.state()
            new_requests = [g['request_json'] for g in state['generations'] if g['run_id'] == later['run_id']]
            require(new_requests and all(corrected not in value and finance not in value for value in new_requests),
                    'forgotten fact entered a future model context')
            require(any(corrected in g['request_json'] for g in state['generations'] if g['run_id'] == receipt['run_id']),
                    'immutable audit was erased')
            report['scenarios'].append(dict(name='forget-fences-frozen-recovery', old_run=old_run, requests=new_requests,
                                            successor=fresh, waiting=waiting, fresh_requests=fresh_requests,
                                            privacy=readback(h)['state']['privacy']))

            # Queued revocation: accept fresh ingress before stop recovery, then
            # prove its immutable receipt leads to one real model answer.
            before_delivery = len(h.command('delivery', 'list'))
            queued = h.send('queued-before-forget', 'REVOKED_QUEUED_INPUT_MARKER')
            manage(h, 'forget-queued', operation='forget', category='health')
            fresh = h.send('fresh-before-queued-settles', 'FRESH_QUEUED_INPUT_MARKER')
            require(fresh['run_id'] != queued['run_id'], 'fresh input reused revoked queued Main')
            h.command('chat', 'work')
            state = h.state()
            require(next(r for r in state['runs'] if r['run_id'] == queued['run_id'])['status'] == 'cancelled',
                    'revoked queued Main was revived')
            require(next(r for r in state['runs'] if r['run_id'] == fresh['run_id'])['status'] == 'completed',
                    'fresh queued input stranded')
            requests = [g['request_json'] for g in state['generations'] if g['run_id'] == fresh['run_id']]
            require(requests and all('REVOKED_QUEUED_INPUT_MARKER' not in value for value in requests),
                    'queued successor imported revoked input')
            require(any('FRESH_QUEUED_INPUT_MARKER' in value for value in requests),
                    'fresh queued input absent from successor context')
            require(len(h.command('delivery', 'list')) == before_delivery + 1,
                    'fresh queued input did not produce exactly one delivery')
            require(h.send('fresh-before-queued-settles', 'FRESH_QUEUED_INPUT_MARKER') == fresh,
                    'fresh terminal receipt changed on retry')
            report['scenarios'].append(dict(name='forget-preserves-queued-ingress', revoked=queued,
                                            successor=fresh, requests=requests, state=state))

            # Fresh authorized provenance must survive content deduplication.
            tea = 'I prefer unsweetened tea'
            def tea_hits(key):
                found = call(h, key, 'builtin__memory__search', {'query': 'unsweetened tea'})
                return [hit for hit in json.loads(content(found)['text'])
                        if hit['source_type'] == 'memory' and hit['content'] == tea]
            manage(h, 'tea-before-unrelated-forget', operation='remember', content=tea)
            work(h)
            prior_hits = tea_hits('tea-baseline-search')
            require(len(prior_hits) == 1, 'baseline explicit preference missing')
            manage(h, 'forget-unrelated-health', operation='forget', category='health')
            work(h)
            require(not tea_hits('tea-barrier-search'), 'old preference crossed conservative privacy barrier')
            reapplied = manage(h, 'tea-after-unrelated-forget', operation='remember', content=tea)
            work(h)
            hits = tea_hits('tea-fresh-search')
            require(len(hits) == 1 and hits[0]['source_ref'] != prior_hits[0]['source_ref'],
                    'fresh remember invisible or old provenance reused')
            snapshot = readback(h)
            record = next(r for r in snapshot['memory']['records']
                          if 'memory:' + r['memory_id'] == hits[0]['source_ref'])
            require('feedback:control:tea-after-unrelated-forget' in record['source_refs']
                    and record['created_at'] > snapshot['state']['privacy'][0]['updated_at'],
                    'fresh record lacks post-barrier provenance')
            require(any(r['event_id'] == reapplied['receipt'].removeprefix('memory-event:') and r['applied_at']
                        for r in snapshot['state']['events']), 'fresh remember receipt not applied')
            report['scenarios'].append(dict(name='remember-after-unrelated-forget', receipt=reapplied,
                                            before=prior_hits, after=hits, record=record))

            # Real CLI originals trigger hierarchical summaries. Compaction does
            # not replace semantic messages or runtime source boundaries.
            submitted = []
            for i in range(24):
                submitted.append(h.send('long-' + str(i), f'Historical source {i}: ' + 'original evidence ' * 60))
            h.command('chat', 'work')
            snapshot = readback(h)
            summaries = [row for row in snapshot['state']['compaction']
                         if json.loads(row['provenance_json'])['stage'] == 'summary']
            require(summaries and snapshot['conversation']['summaries'], 'long context was not summarized and persisted')
            history = h.command('chat', 'history')
            require(all(any(m['message_id'] == source['message_id'] for m in history) for source in submitted),
                    'compaction destroyed originals')
            report['scenarios'].append(dict(name='long-context-provenance', summaries=summaries,
                                            original_message_ids=[s['message_id'] for s in submitted]))

            # Real controlled workspace test, executed through ToolExecutor.
            rules = json.loads(content(call(h, 'rules', 'builtin__workspace__read', {'path': 'AGENTS.md'}))['text'])
            source = json.loads(content(call(h, 'cli-source', 'builtin__workspace__read', {'path': 'cmd/taskctl/main.go'}))['text'])
            replacement = source['text'].replace('status := "" // CLI_STATUS_FLAG',
                                                 'status := ""\n\tflags.StringVar(&status, "status", "", "Filter Task status")')
            require(replacement != source['text'], 'fixture correction did not apply')
            call(h, 'cli-fix', 'builtin__workspace__edit', {'path': 'cmd/taskctl/main.go', 'before_hash': source['hash'],
                                                          'rules_hash': rules['hash'], 'text': replacement},
                 worker=True, drain=delegated_drain)
            tests = [call(h, 'verified-' + str(i), 'builtin__workspace__test', {}) for i in range(2)]
            require(all(json.loads(content(inv)['text'])['passed'] for inv in tests), 'real workspace verification failed')
            # Isolate catalog cardinality from the earlier small-profile context
            # tests: 128 summaries plus list results need an explicit larger model.
            catalog_profile = h.setup(metadata=dict(name='catalog capacity fixture', provider_kind='openai_compatible',
                                                   endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions',
                                                   model_id='fixture-model', context_window=131072, max_output_tokens=8192,
                                                   generation_defaults={}, compatibility=dict(tool_result_format='openai'),
                                                   capabilities=dict(text_input=True)))
            skills = root / 'skills'
            # Fixture data: 127 active entries and more than 128 retained entries.
            # All admission/retirement decisions still use the real Reviewer CLI.
            for i in range(126):
                name = f'slot-{i:03d}'
                folder = skills / name
                folder.mkdir()
                (folder / 'SKILL.md').write_text(f'---\nname: {name}\ndescription: Fixture.\n---\nMethod.\n')
            retained = []
            for prefix, count in (('.erii-retired-', 140), ('.erii-pending-', 5)):
                for i in range(count):
                    folder = skills / f'{prefix}fixture-{i}'
                    folder.mkdir()
                    path = folder / 'SKILL.md'
                    path.write_text('Retained fixture bytes; not active metadata.\n')
                    retained.append(path)
            require(h.command('status')['agent_ingress_enabled'], 'retained history consumed active capacity')

            def retire_skill(request, name):
                retired = h.command('skill', 'manage', text=json.dumps(dict(request, operation='retire')))
                require(retired['status'] == 'retired' and not (skills / name).exists(), 'retirement failed')
                require((skills / ('.erii-retired-' + request['candidate_ref'].removeprefix('experience:')) / 'SKILL.md').is_file(),
                        'retirement lost audit copy')
                require(h.command('status')['agent_ingress_enabled'], 'retirement disabled configured startup')
                remaining = skill_list(h, name + '-retired-list')
                require(len(remaining) == 127 and not any(item['name'] == name for item in remaining),
                        'retirement did not release one active slot')
                return dict(retired=retired, remaining=remaining)

            lifecycles = []
            active_request = None
            active_name = None
            capacity = {}
            for skill_name in ('verified-cli', 'verified---cli-correction'):
                skill_description = 'Review code --- safely.'
                proposal = dict(operation='propose', name=skill_name, description=skill_description,
                                procedure='1. Read AGENTS.md and reproduce the missing CLI flag.\n2. Add the flag using the existing parser.\n3. Run go test -race ./... and inspect compatibility tests.',
                                reason='success', source_refs=['run:' + inv['run_id'] for inv in tests])
                candidate = h.command('skill', 'manage', text=json.dumps(proposal))
                request = dict(candidate_ref=candidate['candidate_ref'], version=candidate['version'])
                denied = h.command('skill', 'manage', text=json.dumps(dict(request, operation='promote')), ok=False)
                require(denied.returncode != 0, 'unverified experience promoted')
                for inv in tests:
                    candidate = h.command('skill', 'manage', text=json.dumps(dict(request, operation='verify',
                                                                                verification_ref='invocation:' + inv['invocation_id'])))
                    request['version'] = candidate['version']
                require(candidate['status'] == 'verified', 'independent verifications did not qualify')
                if active_request is not None:
                    rejected = h.command('skill', 'manage', text=json.dumps(dict(request, operation='promote')), ok=False)
                    require(rejected.returncode != 0 and 'RESOURCE_LIMIT' in rejected.stderr,
                            'full catalog accepted publication or returned an unrelated failure')
                    snapshot = readback(h)
                    row = next(row for row in snapshot['state']['candidates']
                               if row['candidate_id'] == request['candidate_ref'].removeprefix('experience:'))
                    require(row['status'] == 'verified' and row['version'] == request['version'] == 3,
                            'capacity rejection committed candidate status/version')
                    require(not (skills / skill_name).exists(), 'rejected publication left an active entry')
                    require(h.command('status')['agent_ingress_enabled'], 'capacity rejection disabled startup')
                    still_full = skill_list(h, 'capacity-rejected-list')
                    require(len(still_full) == 128, 'capacity rejection changed active catalog')
                    capacity = dict(rejection=rejected.stderr, candidate=row, active_after_rejection=len(still_full))
                    lifecycles[-1].update(retire_skill(active_request, active_name))
                promoted = h.command('skill', 'manage', text=json.dumps(dict(request, operation='promote')))
                require(promoted['status'] == 'promoted', 'verified procedure not published')
                require((root / 'skills' / skill_name / 'SKILL.md').is_file(), 'published Skill missing')
                # Each CLI command starts a new configured process. Prove publication
                # survives startup, exact catalog metadata readback and retirement.
                restarted = h.command('status')
                require(restarted['agent_ingress_enabled'], 'published metadata disabled configured startup')
                summaries = skill_list(h, skill_name + '-list')
                require(len(summaries) == 128, 'last-slot publication did not preserve catalog limit')
                summary = next((item for item in summaries if item['name'] == skill_name), None)
                require(summary and summary['description'] == skill_description,
                        'published punctuation metadata did not round-trip through catalog')
                viewed = call(h, skill_name + '-view', 'builtin__skill__view',
                              {'name': skill_name, 'version': summary['version']})
                require(content(viewed)['text'].endswith(proposal['procedure'] + '\n'),
                        'published Skill body changed across restart')
                lifecycles.append(dict(promoted=promoted, restarted=restarted, summary=summary,
                                       viewed=content(viewed), active_after_promotion=len(summaries)))
                active_request = dict(request, version=promoted['version'])
                active_name = skill_name
            lifecycles[-1].update(retire_skill(active_request, active_name))
            require(all(path.read_text() == 'Retained fixture bytes; not active metadata.\n' for path in retained),
                    'catalog maintenance changed retained history')
            report['scenarios'].append(dict(name='verified-skill-lifecycle', tests=tests,
                                            **lifecycles[0], lifecycles=lifecycles, capacity=capacity,
                                            retained_history_count=len(retained), catalog_profile_id=catalog_profile,
                                            catalog_context_window=131072))
            work(h)
            manage(h, 'forget-health-category', operation='forget', category='health')
            work(h)
            before = readback(h)
            h.send('health-after-opt-out', 'My health report: I slept poorly today')
            h.command('chat', 'work')
            work(h)
            after = readback(h)
            require(len(after['memory']['records']) == len(before['memory']['records']),
                    'automatic health Memory ignored category opt-out')
            report['scenarios'].append(dict(name='persistent-category-opt-out', before=before['memory']['records'],
                                            after=after['memory']['records']))
            report['final'] = readback(h)
        finally:
            if h is not None:
                h.cleanup()
            embeddings.close()
    report.update(passed=True, cleanup='CLI children reaped, HTTP Fake joined, disposable databases/workspace/Skills removed')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    result = run(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(dict(passed=True, scenarios=len(result['scenarios']), cleanup=result['cleanup'])))


if __name__ == '__main__':
    main()
