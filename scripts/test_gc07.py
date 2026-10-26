"""Frozen GC-07 inputs and fail-closed receipt boundaries."""
import copy
import gzip
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

from case_runner import variants
import unscripted_cases
import verify_gc07 as journey
import verify_suite


class NetworkTests(unittest.TestCase):
    def test_twenty_original_blocks_six_variants_five_repeats_and_real_model_inputs(self):
        lock = journey.load()
        self.assertEqual(lock['manifest_sha256'],'0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556')
        self.assertEqual([b['line'] for b in lock['frozen_blocks']],
            [243,245,247,248,249,250,251,253,694,696,710,712,713,714,716,718,719,720,721,722])
        data,case = journey.inputs();rows = list(variants(data,case))
        self.assertEqual(len(rows),6)
        self.assertEqual(len({r['fixture_digest'] for r in rows}),6)
        self.assertEqual(sum(r['noise_characters']>0 for r in rows),2)
        self.assertEqual(journey.REPEATS,5)
        for row in rows:
            live = unscripted_cases.inputs('GC-07',row['variant_id'])
            self.assertEqual(live['context'],row['context'])
            self.assertEqual(live['events'],[dict(at=e['at'],text=e['text']) for e in row['events']])
            self.assertNotIn('Capabilities:',json.dumps(live['events']))
        self.assertIs(unscripted_cases.matrix()['GC-07']['eligible_for_grading'],False)
        self.assertEqual([(s,g.attempts) for s,gs in verify_suite.SHARDS.items() for g in gs if g.script=='verify_gc07.py'],[('delivery',1)])
        self.assertIn("browser: ${{ matrix.shard == 'delivery' || startsWith(matrix.communicationpart, 'gc13-') }}",(journey.ROOT/'.github/workflows/ci.yml').read_text())

    def test_report_reads_actual_scoped_dom_and_never_injects_expected_state(self):
        source = journey.report_source('http://127.0.0.1:12345')
        self.assertIn('await pw.locator("#topology").textContent()',source)
        self.assertIn('await pw.locator("#network").textContent()',source)
        self.assertNotIn('gateway:true',source)
        self.assertNotIn('192.168.0.12',source)
        self.assertIn('provider_acknowledged:false',source)
        self.assertIn('live_verified:false',source)
        self.assertIn('every(k=>network[k]===true)',source)
        self.assertEqual(journey.topology()['world']['network']['dhcp_gateway'],'192.168.0.12')
        self.assertEqual(journey.topology(False)['world']['network']['dhcp_gateway'],'192.168.0.1')

    def test_persisted_return_requires_exactly_one_strict_value_and_matches_native_tool(self):
        output = [dict(type='return',value=dict(restored=False))]
        parent = dict(invocation_id='cell',run_id='main',tool_name='builtin__codemode__execute',arguments_json='{}',request_hash='frozen',
            status='succeeded',attempt_count=1,result_json=json.dumps(dict(text=json.dumps(output))))
        call = dict(parent=parent,nested=[],output=output,value=output[0]['value'])
        final = dict(tools=[copy.deepcopy(parent)])
        self.assertEqual(journey.returned(call,final),dict(restored=False))
        for mutate in (lambda c:c['value'].update(restored=True),lambda c:c['output'].append(dict(type='text',text='forged')),
                lambda c:c['parent'].update(result_json=json.dumps(dict(text=json.dumps(output*2)))),
                lambda c:c['parent'].update(arguments_json='{"different":true}')):
            changed = copy.deepcopy(call);mutate(changed)
            with self.assertRaises(RuntimeError):journey.returned(changed,final)
        for invalid in ('[{"type":"return","value":{},"value":null}]','[{"type":"return","value":NaN}]'):
            changed=copy.deepcopy(call);changed['parent']['result_json']=json.dumps(dict(text=invalid))
            with self.assertRaises(ValueError):journey.returned(changed,final)

    def test_all_thirty_receipts_identity_cleanup_and_false_grades_are_required(self):
        source=dict(sha='a'*40,files={'runner':'hash'})
        report=dict(case_id='GC-07',status='mechanical_pass',release_ready=False,semantic_score=None,eligible_for_grading=False,
            fixture_lock=journey.load(),m6_lock=journey.load_contract(journey.ROOT,'m6-v1')[2],cleanup=True,
            identity=dict(source_before=source,source_after=source,binary_sha256=journey.sha(b'binary'),environment={'python':'test'}),
            rejection_probes={'bad':True},results=[dict(variant_id=v['variant_id'],repetition=n,assertions={'observed':True},
                cleanup=dict(disposable_root_removed=True,owned_processes_joined=True)) for v in variants(*journey.inputs()) for n in range(1,6)])
        with tempfile.TemporaryDirectory() as tmp, patch.object(journey,'source_identity',return_value=source), \
                patch.object(journey,'observe',return_value={'observed':True}),patch.object(journey,'rejection_probes',return_value={'bad':True}), \
                patch.object(journey,'environment',return_value={'python':'test'}):
            binary=Path(tmp)/'erii';binary.write_bytes(b'binary')
            self.assertEqual(journey.validate_report(report,binary)['receipts'],30)
            mutations=[lambda r:r.update(release_ready=True),lambda r:r.update(semantic_score=100),lambda r:r.pop('semantic_score'),
                lambda r:r.update(eligible_for_grading=True),lambda r:r.update(cleanup=False),lambda r:r.update(fixture_lock={}),
                lambda r:r.update(m6_lock={}),lambda r:r['identity'].update(source_after={}),
                lambda r:r['identity'].update(binary_sha256='0'*64),lambda r:r['identity'].update(environment={}),
                lambda r:r['results'].pop(),lambda r:r['results'].append(r['results'][0]),lambda r:r['results'].reverse(),
                lambda r:r['results'][0].update(repetition=True),lambda r:r['results'][0].update(cleanup={}),
                lambda r:r['results'][0].update(assertions={'observed':1}),lambda r:r.update(rejection_probes={})]
            for mutate in mutations:
                changed=copy.deepcopy(report);mutate(changed)
                with self.assertRaises(RuntimeError):journey.validate_report(changed,binary)
            # Both CI parts use the real native validator, with identical oracles.
            # Neither a relabelled full report nor a misplaced/short series passes.
            for part in verify_suite.GC07_VARIANTS:
                selected=copy.deepcopy(report)
                selected['ci_part']=part
                selected['results']=[r for r in selected['results']
                    if r['variant_id'] in verify_suite.GC07_VARIANTS[part]]
                self.assertEqual(journey.validate_report(selected,binary,part=part)['receipts'],15)
                with self.assertRaises(RuntimeError):journey.validate_report(selected,binary)
                other='noise' if part=='wording' else 'wording'
                with self.assertRaises(RuntimeError):journey.validate_report(selected,binary,part=other)
                for mutate in (lambda r:r['results'].pop(),lambda r:r['results'].reverse(),
                        lambda r:r['results'].append(r['results'][0]),
                        lambda r:r['results'][0].update(variant_id='foreign'),
                        lambda r:r['results'][0].update(cleanup={}),
                        lambda r:r['identity'].update(binary_sha256='0'*64),
                        lambda r:r.update(rejection_probes={}),lambda r:r.update(semantic_score=100)):
                    changed=copy.deepcopy(selected);mutate(changed)
                    with self.assertRaises(RuntimeError):journey.validate_report(changed,binary,part=part)
                relabelled=copy.deepcopy(report);relabelled['ci_part']=part
                with self.assertRaises(RuntimeError):journey.validate_report(relabelled,binary,part=part)
            with self.assertRaises(RuntimeError):journey.validate_report(report,binary,part='control')
            with patch.object(journey,'observe',return_value={'observed':False}):
                with self.assertRaises(RuntimeError):journey.validate_report(report,binary)

    def test_native_manifest_hash_size_summary_and_safe_paths(self):
        report=dict(status='mechanical_pass',cleanup=True,results=[dict(payload='NATIVE_SENTINEL'*100000)],
            identity=dict(source_before=dict(sha='a'*40),binary_sha256='b'*64))
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'gc07.json';manifest=journey.retain_report(path,report)
            self.assertLess(path.stat().st_size,2048)
            self.assertNotIn('NATIVE_SENTINEL',path.read_text())
            raw=path.parent/manifest['native']['path']
            self.assertEqual(raw.parent.name,'.gc07-native')
            self.assertEqual(journey.read_report(path),report)
            for mutate in (lambda m:m['summary'].update(semantic_score=100),lambda m:m['summary'].update(cleanup=1),
                    lambda m:m['native'].update(sha256='0'*64),lambda m:m['native'].update(path='../outside.json'),
                    lambda m:m['native'].update(path=str(raw)),lambda m:m.update(native_bytes=True),lambda m:m.update(version=True),
                    lambda m:m.update(native_encoding='plain'),lambda m:m.update(native_uncompressed_bytes=True),
                    lambda m:m.update(native_uncompressed_bytes=0),lambda m:m.update(native_uncompressed_bytes=3<<30),
                    lambda m:m.update(native_uncompressed_bytes=m['native_uncompressed_bytes']+1)):
                changed=copy.deepcopy(manifest);mutate(changed);path.write_text(json.dumps(changed))
                with self.assertRaises((ValueError,RuntimeError)):journey.read_report(path)
            path.write_text(json.dumps(manifest))
            actual=raw.with_suffix('.saved');raw.rename(actual);raw.symlink_to(actual)
            with self.assertRaises(ValueError):journey.read_report(path)
            raw.unlink();actual.rename(raw)
            self.assertGreater(manifest['native_uncompressed_bytes'],100*manifest['native_bytes'])
            self.assertEqual(json.loads(gzip.decompress(raw.read_bytes())),report)
            encoded=raw.read_bytes()
            raw.write_bytes(encoded+b' ')
            with self.assertRaises(ValueError):journey.read_report(path)
            # Even a matching attacker-supplied compressed hash cannot turn
            # a truncated or non-gzip payload into a native receipt.
            for bad in (encoded[:-5],b'not a gzip stream'):
                raw.write_bytes(bad)
                changed=copy.deepcopy(manifest)
                changed['native']['sha256']=journey.sha(bad)
                changed['native_bytes']=len(bad)
                path.write_text(json.dumps(changed))
                with self.assertRaises((OSError,EOFError,ValueError)):journey.read_report(path)
            self.assertNotIn('include-hidden-files: true',(journey.ROOT/'.github/workflows/ci.yml').read_text())

    def test_failure_summary_retains_structural_state_without_raw_requests_or_results(self):
        row=dict(variant_id='wording-1',repetition=1,diagnosis={},failure_state=dict(
            runs=[dict(run_id='root',status='running')],claims=[],code=[dict(state='suspended')],computer=[],computer_operations=[],
            models=[dict(generation_id='generation',status='accepted',request_json='PRIVATE_REQUEST_SENTINEL'*10000)],
            tools=[dict(invocation_id='call',tool_name='computer',status='waiting',result_json='PRIVATE_RESULT_SENTINEL'*10000)]))
        summary=journey.failure_summary(row,RuntimeError('run storage unavailable'))
        self.assertEqual(summary['runs'],row['failure_state']['runs'])
        self.assertEqual(summary['completed_stages'],['diagnosis'])
        self.assertEqual(summary['models'],[dict(generation_id='generation',status='accepted')])
        self.assertEqual(summary['tools'],[dict(invocation_id='call',tool_name='computer',status='waiting')])
        self.assertLess(len(json.dumps(summary)),32768)
        self.assertNotIn('PRIVATE_',json.dumps(summary))

    def test_failed_cli_removes_old_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'gc07.json';path.write_text('{"status":"mechanical_pass"}')
            result=subprocess.run([sys.executable,str(journey.ROOT/'scripts/verify_gc07.py'),'--binary',str(Path(tmp)/'missing'),
                '--evidence',str(path)],capture_output=True,text=True)
            self.assertNotEqual(result.returncode,0)
            self.assertFalse(path.exists())


if __name__=='__main__':unittest.main()
