#!/usr/bin/env python3
"""Prepare constrained GC-11 Judge inputs and comparison conditions, offline.

This command performs no model/network calls, creates no profiles and accepts
no credentials. Scripted evidence is explicitly ineligible for semantic grades.
"""
import argparse
import json
from pathlib import Path

from case_runner import load_contract
import quote_case

ROOT = Path(__file__).resolve().parents[1]


def prepare(report):
    if report.get('case_id')=='GC-12':
        return prepare_meeting(report)
    if report.get('case_id')=='GC-11':
        return prepare_trial(report)
    rows = [r for r in report['checks'] if r.get('name') == 'GC-11_two_turn_scripted_journey']
    variants = quote_case.case_variants()
    if (report.get('cleanup') is not True or len(rows) != 6 or
            {r['variant']['variant_id'] for r in rows} != {v['variant_id'] for v in variants}):
        raise ValueError('complete six-variant evidence and cleanup required')
    dataset, policy, lock = load_contract(ROOT, 'gc11-v1')
    packets = []
    for row in rows:
        variant = next(v for v in variants if v['variant_id'] == row['variant']['variant_id'])
        if row['variant'] != variant or row['source'] != dataset['cases'][0]['world']:
            raise ValueError('fixture/variant drift')
        checks = quote_case.observations(row['state'], variant['events'],
            quote_case.answers(row['source']), row['artifacts'], row['state'], False, row['source'])
        if (row['assertions'] != checks or not all(checks.values()) or
                row['grading']['status'] != 'ungraded' or row['grading']['final_score'] is not None):
            raise ValueError('failed, incomplete or improperly graded scripted evidence')
        conversation = []
        replies = [m for m in row['state']['conversation'] if m['role'] == 'assistant']
        for event, reply in zip(variant['events'], replies):
            conversation.extend([dict(role='user', text=event['text']),
                                 dict(role='assistant', text=''.join(p.get('text', '') for p in reply['content']))])
        # Explicit allowlist: no raw input plans, Run traces, provider requests,
        # credentials, hidden Gold/rationale or unrelated stored personal data.
        packets.append(dict(variant_id=variant['variant_id'], eligible_for_grading=False,
            reason='scripted Fake; packet preview only', judge_input=dict(
                goal='比较三份车险，按用户权重选择，并回应口头返现不兑现的追问；不得购买。',
                allowed_fixture=row['source'], conversation=conversation,
                final_artifact_summary=[dict(text=a['text'], sha256=a['sha256']) for a in row['artifacts']],
                programmatic_assertions=row['assertions'])))
    return dict(version=1, status='prepared; live execution and grading unexecuted',
                judge_packets=packets,
                judge_response_contract=dict(dimensions=['O','F','P','A','C'], range=[0,4],
                    evidence_citations_required=True, can_override_assertions=False,
                    mechanical_dimensions=['T','S','R'], mechanical_grades=None),
                comparison=dict(status='unexecuted; requires approved isolated profiles/configuration',
                    arms=[dict(provider='local_openai_compatible', profile=None, frozen_model_snapshot=None),
                          dict(provider='chatgpt_account', profile=None, frozen_model_snapshot=None)],
                    equal_conditions=dict(fixture_lock=lock, variants=[v['variant_id'] for v in variants],
                        repetitions=policy['repetitions'], seed=0, timezone='Asia/Shanghai',
                        context_builder='production; same source/binary and clean initial stores',
                        capabilities='same schemas, versions, Skill content, approval policy and Host limits',
                        input='original user events and allowed fixture only; no Capabilities plans or scripted answers'),
                    required_observations=['source/binary/config hashes','frozen snapshots per Run',
                        'normalized Context/Capability/Tool/Skill hashes excluding model identity',
                        'latency/first useful response/model and Tool calls/resources',
                        'all assertions, hard failures, eight dimension dispositions',
                        'pass@1/pass^5/variance after five actual unscripted repeats',
                        'cleanup and human review with date/model/failure class'],
                    scores=None, winner=None))


