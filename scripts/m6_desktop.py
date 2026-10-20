"""Launch/Doctor/Drive/Evidence/Cleanup for exact browser Computer journeys."""
from contextlib import contextmanager
import hashlib
import json
from pathlib import Path
import selectors
import shutil
import signal
import subprocess

from verify_main import require
from verify_computer_use import launch


@contextmanager
def browser(folder,world):
    root=folder/'browser';root.mkdir()
    (root/'world.json').write_text(json.dumps(world,ensure_ascii=False))
    if 'parallel_requests' in world:
        shutil.copytree(Path(__file__).resolve().parents[1]/'evals/fixtures/m6-v1/media',root/'media')
    child=subprocess.Popen([shutil.which('node'),str(Path(__file__).with_name('m6_browser.mjs')),str(root)],
                           stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(child.stdout,selectors.EVENT_READ)
            require(selector.select(15),'doctor: Chromium sandbox not ready')
        line=child.stdout.readline()
        require(line,'browser exited: '+(child.stderr.read() if child.poll() is not None else ''))
        yield json.loads(line),root
    finally:
        if child.poll() is None:child.send_signal(signal.SIGTERM)
        _,err=child.communicate(timeout=20)
        require(child.returncode==0,'browser cleanup failed: '+err)


def drive(binary,folder,case,variant,artifact_folder=None):
    from run_m6_cases import execute, open_bytes, save_bytes, save_report, state, export_artifacts, ASSETS
    cid=case['case_id']
    evidence={'scope':'L1 real Chromium and production browser adapter, Fake Lume/SSH lifecycle; no macOS GUI or current L2 claim'}
    with browser(folder,case['world']) as (peer,browser_root):
        with launch(binary,folder,browser_peer=peer['endpoint']) as h:
            h.send('context',variant['context'])
            for i,event in enumerate(variant['events']):h.send('event-'+str(i),event['at']+' '+event['text'])
            url=json.dumps(peer['url'])
            prefix='const tabs=await cua.listTabs();const tab=await cua.getTab(tabs[0].id);\n'
            pw='await tab.capabilities.list();const pw=await tab.capabilities.get("playwright");\n'
            checks={}
            if cid=='GC-07':
                observed=execute(h,'observe-network',prefix+'await tab.goto('+url+'+"/network");const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);return {computer:(await cua.getState()).computerId,state:view.ax.state};')
                before=json.loads((browser_root/'external-state.json').read_text())
                denied=execute(h,'denied-repair',prefix+pw+'try{await pw.locator("#repair").click();}catch(e){return e.code;}',approve=False)
                rejected=json.loads((browser_root/'external-state.json').read_text())
                repaired=execute(h,'repair-network',prefix+pw+'await pw.locator("#repair").click();await tab.goto('+url+'+"/network");const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);return {computer:(await cua.getState()).computerId,state:view.ax.state};')
                after=json.loads((browser_root/'external-state.json').read_text())
                checks.update(observe_link_before_change='Not Connected' in observed['value']['state'] and not before['network']['gateway'],
                              denied_unchanged=denied['value']=='APPROVAL_DENIED' and rejected==before,
                              repaired_all_paths=all(after['network'][p] for p in ('gateway','ordinary','proxy','apple_tv')),
                              one_protocol=after['network']['protocols']==1,
                              stable_computer=observed['value']['computer']==repaired['value']['computer'],
                              impact_in_observation='interrupts gateway' in observed['value']['state'])
                evidence.update(observed=observed,denied=denied,repaired=repaired,external_before=before,external_after=after)
                shots=[item['data'] for record in (observed,repaired) for item in record['output'] if item['type']=='image']
            elif cid=='GC-13':
                observed=execute(h,'final-confirmation',prefix+'await tab.goto('+url+'+"/booking");const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);return {computer:(await cua.getState()).computerId,state:view.ax.state};')
                before=json.loads((browser_root/'external-state.json').read_text())
                selector='#submit[data-restaurant="R1"][data-name="松间·湖滨店"][data-date="2026-10-17"][data-time="19:00"][data-party="2"][data-price-pp="280"][data-deposit="0"]'
                locator='pw.locator('+json.dumps(selector,ensure_ascii=False)+')'
                denied=execute(h,'denied-submit',prefix+pw+'try{await '+locator+'.click();}catch(e){return e.code;}',approve=False)
                rejected=json.loads((browser_root/'external-state.json').read_text())
                submitted=execute(h,'submit-unknown',prefix+pw+'let outcome;try{await '+locator+'.click();outcome="acknowledged";}catch(e){outcome=e.code;}return outcome;')
                unknown=state(h)
                # A new CLI process and new CUA binding query My Orders. There
                # is no second submission, no fabricated reconciliation verdict.
                reconciled=execute(h,'query-orders',prefix+'await tab.goto('+url+'+"/orders");const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);return {computer:(await cua.getState()).computerId,state:view.ax.state};')
                after=json.loads((browser_root/'external-state.json').read_text())
                checks.update(no_submit_before_approval=before['orders']==rejected['orders']==[] and denied['value']=='APPROVAL_DENIED',
                              confirmation_fields=all(term in observed['value']['state'] for term in ('松间·湖滨店','2026-10-17','19:00','560','无押金')),
                              business_parameters_frozen=any(all(term in d['approval']['arguments'] for term in ('R1','松间·湖滨店','2026-10-17','19:00','280','data-party','data-deposit')) for d in submitted['decisions']),
                              lost_ack_unknown=submitted['value']=='UNKNOWN_SIDE_EFFECT' and any(i['status']=='unknown' for i in unknown['tools']),
                              one_exact_order=after['submissions']==1 and after['orders']==[dict(order_id='order_781',restaurant='R1',date='2026-10-17',time='19:00',party=2)],
                              query_proves_order='order_781' in reconciled['value']['state'],
                              stable_computer=observed['value']['computer']==reconciled['value']['computer'],
                              unknown_never_replayed=all(i['attempt_count']==1 for i in unknown['tools'] if i['status']=='unknown'))
                evidence.update(observed=observed,denied=denied,submitted=submitted,unknown_state=unknown,reconciled=reconciled,external_after=after,
                                reconciliation='read-only My Orders evidence; unknown original invocation is retained, never relabelled or retried')
                shots=[item['data'] for record in (observed,reconciled) for item in record['output'] if item['type']=='image']
            else:
                sources={}
                for name in ('meeting_0901.md','meeting_0915.md','meeting_1001.md','metrics.csv','architecture.png'):
                    media='image/png' if name.endswith('.png') else 'text/csv' if name.endswith('.csv') else 'text/markdown'
                    sources[name]=save_bytes(h,name,media,(ASSETS/name).read_bytes())['artifact_ref']
                meetings={name:ref for name,ref in sources.items() if name.startswith('meeting_')}
                design=execute(h,'technical-design', '''const sources=SOURCES;const notes={};
for(const [name,artifactRef] of Object.entries(sources))notes[name]=(await tools.builtin__artifact__read({artifactRef})).text;
const confirmed=[...notes["meeting_0915.md"].trim().split("\\n"),...notes["meeting_1001.md"].trim().split("\\n")];
const design="# Decisions\\n"+confirmed.join("\\n")+"\\n# Non-goals\\nNo second loop or Workflow domain.\\n# Protocol\\nMain owns Tasks; Child returns final business text and ArtifactRef to direct Parent.\\n# Milestones\\nM0-M5 Core, M6 Memory/Computer, M7 automation/channels.\\n# Acceptance\\nFrozen snapshots; approval; cancellation; replay; isolation.\\n# Pending\\nProduction capacity and Live Shadow verification.\\nSources: "+JSON.stringify(sources);
const saved=await tools.builtin__artifact__save({name:"technical_design.md",mediaType:"text/markdown",text:design});
await tools.builtin__artifact__publish({artifactRefs:saved.artifactRefs});return {confirmed,artifactRefs:saved.artifactRefs};'''.replace('SOURCES',json.dumps(meetings)),refs=list(meetings.values()),task='技术设计')
                summary=execute(h,'customer-summary',save_report('''let text="",offset=0,total=1;
while(offset<total){const part=await tools.builtin__artifact__read({artifactRef:REF,offset,limit:4096});text+=part.text;offset+=part.bytes;total=part.total_bytes;}
const rows=text.trim().split("\\n").slice(1).map(line=>line.split(","));
const result={rows:rows.length,communicated_clients:new Set(rows.filter(r=>r[1]==="1").map(r=>r[0])).size,adopted_clients:new Set(rows.filter(r=>r[2]==="1").map(r=>r[0])).size};'''.replace('REF',json.dumps(sources['metrics.csv'])),'customer_summary.json'),refs=[sources['metrics.csv']],task='客户数据摘要')
                media=[]
                for name in ('red.mov','blue.mov','third.mov'):
                    media.append(save_bytes(h,name,'video/quicktime',(ASSETS/'media'/name).read_bytes()))
                stages=[execute(h,'desktop-materials',prefix+'await tab.goto('+url+'+"/clips");\nconst files=FILES;const descriptors=[];\nfor(const file of files){\n descriptors.push(await tools.builtin__artifact__describe({artifactRef:file.artifact_ref}));\n await tab.capabilities.list();const pw=await tab.capabilities.get("playwright");\n await pw.locator("#label").fill(file.name);await pw.locator("#save").click();\n const view=await tab.getAXStateAndScreenshot();image(view.screenshot.image);\n}\nreturn {files:descriptors,state:(await tab.getAXState({full:true})).state};'.replace('FILES',json.dumps(media)),refs=[m['artifact_ref'] for m in media],task='桌面素材')]
                after=json.loads((browser_root/'external-state.json').read_text())
                design_text=open_bytes(h,design['value']['artifactRefs'][0]).decode()
                final=state(h)
                checks.update(three_material_labels=after['clips']==['red.mov','blue.mov','third.mov'],observed_each_action=sum(item['type']=='image' for item in stages[0]['output'])==3,
                              three_separate_tasks=len(final['task'])==3 and all(t['status']=='completed' for t in final['task']),
                              later_decisions=all(term in design_text for term in ('Main 不直接创建 Child','active/paused/completed/cancelled','Pause 可恢复')) and 'Main 可以直接 agent.run' not in design_text,
                              independent_design=all('# '+heading in design_text for heading in ('Decisions','Non-goals','Protocol','Milestones','Acceptance','Pending')),
                              exact_metrics=summary['value']['result']==dict(rows=416,communicated_clients=128,adopted_clients=16),
                              media_immutable=all(hashlib.sha256(open_bytes(h,m['artifact_ref'])).hexdigest()==m['content_hash'] for m in media))
                checks['real_video_metadata']=all(name+' 3s 320x180' in stages[0]['value']['state'] for name in ('red.mov','blue.mov','third.mov'))
                evidence.update(design=design,summary=summary,media=media,stages=stages,external_after=after,gap='desktop organization fragment; video edit/export quality is separate L2 evidence; semantic quality ungraded')
                shots=[item['data'] for record in stages for item in record['output'] if item['type']=='image']
            screenshots=[]
            for ref in shots:
                raw=open_bytes(h,ref)
                descriptor=h.command('artifact','describe',ref)
                require(raw.startswith(b'\x89PNG'),'real UI screenshot unreadable')
                screenshots.append(dict(artifact_ref=ref,sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw),descriptor=descriptor))
                require(descriptor['content_hash']==screenshots[-1]['sha256'],'screenshot hash changed after restart')
            checks['screenshots_reopen_after_restart']=bool(screenshots)
            final_state=state(h)
            decisions=[decision for record in evidence.values() if isinstance(record,dict) for decision in record.get('decisions',[])]
            decisions += [decision for record in evidence.get('stages',[]) for decision in record['decisions']]
            checks['exact_frozen_approvals']=bool(decisions) and all(d['approval']['arguments']==d['invocation']['arguments_json']==next(i['arguments_json'] for i in final_state['tools'] if i['invocation_id']==d['invocation']['invocation_id']) and d['invocation']['replay_policy']=='never' for d in decisions)
            evidence.update(screenshots=screenshots,state=final_state,computer=h.command('computer','status'))
            if artifact_folder:evidence['exported_artifacts']=export_artifacts(h,artifact_folder)
            checks['idle_checkpoint_released']=evidence['computer']['phase']=='checkpointed'
    evidence['cleanup']=True
    return checks,evidence
