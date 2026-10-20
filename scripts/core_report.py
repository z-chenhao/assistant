"""Aggregate five exact fixture runs without turning Fake quality into scores."""
from collections import defaultdict
import hashlib
import json
from pathlib import Path


def aggregate(reports):
    if len(reports) != 5:
        raise ValueError('five complete consecutive reports required')
    signatures = set()
    rows = defaultdict(list)
    for report in reports:
        if not report.get('cleanup') or not report.get('conformance', {}).get('passed'):
            raise ValueError('cleanup/conformance evidence missing or failed')
        signatures.add((report['binary_sha256'], report['dataset_digest'], json.dumps(report['contract_lock'], sort_keys=True)))
        seen = set()
        for result in report['results']:
            key = result['case_id'], result['variant_id'], result['fixture_digest']
            if key in seen:
                raise ValueError('duplicate variant')
            seen.add(key)
            rows[key].append(result)
        if len(seen) != 18:
            raise ValueError('all 3 cases × 6 variants required')
    if len(signatures) != 1 or any(len(series) != 5 or len({tuple(sorted(row['assertions'])) for row in series}) != 1 for series in rows.values()):
        raise ValueError('frozen binary/fixture or repetition drift')
    cases = {}
    for case in ('GC-17', 'GC-19', 'GC-20'):
        series = [rs for key, rs in rows.items() if key[0] == case]
        if len(series) != 6:
            raise ValueError('missing case variants')
        def passed(row):
            assertions = row['assertions']
            return bool(assertions) and all(type(v) is bool and v for v in assertions.values()) and not row['hard_failure'] and row['status'] == 'fragment_pass'
        cases[case] = dict(repetitions=5, variants=6, runs=30,
                          hard_assertions={'pass@1':sum(passed(rs[0]) for rs in series)/6,
                                           'pass^5':sum(all(passed(r) for r in rs) for rs in series)/6},
                          hard_failures=sum(not passed(r) for rs in series for r in rs),
                          final_score=None, dimensions={d:None for d in 'OFPATSRC'},
                          product_metrics={'pass@1':None,'pass^5':None},
                          score_status='ungraded; scripted Fake cannot provide semantic Judge scores',
                          scope='L0/L1 mechanical fragment; full case not_applicable')
    return dict(version=1, cases=cases, passed=all(c['hard_assertions']['pass^5'] == 1 for c in cases.values()),
                suite_mean=None, suite_min=None, product_gate='inconclusive; semantic Judge and L2 unexecuted',
                real_adapters_due_M6=[],
                conformance=reports[0]['conformance']['requirements'],
                boundary_fakes=reports[0]['conformance']['boundary_fakes'])


def write(evidence):
    evidence = Path(evidence)
    paths = [evidence/f'core-cases-{i}.json' for i in range(1,6)]
    report = aggregate([json.loads(p.read_text()) for p in paths])
    report['inputs'] = [{'path':p.name,'sha256':hashlib.sha256(p.read_bytes()).hexdigest()} for p in paths]
    (evidence/'core-summary.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    return report['passed']
