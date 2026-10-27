"""GC-17 receipt integrity and optional real native regression control."""
import copy
import json
import os
from pathlib import Path
import unittest

import gc17
from case_runner import load_contract, variants
from unscripted_cases import inputs, matrix


class FeedbackReceiptTests(unittest.TestCase):
    def test_original_21_blocks_six_variants_and_allowed_inputs(self):
        self.assertEqual(len(gc17.load()['frozen_blocks']),21)
        dataset, _, _=load_contract(gc17.ROOT,'core-v1')
        for variant in variants(dataset,dataset['cases'][0]):
            value=inputs('GC-17',variant['variant_id'])
            event=value['events'][0]['text']
            self.assertTrue(event.startswith(variant['events'][0]['text']+'\n'))
            self.assertEqual(json.loads(event.split('Feedback assets:\n',1)[1]),gc17.assets())
            self.assertEqual(value['context'],variant['context'])
            self.assertNotIn('gold_distribution',event)
            self.assertNotIn('agent.run',event)
            self.assertNotIn('Capabilities:',event)
            self.assertNotIn('call_key',event)
        self.assertFalse(matrix()['GC-17']['eligible_for_grading'])
        with self.assertRaises(ValueError):
            inputs('GC-17','invented')

    def test_compressed_native_bytes_reject_corrupt_or_resealed_sizes(self):
        row=dict(text='逐条回查',state=[1,2,3])
        good=gc17.pack(row)
        self.assertEqual(gc17.unpack(good),row)
        for change in (dict(sha256='0'*64),dict(bytes=good['bytes']+1),dict(bytes=True),
                       dict(uncompressed_bytes=good['uncompressed_bytes']+1),dict(uncompressed_bytes=1),
                       dict(uncompressed_bytes=True),dict(uncompressed_bytes=513<<20),dict(data='not base64'),dict(kind='flags')):
            with self.subTest(change=change), self.assertRaises((RuntimeError,ValueError)):
                gc17.unpack(dict(good,**change))
        extra=dict(good,passed=True)
        with self.assertRaises(RuntimeError):
            gc17.unpack(extra)

    @unittest.skipUnless(os.environ.get('ERII_GC17_TEST_REPORT'),'real receipt exercised by the maintained Core CLI gate')
    def test_actual_native_control_and_replay_synchronized_corruptions(self):
        report=json.loads(Path(os.environ['ERII_GC17_TEST_REPORT']).read_text())
        row=gc17.unpack(report['results'][0]['evidence'])
        self.assertTrue(all(gc17.observe(row).values()))
        seen=set()
        gc17.validate_report(report,seen=seen)
        with self.assertRaisesRegex(RuntimeError,'duplicated execution identity'):
            gc17.validate_report(report,seen=seen)
        for name,mutate in gc17.mutations().items():
            changed=copy.deepcopy(row)
            mutate(changed)
            changed['replay_state']=copy.deepcopy(changed['state'])
            try:
                accepted=all(gc17.observe(changed).values())
            except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
                accepted=False
            with self.subTest(name=name):
                self.assertFalse(accepted)


if __name__=='__main__':
    unittest.main()
