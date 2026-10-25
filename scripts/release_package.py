"""Validate a bounded audit ledger, without certifying product conformance.

The frozen package digest comes from the owner manifest, independently of the
authored dispositions. Native observations remain supporting evidence; only the
release decision path can consume independently acquired release receipts.
"""
from collections import Counter
import hashlib
import json
from pathlib import Path
import re
import subprocess

from release_receipts import strict_json

MANIFEST_HASH = '0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556'
RESEALED_MANIFEST_HASH = 'f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837'
PACKAGE_HASHES = {
    'baseline-01': 'b95fc9fa9861ff62d5551f820ae56dac90f9ad33ecdbd2ad2ef743fcc315db95',
    'baseline-02': '7480b2bdfd3d86256d1d13a10f6b2cd9707fb62f365f07e7e244f17a0b510c69',
    'baseline-03': '1b6367425a571b1bb0fcca0bc64a196a5f962bb28144dda17b06b23b453e7d67',
    'baseline-04': '2e56b4c2b855c9c6a81e667f316e92444e2890a8a35568a36cdce566fb07639e',
    'baseline-05': '3502b3aa7b733584a5ddfe624ab44b6f303201f6c752e38f650fd882ce45eb12',
    'baseline-06': 'b20a9f33e1b58b9b184f1a3004055d12f113546764f6b89a9c103016ab94668a',
    'baseline-07': 'd05cf38d322f6c69ad05e499d0caacafc0c520bec0e728170f05a3cf517fcd63',
    'baseline-08': '12528dae09407c3daa20caf9c1d1c5166920d70b50a3346b4ab875cd93d077e8',
    'baseline-09': '680dd54a6cbc5587ceb348b70adda1956f38a0b03ffe9e622204e587991c5f46',
    'baseline-10': '33ebc44545bf9d3718771a4303266cca26f1923c1f234ee2e30474b5067fa911',
    'baseline-11': '46a87bc1728c849591bf6c45905e56fc3e83a849264dacdda83d92e3d8b04fca',
    'baseline-12': 'aece4cdba5e23a174caf8c4ee1f1b6ae65679bd3b17bd0274304446d21154d65',
    'baseline-13': '9d86d5732f7addecb9001ad5cf9c9b521f212373c5a57149e8c08cb95661570d',
    'golden-common-01': '3421f37dee8becf70cb964d7eb7a998f3a4bb19c006d0e2bcb3bbbb7cabd9d9e',
    'golden-common-02': '8915d2e0d8b4fd82564a0f2ce88988f645d7c24ded65f280b7edaa91f73802cc',
    'golden-common-03': '64a9b52fbe9d60100a4263f8799611c925001895b41b0c6ddd1bcd15c400f60e',
}
ORIGINAL_PACKAGE_HASHES = {
    'baseline-02': 'a19a7c00e2ae1c3683e68ca1bee23210e5ee75cc19174ce2dc0177bacf2d0e63',
    'baseline-07': 'e4c7b80cf8886c25307f3b225c1970d4ff7234ca5f7e6a0cacb36ac4a0c5ace3',
    'baseline-08': '22f68315002ca724f5ce132ee02b9a8c1678f19d4decc3101c42bfa541cb30ee',
    'baseline-09': 'e4c26626df402e3f0f7e9ecf731901b9b17e33fa25cd62049a350d6148fdab53',
    'baseline-10': 'd312f7fa24d257858397fcee94246eb2c01910fb249ec971c7a874829c31ef0f',
    'baseline-11': '415396f2aa95fb071574356fe6cef9f03f8b81e729e0f8c9966bf9395886b534',
    'baseline-12': '81e65f75d0be884021e2fbf9fc2a13a99613e2bd0cd40d52e6a26b8a3c63cfab',
    'baseline-13': '933565336b762b2dec8e8dc2b9459fda3d41dc3c8515bdc8ccce9bced7729e01',
}

