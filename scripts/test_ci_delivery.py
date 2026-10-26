"""Synthetic GC-07 tests exercise CI mechanics, never the consumer's journey."""
import copy
import hashlib
import json
import os
from pathlib import Path
import signal
import sys
import tempfile
import threading
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import ci_delivery as delivery
import verify_suite as suite


class DeliveryTests(unittest.TestCase):
    def fixture(self, root):
        source = dict(source_sha='a'*40, source_dirty=False,
                      source_diff_sha256=hashlib.sha256(b'').hexdigest())
        suites, manifests = {}, {}
        for part in suite.DELIVERY_PARTS:
            folder = root / part
            folder.mkdir()
            gates = suite.delivery_gates(part)
            suites[part] = dict(**source, passed=True, source_stable=True,
                binary_sha256='b'*64, workflow_sha=None, run_id=None, run_attempt=None,
                delivery_part=part, complete_inventory=False, selected_gates=None,
                jobs=1, gate_jobs=2, elapsed_seconds=1,
                shards={'delivery': [dict(gate=g.name, attempt=n, returncode=0,
                    owned_group_drained=True, command=suite.command(g, n, Path('/binary'), folder)[0])
                    for g in gates for n in range(1, g.attempts+1)]})
            receipts = []
            for gate in gates:
                for n in range(1, gate.attempts+1):
                    receipt = dict(gate=gate.name, attempt=n,
                        report=dict(path=suite.report_name(gate,n),bytes=10,sha256='c'*64))
                    if part != 'control':
                        receipt.update(status='mechanical_pass', cleanup=True, semantic_score=None,
                            eligible_for_grading=False, release_ready=False,
                            contract=dict(fixture_lock='synthetic',m6_lock='synthetic'),
                            rejection_probes=dict(synthetic_forgery=True),
                            native=dict(path='synthetic.json.gz',bytes=20,sha256='d'*64),
                            rows=[dict(variant_id=v,repetition=i,
                                assertions={f'synthetic-{a}':True for a in range(44)},
                                cleanup=dict(disposable_root_removed=True,owned_processes_joined=True))
                                for v,i in suite.gc07_pairs(part)])
                    receipts.append(receipt)
            manifests[part] = dict(version=1,part=part,fixtures={},receipts=receipts,
                identity={k:suites[part][k] for k in delivery.DELIVERY_IDENTITY})
        return source, suites, manifests

    def write(self, root, suites, manifests):
        for part in suite.DELIVERY_PARTS:
            path=root/part/f'suite-delivery-{part}.json'
            path.write_text(json.dumps(suites[part]))
            manifests[part]['suite']=delivery.file_record(path)
            (root/part/'delivery-receipts.json').write_text(json.dumps(manifests[part]))

    def consumer(self):
        return patch.dict(suite.SHARDS, delivery=(*suite.SHARDS['delivery'],
                                                suite.Gate('gc07','verify_gc07.py','gc07')))

    def validate(self, source, suites, manifests):
        return delivery.validate(suites,manifests,source,
                                 dict(workflow_sha=None,run_id=None,run_attempt=None),{})

    def test_fixed_selectors_preserve_whole_variant_series(self):
        self.assertEqual(suite.GC07_VARIANTS,dict(wording=('wording-1','wording-2','wording-3'),
                                                noise=('noise-1','noise-2','order-1')))
        self.assertEqual(suite.delivery_gates('control'),suite.SHARDS['delivery'])
        self.assertEqual(suite.delivery_gates('wording'),())
        self.assertEqual(suite.delivery_gates('noise'),())
        with self.consumer():
            for part in suite.GC07_VARIANTS:
                gate,=suite.delivery_gates(part)
                self.assertEqual(gate.arguments,('--ci-part',part))
                self.assertEqual(gate.attempts,1)  # Five rounds live inside each variant series.
                self.assertEqual(suite.gc07_pairs(part),[(v,n) for v in suite.GC07_VARIANTS[part] for n in range(1,6)])
        for part in ('unknown','gc07'):
            with self.assertRaises(ValueError): suite.delivery_gates(part)
        for gate in (suite.Gate('extra','','extra'),suite.Gate('gc07','other.py','gc07')):
            with patch.dict(suite.SHARDS,delivery=(*suite.SHARDS['delivery'],gate)):
                with self.assertRaises(ValueError): suite.delivery_gates('wording')

    def test_synthetic_part_entry_executes_each_five_round_series_on_one_worker(self):
        # Real subprocess and maintained fixture pool, no product GC-07 source.
        for part in suite.GC07_VARIANTS:
            with self.subTest(part=part), tempfile.TemporaryDirectory() as temp:
                root=Path(temp)
                output=root/'rounds.json'
                program=("import json,sys,threading\n"
                    f"sys.path.insert(0,{str(suite.ROOT/'scripts')!r})\n"
                    "from fixture_pool import run_cases\n"
                    "from verify_suite import GC07_VARIANTS\n"
                    "from pathlib import Path\n"
                    "part=sys.argv[1];rows=[];lock=threading.Lock()\n"
                    "def series(variant):\n"
                    " for n in range(1,6):\n"
                    "  with lock: rows.append([variant,n,threading.get_ident()])\n"
                    "run_cases(GC07_VARIANTS[part],series,4)\n"
                    "Path(sys.argv[2]).write_text(json.dumps(rows))\n")
                metrics={}
                self.assertEqual(suite.run_command([sys.executable,'-c',program,part,str(output)],
                    os.environ.copy(),root/'log',threading.Event(),metrics),0)
                self.assertTrue(metrics['owned_group_drained'])
                rows=json.loads(output.read_text())
                self.assertEqual(len(rows),15)
                self.assertEqual({r[0] for r in rows},set(suite.GC07_VARIANTS[part]))
                for variant in suite.GC07_VARIANTS[part]:
                    series=[r for r in rows if r[0]==variant]
                    self.assertEqual([r[1] for r in series],list(range(1,6)))
                    self.assertEqual(len({r[2] for r in series}),1)

    def test_current_main_and_synthetic_consumer_join(self):
        for consumer in (False,True):
            with self.subTest(consumer=consumer), tempfile.TemporaryDirectory() as temp:
                root=Path(temp)
                with self.consumer() if consumer else patch.dict(suite.SHARDS,{}):
                    source,suites,manifests=self.fixture(root)
                    self.write(root,suites,manifests)
                    with patch.object(delivery,'source_state',return_value=source), \
                         patch.object(delivery,'fixtures',return_value={}), patch.dict(os.environ,{},clear=True):
                        full=delivery.join(root)
                    self.assertEqual([(r['gate'],r['attempt']) for r in full['shards']['delivery']],
                        [(g.name,n) for g in suite.SHARDS['delivery'] for n in range(1,g.attempts+1)])
                    self.assertEqual(len(full['gc07']['receipts']),30 if consumer else 0)
                    self.assertEqual(full['gc07']['status'],'mechanical_pass' if consumer else 'not_registered')
                    self.assertTrue(full['complete_inventory'])
                    self.assertFalse(full['release_ready'])
                    self.assertIsNone(full['semantic_score'])
                    if consumer:
                        self.assertEqual([(r['variant_id'],r['repetition']) for r in full['gc07']['receipts']],
                            [pair for p in suite.GC07_VARIANTS for pair in suite.gc07_pairs(p)])

    def test_invalid_receipts_fail_closed(self):
        mutations=('missing_part','extra_part','missing_round','duplicate','reorder','failed','cancelled',
            'unclean','failed_suite','dirty','unstable','focused','missing_focus','complete','other_part',
            'wrong_part','wrong_shard','jobs','lanes','bool_attempt','bool_code','command','argument',
            'manifest_identity','fixtures','receipt_missing','receipt_duplicate','receipt_reorder','report_hash',
            'report_name','native_hash','variant_missing','variant_duplicate','variant_reorder','variant_part',
            'repeat_bool','assertion','assertion_type','assertion_inventory','contract','probes','cleanup',
            'cleanup_type','status','graded','release','eligible',*delivery.DELIVERY_IDENTITY)
        with self.consumer():
            for change in mutations:
                with self.subTest(change=change),tempfile.TemporaryDirectory() as temp:
                    source,suites,manifests=self.fixture(Path(temp))
                    fragment=suites['wording'];rows=fragment['shards']['delivery']
                    manifest=manifests['wording'];receipt=manifest['receipts'][0];case=receipt['rows']
                    if change=='missing_part': suites.pop('noise')
                    elif change=='extra_part': suites['extra']=fragment
                    elif change=='missing_round': rows.clear()
                    elif change=='duplicate': rows.append(copy.deepcopy(rows[0]))
                    elif change=='reorder': suites['control']['shards']['delivery'].reverse()
                    elif change=='failed': rows[0]['returncode']=7
                    elif change=='cancelled': rows[0]['returncode']=130
                    elif change=='unclean': rows[0]['owned_group_drained']=False
                    elif change=='failed_suite': fragment['passed']=False
                    elif change=='dirty': fragment['source_dirty']=True
                    elif change=='unstable': fragment['source_stable']=False
                    elif change=='focused': fragment['selected_gates']=['gc07']
                    elif change=='missing_focus': fragment.pop('selected_gates')
                    elif change=='complete': fragment['complete_inventory']=True
                    elif change=='other_part': fragment['security_part']='finance'
                    elif change=='wrong_part': fragment['delivery_part']='noise'
                    elif change=='wrong_shard': fragment['shards']={'runtime':rows}
                    elif change=='jobs': fragment['jobs']=2
                    elif change=='lanes': fragment['gate_jobs']=4
                    elif change=='bool_attempt': rows[0]['attempt']=True
                    elif change=='bool_code': rows[0]['returncode']=False
                    elif change=='command': rows[0]['command'][2]='scripts/other.py'
                    elif change=='argument': rows[0]['command'].append('--feedback')
                    elif change=='manifest_identity': manifest['identity']['run_id']='old'
                    elif change=='fixtures': manifest['fixtures']={'changed':'x'}
                    elif change=='receipt_missing': manifests['control']['receipts'].pop()
                    elif change=='receipt_duplicate': manifest['receipts'].append(copy.deepcopy(receipt))
                    elif change=='receipt_reorder': manifests['control']['receipts'].reverse()
                    elif change=='report_hash': receipt['report']['sha256']=''
                    elif change=='report_name': receipt['report']['path']='other.json'
                    elif change=='native_hash': receipt['native']['sha256']=''
                    elif change=='variant_missing': case.pop()
                    elif change=='variant_duplicate': case[-1]=copy.deepcopy(case[0])
                    elif change=='variant_reorder': case.reverse()
                    elif change=='variant_part': case[0]['variant_id']='noise-1'
                    elif change=='repeat_bool': case[0]['repetition']=True
                    elif change=='assertion': case[0]['assertions']['synthetic-0']=False
                    elif change=='assertion_type': case[0]['assertions']['synthetic-0']=1
                    elif change=='assertion_inventory': case[0]['assertions'].pop('synthetic-0')
                    elif change=='contract': receipt['contract']['m6_lock']='drift'
                    elif change=='probes': receipt['rejection_probes']['synthetic_forgery']=False
                    elif change=='cleanup': case[0]['cleanup']['owned_processes_joined']=False
                    elif change=='cleanup_type': case[0]['cleanup']['owned_processes_joined']=1
                    elif change=='status': receipt['status']='inconclusive'
                    elif change=='graded': receipt['semantic_score']=100
                    elif change=='release': receipt['release_ready']=True
                    elif change=='eligible': receipt['eligible_for_grading']=True
                    else: fragment[change]='drift'
                    with self.assertRaises(ValueError): self.validate(source,suites,manifests)

    def test_authoritative_run_attempt_and_suite_binding(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);source,suites,manifests=self.fixture(root)
            for field,value in [('source_sha','e'*40),('workflow_sha','e'*40),('run_id','2'),('run_attempt','2')]:
                with self.subTest(field=field),self.assertRaises(ValueError):
                    expected_source=dict(source)
                    expected_run=dict(workflow_sha=None,run_id=None,run_attempt=None)
                    (expected_source if field=='source_sha' else expected_run)[field]=value
                    delivery.validate(suites,manifests,expected_source,expected_run,{})
            self.write(root,suites,manifests)
            path=root/'control/suite-delivery-control.json';path.write_text(path.read_text()+' ')
            with self.assertRaises(ValueError): delivery.join(root)

    def test_collector_recomputes_native_bytes_before_compacting(self):
        with self.consumer(), tempfile.TemporaryDirectory() as temp:
            root=Path(temp);source,suites,manifests=self.fixture(root)
            binary=root/'erii';binary.write_bytes(b'synthetic binary')
            folder=root/'wording';gate,=suite.delivery_gates('wording')
            suites['wording']['binary_sha256']=delivery.file_record(binary)['sha256']
            path=folder/suite.report_name(gate,1)
            native=folder/'synthetic.json.gz';native.write_bytes(b'synthetic native bytes')
            path.write_text(json.dumps(dict(native=dict(path=native.name))))
            (folder/'suite-delivery-wording.json').write_text(json.dumps(suites['wording']))
            receipt=manifests['wording']['receipts'][0]
            report={k:receipt[k] for k in ('status','cleanup','release_ready','eligible_for_grading','semantic_score','rejection_probes')}
            report.update(fixture_lock='synthetic',m6_lock='synthetic',results=receipt['rows'])
            calls=[]
            def validate(native_report, executable, *, part):
                calls.append((executable,part))
                if native.read_bytes()!=b'synthetic native bytes': raise ValueError('synthetic native corruption')
            driver=SimpleNamespace(read_report=lambda p:copy.deepcopy(report),validate_report=validate)
            with patch.dict(sys.modules,verify_gc07=driver),patch.object(delivery,'source_state',return_value=source), \
                 patch.object(delivery,'fixtures',return_value={}),patch.dict(os.environ,{},clear=True):
                collected=delivery.collect(folder,'wording',binary)
                self.assertEqual(calls,[(binary,'wording')])
                self.assertEqual(len(collected['receipts'][0]['rows']),15)
                native.write_bytes(b'corrupt')
                with self.assertRaises(ValueError): delivery.collect(folder,'wording',binary)

    def test_part_cli_preserves_canonical_and_other_parts(self):
        for part in suite.DELIVERY_PARTS:
            with tempfile.TemporaryDirectory() as temp:
                root=Path(temp);(root/'suite-delivery.json').write_text('canonical sentinel')
                (root/'suite-delivery-other.json').write_text('other sentinel')
                handlers={s:signal.getsignal(s) for s in (signal.SIGINT,signal.SIGTERM)}
                def command(gate,n,binary,evidence):
                    return [sys.executable,'-c','pass'],os.environ.copy(),evidence/'logs'/f'{gate.name}-{n}.log'
                try:
                    with patch.object(suite,'command',command),patch.object(sys,'argv',[
                        'verify_suite.py','--shard','delivery','--delivery-part',part,'--jobs','1','--gate-jobs','2',
                        '--binary',sys.executable,'--evidence',str(root)]):
                        self.assertEqual(suite.main(),0)
                    report=json.loads((root/f'suite-delivery-{part}.json').read_text())
                    self.assertFalse(report['complete_inventory'])
                    self.assertEqual(len(report['shards']['delivery']),10 if part=='control' else 0)
                    self.assertEqual((root/'suite-delivery.json').read_text(),'canonical sentinel')
                    self.assertEqual((root/'suite-delivery-other.json').read_text(),'other sentinel')
                finally:
                    for sig,handler in handlers.items(): signal.signal(sig,handler)
        for flags in (['--list'],['--gate','delivery'],['--shard','runtime'],['--shard','delivery','--security-part','cases']):
            with patch.object(sys,'argv',['verify_suite.py','--delivery-part','wording',*flags]),self.assertRaises(SystemExit): suite.main()

    def test_failed_join_removes_stale_verdict(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);output=root/'suite-delivery.json';output.write_text('stale PASS')
            with patch.object(sys,'argv',['ci_delivery.py','join','--parts',str(root/'missing'),'--evidence',str(root)]):
                with self.assertRaises(OSError): delivery.main()
            self.assertFalse(output.exists())


if __name__=='__main__': unittest.main()
