#!/usr/bin/env python3
"""Real Main -> Code Mode -> UI tool -> durable projection -> sandboxed browser."""
import argparse
from contextlib import closing
from http.server import ThreadingHTTPServer
import json
from pathlib import Path
import sqlite3
import tempfile
import threading

from event_broker import Broker
from feishu_sandbox import Sandbox
from fixture_pool import run_cases
from provider_endpoint import Endpoint
from verify_channels import ChannelHarness, require


def run(binary, evidence):
    with tempfile.TemporaryDirectory(prefix='steward-intelligent-ui-') as temp, Sandbox() as platform, Broker(Path(temp)) as broker:
        server = ThreadingHTTPServer(('127.0.0.1', 0), Endpoint)
        server.requests, server.mode = [], 'success'
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        h = ChannelHarness(binary, Path(temp), platform, '')
        try:
            h.config.write_text(h.config.read_text() + 'llm:\n  provider_bridge_command: ' + json.dumps(str(binary)) + '\n  provider_bridge_script: -model-bridge\n  provider_bridge_version: steward-go-v1\nnats:\n  url: ' + json.dumps(broker.url) + '\n')
            h.setup(metadata=dict(name='Intelligent UI peer', provider_kind='openai_compatible', endpoint=f'http://127.0.0.1:{server.server_port}/v1', wire_api='openai_completions', model_id='fixture-model', context_window=32768, max_output_tokens=4096, compatibility=dict(tool_result_format='openai'), capabilities=dict(text_input=True)))
            server.widgets = [
                dict(title='Comparison', fallback='A is 10 and B is 20.', components=[dict(type='table', columns=['Option', 'Value'], rows=[['A', '10'], ['B', '<img src=x onerror=unsafe()>']])]),
                dict(title='Calculator', fallback='The calculator doubles its input. No network or Host access.', html='<label>Number <input id="n" type="number" value="21"></label><button id="compute">Compute</button><output id="answer">42</output><button id="followup">Discuss this result</button><output id="security"></output>', javascript='const root=document.body;root.querySelector("#compute").addEventListener("click",()=>{const n=Number(root.querySelector("#n").value);root.querySelector("#answer").textContent=Number.isFinite(n)?String(n*2):"Invalid number"});root.querySelector("#followup").addEventListener("click",()=>steward.sendPrompt({text:"Discuss the computed result: "+root.querySelector("#answer").textContent}));let blocked=0;try{parent.document.body}catch{blocked++}try{localStorage.getItem("token")}catch{blocked++}fetch("/api/status").then(()=>root.querySelector("#security").textContent="NETWORK_ALLOWED").catch(()=>root.querySelector("#security").textContent="blocked:"+blocked);'),
                dict(title='Broken visual', fallback='The visual failed; this text remains available.', html='<p>Visual</p>', javascript='throw Error("owned-widget-failure")'),
            ]
            server.mode = 'intelligent-ui'
            h.start()
            h.wait_state(lambda status: status['readiness']=='ready', 'UI runtime ready', lambda _: h.http('/api/status'))
            h.http('/api/message', dict(source_id='intelligent-ui-request', content=[dict(type='text', text='SHOW_INTELLIGENT_UI: compare the values and show a small calculator.')]))
            first = h.wait(lambda s: len(s['deliveries']) == 1 and s['deliveries'][0]['status'] == 'sent', 'UI tool and reply delivered')
            history = h.http('/api/conversation')
            require(not any('steward-result' in json.dumps(m) for m in history), 'UI protocol leaked into prose')
            with closing(sqlite3.connect(h.root / 'data/state.db')) as db, db:
                before = db.execute('SELECT count(*) FROM ui_widget WHERE message_id IS NOT NULL').fetchone()[0]
                require(before == 3, 'UI staging lost or repeated artifacts')
                require(db.execute("SELECT count(*) FROM tool_invocation WHERE tool_name='builtin__intelligent_ui_show' AND status='succeeded' AND attempt_count=1").fetchone()[0] == 3, 'UI bypassed the ordinary executor')
                require(db.execute("SELECT count(*) FROM tool_invocation WHERE tool_name IN ('builtin__skill_list','builtin__skill_view') AND status='succeeded' AND attempt_count=1").fetchone()[0] == 2, 'Skill discovery or loading bypassed the ordinary executor')
                ref = db.execute("SELECT ui_ref FROM ui_widget WHERE json_extract(document_json,'$.title')='Calculator'").fetchone()[0]
            response = h.http('/ui/widget?ref=' + ref)
            require('steward-widget-v1' in response, 'sandbox bridge missing')
            h.http('/ui/widget?ref=' + ref, auth=False, expected=401)
            h.http('/ui/widget?ref=ui:outside', expected=404)
            h.browser(evidence.parent / 'intelligent-ui-browser', 'intelligent-ui')
            h.wait(lambda s: len(s['inputs']) == 2 and len(s['deliveries']) == 2 and all(d['status'] == 'sent' for d in s['deliveries']), 'explicit Widget follow-up accepted')
            h.stop(kill=True)
            h.start()
            h.http('/ui/widget?ref=' + ref)
            with closing(sqlite3.connect(h.root / 'data/state.db')) as db, db:
                require(db.execute('SELECT count(*) FROM ui_widget').fetchone()[0] == before, 'restart regenerated UI')
                runs={r[0] for r in db.execute('SELECT run_id FROM run')}
            requests = [r['body'] for r in server.requests if r.get('method') == 'POST' and r.get('session') in runs and r['body'].get('tools')]
            require(len(requests)>=3, 'business HTTP requests not captured')
            require(all([t['function']['name'] for t in body['tools']] == ['builtin__codemode_execute'] for body in requests), 'UI was declared as a native model tool')
            require(all(sum(m['role'] == 'system' for m in body['messages']) == 1 for body in requests), 'multiple system messages reached the provider')
            require(all('<skills>' not in m['content'] and '<skill_index>' not in m['content'] and 'Present a compact native component' not in m['content'] for body in requests for m in body['messages'] if m['role']=='system'), 'business requests preloaded the Skill catalog')
            return dict(checks=['real Go model peer and Main Code Mode', 'Main-owned durable UI tool; native table and custom HTML/JS', 'opaque origin, storage/parent/network blocked', 'theme, expansion, failure/text fallback, mobile', 'Host-confirmed ordinary message follow-up', 'SIGKILL/restart without regeneration', 'one Agent system message and one model tool'], widget_count=before, projection=first['projections'])
        finally:
            h.cleanup()
            server.shutdown()
            server.server_close()
            thread.join()


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    report = run_cases([0], lambda _: run(args.binary.resolve(), args.evidence.resolve()), 1)[0]
    args.evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(checks=report['checks'], evidence=str(args.evidence))))
