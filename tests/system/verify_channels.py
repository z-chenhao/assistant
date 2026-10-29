#!/usr/bin/env python3
"""Launch/Doctor/Drive/Evidence/Cleanup for Web and Feishu through actual HTTP.

Uses the maintained MainHarness, production Actor, Tool/Approval/Delivery stores,
and independent Feishu HTTP Fake. All processes are foreground-owned and joined.
"""
import argparse
import base64
import socketserver
import threading
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import selectors
import sqlite3
import subprocess
import tempfile
import time
from urllib.error import HTTPError
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from feishu_sandbox import Sandbox
from event_broker import Broker
from fixture_pool import run_cases
from verify_main import MainHarness, require
from terminal_driver import Terminal

ROOT = Path(__file__).resolve().parents[2]
TOKEN = 'sandbox-web-owner-token-at-least-24-bytes'


class ChannelHarness(MainHarness):
    def __init__(self, binary, root, platform, fixture='main'):
        self.production_events = False
        super().__init__(binary, root, fixture)
        self.config = root / 'config.yaml'
        self.config.write_text('version: 1\nweb:\n  listen: "127.0.0.1:0"\n  token_ref: env:ERII_WEB_TEST_TOKEN\nfeishu:\n  app_id: sandbox-app\n  tenant_key: sandbox-tenant\n  verification_token_ref: env:ERII_FEISHU_VERIFY\n  encrypt_key_ref: env:ERII_FEISHU_ENCRYPT\n  access_token_ref: env:ERII_FEISHU_ACCESS\n  api_base: ' + platform.url + '\n')
        self.env.update(ERII_WEB_TEST_TOKEN=TOKEN, ERII_FEISHU_VERIFY='sandbox-verify-token',
                        ERII_FEISHU_ENCRYPT='sandbox-encrypt-key', ERII_FEISHU_ACCESS='sandbox-access-token')
        self.bind = self.command('channel','manage',text=json.dumps(dict(op='bind',channel='feishu',subject='sandbox-tenant:sandbox-app:owner',destination='bound-chat',enabled=True,version=0)))
        self.process = None

    def args(self, *args):
        return [str(self.binary), '-config', str(self.config), '-test-bridge', self.fixture, *(['-test-events'] if getattr(self, 'production_events', False) else []), *(['-test-automation-now',self.now] if hasattr(self,'now') else []), *args]

    def start(self, boundary=None):
        args = ('web','serve') if boundary is None else ('-test-main-hold',boundary,'web','serve')
        start=self.processes.start if self.processes is not None else subprocess.Popen
        self.process = start(self.args(*args), stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                        text=True, cwd=self.root, env=self.env)
        self.children.append(self.process)
        with selectors.DefaultSelector() as selector:
            selector.register(self.process.stdout,selectors.EVENT_READ)
            require(selector.select(15),'Web launch did not report listener')
        line=self.process.stdout.readline()
        require(line, 'Web launch exited: '+(self.process.stderr.read() if not line else ''))
        self.url=json.loads(line)['url']
        return self.process

    def stop(self, kill=False):
        if self.process and self.process.poll() is None:
            self.process.kill() if kill else self.process.terminate()
            _, err=self.process.communicate(timeout=15)
            require(kill or self.process.returncode==0, 'Web shutdown failed: '+err)
        self.process=None

    def http(self, path, data=None, method=None, auth=True, headers=None, expected=200):
        headers=dict(headers or {})
        if auth: headers['Authorization']='Bearer '+TOKEN
        if isinstance(data,dict):
            data=json.dumps(data).encode();headers['Content-Type']='application/json'
        req=Request(self.url+path,data=data,method=method,headers=headers)
        try:
            with urlopen(req,timeout=20) as response: status,raw=response.status,response.read()
        except HTTPError as error: status,raw=error.code,error.read()
        require(status==expected,f'{path}: expected {expected}, got {status}: {raw[:200]!r}')
        try: return json.loads(raw)
        except ValueError: return raw.decode()

    def callback(self, source, text, event=None, sender='owner', destination='bound-chat', expected=200, signature=True, kind='text'):
        content=dict(text=text) if kind=='text' else dict(file_key='file-key',file_name='received.txt')
        value=dict(schema='2.0',header=dict(event_id=event or source,event_type='im.message.receive_v1',token='sandbox-verify-token',app_id='sandbox-app',tenant_key='sandbox-tenant'),event=dict(sender=dict(sender_type='user',sender_id=dict(open_id=sender)),message=dict(message_id=source,chat_id=destination,chat_type='p2p',message_type=kind,content=json.dumps(content))))
        raw=json.dumps(value,separators=(',',':')).encode();timestamp=str(int(time.time()));nonce='sandbox-nonce'
        digest=hashlib.sha256(timestamp.encode()+nonce.encode()+b'sandbox-encrypt-key'+raw).hexdigest()
        return self.http('/feishu/callback',raw,auth=False,headers={'X-Lark-Request-Timestamp':timestamp,'X-Lark-Request-Nonce':nonce,'X-Lark-Signature':digest if signature else 'forged'},expected=expected)

    def rows(self):
        with sqlite3.connect(self.root/'data/state.db') as db:
            db.row_factory=sqlite3.Row
            return {name:[dict(r) for r in db.execute(query)] for name,query in dict(
                inputs='SELECT request_key,message_id,run_id FROM main_input ORDER BY sequence',
                callbacks='SELECT * FROM channel_callback ORDER BY event_id',
                deliveries='SELECT delivery_id,message_id,channel,destination_ref,status,attempt_count,version FROM delivery_request ORDER BY created_at,delivery_id',
                receipts='SELECT * FROM channel_receipt ORDER BY delivery_id,part',
                approvals='SELECT approval_id,frozen_arguments_hash,status FROM approval ORDER BY created_at',
                tools='SELECT tool_name,arguments_json,status,attempt_count FROM tool_invocation ORDER BY created_at',
                runs='SELECT run_id,kind,worker_position,status FROM run ORDER BY created_at',
                resources='SELECT * FROM channel_resource',
                projections='SELECT * FROM delivery_projection').items()}

    def wait(self,predicate,description):
        return self.wait_state(predicate,description,lambda _:self.rows(),timeout=20)

    def browser(self,evidence,phase):
        env=self.env.copy()
        executable=Path('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome')
        if executable.exists():env['ERII_BROWSER_EXECUTABLE']=str(executable)
        run=self.processes.run if self.processes is not None else subprocess.run
        result=run(['node',str(ROOT/'tests/system/channels_browser.mjs'),self.url,str(evidence),phase],env=env,capture_output=True,text=True,timeout=45)
        require(result.returncode==0,'browser drive failed: '+result.stderr)


