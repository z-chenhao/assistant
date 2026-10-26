"""GC-13 frozen inputs, independent receipts and fail-closed consumers."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

from case_runner import variants
import ci_communication as ci
import verify_gc13 as journey
import verify_suite


class BookingTests(unittest.TestCase):
    def test_frozen_nineteen_blocks_and_original_input_variants(self):
        lock=journey.load()
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],[327,329,331,332,333,334,335,337,855,857,866,868,869,870,871,873,875,876,877])
        rows=list(variants(*journey.inputs()))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({v['events'][0]['text'] for v in rows}),3)
        self.assertEqual(sum(v['noise_characters']>0 for v in rows),2)
        self.assertEqual(journey.REPEATS,5)
        gate=verify_suite.Gate('gc13','verify_gc13.py','gc13')
        self.assertEqual(ci.pairs(gate),[(v['variant_id'],n) for v in rows for n in range(1,6)])
        workflow=(journey.ROOT/'.github/workflows/ci.yml').read_text()
        self.assertIn('communicationpart: gc13',workflow)
        self.assertIn('for part in gc01 gc02 gc09 gc10 gc13-wording gc13-noise channels;',workflow)
        self.assertIn("matrix.shard == 'delivery' || startsWith(matrix.communicationpart, 'gc13-')",workflow)

    @staticmethod
    def identities(prefix):
        return {table:[{key:prefix+'/'+key}] for table,key in (
            ('runs','run_id'),('tools','invocation_id'),('generations','generation_id'),
            ('conversation','message_id'),('deliveries','delivery_id'),('artifact','artifact_ref'))}

    def report(self,source):
        return dict(case_id='GC-13',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,
            fixture_lock=journey.load(),m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source,binary_sha256=journey.sha(b'binary'),environment=dict(platform='test')),
            rejection_probes={'corrupt':True},results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'native':True},
                cleanup=dict(owned_processes_joined=True,disposable_root_removed=True),started_at='2026-10-15T10:00:00Z',finished_at='2026-10-15T10:01:00Z',
                final=self.identities(v['variant_id']+str(n))) for v in variants(*journey.inputs()) for n in range(1,6)])

    def test_full_native_consumer_rejects_copying_reordering_grades_cleanup_and_identity_drift(self):
        source=dict(sha='a'*40,files={'runner':'hash'})
        with tempfile.TemporaryDirectory() as temp,patch.object(journey,'source_identity',return_value=source),\
                patch.object(journey,'observe',return_value={'native':True}),patch.object(journey,'rejection_probes',return_value={'corrupt':True}),\
                patch.object(journey,'environment',return_value=dict(platform='test')):
            binary=Path(temp)/'erii';binary.write_bytes(b'binary');report=self.report(source)
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            changes=[lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),lambda r:r.update(release_ready=True),
                lambda r:r.update(eligible_for_grading=True),lambda r:r.update(cleanup=1),lambda r:r.update(fixture_lock={}),
                lambda r:r['identity'].update(source_after={}),lambda r:r['identity'].update(binary_sha256='0'*64),
                lambda r:r['identity'].update(environment=True),lambda r:r['results'].pop(),lambda r:r['results'].reverse(),
                lambda r:r['results'][0].update(repetition=True),lambda r:r['results'][0].update(assertions={'native':1}),
                lambda r:r['results'][0].update(cleanup={'owned_processes_joined':True}),lambda r:r.update(rejection_probes={})]
            for table in report['results'][0]['final']:
                changes += [lambda r,t=table:r['results'][5]['final'].update({t:copy.deepcopy(r['results'][0]['final'][t])}),
                    lambda r,t=table:r['results'][0]['final'][t].extend(copy.deepcopy(r['results'][0]['final'][t]))]
            for change in changes:
                bad=copy.deepcopy(report);change(bad)
                with self.assertRaises((RuntimeError,ValueError,TypeError)):journey.validate_report(bad,binary)
            # CI must call the same native oracle, never trust a manifest summary.
            gate=verify_suite.Gate('gc13','verify_gc13.py','gc13')
            path=Path(temp)/'gc13.json';journey.retain_report(path,report)
            receipt=ci.native_receipt(gate,1,Path(temp),binary,dict(binary_sha256=journey.sha(b'binary')))
            ci.check_receipts([receipt],[gate])
            copied=copy.deepcopy(report);copied['results'][5]['final']=copy.deepcopy(copied['results'][0]['final'])
            journey.retain_report(path,copied)
            with self.assertRaises(RuntimeError):ci.native_receipt(gate,1,Path(temp),binary,{})

    def test_fixed_parts_require_exact_selector_all_five_rounds_and_native_validation(self):
        source=dict(sha='a'*40,files={'runner':'hash'})
        with tempfile.TemporaryDirectory() as temp,patch.object(journey,'source_identity',return_value=source),\
                patch.object(journey,'observe',return_value={'native':True}),patch.object(journey,'rejection_probes',return_value={'corrupt':True}),\
                patch.object(journey,'environment',return_value=dict(platform='test')):
            binary=Path(temp)/'erii';binary.write_bytes(b'binary')
            full=self.report(source);all_pairs=[]
            for part in journey.GC13_VARIANTS:
                gate,=verify_suite.communication_gates('gc13-'+part)
                report=copy.deepcopy(full);report['ci_part']=part
                report['results']=[r for r in report['results'] if r['variant_id'] in journey.GC13_VARIANTS[part]]
                self.assertEqual(journey.validate_report(report,binary,part=part)['receipts'],15)
                all_pairs+=ci.pairs(gate)
                path=Path(temp)/verify_suite.report_name(gate,1);journey.retain_report(path,report)
                receipt=ci.native_receipt(gate,1,Path(temp),binary,{})
                ci.check_receipts([receipt],[gate])
                for mutate in (lambda r:r['results'].pop(),lambda r:r['results'].reverse(),lambda r:r.update(ci_part='other'),
                        lambda r:r['results'].append(r['results'][0]),lambda r:r['results'][0].update(repetition=True),
                        lambda r:r['identity'].update(source_after={}),lambda r:r.update(cleanup=False),
                        lambda r:r['results'][0].update(assertions={'native':1})):
                    bad=copy.deepcopy(report);mutate(bad)
                    with self.assertRaises(RuntimeError):journey.validate_report(bad,binary,part=part)
                with self.assertRaises(RuntimeError):journey.validate_report(report,binary)
                wrong=copy.deepcopy(full);wrong['ci_part']=part
                with self.assertRaises(RuntimeError):journey.validate_report(wrong,binary,part=part)
            self.assertEqual(all_pairs,journey.pairs())

    def test_manifest_rejects_missing_tampered_escaped_and_graded_native_bytes(self):
        report=dict(status='mechanical_pass',cleanup=True,results=[{'raw':'RAW'*1000}],identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/'gc13.json';manifest=journey.retain_report(path,report)
            self.assertEqual(journey.read_report(path),report)
            self.assertLess(path.stat().st_size,2048)
            for mutate in (lambda m:m['summary'].update(semantic_score=100),lambda m:m.update(native_bytes=0),
                    lambda m:m['native'].update(path='../escape'),lambda m:m['native'].update(sha256='0'*64),
                    lambda m:m.update(native_uncompressed_bytes=1),lambda m:m['summary'].update(cleanup=1)):
                bad=copy.deepcopy(manifest);mutate(bad);path.write_text(json.dumps(bad))
                with self.assertRaises((RuntimeError,ValueError,FileNotFoundError)):journey.read_report(path)
            path.write_text(json.dumps(manifest));raw=path.parent/manifest['native']['path'];raw.write_bytes(b'broken')
            with self.assertRaises((RuntimeError,ValueError)):journey.read_report(path)


if __name__=='__main__':unittest.main()