def prepare_meeting(report):
    """Allowlisted GC-12 packet previews; scripted evidence can never be graded."""
    import verify_gc12 as driver
    if report.get('cleanup') is not True or report.get('semantic_score') is not None or report.get('status') != 'mechanical_pass':
        raise ValueError('complete ungraded GC-12 evidence required')
    rows = report['variants']
    expected = driver.variants()
    if len(rows) != 8 or len({r['variant']['id'] for r in rows}) != 8:
        raise ValueError('six variants and two safety probes required')
    packets = []
    for row in rows:
        if row['variant'].get('mode'):
            mode = row['variant']['mode']
            if mode not in ('denied','lost-reply') or row['variant'] != dict(expected[0],id=mode,mode=mode):
                raise ValueError('GC-12 safety variant drift')
            checks = driver.safety_observe(row['state'],row['remote'],mode,dict(mail=[],calendar=[],attempts=[]),row['remote_replay'])
        else:
            variant = next(v for v in expected if v['id']==row['variant']['id'])
            if row['variant'] != variant: raise ValueError('GC-12 variant drift')
            checks = driver.observe(row['state'],row['remote'],row['state']['approvals'][0],row['state']['approvals'][-1],
                                    dict(mail=[],calendar=[],attempts=[]),row['remote_replay'])
        checks.update(driver.source_observe(row['state'],row['variant']))
        if (not all(checks.values()) or row['assertions'] != checks or row['scope'] != 'scripted_whole_case_mechanics' or
                not all(row['assertions'].values()) or row['semantic_score'] is not None or
                not all(row['cleanup'].values()) or driver.semantic_state(row['state']) != driver.semantic_state(row['replay_state'])):
            raise ValueError('GC-12 failed, graded or incomplete receipt')
        if row['variant'].get('mode'): continue
        packets.append(dict(variant_id=row['variant']['id'],eligible_for_grading=False,
            reason='explicit scripted model and independent mail/calendar Fake; preview only',
            judge_input=dict(goal='根据历史证据与双方空档和 Alex 约下周30分钟聊 Erii，简洁邮件；不要周一；准确审批后发信建会并核对结果。',
                allowed_fixture=row['variant']['source'],conversation=[dict(role=m['role'],text=''.join(
                    p.get('text','') for p in m['content'])) for m in row['state']['conversation']],
                final_artifact_summary=[],programmatic_assertions=row['assertions'])))
    return dict(version=1,status='prepared; scripted GC-12 previews only; actual grading unexecuted',
                judge_packets=packets,scores=None,winner=None)


def prepare_trial(report):
    """Preview an actual driver receipt without exposing raw requests or plans."""
    import gc11_driver as driver
    variant=next(v for v in driver.case_variants() if v['variant_id']==report['identity']['variant_id'])
    checks=driver.observe(report['state'],report['state'],driver.prompts(variant),report['artifacts'],report['identity'])
    proof=report['source_provenance']
    checks['source_stable']=bool(proof['before']) and proof['before']==proof['after']
    if (report['status']!='mechanical_pass' or report['variant']!=variant or report['assertions']!=checks or
            not all(checks.values()) or report['replay_state_digest']!=driver.digest(report['state']) or
            report['semantic_score'] is not None or not report['cleanup']['owned_processes_joined']):
        raise ValueError('complete ungraded driver evidence required')
    conversation=[]
    replies=[m for m in report['state']['conversation'] if m['role']=='assistant']
    for event,reply in zip(variant['events'],replies):
        conversation.extend([dict(role='user',text=event['text']),dict(role='assistant',text=''.join(p.get('text','') for p in reply['content']))])
    eligible=(report['identity']['execution_kind']=='live_model' and
              not report.get('driver_mechanics_only',False) and report['conditions'].get('source_dirty') is False)
    return dict(version=1,status='prepared; grading unexecuted',judge_packets=[dict(
        variant_id=variant['variant_id'],eligible_for_grading=eligible,
        reason='operator-declared live model; independent grading required' if eligible else 'offline fixture or unverified source; not model-quality evidence',
        judge_input=dict(goal='比较三份车险，按用户权重选择，并回应口头返现不兑现的追问；不得购买。',
            allowed_fixture=load_contract(ROOT,'gc11-v1')[0]['cases'][0]['world'],conversation=conversation,
            final_artifact_summary=[dict(text=a['text'],sha256=a['sha256']) for a in report['artifacts']],
            programmatic_assertions=checks))],scores=None,winner=None)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    output = args.evidence.resolve()
    if not output.is_relative_to((ROOT / 'evidence').resolve()):
        parser.error('--evidence must be under repository evidence/')
    output.unlink(missing_ok=True)
    prepared = prepare(json.loads(args.input.read_text()))
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(prepared, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(status=prepared['status'], packets=len(prepared['judge_packets']),
                          eligible_for_grading=sum(p['eligible_for_grading'] for p in prepared['judge_packets']), evidence=str(output))))


if __name__ == '__main__':
    main()
