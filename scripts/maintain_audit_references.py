"""Prepare an explicit identity-only refresh of a bounded audit's references.

This never reruns native observations or certifies changed code. Review the
proposal, then replace the ledger explicitly; audit_package rechecks every blob.
"""
import argparse
import json
from pathlib import Path

from release_package import digest, frozen_digest, git, local_file, reference_identities, source_git_proof
from release_receipts import strict_json

ROOT = Path(__file__).resolve().parents[1]


def prepare(root, ledger_path, source_sha, reason, references=()):
    ledger_path = ledger_path.resolve()
    name = ledger_path.relative_to(root.resolve()).as_posix()
    ledger = strict_json(local_file(root, name).read_bytes())
    if not reason.strip():
        raise ValueError('reference refresh needs a reason')
    old = ledger.get('reference_maintenance')
    if old:
        provenance = old['original_ledger']
    else:
        raw = git(root, 'show', source_sha + ':' + name)
        if strict_json(raw) != ledger:
            raise ValueError('commit the original ledger before reference maintenance')
        from release_package import REFERENCE_LEDGER_ORIGINALS
        provenance = REFERENCE_LEDGER_ORIGINALS.get(ledger['package'])
        if not provenance or frozen_digest(ledger) != provenance['content_sha256']:
            raise ValueError('original ledger has no approved reference-maintenance seal')
    selected = set(references) | set(old['current']['references'] if old else ())
    selected.update(ref for ref, value in ledger['references'].items()
                    if digest(local_file(root, ref.partition('#')[0]).read_bytes()) != value['sha256'])
    if not selected or not selected <= ledger['references'].keys():
        raise ValueError('select known references or change a referenced file first')
    git(root, 'merge-base', '--is-ancestor', provenance['commit'], source_sha)
    git(root, 'merge-base', '--is-ancestor', source_sha, 'HEAD')
    ledger['reference_maintenance'] = dict(
        version=2, original_ledger=provenance,
        current=dict(source_sha=source_sha, scope='reference_identity_only', reason=reason,
                     binding='source_commit_tree_and_HEAD_blob',
                     git_proof=source_git_proof(root, source_sha, selected),
                     references={ref: dict(sha256=digest(local_file(root, ref.partition('#')[0]).read_bytes()))
                                 for ref in sorted(selected)},
                     invalidated_observations=sorted(ledger['observations'])))
    # Includes sealed original-ledger/current Git and symbol checks. Never
    # manufacture a native receipt from the successful identity comparison.
    reference_identities(root, ledger)
    return ledger


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--ledger', type=Path, required=True)
    parser.add_argument('--source-sha', required=True, help='Full committed current code SHA; must be an ancestor of HEAD')
    parser.add_argument('--reason', required=True)
    parser.add_argument('--reference', action='append', default=[], help='Also recheck an unchanged reference (repeatable)')
    parser.add_argument('--output', type=Path, required=True, help='New proposal under evidence/; never an input ledger')
    args = parser.parse_args()
    output = args.output.resolve()
    evidence = (ROOT / 'evidence').resolve()
    if (not output.is_relative_to(evidence) or output == evidence or
            output == args.ledger.resolve()):
        parser.error('--output must be a separate proposal under repository evidence/')
    output.unlink(missing_ok=True)
    ledger = prepare(ROOT, args.ledger, args.source_sha, args.reason, args.reference)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(ledger, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(proposal=str(output),
                         scope='reference_identity_only', native_revalidated=False,
                         release_ready=False)))


if __name__ == '__main__':
    main()
