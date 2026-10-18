"""Normative seed1701 data; input text contains clues, never hidden labels."""
from collections import Counter
import csv
import io
import random

CATEGORIES = dict(product_bug=28, billing=22, feature_request=20, usability=15,
                  service_outage=10, insufficient_evidence=5)
SEVERITIES = dict(critical=8, high=22, medium=45, low=20, unknown=5)
EDGES = {7: ('service_outage', 'critical', '页面打不开，影响全公司今天投放'),
         21: ('billing', 'high', '扣款两次，订单只有一份'),
         55: ('feature_request', 'low', '希望增加深色模式'),
         99: ('insufficient_evidence', 'unknown', '不好用')}
PAIRS = [(12, 78), (33, 34), (60, 91)]


def feedback(seed=1701):
    rng = random.Random(seed)
    labels = {i: (c, s) for i, (c, s, _) in EDGES.items()}
    for a, b in PAIRS:
        labels[a] = labels[b] = ('product_bug', 'medium')
    remaining = sorted(set(range(1, 101)) - set(labels))
    rng.shuffle(remaining)
    cat_left = Counter(CATEGORIES) - Counter(c for c, _ in labels.values())
    sev_left = Counter(SEVERITIES) - Counter(s for _, s in labels.values())
    # Reserve evidence-poor rows so category and severity are both unknown.
    unknown_ids = remaining[:cat_left['insufficient_evidence']]
    for i in unknown_ids:
        labels[i] = ('insufficient_evidence', 'unknown')
    remaining = remaining[len(unknown_ids):]
    cats = [c for c, n in cat_left.items() if c != 'insufficient_evidence' for _ in range(n)]
    sevs = [s for s, n in sev_left.items() if s != 'unknown' for _ in range(n)]
    rng.shuffle(cats); rng.shuffle(sevs)
    labels.update({i: (c, s) for i, c, s in zip(remaining, cats, sevs)})
    phrases = dict(product_bug='程序错误：保存失败', billing='扣款金额错误',
                   feature_request='希望增加导出选项', usability='操作步骤难理解',
                   service_outage='服务中断', insufficient_evidence='没有补充说明')
    qualifiers = dict(critical='紧急', high='严重', medium='一般', low='轻微', unknown='')
    rows, gold = [], []
    attachments = {1, 20, 40, 60, 80}
    for i in range(1, 101):
        c, s = labels[i]
        text = f'{phrases[c]}，{qualifiers[s]}'
        if i in EDGES:
            text = EDGES[i][2]
        rows.append(dict(row_id=f'F{i:03d}', text=text,
                         attachment_ref=f'feedback_attachment_{i:03d}.txt' if i in attachments else ''))
        gold.append(dict(row_id=f'F{i:03d}', category=c, severity=s))
    for a, b in PAIRS:
        rows[b - 1]['text'] = rows[a - 1]['text']
    return rows, gold


def csv_text(rows):
    output = io.StringIO(newline='')
    writer = csv.DictWriter(output, fieldnames=list(rows[0]), lineterminator='\n')
    writer.writeheader(); writer.writerows(rows)
    return output.getvalue()
