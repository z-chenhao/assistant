#!/usr/bin/env python3
"""CLI/QuickJS -> ordinary ToolExecutor -> actual SSH/Cua/Browser adapters.

Peers are deterministic protocol Fakes. --live-config selects real Lume/Guest
boundaries with the same drive; no online accounts or websites are required.
"""
import argparse
from contextlib import contextmanager, nullcontext
import hashlib
import json
from pathlib import Path
import selectors
import signal
import time
import urllib.error
import shutil
import sqlite3
import subprocess
import sys
import tempfile

from verify_computer import ComputerHarness, request
from verify_main import require


@contextmanager
def launch(binary, root, live_config=None, browser_peer=None):
    source, vm = root / 'source', root / 'vm'
    server = None
    h = ComputerHarness(binary, root, fixture='computer')
    try:
        if live_config:
            computer = json.loads(Path(live_config).read_text())['computer']
            h.env['ERII_COMPUTER_GUEST_TEST_LOGIN'] = 'lume'
            computer['manifest']={'version':1,'applications':[],'artifact_refs':[]}
        else:
            source.mkdir(mode=0o700)
            vm.mkdir(mode=0o700)
            if browser_peer:
                (vm / 'browser-peer.json').write_text(json.dumps({'endpoint': browser_peer}))
            for name in ('disk.img', 'nvram.bin', 'config.json'):
                (source / name).write_text('fixture-' + name)
            (source / 'config.json').write_text(json.dumps({'macAddress': '02:00:00:00:00:01', 'machineIdentifier': 'fixture', 'cpuCount': 4}))
            h.env['ERII_COMPUTER_FIXTURE_PASSWORD'] = 'fixture-only'
            start = h.processes.start if h.processes else subprocess.Popen
            server = start([str(binary), '-test-computer-server', str(vm)], stdout=subprocess.PIPE,
                                      stderr=subprocess.PIPE, text=True, env=h.env, cwd=root)
            with selectors.DefaultSelector() as selector:
                selector.register(server.stdout, selectors.EVENT_READ)
                require(selector.select(10), 'doctor: fixture HTTP/SSH peer not ready')
            computer = json.loads(server.stdout.readline())
            if browser_peer:
                (vm / 'browser-peer.json').unlink()
        node = shutil.which('node')
        require(node, 'doctor: Node required')
        script = Path(__file__).resolve().parents[2] / 'runtime/javascript/src/sidecar.mjs'
        (root / 'config.yaml').write_text(json.dumps({'version': 1, 'javascript': {'command': node, 'script': str(script)}, 'computer': computer}))
        if not live_config:
            require(h.command('computer', 'prepare-base', str(source))['base'] == 'sealed', 'base not sealed')
        if not h.command('status')['agent_ingress_enabled']:
            h.setup()
        yield h
    finally:
        h.cleanup()
        if server:
            server.terminate()
            server.communicate(timeout=15)
            if h.processes:h.processes.retire(server)
            require(server.returncode == 0, 'fixture server did not join cleanly')


def collect_observations(h, report, live_root):
    """Read existing scoped Artifacts; no GUI input or service startup."""
    kind = 'browser' if report.get('real_guest_browser') else 'native'
    saved = []
    for check in report['checks']:
        for nested in check['nested']:
            result = nested.get('result') or {}
            if result.get('error'): continue
            try: value = json.loads(result.get('text', ''))
            except (ValueError, TypeError): continue
            if not isinstance(value, dict): continue
            shot = value.get('screenshot', value)
            if not isinstance(shot, dict) or not isinstance(shot.get('image'), dict): continue
            ref = shot['image'].get('artifactRef')
            if not ref: continue
            readback = subprocess.run(h.args('artifact', 'open', ref), env=h.env, capture_output=True, timeout=20)
            require(readback.returncode == 0 and readback.stdout.startswith(b'\x89PNG'), 'observation Artifact unreadable')
            name = f'computer-use-{kind}-observation-{len(saved)+1:02}.png'
            (live_root/'evidence'/name).write_bytes(readback.stdout)
            saved.append({'file':name, 'artifactRef':ref, 'sha256':hashlib.sha256(readback.stdout).hexdigest()})
    require(saved, 'no screenshot observations collected')
    report['screenshots'] = saved


