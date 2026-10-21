#!/usr/bin/env python3
"""One explicitly owned GC-11 trial through production CLI/Bridge, without plans.

The operator prepares a dedicated installation/profile first. This driver never
logs in, selects a profile, reads credentials or supplies model answers. Retained
trial state belongs to the operator; all driver-owned processes are joined.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import resource
import signal
import sqlite3
import time

from case_runner import digest, load_contract
from fixture_pool import run_cases
from quote_case import case_variants
from verify_main import MainHarness
from verify_suite import source_state, runner_hardware

ROOT = Path(__file__).resolve().parents[1]


def owned_execution(action):
    """Let the maintained Scope unwind/kill owned sessions on CLI termination."""
    previous={sig:signal.getsignal(sig) for sig in (signal.SIGINT,signal.SIGTERM)}
    def interrupted(*_):raise KeyboardInterrupt('evaluation interrupted')
    try:
        for sig in previous:signal.signal(sig,interrupted)
        return action()
    finally:
        for sig,handler in previous.items():signal.signal(sig,handler)


def file_hash(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def prompts(variant):
    return [variant['context'] + '\n' + variant['events'][0]['text'], variant['events'][1]['text']]


def owned(manifest, binary):
    root = manifest.parent.resolve()
    if manifest.is_symlink() or any(p.is_symlink() for p in [manifest.parent, *manifest.parent.parents]):
        raise ValueError('trial paths must not contain symlinks')
    value = json.loads(manifest.read_text())
    fields={'version','issue','execution_kind','arm','repetition','variant_id','turn_timeout_seconds',
            'limits','config_sha256','profile_id','profile_version'}
    if set(value)!=fields or not value['profile_id'] or type(value['profile_version']) is not int or value['profile_version']<1:
        raise ValueError('exact non-secret trial manifest fields required')
    if (value['version'] != 1 or value['issue'] != 'ERII-30' or
            value['execution_kind'] not in ('offline_loopback_fixture', 'live_model') or
            value['arm'] not in ('local', 'account') or type(value['repetition']) is not int or
            not 1 <= value['repetition'] <= 5 or not 1 <= value['turn_timeout_seconds'] <= 300):
        raise ValueError('invalid owned trial identity/limits')
    if value['execution_kind'] == 'offline_loopback_fixture' and value['arm'] != 'local':
        raise ValueError('loopback fixture is not account evidence')
    if set(value['limits']) != {'context_window','max_output_tokens'}:
        raise ValueError('explicit context/output limits required')
    variant = next((v for v in case_variants() if v['variant_id'] == value['variant_id']), None)
    if variant is None:
        raise ValueError('unknown locked variant')
    config = root / 'config.json'
    if config.is_symlink() or file_hash(config) != value['config_sha256']:
        raise ValueError('approved configuration changed')
    cfg = json.loads(config.read_text())
    # This bounded fixture requires only the official Bridge and read-only MCP.
    # Reject richer configurations before admitting any model work.
    if (set(cfg) != {'version', 'app', 'llm', 'mcp'} or cfg['version'] != 1 or
            cfg['app'] != {'data_root': str(root / 'data')} or
            cfg['llm'] != dict(provider_bridge_command='node',
                provider_bridge_script=str(ROOT / 'provider-bridge/bridge.mjs'),
                provider_bridge_version='pi-ai-1.3.0-v1') or
            cfg['mcp'] != [dict(name='fixture', command=str(binary),
                args=['-test-mcp-server', str(root / 'provider')],
                directory=str(root / 'provider'), tools={'read': 'read_only'})]):
        raise ValueError('requires the bounded read-only GC-11 fixture configuration')
    provider = root / 'provider'
    data = root / 'data'
    if (provider.is_symlink() or data.is_symlink() or not data.is_dir() or
            any(p.is_symlink() for p in [provider/'mcp-data.json',data/'state.db',data/'conversation.db'])):
        raise ValueError('owned provider/data directories required')
    dataset, _, lock = load_contract(ROOT, 'gc11-v1')
    if json.loads((provider / 'mcp-data.json').read_text()) != {'insurance_quotes': dataset['cases'][0]['world']}:
        raise ValueError('provider fixture differs from the locked source')
    normalized = json.loads(json.dumps(cfg).replace(str(root), '$TRIAL').replace(str(ROOT), '$SOURCE'))
    return value, variant, config, normalized, lock


class Driver(MainHarness):
    def __init__(self, binary, root, config, timeout):
        super().__init__(binary, root)
        self.config, self.timeout, self.commands = config, timeout, []
        # Credential access stays in the explicitly configured SecretStore.
        # Do not inherit model/API keys from the coding-agent environment.
        allowed = {'PATH','HOME','TMPDIR','GOROOT','GOPATH','GOCACHE','GOTOOLCHAIN','LANG','LC_ALL'}
        self.env = {k: v for k, v in os.environ.items() if k in allowed}
        self.env['ERII_DATA_ROOT'] = str(root / 'data')

    def args(self, *args):
        return [str(self.binary), '-config', str(self.config), *args]

    def command(self, *args, **kwargs):
        started = time.monotonic()
        try:
            return super().command(*args, timeout=self.timeout, **kwargs)
        finally:
            self.commands.append(dict(operation=list(args[:2]), seconds=time.monotonic()-started))

    def state(self):
        value = super().state()
        with sqlite3.connect(self.root / 'data/state.db') as db:
            db.row_factory = sqlite3.Row
            value['agent_id']=db.execute('SELECT agent_id FROM agent_config').fetchone()[0]
            for table in ('task','approval','artifact','artifact_reference','artifact_publication',
                          'event_outbox','automation','code_call','code_execution_state'):
                value[table] = [dict(r) for r in db.execute('SELECT * FROM ' + table)]
            value['provider_usage'] = [dict(r) for r in db.execute(
                'SELECT generation_id,latency_ms,input_tokens,output_tokens,error_code FROM provider_usage '
                'WHERE generation_id IN (SELECT generation_id FROM model_generation)')]
        return value


def normalized_capability(cap):
    return {k: v for k, v in cap.items() if k not in ('snapshot_id','created_at')}


def frozen_model(snapshot):
    return {k: v for k, v in snapshot.items() if k not in ('snapshot_id','created_at')}


def provenance(binary, manifest, config):
    return dict(**source_state(),binary_sha256=file_hash(binary),
                driver_sha256=digest({p:file_hash(ROOT/p) for p in (
                    'scripts/gc11_driver.py','scripts/verify_main.py','scripts/quote_case.py','scripts/case_runner.py')}),
                bridge_sha256=file_hash(ROOT/'provider-bridge/bridge.mjs'),
                bridge_lock_sha256=file_hash(ROOT/'provider-bridge/package-lock.json'),
                config_sha256=file_hash(config),manifest_sha256=file_hash(manifest),
                provider_data_sha256=file_hash(manifest.parent/'provider/mcp-data.json'))


def derived_conditions(value, identity, variant):
    requests=[json.loads(g['request_json']) for g in value['generations']]
    first=requests[0] if requests else {}
    caps=[normalized_capability(c) for c in value['capabilities']]
    for cap in caps:
        if cap['data_scope']!=value['agent_id']:
            raise ValueError('capability scope is not the owned Agent')
        # Independent fresh installations have different trusted Agent IDs.
        # Normalize that namespace only; retain every permission/schema flag.
        cap['data_scope']='$OWNED_AGENT'
    return dict(inputs=prompts(variant),limits=identity['limits'],
                turn_timeout_seconds=identity['turn_timeout_seconds'],
                capability=caps[0] if caps else None,initial_messages=first.get('messages'),
                tools=first.get('tools'),generation_defaults=first.get('model_snapshot',{}).get('generation_defaults'))


def observe(value, replayed, inputs, artifacts, identity):
    messages, calls = value['conversation'], value['tools']
    requests = [json.loads(g['request_json']) for g in value['generations']]
    models = [r['model_snapshot'] for r in requests]
    caps = {c['snapshot_id']: c for c in value['capabilities']}
    runs = {r['run_id']: r for r in value['runs']}
    tools = {c['snapshot_id']: {t['name']: t['permission'] for t in json.loads(c['tools_json'])}
             for c in value['capabilities']}
    source = load_contract(ROOT, 'gc11-v1')[0]['cases'][0]['world']
    reads = [t for t in calls if t['tool_name'] == 'mcp__fixture__read' and t['status'] == 'succeeded']
    stored={a['artifact_ref']:a for a in value['artifact']}
    kind='openai_compatible' if identity['arm']=='local' else 'openai_account'
    return dict(
        original_two_turns=[m['role'] for m in messages] == ['user','assistant'] * 2 and
            [m['content'][0].get('text') for m in messages if m['role']=='user'] == inputs,
        nonempty_visible_replies=len([m for m in messages if m['role']=='assistant']) == 2 and
            all(any(p.get('text','').strip() for p in m['content']) for m in messages if m['role']=='assistant'),
        synchronous_main=len(runs)==2 and all(r['kind']=='main' and r['status']=='completed' for r in runs.values()) and
            not value['task'] and not value['automation'],
        no_effect_or_approval=not value['approval'] and all(
            tools.get(runs.get(t['run_id'],{}).get('capability_snapshot_id'),{}).get(t['tool_name'])
            in ('read_only','artifact_write') for t in calls),
        quote_source_observed=all(
            json.loads(json.loads(t['result_json'])['text'])==source for t in reads),
        tools_completed=all(t['status']=='succeeded' and t['attempt_count']>=1 for t in calls),
        artifact_readback=len(artifacts)==len(value['artifact']) and {a['ref'] for a in artifacts}==set(stored) and all(
            a['verified'] and a['sha256']==hashlib.sha256(a['text'].encode()).hexdigest()==stored[a['ref']]['content_hash'] and
            len(a['text'].encode())==stored[a['ref']]['byte_size'] for a in artifacts),
        frozen_profile=bool(models) and all(s['profile_id']==identity['profile_id'] and
            s['profile_version']==identity['profile_version'] and s['provider_kind']==kind and
            all(s[k]==v for k,v in identity['limits'].items()) for s in models) and
            all(frozen_model(s)==frozen_model(models[0]) for s in models) and
            all(r['model_snapshot']['snapshot_id']==runs[r['run_id']]['model_snapshot_id'] for r in requests),
        frozen_capability=bool(caps) and all(normalized_capability(c)==normalized_capability(next(iter(caps.values()))) for c in caps.values()) and
            all(not c['can_spawn_agents'] and c['role']=='main' and c['data_scope']==value['agent_id'] for c in caps.values()) and
            bool(requests) and all(r['tools']==requests[0]['tools'] for r in requests),
        generations_complete=bool(requests) and all(g['status']=='completed' for g in value['generations']),
        delivered_once=len(value['deliveries'])==2 and all(d['status']=='sent' for d in value['deliveries']) and
            len({d['message_id'] for d in value['deliveries']})==2,
        duplicate_input_no_work=value==replayed,
    )


def trial(binary, manifest):
    identity, variant, config, normalized, lock = owned(manifest, binary)
    h = Driver(binary, manifest.parent.resolve(), config, identity['turn_timeout_seconds'])
    started, cpu = time.monotonic(), resource.getrusage(resource.RUSAGE_CHILDREN)
    initial_source=provenance(binary,manifest,config)
    doctor={}
    try:
        status = h.command('status')
        profile = next((p for p in h.command('model','list') if p['profile_id']==identity['profile_id']), None)
        expected = 'openai_compatible' if identity['arm']=='local' else 'openai_account'
        doctor=dict(status=status,profile=None if profile is None else {k:profile.get(k) for k in
                    ('profile_id','version','provider_kind','context_window','max_output_tokens','status')})
        if (status['readiness']!='ready' or status['active_model_profile_id']!=identity['profile_id'] or
                profile is None or profile['version']!=identity['profile_version'] or
                profile['provider_kind']!=expected or any(profile[k]!=v for k,v in identity['limits'].items())):
            raise ValueError('approved exact profile/limits must already be active and ready')
        before = h.state()
        if any(before[k] for k in ('runs','conversation','tools','task','approval','artifact','automation')):
            raise ValueError('trial installation must have no prior conversation or domain work')
        inputs = prompts(variant)
        first_reply = None
        for i, text in enumerate(inputs):
            h.send('gc11-turn-'+str(i), text)
            h.command('chat','work')
            if i==0:
                first_reply=time.monotonic()-started
        value = h.state()
        artifacts = []
        for a in value['artifact']:
            if a['byte_size'] > 1_000_000:
                raise ValueError('quote Artifact exceeds bounded evidence size')
            opened = h.command('artifact','open',a['artifact_ref'],ok=False)
            raw = opened.stdout.encode()
            artifacts.append(dict(ref=a['artifact_ref'],text=opened.stdout,sha256=hashlib.sha256(raw).hexdigest(),
                verified=opened.returncode==0 and hashlib.sha256(raw).hexdigest()==a['content_hash'] and len(raw)==a['byte_size']))
        h.send('gc11-turn-1', inputs[1]); h.command('chat','work')
        replayed = h.state()
        checks = observe(value,replayed,inputs,artifacts,identity)
        final_source=provenance(binary,manifest,config)
        stable=initial_source==final_source
        checks['source_stable']=stable
        requests = [json.loads(g['request_json']) for g in value['generations']]
        conditions = dict(**{k:v for k,v in initial_source.items() if k not in ('config_sha256','manifest_sha256')},
            fixture_lock=lock,config=normalized,
            **derived_conditions(value,identity,variant),
            )
        after_cpu=resource.getrusage(resource.RUSAGE_CHILDREN)
        return dict(version=1,case_id='GC-11',identity=identity,variant=variant,
            status='mechanical_pass' if all(checks.values()) else 'mechanical_failure',
            assertions=checks,state=value,artifacts=artifacts,conditions=conditions,
            conditions_digest=digest(conditions),snapshots=[r['model_snapshot'] for r in requests],
            replay_state_digest=digest(replayed),
            source_provenance=dict(before=initial_source,after=final_source),
            semantic_score=None,golden_case_status='inconclusive; no semantic grades',
            metrics=dict(wall_seconds=time.monotonic()-started,first_completed_reply_seconds=first_reply,
                first_useful_feedback_seconds=None,logical_model_generations=len(requests),
                generation_attempts=sum(g['attempt_count'] for g in value['generations']),
                provider_network_attempts=None,tool_dispatch_attempts=sum(t['attempt_count'] for t in value['tools']),
                tool_invocations=len(value['tools']),
                cli_host_hardware=runner_hardware(),external_model_resources=None,
                child_cpu_seconds=after_cpu.ru_utime+after_cpu.ru_stime-cpu.ru_utime-cpu.ru_stime),
            commands=h.commands,cleanup=dict(owned_processes_joined=True,state_retained_for_operator=True))
    except Exception as error:
        # Failed work is retained explicitly; never leave a prior success report.
        try:
            failed_state=h.state()
        except Exception as read_error:
            failed_state=dict(readback_error=type(read_error).__name__)
        return dict(version=1,case_id='GC-11',identity=identity,variant=variant,
                    status='mechanical_failure',failure=type(error).__name__,
                    reason=str(error),state=failed_state,doctor=doctor,
                    semantic_score=None,golden_case_status='inconclusive',commands=h.commands,
                    cleanup=dict(owned_processes_joined=True,state_retained_for_operator=True))
    finally:
        h.cleanup()


def preflight(reports, binary=None):
    expected={(a,v['variant_id'],n) for a in ('local','account') for v in case_variants() for n in range(1,6)}
    indexed={}
    seen_runs=set()
    for r in reports:
        i=r['identity'];key=(i['arm'],i['variant_id'],i['repetition'])
        if key in indexed or key not in expected:
            raise ValueError('duplicate or unknown comparison trial')
        if r['status']!='mechanical_pass':
            raise ValueError('failed comparison trial')
        run_ids={run['run_id'] for run in r['state']['runs']}
        if len(run_ids)!=2 or seen_runs & run_ids:
            raise ValueError('reused comparison Run evidence')
        seen_runs.update(run_ids)
        checks=observe(r['state'],r['state'],prompts(r['variant']),r['artifacts'],i)
        proof=r['source_provenance']
        source_fields={'source_sha','source_dirty','source_diff_sha256','binary_sha256','driver_sha256',
                       'bridge_sha256','bridge_lock_sha256','provider_data_sha256','config_sha256','manifest_sha256'}
        if set(proof['before'])!=source_fields or set(proof['after'])!=source_fields:
            raise ValueError('incomplete frozen source provenance')
        checks['source_stable']=bool(proof['before']) and proof['before']==proof['after']
        variant=next(v for v in case_variants() if v['variant_id']==i['variant_id'])
        derived=derived_conditions(r['state'],i,variant)
        if (r['conditions_digest']!=digest(r['conditions']) or r['semantic_score'] is not None or
                r['assertions']!=checks or not all(checks.values()) or r['replay_state_digest']!=digest(r['state']) or
                r['variant']!=variant or any(r['conditions'].get(k)!=v for k,v in derived.items()) or
                r['conditions'].get('fixture_lock')!=load_contract(ROOT,'gc11-v1')[2] or
                r['conditions'].get('source_dirty') is not False or
                any(r['conditions'].get(k)!=v for k,v in proof['before'].items() if k not in ('config_sha256','manifest_sha256')) or
                r['snapshots']!=[json.loads(g['request_json'])['model_snapshot'] for g in r['state']['generations']] or
                r['cleanup']['owned_processes_joined'] is not True):
            raise ValueError('invalid, failed, graded or unjoined trial')
        indexed[key]=r
    if set(indexed)!=expected:
        raise ValueError('requires exactly two arms, six variants and five repetitions')
    common=lambda r:{k:v for k,v in r['conditions'].items() if k not in ('inputs','initial_messages')}
    if any(common(r)!=common(reports[0]) for r in reports):
        raise ValueError('source/configuration/limit drift across variants')
    for v in case_variants():
        reference=indexed['local',v['variant_id'],1]['conditions']
        if any(indexed[a,v['variant_id'],n]['conditions']!=reference for a in ('local','account') for n in range(1,6)):
            raise ValueError('frozen comparison condition drift: '+v['variant_id'])
    live=all(r['identity']['execution_kind']=='live_model' and not r.get('driver_mechanics_only',False) for r in reports)
    if live:
        source=source_state();conditions=reports[0]['conditions']
        driver_hash=digest({p:file_hash(ROOT/p) for p in (
            'scripts/gc11_driver.py','scripts/verify_main.py','scripts/quote_case.py','scripts/case_runner.py')})
        if (binary is None or source['source_dirty'] or source['source_sha']!=conditions['source_sha'] or
                file_hash(binary)!=conditions['binary_sha256'] or driver_hash!=conditions['driver_sha256']):
            raise ValueError('live cohort must match the current clean source/driver and explicit binary')
    return dict(status='conditions_match; grading unexecuted' if live else 'inconclusive; offline fixture cohort',
                trials=len(reports),eligible_for_model_comparison=live,scores=None,winner=None,
                snapshots={a:[dict(variant_id=r['identity']['variant_id'],repetition=r['identity']['repetition'],
                                  model_snapshot=r['snapshots'][0]) for r in reports if r['identity']['arm']==a] for a in ('local','account')})


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary',type=Path)
    group=parser.add_mutually_exclusive_group(required=True)
    group.add_argument('--trial',type=Path)
    group.add_argument('--compare',type=Path,help='Directory of sixty completed trial JSON reports')
    parser.add_argument('--evidence',type=Path,required=True)
    args=parser.parse_args()
    if not args.evidence.resolve().is_relative_to((ROOT/'evidence').resolve()):
        parser.error('--evidence must be under repository evidence/')
    args.evidence.unlink(missing_ok=True)
    if args.trial:
        if not args.binary: parser.error('--binary required for --trial')
        report=owned_execution(lambda:run_cases([args.trial.absolute()],lambda m:trial(args.binary.resolve(),m),1)[0])
    else:
        report=preflight([json.loads(p.read_text()) for p in sorted(args.compare.glob('*.json'))],args.binary.resolve() if args.binary else None)
    args.evidence.parent.mkdir(parents=True,exist_ok=True)
    args.evidence.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(dict(status=report['status'],evidence=str(args.evidence))))
    return 2 if report['status']=='mechanical_failure' else 0


if __name__=='__main__':
    raise SystemExit(main())
