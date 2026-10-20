#!/usr/bin/env python3
"""Automation Launch/Doctor/Drive/Evidence/Cleanup over real CLI and Actor."""
import argparse
from datetime import datetime, timezone
import json
from pathlib import Path
import sqlite3
import tempfile

from verify_main import MainHarness, require

FIXTURE = Path(__file__).resolve().parents[1] / 'evals/fixtures/automation-v1.json'

class Harness(MainHarness):
    now = '2026-10-15T12:00:00+08:00'

    def args(self, *args):
        return super().args('-test-automation-now', self.now, *args)

    def manage(self, key, **q):
        result = self.command('automation', 'manage', key, text=json.dumps(q))
        require(not result.get('error'), 'Automation request failed: ' + repr(result))
        return json.loads(result['text'])

    def automations(self):
        return json.loads(self.command('automation', 'list')['text'])

    def evidence(self):
        value = self.state()
        with sqlite3.connect(Path(self.env['ERII_DATA_ROOT']) / 'state.db') as db:
            db.row_factory = sqlite3.Row
            for table in ('automation', 'automation_fire', 'task', 'task_update', 'task_instruction', 'tool_invocation', 'main_inbox'):
                value[table] = [dict(row) for row in db.execute('SELECT * FROM ' + table)]
            value['automation_inbox'] = [dict(row) for row in db.execute("SELECT * FROM event_inbox WHERE consumer='automation_actor'")]
        return value


