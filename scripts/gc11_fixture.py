"""Deterministic HTTP peer for driver contracts, never model-quality evidence."""
from http.server import ThreadingHTTPServer
import json
from pathlib import Path
import tempfile
import threading
import sys
import io
from contextlib import redirect_stdout
from unittest.mock import patch

from case_runner import load_contract
from fixture_pool import run_cases
import gc11_driver as driver


def reply(body, chunk):
    messages=body['messages']
    last=max(i for i,m in enumerate(messages) if m['role']=='user')
    tool_results=[m for m in messages[last+1:] if m['role']=='tool']
    turn=sum(m['role']=='user' for m in messages)
    if not tool_results:
        name,args='mcp__fixture__read',dict(text='insurance_quotes')
    elif len(tool_results)==1:
        source=json.loads(tool_results[0]['content'])
        # Derive an observation from actual Tool bytes, without Gold advice.
        text=json.dumps(dict(quote_ids=[q['id'] for q in source['quotes']],
                             current_user_input=messages[last]['content']),ensure_ascii=False)
        name,args='builtin__artifact__manage',dict(operation='save',name=f'observation-{turn}.json',mediaType='application/json',text=text)
    else:
        chunk({'content':f'Observed quote source and saved turn {turn}.'})
        chunk({},'stop')
        return
    chunk({'tool_calls':[dict(index=0,id='quote-observation',type='function',
                              function=dict(name=name,arguments=json.dumps(args)))]})
    chunk({},'tool_calls')


def verify(binary, bridge, endpoint_handler, evidence=None):
    def execute(_):
        server=ThreadingHTTPServer(('127.0.0.1',0),endpoint_handler)
        server.daemon_threads=True
        server.requests,server.mode=[],'success'
        thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        try:
            with tempfile.TemporaryDirectory(prefix='erii-gc11-driver-') as temp:
                root=Path(temp).resolve();(root/'provider').mkdir()
                dataset=load_contract(driver.ROOT,'gc11-v1')[0]
                (root/'provider/mcp-data.json').write_text(json.dumps({'insurance_quotes':dataset['cases'][0]['world']}))
                cfg=dict(version=1,app=dict(data_root=str(root/'data')),
                    llm=dict(provider_bridge_command='node',provider_bridge_script=str(bridge),provider_bridge_version='pi-ai-1.3.0-v1'),
                    mcp=[dict(name='fixture',command=str(binary),args=['-test-mcp-server',str(root/'provider')],
                              directory=str(root/'provider'),tools=dict(read='read_only'))])
                config=root/'config.json';config.write_text(json.dumps(cfg))
                h=driver.Driver(binary,root,config,30)
                profile=h.command('model','add',f'http://127.0.0.1:{server.server_port}/v1','fixture-model')
                h.command('model','test',profile['profile_id'])
                h.command('model','select',profile['profile_id'],str(profile['version']),str(h.command('status')['config_version']))
                profile=next(p for p in h.command('model','list') if p['profile_id']==profile['profile_id'])
                marker=dict(version=1,issue='ERII-30',execution_kind='offline_loopback_fixture',arm='local',
                    variant_id='wording-1',repetition=1,profile_id=profile['profile_id'],profile_version=profile['version'],
                    limits={k:profile[k] for k in ('context_window','max_output_tokens')},
                    config_sha256=driver.file_hash(config),turn_timeout_seconds=30)
                manifest=root/'erii-eval.json';manifest.write_text(json.dumps(marker))
                server.mode='gc11';start=len(server.requests)
                evidence_root=driver.ROOT/'evidence';evidence_root.mkdir(exist_ok=True)
                with tempfile.TemporaryDirectory(dir=evidence_root,prefix='gc11-driver-cli-') as output:
                    output=Path(output)
                    command=[sys.executable,str(driver.ROOT/'scripts/gc11_driver.py'),'--binary',str(binary),
                             '--trial',str(manifest),'--evidence',str(output/'trial.json')]
                    # Exercise argparse/output/exit semantics in the current
                    # owned Scope. Native Go/Bridge/MCP calls remain real child
                    # processes; avoid nesting independent ownership sessions.
                    def call_cli(arguments):
                        with patch.object(sys,'argv',arguments[1:]),redirect_stdout(io.StringIO()):
                            return driver.main()
                    completed=call_cli(command)
                    report=json.loads((output/'trial.json').read_text())
                    requests=server.requests[start:]
                    # Exercise the same executable again; it must reject the
                    # populated installation before a new model HTTP request.
                    rejected=call_cli(command[:-1]+[str(output/'rejected.json')])
                    again=json.loads((output/'rejected.json').read_text())
                    if rejected!=2 or again['status']!='mechanical_failure' or len(server.requests)!=start+len(requests):
                        raise RuntimeError('used trial installation was not rejected')
                if report['status']!='mechanical_pass':
                    if evidence:
                        evidence.parent.mkdir(parents=True,exist_ok=True)
                        evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
                    raise RuntimeError('GC-11 production driver: '+str(report.get('assertions',report.get('failure'))))
                if completed!=0:raise RuntimeError('driver CLI failed')
                bodies=[r['body'] for r in requests if r['method']=='POST']
                raw=json.dumps(bodies,ensure_ascii=False)
                assert 'Capabilities: ' not in raw and 'Delegate capabilities:' not in raw
                assert all(r.get('auth') is None for r in requests)
                assert len(report['artifacts'])==2 and all(a['verified'] for a in report['artifacts'])
                report.update(http_requests=bodies,reused_installation_rejected=True,
                              driver_mechanics_only=True,semantic_score=None)
                h.cleanup()
            report['cleanup']['fixture_root_removed']=not root.exists()
            report['cleanup']['state_retained_for_operator']=False
            return report
        finally:
            server.shutdown();server.server_close();thread.join(timeout=5)
            if thread.is_alive():raise RuntimeError('GC-11 endpoint did not stop')
    return driver.owned_execution(lambda:run_cases([0],execute,1)[0])
