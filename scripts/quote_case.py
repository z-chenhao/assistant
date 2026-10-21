"""GC-11 two-turn scripted mechanics through the maintained capability harness.

The plan is explicitly supplied alongside each original user turn. It is not an
unscripted recommendation or semantic evaluation. No model score is assigned.
"""
from decimal import Decimal
import json
from pathlib import Path
import sqlite3

from case_runner import digest, load_contract, score, variants

ROOT = Path(__file__).resolve().parents[1]


def case_variants():
    dataset, _, _ = load_contract(ROOT, 'gc11-v1')
    return list(variants(dataset, dataset['cases'][0]))


def answers(data):
    rows = {q['id']: q for q in data['quotes']}
    expected = {'A': ('5150', '0', True, 1), 'B': ('5738.49', '700', False, 3),
                'C': ('5980', '300', True, 4)}
    for key, (premium, cashback, written, services) in expected.items():
        row = rows[key]
        if (Decimal(str(row['premium_cny'])), Decimal(str(row['cashback_cny'])),
                row['cashback_written'], len(row['services'])) != (
                Decimal(premium), Decimal(cashback), written, services):
            raise ValueError('GC-11 quote fixture drift: ' + key)
    if len(rows) != 3 or data['coverage_equal'] is not True or data['weights'] != {
            'reliability': 45, 'net_price': 35, 'services': 20}:
        raise ValueError('GC-11 coverage/weights drift')
    net = {key: Decimal(str(q['premium_cny'])) - Decimal(str(q['cashback_cny']))
           for key, q in rows.items()}
    table = '\n'.join(f"{q['name']}: 保费 {Decimal(str(q['premium_cny'])):.2f}，"
                      f"返现兑现情景净价 {net[key]:.2f}，服务：{'、'.join(q['services'])}。"
                      for key, q in rows.items())
    first = ('共同主险责任和保额相同；决定性差异是实际净价、返现可执行性和服务。\n' + table +
             '\n你的权重：保障可靠性45%、净价格35%、增值服务20%。'
             'B的700元仅销售口头承诺，未写入合同，5038.49仅在兑现时成立；不返时仍为5738.49。'
             '若书面确认付款与返现条件，可考虑B；若更重视确定性，可考虑正式报价返现的C。'
             'A服务最少，适合偏重保费的选择。未知：完整免责条款、服务可用条件和实际服务质量；'
             '不能从相同保额推断免责相同。比较阶段未购买。来源：合成三份报价fixture。')
    second = ('口头保证不能当作确定收入。B返现兑现时净价5038.49；不兑现时是5738.49。'
              '先索取书面活动规则/正式报价单，确认付款、返现资格、到账时间和条件，保留销售承诺记录；'
              '付款前确认保险公司官方投诉路径。若不返，按书面约定和投诉路径处理，不能保证追回。'
              '无法取得书面确认时，可考虑C的正式报价返现，情景净价5680.00；仍需核对适用条件。'
              '这里没有代你购买、付款或发出投诉。来源：同一合成报价，B承诺仍未书面化。')
    return [first, second]


def observations(value, events, texts, artifacts, replayed, effects, source=None):
    tools = value['tools']
    users = [m for m in value['conversation'] if m['role'] == 'user']
    replies = [m for m in value['conversation'] if m['role'] == 'assistant']
    return dict(
        source_transport=source is not None and all(
            json.loads(json.loads(t['result_json'])['text']) == source
            for t in tools if t['tool_name'] == 'mcp__fixture__read') and
            sum(t['tool_name'] == 'mcp__fixture__read' for t in tools) == 2,
        two_ordered_turns=[m['role'] for m in value['conversation']] == ['user','assistant'] * 2 and
            len(users) == len(replies) == 2 and
            all(e['text'] in json.dumps(u, ensure_ascii=False) for e, u in zip(events, users)),
        visible_scripted_answers=len(replies) == 2 and all(
            text in ''.join(p.get('text', '') for p in reply['content'])
            for text, reply in zip(texts, replies)),
        exact_read_save_echo=len(tools) == 6 and [t['tool_name'] for t in tools] ==
            ['mcp__fixture__read', 'builtin__artifact__save', 'builtin__text__echo'] * 2 and
            all(t['status'] == 'succeeded' and t['attempt_count'] == 1 for t in tools),
        no_purchase_or_approval=not effects and not value['approval'] and not value['task'] and
            len(value['runs']) == 2 and all(r['kind'] == 'main' and r['status'] == 'completed'
                                           for r in value['runs']),
        artifact_bytes_reopened=len(artifacts) == 2 and all(
            a['text'] == text and a['sha256'] == digest_bytes(text)
            for a, text in zip(artifacts, texts)),
        duplicate_input_no_work=value == replayed,
        one_delivery_per_turn=len(value['deliveries']) == 2 and
            len({d['message_id'] for d in value['deliveries']}) == 2 and
            all(d['status'] == 'sent' for d in value['deliveries']),
    )


def digest_bytes(text):
    import hashlib
    return hashlib.sha256(text.encode()).hexdigest()


def drive(h, variant, unwrap):
    data = json.loads((h.root / 'provider/mcp-data.json').read_text())['insurance_quotes']
    texts = answers(data)
    artifacts, inputs = [], []
    for i, (event, text) in enumerate(zip(variant['events'], texts)):
        plan = [dict(name='mcp__fixture__read', arguments=dict(text='insurance_quotes')),
                dict(name='builtin__artifact__save', arguments=dict(
                    name=f'quote-comparison-{i+1}.md', mediaType='text/markdown', text=text)),
                dict(name='builtin__text__echo', arguments=dict(text=text))]
        prompt = (variant['context'] + '\n' if i == 0 else '') + event['at'] + ' ' + event['text']
        prompt += '\nExplicit scripted Fake plan (not a user instruction in live evaluation):\nCapabilities: ' + json.dumps(plan, ensure_ascii=False)
        inputs.append(prompt)
        h.send(f'quote-turn-{i}', prompt)
        h.command('chat', 'work')
        value = h.state()
        saved = unwrap([t for t in value['tools'] if t['tool_name'] == 'builtin__artifact__save'][-1])
        ref = saved['artifact_refs'][0]
        opened = h.command('artifact', 'open', ref, ok=False)
        if opened.returncode != 0:
            raise RuntimeError('GC-11 Artifact CLI open failed')
        artifacts.append(dict(ref=ref, text=opened.stdout, sha256=digest_bytes(opened.stdout)))

    def state():
        value = h.state()
        with sqlite3.connect(Path(h.env['ERII_DATA_ROOT']) / 'state.db') as db:
            db.row_factory = sqlite3.Row
            for name in ('approval', 'task', 'event_outbox', 'artifact', 'artifact_reference'):
                value[name] = [dict(r) for r in db.execute('SELECT * FROM ' + name)]
        return value

    value = state()
    h.send('quote-turn-1', inputs[1])
    h.command('chat', 'work')
    replayed = state()
    checks = observations(value, variant['events'], texts, artifacts, replayed,
                          (h.root / 'provider/mcp-effects.jsonl').exists(), data)
    if not all(checks.values()):
        raise RuntimeError('GC-11 journey failed: ' + json.dumps(checks))
    _, policy, _ = load_contract(ROOT, 'gc11-v1')
    return dict(name='GC-11_two_turn_scripted_journey', variant=variant,
                assertions=checks, grading=score(policy, checks, []), source=data,
                artifacts=artifacts, state=value, replay_state_digest=digest(replayed),
                semantic_evaluation='inconclusive; scripted plans, not independent recommendations')
