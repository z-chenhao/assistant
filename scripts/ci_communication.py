#!/usr/bin/env python3
"""Validate native communication receipts locally; join compact CI manifests.

Manifests prove deterministic gate completion only. Native bytes remain required
for independent Case recomputation; no semantic or release verdict is produced.
"""
import argparse
from collections import Counter
import hashlib
import importlib
import json
from pathlib import Path
import re
import subprocess

from ci_codemode import IDENTITY
from verify_suite import ROOT, SHARDS, report_name, source_state


def file_record(path):
    h = hashlib.sha256()
    with path.open('rb') as stream:
        for block in iter(lambda: stream.read(1 << 20), b''):
            h.update(block)
    return dict(path=path.name, bytes=path.stat().st_size, sha256=h.hexdigest())


def fixtures():
    names = subprocess.check_output(['git', 'ls-files', '-z', 'evals'], cwd=ROOT).decode().split('\0')
    return {name: file_record(ROOT / name)['sha256'] for name in names if name}


def pairs(gate):
    driver = importlib.import_module('verify_' + gate.name)
    if gate.name in ('gc01', 'gc02'):
        from case_runner import variants
        data = driver._case_dataset() if gate.name == 'gc01' else driver.inputs()
        return [(v['variant_id'], n) for v in variants(*data) for n in range(1, 6)]
    ids = [v['id'] for v in driver.variants()]
    if gate.name == 'gc12':
        ids += ['denied', 'lost-reply']
    return [(key, 1) for key in ids]


def check_suite(suite, part, source):
    from verify_suite import communication_gates
    if (suite.get('passed') is not True or suite.get('source_stable') is not True or
            suite.get('source_dirty') is not False or suite.get('selected_gates') is not None or
            suite.get('communication_part') != part or suite.get('complete_inventory') is not False or
            set(suite.get('shards', {})) != {'communication'} or
            any(suite.get(k) is not None for k in ('codemode_part', 'personal_part', 'provider_part')) or
            any(suite.get(k) != v for k, v in source.items()) or
            not re.fullmatch('[0-9a-f]{64}', suite.get('binary_sha256', ''))):
        raise ValueError('failed, dirty, focused or drifted communication part: ' + part)
    rows = suite['shards']['communication']
    gates = communication_gates(part)
    expected = [(g.name, n) for g in gates for n in range(1, g.attempts + 1)]
    if ([(r['gate'], r['attempt']) for r in rows] != expected or
            any(type(r['attempt']) is not int or type(r.get('returncode')) is not int or
                r['returncode'] != 0 or r.get('owned_group_drained') is not True for r in rows)):
        raise ValueError('missing, duplicate, reordered, failed or unclean communication repetition')
    return gates, rows


def native_receipt(gate, attempt, folder, binary, suite):
    driver = importlib.import_module('verify_' + gate.name)
    path = folder / report_name(gate, attempt)
    report = driver.read_report(path) if gate.name in ('gc02','gc08') else json.loads(path.read_text())
    if gate.name in ('gc01', 'gc02'):
        driver.validate_report(report, binary)
        rows = report['results']
    else:
        from prepare_release_eval import prepare
        prepare(report)  # Recompute existing native oracles; stored flags cannot override them.
        rows = report['variants']
        if gate.name == 'gc12':
            if report.get('binary_sha256') != suite['binary_sha256']:
                raise ValueError('GC-12 binary drift')
            fixture_sha = file_record(driver.FIXTURE)['sha256']
            if any(r.get('fixture_sha256') != fixture_sha for r in rows):
                raise ValueError('GC-12 fixture drift')
        else:
            proof = report['source_provenance']
            expected = {k: suite[k] for k in ('source_sha', 'source_dirty', 'source_diff_sha256', 'binary_sha256')}
            if proof.get('before') != expected or proof.get('after') != expected:
                raise ValueError('native source/binary drift')
    receipts = []
    for row in rows:
        key = row['variant_id'] if gate.name in ('gc01', 'gc02') else row['variant']['id']
        checks = dict(row['assertions'])
        if gate.name == 'gc08':
            for field in ('denial_assertions', 'lifecycle_assertions', 'completion_assertions'):
                checks.update({field + '/' + k: v for k, v in row[field].items()})
            # All four native safety branches are recomputed by prepare().
            for probe in report['safety_probes']:
                checks.update({'safety/'+probe['safety']['mode']+'/'+k:v for k,v in probe['safety_assertions'].items()})
        receipts.append(dict(variant_id=key, repetition=row.get('repetition', 1),
                             assertions=checks, cleanup=row['cleanup']))
    raw = path
    if gate.name in ('gc02','gc08'):
        raw = path.parent / json.loads(path.read_text())['native']['path']
    return dict(gate=gate.name, attempt=attempt, native=file_record(raw),
                status=report['status'], semantic_score=report['semantic_score'],
                cleanup=report['cleanup'], rows=receipts)


