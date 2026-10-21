"""Aggregate exact M6 repetitions; mechanical success never supplies a score."""
from collections import defaultdict
import hashlib
import json
from pathlib import Path

CASES = ('GC-01','GC-02','GC-03','GC-04','GC-05','GC-06','GC-07','GC-10','GC-13','GC-15','GC-18')
VARIANTS = ('wording-1','wording-2','wording-3','noise-1','noise-2','order-1')

# Private CI admission groups; every pair remains in exactly one five-run series.
PARTS = {name: tuple((case, variant) for case in cases for variant in VARIANTS)
         for name, cases in {'context':('GC-01','GC-04','GC-05','GC-10','GC-15'),
                             'memory':('GC-02','GC-03','GC-06'),
                             'network':('GC-07',), 'booking':('GC-13',)}.items()}
PARTS.update({f'desktop-{i+1}':tuple(('GC-18',v) for v in VARIANTS[i*2:i*2+2]) for i in range(3)})


def aggregate(reports):
    if len(reports) != 5:
        raise ValueError('five consecutive M6 reports required')
    signatures=set()
    groups=defaultdict(list)
    expected={(c,v) for c in CASES for v in VARIANTS}
    for report in reports:
        if not report.get('cleanup'):
            raise ValueError('cleanup missing')
        signatures.add((report['binary_sha256'],report['dataset_digest'],report['driver_digest'],json.dumps(report['contract_lock'],sort_keys=True)))
        rows=report['results']
        if len(rows)!=66 or {(r['case_id'],r['variant_id']) for r in rows}!=expected:
            raise ValueError('all eleven exact cases and six variants required')
        for row in rows:
            if row.get('final_score') is not None:
                raise ValueError('scripted Fake cannot supply a product score')
            groups[row['case_id'],row['variant_id'],row['fixture_digest']].append(row)
    if len(signatures)!=1 or len(groups)!=66 or any(len(series)!=5 for series in groups.values()):
        raise ValueError('binary/fixture/repetition drift')
    def passed(row):
        checks=row['assertions']
        return bool(checks) and all(type(v) is bool and v for v in checks.values()) and not row['hard_failure'] and row['status']=='fragment_pass'
    cases={}
    for case in CASES:
        series=[rows for key,rows in groups.items() if key[0]==case]
        if any(len({tuple(sorted(r['assertions'])) for r in rows})!=1 for rows in series):
            raise ValueError('assertion inventory drift')
        cases[case]=dict(runs=30,mechanical={'pass@1':sum(passed(s[0]) for s in series)/6,
                                          'pass^5':sum(all(passed(r) for r in s) for s in series)/6},
                         final_score=None,dimensions={d:None for d in 'OFPATSRC'},
                         semantic_metrics={'pass@1':None,'pass^5':None},scope='mechanical L0/L1 fragment; semantic evaluation inconclusive')
    return dict(version=1,cases=cases,passed=all(c['mechanical']['pass^5']==1 for c in cases.values()),
                product_gate='inconclusive; no semantic Judge or current L2',suite_mean=None,suite_min=None,
                not_applicable=['M7 scheduled/channel portions'],live_shadow=reports[0]['live_shadow'])


def write(evidence):
    evidence=Path(evidence)
    paths=[evidence/f'm6-cases-{i}.json' for i in range(1,6)]
    summary=aggregate([json.loads(p.read_text()) for p in paths])
    summary['inputs']=[dict(path=p.name,sha256=hashlib.sha256(p.read_bytes()).hexdigest()) for p in paths]
    (evidence/'m6-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n')
    return summary['passed']
