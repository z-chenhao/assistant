"""Private release evidence readers. Hashes bind bytes; they do not confer trust.

The caller supplies an independently acquired, hash-pinned evidence pack. No
network, model call, policy DSL or product permission is introduced here.
"""
from datetime import datetime, timezone
import hashlib
import json
import math
from pathlib import Path
import subprocess

from case_runner import digest, score

DIMENSIONS = set('OFPATSRC')
SEMANTIC = set('OFPAC')
MECHANICAL = set('TSR')
CASES = {f'GC-{n:02}' for n in range(1, 21)}
VARIANTS = {'wording-1', 'wording-2', 'wording-3', 'noise-1', 'noise-2', 'order-1'}
ARMS = {'local', 'account'}
PACKET_FIELDS = {'goal', 'allowed_fixture', 'conversation', 'final_artifact_summary',
                 'programmatic_assertions'}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def strict_json(data):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, 'duplicate JSON key: ' + key)
            result[key] = value
        return result
    def invalid(value):
        raise ValueError('non-finite JSON number: ' + value)
    def finite(value):
        number = float(value)
        require(math.isfinite(number), 'non-finite JSON number')
        return number
    return json.loads(data, object_pairs_hook=unique, parse_constant=invalid, parse_float=finite)


def hashed(data):
    return hashlib.sha256(data).hexdigest()


def hash_value(value):
    return isinstance(value, str) and len(value) == 64 and all(c in '0123456789abcdef' for c in value)


def git(root, *args):
    result = subprocess.run(['git', *args], cwd=root, capture_output=True)
    require(result.returncode == 0, 'git evidence unavailable: ' + ' '.join(args))
    return result.stdout


def timestamp(value):
    result = datetime.fromisoformat(value.replace('Z', '+00:00'))
    require(result.tzinfo is not None, 'timestamp must include timezone')
    return result.astimezone(timezone.utc)


def checks(value, expected=None):
    require(isinstance(value, dict) and bool(value) and
            all(type(v) is bool for v in value.values()), 'explicit boolean assertions required')
    if expected is not None:
        require(set(value) == set(expected), 'assertion inventory mismatch')
    return all(value.values())


class Receipts:
    def __init__(self, folder, binding, earliest=None):
        self.folder = folder.resolve()
        self.binding = binding
        self.used = set()
        self.gate_executions = set()
        self.earliest = earliest

    def bytes(self, ref):
        require(isinstance(ref, dict) and set(ref) == {'path', 'sha256'}, 'exact file reference required')
        path = Path(ref['path'])
        require(not path.is_absolute() and '..' not in path.parts, 'unsafe evidence path')
        target = self.folder / path
        require(target.resolve().is_relative_to(self.folder) and target.is_file(), 'missing evidence file')
        require(not any(p.is_symlink() for p in [target, *target.parents] if p != self.folder.parent),
                'symlink evidence rejected')
        data = target.read_bytes()
        require(hashed(data) == ref['sha256'], 'evidence hash mismatch: ' + str(path))
        return data

    def read(self, ref, kind, unique=True):
        if unique:
            require(ref['sha256'] not in self.used, 'duplicate evidence receipt')
            self.used.add(ref['sha256'])
        value = strict_json(self.bytes(ref))
        require(value.get('kind') == kind and value.get('binding') == self.binding,
                'receipt kind/source/model suite binding mismatch')
        return value

    def raw(self, refs):
        require(isinstance(refs, list) and bool(refs), 'native raw evidence required')
        for ref in refs:
            self.bytes(ref)


