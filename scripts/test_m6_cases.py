import copy
import json
from pathlib import Path
import struct
import tempfile
import subprocess
import sys
import unittest
from unittest.mock import patch
import time
import threading

from case_runner import load_contract, variants
from m6_report import aggregate, CASES, VARIANTS, GC02_SCHEDULE_ASSERTIONS
from m6_sources import validate

ROOT=Path(__file__).resolve().parents[1]


class M6Cases(unittest.TestCase):
    def test_monthly_review_judge_rejects_wrong_schedule_duplicates_and_early_work(self):
        from run_m6_cases import goal_schedule_checks
        before=dict(automation=[],automation_fire=[],task=[],manage_calls=[],generations=5,run_kinds={})
        after=dict(automation=[dict(automation_id='automation:one',next_fire_at=1795953600000000)],automation_fire=[],task=[],
                   manage_calls=[dict(run_id='main',status='succeeded')],generations=7,run_kinds=dict(main='main'))
        views=[dict(automationRef='automation:one',enabled=True,triggerType='cron',triggerSpec='0 20 * * 0L',
                    timezone='Asia/Shanghai',nextFire='2026-11-29T12:00:00Z')]
        self.assertTrue(all(goal_schedule_checks(before,after,copy.deepcopy(after),views).values()))
        for field,value in (('nextFire','2026-11-22T12:00:00Z'),('timezone','UTC'),
                            ('triggerSpec','0 20 * * 0'),('triggerType','at'),('enabled',False),
                            ('automationRef','automation:other')):
            bad=copy.deepcopy(views);bad[0][field]=value
            with self.subTest(field=field):
                self.assertFalse(all(goal_schedule_checks(before,after,after,bad).values()))
        for field,value in (('task',[dict(task_id='unsolicited')]),('automation_fire',[dict(event_id='early')]),
                            ('manage_calls',[]),('run_kinds',dict(main='worker')),
                            ('automation',[dict(automation_id='one'),dict(automation_id='two')])):
            bad=copy.deepcopy(after);bad[field]=value
            with self.subTest(field=field):
                self.assertFalse(all(goal_schedule_checks(before,bad,bad,views).values()))
        replayed=copy.deepcopy(after);replayed['generations']+=1
        self.assertFalse(all(goal_schedule_checks(before,after,replayed,views).values()))
        wrong_db=copy.deepcopy(after);wrong_db['automation'][0]['next_fire_at']+=1000000
        self.assertFalse(all(goal_schedule_checks(before,wrong_db,wrong_db,views).values()))
        early=copy.deepcopy(before);early['automation']=after['automation']
        self.assertFalse(all(goal_schedule_checks(early,after,after,views).values()))

    def test_browser_cleanup_preserves_primary_failure_and_still_rejects_bad_exit(self):
        from m6_desktop import browser
        popen=subprocess.Popen
        children=[]
        def start(*args,**kwargs):
            child=popen([sys.executable,'-c','print(\'{"endpoint":"fixture"}\',flush=True);raise SystemExit(7)'],**kwargs)
            children.append(child)
            return child
        for primary in (False,True):
            with self.subTest(primary=primary), tempfile.TemporaryDirectory() as temporary:
                with patch('m6_desktop.subprocess.Popen',side_effect=start):
                    with self.assertRaisesRegex(RuntimeError,'original drive failure' if primary else 'browser cleanup failed') as caught:
                        with browser(Path(temporary),{}):
                            if primary:raise RuntimeError('original drive failure')
                if primary:self.assertIn('browser cleanup failed',str(caught.exception.__cause__))
                self.assertIsNotNone(children[-1].poll())
                self.assertTrue(children[-1].stdout.closed)
                self.assertTrue(children[-1].stderr.closed)

    def test_parallel_m6_retains_complete_ordered_evidence_and_isolated_roots(self):
        from run_m6_cases import run
        active, peak = 0, 0
        lock = threading.Lock()
        roots = set()
        running_cases = []
        class Peer:
            def close(self):pass
        def action(binary, folder, case, variant, *args):
            nonlocal active, peak
            with lock:
                self.assertNotIn(folder, roots)
                roots.add(folder)
                active += 1
                running_cases.append(case['case_id'])
                if case['case_id']=='GC-04':
                    self.assertLessEqual(active,2)
                    self.assertEqual(set(running_cases),{'GC-04'})
                else:
                    self.assertNotIn('GC-04',running_cases)
                peak = max(peak, active)
            (folder/'state').write_text(variant['variant_id'])
            time.sleep(.01)
            self.assertEqual((folder/'state').read_text(), variant['variant_id'])
            with lock:
                active -= 1
                running_cases.remove(case['case_id'])
            return dict(observed=True), dict(root=str(folder))
        with tempfile.TemporaryDirectory() as temporary:
            output=Path(temporary)/'report.json'
            with patch('run_m6_cases.Embeddings', Peer), patch('run_m6_cases.personal_case', action), \
                 patch('run_m6_cases.health_safety', return_value=({'safety':True},{})), \
                 patch('m6_desktop.drive', action):
                report=run(Path(__file__),destination=output,scenario_jobs=4)
            self.assertEqual(peak,4)
            self.assertEqual([(r['case_id'],r['variant_id']) for r in report['results']],
                             [(c,v) for c in CASES for v in VARIANTS])
            self.assertTrue(report['cleanup'])
            self.assertEqual(json.loads(output.read_text()),report)
            self.assertTrue(all(not p.exists() for p in roots))
            self.assertEqual(len(json.loads(output.with_suffix('.progress.json').read_text())),66)

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
        for row in rows:
            if row['case_id']=='GC-02':
                row['assertions'].update(dict.fromkeys(GC02_SCHEDULE_ASSERTIONS,True))
        return [copy.deepcopy(dict(binary_sha256='binary',dataset_digest='fixture',driver_digest='driver',contract_lock={},cleanup=True,results=rows,live_shadow={})) for _ in range(5)]

    def test_same_omitted_monthly_assertion_in_all_rounds_cannot_pass(self):
        for assertion in GC02_SCHEDULE_ASSERTIONS:
            reports=self.reports()
            for report in reports:
                for row in report['results']:
                    if row['case_id']=='GC-02':
                        del row['assertions'][assertion]
            with self.subTest(assertion=assertion):
                self.assertFalse(aggregate(reports)['passed'])

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
