"""Cross-job race inventory cannot certify partial/duplicate/drifted execution."""
import copy
import json
from pathlib import Path
import tempfile
import unittest

import ci_race as race
import ci_race_join as joined


class RaceJoinTests(unittest.TestCase):
    packages = [race.BRIDGE, race.SQLITE, 'github.com/z-chenhao/erii/new/package']
    source = dict(source_sha='a'*40, source_dirty=False, source_diff_sha256='empty')

    def fixture(self, root):
        for part in joined.PARTS:
            folder = root/part/'race'; folder.mkdir(parents=True)
            stages = []
            for name, selected in race.groups(self.packages, part):
                stage = dict(name=name, packages=selected, returncode=0, owned_group_drained=True)
                if name != 'remainder':
                    stage['command'] = race.RACE+(['-p=1'] if name == 'other-packages' else [])+selected
                    (folder/(name+'.log')).write_text('ok '+selected[0]+'\n')
                else:
                    names = [*race.SERIAL_SQLITE, 'TestA', 'TestB']
                    (folder/'sqlite-list.log').write_text('\n'.join(names)+'\nok\t'+race.SQLITE+'\t0.01s\n')
                    workers = []
                    for worker, tests in [('sqlite-serial', race.SERIAL_SQLITE), ('sqlite-1', names[1:])]:
                        path = folder/(worker+'.log')
                        events = [dict(Action='pass', Package=race.SQLITE, Test=n, Elapsed=.1) for n in tests]
                        events.append(dict(Action='pass', Package=race.SQLITE))
                        path.write_text('\n'.join(map(json.dumps, events)))
                        workers.append(dict(name=worker, returncode=0, owned_group_drained=True, expected_tests=tests,
                            gomaxprocs=4 if worker == 'sqlite-serial' else 1,
                            command=race.RACE+['-json','-run','^('+'|'.join(tests)+')$',race.SQLITE],
                            tests=race.sqlite_verdict(path, tests)))
                    stage.update(discovery=race.RACE+['-list','.',race.SQLITE], expected_tests=names, cpu_budget=4,
                                 selection_complete=True, isolated_tests=race.SERIAL_SQLITE,
                                 discovery_metrics=dict(owned_group_drained=True), workers=workers)
                stages.append(stage)
            report = dict(version=1, **self.source, source_stable=True, part=part, packages=self.packages,
                          selected_packages=[p for _, selected in race.groups(self.packages, part) for p in selected],
                          passed=True, stages=stages)
            (folder/'inventory.json').write_text(json.dumps(report))

    def test_two_parts_cover_default_inventory_without_new_or_omitted_packages(self):
        for packages in (self.packages, self.packages+[self.packages[-1]+'2'], self.packages[:2]):
            default = [p for _, group in race.groups(packages) for p in group]
            parts = [p for part in joined.PARTS for _, group in race.groups(packages,part) for p in group]
            self.assertCountEqual(default, parts)
            self.assertEqual(len(parts), len(set(parts)))
        with self.assertRaises(ValueError): race.groups(self.packages, 'unknown')

    def test_join_re_reads_actual_discovery_and_events_not_just_passed_boolean(self):
        for mutation in ('valid', 'missing-part', 'source', 'dirty', 'failed', 'packages', 'duplicate',
                         'command', 'process', 'discovery', 'events', 'outcome', 'missing-root', 'cpu'):
            with self.subTest(mutation=mutation), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp); self.fixture(root)
                path = root/'sqlite/race/inventory.json'; r = json.loads(path.read_text())
                stage = r['stages'][0]
                worker = stage['workers'][1]
                if mutation == 'valid':
                    self.assertTrue(joined.join(root,self.packages,self.source)['passed']); continue
                if mutation == 'missing-part':
                    (root/'packages').rename(root/'unexpected')
                elif mutation == 'source': r['source_sha'] = 'b'*40
                elif mutation == 'dirty': r['source_dirty'] = True
                elif mutation == 'failed': r['passed'] = False
                elif mutation == 'packages': r['selected_packages'].append(race.BRIDGE)
                elif mutation == 'duplicate': stage['workers'].append(copy.deepcopy(worker))
                elif mutation == 'command': worker['command'].remove('-race')
                elif mutation == 'process': worker['owned_group_drained'] = False
                elif mutation == 'discovery': (root/'sqlite/race/sqlite-list.log').write_text('TestMissing\n')
                elif mutation == 'events': (root/'sqlite/race/sqlite-1.log').write_text('')
                elif mutation == 'outcome': worker['tests']['TestA']['outcome'] = 'skip'
                elif mutation == 'cpu': worker['gomaxprocs'] = 8
                else: worker['expected_tests'].remove('TestB')
                path.write_text(json.dumps(r))
                with self.assertRaises(ValueError): joined.join(root,self.packages,self.source)
