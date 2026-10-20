import copy
import json
from pathlib import Path
import struct
import tempfile
import unittest

from case_runner import load_contract, variants
from m6_report import aggregate, CASES, VARIANTS
from m6_sources import validate

ROOT=Path(__file__).resolve().parents[1]


class M6Cases(unittest.TestCase):
    def test_external_write_guard_uses_frozen_capabilities_and_fails_unknown(self):
        from run_m6_cases import no_external_writes
        value=dict(capabilities=[dict(snapshot_id='cap',tools_json=json.dumps([dict(name='read',permission='read_only'),dict(name='send',permission='external_effect')]))],
                   runs=[dict(run_id='run',capability_snapshot_id='cap')],tools=[dict(run_id='run',tool_name='read')])
        self.assertTrue(no_external_writes(value))
        for tool in ('send','unadvertised'):
            bad=copy.deepcopy(value);bad['tools'][0]['tool_name']=tool
            self.assertFalse(no_external_writes(bad))
        bad=copy.deepcopy(value);bad['tools'][0]['run_id']='unknown'
        self.assertFalse(no_external_writes(bad))

    def test_locked_exact_inputs_and_wording_noise_variants(self):
        data,_,lock=load_contract(ROOT,'m6-v1')
        self.assertEqual(tuple(c['case_id'] for c in data['cases']),CASES)
        for case in data['cases']:
            generated=list(variants(data,case))
            self.assertEqual(tuple(v['variant_id'] for v in generated),VARIANTS)
            self.assertEqual(len({v['fixture_digest'] for v in generated}),6)
            self.assertEqual(sum(v['noise_characters']>0 for v in generated),2)
        worlds={c['case_id']:c['world'] for c in data['cases']}
        self.assertEqual(worlds['GC-03']['employer_options']['vested_remaining'],200)
        self.assertEqual(worlds['GC-03']['fx'],dict(usd_cny=6.73,as_of='2026-09-30'))
        self.assertEqual(worlds['GC-13']['restaurants'][0]['name'],'松间·湖滨店')
        self.assertFalse(worlds['GC-15']['flight_order']['inbound_year_visible'])
        self.assertIn('evals/fixtures/m6-v1/media/third.mov',lock['files'])

    def test_masked_year_region_hash_and_dimensions_fail_closed(self):
        folder=ROOT/'evals/fixtures/m6-v1'
        manifest=json.loads((folder/'regions.json').read_text())
        for name,document in manifest.items():
            raw=(folder/name).read_bytes()
            size=list(struct.unpack('>II',raw[16:24]))
            descriptor=dict(content_hash=document['sha256'],artifact_ref='artifact:fixture')
            self.assertEqual(validate(document,descriptor,size)['artifactRef'],'artifact:fixture')
            with self.assertRaises(ValueError):validate(document,dict(descriptor,content_hash='bad'),size)
            with self.assertRaises(ValueError):validate(document,descriptor,[1,1])
        original=manifest['flight_order.png']
        for value in (2026,'2026','unknown'):
            changed=copy.deepcopy(original);changed['fields']['inbound_year']['value']=value
            with self.assertRaises(ValueError):validate(changed,dict(content_hash=changed['sha256'],artifact_ref='artifact:fixture'),[900,360])
        for region in ([-1,0,20,20],[0,0,901,20],[0,0,0,2],[0,0,float('nan'),2]):
            changed=copy.deepcopy(original);changed['fields']['inbound_year']['region']=region
            with self.assertRaises(ValueError):validate(changed,dict(content_hash=changed['sha256'],artifact_ref='artifact:fixture'),[900,360])

    def reports(self):
        rows=[dict(case_id=c,variant_id=v,fixture_digest=c+v,assertions=dict(observed=True),hard_failure=False,status='fragment_pass',final_score=None) for c in CASES for v in VARIANTS]
        return [copy.deepcopy(dict(binary_sha256='binary',dataset_digest='fixture',driver_digest='driver',contract_lock={},cleanup=True,results=rows,live_shadow={})) for _ in range(5)]

    def test_missing_failed_repetition_or_invented_score_cannot_pass(self):
        good=self.reports()
        summary=aggregate(good)
        self.assertTrue(summary['passed'])
        self.assertIsNone(summary['suite_mean'])
        for change in ('missing','binary','driver','score','cleanup','variant','digest'):
            bad=copy.deepcopy(good)
            if change=='missing':bad.pop()
            elif change=='binary':bad[1]['binary_sha256']='other'
            elif change=='driver':bad[1]['driver_digest']='other'
            elif change=='score':bad[0]['results'][0]['final_score']=100
            elif change=='cleanup':bad[0]['cleanup']=False
            elif change=='variant':bad[0]['results'][0]['variant_id']='wrong'
            else:bad[0]['results'][0]['fixture_digest']='drift'
            with self.assertRaises(ValueError):aggregate(bad)
        for change in ('assertion','status','hard_failure'):
            bad=copy.deepcopy(good)
            row=bad[2]['results'][5]
            if change=='assertion':row['assertions']['observed']=False
            elif change=='status':row['status']='fragment_fail'
            else:row['hard_failure']=True
            self.assertFalse(aggregate(bad)['passed'])

    def test_artifact_export_refuses_symlink_before_read_or_write(self):
        from run_m6_cases import export_artifacts
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp).resolve()
            owned=root/'owned';owned.mkdir();(owned/'.m6-evidence.json').write_text('{}')
            other=root/'other';other.mkdir();(other/'sentinel').write_text('user data')
            (owned/'GC-15').symlink_to(other,target_is_directory=True)
            with self.assertRaises(RuntimeError):export_artifacts(None,owned/'GC-15'/'wording-1')
            self.assertEqual(list(other.iterdir()),[other/'sentinel'])
            self.assertEqual((other/'sentinel').read_text(),'user data')
