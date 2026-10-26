#!/usr/bin/env python3
"""Join fixed delivery parts, preserving the native GC-07 validation boundary.

Current main has only delivery/channels. Empty Case parts explicitly mean
not_registered, never a GC-07 PASS. A registered consumer must accept --ci-part,
read native bytes and validate_report(report, binary, part=part) using its oracles.
Compact manifests prove CI completion; they cannot replace native Case evidence.
"""
import argparse
import importlib
import json
import os
from pathlib import Path
import re

from ci_codemode import IDENTITY
from ci_communication import file_record, fixtures
from verify_suite import (DELIVERY_PARTS, GC07_VARIANTS, SHARDS, delivery_gates,
                          gc07_pairs, report_name, source_state)

DELIVERY_IDENTITY = (*IDENTITY, 'run_id', 'run_attempt')
OTHER_PARTS = ('codemode_part', 'personal_part', 'provider_part', 'communication_part', 'security_part')


def run_identity():
    return {key: os.environ.get(env) for key, env in
            (('workflow_sha', 'GITHUB_SHA'), ('run_id', 'GITHUB_RUN_ID'), ('run_attempt', 'GITHUB_RUN_ATTEMPT'))}


def check_suite(suite, part, source, run):
    if (suite.get('passed') is not True or suite.get('source_stable') is not True or
            suite.get('source_dirty') is not False or suite.get('selected_gates', []) is not None or
            suite.get('complete_inventory') is not False or suite.get('delivery_part') != part or
            any(suite.get(k) is not None for k in OTHER_PARTS) or
            set(suite.get('shards', {})) != {'delivery'} or
            type(suite.get('jobs')) is not int or suite['jobs'] != 1 or
            type(suite.get('gate_jobs')) is not int or suite['gate_jobs'] != 2 or
            any(suite.get(k) != v for k, v in {**source, **run}.items()) or
            any(k not in suite for k in DELIVERY_IDENTITY) or
            not re.fullmatch('[0-9a-f]{40}', suite.get('source_sha', '')) or
            not re.fullmatch('[0-9a-f]{64}', suite.get('binary_sha256', ''))):
        raise ValueError('failed, focused, dirty or drifted delivery part: ' + part)
    gates = delivery_gates(part)
    rows = suite['shards']['delivery']
    if ([(r['gate'], r['attempt']) for r in rows] !=
            [(g.name, n) for g in gates for n in range(1, g.attempts + 1)] or
            any(type(r['attempt']) is not int or type(r.get('returncode')) is not int or
                r['returncode'] != 0 or r.get('owned_group_drained') is not True for r in rows)):
        raise ValueError('missing, duplicate, reordered, failed or unclean delivery round')
    for gate in gates:
        for row in (r for r in rows if r['gate'] == gate.name):
            command = row.get('command', [])
            if (len(command) != 7 + len(gate.arguments) or
                    command[1:4] != ['scripts/profile_harness.py', 'scripts/' + gate.script, '--binary'] or
                    command[5] != '--evidence' or Path(command[6]).name != report_name(gate, row['attempt']) or
                    command[7:] != list(gate.arguments)):
                raise ValueError('delivery command/parameter drift')
    return gates, rows


def check_case(receipt, part):
    if (receipt.get('status') != 'mechanical_pass' or receipt.get('cleanup') is not True or
            receipt.get('release_ready') is not False or receipt.get('eligible_for_grading') is not False or
            'semantic_score' not in receipt or receipt['semantic_score'] is not None or
            not receipt.get('contract') or not receipt.get('rejection_probes') or
            any(v is not True for v in receipt['rejection_probes'].values())):
        raise ValueError('failed, graded, unclean or unvalidated GC-07 receipt')
    rows = receipt['rows']
    if [(r['variant_id'], r['repetition']) for r in rows] != gc07_pairs(part):
        raise ValueError('missing, duplicate, misplaced or reordered GC-07 repetition')
    for row in rows:
        if (type(row['repetition']) is not int or not row.get('assertions') or
                any(v is not True for v in row['assertions'].values()) or
                row.get('cleanup') != dict(disposable_root_removed=True, owned_processes_joined=True) or
                any(v is not True for v in row['cleanup'].values())):
            raise ValueError('failed assertion or unclean GC-07 variant')


def collect(folder, part, binary):
    source, run, locked = source_state(), run_identity(), fixtures()
    path = folder / f'suite-delivery-{part}.json'
    suite = json.loads(path.read_text())
    gates, _ = check_suite(suite, part, source, run)
    if file_record(binary)['sha256'] != suite['binary_sha256']:
        raise ValueError('binary drift before delivery collection')
    receipts = []
    for gate in gates:
        for attempt in range(1, gate.attempts + 1):
            report_path = folder / report_name(gate, attempt)
            receipt = dict(gate=gate.name, attempt=attempt, report=file_record(report_path))
            if part != 'control':
                driver = importlib.import_module('verify_gc07')
                report = driver.read_report(report_path)
                # The consumer owns native oracles. Old all-30-only drivers fail
                # closed here until that consumer implements the fixed selector.
                driver.validate_report(report, binary, part=part)
                manifest = json.loads(report_path.read_text())
                native = (report_path.parent / manifest['native']['path']).resolve()
                if not native.is_relative_to(folder.resolve()):
                    raise ValueError('native receipt escapes delivery evidence')
                receipt.update(native=file_record(native),
                    contract={k: report[k] for k in ('fixture_lock', 'm6_lock')},
                    rejection_probes=report['rejection_probes'],
                    rows=[{k: row[k] for k in ('variant_id', 'repetition', 'assertions', 'cleanup')}
                          for row in report['results']],
                    **{k: report[k] for k in ('status', 'cleanup', 'release_ready', 'eligible_for_grading', 'semantic_score')})
                check_case(receipt, part)
            receipts.append(receipt)
    if source != source_state() or run != run_identity() or locked != fixtures() or file_record(binary)['sha256'] != suite['binary_sha256']:
        raise ValueError('source/fixture/binary changed during delivery validation')
    return dict(version=1, part=part, suite=file_record(path), fixtures=locked,
                identity={k: suite[k] for k in DELIVERY_IDENTITY}, receipts=receipts)