def run(binary,evidence):
    report=dict(checks=[],scenarios={})
    with tempfile.TemporaryDirectory(prefix='erii-channels-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform)
        try:
            h.start()
            require('Welcome to Erii' in h.http('/',auth=False),'login page missing')
            h.http('/api/conversation',auth=False,expected=401)
            h.http('/api/message',dict(source_id='x',content=[dict(type='text',text='x')]),headers={'Origin':'http://evil.example'},expected=403)
            h.http('/api/progress',data=b'x',method='POST',expected=404)
            h.callback('forged','bad',signature=False,expected=401)
            h.callback('unbound','bad',sender='other',expected=403)
            h.callback('recipient','bad',destination='other',expected=403)
            h.browser(evidence.parent/'channels-ui','setup')
            h.wait(lambda s:any(d['status']=='sent' and d['channel']=='web' for d in s['deliveries']),'browser setup and first reply')
            report['checks'].append('Launch/Doctor: real loopback Web setup and file attachment through browser; explicit model Fake; auth/CSRF/signature/identity/recipient rejection')
            # Duplicate callback IDs and new callback IDs carrying the same source
            # message are distinct retry patterns, both deduplicated durably.
            with ThreadPoolExecutor(max_workers=8) as pool:
                list(pool.map(lambda i:h.callback('feishu-source','Continue the same plan in Feishu.',event='feishu-event-'+str(i%3)),range(12)))
            settled=h.wait(lambda s:len(s['inputs'])==2 and len(s['deliveries'])==2 and all(d['status']=='sent' for d in s['deliveries']),'one Feishu input and notice')
            h.callback('feishu-source','parameter drift',event='different-retry',expected=409)
            history=h.http('/api/conversation')
            require(len(history)==4 and [m['source'] for m in history if m['role']=='user']==['web','feishu'],'shared Conversation order')
            require([m['sequence'] for m in history]==list(range(1,5)),'nonmonotonic Conversation')
            require(len(platform.evidence()['messages'])==1 and len(platform.evidence()['send_attempts'])==1,'duplicate remote send attempt or notification')
            h.stop();h.start()
            h.callback('feishu-source','Continue the same plan in Feishu.',event='after-restart')
            require(len(h.rows()['inputs'])==2 and len(platform.evidence()['messages'])==1,'restart duplicated accepted callback/delivery')
            h.browser(evidence.parent/'channels-ui','conversation')
            with Terminal(h.args('tui'),h.env,h.root) as terminal:
                terminal.expect('Erii: idle');terminal.send('/history')
                terminal.expect('Processed: Continue the same plan in Feishu.')
                require(terminal.stop()==0,'shared-history TUI did not stop')
                report['scenarios']['tui-history']=terminal.transcript()
            report['scenarios']['shared-conversation']=dict(state=h.rows(),conversation=history,platform=platform.evidence())
            report['checks'].append('Drive/Evidence: Web and signed Feishu callbacks share ordered Conversation; concurrent event/source retries, drift rejection and restart produce one remote reply')
            # Incoming attachment import is stable across independent callback IDs.
            h.callback('media-message','',kind='file')
            h.wait(lambda s:len(s['inputs'])==3 and len(s['deliveries'])==3 and all(d['status']=='sent' for d in s['deliveries']),'attachment reply')
            h.callback('media-message','',kind='file',event='media-retry')
            state=h.rows();require(len(state['resources'])==2 and len(state['inputs'])==3,'attachment duplicate changed reference')
            attached=[p['artifact_ref'] for m in h.http('/api/conversation') if m['source']=='feishu' for p in m['content'] if p.get('artifact_ref')]
            require(len(attached)==1,'missing incoming Artifact reference')
            downloaded=h.http('/artifact?'+urlencode(dict(ref=attached[0])))
            require(downloaded=='Inbound attachment from the authenticated Feishu owner.\n','wrong attachment bytes')
            h.http('/artifact?ref=artifact:other',expected=404)
            report['scenarios']['attachment']=dict(state=state,artifact=attached[0],sha256=hashlib.sha256(downloaded.encode()).hexdigest())
            report['checks'].append('Verified Feishu attachment imports once, opens through authenticated Web boundary, and rejects unknown references')
            h.stop()
        finally:h.cleanup()

    # F-019: keep a real browser draft and attachment across committed replies,
    # approval decisions, Task status, network loss and session rotation.
    with tempfile.TemporaryDirectory(prefix='erii-channel-live-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform,'capabilities')
        try:
            h.setup();h.start()
            h.browser(evidence.parent/'channels-live-ui','live')
            state=h.rows()
            require(any(a['status']=='denied' for a in state['approvals']), 'live UI did not persist exact approval rejection')
            require(not (h.root/'data.computer/unapproved.txt').exists(), 'rejected live action executed')
            report['scenarios']['live-updates']=state
            report['checks'].append('Live Web: committed replies/approvals/progress update while draft, file, focus, selection, scroll and request identity survive; network/session recovery without reload')
        finally:h.cleanup()

    inspection_relationships(binary,evidence,report)

    # Approval recovery uses the same external-effect Fake and persistent Security
    # lifecycle as TUI. Callback races cross two authenticated transports.
    with tempfile.TemporaryDirectory(prefix='erii-channel-approval-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform,'approval')
        try:
            h.setup();h.start()
            h.callback('effect-source',json.dumps(dict(target='alex.chen@example.test',text='Erii review: 2026-10-19 15:00–15:30 +08:00')))
            state=h.wait(lambda s:len(s['approvals'])==1 and s['approvals'][0]['status']=='pending','approval notice')
            old=state['approvals'][0]
            h.browser(evidence.parent/'channels-ui','approval')
            h.http('/api/message',dict(source_id='steering',content=[dict(type='text',text=json.dumps(dict(target='alex.chen@example.test',text='Erii review: 2026-10-20 11:00–11:30 +08:00; do not use Monday.')))]))
            state=h.wait(lambda s:len(s['approvals'])==2 and s['approvals'][-1]['status']=='pending','steering invalidates old approval')
            require(state['approvals'][0]['status']=='invalidated','old Monday approval remained valid')
            approval=state['approvals'][-1]
            h.callback('old-approve','/approve '+old['approval_id']+' '+old['frozen_arguments_hash'],expected=409)
            h.stop(kill=True);h.start()
            h.http('/approval',data=urlencode(dict(approval_id=approval['approval_id'],hash='drift',decision='approved')).encode(),headers={'Content-Type':'application/x-www-form-urlencoded'},expected=409)
            command='/approve '+approval['approval_id']+' '+approval['frozen_arguments_hash']
            with ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(lambda i:h.callback('approve-'+str(i),command),range(6)))
            state=h.wait(lambda s:any(t['tool_name']=='mcp__fixture__send' and t['status']=='succeeded' for t in s['tools']) and len(s['deliveries'])==3 and all(d['status']=='sent' for d in s['deliveries']),'approved effect once')
            effects=[t for t in state['tools'] if t['tool_name']=='mcp__fixture__send' and t['status']=='succeeded']
            require(len(effects)==1 and effects[0]['attempt_count']==1,'duplicate approved effect')
            frozen=json.loads(effects[0]['arguments_json'])
            require(frozen['target']=='alex.chen@example.test' and '2026-10-20 11:00' in frozen['text'],'wrong final recipient or date')
            with sqlite3.connect(h.root/'data/effect-ledger.db') as db:
                ledger=[dict(zip(('key','args','result'),r)) for r in db.execute('SELECT * FROM effects')]
            require(len(ledger)==1 and json.loads(ledger[0]['args'])==frozen,'external ledger disagrees with approved action')
            report['scenarios']['approval-restart']=dict(state=state,platform=platform.evidence(),effect_ledger=ledger)
            report['checks'].append('GC-12 channel/Security fragment; Approval: exact frozen-hash rejection, SIGKILL/restart, concurrent duplicate callbacks; one approved Tool dispatch')
            h.stop()
        finally:h.cleanup()
    dependency_recovery(binary,report)
    extra_scenarios(binary,report)
    native_components(binary, evidence, report)
    delivery_leases(binary, report)
    account_presentation(binary,report)
    native_trace_inspection(binary,evidence,report)
    report['checks'].append('Cleanup: Web/Bridge/browser children joined, sandbox HTTP servers stopped, disposable roots removed')
    return report


def native_trace_inspection(binary,evidence,report):
    """Actual Pi/HTTP retries, durable result reuse, and the tracing UI."""
    from http.server import ThreadingHTTPServer
    from verify_local_provider import Endpoint
    class TraceEndpoint(Endpoint):
        def do_POST(self):
            time.sleep(.04)
            if getattr(self.server,'reject_next',False):
                self.server.reject_next=False
                body=json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                self.server.requests.append(dict(method='POST',auth=self.headers.get('Authorization'),session=self.headers.get('session_id'),body=body))
                self.send_response(503);self.send_header('Content-Length','0');self.end_headers();return
            return super().do_POST()
    with tempfile.TemporaryDirectory(prefix='erii-native-traces-') as temp, Sandbox() as platform, Broker(Path(temp)) as broker:
        server=ThreadingHTTPServer(('127.0.0.1',0),TraceEndpoint);server.daemon_threads=True
        server.requests=[];server.mode='success'
        thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        h=ChannelHarness(binary,Path(temp),platform,'')
        try:
            h.config.write_text(h.config.read_text()+'llm:\n  provider_bridge_command: node\n  provider_bridge_script: '+json.dumps(str(ROOT/'provider-bridge/bridge.mjs'))+'\n  provider_bridge_version: pi-ai-1.3.0-v1\nnats:\n  url: '+json.dumps(broker.url)+'\n')
            h.env['ERII_TRACE_KEY']='SYNTHETIC_TRACE_CREDENTIAL_NEVER_STORE'
            auth=h.command('model','login','ERII_TRACE_KEY')
            h.setup(metadata=dict(name='Trace HTTP fixture',provider_kind='openai_compatible',endpoint=f'http://127.0.0.1:{server.server_port}/v1',wire_api='openai_completions',model_id='fixture-model',context_window=65536,max_output_tokens=4096,generation_defaults={},compatibility=dict(tool_result_format='openai'),capabilities=dict(text_input=True),auth_connection_id=auth['connection_id']))
            server.mode='tasks';server.reject_next=True
            h.start()
            h.wait_state(lambda status:status['readiness']=='ready','native trace runtime readiness',lambda _:h.http('/api/status'))
            h.http('/api/message',dict(source_id='native-trace',content=[dict(type='text',text='Compare the available evidence and prepare an independent report. Keep the original sources and return a concise summary.')]))
            h.wait(lambda state:any(r['worker_position']=='root' for r in state['runs']) and all(r['status'] in ('completed','failed','cancelled') for r in state['runs']) and all(d['status']=='sent' for d in state['deliveries']),'native trace completion')
            capture_state={}
            def captured(_):
                with sqlite3.connect(h.root/'data/state.db') as db:
                    runs={r[0] for r in db.execute('SELECT run_id FROM run')}
                    physical=[r for r in server.requests if r.get('session') in runs and r['method']=='POST']
                    capture_state.update(physical=len(physical),phases=db.execute('SELECT phase,count(*) FROM dispatch_observation GROUP BY phase').fetchall(),generations=db.execute('SELECT status,count(*) FROM model_generation GROUP BY status').fetchall())
                    return db.execute("SELECT count(*) FROM dispatch_observation WHERE phase='headers'").fetchone()[0]==len(physical)
            try:
                h.wait_state(captured,'all terminal HTTP observations persisted',timeout=10)
            except RuntimeError as error:
                raise RuntimeError(str(error)+'; '+json.dumps(capture_state)) from error
            with sqlite3.connect(h.root/'data/state.db') as db:
                generation,main=db.execute('SELECT generation_id,run_id FROM model_generation ORDER BY created_at LIMIT 1').fetchone()
                before=[r['body'] for r in server.requests if r.get('session')==main and r['method']=='POST'][:2]
                require(len(before)==2 and before[0]==before[1],'actual HTTP retry was not exercised')
                root=db.execute("SELECT run_id FROM run WHERE worker_position='root'").fetchone()[0]
                attempts=db.execute("SELECT body_json FROM dispatch_observation WHERE generation_id=? AND phase='started' ORDER BY occurred_at",(generation,)).fetchall()
                require(len(attempts)==2,'result reuse fabricated a new HTTP attempt')
                require([json.loads(a[0]) for a in attempts]==before,'observed final request differs from actual HTTP body')
                raw=''.join(r[0] or '' for r in db.execute('SELECT body_json FROM dispatch_observation'))
                require(h.env['ERII_TRACE_KEY'] not in raw and 'Bearer ' not in raw,'wire capture persisted credentials')
                facts=dict(main=main,root=root,generation=generation,counts={table:db.execute('SELECT count(*) FROM '+table).fetchone()[0] for table in ('model_generation','provider_usage','tool_invocation','dispatch_observation','run_trace')})
            tree=h.http('/api/developer/observations?'+urlencode(dict(run=main)))
            require(sum(o['name']=='Provider HTTP' for o in tree['items'])>=2,'actual sends missing from observation tree')
            h.stop(kill=True);h.start()
            require(h.http('/api/developer/observations?'+urlencode(dict(run=main)))==tree,'completed trace changed across restart')
            h.env['ERII_NATIVE_TRACE_FACTS']=json.dumps(facts)
            h.browser(evidence.parent/'trace-ui','native-trace')
            with sqlite3.connect(h.root/'data/state.db') as db:
                after={table:db.execute('SELECT count(*) FROM '+table).fetchone()[0] for table in facts['counts']}
            require(after==facts['counts'],'browsing tracing mutated execution or observations')
            report['scenarios']['native-traces']=dict(read_only=True,actual_attempts=2,restart=True)
            report['checks'].append('Langfuse-style trace tree/timeline/details; actual Pi final HTTP bodies and retries, terminal restart without re-execution, missing usage/pricing, authenticated browser, responsive/no-JS and unchanged ledgers')
        finally:
            h.cleanup();server.shutdown();server.server_close();thread.join()


def inspection_relationships(binary, evidence, report):
    """Real Main -> Task -> Root -> recursive Child, read through Web only."""
    def plan(calls): return 'Capabilities: '+json.dumps(calls)
    leaf=plan([dict(name='mcp__fixture__echo',arguments=dict(text='Inspection leaf completed.'))])
    child=plan([dict(name='builtin__agent_run',arguments=dict(task=leaf))])
    root='Inspection Task <img src=x onerror="unsafe()">\n'+plan([dict(name='builtin__agent_run',arguments=dict(task=child))])
    with tempfile.TemporaryDirectory(prefix='erii-inspection-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform,'capabilities')
        try:
            h.setup(metadata=dict(name='inspection fixture',provider_kind='openai_compatible',
                endpoint='http://127.0.0.1:1/v1',wire_api='openai_completions',model_id='fixture-model',
                context_window=65536,max_output_tokens=4096,generation_defaults={},
                compatibility=dict(tool_result_format='openai'),capabilities=dict(text_input=True)))
            h.start('after_generation_commit')
            h.wait_state(lambda status:status['readiness']=='ready','inspection runtime readiness',lambda _:h.http('/api/status'))
            h.http('/api/message',dict(source_id='inspect-tree',content=[dict(type='text',text=plan([
                dict(name='builtin__task_manage',arguments=dict(operation='create',task=root))]))]))
            def first_accepted(_):
                with sqlite3.connect(h.root/'data/state.db') as db:
                    return db.execute("SELECT count(*) FROM model_generation WHERE status='completed'").fetchone()[0]==1
            h.wait_state(first_accepted,'accepted model result before checkpoint',timeout=10)
            h.stop(kill=True);h.start()
            h.wait(lambda s:sum(r['worker_position']=='child' for r in s['runs'])==2
                   and all(r['status'] in ('completed','failed','cancelled') for r in s['runs'])
                   and all(d['status']=='sent' for d in s['deliveries']), 'completed inspection tree')
            # A real long user turn proves continuation through the actual Web
            # surface; no synthetic internal acceptance or checkpoint rewrite.
            h.http('/api/message',dict(source_id='inspect-long-context',content=[dict(type='text',text='Capabilities: []\n'+'完整上下文🦊 '*1000+'<img src=x onerror="unsafe()">LONG_CONTEXT_END')]))
            h.wait(lambda s:all(r['status'] in ('completed','failed','cancelled') for r in s['runs'])
                   and all(d['status']=='sent' for d in s['deliveries']), 'completed long context turn')
            def facts():
                with sqlite3.connect(h.root/'data/state.db') as db:
                    return dict(runs=[dict(zip(('id','role','parent','root'),row)) for row in db.execute(
                        "SELECT run_id,CASE WHEN kind='main' THEN 'main' ELSE worker_position END,coalesce(parent_run_id,''),coalesce(root_run_id,'') FROM run ORDER BY created_at,run_id")],
                        task=db.execute('SELECT task_ref FROM task').fetchone()[0],
                        long_context=dict(zip(('run','step','item'),db.execute("SELECT g.run_id,g.step_index,cast(j.key AS integer) FROM model_generation g,json_each(g.request_json,'$.messages') j WHERE json_extract(j.value,'$.text') LIKE '%LONG_CONTEXT_END%' ORDER BY g.created_at DESC LIMIT 1").fetchone())),
                        frozen_hash=hashlib.sha256(json.dumps(db.execute('SELECT generation_id,request_hash,request_json,result_json FROM model_generation ORDER BY generation_id').fetchall()).encode()).hexdigest(),
                        main=db.execute("SELECT i.run_id FROM task_request q JOIN tool_invocation i USING(invocation_id) WHERE json_extract(i.arguments_json,'$.operation')='create'").fetchone()[0],
                        counts={table:db.execute('SELECT count(*) FROM '+table).fetchone()[0] for table in
                                ('run','task','tool_invocation','task_request','task_invocation','model_generation','run_trace','event_outbox')})
            original=facts()
            h.http('/developer/observability',auth=False,expected=401)
            h.http('/api/developer/executions',auth=False,expected=401)
            h.http('/api/developer/executions?limit=101',expected=400)
            h.http('/api/developer/relationships?run=missing',expected=404)
            h.http('/api/developer/relationships?task=task:missing',expected=404)
            ids=[];cursor=''
            for _ in range(len(original['runs'])+1):
                page=h.http('/api/developer/executions?'+urlencode(dict(limit=1,cursor=cursor)))
                ids.extend(r['run_id'] for r in page['items']);cursor=page.get('next','')
                if not cursor:break
            require(len(ids)==len(set(ids))==len(original['runs']) and set(ids)=={r['id'] for r in original['runs']},'inspection pagination omitted or duplicated an execution')
            with sqlite3.connect(h.root/'data/state.db') as db:
                first=db.execute('SELECT generation_id FROM model_generation WHERE run_id=? ORDER BY step_index LIMIT 1',(original['main'],)).fetchone()[0]
            audited=[json.loads(line) for line in (h.root/'data/fake-provider-calls.jsonl').read_text().splitlines()]
            require(sum(row['generation_id']==first for row in audited)==1,'accepted model result dispatched twice after recovery')
            trace=h.http('/api/developer/observations?'+urlencode(dict(run=original['main'])))
            require(any(node['status']=='reused' for node in trace['items']),'accepted result reuse missing from trace')
            require(not any(node['name']=='Provider HTTP' for node in trace['items']),'scripted provider fabricated HTTP attempts')
            source=h.http('/api/developer/relationships?'+urlencode(dict(run=original['main'])))
            require(len(source['tasks'])==1 and source['tasks'][0]['task_ref']==original['task'] and source['tasks'][0]['operation']=='create','Main creation receipt missing')
            tree=h.http('/api/developer/relationships?'+urlencode(dict(task=original['task'])))
            actual={r['run_id']:(r['role'],r.get('parent_run_id',''),r.get('root_run_id','')) for r in tree['executions']}
            require(actual=={r['id']:(r['role'],r['parent'],r['root']) for r in original['runs'] if r['role']!='main'},'inspection tree differs from authoritative Run links')
            require(tree['creator']['run_id']==original['main'],'Task origin lost')
            require(all(parent!=original['main'] for _,parent,_ in actual.values()),'Root mislabeled as Main Child')
            inspect_frozen_context(h)
            h.env['ERII_INSPECTION_FACTS']=json.dumps(original)
            h.browser(evidence.parent/'inspection-ui','inspection')
            require(facts()==original,'opening inspection changed execution or domain records')
            h.stop(kill=True);h.start()
            recovered=h.http('/api/developer/relationships?'+urlencode(dict(task=original['task'])))
            require(recovered==tree and facts()==original,'inspection restart rewrote or reran completed work')
            inspect_frozen_context(h)
            report['scenarios']['inspection']=dict(source=source,tree=tree,paginated_ids=ids,read_only=True,restart=True)
            report['checks'].append('Developer Observability: actual Main/Task/Root/recursive Child, paged metadata, original receipt causality, browser filters/navigation/escaping/mobile, read-only restart')
            report['checks'].append('Frozen context: ordered stored messages/tools/responses, recorded provenance, adjacent input diff, complete UTF-8 content, authenticated browser and restart without re-execution')
        finally:h.cleanup()


def inspect_frozen_context(h):
    """Compare the HTTP projection with the actual frozen Host records."""
    def content(run,step,section,index):
        parts=[];offset=0
        for _ in range(1000):
            view=h.http('/api/developer/context?'+urlencode(dict(run=run,step=step,section=section,item=index,offset=offset,format='json')))
            require(len(view['content'].encode())<=8192,'unbounded context part')
            parts.append(view['content'])
            if 'next_offset' not in view:return json.loads(''.join(parts))
            require(view['next_offset']>offset,'context continuation did not advance')
            offset=view['next_offset']
        raise AssertionError('context continuation never completed')
    h.http('/api/developer/rounds?run=missing',auth=False,expected=401)
    h.http('/api/developer/context?run=missing&step=0',expected=404)
    h.http('/api/developer/context?run=missing&step=-1',expected=400)
    with sqlite3.connect(h.root/'data/state.db') as db:
        frozen=db.execute('SELECT run_id,step_index,request_json,result_json,status FROM model_generation ORDER BY run_id,step_index').fetchall()
        for run in {row[0] for row in frozen}:
            rounds=h.http('/api/developer/rounds?'+urlencode(dict(run=run)))
            require([r['step'] for r in rounds['items']]==[row[1] for row in frozen if row[0]==run],'round order differs from frozen history')
        for run,step,raw,result,status in frozen:
            request=json.loads(raw)
            for section in ('messages','tools'):
                items=request.get(section,[])
                if not items:continue
                view=h.http('/api/developer/context?'+urlencode(dict(run=run,step=step,section=section)))
                require([item['index'] for item in view['items']]==list(range(len(items))),'context order differs from snapshot')
                for index,expected in enumerate(items):
                    require(content(run,step,section,index)==expected,'HTTP inspection changed frozen '+section)
                diff=h.http('/api/developer/context-diff?'+urlencode(dict(run=run,step=step,section=section)))
                require(all(change['kind'] in ('added','removed','changed','moved') for change in diff['items']),'unknown diff semantics')
            if status=='completed':require(content(run,step,'response',0)==json.loads(result),'response or tool calls differ from accepted result')
            provenance=db.execute('SELECT provenance_json FROM context_compaction WHERE run_id=? AND step_index=?',(run,step)).fetchone()
            if provenance:require(content(run,step,'provenance',0)==json.loads(provenance[0]),'compaction provenance rewritten')


def dependency_recovery(binary, report):
    with tempfile.TemporaryDirectory(prefix='erii-channel-repair-') as temp, Sandbox() as platform, Broker(Path(temp)) as broker:
        h = ChannelHarness(binary, Path(temp), platform)
        try:
            h.setup()
            h.production_events = True
            h.env['ERII_NATS_URL'] = broker.url
            catalog = h.root / 'repaired-skills'
            with h.config.open('a') as config:
                config.write('skills:\n  root: ' + str(catalog) + '\n  scope: agent\n')
            h.start()
            require('Model connection' in h.http('/settings'), 'dependency failure hid Settings')
            require('Your ongoing work' in h.http('/progress'), 'dependency failure hid Progress')
            require(h.http('/api/status')['readiness'] != 'ready', 'broken runtime admitted work')
            h.http('/api/message', dict(source_id='blocked', content=[dict(type='text', text='blocked')]), expected=409)
            catalog.mkdir(mode=0o700)
            h.wait_state(lambda status: status['readiness'] == 'ready', 'dependency repair reopened ingress', reader=lambda _: h.http('/api/status'))
            h.http('/api/message', dict(source_id='repaired', content=[dict(type='text', text='Continue after repair.')]))
            h.wait(lambda state: any(row['status'] == 'sent' for row in state['deliveries']), 'repaired runtime reply')
            # Repeated UI reads must not create traces, projections or intents.
            h.wait(lambda state: all(row['status'] in ('completed', 'failed') for row in state['runs']), 'settled reads')
            before = h.rows()
            for path in ('/settings', '/progress', '/', '/api/conversation'):
                h.http(path)
            after = h.rows()
            require(before == after, 'GET mutated authoritative state')
            report['scenarios']['dependency-repair'] = dict(status='passed', state=after)
            report['checks'].append('Unavailable optional dependency keeps control pages usable; in-process repair opens ingress; GET leaves settled state unchanged')
            h.stop()
        finally:
            h.cleanup()


def native_components(binary, evidence, report):
    """Seed immutable controlled projections before the first transport call.

    Semantic inputs still enter the real signed callback/Actor boundary. Only
    Host-owned presentation fixtures are inserted; no model HTML is enabled.
    """
    with tempfile.TemporaryDirectory(prefix='erii-channel-components-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform)
        try:
            h.setup();h.start('before_delivery_send')
            h.callback('native-table','Compare these two choices.')
            with selectors.DefaultSelector() as selector:
                selector.register(h.process.stdout,selectors.EVENT_READ)
                require(selector.select(15),'native component fixture did not reach transport boundary')
            require(json.loads(h.process.stdout.readline())['boundary']=='before_delivery_send','wrong component fixture boundary')
            h.callback('unknown-component','Retain this semantic answer.')
            h.wait(lambda s:len(s['deliveries'])==2,'unknown component semantic reply')
            h.callback('invalid-table','Retain this answer when table rendering fails.')
            h.wait(lambda s:len(s['deliveries'])==3,'component fixture semantic replies')
            h.callback('empty-projection','Keep the original text for an empty projection.')
            h.wait(lambda s:len(s['deliveries'])==4,'empty projection semantic reply')
            h.callback('unknown-version','Keep the original text for an unknown version.')
            h.wait(lambda s:len(s['deliveries'])==5,'unknown version semantic reply')
            messages=[m for m in h.http('/api/conversation') if m['role']=='assistant']
            specs=[
                dict(schema_version=1,fallback='Native table semantic fallback',components=[dict(type='table',columns=['Option','Result'],rows=[['A','<img src=x onerror=alert(1)>'],['B','Exact value: 42']])]),
                dict(schema_version=1,fallback='Unknown component semantic fallback',components=[dict(type='react',text='<script>alert(1)</script>')]),
                dict(schema_version=1,fallback='Invalid table semantic fallback',components=[dict(type='table',columns=['One'],rows=[['too','many']])]),
                dict(schema_version=1,fallback='Untrusted projection fallback',components=[]),
                dict(schema_version=99,fallback='Untrusted projection fallback',components=[]),
            ]
            with sqlite3.connect(h.root/'data/state.db') as db:
                for message,spec in zip(messages,specs):
                    canonical=json.dumps(message,sort_keys=True,separators=(',',':'),ensure_ascii=False).replace('&','\\u0026').replace('<','\\u003c').replace('>','\\u003e')
                    identity=hashlib.sha256(canonical.encode()).hexdigest()
                    delivery_id=db.execute('SELECT delivery_id FROM delivery_request WHERE message_id=?',(message['message_id'],)).fetchone()[0]
                    db.execute('INSERT INTO delivery_projection VALUES(?,?,?,?)',(delivery_id,identity,1,json.dumps(spec)))
            h.browser(evidence.parent/'channels-ui','components')
            require(h.http('/api/conversation')[-1]==messages[-1],'presentation changed semantic Conversation')
            report['scenarios']['native-components']=dict(state=h.rows(),specs=specs,conversation_unchanged=True)
            report['checks'].append('UI: bundled ad-agent components, OpenIntelligentUI native table, original Eri logo/avatar, desktop/mobile layouts; escaped cells, unknown/invalid component fallback; no external frontend requests')
            h.stop()
        finally:h.cleanup()


def early_composer(binary, evidence, report):
    with tempfile.TemporaryDirectory(prefix='erii-channel-early-composer-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform)
        try:
            h.start()
            h.browser(evidence.parent/'channels-ui','setup')
            state=h.wait(lambda s:len(s['inputs'])==1 and len(s['deliveries'])==1 and s['deliveries'][0]['status']=='sent','early draft accepted once with attachment')
            history=h.http('/api/conversation')
            require(any(p.get('artifact_ref') for m in history if m['role']=='user' for p in m['content']),'early attachment did not reach Conversation')
            report['scenarios']['early-composer']=dict(state=state,conversation=history)
            report['checks'].append('Composer: delayed local bundle preserves early server-rendered draft and attachment, then submits through the real form once')
            h.stop()
        finally:h.cleanup()


def delivery_leases(binary, report):
    """Exercise the same foreground delivery path beyond the default 5s lease."""
    for name, fixture, send_delay, upload_delay in (
            ('slow-send', 'main', 7, 0),
            ('slow-upload', 'artifacts', 0, 6),
            ('cumulative-multipart', 'artifacts', 2, 2)):
        with tempfile.TemporaryDirectory(prefix='erii-channel-lease-') as temp, Sandbox(send_delay, upload_delay) as platform:
            h = ChannelHarness(binary, Path(temp), platform, fixture)
            try:
                h.setup(); h.start()
                content = 'Slow delivery must retain its original fenced owner.'
                text = content if fixture == 'main' else json.dumps(dict(name='slow.txt', mediaType='text/plain', text=content))
                started = time.monotonic()
                h.callback(name, text)
                require(platform.send_started.wait(10), 'slow delivery did not reach platform')
                competitor = None
                if name == 'slow-send':
                    with sqlite3.connect(h.root/'data/state.db') as db:
                        # Renewal leaves updated_at/version unchanged. Derive
                        # the original expiry even if a heartbeat already ran.
                        initial_expiry, token = db.execute('SELECT d.updated_at+5000000,o.claim_token FROM delivery_outbox o JOIN delivery_request d USING(delivery_id)').fetchone()
                    h.wait(lambda _: time.time() > initial_expiry/1_000_000 + .1, 'past original claim expiry')
                    require(platform.evidence()['operations'][0]['finished_at'] is None, 'slow send finished before competing scan')
                    h.command('delivery', 'work')
                    with sqlite3.connect(h.root/'data/state.db') as db:
                        expiry, current = db.execute('SELECT claim_expires_at,claim_token FROM delivery_outbox').fetchone()
                    require(current == token and expiry > initial_expiry, 'competing consumer took a live renewed claim')
                    competitor = dict(same_owner=True, initial_expiry=initial_expiry, renewed_expiry=expiry)
                state = h.wait(lambda s: len(s['deliveries']) == 1 and s['deliveries'][0]['status'] == 'sent', name+' settled')
                elapsed = time.monotonic() - started
                ledger = platform.evidence()
                expected = 1 if fixture == 'main' else 2
                require(elapsed > 5 and len(state['receipts']) == expected and len(ledger['send_attempts']) == expected, 'slow delivery lost or repeated a part')
                require(state['deliveries'][0]['attempt_count'] == 1, 'slow send became another attempt')
                require(h.process.poll() is None and h.http('/api/status')['readiness'] == 'ready', 'slow delivery stopped ingress')
                if fixture == 'artifacts':
                    require(len(ledger['uploads']) == 1 and ledger['uploads'][0]['name'] == 'slow.txt' and ledger['uploads'][0]['sha256'] == hashlib.sha256(content.encode()).hexdigest(), 'slow multipart changed attachment')
                report['scenarios'][name] = dict(state=state, platform=ledger, elapsed_seconds=elapsed, competing_scan=competitor)
                h.stop()
            finally:
                h.cleanup()

    # Expire a live sender after the platform recorded the effect, then use the
    # real CLI as a competing recovery owner. The old HTTP call must cancel and
    # join without taking down Web ingress; uncertainty is still query-only.
    with tempfile.TemporaryDirectory(prefix='erii-channel-lost-claim-') as temp, Sandbox(send_delay=6) as platform:
        h = ChannelHarness(binary, Path(temp), platform)
        try:
            h.setup(); h.start()
            h.callback('lost-claim', 'Keep ingress alive if this transport loses its claim.')
            require(platform.send_started.wait(10), 'lost-claim send did not reach platform')
            with sqlite3.connect(h.root/'data/state.db') as db:
                db.execute('UPDATE delivery_outbox SET claim_expires_at=0')
            h.command('delivery', 'work')
            h.wait(lambda s: s['deliveries'][0]['status'] == 'unknown', 'competing recovery query')
            h.http('/api/message', dict(source_id='after-lost-claim', content=[dict(type='text', text='Web still accepts unrelated conversation input.')]))
            state = h.wait(lambda s: len(s['deliveries']) == 2 and any(d['channel'] == 'web' and d['status'] == 'sent' for d in s['deliveries']), 'ingress and delivery survive ownership loss')
            h.wait(lambda _: platform.evidence()['operations'][0]['finished_at'] is not None, 'late platform response finished')
            h.stop(kill=True)
            prior = next(d for d in h.rows()['deliveries'] if d['channel'] == 'feishu')['version']
            h.start()
            state = h.wait(lambda s: any(d['channel'] == 'feishu' and d['status'] == 'unknown' and d['version'] >= prior+2 for d in s['deliveries']), 'unknown query settled after restart')
            ledger = platform.evidence()
            require(len(ledger['send_attempts']) == 1 and len(ledger['messages']) == 1, 'competing recovery resent unknown side effect')
            feishu_id = next(d for d in state['deliveries'] if d['channel'] == 'feishu')['delivery_id']
            require(not any(r['delivery_id'] == feishu_id for r in state['receipts']), 'lost sender committed a late channel receipt')
            require(h.process.poll() is None and h.http('/api/status')['readiness'] == 'ready', 'late sender or recovery stopped ingress')
            report['scenarios']['lost-claim-competing-recovery'] = dict(state=state, platform=ledger)
            h.stop()
        finally:
            h.cleanup()
    report['checks'].append('Delivery lease: slow send, slow upload and cumulative multipart settle under one renewed claim; competing live scan excluded; lost owner cancels without stopping ingress, restart keeps unknown query-only with one remote send')


def extra_scenarios(binary,report):
    # Outbound attachment: semantic Artifact -> same DeliveryRequest -> real
    # multipart upload + bound-recipient platform message -> durable receipts.
    for lost_ack in (False, True):
        with tempfile.TemporaryDirectory(prefix='erii-channel-output-') as temp, Sandbox() as platform:
            h=ChannelHarness(binary,Path(temp),platform,'artifacts')
            try:
                h.setup();platform.drop_response=lost_ack;h.start()
                content='Final wedding checklist: 128 guests; confirm seats per table.'
                h.callback('output',json.dumps(dict(name='checklist.txt',mediaType='text/plain',text=content)))
                status='unknown' if lost_ack else 'sent'
                state=h.wait(lambda s:len(s['deliveries'])==1 and s['deliveries'][0]['status']==status,'outbound attachment receipt')
                if not lost_ack:
                    uploads=platform.evidence()['uploads']
                    require(len(uploads)==1 and uploads[0]['name']=='checklist.txt' and uploads[0]['sha256']==hashlib.sha256(content.encode()).hexdigest(),'wrong outbound attachment')
                    require(len(platform.evidence()['messages'])==2 and len(state['receipts'])==2,'attachment parts lost or duplicated')
                before=platform.evidence()
                require(len(before['send_attempts'])==(1 if lost_ack else 2),'unexpected remote send attempts before restart')
                h.stop(kill=True)
                prior_version=h.rows()['deliveries'][0]['version']
                h.start()
                h.callback('output',json.dumps(dict(name='checklist.txt',mediaType='text/plain',text=content)),event='output-retry')
                if lost_ack:
                    h.wait(lambda s:s['deliveries'][0]['status']=='unknown' and s['deliveries'][0]['version']>=prior_version+2,'unknown delivery was queried after restart')
                require(h.rows()['deliveries'][0]['status']==status and platform.evidence()==before,'uncertain effect or sent attachment retried')
                report['scenarios']['lost-remote-ack' if lost_ack else 'outbound-attachment']=dict(state=h.rows(),platform=platform.evidence())
                h.stop()
            finally:h.cleanup()
    report['checks'].append('Outbound Artifact multipart bytes and recipient verified; sent multipart receipts survive restart; ambiguous remote send stays unknown without resend')

    # SIGKILL immediately before and after the input transaction, and after a
    # channel accepted delivery but before outbox acknowledgement.
    for boundary in ('before_channel_input_commit','after_channel_input_commit','after_delivery_send'):
        with tempfile.TemporaryDirectory(prefix='erii-channel-kill-') as temp, Sandbox() as platform:
            h=ChannelHarness(binary,Path(temp),platform)
            try:
                h.setup();h.start(boundary)
                def invoke():
                    try:h.callback('crash-input','Recover the same channel message.')
                    except (OSError,HTTPError):pass
                with ThreadPoolExecutor(max_workers=1) as pool:
                    future=pool.submit(invoke)
                    with selectors.DefaultSelector() as selector:
                        selector.register(h.process.stdout,selectors.EVENT_READ)
                        require(selector.select(15),'missing channel crash boundary '+boundary)
                    require(json.loads(h.process.stdout.readline())['boundary']==boundary,'wrong crash boundary')
                    h.stop(kill=True);future.result(timeout=20)
                h.start();h.callback('crash-input','Recover the same channel message.',event='retry-after-kill')
                state=h.wait(lambda s:len(s['inputs'])==1 and len(s['deliveries'])==1 and s['deliveries'][0]['status']=='sent','channel crash recovery')
                require(len(platform.evidence()['messages'])==1 and len(platform.evidence()['send_attempts'])==1,'channel crash duplicated send attempt or notification')
                report['scenarios'][boundary]=dict(state=state,platform=platform.evidence())
                h.stop()
            finally:h.cleanup()
    report['checks'].append('Real SIGKILL before/after inbound commit and after remote delivery receipt: one accepted message, one execution and one notice after restart')

    # Exercise Task continuity and notification deduplication through the transports.
    for case,fixture in (('task-continuation','tasks'),('scheduled-delivery','automation')):
        with tempfile.TemporaryDirectory(prefix='erii-channel-'+case+'-') as temp, Sandbox() as platform:
            h=ChannelHarness(binary,Path(temp),platform,fixture)
            try:
                h.setup()
                if case=='scheduled-delivery':h.now='2026-10-15T12:00:00+08:00'
                h.start()
                if case=='task-continuation':
                    h.callback(case,'Delegate: Prepare the wedding venue checklist with material dependencies; contact nobody.')
                    h.wait(lambda s:len(s['runs'])>=3 and all(r['status']=='completed' for r in s['runs']),'Task completion')
                    h.http('/api/message',dict(source_id='wedding-update',content=[dict(type='text',text='Continue: wedding | Guests confirmed: 128. Preserve the checklist and ask before contacting the hotel.')]))
                    h.wait(lambda s:len([r for r in s['runs'] if r['kind']=='worker'])==2 and all(r['status']=='completed' for r in s['runs']) and len(s['deliveries'])==4 and all(d['status']=='sent' for d in s['deliveries']),'Task continuation')
                    with sqlite3.connect(h.root/'data/state.db') as db:
                        tasks=list(db.execute('SELECT task_id,anchor_task,task_epoch FROM task'))
                        instructions=list(db.execute('SELECT instruction FROM task_instruction'))
                    require(len(platform.evidence()['messages'])==3,'wedding notices did not use bound/default Feishu')
                    require(len(tasks)==1 and tasks[0][2]==2 and len(instructions)==1 and '128' in instructions[0][0],'wedding continuity broken')
                    history=h.http('/api/conversation')
                else:
                    q=dict(operation='create',instruction='Check HKG-CDG 2026-12-01 daily price; notify only a new entry below threshold.',triggerType='cron',triggerSpec='0 8 * * *',timezone='Asia/Shanghai',below=1800)
                    h.callback(case,'Automation: '+json.dumps(q))
                    h.wait(lambda s:len(s['deliveries'])==1 and s['deliveries'][0]['status']=='sent','Automation accepted over channel')
                    h.stop()
                    for i,expected in enumerate((1,1,2,2,2,3),16):
                        h.now=f'2026-10-{i:02d}T08:00:00+08:00';h.start()
                        h.wait(lambda s:len([r for r in s['runs'] if r['kind']=='worker' and r['status']=='completed'])==i-15 and len(s['deliveries'])==expected and all(d['status']=='sent' for d in s['deliveries']),'quiet threshold day')
                        require(len(platform.evidence()['messages'])==expected,'threshold notification storm')
                        h.stop()
                    with sqlite3.connect(h.root/'data/state.db') as db:
                        require(db.execute("SELECT count(*) FROM task WHERE status='active'").fetchone()[0]==1,'ongoing monitor completed')
                    h.start();history=h.http('/api/conversation')
                report['scenarios'][case]=dict(status='passed',state=h.rows(),conversation=history,platform=platform.evidence())
                h.stop()
            finally:h.cleanup()
    report['checks'].append('Task continuation across Feishu/Web; real zoned scheduling to default Feishu with only two threshold notices')


def account_presentation(binary,report):
    # Reuse the account harness' owned OAuth network substitution and SecretStore.
    # The production Bridge/Manager is real; no login token or auth URL is evidence.
    from verify_account import account_config, Secrets
    from fixture_pool import current
    from event_broker import Broker
    scope=current()
    fixture=scope.start(['node',str(ROOT/'provider-bridge/test/account-fixture.mjs')],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(fixture.stdout,selectors.EVENT_READ)
            require(selector.select(10),'OAuth fixture not ready')
        endpoint=fixture.stdout.readline().strip()
        with tempfile.TemporaryDirectory(prefix='ec-auth-',dir='/tmp') as temp, Broker(Path(temp)) as broker:
            root=Path(temp);socket=root/'secrets.sock'
            server=socketserver.ThreadingUnixStreamServer(str(socket),Secrets)
            server.values={'credential:web-owner':base64.b64encode(TOKEN.encode()).decode()}
            server.lock=threading.Lock();server.sensitive=set();os.chmod(socket,0o600)
            thread=threading.Thread(target=server.serve_forever);thread.start()
            h=object.__new__(ChannelHarness);MainHarness.__init__(h,binary,root,fixture='')
            h.env.update(ERII_NATS_URL=broker.url,ERII_NATS_STREAM_PREFIX='web_account')
            try:
                h.config=account_config(root,ROOT/'provider-bridge/bridge.mjs',endpoint)
                with h.config.open('a') as out:out.write('web:\n  listen: 127.0.0.1:0\n  token_ref: credential:web-owner\n')
                original=h.args
                h.args=lambda *args:original('-test-secret-store',str(socket),*args)
                h.env['ERII_WEB_TEST_TOKEN']=TOKEN;h.process=None;h.start()
                form={'Content-Type':'application/x-www-form-urlencoded'}
                h.http('/auth/login',b'',headers=form)
                pending=h.wait_state(lambda s:any(e['kind']=='open_url' for e in s['events']),'UI-neutral account authorization event',lambda _:h.http('/api/auth'))
                private_url=next(e['url'] for e in pending['events'] if e['kind']=='open_url')
                request=Request(endpoint+'/fixture/authorize',data=json.dumps(dict(url=private_url)).encode(),headers={'Content-Type':'application/json'})
                with urlopen(request,timeout=10) as response:callback=json.load(response)['callback']
                with urlopen(callback,timeout=10) as response:require(response.status==200,'OAuth callback rejected')
                connected=h.wait_state(lambda s:s['status']=='connected','Web account login completes',lambda _:h.http('/api/auth'))
                aid=connected['account_id'];require(aid,'account metadata absent')
                h.http('/model/add',urlencode(dict(account=aid,model='hidden-model')).encode(),headers=form,expected=409)
                h.http('/model/add',urlencode(dict(account=aid,model='fixture-account-model')).encode(),headers=form)
                profile=h.command('model','list')[0]
                h.http('/model/test',urlencode(dict(profile=profile['profile_id'])).encode(),headers=form)
                h.http('/model/select',urlencode(dict(profile=profile['profile_id'],profile_version=profile['version'],config_version=1,consent='yes')).encode(),headers=form)
                require(h.http('/api/status')['readiness']=='ready','Web account profile unavailable')
                h.stop();h.start()
                require('Account ' in h.http('/settings'),'restart hid connected account')
                h.http('/auth/logout',urlencode(dict(account=aid)).encode(),headers=form)
                require(h.command('model','accounts')[0]['status']=='logged_out','Web logout missed existing lifecycle')
                with server.lock:
                    sensitive=list(server.sensitive)
                for path in (root/'data').rglob('*'):
                    if path.is_file():require(all(value not in path.read_bytes() for value in sensitive),'credential persisted outside SecretStore')
                report['scenarios']['account-auth']=dict(status='connected_then_logged_out',event_kinds=[e['kind'] for e in pending['events']],profile_id=profile['profile_id'],credential_scan='clean')
                report['checks'].append('Web account login uses real Manager/Bridge against owned OAuth Fake; ephemeral URL, hidden-model rejection, actual Probe/Select, restart picker, logout and credential scan verified')
                h.stop()
            finally:
                h.cleanup();server.shutdown();server.server_close();thread.join(timeout=10)
                require(not thread.is_alive(),'SecretStore server did not stop')
    finally:
        fixture.terminate();fixture.communicate(timeout=10);scope.retire(fixture)


if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--binary',type=Path,required=True);parser.add_argument('--evidence',type=Path,required=True)
    action=parser.add_mutually_exclusive_group()
    action.add_argument('--delivery-lease-only',action='store_true')
    action.add_argument('--ui-components-only',action='store_true')
    args=parser.parse_args()
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    def verify(_):
        if not args.delivery_lease_only and not args.ui_components_only:
            return run(args.binary.resolve(),args.evidence.resolve())
        report = dict(checks=[], scenarios={})
        if args.ui_components_only:
            early_composer(args.binary.resolve(),args.evidence.resolve(),report)
            native_components(args.binary.resolve(),args.evidence.resolve(),report)
        else:
            delivery_leases(args.binary.resolve(), report)
        report['checks'].append('Cleanup: Web/Bridge children joined, sandbox HTTP servers stopped, disposable roots removed')
        return report
    report=run_cases([0],verify,1)[0]
    report['binary_sha256']=hashlib.sha256(args.binary.read_bytes()).hexdigest()
    args.evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(dict(evidence=str(args.evidence),checks=report['checks'])))
