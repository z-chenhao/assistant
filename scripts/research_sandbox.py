#!/usr/bin/env python3
"""Read-only GC-16 corpus. Log every request; never rank or synthesize Gold.

No network/domain DB access. The locked corpus supplies search order and links.
Separate stdio peers append bounded single-write records to one private log.
"""
import json
import os
from pathlib import Path
import sys


def state(root):
    path = root / 'reads.jsonl'
    return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []


def read(root, operation, key):
    fd = os.open(root / 'reads.jsonl', os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o600)
    try:
        record = (json.dumps(dict(operation=operation, key=key)) + '\n').encode()
        if os.write(fd, record) != len(record):
            raise OSError('incomplete research request log')
    finally:
        os.close(fd)
    source = json.loads((root / 'source.json').read_text())
    docs = {d['id']: d for d in source['documents']}
    if operation == 'search':
        return dict(hits=[dict(id=i, title=docs[i]['title'], url=docs[i]['url']) for i in source['search'][key]])
    if operation == 'open':
        return docs[key]
    raise ValueError('unknown research operation')


def serve(root):
    for line in sys.stdin:
        req = json.loads(line)
        if 'id' not in req:
            continue
        if req['method'] == 'initialize':
            result = dict(protocolVersion='2025-03-26', capabilities=dict(tools={}),
                          serverInfo=dict(name='research-sandbox', version='1'))
        elif req['method'] == 'tools/list':
            result = dict(tools=[dict(name=name, description=description, inputSchema=dict(
                type='object', properties=dict(key=dict(type='string', minLength=1)),
                required=['key'], additionalProperties=False)) for name, description in (
                    ('search', 'Find document references for a research query'),
                    ('open', 'Read one referenced research document, including links'))])
        elif req['method'] == 'tools/call':
            p = req['params']
            value = read(root, p['name'], p['arguments']['key'])
            result = dict(content=[dict(type='text', text=json.dumps(value, ensure_ascii=False))], isError=False)
        else:
            raise ValueError('unknown method')
        print(json.dumps(dict(jsonrpc='2.0', id=req['id'], result=result)), flush=True)


if __name__ == '__main__':
    serve(Path(sys.argv[1]).resolve())