def validate(suites, manifests, source, run, locked):
    if set(suites) != set(DELIVERY_PARTS) or set(manifests) != set(DELIVERY_PARTS):
        raise ValueError('exactly all three delivery parts required')
    first = suites['control']
    rows, case_rows, cases = [], [], []
    for part in DELIVERY_PARTS:
        suite, manifest = suites[part], manifests[part]
        gates, members = check_suite(suite, part, source, run)
        if (any(suite[k] != first[k] for k in DELIVERY_IDENTITY) or
                type(manifest.get('version')) is not int or manifest['version'] != 1 or
                manifest.get('part') != part or manifest.get('fixtures') != locked or
                manifest.get('identity') != {k: suite[k] for k in DELIVERY_IDENTITY} or
                [(r['gate'], r['attempt']) for r in manifest['receipts']] !=
                [(g.name, n) for g in gates for n in range(1, g.attempts + 1)]):
            raise ValueError('delivery source/build/run/fixture/receipt drift')
        for receipt in manifest['receipts']:
            gate = next(g for g in gates if g.name == receipt['gate'])
            record = receipt['report']
            if (type(receipt['attempt']) is not int or record.get('path') != report_name(gate, receipt['attempt']) or
                    type(record.get('bytes')) is not int or record['bytes'] <= 0 or
                    not re.fullmatch('[0-9a-f]{64}', record.get('sha256', ''))):
                raise ValueError('missing or invalid delivery report identity')
            if part != 'control':
                check_case(receipt, part)
                native = receipt.get('native', {})
                if (type(native.get('bytes')) is not int or native['bytes'] <= 0 or
                        not re.fullmatch('[0-9a-f]{64}', native.get('sha256', '')) or not native.get('path')):
                    raise ValueError('missing GC-07 native identity')
                cases.append(receipt)
                case_rows.extend(receipt['rows'])
        if part == 'control':
            rows.extend(members)
    registered = any(g.name == 'gc07' for g in SHARDS['delivery'])
    expected = [pair for part in GC07_VARIANTS for pair in gc07_pairs(part)] if registered else []
    if [(r['variant_id'], r['repetition']) for r in case_rows] != expected:
        raise ValueError('full ordered thirty GC-07 receipts required')
    if registered:
        if (len(cases) != 2 or cases[0]['contract'] != cases[1]['contract'] or
                cases[0]['rejection_probes'] != cases[1]['rejection_probes'] or
                len({tuple(sorted(r['assertions'])) for r in case_rows}) != 1):
            raise ValueError('GC-07 frozen contract/assertion inventory drift')
        rows.append(dict(gate='gc07', attempt=1, returncode=0, owned_group_drained=True,
                         parts=[suites[p]['shards']['delivery'][0] for p in GC07_VARIANTS]))
    if [(r['gate'], r['attempt']) for r in rows] != [
            (g.name, n) for g in SHARDS['delivery'] for n in range(1, g.attempts + 1)]:
        raise ValueError('logical delivery inventory drift')
    return dict(first, delivery_part=None, complete_inventory=True, shards={'delivery': rows},
                parts=suites, native_receipts=manifests,
                gc07=dict(status='mechanical_pass' if registered else 'not_registered',
                          receipts=case_rows, semantic_score=None, release_ready=False),
                semantic_score=None, release_ready=False,
                elapsed_seconds=max(s['elapsed_seconds'] for s in suites.values()))


def join(root):
    if {p.name for p in root.iterdir()} != set(DELIVERY_PARTS):
        raise ValueError('missing or unexpected delivery part directory')
    suites, manifests = {}, {}
    for part in DELIVERY_PARTS:
        folder = root / part
        path = folder / f'suite-delivery-{part}.json'
        suites[part] = json.loads(path.read_text())
        manifests[part] = json.loads((folder / 'delivery-receipts.json').read_text())
        if manifests[part].get('suite') != file_record(path):
            raise ValueError('delivery manifest/suite file drift')
    return validate(suites, manifests, source_state(), run_identity(), fixtures())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest='operation', required=True)
    part_parser = sub.add_parser('collect')
    part_parser.add_argument('--part', choices=DELIVERY_PARTS, required=True)
    part_parser.add_argument('--binary', type=Path, required=True)
    part_parser.add_argument('--evidence', type=Path, required=True)
    join_parser = sub.add_parser('join')
    join_parser.add_argument('--parts', type=Path, required=True)
    join_parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.mkdir(parents=True, exist_ok=True)
    output = args.evidence / ('delivery-receipts.json' if args.operation == 'collect' else 'suite-delivery.json')
    output.unlink(missing_ok=True)
    result = collect(args.evidence, args.part, args.binary) if args.operation == 'collect' else join(args.parts)
    output.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(dict(passed=True, operation=args.operation, output=str(output), semantic_score=None, release_ready=False)))


if __name__ == '__main__':
    main()
