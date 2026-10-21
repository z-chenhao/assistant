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
from fixture_pool import run_cases
from verify_main import MainHarness, require
from terminal_driver import Terminal

ROOT = Path(__file__).resolve().parents[1]
TOKEN = 'sandbox-web-owner-token-at-least-24-bytes'


class ChannelHarness(MainHarness):
    def __init__(self, binary, root, platform, fixture='main'):
        super().__init__(binary, root, fixture)
        self.config = root / 'config.yaml'
        self.config.write_text('version: 1\nweb:\n  listen: "127.0.0.1:0"\n  token_ref: env:ERII_WEB_TEST_TOKEN\nfeishu:\n  app_id: sandbox-app\n  tenant_key: sandbox-tenant\n  verification_token_ref: env:ERII_FEISHU_VERIFY\n  encrypt_key_ref: env:ERII_FEISHU_ENCRYPT\n  access_token_ref: env:ERII_FEISHU_ACCESS\n  api_base: ' + platform.url + '\n')
        self.env.update(ERII_WEB_TEST_TOKEN=TOKEN, ERII_FEISHU_VERIFY='sandbox-verify-token',
                        ERII_FEISHU_ENCRYPT='sandbox-encrypt-key', ERII_FEISHU_ACCESS='sandbox-access-token')
        self.bind = self.command('channel','manage',text=json.dumps(dict(op='bind',channel='feishu',subject='sandbox-tenant:sandbox-app:owner',destination='bound-chat',enabled=True,version=0)))
        self.process = None

    def args(self, *args):
        return [str(self.binary), '-config', str(self.config), '-test-bridge', self.fixture, *(['-test-automation-now',self.now] if hasattr(self,'now') else []), *args]

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
        require(line, 'Web launch exited')
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
        result=run(['node',str(ROOT/'scripts/channels_browser.mjs'),self.url,str(evidence),phase],env=env,capture_output=True,text=True,timeout=45)
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

    # Approval recovery uses the same external-effect Fake and persistent Security
    # lifecycle as TUI. Callback races cross two authenticated transports.
    with tempfile.TemporaryDirectory(prefix='erii-channel-approval-') as temp, Sandbox() as platform:
        h=ChannelHarness(binary,Path(temp),platform,'approval')
        try:
            h.setup();h.start()
            h.callback('effect-source',json.dumps(dict(target='alex.chen@example.test',text='Erii review: 2026-10-19 15:00–15:30 +08:00')))
            state=h.wait(lambda s:len(s['approvals'])==1 and s['approvals'][0]['status']=='pending','approval notice')
            old=state['approvals'][0]
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
    extra_scenarios(binary,report)
    account_presentation(binary,report)
    report['checks'].append('Cleanup: Web/Bridge/browser children joined, sandbox HTTP servers stopped, disposable roots removed')
    return report


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
                    h.wait(lambda s:s['deliveries'][0]['status']=='unknown' and s['deliveries'][0]['version']>prior_version,'unknown delivery was queried after restart')
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

    # Use existing versioned scripted-policy fixtures through the new transports.
    # These are transport/state fragments; they never claim full semantic scores.
    for case,fixture in (('GC-08','tasks'),('GC-09','golden'),('GC-14','automation')):
        with tempfile.TemporaryDirectory(prefix='erii-channel-'+case+'-') as temp, Sandbox() as platform:
            h=ChannelHarness(binary,Path(temp),platform,fixture)
            try:
                h.setup()
                if case=='GC-14':h.now='2026-10-15T12:00:00+08:00'
                h.start()
                if case=='GC-09':
                    text='发布风险说明. Calendar: 10:00 weekly meeting, 14:00 interview. Inbox: E1 release risk due 12:00; E2 newsletter; E3 hotel already confirmed; E4 interview confirmation. 我今天先做什么？给我一个尽量短的安排。'
                    h.http('/api/message',dict(source_id=case,content=[dict(type='text',text=text)]))
                    h.wait(lambda s:len(s['deliveries'])==1 and s['deliveries'][0]['status']=='sent','GC-09 Web agenda')
                    history=h.http('/api/conversation');reply=history[-1]['content'][0]['text']
                    require('12:00' in reply and '14:00' in reply and 'E4' in reply,'agenda contract missing')
                    require(not platform.evidence()['messages'],'synchronous Web reply notified Feishu')
                elif case=='GC-08':
                    h.callback(case,'Delegate: Prepare the wedding venue checklist with material dependencies; contact nobody.')
                    h.wait(lambda s:len(s['runs'])>=3 and all(r['status']=='completed' for r in s['runs']),'GC-08 Task completion')
                    h.http('/api/message',dict(source_id='wedding-update',content=[dict(type='text',text='Continue: wedding | Guests confirmed: 128. Preserve the checklist and ask before contacting the hotel.')]))
                    h.wait(lambda s:len([r for r in s['runs'] if r['kind']=='worker'])==2 and all(r['status']=='completed' for r in s['runs']) and len(s['deliveries'])==4 and all(d['status']=='sent' for d in s['deliveries']),'GC-08 continuation')
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
                        h.wait(lambda s:len([r for r in s['runs'] if r['kind']=='worker' and r['status']=='completed'])==i-15 and len(s['deliveries'])==expected and all(d['status']=='sent' for d in s['deliveries']),'GC-14 quiet threshold day')
                        require(len(platform.evidence()['messages'])==expected,'threshold notification storm')
                        h.stop()
                    with sqlite3.connect(h.root/'data/state.db') as db:
                        require(db.execute("SELECT count(*) FROM task WHERE status='active'").fetchone()[0]==1,'ongoing monitor completed')
                    h.start();history=h.http('/api/conversation')
                report['scenarios'][case]=dict(status='fragment_pass',full_case='not_applicable',state=h.rows(),conversation=history,platform=platform.evidence())
                h.stop()
            finally:h.cleanup()
    report['checks'].append('GC-08 Task continuation across Feishu/Web; GC-09 agenda through Web; GC-14 real zoned scheduling to default Feishu with only two threshold notices; scripted transport fragments, no full semantic score claim')


def account_presentation(binary,report):
    # Reuse the account harness' owned OAuth network substitution and SecretStore.
    # The production Bridge/Manager is real; no login token or auth URL is evidence.
    from verify_account import account_config, Secrets
    from fixture_pool import current
    scope=current()
    fixture=scope.start(['node',str(ROOT/'provider-bridge/test/account-fixture.mjs')],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(fixture.stdout,selectors.EVENT_READ)
            require(selector.select(10),'OAuth fixture not ready')
        endpoint=fixture.stdout.readline().strip()
        with tempfile.TemporaryDirectory(prefix='ec-auth-',dir='/tmp') as temp:
            root=Path(temp);socket=root/'secrets.sock'
            server=socketserver.ThreadingUnixStreamServer(str(socket),Secrets)
            server.values={'credential:web-owner':base64.b64encode(TOKEN.encode()).decode()}
            server.lock=threading.Lock();server.sensitive=set();os.chmod(socket,0o600)
            thread=threading.Thread(target=server.serve_forever);thread.start()
            h=object.__new__(ChannelHarness);MainHarness.__init__(h,binary,root,fixture='')
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
    parser=argparse.ArgumentParser();parser.add_argument('--binary',type=Path,required=True);parser.add_argument('--evidence',type=Path,required=True);args=parser.parse_args()
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    report=run_cases([0],lambda _:run(args.binary.resolve(),args.evidence.resolve()),1)[0]
    report['binary_sha256']=hashlib.sha256(args.binary.read_bytes()).hexdigest()
    args.evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(dict(evidence=str(args.evidence),checks=report['checks'])))
