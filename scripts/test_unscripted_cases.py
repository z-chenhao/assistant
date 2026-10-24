"""Producer contracts; no model, credentials or semantic grades."""
import copy
import json
from pathlib import Path
import tempfile
import unittest

from case_runner import digest, load_contract, variants
from release_receipts import CASES, PACKET_FIELDS
from unscripted_cases import ROOT, INPUTS, artifact_receipts, configuration, frozen_requests, inputs, matrix
from release_receipts import hashed


class UnscriptedCasesTests(unittest.TestCase):
    def test_export_requires_native_artifact_size_hash_and_safe_storage_key(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            folder = root/'data/artifacts'
            folder.mkdir(parents=True)
            out = root/'output'
            out.mkdir()
            data = b'actual synthetic Artifact bytes\x00\xff'
            (folder/'object').write_bytes(data)
            artifact = dict(deleted_at=None, storage_key='object', artifact_ref='artifact:test',
                            content_hash=hashed(data), byte_size=len(data))
            result = artifact_receipts(root, out, dict(artifact=[artifact]))
            self.assertEqual((out/result[0]['path']).read_bytes(), data)
            self.assertEqual(result[0]['sha256'], hashed(data))
            for overrides in ({'content_hash': 'a'*64}, {'byte_size': 0}, {'storage_key': '../escape'}):
                with self.subTest(overrides=overrides), self.assertRaises(ValueError):
                    artifact_receipts(root, out, dict(artifact=[dict(artifact, **overrides)]))

    def test_every_case_has_an_explicit_ungraded_disposition(self):
        support = matrix()
        self.assertEqual(set(support), CASES)
        self.assertEqual(len(INPUTS), 12)
        for case, row in support.items():
            self.assertFalse(row['eligible_for_grading'])
            self.assertTrue(row['missing'])
            if row['adapter'] is None:
                with self.assertRaisesRegex(ValueError, 'missing adapter'):
                    inputs(case, 'wording-1')

    def test_all_input_variants_preserve_context_noise_order_and_never_send_gold(self):
        for case_id, fixture in INPUTS.items():
            dataset, _, lock = load_contract(ROOT, fixture)
            case = next(c for c in dataset['cases'] if c['case_id'] == case_id)
            for variant in variants(dataset, case):
                with self.subTest(case=case_id, variant=variant['variant_id']):
                    value = inputs(case_id, variant['variant_id'])
                    self.assertEqual(value['context'], variant['context'])
                    self.assertEqual(value['lock'], lock)
                    self.assertEqual(value['fixture_digest'], variant['fixture_digest'])
                    self.assertEqual(value['events'], [dict(at=e['at'], text=e['text']) for e in variant['events']])
                    for event in value['events']:
                        self.assertEqual(set(event), {'at', 'text'})
                        self.assertNotIn('Capabilities:', event['text'])
                        self.assertNotIn('Delegate:', event['text'])
        with self.assertRaisesRegex(ValueError, 'unknown variant'):
            inputs('GC-09', 'invented')

    def test_configuration_cannot_attach_external_or_user_state(self):
        valid = dict(version=1, llm=dict(provider_bridge_command='node',
            provider_bridge_script=str(ROOT/'provider-bridge/bridge.mjs'),
            provider_bridge_version='pi-ai-1.3.0-v1'))
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp)/'config.json'
            path.write_text(json.dumps(valid))
            self.assertEqual(configuration(path), valid)
            for field in ('app', 'mcp', 'workspace', 'memory', 'embedding', 'computer', 'skills', 'feishu'):
                path.write_text(json.dumps(dict(valid, **{field: {}})))
                with self.subTest(field=field), self.assertRaises(ValueError):
                    configuration(path)
            invalid = copy.deepcopy(valid)
            invalid['llm']['provider_bridge_script'] = '/tmp/custom-bridge.mjs'
            path.write_text(json.dumps(invalid))
            with self.assertRaises(ValueError):
                configuration(path)

    def test_model_endpoint_parameters_and_tool_drift_are_rejected(self):
        profile = dict(profile_id='p', version=2, model_id='model', provider_kind='openai_compatible',
            endpoint='http://127.0.0.1:1/v1', wire_api='openai_completions', context_window=8192,
            max_output_tokens=1024, generation_defaults={}, compatibility={}, capabilities={})
        snapshot = {k: v for k, v in profile.items() if k not in ('version', 'endpoint')}
        snapshot.update(profile_version=2, endpoint_hash=digest({'endpoint': profile['endpoint']}))
        requests = [dict(run_id='r', model_snapshot=copy.deepcopy(snapshot), tools=[{'name': 'one'}]) for _ in range(2)]
        def state(values):
            return dict(generations=[dict(request_json=json.dumps(r)) for r in values])
        self.assertTrue(frozen_requests(state(requests), profile))
        self.assertFalse(frozen_requests(state([]), profile))
        for key, value in (('model_id', 'other'), ('profile_version', 3), ('endpoint_hash', 'a'*64),
                           ('context_window', 9999), ('generation_defaults', {'temperature': 1})):
            changed = copy.deepcopy(requests)
            changed[0]['model_snapshot'][key] = value
            with self.subTest(key=key):
                self.assertFalse(frozen_requests(state(changed), profile))
        changed = copy.deepcopy(requests)
        changed[0]['tools'] = []
        self.assertFalse(frozen_requests(state(changed), profile))


if __name__ == '__main__':
    unittest.main()
