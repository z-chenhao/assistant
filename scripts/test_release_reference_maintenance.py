"""Independent Git histories: edited shared code cannot borrow an audited PASS."""
import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

import maintain_audit_references as maintenance
import release_audit
import release_package as package


class ReferenceMaintenanceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.ledger = json.loads((release_audit.ROOT / 'docs/audits/baseline-01.json').read_text())
        self.units = [dict(source=source, **row)
                      for source in ('docs/technical-design.md', 'evals/golden-cases.md')
                      for row in release_audit.inventory((release_audit.ROOT / source).read_text())]
        for name in {r.partition('#')[0] for r in self.ledger['references']} | {'docs/technical-design.md'}:
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes((release_audit.ROOT / name).read_bytes())
        self.git('init', '-q')
        self.base = self.commit('original synthetic code')
        self.ledger['audited_base'] = self.base
        for obs in self.ledger['observations'].values():
            obs['source_sha'] = self.base
        for pr in self.ledger['merged_prs'].values():
            pr['mergeCommit']['oid'] = self.base
        self.path = self.root / 'docs/audits/baseline-01.json'
        self.path.parent.mkdir(parents=True)
        self.path.write_text(json.dumps(self.ledger))
        self.original_bytes = self.path.read_bytes()
        self.anchor = self.commit('original synthetic ledger')
        self.ref = next(iter(self.ledger['references']))
        self.shared = self.root / self.ref.partition('#')[0]
        self.shared.write_bytes(self.shared.read_bytes() + b'\n// normal shared code edit\n')
        self.current_sha = self.commit('normal synthetic shared edit')
        # The production seals pin the five real merged ledgers. This independent
        # synthetic history needs its own original seal; all Git checks stay real.
        seal = dict(commit=self.anchor, path='docs/audits/baseline-01.json',
                    sha256=package.digest(self.original_bytes),
                    content_sha256=package.frozen_digest(self.ledger))
        patcher = patch.dict(package.REFERENCE_LEDGER_ORIGINALS, {'baseline-01': seal})
        patcher.start()
        self.addCleanup(patcher.stop)

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args], stderr=subprocess.DEVNULL)

    def commit(self, message):
        self.git('add', '.')
        self.git('-c', 'user.name=Synthetic Fixture', '-c', 'user.email=fixture@example.invalid',
                 'commit', '-qm', message)
        return self.git('rev-parse', 'HEAD').decode().strip()

    def refresh(self):
        return maintenance.prepare(self.root, self.path, self.current_sha,
                                   'Synthetic identity check only; native rerun still required.')

    def audit(self, ledger):
        return package.audit_package(self.root, ledger, self.units, verify_git=True)

    def test_normal_edit_requires_refresh_and_old_observations_stay_historical(self):
        with self.assertRaisesRegex(ValueError, 'stale current reference'):
            self.audit(self.ledger)
        refreshed = self.refresh()
        report = self.audit(refreshed)
        self.assertEqual(self.path.read_bytes(), self.original_bytes, 'proposal overwrote original ledger')
        self.assertEqual({k: v for k, v in refreshed.items() if k != 'reference_maintenance'}, self.ledger)
        self.assertEqual(report['observations'], self.ledger['observations'])
        self.assertTrue(all(v['status'] == 'historical_only' and not v['current_native_revalidated']
                            for v in report['observation_applicability'].values()))
        identity = report['reference_identities'][self.ref]
        self.assertEqual(identity['historical_sha256'], self.ledger['references'][self.ref]['sha256'])
        self.assertEqual(identity['current_sha256'], package.digest(self.shared.read_bytes()))
        self.assertNotEqual(identity['current_sha256'], identity['historical_sha256'])
        self.assertEqual(identity['current_source_sha'], self.current_sha)
        self.assertFalse(report['release_ready'])
        self.assertEqual(report['verified_requirements'], 0)
        # A second normal edit fails closed until a second explicit refresh.
        self.path.write_text(json.dumps(refreshed))
        self.commit('maintained synthetic ledger')
        self.shared.write_bytes(self.shared.read_bytes() + b'\n// second edit\n')
        self.current_sha = self.commit('second synthetic edit')
        with self.assertRaisesRegex(ValueError, 'stale current reference'):
            self.audit(refreshed)
        self.audit(self.refresh())

    def test_missing_provenance_revalidation_or_invalidation_rejects_even_without_flag(self):
        good = self.refresh()
        for mutation in ('provenance', 'sha', 'revalidation', 'observations', 'scope', 'reason',
                         'unknown_ref', 'old_hash', 'old_observation', 'old_pr', 'gap', 'proof_limit'):
            with self.subTest(mutation=mutation):
                ledger = copy.deepcopy(good)
                record = ledger['reference_maintenance']
                if mutation == 'provenance': del record['original_ledger']
                if mutation == 'sha': record['original_ledger']['sha256'] = '0' * 64
                if mutation == 'revalidation': record['current']['references'] = {}
                if mutation == 'observations': record['current']['invalidated_observations'] = []
                if mutation == 'scope': record['current']['scope'] = 'native_pass'
                if mutation == 'reason': record['current']['reason'] = ''
                if mutation == 'unknown_ref': record['current']['references']['invented.go'] = {'sha256': '0' * 64}
                if mutation == 'old_hash': ledger['references'][self.ref]['sha256'] = '0' * 64
                if mutation == 'old_observation': next(iter(ledger['observations'].values()))['limits'] = 'new PASS'
                if mutation == 'old_pr': ledger['merged_prs'] = {}
                if mutation == 'gap': ledger['gaps'] = {}
                if mutation == 'proof_limit': ledger['reviews'][0]['proof_limit'] = 'PASS'
                with self.assertRaises(ValueError):
                    package.audit_package(self.root, ledger, self.units)

    def test_wrong_current_sha_fake_blob_missing_symbol_or_uncommitted_change_rejects(self):
        good = self.refresh()
        for mutation in ('unknown_sha', 'short_sha', 'old_sha', 'wrong_hash', 'fake_blob'):
            with self.subTest(mutation=mutation):
                ledger = copy.deepcopy(good)
                current = ledger['reference_maintenance']['current']
                if mutation == 'unknown_sha': current['source_sha'] = '0' * 40
                if mutation == 'short_sha': current['source_sha'] = self.current_sha[:8]
                if mutation == 'old_sha': current['source_sha'] = self.anchor
                if mutation == 'wrong_hash': current['references'][self.ref]['sha256'] = '0' * 64
                if mutation == 'fake_blob':
                    # Matching worktree hash cannot disguise another committed blob.
                    current['references'][self.ref]['sha256'] = package.digest(self.shared.read_bytes() + b'!')
                with self.assertRaises(ValueError): self.audit(ledger)
                with self.assertRaises(ValueError):
                    package.audit_package(self.root, ledger, self.units)
        self.shared.write_bytes(self.shared.read_bytes() + b'!')
        with self.assertRaisesRegex(ValueError, 'current Git blob mismatch'):
            self.refresh()
        with self.assertRaisesRegex(ValueError, 'stale current reference'):
            self.audit(good)

    def test_refresh_cannot_reintroduce_a_missing_symbol(self):
        ref = next(r for r in self.ledger['references'] if '#' in r)
        path, _, symbol = ref.partition('#')
        target = self.root / path
        target.write_text(target.read_text().replace(symbol, 'removed_symbol'))
        self.current_sha = self.commit('remove synthetic symbol')
        with self.assertRaisesRegex(ValueError, 'missing current symbol'):
            self.refresh()

    def test_fake_matching_worktree_and_hash_still_rejects_the_committed_blob(self):
        good = self.refresh()
        self.shared.write_bytes(self.shared.read_bytes() + b'!')
        good['reference_maintenance']['current']['references'][self.ref]['sha256'] = package.digest(self.shared.read_bytes())
        with self.assertRaisesRegex(ValueError, 'current Git blob mismatch'):
            package.audit_package(self.root, good, self.units)

    def test_removing_refresh_and_rewriting_the_old_hash_cannot_bypass_the_seal(self):
        good = self.refresh()
        del good['reference_maintenance']
        good['references'][self.ref]['sha256'] = package.digest(self.shared.read_bytes())
        with self.assertRaisesRegex(ValueError, 'original audit ledger changed'):
            package.audit_package(self.root, good, self.units)

    def test_stronger_source_ancestry_check_is_not_silently_relaxed(self):
        ledger = self.refresh()
        # An orphan source commit cannot borrow current provenance. No history
        # fallback is introduced by the maintenance path.
        ledger['audited_base'] = '0' * 40
        with self.assertRaisesRegex(ValueError, 'package Git binding rejected'):
            self.audit(ledger)

    def test_frozen_duplicate_and_ambiguous_sources_still_reject_after_refresh(self):
        ledger = self.refresh()
        ledger['frozen_blocks'].append(copy.deepcopy(ledger['frozen_blocks'][0]))
        with self.assertRaises(ValueError): self.audit(ledger)
        ledger = self.refresh()
        units = self.units + [copy.deepcopy(next(u for u in self.units
                             if package.identity(u) == package.identity(ledger['frozen_blocks'][0])))]
        with self.assertRaisesRegex(ValueError, 'ambiguous frozen'):
            package.audit_package(self.root, ledger, units)

    def test_cli_proposal_cleanup_and_input_protection(self):
        output = self.root / 'evidence/proposal.json'
        command = ['maintenance', '--ledger', str(self.path), '--source-sha', self.current_sha,
                   '--reason', 'Synthetic reference identity check', '--output', str(output)]
        with patch.object(maintenance, 'ROOT', self.root), patch.object(sys, 'argv', command):
            maintenance.main()
        self.audit(json.loads(output.read_text()))
        self.assertEqual(self.path.read_bytes(), self.original_bytes)
        self.shared.write_bytes(self.shared.read_bytes() + b'!')
        with patch.object(maintenance, 'ROOT', self.root), patch.object(sys, 'argv', command):
            with self.assertRaises(ValueError): maintenance.main()
        self.assertFalse(output.exists(), 'failed refresh left a stale proposal')
        command[-1] = str(self.path)
        with patch.object(maintenance, 'ROOT', self.root), patch.object(sys, 'argv', command):
            with self.assertRaises(SystemExit): maintenance.main()
        self.assertEqual(self.path.read_bytes(), self.original_bytes)


if __name__ == '__main__':
    unittest.main()
