import unittest

from clean_branches import select


class BranchSelectionTests(unittest.TestCase):
    def test_unique_work_worktrees_and_open_prs_are_protected(self):
        branches = dict(main='base', current='base', active='merged',
                        snapshot='unique', safe='base', squash='squash', drift='new')
        prs = [dict(state='OPEN', headRefName='active', headRefOid='merged'),
               dict(state='MERGED', headRefName='squash', headRefOid='squash'),
               dict(state='MERGED', headRefName='drift', headRefOid='old')]
        report = select(branches, branches, {'base', 'merged'}, ['current'], prs)
        self.assertEqual({(x['scope'], x['branch']) for x in report['remove']},
                         {('local', 'safe'), ('remote', 'safe'), ('local', 'squash'), ('remote', 'squash')})
        self.assertTrue(all(x['sha'] for x in report['remove']))

    def test_missing_or_incomplete_pr_inventory_cannot_prove_a_merge(self):
        report = select({'snapshot': 'unique'}, {'unknown': 'unknown'}, set(), [], [])
        self.assertEqual(report['remove'], [])
        self.assertEqual(len(report['keep']), 2)