def run(binary):
    fixture = json.loads(FIXTURE.read_text())
    report = dict(version=1, fixture=fixture, scenarios={}, scope='Deterministic scheduling and notification fragments of GC-02/08/14, SC-05/12; no full M7 or model-quality score')
    with tempfile.TemporaryDirectory(prefix='erii-automation-') as folder:
        def scenario(name, action):
            root = Path(folder) / name
            root.mkdir()
            h = Harness(binary, root, fixture='automation')
            try:
                h.setup()
                report['scenarios'][name] = action(h)
            finally:
                h.cleanup()

        def goals(h):
            h.now = fixture['GC-02']['clock']
            q = dict(operation='create', instruction='Monthly goals review', triggerType='cron', triggerSpec=fixture['GC-02']['cron'], timezone='Asia/Shanghai')
            h.send('goals', 'Automation: ' + json.dumps(q))
            h.command('chat', 'work')
            rows = h.automations()
            require(len(rows) == 1 and rows[0]['nextFire'] == '2026-11-29T12:00:00Z', 'GC-02 first fire wrong')
            require(h.send('goals', 'Automation: ' + json.dumps(q)), 'duplicate ingress failed')
            h.command('chat', 'work')
            require(len(h.automations()) == 1, 'duplicate Automation')
            return h.evidence()
        scenario('GC-02', goals)

        def prices(h):
            q = dict(operation='create', instruction='Check HKG-CDG 2026-12-01 daily price; notify only a new entry below threshold.', triggerType='cron', triggerSpec='0 8 * * *', timezone='Asia/Shanghai', below=1800)
            receipt = h.manage('prices', **q)
            require(receipt == h.manage('prices', **q), 'create retry changed receipt')
            require(h.automations()[0]['nextFire'] == '2026-10-16T00:00:00Z', 'first price fire')
            states = []
            for day in fixture['GC-14']['prices']:
                h.now = day + 'T08:00:00+08:00'
                h.command('automation', 'tick')
                h.command('automation', 'tick')
                before = h.evidence()
                require(len(before['automation_fire']) == len(states)+1, 'duplicate day')
                h.command('chat', 'work')
                value = h.evidence()
                require(len([r for r in value['runs'] if r['kind']=='worker']) == len(states)+1, 'wrong Root count')
                require(value['task'][0]['status']=='active' and value['task'][0]['completion_policy']=='ongoing', 'monitor Task completed')
                h.command('chat', 'work')
                require(value == h.evidence(), 'repeat drain changed state')
                states.append(value)
            final = states[-1]
            notices = [f['fire_at'] for f in final['automation_fire'] if f['notify']]
            expected = [f['fire_at'] for f in final['automation_fire'] if any(d in datetime.fromtimestamp(f['fire_at']/1e6, timezone.utc).isoformat() for d in fixture['GC-14']['notify_dates'])]
            require(notices == expected and len(notices)==2 and len(final['task_update'])==2, 'GC-14 must notify only Oct18/Oct21')
            require(len(final['tool_invocation'])==12, 'two observed sources per day were not executed')
            row = h.automations()[0]
            h.manage('cancel', operation='cancel', automationRef=row['automationRef'], version=row['version'])
            h.now = '2026-11-01T08:00:00+08:00'
            h.command('chat', 'work')
            after = h.evidence()
            require(len(after['runs'])==len(final['runs']), 'Run created after cancel')
            return dict(daily_states=states, after_cancel=after)
        scenario('GC-14', prices)

        def delayed_cancel(h):
            h.manage('one', operation='create', instruction='Wedding guest-count reminder', triggerType='at', triggerSpec='2026-10-20T08:00:00+08:00', timezone='Asia/Shanghai')
            h.now='2026-10-22T08:00:00+08:00'
            h.command('automation','tick')
            row=h.automations()[0]
            h.manage('stop',operation='cancel',automationRef=row['automationRef'],version=row['version'])
            h.command('chat','work')
            value=h.evidence()
            require(not value['runs'] and value['automation_fire'][0]['state']=='skipped','delayed fire survived cancellation')
            return value
        scenario('GC-08-delayed-cancel', delayed_cancel)

        for boundary in ('after_result_commit', 'after_report_commit'):
            def edit_committed_result(h, boundary=boundary):
                h.manage('old', operation='create', instruction='OLD reminder intent', triggerType='at', triggerSpec='2026-10-20T08:00:00+08:00', timezone='Asia/Shanghai')
                h.now = '2026-10-20T08:00:00+08:00'
                child = h.hold(boundary)
                child.kill()
                child.communicate(timeout=10)
                before = h.evidence()
                require(len(before['task_update']) == 1, 'result was not committed before edit')
                require(len(before['main_inbox']) == int(boundary == 'after_report_commit'), 'wrong report boundary')
                row = h.automations()[0]
                h.manage('edit', operation='update', automationRef=row['automationRef'], version=row['version'], instruction='NEW reminder intent', triggerType='at', triggerSpec='2026-10-22T08:00:00+08:00', timezone='Asia/Shanghai')
                h.command('chat', 'work')
                stale = h.evidence()
                require(stale['automation'][0]['schedule_version'] == 2 and stale['automation_fire'][0]['schedule_version'] == 1, 'edit did not change lineage')
                require(not [r for r in stale['runs'] if r['kind'] == 'main'] and not stale['deliveries'], 'obsolete result produced Main or delivery')
                h.command('chat', 'work')
                require(stale == h.evidence(), 'obsolete inbox did not drain idempotently')
                h.now = '2026-10-22T08:00:00+08:00'
                h.command('chat', 'work')
                current = h.evidence()
                require(len(current['automation_fire']) == 2 and current['automation_fire'][1]['schedule_version'] == 2 and current['automation_fire'][1]['instruction'] == 'NEW reminder intent', 'edited intent did not fire')
                require(len([r for r in current['runs'] if r['kind'] == 'main' and r['status'] == 'completed']) == 1 and len(current['deliveries']) == 1 and current['deliveries'][0]['status'] == 'sent', 'edited intent did not deliver once')
                h.command('chat', 'work')
                require(current == h.evidence(), 'edited intent repeated delivery')
                return dict(before_edit=before, obsolete_suppressed=stale, edited_intent_delivered=current)
            scenario('schedule-edit-' + boundary, edit_committed_result)

        for boundary in ('before_automation_fire_commit','after_automation_fire_commit','before_automation_admit_commit','after_automation_admit_commit'):
            def crash(h, boundary=boundary):
                h.manage('one', operation='create', instruction='Wedding reminder', triggerType='at', triggerSpec='2026-10-20T08:00:00+08:00', timezone='Asia/Shanghai')
                h.now='2026-10-20T08:00:00+08:00'
                command=('automation','tick') if 'fire_commit' in boundary else ('automation','work')
                if 'admit' in boundary:
                    h.command('automation','tick')
                child=h.hold(boundary,command=command)
                child.kill();child.communicate(timeout=10)
                before=h.evidence()
                h.command('chat','work')
                after=h.evidence()
                require(len(after['automation_fire'])==1 and len(after['task'])==1 and len(after['automation_inbox'])==1,'restart duplicated trigger/admission')
                require(len([r for r in after['runs'] if r['kind']=='worker'])==1 and len(after['task_update'])==1,'restart lost or duplicated Root/result')
                h.command('chat','work')
                require(after==h.evidence(),'repeated restart changed final state')
                return dict(before_kill=before,recovered=after)
            scenario(boundary,crash)
    report['cleanup']='All owned processes joined; disposable state removed'
    return report


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--binary',type=Path,required=True)
    parser.add_argument('--evidence',type=Path,required=True)
    args=parser.parse_args();args.evidence.unlink(missing_ok=True)
    value=run(args.binary.resolve())
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    args.evidence.write_text(json.dumps(value,indent=2)+'\n')
    print(json.dumps(dict(result='PASS',evidence=str(args.evidence),scenarios=len(value['scenarios']),cleanup=value['cleanup'])))

if __name__=='__main__':main()