def run(binary, live_config=None, live_root=None, work_root=None, native_only=False, provision=False, browser_live=False):
    drive_nonce = str(time.time_ns())
    report = {'version': 1, 'scope': 'real Guest' if live_config else 'offline SSH/Cua/Playwright protocol peers; no real GUI claim', 'checks': [], 'cleanup': False}
    with (nullcontext(work_root) if work_root else nullcontext(tempfile.mkdtemp(prefix='erii25-cua-use-',dir=live_root)) if live_config else tempfile.TemporaryDirectory(prefix='erii-cua-use-')) as temporary:
        root = Path(temporary).resolve()
        with launch(binary, root, live_config) as h:
            state = Path(h.env['ERII_DATA_ROOT']) / 'state.db'
            if native_only or browser_live:
                metadata={'name':'CUA live evidence','provider_kind':'openai_compatible','endpoint':'http://127.0.0.1:1/v1','wire_api':'openai_completions','model_id':'fixture-model','context_window':65536,'max_output_tokens':4096,'compatibility':{'tool_result_format':'openai'},'capabilities':{'text_input':True,'tool_calls':True,'streaming':True}}
                h.setup(metadata=metadata)
            if browser_live:
                config=json.loads((root/'config.yaml').read_text())
                config['computer']['browser']={'node':'/Users/lume/erii25-runtime/node/bin/node','module':'/Users/lume/erii25-runtime/browser/node_modules/playwright-core/index.mjs','port':9222}
                (root/'config.yaml').write_text(json.dumps(config))
            artifact = h.command('artifact', 'save', 'private-fixture.txt', 'text/plain', text='private fixture bytes')
            def rows():
                with sqlite3.connect(state) as db:
                    db.row_factory = sqlite3.Row
                    return [dict(r) for r in db.execute('SELECT * FROM tool_invocation ORDER BY created_at,rowid')]

            def execute(key, source, approve=True, refs=()):
                plan = json.dumps([{'name': 'builtin__codemode_execute', 'arguments': {'source': cell}} for cell in (source if isinstance(source,list) else [source])])
                key_label = key
                if live_config: key = key + ':' + drive_nonce
                if refs:
                    h.command('chat', 'send-parts', key, text=json.dumps([{'type':'text','text':'Capabilities: '+plan}, *[{'type':'file','artifact_ref':ref} for ref in refs]]))
                else:
                    h.send(key, 'Capabilities: ' + plan)
                approvals = []
                for _ in range(40):
                    if live_config and key_label=='inventory-bind-observe-independent-revisions' and _==0:
                        release=root/'release-live-install'
                        release.unlink(missing_ok=True)
                        worker=h.hold('computer_ready',extra_flags=('-test-main-release',str(release)),timeout=240)
                        with sqlite3.connect(state) as db:
                            instance=json.loads(db.execute('SELECT instance_json FROM computer').fetchone()[0])
                        from lume_live import Live
                        live=Live(live_root,'product',instance['native_name'])
                        live.put_fixture('erii25Fixture')
                        plist='<?xml version="1.0"?><plist version="1.0"><dict><key>CFBundleExecutable</key><string>Fixture</string><key>CFBundleIdentifier</key><string>com.erii.fixture</string><key>CFBundleName</key><string>Erii CUA Fixture</string><key>CFBundlePackageType</key><string>APPL</string></dict></plist>'
                        import shlex
                        directory='/Users/lume/Applications/EriiCUAFixture.app/Contents'
                        h.guest_command(live_root,'mkdir -p '+directory+'/MacOS && cp /Users/lume/erii24-fixtures/erii25Fixture '+directory+'/MacOS/Fixture && chmod 755 '+directory+'/MacOS/Fixture && printf %s '+shlex.quote(plist)+' > '+directory+'/Info.plist && /usr/bin/codesign --force --sign - '+directory+'/.. && /System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister -f '+directory+'/..')
                        release.touch();worker.communicate(timeout=320);require(worker.returncode==0,'live initial worker failed')
                    elif browser_live and _==0:
                        release=root/'release-browser-doctor'
                        release.unlink(missing_ok=True)
                        worker=h.hold('computer_ready',extra_flags=('-test-main-release',str(release)),timeout=240)
                        with sqlite3.connect(state) as db: instance=json.loads(db.execute('SELECT instance_json FROM computer').fetchone()[0])
                        from lume_live import Live
                        live=Live(live_root,'product',instance['native_name'])
                        target=live_root/'fixtures/erii25-browser-doctor.sh'
                        require(not target.exists(),'refusing to replace an existing Doctor fixture')
                        shutil.copyfile(Path(__file__).resolve().parents[2]/'tests/fixtures/computer-use-v1/doctor_browser.sh',target)
                        try: live.put_fixture(target.name)
                        finally: target.unlink()
                        try: report['guest_browser_doctor']=live.guest('/bin/bash /Users/lume/erii24-fixtures/erii25-browser-doctor.sh',timeout=45)
                        finally: release.touch()
                        worker.communicate(timeout=320)
                        require(worker.returncode==0,'browser Doctor worker failed')
                    elif provision and key_label=='native-elements-secondary-selection-and-coordinates' and _==1:
                        release=root/'release-browser-provision'
                        release.unlink(missing_ok=True)
                        worker=h.hold('computer_ready',extra_flags=('-test-main-release',str(release)),timeout=240)
                        with sqlite3.connect(state) as db: instance=json.loads(db.execute('SELECT instance_json FROM computer').fetchone()[0])
                        from lume_live import Live
                        live=Live(live_root,'product',instance['native_name'])
                        sources={'erii25-package.json':'guest-browser/package.json','erii25-package-lock.json':'guest-browser/package-lock.json','erii25-transfer.py':'internal/computer/macosvm/transfer.py','erii25-site.mjs':'tests/fixtures/computer-use-v1/site.mjs','erii25-provision.sh':'tests/fixtures/computer-use-v1/provision_browser.sh'}
                        repository=Path(__file__).resolve().parents[2]
                        for name,path in sources.items():
                            target=live_root/'fixtures'/name
                            require(not target.exists(),'refusing to replace an existing provision fixture')
                            shutil.copyfile(repository/path,target)
                            try: live.put_fixture(name)
                            finally: target.unlink()
                        report['guest_browser_provision']=live.guest('/bin/bash /Users/lume/erii24-fixtures/erii25-provision.sh',timeout=180)
                        release.touch();worker.communicate(timeout=320);require(worker.returncode==0,'provision worker failed')
                    else:
                        h.command('chat', 'work')
                    pending = [a for a in h.command('approval', 'list') if a['status'] == 'pending']
                    if not pending: break
                    for a in pending:
                        before = rows()
                        with sqlite3.connect(state) as db:
                            target_id = db.execute('SELECT invocation_id FROM approval WHERE approval_id=?', (a['approval_id'],)).fetchone()[0]
                        target = next(i for i in before if i['invocation_id'] == target_id)
                        require(target['attempt_count'] == 0, 'effect dispatched before approval')
                        require(target['replay_policy'] == 'never', 'effect became safely retryable')
                        approvals.append({'tool': target['tool_name'], 'arguments': json.loads(target['arguments_json']), 'hash': a['frozen_arguments_hash']})
                        h.command('approval', 'approve' if approve else 'deny', a['approval_id'], a['frozen_arguments_hash'])
                else: raise AssertionError('drive did not settle')
                invocations = rows()
                parent = [r for r in invocations if r['tool_name'] == 'builtin__codemode_execute'][-1]
                require(parent['status'] == 'succeeded', f'cell failed: {parent["result_json"]}')
                nested = [r for r in invocations if r['parent_invocation_id'] == parent['invocation_id']]
                require(all(json.loads(r['conflict_keys']) for r in nested), 'read/action missed desktop barrier')
                output = json.loads(json.loads(parent['result_json'])['text'])
                report['checks'].append({'name': key, 'output': output, 'approvals': approvals,
                                         'nested': [{'tool': i['tool_name'], 'status': i['status'], 'attempts': i['attempt_count'], 'policy': json.loads(i['policy_json']), 'arguments': json.loads(i['arguments_json']), 'result': json.loads(i['result_json']) if i['result_json'] else None} for i in nested]})
                return output, approvals

            if not native_only and not browser_live:
                app = 'com.erii.fixture' if live_config else 'com.apple.TextEdit'
                output, _ = execute('inventory-bind-observe-independent-revisions', f'''
    const inventory=await cua.getState();
    {('await cua.computer.launchApp({app:"com.erii.fixture"});' if live_config else '')}
    const app=await cua.getApp({json.dumps(app)});
    const ax=await app.getAXState({{full:true}});
    const shot=await app.getScreenshot();
    image(shot.image);
    return {{computer:inventory.computerId, ax:ax.state, revision:ax.revision, screenshot:shot.revision}};
    ''')
                require(output[0]['type'] == 'image' and output[0]['mimeType'] == 'application/x-erii-artifact', 'explicit image reference output missing')
                image_ref = output[0]['data']
                image_bytes = subprocess.run(h.args('artifact', 'open', image_ref), env=h.env, capture_output=True, timeout=20)
                require(image_bytes.returncode == 0 and image_bytes.stdout.startswith(b'\x89PNG'), 'screenshot Artifact not readable')
                report['screenshot_sha256'] = hashlib.sha256(image_bytes.stdout).hexdigest()

                output, approvals = execute('action-observe-old-reference-and-wait', f'''
    {('await cua.computer.launchApp({app:"com.erii.fixture"});' if live_config else '')}
    const app=await cua.getApp({json.dumps(app)});
    const old=await app.getAXState();
    await app.typeText("approved fixture");
    const current=await app.getAXStateAndScreenshot();
    let stale;try{{await app.click({{observationRevision:old.revision,index:0}});}}catch(e){{stale=e.code;}}
    let wait;try{{await app.waitForChange({{sinceRevision:current.ax.revision,timeoutMs:1}});}}catch(e){{wait={{code:e.code,observation:e.details?.observation}};}}
    return {{state:current.ax.state,stale,wait}};
    ''')
                value = output[-1]['value']
                require(value['stale'] == 'STALE_ELEMENT_REFERENCE' and value['wait']['code'] == 'WAIT_TIMEOUT', 'stale/wait contract missing')
                require('approved fixture' in value['state'], 'post-action observation did not show input')
                require(approvals, 'input had no approval')

            if live_config and not browser_live:
                output,_=execute('native-elements-secondary-selection-and-coordinates', r'''
await cua.computer.launchApp({app:"com.erii.fixture"});
const window=(await cua.listWindows()).find(w=>w.name==="Erii CUA Fixture");
if(!window)throw new Error("fixture window not discovered");
const app=await cua.getApp({windowId:window.id});
const field=ax=>Number(ax.state.match(/\[(\d+)\] AXTextField/)[1]);
const before=await app.getAXState();
await app.getScreenshot();
await app.setValue({observationRevision:before.revision,index:field(before)},"committed value");
const set=await app.getAXState();
await app.performSecondaryAction({observationRevision:set.revision,index:field(set)},"confirm");
const committed=await app.getAXState();
if(!committed.state.includes("committed=committed value")) throw new Error("commit readback failed");
await app.selectText({observationRevision:committed.revision,index:field(committed)},"committed");
const selected=await app.getAXStateAndScreenshot();
if(!selected.ax.state.includes("selection=0:9")) throw new Error("selection readback failed");
const button=Number(selected.ax.state.match(/\[(\d+)\] AXButton "Increment"/)[1]);
await app.click({observationRevision:selected.ax.revision,index:button});
const clicked=await app.getAXStateAndScreenshot();
if(!clicked.ax.state.includes("count=1")) throw new Error("element click readback failed");
await app.click([105,186]);
const pointer=await app.getAXStateAndScreenshot();
if(!pointer.ax.state.includes("count=2")) throw new Error("coordinate click readback failed");
image(pointer.screenshot.image);
text(pointer.ax.state.split("\n").slice(0,7).join("\n"));
return {committed:true,selected:"0:9",elementCount:1,pointerCount:2};
''')
                report['native_action_readback']=True
                selected=next(item for item in output if item['type']=='image')
                image_bytes=subprocess.run(h.args('artifact','open',selected['data']),env=h.env,capture_output=True,timeout=20)
                require(image_bytes.returncode==0,'native screenshot artifact unavailable')
                report['screenshot_sha256']=hashlib.sha256(image_bytes.stdout).hexdigest()
            if browser_live:
                output,approvals=execute('real-guest-browser-locators-transfers-and-expiry', r'''
const tabs=await cua.listTabs();const tab=await cua.getTab(tabs[0].id);
await tab.goto("http://127.0.0.1:18025/");
let observed=await tab.getAXStateAndScreenshot();
const input=Number(observed.ax.state.match(/\[(\d+)\] INPUT "Fixture input"/)[1]);
await tab.setValue({observationRevision:observed.ax.revision,index:input},"Guest core input");
observed=await tab.getAXStateAndScreenshot();
if(!observed.ax.state.includes("Guest core input"))throw new Error("core input readback");
await tab.capabilities.list();const pw=await tab.capabilities.get("playwright");
await pw.locator("#field").fill("Guest verified input");
observed=await tab.getAXStateAndScreenshot();
if(!observed.ax.state.includes("Guest verified input"))throw new Error("fill readback");
await pw.locator("#button").click();
observed=await tab.getAXStateAndScreenshot();
if(!observed.ax.state.includes("clicked"))throw new Error("click readback");
const download=await pw.locator("#download").download();
observed=await tab.getAXStateAndScreenshot();
const exported=await cua.files.exportArtifact({path:download.path,name:"guest-download.txt",mediaType:"text/plain"});
const file=await cua.files.importArtifact({artifactRef:SOURCE_REF,destination:UPLOAD_PATH});
await pw.locator("#upload").upload(file);
observed=await tab.getAXStateAndScreenshot();
if(!observed.ax.state.includes("guest-upload"))throw new Error("chooser readback");
await tab.reload();observed=await tab.getAXStateAndScreenshot();
let stale;try{await pw.locator("#field").count();}catch(e){stale=e.code;}
if(stale!=="STALE_CAPABILITY_HANDLE")throw new Error("navigation expiry");
image(observed.screenshot.image);return {exported,stale,coreInput:true,fill:true,click:true,chooser:true};
'''.replace('SOURCE_REF',json.dumps(artifact['artifact_ref'])).replace('UPLOAD_PATH',json.dumps('imports/guest-upload-'+drive_nonce)),refs=[artifact['artifact_ref']])
                exported=output[-1]['value']['exported']['artifactId']
                readback=subprocess.run(h.args('artifact','open',exported),env=h.env,capture_output=True,timeout=20)
                require(readback.returncode==0 and readback.stdout==b'Guest browser download fixture','real Guest download bytes differ')
                selected=next(item for item in output if item['type']=='image')
                image_bytes=subprocess.run(h.args('artifact','open',selected['data']),env=h.env,capture_output=True,timeout=20)
                require(image_bytes.returncode==0,'browser screenshot artifact unavailable')
                report['screenshot_sha256']=hashlib.sha256(image_bytes.stdout).hexdigest()
                report['real_guest_browser']=True
            if live_config:
                require(h.command('computer','status')['phase']=='checkpointed','live Computer was not released')
                report['screenshot_file']='computer-use-browser-live.png' if browser_live else 'computer-use-native-live.png'
                (live_root/'evidence'/report['screenshot_file']).write_bytes(image_bytes.stdout)
                collect_observations(h, report, live_root)
                report['cleanup']=True
                return report

            output, approvals = execute('transfer-overwrite-export-and-upload-separate-consent', f'''
const file=await cua.files.importArtifact({{artifactRef:{json.dumps(artifact['artifact_ref'])},destination:"imports/fixture"}});
await cua.files.importArtifact({{artifactRef:{json.dumps(artifact['artifact_ref'])},destination:"imports/fixture",overwrite:true}});
const exported=await cua.files.exportArtifact({{path:file.path,name:"roundtrip.txt",mediaType:"text/plain"}});
const tab=await cua.getTab("fixture-tab");
await tab.capabilities.list();
const pw=await tab.capabilities.get("playwright");
await pw.locator("#upload").upload(file);
const download=await pw.locator("#download").download();
const downloaded=await cua.files.exportArtifact({{path:download.path,name:"download.txt",mediaType:"text/plain"}});
return {{exported,downloaded}};
''', refs=[artifact['artifact_ref']])
            require(len(approvals) == 6, f'import, overwrite, export, upload, download approvals were conflated: {len(approvals)} {output}')
            exported = output[-1]['value']['exported']['artifactId']
            readback = subprocess.run(h.args('artifact', 'open', exported), env=h.env, capture_output=True, timeout=20)
            require(readback.returncode == 0 and readback.stdout == b'private fixture bytes', 'transfer roundtrip bytes differ')

            output,approvals=execute('registered-files-survive-cell-and-instance-replacement',[
                'return await cua.files.importArtifact({artifactRef:'+json.dumps(artifact['artifact_ref'])+',destination:"imports/cross-cell"});',
                'return await cua.files.exportArtifact({path:"imports/cross-cell",name:"cross-cell.txt",mediaType:"text/plain"});'
            ],refs=[artifact['artifact_ref']])
            require(len(approvals)==2 and output[-1]['value']['artifactId'],'file registration lost between cells')
            output,_=execute('known-file-path-is-not-cross-run-authority','try{await cua.files.exportArtifact({path:"imports/cross-cell"});}catch(e){return e.code;}')
            require(output[-1]['value']=='PERMISSION_DENIED','file scope expanded to another Run')

            output, _ = execute('navigation-expires-capability-and-screenshotless-pointer', '''
const tab=await cua.getTab("fixture-tab");await tab.capabilities.list();const pw=await tab.capabilities.get("playwright");
await tab.goto("http://fixture.test/next");
let expired;try{await pw.locator("input").count();}catch(e){expired=e.code;}
let pointer;try{await tab.click([1,1]);}catch(e){pointer=e.code;}
return {expired,pointer};
''')
            require(output[-1]['value'] == {'expired': 'STALE_CAPABILITY_HANDLE', 'pointer': 'SCREENSHOT_REQUIRED'}, 'navigation/capture fence missing')
            output, _ = execute('denied-input-does-not-dispatch', f'''const app=await cua.getApp({json.dumps(app)});try{{await app.typeText("must not dispatch");}}catch(e){{return e.code;}}''', approve=False)
            require(output[-1]['value'] == 'APPROVAL_DENIED', 'denied input dispatched')
            output, _ = execute('guest-path-traversal-rejected', '''try{await cua.files.exportArtifact({path:"../private"});}catch(e){return e.code;}''')
            require(output[-1]['value'] == 'PERMISSION_DENIED', 'path traversal accepted')
            with sqlite3.connect(state) as db:
                requests=[json.loads(row[0]) for row in db.execute('SELECT request_json FROM model_generation')]
                selected={ref for req in requests for message in req['messages'] for ref in message.get('image_refs',[])}
                require(selected=={image_ref},'unselected observation entered model image context')
                require(all('screenshot_png_b64' not in json.dumps(req) for req in requests),'binary screenshot entered model ledger')
                artifacts=db.execute("SELECT count(*) FROM artifact WHERE media_type='image/png'").fetchone()[0]
                require(artifacts>len(selected),'nested screenshots were not retained separately')
                report['output_selection']={'model_image_refs':list(selected),'internal_screenshots':artifacts,'binary_in_ledger':False}
            require(h.command('computer', 'status')['phase'] == 'checkpointed', 'idle did not release Computer')
        report['cleanup'] = True
    return report


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--live-config', type=Path)
    parser.add_argument('--live-root', type=Path)
    parser.add_argument('--resume-root', type=Path)
    parser.add_argument('--native-actions-only', action='store_true')
    parser.add_argument('--provision-browser', action='store_true')
    parser.add_argument('--live-browser', action='store_true')
    parser.add_argument('--collect-observations', type=Path, help='read an existing live report and its scoped screenshot Artifacts without starting services')
    args = parser.parse_args()
    if args.collect_observations:
        require(args.resume_root and args.live_root and args.resume_root.parent.resolve()==args.live_root.resolve() and args.resume_root.name.startswith('erii25-cua-use-'), 'collection requires the owned live evidence root')
        report=json.loads(args.collect_observations.read_text())
        require(report.get('scope')=='real Guest' and report.get('cleanup'), 'collection requires a completed live report')
        h=ComputerHarness(args.binary.resolve(),args.resume_root)
        collect_observations(h,report,args.live_root)
        args.evidence.parent.mkdir(parents=True,exist_ok=True)
        args.evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        print(json.dumps({'evidence':str(args.evidence),'screenshots':len(report['screenshots']),'read_only':True}))
        sys.exit(0)
    server=None
    log=None
    compiled=None
    try:
        if args.live_config:
            require(args.live_root is not None,'--live-root is required with --live-config')
            if args.resume_root:
                require(args.resume_root.parent.resolve()==args.live_root.resolve() and args.resume_root.name.startswith('erii25-cua-use-'),'resume must use the owned evidence directory')
            from lume_live import Live
            live=Live(args.live_root,'product')
            live.doctor()
            fixture=Path(__file__).resolve().parents[2]/'tests/fixtures/computer-use-v1/Fixture.swift'
            compiled=args.live_root/'fixtures/erii25Fixture'
            require(not compiled.exists(),'refusing to replace an existing live fixture')
            subprocess.run(['xcrun','swiftc','-target','arm64-apple-macos15.0','-framework','AppKit',str(fixture),'-o',str(compiled)],check=True,timeout=60)
            log=(args.live_root/'logs/erii25-server.log').open('w')
            server=subprocess.Popen([sys.executable,str(Path(__file__).with_name('lume_live.py')),'--root',str(args.live_root),'--profile','product','serve'],stdout=log,stderr=log)
            endpoint=json.loads(args.live_config.read_text())['computer']['endpoint']
            for _ in range(100):
                try: request(endpoint,'/lume/config');break
                except (urllib.error.URLError,ConnectionError): time.sleep(.1)
            else: raise RuntimeError('live Lume API did not start')
        report = run(args.binary.resolve(), args.live_config, args.live_root,args.resume_root,args.native_actions_only,args.provision_browser,args.live_browser)
    finally:
        if server is not None:
            server.send_signal(signal.SIGINT)
            server.wait(timeout=45)
        if log is not None: log.close()
        if compiled is not None and compiled.exists(): compiled.unlink()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'evidence': str(args.evidence), 'checks': len(report['checks']), 'cleanup': report['cleanup']}))
