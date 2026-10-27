"""Locked GC-15 source oracle, native receipts and fail-closed consumers."""
import copy
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import prepare_release_eval
import unscripted_cases
import verify_gc15 as journey
import verify_suite


class MultimodalTests(unittest.TestCase):
    def test_frozen_original_blocks_assets_variants_and_existing_repetitions(self):
        lock=journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [355,357,359,360,361,362,363,365,910,912,921,923,925,926,927,928])
        rows=list(journey.variants(*journey.inputs()))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['fixture_digest'] for r in rows}),6)
        self.assertEqual(sum(r['noise_characters']>0 for r in rows),2)
        for row in rows:
            value=unscripted_cases.inputs('GC-15',row['variant_id'])
            self.assertEqual(value['context'],row['context'])
            self.assertEqual(value['events'],[dict(at=e['at'],text=e['text']) for e in row['events']])
            self.assertNotIn('Capabilities:',json.dumps(value))
        self.assertFalse(unscripted_cases.matrix()['GC-15']['eligible_for_grading'])
        self.assertEqual(journey.REPEATS,5)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script=='verify_gc15.py'],[('security',1)])
        self.assertIn('gc15',[g.name for g in verify_suite.security_gates('cases')])
        self.assertEqual(next(g.attempts for g in verify_suite.SHARDS['security'] if g.name=='artifacts'),5)

    def test_independent_date_mask_and_region_oracle(self):
        documents=[dict(name=n,artifactRef='artifact:'+n,**d) for n,d in json.loads((journey.ASSETS/'regions.json').read_text()).items()]
        result=dict(facts=dict(valid_from='2026-09-20',valid_until='2026-10-20',duration_days=15,entries='01',outbound='2026-10-03',inbound_day_month='10-11'),
            calculation=dict(entry='2026-10-03',exit='2026-10-11',inclusive_days=9,window_covers=True,within_duration=True,entry_count=1),
            unknown=dict(return_year=None,order_matches_2026=None,clarification='provide a clear screenshot or other order evidence'),sources=documents)
        self.assertTrue(all(journey.document_checks(result,documents).values()))
        mutations=[lambda d:d['facts'].update(entries='MULT'),lambda d:d['facts'].update(duration_days=30),
            lambda d:d['facts'].update(valid_until='2026-10-11'),lambda d:d['facts'].update(outbound='2026-10-04'),
            lambda d:d['calculation'].update(inclusive_days=8),lambda d:d['calculation'].update(entry_count=True),
            lambda d:d['calculation'].update(within_duration=1),lambda d:d['calculation'].update(window_covers=False),
            lambda d:d['unknown'].update(return_year=2026),lambda d:d['unknown'].update(order_matches_2026=True),
            lambda d:d['unknown'].update(clarification=''),lambda d:d['sources'][0]['fields']['valid_from'].update(region=[1,2,3,4]),
            lambda d:d['sources'][1]['fields']['inbound_year'].update(visible=True,value=2026)]
        for mutate in mutations:
            changed=copy.deepcopy(result);mutate(changed)
            self.assertFalse(all(journey.document_checks(changed,documents).values()))
        source=journey.report_source(documents)
        self.assertIn('image({artifactRef:document.artifactRef})',source)
        self.assertIn('Number(exit.slice(-2))-Number(entry.slice(-2))+1',source)
        self.assertNotIn('const days=9',source)
        self.assertNotIn('agent.run(',source)
        self.assertNotIn('builtin__memory__manage',source)

    def fixture(self,source):
        rows=[]
        for variant in journey.variants(*journey.inputs()):
            for n in range(1,6):
                prefix=variant['variant_id']+'-'+str(n)
                native={table:[{key:prefix+'-'+key}] for table,key in
                    (('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),('deliveries','delivery_id'))}
                rows.append(dict(variant_id=variant['variant_id'],repetition=n,final=native,assertions=dict(observed=True),
                    cleanup=dict(disposable_root_removed=True,owned_processes_joined=True,embedding_peer_joined=True)))
        return dict(case_id='GC-15',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,
            fixture_lock=journey.load(),m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source,binary_sha256=journey.sha(b'binary'),environment=dict(python='test')),
            rejection_probes=dict(bad=True),results=rows)

    def test_receipt_identity_inventory_cleanup_and_copied_repetitions_reject(self):
        source=dict(sha='a'*40,files={'runner':'hash'});report=self.fixture(source)
        with tempfile.TemporaryDirectory() as temp,patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'observe',return_value=dict(observed=True)),patch.object(journey,'rejection_probes',return_value=dict(bad=True)):
            binary=Path(temp)/'erii';binary.write_bytes(b'binary')
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            mutations=[lambda r:r.update(release_ready=True),lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
                lambda r:r.update(eligible_for_grading=True),lambda r:r.update(cleanup=False),lambda r:r.update(fixture_lock={}),
                lambda r:r.update(m6_lock={}),lambda r:r['identity'].update(source_after={}),lambda r:r['identity'].update(binary_sha256='0'*64),
                lambda r:r['identity'].update(environment={}),lambda r:r['results'].pop(),lambda r:r['results'].reverse(),
                lambda r:r['results'][0].update(repetition=True),lambda r:r['results'][0].update(cleanup={}),
                lambda r:r['results'][0].update(assertions=dict(observed=1)),lambda r:r.update(rejection_probes={}),
                lambda r:r['results'][1].update(final=copy.deepcopy(r['results'][0]['final'])),
                lambda r:r['results'][0]['final']['runs'].append(copy.deepcopy(r['results'][0]['final']['runs'][0]))]
            for mutate in mutations:
                changed=copy.deepcopy(report);mutate(changed)
                with self.assertRaises(RuntimeError):journey.validate_report(changed,binary)
            with patch.object(journey,'observe',return_value=dict(observed=False)):
                with self.assertRaises(RuntimeError):journey.validate_report(report,binary)

    def test_manifest_rejects_tamper_and_path_escape(self):
        report=dict(status='mechanical_pass',cleanup=True,results=[dict(payload='NATIVE_SENTINEL'*10000)],
            identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/'gc15.json';manifest=journey.retain_report(path,report)
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('NATIVE_SENTINEL',path.read_text())
            self.assertEqual(journey.read_report(path),report)
            raw=path.parent/manifest['native']['path']
            for mutate in (lambda m:m['summary'].update(receipts=30),lambda m:m['summary'].update(cleanup=1),
                    lambda m:m['summary'].update(semantic_score=100),lambda m:m['native'].update(sha256='0'*64),
                    lambda m:m['native'].update(path='../outside.json'),lambda m:m['native'].update(path=str(raw)),
                    lambda m:m.update(native_bytes=True),lambda m:m.update(version=True)):
                changed=copy.deepcopy(manifest);mutate(changed);path.write_text(json.dumps(changed))
                with self.assertRaises((RuntimeError,ValueError)):journey.read_report(path)
            path.write_text(json.dumps(manifest));saved=raw.with_suffix('.saved');raw.rename(saved);raw.symlink_to(saved)
            with self.assertRaises(ValueError):journey.read_report(path)
            raw.unlink();saved.rename(raw);raw.write_text(raw.read_text()+' ')
            with self.assertRaises(ValueError):journey.read_report(path)

    def test_scripted_preview_never_issues_judge_packets_or_scores(self):
        with patch.object(journey,'validate_report',return_value=dict(receipts=30)) as check:
            preview=prepare_release_eval.prepare(dict(case_id='GC-15'))
        check.assert_called_once()
        self.assertEqual(preview['judge_packets'],[])
        self.assertFalse(preview['eligible_for_grading'])
        self.assertIsNone(preview['scores']);self.assertIsNone(preview['winner'])
        with self.assertRaises(RuntimeError):prepare_release_eval.prepare(dict(case_id='GC-15',semantic_score=100))

    @unittest.skipUnless(os.environ.get('ERII_GC15_NATIVE_ROW'),'requires a retained real CLI receipt')
    def test_actual_native_corruption_recomputed_without_trusting_flags(self):
        row=json.loads(Path(os.environ['ERII_GC15_NATIVE_ROW']).read_text())
        self.assertTrue(all(journey.observe(row).values()))
        probes=journey.rejection_probes(row)
        self.assertGreaterEqual(len(probes),23)
        self.assertTrue(all(probes.values()))


if __name__=='__main__':unittest.main()
