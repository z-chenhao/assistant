#!/usr/bin/env python3
"""Decide v1 evidence completeness from an independently acquired, pinned pack.

This private consumer never creates evidence or authenticates its own assertions.
Missing observations remain inconclusive. Invalid bindings fail closed.
"""
import argparse
from collections import defaultdict
from datetime import datetime, timezone
from functools import lru_cache
import json
from pathlib import Path
import statistics

from case_runner import digest, load_contract
import ci_gate
import release_audit
from release_receipts import (ARMS, CASES, DIMENSIONS, VARIANTS, Receipts, checks,
                              git, hash_value, hashed, model_identity, require, review,
                              strict_json, timestamp, trial)

ROOT = Path(__file__).resolve().parents[1]
PROTECTED = {'GC-12', 'GC-13', 'GC-17', 'GC-19', 'GC-20'}
SYSTEMS = {f'SC-{n:02}' for n in range(1, 13)}
PROTECTED_SYSTEMS = {f'SC-{n:02}' for n in range(3, 8)}
SAFETY = {'security', 'approval', 'cancellation', 'durable_replay', 'coding_workspace'}
OPERATIONS = {'kill_restart', 'backup_restore', 'upgrade', 'test', 'lint', 'race',
              'sidecar', 'e2e', 'fault_injection'}
LIVE = {'search', 'login', 'mail', 'calendar', 'feishu', 'browser', 'computer', 'jianying'}
CONDITIONS = {'fixture_sha256', 'input_sha256', 'context_builder_sha256', 'context_sha256',
              'capability_sha256', 'tools_sha256', 'skills_sha256', 'limits', 'seed', 'timezone',
              'binary_sha256', 'configuration_sha256'}


def source_key(row):
    return digest({key: row[key] for key in ('source', 'section', 'line', 'sha256')})


@lru_cache(maxsize=512)
def source_blob(root, sha, path):
    return git(root, 'show', f'{sha}:{path}')


@lru_cache(maxsize=256)
def merged_ancestor(root, commit, sha):
    git(root, 'merge-base', '--is-ancestor', commit, sha)


def current_reference(root, sha, ref):
    require(set(ref) == {'path', 'sha256', 'symbol'}, 'exact code/test reference required')
    path = Path(ref['path'])
    require(not path.is_absolute() and '..' not in path.parts, 'unsafe code reference')
    data = source_blob(str(root), sha, path.as_posix())
    require(hashed(data) == ref['sha256'] and bool(ref['symbol']) and
            ref['symbol'].encode() in data, 'code/test reference mismatch')
    return ref


def clause_results(root, store, units, entries, sha, now):
    by_key = {source_key(u): u for u in units}
    results = {key: dict(source=u['source'], section=u['section'], line=u['line'],
                         sha256=u['sha256'], status='unmet', reason='no current positive evidence')
               for key, u in by_key.items()}
    seen = set()
    for entry in entries:
        key = entry['source_key']
        require(key in by_key and key not in seen, 'unknown/stale/duplicate source clause')
        seen.add(key)
        require(entry['disposition'] in ('verified', 'unmet', 'rejected', 'expired'), 'unknown clause disposition')
        row = results[key]
        row.update(status=entry['disposition'], reason=entry['reason'])
        require(bool(entry['reason']), 'clause reason required')
        if entry['disposition'] != 'verified':
            continue
        for field in ('code', 'tests'):
            require(bool(entry[field]), 'current code and test required for each clause')
            for ref in entry[field]:
                current_reference(root, sha, ref)
        pr = store.read(entry['merged_pr'], 'merged_pr', unique=False)
        require(pr.get('repository') == 'z-chenhao/erii' and pr.get('state') == 'MERGED' and
                bool(pr.get('merged_at')) and bool(pr.get('merged_by')) and
                type(pr.get('number')) is int and pr['number'] > 0 and
                pr.get('url') == f"https://github.com/z-chenhao/erii/pull/{pr['number']}",
                'merged PR provenance required')
        require(timestamp(pr['merged_at']) <= now, 'future PR merge')
        metadata = strict_json(store.bytes(pr['native_response']))
        require(metadata['number'] == pr['number'] and metadata['url'] == pr['url'] and
                metadata['state'] == 'MERGED' and metadata['mergedAt'] == pr['merged_at'] and
                metadata['mergedBy']['login'] == pr['merged_by'] and
                metadata['mergeCommit']['oid'] == pr['merge_commit'] and
                metadata['repository']['nameWithOwner'] == pr['repository'], 'native PR metadata mismatch')
        merged_ancestor(str(root), pr['merge_commit'], sha)
        store.raw(pr['raw_evidence'])
        native = store.read(entry['native'], 'clause_evidence', unique=False)
        require(native.get('source_key') == key and native.get('producer') and native.get('command') and native.get('author'),
                'clause evidence must identify exact source and producer')
        store.raw(native['raw_evidence'])
        passed = checks(native['assertions'])
        subject = digest({k: entry[k] for k in ('source_key', 'code', 'tests', 'merged_pr', 'native')})
        state = review(store, entry['review'], subject, now, native.get('author'))
        row.update(status=state if passed else 'rejected', code=entry['code'], tests=entry['tests'],
                   merged_pr=entry['merged_pr'], native=entry['native'], review=entry['review'])
    return results


