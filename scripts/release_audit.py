#!/usr/bin/env python3
"""Inventory v1 source clauses and evidence candidates; never certify release.

The ledger is a navigation index, not proof that a clause passed. Partial reviews
record both evidence and unresolved obligations. Existing Fake fragments cannot supply
full-case semantic scores or current Live Shadow evidence.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
LEGACY_SHA = '38faf6144d9ae0dc5e5d38aaf32d06052820d683'


def check_reference(root, ref):
    # The preserved ledger indexes an unmerged checkpoint. These are historical
    # navigation links, never current-code or release evidence. Do not import
    # all of that branch's product changes merely to make its links resolve.
    path, _, symbol = ref.partition('#')
    target = root / path
    if not target.resolve().is_relative_to(root.resolve()) or Path(path).is_absolute():
        raise ValueError('invalid review reference: ' + ref)
    current = target.read_text() if target.is_file() else ''
    if not current or (symbol and symbol not in current):
        index = root / 'docs/v1-legacy-references.json'
        if sha256(index) != '090865509132a0be9a998c8cce6cae720b7d0dead90492624e0b65f8532f043d':
            raise ValueError('legacy reference index changed')
        if ref not in json.loads(index.read_text())['references']:
            raise ValueError('invalid legacy reference: ' + ref)
        return LEGACY_SHA
    if symbol and symbol not in current:
        raise ValueError('missing review symbol: ' + ref)
    return 'worktree'


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def inventory(text):
    """Keep paragraphs, list items, table rows and fenced contracts inspectable."""
    section, fence, start, block = None, False, 0, []
    rows = []

    def flush():
        if block and section:
            content = '\n'.join(block)
            rows.append(dict(section=section, line=start, text=content,
                             sha256=hashlib.sha256(content.encode()).hexdigest()))
        block.clear()

    for line, value in enumerate(text.splitlines(), 1):
        heading = re.match(r'^(#{2,4}) (.+)$', value) if not fence else None
        if heading:
            flush()
            title = heading[2]
            numeric = re.match(r'(\d+(?:\.\d+)*)\.?\s', title)
            special = re.match(r'(GC-\d{2}|M[0-7])', title)
            if numeric:
                section = numeric[1]
            elif special:
                section = ('7.' if special[1].startswith('GC-') else '17.') + special[1]
            elif section and len(heading[1]) > 2:
                section = section.split('.')[0] + '.' + title
            else:
                raise ValueError('unrecognized source heading: ' + title)
            continue
        if value.startswith('```'):
            if not fence:
                flush()
                start = line
            fence = not fence
            block.append(value)
            if not fence:
                flush()
            continue
        if not fence and (not value.strip() or re.match(r'^(?:- |\d+\. |\|)', value)):
            flush()
        if value.strip() or fence:
            if not block:
                start = line
            block.append(value)
    flush()
    if fence:
        raise ValueError('unterminated source fence')
    return rows


def route(section, mappings):
    matches = [key for key in mappings if section == key or section.startswith(key + '.')]
    if not matches:
        raise ValueError('unmapped source section: ' + section)
    return mappings[max(matches, key=len)]


def apply_reviews(root, units, reviews):
    """Bind bounded review notes to exact source blocks, without certifying PASS."""
    def identity(u):
        return u['source'], u['section'], u['line'], u['sha256']
    by_key = {identity(u): u for u in units}
    seen = set()
    for review in reviews:
        key = identity(review)
        if key not in by_key or key in seen:
            raise ValueError('stale or duplicate individual review: ' + str(key))
        seen.add(key)
        if review['disposition'] not in ('partial_mechanical', 'evidence_gap'):
            raise ValueError('individual review cannot certify release conformance')
        for field in ('observed', 'remaining', 'code', 'tests', 'evidence', 'pr'):
            if not review.get(field):
                raise ValueError('missing individual review disposition: ' + field)
        if review['pr'] != 'https://github.com/z-chenhao/erii/pull/32 (unmerged; independent review required)':
            raise ValueError('unrecognized checkpoint PR disposition')
        for category in ('code', 'tests', 'evidence'):
            for ref in review[category]:
                check_reference(root, ref)
        by_key[key]['disposition'] = review['disposition']
        by_key[key]['review'] = review
    return len(seen)


def audit(root, ledger):
    if ledger.get('version') != 1:
        raise ValueError('unsupported release ledger')
    areas = ledger['areas']
    for name, area in areas.items():
        for category in ('code', 'tests', 'evidence'):
            if not area[category]:
                raise ValueError('empty ' + category + ': ' + name)
            for ref in area[category]:
                check_reference(root, ref)
        if not area['prs'] or not set(area['prs']) <= set(ledger['merged_prs']):
            raise ValueError('missing PR mapping: ' + name)
    units, sources = [], {}
    for source, definition in ledger['sources'].items():
        path = root / source
        if sha256(path) != definition['sha256']:
            raise ValueError('source changed; re-audit and update ledger: ' + source)
        rows = inventory(path.read_text())
        if not rows:
            raise ValueError('empty source: ' + source)
        for row in rows:
            area = route(row['section'], definition['sections'])
            if area not in areas:
                raise ValueError('unknown area: ' + area)
            units.append(dict(source=source, **row, area=area,
                              disposition='pending_individual_review'))
        sources[source] = dict(sha256=sha256(path), units=len(rows))
    reviewed = apply_reviews(root, units, ledger.get('individual_reviews', []))
    # Indexing source text, passing contracts and merged PRs are distinct facts.
    # This command intentionally has no path from those facts to release PASS.
    return dict(version=1, audited_base=ledger['audited_base'], sources=sources,
                legacy_reference_sha=LEGACY_SHA,
                status='inconclusive', release_ready=False,
                scope='source inventory and evidence candidates, not completed normative audit',
                individually_verified_requirements=0,
                individually_reviewed_blocks=reviewed,
                merged_prs=ledger['merged_prs'], areas=areas, source_units=units,
                golden_cases=ledger['golden_cases'], gaps=ledger['gaps'],
                scores=dict(suite_mean=None, suite_min=None,
                            dimensions={key: None for key in 'OFPATSRC'},
                            product_pass_at_1=None, product_pass_power_5=None),
                live_shadow='inconclusive; current release observation and human review absent')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ledger', type=Path, default=ROOT / 'docs/v1-release-ledger.json')
    parser.add_argument('--evidence', type=Path, default=ROOT / 'evidence/release-audit.json')
    parser.add_argument('--require-complete', action='store_true')
    args = parser.parse_args()
    # Remove a prior report before validation; a failed re-audit cannot leave
    # an old result looking current. Output must be disposable evidence.
    evidence_root = (ROOT / 'evidence').resolve()
    output = args.evidence.resolve()
    if not output.is_relative_to(evidence_root) or output == evidence_root:
        parser.error('--evidence must be a file under repository evidence/')
    output.unlink(missing_ok=True)
    report = audit(ROOT, json.loads(args.ledger.read_text()))
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(status=report['status'], release_ready=False,
                          source_units=len(report['source_units']),
                          evidence=str(output))))
    return 2 if args.require_complete else 0


if __name__ == '__main__':
    raise SystemExit(main())
