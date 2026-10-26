"""Communication partitioning must preserve commands, native checks and inventory."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import ci_communication as ci
from verify_suite import SHARDS, command, communication_gates, COMMUNICATION_PARTS


class CommunicationTests(unittest.TestCase):
    def test_ci_retains_compact_failure_diagnostics_beside_manifests(self):
        workflow=(ci.ROOT/'.github/workflows/ci.yml').read_text()
        upload=workflow.split('- name: Retain compact communication evidence, including failures\n',1)[1].split('\n  communication_join:',1)[0]
        self.assertIn("always() && matrix.shard == 'communication'",upload)
        paths=upload.split('          path: |\n',1)[1].split('          if-no-files-found:',1)[0]
        self.assertIn('            evidence/gc12-*/failure/\n',paths)
        self.assertIn('            evidence/gc13-*.progress.jsonl\n',paths)
        self.assertIn('            evidence/*.failure.summary.json\n',paths)
        self.assertNotIn('.gc13-native',paths)
        self.assertNotIn('rejected-report.json',paths)
        self.assertNotIn('            evidence/\n',paths)

    def fixture(self, root):
        source = dict(source_sha='a'*40, source_dirty=False, source_diff_sha256='b'*64)
        suites, manifests = {}, {}
        for part in COMMUNICATION_PARTS:
            (root / part).mkdir()
            gates = communication_gates(part)
            suites[part] = dict(**source, passed=True, source_stable=True, selected_gates=None,
                communication_part=part, complete_inventory=False, binary_sha256='c'*64,
                **ci.run_identity(), jobs=1, gate_jobs=2, elapsed_seconds=12, shards={'communication': [
                    dict(gate=g.name, attempt=n, returncode=0, owned_group_drained=True,
                         command=command(g,n,Path('/binary'),root/part)[0])
                    for g in gates for n in range(1, g.attempts+1)]})
            manifests[part] = dict(version=1, part=part, fixtures={'fixture':'sha'},
                identity={k:suites[part][k] for k in ci.IDENTITY}, receipts=[
                    dict(gate=g.name, attempt=n, status='mechanical_pass', semantic_score=None,
                        cleanup=True, native=dict(path=ci.report_name(g,n), bytes=100, sha256='e'*64),
                        rows=[dict(variant_id=v, repetition=r, assertions={'native':True}, cleanup=dict(
                            owned_processes_joined=True, disposable_root_removed=True,
                            **({'http_peer_joined':True} if g.name=='gc12' else {})),
                            **({'execution_ids':{table:[f'{n}/{v}/{table}'] for table in
                                ('runs','tools','generations','requests','conversation')}} if g.name=='gc12' else {}),
                            **({'execution_ids':{table:[f'{g.name}/{v}/{r}/{table}'] for table in
                                ('runs','tools','generations','messages','deliveries','artifacts')}} if g.script=='verify_gc13.py' else {}))
                            for v,r in ci.pairs(g)])
                    for g in gates for n in range(1,g.attempts+1)])
        return source, suites, manifests

    def write(self, root, suites, manifests):
        for part,suite in suites.items():
            folder=root/part
            path=folder/f'suite-communication-{part}.json'
            path.write_text(json.dumps(suite))
            manifests[part]['suite']=ci.file_record(path)
            (folder/'communication-receipts.json').write_text(json.dumps(manifests[part]))

    def test_partition_changes_placement_only_and_join_restores_original_inventory(self):
        planned=[g for part in COMMUNICATION_PARTS for g in communication_gates(part)]
        self.assertCountEqual(planned,SHARDS['communication'])
        self.assertEqual([g.name for g in communication_gates('gc01')],['gc01'])
        self.assertEqual(COMMUNICATION_PARTS, ('gc01','gc02','gc09','gc10','gc13-wording-1','gc13-wording-2','gc13-wording-3','gc13-noise-1','gc13-noise-2','gc13-order-1','channels'))
        for part in COMMUNICATION_PARTS[:-1]:
            self.assertEqual([g.name for g in communication_gates(part)],[part])
        self.assertEqual([g.name for g in communication_gates('channels')],['gc12','gc08','gc14','gc16','gc11'])
        for gate in planned:
            original=next(g for g in SHARDS['communication'] if g.name==gate.name)
            if gate.script=='verify_gc13.py':
                self.assertEqual(gate.arguments,('--ci-part',gate.name.removeprefix('gc13-')))
                continue
            self.assertEqual(command(gate,1,Path('/binary'),Path('/evidence')),
                             command(original,1,Path('/binary'),Path('/evidence')))
        with self.assertRaises(ValueError): communication_gates('unknown')
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);source,suites,manifests=self.fixture(root)
            self.write(root,suites,manifests)
            with patch.object(ci,'source_state',return_value=source),patch.object(ci,'fixtures',return_value={'fixture':'sha'}):
                full=ci.join(root)
            self.assertTrue(full['complete_inventory'])
            self.assertIsNone(full['semantic_score'])
            self.assertIs(full['release_ready'], False)
            self.assertIsNone(full['communication_part'])
            self.assertEqual([(r['gate'],r['attempt']) for r in full['shards']['communication']],
                [(g.name,n) for g in SHARDS['communication'] for n in range(1,g.attempts+1)])
            self.assertLess(sum((root/p/'communication-receipts.json').stat().st_size for p in COMMUNICATION_PARTS),1<<20)

    def test_join_rejects_every_incomplete_failed_or_drifted_boundary(self):
        mutations=('missing_part','extra_part','missing_round','duplicate_round','reordered_round',
            'failed','cancelled','unclean','dirty','unstable','focused','wrong_part','complete',
            'source','binary','workflow','fixture','suite_hash','missing_report','duplicate_report',
            'reordered_report','missing_variant','duplicate_variant','reordered_variant','bad_repetition',
            'failed_assertion','false_bool','empty_assertions','cleanup','missing_cleanup',
            'missing_native','invalid_native_hash','graded','failed_native','manifest_identity')
        for mutation in mutations:
            with self.subTest(mutation=mutation),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);source,suites,manifests=self.fixture(root)
                suite=suites['channels'];rows=suite['shards']['communication'];manifest=manifests['channels']
                receipt=manifest['receipts'][0];variants=receipt['rows']
                if mutation=='missing_part': del suites['gc01']
                elif mutation=='extra_part': (root/'unexpected').mkdir()
                elif mutation=='missing_round': rows.pop()
                elif mutation=='duplicate_round': rows.append(copy.deepcopy(rows[0]))
                elif mutation=='reordered_round': rows[0],rows[1]=rows[1],rows[0]
                elif mutation=='failed': rows[0]['returncode']=1
                elif mutation=='cancelled': rows[0]['returncode']=130
                elif mutation=='unclean': rows[0]['owned_group_drained']=False
                elif mutation=='dirty': suite['source_dirty']=True
                elif mutation=='unstable': suite['source_stable']=False
                elif mutation=='focused': suite['selected_gates']=['gc12']
                elif mutation=='wrong_part': suite['communication_part']='gc01'
                elif mutation=='complete': suite['complete_inventory']=True
                elif mutation=='source': suite['source_sha']='drift'
                elif mutation=='binary': suite['binary_sha256']='drift';manifest['identity']['binary_sha256']='drift'
                elif mutation=='workflow': suite['workflow_sha']='drift';manifest['identity']['workflow_sha']='drift'
                elif mutation=='fixture': manifest['fixtures']={}
                elif mutation=='missing_report': manifest['receipts'].pop()
                elif mutation=='duplicate_report': manifest['receipts'].append(copy.deepcopy(receipt))
                elif mutation=='reordered_report': manifest['receipts'].reverse()
                elif mutation=='missing_variant': variants.pop()
                elif mutation=='duplicate_variant': variants.append(copy.deepcopy(variants[0]))
                elif mutation=='reordered_variant': variants.reverse()
                elif mutation=='bad_repetition': variants[0]['repetition']=True
                elif mutation=='failed_assertion': variants[0]['assertions']['native']=False
                elif mutation=='false_bool': variants[0]['assertions']['native']=1
                elif mutation=='empty_assertions': variants[0]['assertions']={}
                elif mutation=='cleanup': variants[0]['cleanup']['owned_processes_joined']=False
                elif mutation=='missing_cleanup': variants[0]['cleanup'].pop('http_peer_joined')
                elif mutation=='missing_native': receipt['native']['bytes']=0
                elif mutation=='invalid_native_hash': receipt['native']['sha256']='z'*64
                elif mutation=='graded': receipt['semantic_score']=100
                elif mutation=='failed_native': receipt['status']='inconclusive'
                elif mutation=='manifest_identity': manifest['identity']={}
                self.write(root,suites,manifests)
                if mutation=='suite_hash':
                    path=root/'channels'/'communication-receipts.json'
                    value=json.loads(path.read_text());value['suite']['sha256']='f'*64;path.write_text(json.dumps(value))
                with patch.object(ci,'source_state',return_value=source),patch.object(ci,'fixtures',return_value={'fixture':'sha'}):
                    with self.assertRaises((ValueError,OSError,KeyError)): ci.join(root)

    def test_each_fixed_part_rejects_failed_or_mismatched_receipts(self):
        for part in COMMUNICATION_PARTS:
            for mutation in ('missing','failed','cancelled','unclean','focused','command',
                             'jobs','gate_jobs','run_id','run_attempt','workflow_sha','missing_identity',
                             'missing_native','duplicate_native','reordered_variant','assertion','cleanup'):
                with self.subTest(part=part,mutation=mutation),tempfile.TemporaryDirectory() as temp:
                    root=Path(temp);source,suites,manifests=self.fixture(root)
                    suite=suites[part];row=suite['shards']['communication'][0]
                    receipt=manifests[part]['receipts'][0]
                    if mutation=='missing': del suites[part]
                    elif mutation=='failed': row['returncode']=1
                    elif mutation=='cancelled': row['returncode']=130
                    elif mutation=='unclean': row['owned_group_drained']=False
                    elif mutation=='focused': suite['selected_gates']=[row['gate']]
                    elif mutation=='command': row['command'].append('--focused')
                    elif mutation in ('jobs','gate_jobs'): suite[mutation]=3
                    elif mutation in ('run_id','run_attempt','workflow_sha'): suite[mutation]='other'
                    elif mutation=='missing_identity': del suite['run_attempt']
                    elif mutation=='missing_native': manifests[part]['receipts'].pop()
                    elif mutation=='duplicate_native': manifests[part]['receipts'].append(copy.deepcopy(receipt))
                    elif mutation=='reordered_variant': receipt['rows'].reverse()
                    elif mutation=='assertion': receipt['rows'][0]['assertions']['native']=False
                    elif mutation=='cleanup': receipt['rows'][0]['cleanup']['owned_processes_joined']=False
                    # Reseal the manifest: independently validated fields must still reject it.
                    if mutation in ('run_id','run_attempt','workflow_sha'):
                        manifests[part]['identity'][mutation]='other'
                    self.write(root,suites,manifests)
                    with patch.object(ci,'source_state',return_value=source),patch.object(ci,'fixtures',return_value={'fixture':'sha'}):
                        with self.assertRaises((ValueError,OSError,KeyError)): ci.join(root)

    def test_matched_old_run_or_attempt_cannot_be_joined_in_current_workflow(self):
        for field in ('run_id','run_attempt','workflow_sha'):
            with self.subTest(field=field),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);source,suites,manifests=self.fixture(root)
                for part in COMMUNICATION_PARTS:
                    suites[part][field]='old'
                    manifests[part]['identity'][field]='old'
                self.write(root,suites,manifests)
                with patch.object(ci,'source_state',return_value=source),patch.object(ci,'fixtures',return_value={'fixture':'sha'}):
                    with self.assertRaises(ValueError): ci.join(root)

    def test_collector_recomputes_native_oracle_before_emitting_a_manifest(self):
        from test_gc12 import MeetingTests
        import verify_gc12 as driver
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);gate=next(g for g in SHARDS['communication'] if g.name=='gc12')
            report=MeetingTests().packets()
            binary=root/'erii';binary.write_bytes(b'unit test binary identity only')
            report['binary_sha256']=ci.file_record(binary)['sha256']
            report['identity']['binary_sha256']=report['binary_sha256']
            for row in report['variants']:
                row['fixture_sha256']=ci.file_record(driver.FIXTURE)['sha256']
                row['cleanup']['http_peer_joined']=True
            path=root/'gc12-1.json';path.write_text(json.dumps(report))
            suite={'binary_sha256':report['binary_sha256']}
            receipt=ci.native_receipt(gate,1,root,binary,suite)
            ci.check_receipts([receipt], [type(gate)(gate.name,gate.script,'gc12-1',1)])
            # Keep all author flags true while corrupting an observed side effect.
            report['variants'][0]['remote']['mail'][0]['recipient']='wrong@example.test'
            path.write_text(json.dumps(report))
            with self.assertRaises(ValueError): ci.native_receipt(gate,1,root,binary,suite)

    def test_join_rejects_native_journey_reuse_across_gc12_rounds(self):
        for field in ('runs','tools','generations','requests','conversation'):
            with self.subTest(field=field),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);source,suites,manifests=self.fixture(root)
                gc12=[r for r in manifests['channels']['receipts'] if r['gate']=='gc12']
                gc12[1]['rows'][0]['execution_ids'][field]=copy.deepcopy(gc12[0]['rows'][0]['execution_ids'][field])
                self.write(root,suites,manifests)
                with patch.object(ci,'source_state',return_value=source),patch.object(ci,'fixtures',return_value={'fixture':'sha'}):
                    with self.assertRaisesRegex(ValueError,'reused or invalid GC-12 execution identities: '+field):ci.join(root)

    def test_join_rejects_resealed_gc13_reuse_across_fixed_parts(self):
        fields=('runs','tools','generations','messages','deliveries','artifacts')
        for field in fields:
            with self.subTest(field=field),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);source,suites,manifests=self.fixture(root)
                first=manifests['gc13-wording-1']['receipts'][0]['rows'][0]
                second=manifests['gc13-noise-1']['receipts'][0]['rows'][0]
                second['execution_ids'][field]=copy.deepcopy(first['execution_ids'][field])
                self.write(root,suites,manifests)
                with patch.object(ci,'source_state',return_value=source),patch.object(ci,'fixtures',return_value={'fixture':'sha'}):
                    with self.assertRaisesRegex(ValueError,'reused or invalid GC-13 execution identities: '+field):ci.join(root)

    def test_part_selection_rejects_wrong_shard_and_listing(self):
        import contextlib
        import io
        import verify_suite
        for extra in (['--shard','security'],['--list']):
            with self.subTest(extra=extra),patch('sys.argv',['verify_suite.py','--communication-part','gc01',*extra]),contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as error: verify_suite.main()
                self.assertEqual(error.exception.code,2)

    def test_failed_collection_or_join_clears_a_previous_success(self):
        for operation, filename in [('collect','communication-receipts.json'),('join','suite-communication.json')]:
            with self.subTest(operation=operation), tempfile.TemporaryDirectory() as temp:
                root=Path(temp);output=root/filename;output.write_text('previous success')
                argv=['ci_communication.py',operation,'--evidence',temp]
                argv += ['--part','gc01','--binary','/unused'] if operation=='collect' else ['--parts',temp]
                with patch('sys.argv',argv),patch.object(ci,operation,side_effect=ValueError('incomplete')):
                    with self.assertRaises(ValueError): ci.main()
                self.assertFalse(output.exists())

    def test_collector_rejects_source_fixture_or_binary_change_during_validation(self):
        for drift in ('source','fixture','binary'):
            with self.subTest(drift=drift),tempfile.TemporaryDirectory() as temp:
                root=Path(temp);source,suites,manifests=self.fixture(root)
                binary=root/'erii';binary.write_bytes(b'binary')
                suites['gc01']['binary_sha256']=ci.file_record(binary)['sha256']
                self.write(root,suites,manifests)
                by_key={(r['gate'],r['attempt']):r for r in manifests['gc01']['receipts']}
                def native(g,n,*_):
                    if drift=='binary': binary.write_bytes(b'changed')
                    return by_key[g.name,n]
                with patch.object(ci,'native_receipt',side_effect=native), \
                     patch.object(ci,'source_state',side_effect=[source,dict(source,source_sha='changed')] if drift=='source' else [source,source]), \
                     patch.object(ci,'fixtures',side_effect=[{'fixture':'sha'},{'fixture':'changed'}] if drift=='fixture' else [{'fixture':'sha'},{'fixture':'sha'}]):
                    with self.assertRaises(ValueError): ci.collect(root/'gc01','gc01',binary)


if __name__=='__main__': unittest.main()
