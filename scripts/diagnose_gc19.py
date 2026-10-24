#!/usr/bin/env python3
"""Bounded, fixture-only GC-19 diagnosis; never substitutes for normal CI."""
import argparse
from collections import Counter
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]


def overlay_source(source):
    signature = 'func (s *Store) Transaction(ctx context.Context, write func(*sql.Tx) error) error {'
    if source.count(signature) != 1 or source.count('"runtime/debug"'):
        raise ValueError('transaction diagnostic source drift')
    source = source.replace('"path/filepath"', '"path/filepath"\n"runtime/debug"')
    return source.replace(signature, '''func (s *Store) Transaction(ctx context.Context, write func(*sql.Tx) error) (result error) {
    defer func() {
        if result != nil && os.Getenv("ERII_SQL_FIXTURE_DIAGNOSTIC") == "1" {
            code := 0
            var sqliteErr *sqlitedriver.Error
            if errors.As(result, &sqliteErr) { code = sqliteErr.Code() }
            fmt.Fprintf(os.Stderr, "FIXTURE_TX_ERROR type=%T code=%d error=%v\\n%s\\n", result, code, result, debug.Stack())
        }
    }()
''')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    evidence = args.evidence.resolve()
    evidence.mkdir(parents=True, exist_ok=True)
    source = ROOT / 'internal/agent/sqlite/store.go'
    with tempfile.TemporaryDirectory(prefix='erii-sql-diagnostic-') as temp:
        temp = Path(temp)
        replacement = temp / 'store.go'
        replacement.write_text(overlay_source(source.read_text()))
        overlay = temp / 'overlay.json'
        overlay.write_text(json.dumps({'Replace': {str(source): str(replacement)}}))
        binary = temp / 'erii-diagnostic'
        build = ['go', 'build', '-overlay', str(overlay), '-o', str(binary), './cmd/erii']
        env = os.environ.copy()
        if env.get('ERII_CI_BASE_GOROOT'):
            env['GOROOT'] = env['ERII_CI_BASE_GOROOT']
            env['PATH'] = env['GOROOT'] + '/bin:' + env['PATH']
        subprocess.run(build, cwd=ROOT, env=env, check=True)
        env['ERII_CAPABILITY_DIAGNOSTICS'] = str(evidence / 'fixtures')
        env['ERII_VERIFY_CASE_JOBS'] = '4'
        command = ['python3', 'scripts/verify_suite.py', '--binary', str(binary),
                   '--evidence', str(evidence / 'suite'), '--shard', 'codemode',
                   '--codemode-part', 'GC-19', '--jobs', '1']
        record = dict(source_sha=subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
                      diagnostic_only=True, build=build, command=command,
                      source_sha256=hashlib.sha256(source.read_bytes()).hexdigest(),
                      overlay_sha256=hashlib.sha256(replacement.read_bytes()).hexdigest(),
                      binary_sha256=hashlib.sha256(binary.read_bytes()).hexdigest())
        result = subprocess.run(command, cwd=ROOT, env=env)
        record['returncode'] = result.returncode
        errors = Counter()
        failures = []
        for path in (evidence / 'fixtures').glob('*/command.json'):
            row = json.loads(path.read_text())
            errors.update(line for line in row['stderr'].splitlines() if line.startswith('FIXTURE_TX_ERROR'))
            if row['returncode']:
                failures.append(str(path.relative_to(evidence)))
        record.update(transaction_errors=dict(errors), failed_commands=failures)
        (evidence / 'diagnostic.json').write_text(json.dumps(record, indent=2) + '\n')
        print(json.dumps(record), flush=True)
        return result.returncode


if __name__ == '__main__':
    raise SystemExit(main())
