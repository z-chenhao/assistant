"""Apply the audit rejection contracts to the explicitly resealed Task package."""
import copy
import json
from pathlib import Path
import tempfile
from unittest.mock import patch

import release_package as package
import test_release_package


class Baseline02Tests(test_release_package.ReleasePackageTests):
    expected_blocks = 83

    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        cls.original = json.loads((cls.root / 'docs/audits/baseline-02.json').read_text())

    def test_reseal_retains_original_provenance_without_inheriting_conclusions(self):
        self.assertEqual(len(self.ledger['original_frozen_blocks']), 80)
        self.assertEqual(package.frozen_digest(self.ledger['frozen_blocks']),
                         '7480b2bdfd3d86256d1d13a10f6b2cd9707fb62f365f07e7e244f17a0b510c69')
        origins = [r['source_origin'] for r in self.ledger['reviews']]
        self.assertEqual(origins.count('unchanged'), 78)
        self.assertEqual(origins.count('ADR006_replaced'), 2)
        self.assertEqual(origins.count('ADR006_added'), 3)
        for field in ('original_manifest_sha256', 'original_frozen_blocks'):
            with self.subTest(field=field):
                self.setUp()
                self.ledger[field] = '' if field.endswith('sha256') else []
                with self.assertRaisesRegex(ValueError, 'original manifest provenance'):
                    self.run_audit()

    def test_all_required_duties_need_routes_and_explicit_exemptions(self):
        for mutation in ('kind', 'route', 'exemption'):
            with self.subTest(mutation=mutation):
                self.setUp()
                row = self.ledger['reviews'][0]
                if mutation == 'kind': row['obligation_kinds'] = {}
                if mutation == 'route': row['remaining'].pop()
                if mutation == 'exemption':
                    row['obligation_kinds'][row['obligations'][0]] = 'deferred'
                    row['remaining'].pop(0)
                with self.assertRaises(ValueError): self.run_audit()

    def test_resealed_manifest_must_cover_entire_current_inventory_once(self):
        # Synthetic manifest isolates the assignment consumer. The deployed
        # CLI still pins the coordinator's independently verified byte digest.
        owned = {package.identity(b) for b in self.ledger['frozen_blocks']}
        rest = [{k: u[k] for k in ('source', 'section', 'line', 'sha256')}
                for u in self.units if package.identity(u) not in owned]
        original = dict(packages={
            'baseline-02': dict(blocks=copy.deepcopy(self.ledger['frozen_blocks'])),
            'other-packages': dict(blocks=rest),
        })
        for mutation in ('valid', 'missing', 'duplicate', 'stale', 'extra', 'wrong_owner'):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as temp:
                self.setUp()
                definition = copy.deepcopy(original)
                other = definition['packages']['other-packages']['blocks']
                if mutation == 'missing': other.pop()
                if mutation == 'duplicate': other[-1] = other[0]
                if mutation == 'stale': other[-1]['sha256'] = '0' * 64
                if mutation == 'extra': other.append(copy.deepcopy(other[0]))
                if mutation == 'wrong_owner':
                    other.append(definition['packages']['baseline-02']['blocks'].pop())
                raw = json.dumps(definition).encode()
                path = Path(temp) / 'manifest.json'
                path.write_bytes(raw)
                self.ledger['manifest_sha256'] = package.digest(raw)
                with patch.object(package, 'RESEALED_MANIFEST_HASH', package.digest(raw)):
                    if mutation == 'valid':
                        report = self.run_audit(manifest=path)
                        self.assertTrue(report['manifest_checked'])
                        self.assertFalse(report['release_ready'])
                    else:
                        with self.assertRaises(ValueError): self.run_audit(manifest=path)

    def test_original_manifest_cannot_be_used_for_new_package(self):
        self.ledger['manifest_sha256'] = package.MANIFEST_HASH
        with self.assertRaisesRegex(ValueError, 'frozen package changed'):
            self.run_audit()