def observed_gate(store, ref, name, now, live=False, models=None):
    value = store.read(ref, 'live_observation' if live else 'gate')
    require(value.get('name') == name and value.get('producer') and value.get('command') and value.get('author'), 'gate identity missing')
    execution = (name, value.get('execution_id'))
    require(execution[1] and execution not in store.gate_executions, 'missing/reused native gate execution')
    store.gate_executions.add(execution)
    store.raw(value['raw_evidence'])
    require(timestamp(value['observed_at']) <= now, 'future observation')
    require(timestamp(value['observed_at']) >= store.earliest, 'observation predates release source')
    state = review(store, value['review'], digest({k: v for k, v in value.items() if k != 'review'}),
                   now, value.get('author'), human=live)
    if live:
        require(value.get('level') == 'L2' and value.get('environment') and
                value.get('failure_class') is not None and value.get('model_snapshot') and
                value.get('actual_state') and value.get('artifacts'), 'complete current L2 observation required')
        require(model_identity(value['model_snapshot']) in models.values(), 'L2 model differs from approved comparison models')
        store.raw(value['artifacts'])
    return state if checks(value['assertions']) else 'rejected'


def aggregate_trials(store, rows, plan, policy, models):
    groups = defaultdict(list)
    identities, executions = set(), set()
    for row in rows:
        result = trial(store, row['trial'], row['judge'], plan, policy, models)
        identity = result['identity']
        key = tuple(identity[k] for k in ('arm', 'case_id', 'variant_id', 'repetition'))
        require(key not in identities and result['execution_id'] not in executions, 'duplicate/reused native execution')
        identities.add(key)
        executions.add(result['execution_id'])
        groups[key[:2]].append(result)
    expected = {(arm, case, variant, rep) for arm in ARMS for case in CASES
                for variant in VARIANTS for rep in range(1, 6)}
    cases, arms = {}, {}
    for arm in sorted(ARMS):
        arm_cases = {}
        for case in sorted(CASES):
            values = groups[arm, case]
            scores = [v['exact_score'] for v in values]
            complete = len(values) == 30 and all(n is not None for n in scores)
            # Missing/null never becomes zero or success. A recorded mechanical
            # hard failure remains an observed fail, even if other scores lack.
            failed = any(v['status'] == 'fail' for v in values)
            state = 'rejected' if failed else ('verified' if complete else 'unmet')
            series = {variant: sorted([v for v in values if v['identity']['variant_id'] == variant],
                                      key=lambda v: v['identity']['repetition']) for variant in VARIANTS}
            passed = lambda v: v['status'] == 'pass'
            arm_cases[case] = dict(status=state, runs=len(values),
                score=statistics.mean(scores) if complete else None,
                minimum=min(scores) if complete else None,
                variance=statistics.pvariance(scores) if complete else None,
                pass_at_1=sum(passed(s[0]) for s in series.values()) / 6 if complete else None,
                pass_power_5=sum(all(passed(v) for v in s) for s in series.values()) / 6 if complete else None,
                variants={variant: dict(scores=[v['exact_score'] for v in s],
                    variance=statistics.pvariance([v['exact_score'] for v in s]) if complete else None)
                          for variant, s in series.items()},
                hard_failure_count=sum(bool(v['hard_failures']) for v in values),
                dimensions={d: dict(mean=statistics.mean([v['dimensions'][d] for v in values]),
                                    minimum=min(v['dimensions'][d] for v in values))
                            if complete and d in plan[case]['applicable_dimensions'] else None for d in DIMENSIONS},
                metrics=[dict(identity=v['identity'], **v['metrics']) for v in values])
        complete = all(v['score'] is not None for v in arm_cases.values())
        mean = statistics.mean([v['score'] for v in arm_cases.values()]) if complete else None
        minimum = min(v['minimum'] for v in arm_cases.values()) if complete else None
        arms[arm] = dict(model=models[arm], suite_mean=mean, suite_min=minimum,
            dimensions={d: dict(mean=statistics.mean([v['dimensions'][d]['mean'] for v in arm_cases.values()
                                                      if v['dimensions'][d] is not None]),
                                minimum=min(v['dimensions'][d]['minimum'] for v in arm_cases.values()
                                            if v['dimensions'][d] is not None))
                        if complete and any(v['dimensions'][d] is not None for v in arm_cases.values()) else None
                        for d in DIMENSIONS},
            metrics_complete=all(v['metrics_complete'] for (a, _), group in groups.items() if a == arm for v in group),
            status='rejected' if any(v['status'] == 'rejected' for v in arm_cases.values()) or
                (complete and mean < policy['suite_mean_min']) else ('verified' if complete else 'unmet'))
        cases[arm] = arm_cases
    return dict(cases=cases, arms=arms, complete=identities == expected,
                protected_cases=sorted(PROTECTED), required_repetitions=5)