def check_receipts(receipts, gates):
    expected = [(g.name, n) for g in gates for n in range(1, g.attempts + 1)]
    if [(r['gate'], r['attempt']) for r in receipts] != expected:
        raise ValueError('missing, duplicate or reordered native reports')
    for receipt in receipts:
        gate = next(g for g in gates if g.name == receipt['gate'])
        native = receipt['native']
        if (receipt.get('status') != 'mechanical_pass' or receipt.get('cleanup') is not True or
                'semantic_score' not in receipt or receipt['semantic_score'] is not None or
                type(native.get('bytes')) is not int or native['bytes'] <= 0 or
                not re.fullmatch('[0-9a-f]{64}', native.get('sha256', '')) or
                native.get('path') != report_name(gate, receipt['attempt'])):
            raise ValueError('failed, graded, unclean or missing native report')
        rows = receipt['rows']
        if [(r['variant_id'], r['repetition']) for r in rows] != pairs(gate):
            raise ValueError('missing, duplicate or reordered communication variants')
        cleanup = dict(owned_processes_joined=True, disposable_root_removed=True)
        if gate.name == 'gc12':
            cleanup['http_peer_joined'] = True
        for row in rows:
            if (row.get('cleanup') != cleanup or type(row['repetition']) is not int or
                    not row.get('assertions') or
                    any(v is not True for v in row['assertions'].values()) or
                    any(v is not True for v in row['cleanup'].values())):
                raise ValueError('failed assertion or unclean native variant')


def collect(folder, part, binary):
    source = source_state()
    suite_path = folder / f'suite-communication-{part}.json'
    suite = json.loads(suite_path.read_text())
    gates, _ = check_suite(suite, part, source)
    if file_record(binary)['sha256'] != suite['binary_sha256']:
        raise ValueError('binary changed before native validation')
    locked = fixtures()
    receipts = [native_receipt(g, n, folder, binary, suite) for g in gates for n in range(1, g.attempts + 1)]
    check_receipts(receipts, gates)
    if source != source_state() or locked != fixtures() or file_record(binary)['sha256'] != suite['binary_sha256']:
        raise ValueError('source/fixture/binary changed during native validation')
    return dict(version=1, part=part, suite=file_record(suite_path),
                identity={k: suite[k] for k in IDENTITY}, fixtures=locked, receipts=receipts,
                proof_limit='Deterministic CI only; retain native bytes for independent recomputation. No semantic/release PASS.')


def join(root):
    from verify_suite import COMMUNICATION_PARTS
    source = source_state()
    if source['source_dirty'] or {p.name for p in root.iterdir()} != set(COMMUNICATION_PARTS):
        raise ValueError('clean source and exactly the required communication parts required')
    suites, manifests, rows = {}, {}, []
    locked = fixtures()
    for part in COMMUNICATION_PARTS:
        folder = root / part
        path = folder / f'suite-communication-{part}.json'
        suite = json.loads(path.read_text())
        gates, members = check_suite(suite, part, source)
        manifest = json.loads((folder / 'communication-receipts.json').read_text())
        if (manifest.get('version') != 1 or manifest.get('part') != part or
                manifest.get('suite') != file_record(path) or manifest.get('fixtures') != locked or
                manifest.get('identity') != {k: suite[k] for k in IDENTITY}):
            raise ValueError('communication manifest/source/fixture drift')
        check_receipts(manifest['receipts'], gates)
        suites[part] = suite
        manifests[part] = manifest
        rows.extend(members)
    first = suites[COMMUNICATION_PARTS[0]]
    if any(any(s.get(k) != first.get(k) for k in IDENTITY) for s in suites.values()):
        raise ValueError('communication source/build/workflow drift')
    expected = [(g.name, n) for g in SHARDS['communication'] for n in range(1, g.attempts + 1)]
    if Counter((r['gate'], r['attempt']) for r in rows) != Counter(expected):
        raise ValueError('logical communication inventory drift')
    order = {key: i for i, key in enumerate(expected)}
    rows.sort(key=lambda r: order[r['gate'], r['attempt']])
    return dict(first, communication_part=None, complete_inventory=True,
                shards={'communication': rows}, parts=suites, native_receipts=manifests,
                elapsed_seconds=max(s['elapsed_seconds'] for s in suites.values()))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='operation', required=True)
    collect_parser = sub.add_parser('collect')
    collect_parser.add_argument('--part', required=True)
    collect_parser.add_argument('--binary', type=Path, required=True)
    collect_parser.add_argument('--evidence', type=Path, required=True)
    join_parser = sub.add_parser('join')
    join_parser.add_argument('--parts', type=Path, required=True)
    join_parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    output = args.evidence / ('communication-receipts.json' if args.operation == 'collect' else 'suite-communication.json')
    output.unlink(missing_ok=True)
    report = collect(args.evidence, args.part, args.binary) if args.operation == 'collect' else join(args.parts)
    output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(dict(passed=True, operation=args.operation, output=str(output))))


if __name__ == '__main__':
    main()
