"""Synthetic mechanism composition, never real Erii model/release evidence."""
import copy
from datetime import datetime, timezone
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from case_runner import digest
import ci_gate
import release_audit as audit
import release_decision as decision
import release_receipts as receipts

ROOT = decision.ROOT
NOW = datetime.now(timezone.utc)
OBSERVED = receipts.git(ROOT, 'show', '-s', '--format=%cI', 'HEAD').decode().strip()


class PackFixture:
    """Disposable files exercise the consumer; no generated score is product evidence."""
    def __init__(self, folder):
        self.folder = folder
        self.counter = 0
        self.sha = receipts.git(ROOT, 'rev-parse', 'HEAD').decode().strip()
        self.inventory = audit.audit(ROOT, json.loads((ROOT / 'docs/v1-release-ledger.json').read_text()))
        model = dict(provider_kind='openai_compatible', model_id='qwen-synthetic', wire_api='openai_responses',
                     context_window=8192, max_output_tokens=1024, generation_defaults={},
                     compatibility={'tool_result_format': 'openai'}, capabilities={'text_input': True}, endpoint_hash='a' * 64)
        conditions = {key: 'a' * 64 for key in decision.CONDITIONS}
        conditions.update(limits={'context_window': 8192, 'max_output_tokens': 1024}, seed=0, timezone='Asia/Shanghai')
        suite = dict(cases={case: dict(assertions=['goal', 'safe'], applicable_dimensions=list('OFPATSRC'),
                                      not_applicable={}, conditions={variant: conditions for variant in receipts.VARIANTS})
                            for case in sorted(receipts.CASES)},
                     models={'local': model, 'account': dict(model, provider_kind='openai_account', model_id='account-synthetic')},
                     systems={name: ['observed'] for name in decision.SYSTEMS},
                     safety={name: ['observed'] for name in decision.SAFETY})
        self.binding = dict(release_sha=self.sha,
            sources={p: row['sha256'] for p, row in self.inventory['sources'].items()},
            scoring_sha256=receipts.hashed((ROOT / 'evals/scoring-v1.json').read_bytes()), suite_sha256=digest(suite))
        self.raw = self.file(dict(note='SYNTHETIC MECHANISM TEST ONLY'))
        pr_response = self.file(dict(number=1, url='https://github.com/z-chenhao/erii/pull/1',
            state='MERGED', mergedAt='2026-10-23T00:00:00Z', mergedBy={'login': 'synthetic-maintainer'},
            mergeCommit={'oid': self.sha}, repository={'nameWithOwner': 'z-chenhao/erii'}))
        pr = self.record('merged_pr', repository='z-chenhao/erii', number=1,
                         url='https://github.com/z-chenhao/erii/pull/1', state='MERGED', merge_commit=self.sha,
                         merged_at='2026-10-23T00:00:00Z', merged_by='synthetic-maintainer', raw_evidence=[self.raw],
                         native_response=pr_response)
        data = receipts.git(ROOT, 'show', self.sha + ':scripts/case_runner.py')
        code = dict(path='scripts/case_runner.py', sha256=receipts.hashed(data), symbol='def score(')
        clauses = []
        for unit in self.inventory['source_units']:
            key = decision.source_key(unit)
            native = self.record('clause_evidence', source_key=key, producer='synthetic-observer',
                                 command='synthetic-only', author='author', assertions={'observed': True}, raw_evidence=[self.raw])
            entry = dict(source_key=key, disposition='verified', reason='synthetic composition',
                         code=[code], tests=[code], merged_pr=pr, native=native)
            entry['review'] = self.review(digest({k: entry[k] for k in ('source_key', 'code', 'tests', 'merged_pr', 'native')}))
            clauses.append(entry)
        trials = []
        packet = dict(goal='synthetic goal', allowed_fixture={'synthetic': True},
                      conversation=[{'role': 'assistant', 'text': 'observed synthetic answer'}],
                      final_artifact_summary=[], programmatic_assertions={'goal': True, 'safe': True})
        packet_ref = self.file(packet)
        grade = dict(score=4, reason='synthetic test', citations=[dict(pointer='/conversation/0/text', quote='observed')])
        response = self.file(dict(dimensions={d: grade for d in receipts.SEMANTIC}))
        for arm in sorted(receipts.ARMS):
            for case in sorted(receipts.CASES):
                for variant in sorted(receipts.VARIANTS):
                    for repetition in range(1, 6):
                        identity = dict(case_id=case, variant_id=variant, arm=arm, repetition=repetition)
                        native = self.record('trial', identity=identity, execution_id=f'{arm}-{case}-{variant}-{repetition}',
                            execution_kind='live_model', eligible_for_grading=True, scope='whole_case', cleanup=True,
                            model_snapshot=suite['models'][arm], conditions=conditions,
                            assertions=packet['programmatic_assertions'],
                            hard_failure_checks={name: True for name in json.loads((ROOT / 'evals/scoring-v1.json').read_text())['hard_failure_categories']},
                            runner='author', raw_evidence=[self.raw], judge_input=packet_ref,
                            mechanical_dimensions={d: grade for d in receipts.MECHANICAL},
                            metrics=dict(latency_ms=1, first_useful_feedback_ms=1, model_calls=1, tool_calls=1,
                                resources={'synthetic': True}, user_questions=0, approvals=0, unhelpful_notifications=0,
                                durable={'synthetic': True}, memory={'synthetic': True}, coding={'synthetic': True}))
                        judge = self.record('judge', trial_sha256=native['sha256'], judge_input_sha256=packet_ref['sha256'],
                                            judge_model='synthetic-judge', judge_actor='independent-judge', native_response=response)
                        trials.append(dict(trial=native, judge=judge))
        self.pack = dict(version=1, author='author', binding=self.binding, suite=suite, clauses=clauses, trials=trials,
                         scope_review=self.review(digest(suite)),
                         systems={name: [self.gate(name, i) for i in range(1, 6 if name in decision.PROTECTED_SYSTEMS else 2)]
                                  for name in decision.SYSTEMS},
                         safety={name: [self.gate(name, i) for i in range(1, 6)] for name in decision.SAFETY},
                         operations={name: self.gate(name) for name in decision.OPERATIONS},
                         live={name: self.gate(name, live=True, model=model) for name in decision.LIVE})
        platforms = {}
        for i, platform in enumerate(('ubuntu', 'macos'), 1):
            platforms[platform] = self.file(dict(repository={'full_name': 'z-chenhao/erii'}, head_sha=self.sha,
                id=i, run_attempt=1, html_url=f'https://github.com/z-chenhao/erii/actions/runs/{i}',
                path=f".github/workflows/{'ci' if platform == 'ubuntu' else 'release'}.yml",
                jobs=[dict(id=n, run_id=i, run_attempt=1, name=name, conclusion='success')
                      for n, name in enumerate(['Ubuntu checks'] if platform == 'ubuntu' else [
                          'Package (ubuntu-24.04)', 'Package (ubuntu-24.04-arm)', 'Package (macos-15)', 'Package (macos-15-intel)'], 1)],
                status='completed', conclusion='success'))
        needs = self.file(dict(source_sha=self.sha, run_id=1, run_attempt=1,
                               needs={key: {'result': 'success'} for key in ci_gate.REQUIRED}))
        self.pack['native_ci'] = self.record('native_ci', platforms=platforms, ubuntu_needs=needs)
        self.accept(self.pack)

    def file(self, value):
        data = (json.dumps(value, ensure_ascii=False, sort_keys=True) + '\n').encode()
        self.counter += 1
        path = f'raw-{self.counter}.json'
        (self.folder / path).write_bytes(data)
        return dict(path=path, sha256=receipts.hashed(data))

    def read(self, ref):
        return json.loads((self.folder / ref['path']).read_text())

    def record(self, kind, **fields):
        return self.file(dict(kind=kind, binding=self.binding, **fields))

    def review(self, subject, **overrides):
        return self.record('independent_review', **dict(dict(subject_sha256=subject, mode='acceptance',
            reviewer='independent-reviewer', actor_type='member', reason='synthetic only', origin='synthetic test authority',
            reviewed_at=OBSERVED, expires_at='2099-01-01T00:00:00Z',
            raw_evidence=[self.raw], verdict='PASS'), **overrides))

    def gate(self, name, repetition=1, live=False, model=None):
        value = dict(kind='live_observation' if live else 'gate', binding=self.binding, name=name,
                     execution_id=f'{name}-{repetition}',
                     producer='synthetic-observer', command='synthetic-only', author='author',
                     assertions={'observed': True}, repetition=repetition,
                     raw_evidence=[self.raw], observed_at=OBSERVED)
        if live:
            value.update(level='L2', environment='synthetic', failure_class='none', model_snapshot=model,
                         actual_state={'synthetic': True}, artifacts=[self.raw])
        value['review'] = self.review(digest(value))
        return self.file(value)

    def accept(self, pack):
        pack.pop('human_acceptance', None)
        pack['human_acceptance'] = self.review(digest(pack), reviewer='named-human')

    def decide(self, pack):
        path = self.folder / 'pack.json'
        path.write_text(json.dumps(pack))
        return decision.decide(ROOT, path, receipts.hashed(path.read_bytes()), NOW)

    def mutate_trial(self, pack, change):
        row = pack['trials'][0]
        value = self.read(row['trial'])
        change(value)
        row['trial'] = self.file(value)
        judge = self.read(row['judge'])
        judge['trial_sha256'] = row['trial']['sha256']
        row['judge'] = self.file(judge)

    def rescore(self, pack, number):
        # Rewrite native and Judge receipts coherently, rather than injecting an
        # aggregate score that bypasses the actual receipt/scoring boundary.
        for row in pack['trials']:
            native = self.read(row['trial'])
            for value in native['mechanical_dimensions'].values(): value['score'] = number
            row['trial'] = self.file(native)
            judge = self.read(row['judge'])
            response = self.read(judge['native_response'])
            for value in response['dimensions'].values(): value['score'] = number
            judge.update(trial_sha256=row['trial']['sha256'], native_response=self.file(response))
            row['judge'] = self.file(judge)


class ReleaseDecisionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp = tempfile.TemporaryDirectory(prefix='erii-release-mechanism-')
        cls.fixture = PackFixture(Path(cls.temp.name))

    @classmethod
    def tearDownClass(cls):
        cls.temp.cleanup()

    def setUp(self):
        self.pack = copy.deepcopy(self.fixture.pack)

    def test_complete_synthetic_composition_is_satisfiable_but_not_release_evidence(self):
        report = self.fixture.decide(self.pack)
        self.assertTrue(report['release_ready'])
        self.assertEqual(report['individually_verified_requirements'], len(self.fixture.inventory['source_units']))
        self.assertEqual(report['legacy_inventory']['individually_reviewed_blocks'], 122)
        for arm in receipts.ARMS:
            self.assertEqual(report['evaluation']['arms'][arm]['suite_mean'], 100)
            for case in receipts.CASES:
                self.assertEqual(report['evaluation']['cases'][arm][case]['pass_power_5'], 1)

    def test_missing_clauses_or_trials_never_infer_success(self):
        for field in ('clauses', 'trials'):
            with self.subTest(field=field):
                pack = copy.deepcopy(self.pack)
                pack[field].pop()
                self.fixture.accept(pack)
                report = self.fixture.decide(pack)
                self.assertFalse(report['release_ready'])
                self.assertEqual(report['status'], 'inconclusive')

    def test_all_required_gates_missing_rounds_or_human_fail_closed(self):
        for category, name in [('systems', 'SC-03'), ('safety', 'durable_replay'),
                               ('operations', 'backup_restore'), ('live', 'computer')]:
            with self.subTest(category=category):
                pack = copy.deepcopy(self.pack)
                pack[category].pop(name)
                self.fixture.accept(pack)
                self.assertFalse(self.fixture.decide(pack)['release_ready'])
        self.pack['human_acceptance'] = None
        self.assertFalse(self.fixture.decide(self.pack)['release_ready'])

    def test_protected_rounds_are_five_unique_ordered_executions(self):
        for name in decision.PROTECTED_SYSTEMS | decision.SAFETY:
            category = 'systems' if name in decision.PROTECTED_SYSTEMS else 'safety'
            pack = copy.deepcopy(self.pack)
            pack[category][name].pop()
            self.fixture.accept(pack)
            self.assertFalse(self.fixture.decide(pack)['release_ready'])
        self.pack['systems']['SC-03'].reverse()
        with self.assertRaises(ValueError):
            self.fixture.decide(self.pack)

    def test_duplicate_trial_source_and_wrong_suite_sha_are_rejected(self):
        for field in ('trials', 'clauses'):
            pack = copy.deepcopy(self.pack)
            pack[field].append(pack[field][0])
            with self.assertRaises(ValueError):
                self.fixture.decide(pack)
        self.pack['binding']['suite_sha256'] = '0' * 64
        with self.assertRaises(ValueError):
            self.fixture.decide(self.pack)

    def test_ineligible_partial_fake_model_condition_and_assertion_drift_reject(self):
        mutations = [lambda v: v.update(eligible_for_grading=False), lambda v: v.update(execution_kind='scripted_fake'),
                     lambda v: v.update(scope='fragment'), lambda v: v['model_snapshot'].update(model_id='drift'),
                     lambda v: v['conditions'].update(seed=42), lambda v: v['assertions'].pop('safe'),
                     lambda v: v['hard_failure_checks'].pop('secret_or_scope_leak'),
                     lambda v: v['binding'].update(release_sha='0' * 40)]
        for change in mutations:
            with self.subTest(change=change):
                pack = copy.deepcopy(self.pack)
                self.fixture.mutate_trial(pack, change)
                with self.assertRaises(ValueError):
                    self.fixture.decide(pack)

    def test_null_grade_is_inconclusive_and_mechanical_failure_overrides_judge(self):
        self.fixture.mutate_trial(self.pack, lambda v: v['mechanical_dimensions']['T'].update(score=None))
        self.fixture.accept(self.pack)
        report = self.fixture.decide(self.pack)
        self.assertFalse(report['release_ready'])
        self.assertIsNone(report['evaluation']['cases']['account']['GC-01']['score'])
        for category in ('assertions', 'hard_failure_checks'):
            pack = copy.deepcopy(self.fixture.pack)
            self.fixture.mutate_trial(pack, lambda v: v[category].update({next(iter(v[category])): False}))
            # Assertion drift relative to the Judge input rejects even earlier.
            if category == 'assertions':
                with self.assertRaises(ValueError):
                    self.fixture.decide(pack)
            else:
                self.fixture.accept(pack)
                result = self.fixture.decide(pack)
                self.assertFalse(result['release_ready'])
                self.assertEqual(result['evaluation']['cases']['account']['GC-01']['minimum'], 0)

    def test_judge_binding_dimension_citation_and_invalid_numeric_reject(self):
        for mutation in ('subject', 'packet', 'dimension', 'citation', 'bool', 'range'):
            with self.subTest(mutation=mutation):
                pack = copy.deepcopy(self.pack)
                row = pack['trials'][0]
                judge = self.fixture.read(row['judge'])
                if mutation == 'subject': judge['trial_sha256'] = '0' * 64
                elif mutation == 'packet': judge['judge_input_sha256'] = '0' * 64
                else:
                    response = self.fixture.read(judge['native_response'])
                    if mutation == 'dimension': response['dimensions'].pop('O')
                    if mutation == 'citation': response['dimensions']['O']['citations'][0]['quote'] = 'absent'
                    if mutation == 'bool': response['dimensions']['O']['score'] = True
                    if mutation == 'range': response['dimensions']['O']['score'] = 4.1
                    judge['native_response'] = self.fixture.file(response)
                row['judge'] = self.fixture.file(judge)
                with self.assertRaises(ValueError):
                    self.fixture.decide(pack)

    def test_old_l2_expired_rejected_or_checkpoint_review_never_pass(self):
        for mutation in ('sha', 'expiry', 'rejected', 'checkpoint', 'human'):
            pack = copy.deepcopy(self.pack)
            gate = self.fixture.read(pack['live']['computer'])
            if mutation == 'sha': gate['binding']['release_sha'] = '0' * 40
            else:
                review = self.fixture.read(gate['review'])
                if mutation == 'expiry': review['expires_at'] = '2026-10-22T00:00:00Z'
                if mutation == 'rejected': review['verdict'] = 'REJECT'
                if mutation == 'checkpoint': review['mode'] = 'checkpoint'
                if mutation == 'human': review['reviewer'] = ''
                gate['review'] = self.fixture.file(review)
            pack['live']['computer'] = self.fixture.file(gate)
            self.fixture.accept(pack)
            if mutation in ('expiry', 'rejected'):
                result = self.fixture.decide(pack)
                self.assertFalse(result['release_ready'])
                self.assertEqual(result['gates']['computer'], 'expired' if mutation == 'expiry' else 'rejected')
            else:
                with self.assertRaises(ValueError):
                    self.fixture.decide(pack)

    def test_unmerged_pr_and_current_reference_tamper_rejected(self):
        for mutation in ('pr', 'code', 'source'):
            pack = copy.deepcopy(self.pack)
            if mutation == 'pr':
                pr = self.fixture.read(pack['clauses'][0]['merged_pr'])
                pr['state'] = 'OPEN'
                pack['clauses'][0]['merged_pr'] = self.fixture.file(pr)
            elif mutation == 'code': pack['clauses'][0]['code'][0]['sha256'] = '0' * 64
            else: pack['binding']['sources']['evals/golden-cases.md'] = '0' * 64
            with self.assertRaises(ValueError):
                self.fixture.decide(pack)

    def test_ci_wrong_sha_failed_join_missing_native_platform_never_pass(self):
        for mutation in ('sha', 'join', 'platform', 'join-sha', 'join-attempt'):
            pack = copy.deepcopy(self.pack)
            ci = self.fixture.read(pack['native_ci'])
            if mutation == 'sha':
                run = self.fixture.read(ci['platforms']['macos'])
                run['head_sha'] = '0' * 40
                ci['platforms']['macos'] = self.fixture.file(run)
            elif mutation == 'platform': ci['platforms'].pop('macos')
            else:
                join = self.fixture.read(ci['ubuntu_needs'])
                if mutation == 'join': join['needs'] = {'fast': {'result': 'success'}}
                if mutation == 'join-sha': join['source_sha'] = '0' * 40
                if mutation == 'join-attempt': join['run_attempt'] = 2
                ci['ubuntu_needs'] = self.fixture.file(join)
            pack['native_ci'] = self.fixture.file(ci)
            self.fixture.accept(pack)
            if mutation == 'join':
                self.assertFalse(self.fixture.decide(pack)['release_ready'])
            else:
                with self.assertRaises(ValueError):
                    self.fixture.decide(pack)

    def test_json_duplicate_keys_nan_tamper_and_path_escape_rejected(self):
        for data in ('{"x":1,"x":2}', '{"x":NaN}', '{"x":1e999}'):
            with self.assertRaises(ValueError): receipts.strict_json(data)
        store = receipts.Receipts(self.fixture.folder, self.fixture.binding)
        for ref in (dict(path='../outside', sha256='0' * 64), dict(path=self.fixture.raw['path'], sha256='0' * 64)):
            with self.assertRaises(ValueError): store.bytes(ref)

    def test_real_cli_rejects_bad_pack_hash_and_replaces_stale_success(self):
        folder = ROOT / 'evidence'
        folder.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='release-cli-', dir=folder) as directory:
            output = Path(directory) / 'result.json'
            output.write_text('{"release_ready":true}')
            path = self.fixture.folder / 'pack.json'
            path.write_text(json.dumps(self.pack))
            result = subprocess.run([sys.executable, str(ROOT / 'scripts/release_decision.py'), '--pack', str(path),
                '--pack-sha256', '0' * 64, '--evidence', str(output), '--require-complete'], capture_output=True, text=True)
            self.assertEqual(result.returncode, 1, result.stderr)
            self.assertEqual(json.loads(output.read_text())['status'], 'rejected')
            self.assertFalse(json.loads(output.read_text())['release_ready'])

    def test_real_cli_positive_and_incomplete_paths_have_distinct_exit_codes(self):
        folder = ROOT / 'evidence'
        folder.mkdir(exist_ok=True)
        with tempfile.TemporaryDirectory(prefix='release-cli-', dir=folder) as directory:
            output = Path(directory) / 'result.json'
            path = self.fixture.folder / 'pack.json'
            for complete, expected in [(True, 0), (False, 2)]:
                pack = copy.deepcopy(self.pack)
                if not complete:
                    pack['trials'] = []
                    self.fixture.accept(pack)
                path.write_text(json.dumps(pack))
                result = subprocess.run([sys.executable, str(ROOT / 'scripts/release_decision.py'), '--pack', str(path),
                    '--pack-sha256', receipts.hashed(path.read_bytes()), '--evidence', str(output), '--require-complete'],
                    capture_output=True, text=True)
                self.assertEqual(result.returncode, expected, result.stderr)
                self.assertEqual(json.loads(output.read_text())['release_ready'], complete)

    def test_suite_threshold_uses_unrounded_scores(self):
        for grade, ready in [(3.4, True), (3.399999, False)]:
            with self.subTest(grade=grade):
                pack = copy.deepcopy(self.pack)
                self.fixture.rescore(pack, grade)
                self.fixture.accept(pack)
                report = self.fixture.decide(pack)
                self.assertEqual(report['release_ready'], ready)
                if not ready:
                    self.assertLess(report['evaluation']['arms']['local']['suite_mean'], 85)

    def test_each_case_minimum_and_five_protected_trials_cannot_be_averaged_away(self):
        for case in decision.PROTECTED:
            pack = copy.deepcopy(self.pack)
            row = next(r for r in pack['trials'] if self.fixture.read(r['trial'])['identity']['case_id'] == case)
            native = self.fixture.read(row['trial'])
            native['hard_failure_checks']['workspace_damage'] = False
            changed = {'trials': [row]}
            self.fixture.mutate_trial(changed, lambda v: v.update(hard_failure_checks=native['hard_failure_checks']))
            self.fixture.accept(pack)
            report = self.fixture.decide(pack)
            self.assertFalse(report['release_ready'])
            self.assertEqual(report['evaluation']['cases']['account'][case]['status'], 'rejected')

    def test_missing_eligible_semantic_dimensions_and_grade_threshold(self):
        policy = json.loads((ROOT / 'evals/scoring-v1.json').read_text())
        for value, status in [(3.2, 'pass'), (3.199999, 'fail')]:
            self.assertEqual(receipts.score(policy, {'required': True}, [], {'O': value}, ['O'])['status'], status)
        for kind in ('null', 'absent'):
            pack = copy.deepcopy(self.pack)
            row = pack['trials'][0]
            judge = self.fixture.read(row['judge'])
            response = self.fixture.read(judge['native_response'])
            if kind == 'null': response['dimensions']['O']['score'] = None
            else: response['dimensions'].pop('O')
            judge['native_response'] = self.fixture.file(response)
            row['judge'] = self.fixture.file(judge)
            if kind == 'absent':
                with self.assertRaises(ValueError): self.fixture.decide(pack)
            else:
                self.fixture.accept(pack)
                self.assertFalse(self.fixture.decide(pack)['release_ready'])

    def test_missing_observed_metrics_remain_inconclusive(self):
        self.fixture.mutate_trial(self.pack, lambda v: v['metrics'].update(latency_ms=None))
        self.fixture.accept(self.pack)
        result = self.fixture.decide(self.pack)
        self.assertEqual(result['gates']['reporting_metrics'], 'unmet')
        self.assertFalse(result['release_ready'])

    def test_model_identity_only_normalizes_installation_fields(self):
        model = self.pack['suite']['models']['local']
        expected = receipts.model_identity(model)
        self.assertEqual(receipts.model_identity(dict(model, snapshot_id='a', profile_id='b', created_at='c')), expected)
        for field, value in [('endpoint_hash', 'b' * 64), ('reasoning_effort', 'high'), ('future_option', True)]:
            self.assertNotEqual(receipts.model_identity(dict(model, **{field: value})), expected)

    def test_native_metadata_and_rehashed_reused_execution_are_rejected(self):
        pr = self.fixture.read(self.pack['clauses'][0]['merged_pr'])
        native = self.fixture.read(pr['native_response'])
        native['state'] = 'OPEN'
        pr['native_response'] = self.fixture.file(native)
        self.pack['clauses'][0]['merged_pr'] = self.fixture.file(pr)
        with self.assertRaises(ValueError): self.fixture.decide(self.pack)
        pack = copy.deepcopy(self.fixture.pack)
        first, second = pack['systems']['SC-03'][:2]
        value = self.fixture.read(second)
        value['execution_id'] = self.fixture.read(first)['execution_id']
        value['review'] = self.fixture.review(digest({k: v for k, v in value.items() if k != 'review'}))
        pack['systems']['SC-03'][1] = self.fixture.file(value)
        with self.assertRaises(ValueError): self.fixture.decide(pack)


if __name__ == '__main__':
    unittest.main()
