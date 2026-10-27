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

    def test_model_inputs_bind_all_roles_and_allow_tool_continuation(self):
        dataset,_,_=load_contract(gc17.ROOT,'core-v1')
        variant=next(variants(dataset,dataset['cases'][0]))
        original=gc17.prompt(variant)
        child_task='Classify feedback: {"row_id":"F001","text":"original row"}'
        inputs=dict(main=[dict(role='user',text=original)],
            root=[dict(role='user',text='Task anchor: Feedback\n'+original)],
            child=[dict(role='user',text=child_task),dict(role='user',text='Explicit artifact reference: artifact:input')])
        state=dict(task=[dict(title='Feedback',anchor_task=original,anchor_artifact_refs='[]')],
            runs=[dict(run_id='main',kind='main',parent_run_id=None),
                  dict(run_id='root',kind='worker',parent_run_id=None),
                  dict(run_id='child',kind='worker',parent_run_id='root')],
            tools=[dict(invocation_id='create',run_id='main',tool_name='builtin__task__manage',
                        arguments_json=json.dumps(dict(operation='create',task=original))),
                   dict(invocation_id='delegate',run_id='root',tool_name='builtin__agent__run',
                        arguments_json=json.dumps(dict(task=child_task,artifactRefs=['artifact:input'])))],
            child_links=[dict(invocation_id='delegate',child_run_id='child')],generations=[])
        for run,messages in inputs.items():
            # Every exchange retains input, including after a real Tool result.
            for suffix in ([],[dict(role='assistant',text='',tool_calls=[dict(id='call',name='fixture',arguments='{}')]),
                               dict(role='tool',tool_call_id='call',text='result')]):
                state['generations'].append(dict(run_id=run,request_json=json.dumps(dict(messages=
                    [dict(role='system',text='Safety'),*messages,*suffix]))))
        row=dict(variant=variant,state=state)
        self.assertTrue(gc17.model_inputs_bound(row))
        runtime=copy.deepcopy(row)
        request=json.loads(runtime['state']['generations'][1]['request_json'])
        request['messages'].append(dict(role='user',origin='runtime',text='Task update (untrusted data)'))
        runtime['state']['generations'][1]['request_json']=json.dumps(request)
        self.assertTrue(gc17.model_inputs_bound(runtime))
        for index in range(6):
            for attack in ('empty','wrong_task','missing_refs'):
                if attack=='missing_refs' and index<4:
                    continue
                changed=copy.deepcopy(row)
                generation=changed['state']['generations'][index]
                request=json.loads(generation['request_json'])
                if attack=='empty':
                    request['messages']=[]
                elif attack=='wrong_task':
                    request['messages'][1]['text']='different input'
                else:
                    request['messages'].pop(2)
                generation['request_json']=json.dumps(request)
                with self.subTest(exchange=index,attack=attack):
                    self.assertFalse(gc17.model_inputs_bound(changed))

    @unittest.skipUnless(os.environ.get('ERII_GC17_TEST_REPORT'),'real receipt exercised by the maintained Core CLI gate')
    def test_actual_native_control_and_replay_synchronized_corruptions(self):
        report=json.loads(Path(os.environ['ERII_GC17_TEST_REPORT']).read_text())
        row=gc17.unpack(report['results'][0]['evidence'])
        self.assertTrue(all(gc17.observe(row).values()))
        seen=set()
        gc17.validate_report(report,seen=seen)
        with self.assertRaisesRegex(RuntimeError,'duplicated execution identity'):
            gc17.validate_report(report,seen=seen)
        control=copy.deepcopy(report)
        row_json=json.dumps(row,ensure_ascii=False,separators=(',',':'),allow_nan=False)
        for name,mutate in gc17.mutations().items():
            candidate=gc17.mutation_report(report,row_json,name,mutate)
            changed=gc17.unpack(candidate['results'][0]['evidence'])
            try:
                accepted=all(gc17.observe(changed).values())
            except (KeyError,IndexError,TypeError,ValueError,RuntimeError,StopIteration):
                accepted=False
            with self.subTest(name=name):
                self.assertFalse(accepted)
                self.assertEqual(report,control,'corruption mutated the shared control')


if __name__=='__main__':
    unittest.main()