def review(store, ref, subject, now, author=None, human=False):
    value = store.read(ref, 'independent_review', unique=False)
    require(value.get('subject_sha256') == subject and value.get('mode') == 'acceptance',
            'review subject or mode mismatch')
    require(isinstance(value.get('reviewer'), str) and bool(value['reviewer'].strip()) and
            value['reviewer'] != author, 'named independent reviewer required')
    require(bool(value.get('reason')) and bool(value.get('origin')), 'review provenance required')
    require(value.get('actor_type') in ('agent', 'member') and
            (not human or value['actor_type'] == 'member'), 'named human review required')
    store.raw(value['raw_evidence'])
    require(timestamp(value['reviewed_at']) <= now, 'future review')
    require(store.earliest is None or timestamp(value['reviewed_at']) >= store.earliest, 'review predates release source')
    if timestamp(value['expires_at']) < now:
        return 'expired'
    require(value.get('verdict') in ('PASS', 'REJECT', 'INCONCLUSIVE'), 'unknown review verdict')
    return {'PASS': 'verified', 'REJECT': 'rejected', 'INCONCLUSIVE': 'unmet'}[value['verdict']]


def model_identity(snapshot):
    required = {'provider_kind', 'model_id', 'wire_api', 'context_window', 'max_output_tokens',
                'generation_defaults', 'compatibility', 'capabilities'}
    require(required <= snapshot.keys(), 'incomplete model identity')
    require(all(isinstance(snapshot[k], str) and snapshot[k].strip()
                for k in ('provider_kind', 'model_id', 'wire_api')), 'invalid model identity')
    require(all(isinstance(snapshot[k], dict) for k in ('generation_defaults', 'compatibility', 'capabilities')),
            'incomplete frozen model metadata')
    require(all(type(snapshot[k]) is int and snapshot[k] > 0 for k in ('context_window', 'max_output_tokens')),
            'invalid model limits')
    require(snapshot['max_output_tokens'] <= snapshot['context_window'] and
            snapshot['wire_api'] in ('openai_completions', 'openai_responses') and
            snapshot['capabilities'].get('text_input') is True and
            snapshot['compatibility'].get('tool_result_format') in ('openai', 'provider_specific'),
            'invalid production model contract')
    if snapshot['provider_kind'] == 'openai_compatible':
        require(hash_value(snapshot.get('endpoint_hash')), 'local endpoint identity required')
    # Only installation-generated identity is removed. Unknown/future model
    # parameters remain visible and participate in comparison.
    return {k: v for k, v in snapshot.items() if k not in
            ('snapshot_id', 'created_at', 'profile_id', 'profile_version', 'auth_connection_id')}


def citation(packet, item):
    require(set(item) == {'pointer', 'quote'} and item['pointer'].startswith('/'), 'invalid Judge citation')
    value = packet
    for part in item['pointer'][1:].split('/'):
        part = part.replace('~1', '/').replace('~0', '~')
        value = value[int(part)] if isinstance(value, list) else value[part]
    require(isinstance(item['quote'], str) and bool(item['quote']) and
            item['quote'] in json.dumps(value, ensure_ascii=False), 'Judge citation does not resolve')


def dimension(value, packet):
    require(set(value) == {'score', 'reason', 'citations'}, 'exact dimension receipt required')
    require(bool(value['reason']) and bool(value['citations']), 'dimension reasoning/citations required')
    for item in value['citations']:
        citation(packet, item)
    number = value['score']
    require(number is None or (type(number) in (int, float) and math.isfinite(number) and 0 <= number <= 4),
            'invalid dimension score')
    return number