def decide(root, pack_path, expected_hash, now=None):
    now = now or datetime.now(timezone.utc)
    data = pack_path.read_bytes()
    require(hashed(data) == expected_hash, 'pack does not match independently supplied hash')
    pack = strict_json(data)
    require(pack.get('version') == 1, 'unsupported private pack')
    sha = git(root, 'rev-parse', 'HEAD').decode().strip()
    require(pack['binding']['release_sha'] == sha, 'stale release SHA')
    ledger = strict_json((root / 'docs/v1-release-ledger.json').read_bytes())
    inventory = release_audit.audit(root, ledger)
    sources = {p: v['sha256'] for p, v in inventory['sources'].items()}
    policy_path = root / 'evals/scoring-v1.json'
    _, policy, _ = load_contract(root)
    require(policy_path.read_bytes() == source_blob(str(root), sha, 'evals/scoring-v1.json'),
            'scoring policy differs from release commit')
    suite = pack['suite']
    binding = dict(release_sha=sha, sources=sources, scoring_sha256=hashed(policy_path.read_bytes()),
                   suite_sha256=digest(suite))
    require(pack['binding'] == binding, 'source/scoring/suite binding mismatch')
    store = Receipts(pack_path.parent, binding,
                     timestamp(git(root, 'show', '-s', '--format=%cI', sha).decode().strip()))
    gates = {}
    gates['scope_review'] = review(store, pack['scope_review'], digest(suite), now, pack['author'])
    require(set(suite) == {'cases', 'models', 'systems', 'safety'}, 'release suite inventory mismatch')
    require(set(suite['cases']) == CASES and set(suite['models']) == ARMS and
            set(suite['systems']) == SYSTEMS and set(suite['safety']) == SAFETY, 'full v1 suite required')
    models = {arm: model_identity(suite['models'][arm]) for arm in ARMS}
    require(models['local']['provider_kind'] == 'openai_compatible' and
            models['account']['provider_kind'] == 'openai_account', 'local/account comparison arms required')
    for case, definition in suite['cases'].items():
        require(bool(definition['assertions']) and len(set(definition['assertions'])) == len(definition['assertions']),
                'reviewed whole-case assertions required')
        require(set(definition['conditions']) == VARIANTS, 'all wording/noise/order variants required')
        require(set(definition['not_applicable']) == DIMENSIONS - set(definition['applicable_dimensions']),
                'all dimension dispositions required')
        for conditions in definition['conditions'].values():
            require(set(conditions) == CONDITIONS and all(v is not None for v in conditions.values()),
                    'incomplete comparison conditions')
            require(all(hash_value(v) for k, v in conditions.items() if k.endswith('_sha256')),
                    'invalid comparison hash')
    clauses = clause_results(root, store, inventory['source_units'], pack['clauses'], sha, now)
    gates['source_clauses'] = 'verified' if all(v['status'] == 'verified' for v in clauses.values()) else 'unmet'
    evaluation = aggregate_trials(store, pack['trials'], suite['cases'], policy, models)
    gates['golden_cases'] = 'verified' if evaluation['complete'] and all(
        v['status'] == 'verified' for v in evaluation['arms'].values()) else 'unmet'
    gates['reporting_metrics'] = 'verified' if evaluation['complete'] and all(
        v['metrics_complete'] for v in evaluation['arms'].values()) else 'unmet'
    for category, names in [('systems', SYSTEMS), ('safety', SAFETY)]:
        receipts = pack.get(category, {})
        require(set(receipts) <= names, 'unknown system/safety gate')
        for name in sorted(names):
            rows = receipts.get(name, [])
            minimum = 5 if name in PROTECTED_SYSTEMS or category == 'safety' else 1
            rounds = []
            states = []
            for ref in rows:
                value = store.read(ref, 'gate', unique=False)
                require(set(value['assertions']) == set(suite[category][name]), 'system/safety assertion inventory drift')
                rounds.append(value['repetition'])
                states.append(observed_gate(store, ref, name, now))
            require(rounds == list(range(1, len(rounds) + 1)), 'duplicate/out-of-order protected rounds')
            gates[name] = ('rejected' if 'rejected' in states else 'expired' if 'expired' in states else
                           'verified' if len(rows) >= minimum and all(s == 'verified' for s in states) else 'unmet')
    for category, names in [('operations', OPERATIONS), ('live', LIVE)]:
        receipts = pack.get(category, {})
        require(set(receipts) <= names, 'unknown operations/live gate')
        for name in sorted(names):
            gates[name] = observed_gate(store, receipts[name], name, now, live=category == 'live', models=models) if name in receipts else 'unmet'
    # GitHub-native snapshots must be acquired through an authenticated read by
    # the pack authority. A green string/PR count cannot stand in for this join.
    gates['native_ci'] = 'unmet'
    if pack.get('native_ci'):
        ci = store.read(pack['native_ci'], 'native_ci')
        require(set(ci['platforms']) == {'ubuntu', 'macos'}, 'both native platforms required')
        passed = True
        for platform, ref in ci['platforms'].items():
            run = strict_json(store.bytes(ref))
            require(run['repository']['full_name'] == 'z-chenhao/erii' and run['head_sha'] == sha and
                    type(run['id']) is int and run['id'] > 0 and type(run['run_attempt']) is int and
                    run['run_attempt'] > 0 and run['html_url'] == f"https://github.com/z-chenhao/erii/actions/runs/{run['id']}",
                    'native CI run identity mismatch')
            require(run['path'] == f".github/workflows/{'ci' if platform == 'ubuntu' else 'release'}.yml",
                    'wrong native CI workflow')
            jobs = run['jobs']
            require(bool(jobs) and len({j['id'] for j in jobs}) == len(jobs) and
                    len({j['name'] for j in jobs}) == len(jobs) and
                    all(j['run_id'] == run['id'] and j['run_attempt'] == run['run_attempt'] for j in jobs),
                    'CI jobs missing, duplicate or from another run/attempt')
            required = {'Ubuntu checks'} if platform == 'ubuntu' else {
                'Package (ubuntu-24.04)', 'Package (ubuntu-24.04-arm)', 'Package (macos-15)', 'Package (macos-15-intel)'}
            require(required <= {j['name'] for j in jobs}, 'native CI job inventory missing')
            passed &= all(j['conclusion'] == 'success' for j in jobs if j['name'] in required)
            passed &= run['status'] == 'completed' and run['conclusion'] == 'success'
        join = strict_json(store.bytes(ci['ubuntu_needs']))
        gates['native_ci'] = 'verified' if passed and ci_gate.passed(join) else 'rejected'
    gates['final_human_acceptance'] = review(store, pack['human_acceptance'],
        digest({k: v for k, v in pack.items() if k != 'human_acceptance'}), now, pack['author'], human=True) if pack.get('human_acceptance') else 'unmet'
    ready = all(state == 'verified' for state in gates.values())
    rejected = (any(state == 'rejected' for state in gates.values()) or
                any(row['status'] == 'rejected' for row in clauses.values()) or
                any(row['status'] == 'rejected' for row in evaluation['arms'].values()))
    return dict(version=1, release_sha=sha, evidence_pack_sha256=expected_hash,
                status='pass' if ready else 'rejected' if rejected else 'inconclusive', release_ready=ready,
                gates=gates, source_clauses=clauses,
                individually_verified_requirements=sum(v['status'] == 'verified' for v in clauses.values()),
                evaluation=evaluation, legacy_inventory=inventory,
                trust='Externally pinned evidence and named reviews; hashes do not authenticate acquisition. No publish/tag/deploy.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pack', type=Path, required=True)
    parser.add_argument('--pack-sha256', required=True, help='trusted hash from independent acquisition/review')
    parser.add_argument('--evidence', type=Path, default=ROOT / 'evidence/release-decision.json')
    parser.add_argument('--require-complete', action='store_true')
    args = parser.parse_args()
    output = args.evidence.resolve()
    require(output.is_relative_to((ROOT / 'evidence').resolve()) and output != (ROOT / 'evidence').resolve(),
            'output must be a disposable file under evidence/')
    require(output != args.pack.resolve(), 'output cannot overwrite input pack')
    require(not output.is_relative_to(args.pack.parent.resolve()), 'output must be outside the input pack directory')
    output.unlink(missing_ok=True)
    try:
        report = decide(ROOT, args.pack, args.pack_sha256)
    except (ValueError, KeyError, TypeError, OSError, IndexError) as error:
        report = dict(version=1, status='rejected', release_ready=False, error=str(error))
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2, allow_nan=False) + '\n')
    print(json.dumps({k: report[k] for k in ('status', 'release_ready')}))
    return 1 if report['status'] == 'rejected' else (2 if args.require_complete and not report['release_ready'] else 0)


if __name__ == '__main__':
    raise SystemExit(main())
