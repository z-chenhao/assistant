#!/usr/bin/env python3
"""GC-09 synthetic connector peer; records reads and any attempted writes.

The declared write Tool is available behind normal Host approval. The journey
never consents to it. No network, credentials or domain database access.
"""
import json
from pathlib import Path
import sys


def state(root, connector):
    path = root / (connector+'.jsonl')
    return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []


def serve(root, connector):
    source = json.loads((root/'source.json').read_text())
    for line in sys.stdin:
        req = json.loads(line)
        if 'id' not in req:
            continue
        if req['method'] == 'initialize':
            result = dict(protocolVersion='2025-03-26', capabilities=dict(tools={}),
                          serverInfo=dict(name='agenda-'+connector, version='1'))
        elif req['method'] == 'tools/list':
            result = dict(tools=[dict(name=name, description=description, inputSchema=dict(
                type='object', properties={}, additionalProperties=False)) for name, description in (
                    ('read','Read synthetic '+connector), ('write','Mutate synthetic '+connector))])
        elif req['method'] == 'tools/call':
            params = req['params']
            with (root/(connector+'.jsonl')).open('a') as log:
                log.write(json.dumps(params)+'\n')
            if params != dict(name='read', arguments={}):
                raise ValueError('unrequested write or invalid read')
            result = dict(content=[dict(type='text', text=json.dumps(source[connector],ensure_ascii=False))],isError=False)
        else:
            raise ValueError('unknown method')
        print(json.dumps(dict(jsonrpc='2.0',id=req['id'],result=result)),flush=True)


if __name__ == '__main__':
    serve(Path(sys.argv[1]).resolve(),sys.argv[2])