def trial(store, ref, judge_ref, plan, policy, models):
    value = store.read(ref, 'trial')
    identity = value['identity']
    require(set(identity) == {'case_id', 'variant_id', 'arm', 'repetition'}, 'invalid trial identity')
    case, variant, arm, repetition = (identity[k] for k in ('case_id', 'variant_id', 'arm', 'repetition'))
    require(case in CASES and variant in VARIANTS and arm in ARMS and
            type(repetition) is int and 1 <= repetition <= 5, 'unknown case/variant/arm/repetition')
    require(value.get('execution_kind') == 'live_model' and value.get('eligible_for_grading') is True and
            value.get('scope') == 'whole_case' and value.get('cleanup') is True,
            'ineligible/scripted/partial/unclean trial')
    require(model_identity(value['model_snapshot']) == models[arm], 'model drift within comparison arm')
    require(value['conditions'] == plan[case]['conditions'][variant], 'comparison conditions drift')
    require(value['conditions']['limits'] == {k: value['model_snapshot'][k] for k in
            ('context_window', 'max_output_tokens')}, 'model and comparison limits differ')
    require(bool(value.get('execution_id')) and bool(value.get('runner')), 'native execution/runner identity required')
    checks(value['assertions'], plan[case]['assertions'])
    require(set(value['hard_failure_checks']) == set(policy['hard_failure_categories']), 'hard failure coverage missing')
    checks(value['hard_failure_checks'])
    failures = [k for k, safe in value['hard_failure_checks'].items() if not safe]
    store.raw(value['raw_evidence'])
    packet = strict_json(store.bytes(value['judge_input']))
    require(set(packet) == PACKET_FIELDS and packet['programmatic_assertions'] == value['assertions'],
            'Judge input leakage or assertion drift')
    # A Judge cannot edit assertions, applicability, or mechanical grades.
    receipt = store.read(judge_ref, 'judge')
    require(receipt.get('trial_sha256') == ref['sha256'] and
            receipt.get('judge_input_sha256') == value['judge_input']['sha256'], 'Judge receipt binding mismatch')
    require(bool(receipt.get('judge_model')) and receipt.get('judge_actor') != value.get('runner') and
            bool(receipt.get('judge_actor')), 'independent Judge identity required')
    response = strict_json(store.bytes(receipt['native_response']))
    require(set(response) == {'dimensions'} and set(response['dimensions']) == SEMANTIC,
            'Judge must cover exactly O/F/P/A/C')
    require(set(value['mechanical_dimensions']) == MECHANICAL, 'mechanical T/S/R coverage missing')
    dispositions = dict(response['dimensions'], **value['mechanical_dimensions'])
    grades = {d: dimension(dispositions[d], packet) for d in DIMENSIONS}
    # Every dimension is disposed. Explicit N/A is frozen by independent scope
    # review, has a reason, and removes its weight via the existing scorer.
    applicable = plan[case]['applicable_dimensions']
    require(set(applicable) <= DIMENSIONS and bool(applicable) and len(set(applicable)) == len(applicable),
            'invalid dimension applicability')
    for d in DIMENSIONS - set(applicable):
        require(grades[d] is None and bool(plan[case]['not_applicable'].get(d)), 'unreviewed dimension waiver')
    scored = score(policy, value['assertions'], failures,
                   None if any(grades[d] is None for d in applicable) else {d: grades[d] for d in applicable}, applicable)
    exact = (0 if failures or not all(value['assertions'].values()) else
             None if scored['final_score'] is None else
             sum(policy['weights'][d] * grades[d] / 4 for d in applicable) /
             sum(policy['weights'][d] for d in applicable) * 100)
    metrics = value['metrics']
    required_metrics = {'latency_ms', 'first_useful_feedback_ms', 'model_calls', 'tool_calls', 'resources',
                        'user_questions', 'approvals', 'unhelpful_notifications', 'durable', 'memory', 'coding'}
    require(set(metrics) == required_metrics, 'release metric inventory missing')
    for key in ('latency_ms', 'first_useful_feedback_ms', 'model_calls', 'tool_calls',
                'user_questions', 'approvals', 'unhelpful_notifications'):
        number = metrics[key]
        require(number is None or (type(number) in (int, float) and math.isfinite(number) and number >= 0),
                'invalid release metric')
    for key in ('resources', 'durable', 'memory', 'coding'):
        require(metrics[key] is None or (isinstance(metrics[key], dict) and bool(metrics[key])), 'metric observations required')
    return dict(identity=identity, execution_id=value['execution_id'], dimensions=grades,
                metrics=metrics, metrics_complete=all(v is not None for v in metrics.values()),
                hard_failures=failures, exact_score=exact, **scored)
