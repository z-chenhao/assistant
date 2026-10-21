#!/usr/bin/env python3
"""Trusted-base change impact for early feedback only. Full CI always runs.

Unknown paths, risky names, invalid/missing/drifted manifests and unavailable
base history select all shards. No PR title, label or model input is used.
"""
import argparse
import json
from pathlib import Path, PurePosixPath
import re
import subprocess

from verify_suite import SHARDS

MANIFEST = 'scripts/ci_impact.json'
ALL = sorted(SHARDS)
FORCED = re.compile(r'(^|/)(scripts|\.github|evals|runtime)(/|$)|'
                    r'(^|/)(Makefile|AGENTS\.md|go\.mod|go\.sum)$|'
                    r'security|replay|protocol|schema|migration|technical-design|codemode|code_recovery|'
                    r'(^|/)package(-lock)?\.json$', re.IGNORECASE)


def valid(manifest):
    if not isinstance(manifest, dict) or manifest.get('version') != 1:
        return False
    rules = manifest.get('rules')
    if not isinstance(rules, list) or not rules:
        return False
    prefixes = set()
    for rule in rules:
        if not isinstance(rule, dict) or set(rule) != {'prefix', 'shards', 'example'}:
            return False
        prefix, shards, example = rule['prefix'], rule['shards'], rule['example']
        if (not isinstance(prefix, str) or not prefix or prefix.startswith('/') or
                '..' in PurePosixPath(prefix).parts or any(c in prefix for c in '*?\\') or
                prefix in prefixes or not isinstance(shards, list) or
                any(not isinstance(s, str) or s not in ALL for s in shards) or len(set(shards)) != len(shards) or
                not isinstance(example, str) or not matches(example, prefix)):
            return False
        prefixes.add(prefix)
    return True


def matches(path, prefix):
    return path.startswith(prefix) if prefix.endswith('/') else path == prefix


def select(paths, manifest, trusted=True):
    if not trusted or not valid(manifest):
        return dict(shards=ALL, reason='manifest missing, invalid or drifted')
    selected = set()
    if not paths:
        return dict(shards=ALL, reason='empty or unavailable change set')
    for path in paths:
        if (not isinstance(path, str) or path.startswith('/') or '..' in PurePosixPath(path).parts or
                FORCED.search(path)):
            return dict(shards=ALL, reason='risk-sensitive path: ' + str(path))
        rules = [r for r in manifest['rules'] if matches(path, r['prefix'])]
        if len(rules) != 1:
            return dict(shards=ALL, reason='unknown or ambiguous path: ' + path)
        selected.update(rules[0]['shards'])
    return dict(shards=sorted(selected), reason='trusted path mappings; full merge inventory unchanged')


def plan(root, base):
    def git(*args):
        return subprocess.check_output(['git', *args], cwd=root, stderr=subprocess.DEVNULL)
    source = git('rev-parse', 'HEAD').decode().strip()
    try:
        if not re.fullmatch('[0-9a-f]{40}', base):
            raise ValueError('invalid base SHA')
        subprocess.run(['git', 'merge-base', '--is-ancestor', base, source], cwd=root, check=True)
        trusted = git('show', base + ':' + MANIFEST)
        current = (root / MANIFEST).read_bytes()
        paths = git('diff', '--name-only', '--no-renames', '-z', base, source).decode().split('\0')
        paths = [p for p in paths if p]
        result = select(paths, json.loads(trusted), trusted == current)
    except (ValueError, OSError, subprocess.CalledProcessError):
        paths = []
        result = select([], None, False)
    return dict(version=1, source_sha=source, base_sha=base, paths=paths, **result,
                full_shards=ALL, full_gate='always required; impact never skips merge evidence')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', default='')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = plan(Path(__file__).resolve().parents[1], args.base)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