# Original merged ledger seals for the five demonstrated shared-reference
# consumers. Refreshes cannot rewrite frozen notes, receipts, gaps or provenance.
REFERENCE_LEDGER_ORIGINALS = {
    'baseline-12': {
        'commit': '49fc12899b3d07b5faee41c191bf9993217b7168',
        'path': 'docs/audits/baseline-12.json',
        'sha256': 'e8320e67006fd3c0517609fe6256f578a042394ba661dda60d5008ede9320368',
        'content_sha256': '8876fee8d5b42b820544d22321731335166b89c56f4dc3ffd208fd0b04c5976c',
    },
    'baseline-13': {
        'commit': '49fc12899b3d07b5faee41c191bf9993217b7168',
        'path': 'docs/audits/baseline-13.json',
        'sha256': '290b7cda73cacd979ac84022f4560c3a1e5a5a4abe8868a05d3cc17d4323f784',
        'content_sha256': '5dedd28f05e620f8324ff15bf3c62d096e4c14dafefe33ffc7f87bf491ad9782',
    },
    'golden-common-01': {
        'commit': '49fc12899b3d07b5faee41c191bf9993217b7168',
        'path': 'docs/audits/golden-common-01.json',
        'sha256': '263d478aa272f20092ed83434794cc01d23686c3cd2fc3c483c2be3114545f43',
        'content_sha256': '7ef80a12ca25b5f703df6ca0dd1b35360c29a295685dbac44be2aba440453c17',
    },
    'golden-common-02': {
        'commit': '49fc12899b3d07b5faee41c191bf9993217b7168',
        'path': 'docs/audits/golden-common-02.json',
        'sha256': 'e3a76b87bb643f334d999270b8a25de8a9d27439d8cc1f39c6fb3c621f8d5f52',
        'content_sha256': '15b301f614532dade8c6526693d5b7bdb32a78f6970d2ad54a5edff6dd3742fb',
    },
    'golden-common-03': {
        'commit': '49fc12899b3d07b5faee41c191bf9993217b7168',
        'path': 'docs/audits/golden-common-03.json',
        'sha256': '25f3d5f361718ec4e6452295f4d1917fb0f2750922210c62b592328b5d7a5bd9',
        'content_sha256': '44801752156e998572d1d7e3e8c81c3ef12f9c55193cd29e37d8a74237a79ef0',
    },
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def frozen_digest(blocks):
    return digest(json.dumps(blocks, sort_keys=True, separators=(',', ':')).encode())


def identity(block):
    return block['source'], block['section'], block['sha256']


def local_file(root, name):
    path = root / name
    if (Path(name).is_absolute() or not path.resolve().is_relative_to(root.resolve())
            or path.is_symlink() or not path.is_file()):
        raise ValueError('invalid package file: ' + name)
    return path


def git(root, *args):
    result = subprocess.run(['git', '-C', str(root), *args], capture_output=True)
    if result.returncode:
        raise ValueError('package Git binding rejected: ' + ' '.join(args))
    return result.stdout


def reference_identities(root, ledger, verify_git=False):
    """Keep original observations bound to old bytes after an explicit refresh."""
    base = ledger['audited_base']
    references = ledger['references']
    maintenance = ledger.get('reference_maintenance')
    original = {k: v for k, v in ledger.items() if k != 'reference_maintenance'}
    seal = REFERENCE_LEDGER_ORIGINALS.get(ledger['package'])
    # Removing the maintenance envelope must not reopen the old mutable-hash
    # path. The untouched original ledger remains inspectable on original bytes.
    if seal and frozen_digest(original) != seal['content_sha256']:
        raise ValueError('original audit ledger changed')
    current = {}
    if maintenance is not None:
        if maintenance.get('version') != 1:
            raise ValueError('unsupported reference maintenance')
        provenance = maintenance.get('original_ledger', {})
        if provenance != REFERENCE_LEDGER_ORIGINALS.get(ledger['package']):
            raise ValueError('missing original ledger provenance')
        git(root, 'merge-base', '--is-ancestor', provenance['commit'], 'HEAD')
        raw = git(root, 'show', provenance['commit'] + ':' + provenance['path'])
        if digest(raw) != provenance['sha256'] or strict_json(raw) != original:
            raise ValueError('original ledger Git blob mismatch')
        update = maintenance.get('current', {})
        revision = update.get('source_sha', '')
        current = update.get('references', {})
        if (not re.fullmatch(r'[0-9a-f]{40}', revision) or
                update.get('scope') != 'reference_identity_only' or
                not isinstance(update.get('reason'), str) or not update['reason'].strip() or
                not current or not current.keys() <= references.keys() or
                update.get('invalidated_observations') != sorted(ledger['observations'])):
            raise ValueError('missing current identity revalidation or observation invalidation')
        git(root, 'merge-base', '--is-ancestor', provenance['commit'], revision)
        git(root, 'merge-base', '--is-ancestor', revision, 'HEAD')
    result = {}
    for ref, value in references.items():
        path, _, symbol = ref.partition('#')
        data = local_file(root, path).read_bytes()
        expected = current.get(ref, value)['sha256']
        if digest(data) != expected:
            raise ValueError('stale current reference: ' + ref)
        if verify_git:
            if digest(git(root, 'show', base + ':' + path)) != value['sha256']:
                raise ValueError('audited Git blob mismatch: ' + ref)
        if ref in current and digest(git(root, 'show', revision + ':' + path)) != expected:
            raise ValueError('current Git blob mismatch: ' + ref)
        if symbol and symbol not in data.decode():
            raise ValueError('missing current symbol: ' + ref)
        result[ref] = dict(historical_sha256=value['sha256'], current_sha256=expected,
                           current_source_sha=revision if ref in current else base,
                           current_git_blob_checked=ref in current or verify_git,
                           historical_git_blob_checked=verify_git,
                           revalidation='reference_identity_only' if ref in current else 'unchanged')
    return result


def audit_package(root, ledger, units, manifest=None, native_root=None, verify_git=False):
    name = ledger['package']
    frozen = ledger['frozen_blocks']
    # ADR006 changes baseline-02, baseline-11 and baseline-13; other resealed packages
    # retain content identities with shifted lines. Keep original provenance.
    resealed = name in ORIGINAL_PACKAGE_HASHES
    manifest_hash = RESEALED_MANIFEST_HASH if resealed else MANIFEST_HASH
    if (ledger.get('version') != 1 or ledger['manifest_sha256'] != manifest_hash
            or name not in PACKAGE_HASHES or frozen_digest(frozen) != PACKAGE_HASHES[name]):
        raise ValueError('frozen package changed; coordinator must reseal it')
    if resealed:
        if (ledger.get('original_manifest_sha256') != MANIFEST_HASH or
                frozen_digest(ledger.get('original_frozen_blocks', [])) !=
                ORIGINAL_PACKAGE_HASHES[name]):
            raise ValueError('original manifest provenance changed')
    if name in ('baseline-11', 'baseline-13'):
        old_hash, new_hash, coordinator = {
            'baseline-11': ('c158433cfab75928e00c5392a666e7af064ad0362ab39e3f8ac32fe8153294f1',
                            'a719ccca98f823a521cd0bbd69518e1b8e0e959a42bea50bd72c7b00fe84fa86',
                            '01a16e27-6501-716a-8a22-0487065ce998'),
            'baseline-13': ('9602850bb1308669571ef7e86df0bf9bc91e3bee774426a3f263cb7faa21cfda',
                            '1bbef426eb568fcf13f01f9caa4add3de302d98724f9f40fde377bb0cfe0824e',
                            '01a16e27-6914-7c76-9607-efe9f00089c3'),
        }[name]
        changed = [(old, new) for old, new in
                   zip(ledger['original_frozen_blocks'], frozen)
                   if identity(old) != identity(new)]
        mapping = ledger.get('reseal_mapping', [])
        if (len(changed) != 1 or len(mapping) != 1 or
                changed[0][0]['sha256'] != old_hash or
                changed[0][1]['sha256'] != new_hash or
                mapping[0] != dict(original=changed[0][0], sealed=changed[0][1],
                                   authorization_comment='01a16e1d-3e2c-78fb-ba1d-5621d178a9bd',
                                   coordinator_comment=coordinator,
                                   conclusion_inherited=False)):
            raise ValueError(name + ' ADR006 source mapping changed')
    if manifest is not None:
        raw = manifest.read_bytes()
        if digest(raw) != manifest_hash:
            raise ValueError('owner manifest hash mismatch')
        definition = strict_json(raw)
        assigned = [identity(b) for p in definition['packages'].values() for b in p['blocks']]
        count = 1588 if resealed else 1585
        if len(assigned) != count or any(n != 1 for n in Counter(assigned).values()):
            raise ValueError('owner manifest assignment is not unique and complete')
        if resealed and Counter(assigned) != Counter(map(identity, units)):
            raise ValueError('resealed manifest does not cover the current source inventory')
        if definition['packages'][name]['blocks'] != frozen:
            raise ValueError('package differs from owner manifest')
    by_key = {}
    for unit in units:
        by_key.setdefault(identity(unit), []).append(unit)
    expected = {identity(b) for b in frozen}
    reviews = ledger['reviews']
    if (len(reviews) != len(frozen) or len({identity(r) for r in reviews}) != len(reviews)
            or {identity(r) for r in reviews} != expected):
        raise ValueError('missing, duplicate or out-of-package disposition')
    for block in frozen:
        if len(by_key.get(identity(block), [])) != 1:
            raise ValueError('changed or ambiguous frozen source block; reseal required: ' + str(block))
    # Historical identities stay immutable. An explicit maintenance record may
    # bind new current bytes, but never lends them an old native observation.
    base = ledger['audited_base']
    if verify_git:
        git(root, 'merge-base', '--is-ancestor', base, 'HEAD')
        source = frozen[0]['source']
        if digest(git(root, 'show', base + ':' + source)) != ledger['source_sha256']:
            raise ValueError('audited source hash mismatch')
    references = ledger['references']
    reference_report = reference_identities(root, ledger, verify_git)
    for number, pr in ledger['merged_prs'].items():
        if (pr['state'] != 'MERGED' or pr['number'] != int(number)
                or pr['url'] != 'https://github.com/z-chenhao/erii/pull/' + number
                or not pr['mergedAt']):
            raise ValueError('unmerged or wrong PR provenance')
        if verify_git:
            git(root, 'merge-base', '--is-ancestor', pr['mergeCommit']['oid'], base)
    gaps = ledger['gaps']
    for key, gap in gaps.items():
        if (not gap['remaining'] or not gap['exit_condition'] or not gap['deduplication']
                or not gap['issue_url'].startswith('https://multica.ai/erii-studio/issues/')
                or not gap['issue'].startswith('ERII-')):
            raise ValueError('unrouted package gap: ' + key)
    observations = ledger['observations']
    for obs in observations.values():
        if (obs['source_sha'] != base or not obs['command'] or not obs['limits']
                or not obs['files'] or obs['exit_code'] != 0):
            raise ValueError('incomplete native observation receipt')
        for f in obs['files']:
            if native_root is not None:
                raw = local_file(native_root, f['path']).read_bytes()
                if digest(raw) != f['sha256'] or len(raw) != f['size_bytes']:
                    raise ValueError('native observation bytes changed')
    counts = Counter()
    used_gaps = set()
    for review in reviews:
        source = by_key[identity(review)][0]
        original = next(b for b in frozen if identity(b) == identity(review))
        if (review['text'] != source['text'] or review['current_line'] != source['line']
                or review['line'] != original['line']):
            raise ValueError('review source text or location drifted')
        if name in ('baseline-07', 'baseline-08', 'baseline-09', 'baseline-10', 'baseline-11', 'baseline-12', 'baseline-13'):
            # These pinned seals have changed Task rows. Their
            # original location comes from the exact old/new mapping above.
            if name in ('baseline-11', 'baseline-13'):
                prior = ledger['original_frozen_blocks'][frozen.index(original)]
            else:
                prior = next(b for b in ledger['original_frozen_blocks']
                             if identity(b) == identity(review))
            if review.get('original_line') != prior['line']:
                raise ValueError('original package location drifted')
        if name == 'baseline-12':
            # The fenced minimum configuration list is one source block but
            # each full key needs its own disposition and implementation map.
            if source['text'].startswith('```text\napp.data_root'):
                keys = set(re.findall(r'\b[a-z_]+\.[a-z_]+', source['text']))
                mapping = review.get('config_mapping', {})
                if (set(mapping) != keys or set(review['obligations']) != keys or
                        any(not isinstance(v, str) or not v.strip() for v in mapping.values())):
                    raise ValueError('minimum configuration key mapping incomplete')
            if source['section'] == '16.7' and source['text'].startswith('- `'):
                mapping = review.get('interface_mapping', {})
                if (set(mapping) != set(review['obligations']) or
                        any(not isinstance(v, str) or not v.strip() for v in mapping.values())):
                    raise ValueError('consumer interface method mapping incomplete')
        if (name == 'baseline-11' and review['section'] == '16.2'
                and source['text'].startswith('| `')):
            # Each minimum-model table row has one duty and physical mapping
            # for every field; optional fields still require storage semantics.
            cells = re.findall(r'`([^`]+)`', source['text'])
            record, fields = cells[0], cells[1].split(', ')
            mapping = review.get('field_mapping', {})
            if (set(mapping) != set(fields) or
                    set(review['obligations']) != {record + '.' + f for f in fields} or
                    any(not isinstance(value, str) or not value.strip()
                        for value in mapping.values())):
                raise ValueError('minimum persistence field mapping incomplete')
        if review['disposition'] not in ('partial_mechanical', 'evidence_gap', 'policy', 'background', 'deferred'):
            raise ValueError('package reviews cannot certify conformance')
        for field in ('obligations', 'observed', 'proof_limit', 'references', 'prs'):
            if not review.get(field):
                raise ValueError('missing package review field: ' + field)
        if len(set(review['obligations'])) != len(review['obligations']):
            raise ValueError('duplicate sub-obligation')
        if any(not isinstance(o, str) or not o.strip() for o in review['obligations']):
            raise ValueError('empty sub-obligation')
        # New audits explicitly distinguish mandatory duties from mixed-in
        # background/deferred clauses. Preserve the first package's format.
        if name != 'baseline-01' or 'obligation_kinds' in review:
            kinds = review.get('obligation_kinds', {})
            if (set(kinds) != set(review['obligations']) or
                    not set(kinds.values()) <= {'required_v1', 'policy', 'background', 'deferred'}):
                raise ValueError('missing or invalid sub-obligation scope')
            exempt = {o for o, kind in kinds.items() if kind in ('background', 'deferred')}
            reasons = review.get('exemptions', {})
            remaining = {r['obligation'] for r in review['remaining']}
            if (set(reasons) != exempt or any(not reason.strip() for reason in reasons.values())
                    or remaining != set(kinds) - exempt):
                raise ValueError('mandatory duty unrouted or exemption unexplained')
        if not set(review['references']) <= references.keys() or not set(review['prs']) <= ledger['merged_prs'].keys():
            raise ValueError('unknown current reference or PR')
        if not set(review['observations']) <= observations.keys():
            raise ValueError('unknown native observation')
        if review['disposition'] in ('background', 'deferred') and (review['remaining'] or not review.get('reason')):
            raise ValueError('non-v1 disposition needs an explicit reason')
        if review['disposition'] == 'evidence_gap' and not review['remaining']:
            raise ValueError('evidence gap has no routed obligation')
        for remaining in review['remaining']:
            if (remaining['obligation'] not in review['obligations'] or remaining['gap'] not in gaps
                    or not remaining['detail']):
                raise ValueError('unrouted sub-obligation')
            used_gaps.add(remaining['gap'])
        counts[review['disposition']] += 1
    if used_gaps != gaps.keys():
        raise ValueError('orphan gap route')
    return dict(package=name, reviewed_blocks=len(reviews), dispositions=dict(counts),
                frozen_package_sha256=PACKAGE_HASHES[name],
                manifest_checked=manifest is not None, native_bytes_checked=native_root is not None,
                git_provenance_checked=verify_git,
                current_reference_git_checked=ledger.get('reference_maintenance') is not None or verify_git,
                reference_identities=reference_report,
                observation_applicability={key: dict(
                    status='historical_only' if ledger.get('reference_maintenance') else 'audited_identity_only',
                    current_native_revalidated=False,
                    reason='Native observations bind audited_base; identity checks never rerun commands.')
                    for key in observations},
                status='inconclusive', release_ready=False, verified_requirements=0,
                gaps=gaps, observations=observations, reviews=reviews)
