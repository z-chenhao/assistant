#!/usr/bin/env python3
"""Compare serial and parallel runs; report sample counts and observed spread."""
import argparse
from collections import defaultdict
import json
from pathlib import Path
import re
import statistics


def stats(values):
    return dict(samples=len(values), total_seconds=sum(values),
                median_seconds=statistics.median(values), min_seconds=min(values),
                max_seconds=max(values), stdev_seconds=statistics.stdev(values) if len(values) > 1 else None)


def summarize(folder):
    harnesses, cases = defaultdict(list), defaultdict(list)
    for path in sorted((folder / 'profiles').glob('**/*.json')):
        profile = json.loads(path.read_text())
        harnesses[profile['harness']].append(profile['elapsed_seconds'])
        for case in profile.get('cases', []):
            cases[profile['harness'] + '/' + case['name']].append(case['elapsed_seconds'])
    models = folder / 'models.jsonl'
    if models.exists():
        for line in models.read_text().splitlines():
            event = json.loads(line)
            if event.get('Test') and event.get('Action') in ('pass', 'fail'):
                cases[event['Package'] + '/' + event['Test']].append(event['Elapsed'])
            elif event.get('Action') in ('pass', 'fail'):
                # Go packages run concurrently; package elapsed times overlap.
                harnesses['go-package/' + event['Package']].append(event['Elapsed'])
    tap = folder / 'pi-bridge.tap'
    if tap.exists():
        name = None
        for line in tap.read_text().splitlines():
            total = re.match(r'# duration_ms ([\d.]+)', line)
            if total:
                harnesses['pi-bridge'].append(float(total[1]) / 1000)
            if line.startswith('# Subtest: '):
                name = line.removeprefix('# Subtest: ')
            duration = re.match(r'\s+duration_ms: ([\d.]+)', line)
            if duration and name:
                cases['pi-bridge/' + name].append(float(duration[1]) / 1000)
                name = None
    runs = list(folder.glob('run*.json')) or [folder / 'suite-all.json']
    metadata = [json.loads(p.read_text()) for p in runs]
    if any(r.get('returncode', 0) != 0 or r.get('passed', True) is not True for r in metadata):
        raise ValueError('Cannot compare an incomplete or failed verification run')
    return dict(directory=str(folder), runs=metadata,
                elapsed=stats([r['elapsed_seconds'] for r in metadata]),
                harnesses={k: stats(v) for k, v in sorted(harnesses.items())},
                cases={k: stats(v) for k, v in sorted(cases.items())})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--before', type=Path, required=True)
    parser.add_argument('--after', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    before, after = summarize(args.before), summarize(args.after)
    report = dict(version=1, before=before, after=after,
                  median_speedup=before['elapsed']['median_seconds'] / after['elapsed']['median_seconds'],
                  note='Full-suite sample counts are explicit. Five-run harness ranges measure repeated-gate variance; they are not independent full-suite samples. Function/case timing includes blocking waits.')
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({k: report[k] for k in ['median_speedup', 'note']}))


if __name__ == '__main__':
    main()
