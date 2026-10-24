#!/usr/bin/env python3
"""GC-14 read-only stdio source with an independent request log.

The peer has no clock, network or domain database access. It returns the exact
requested fixture bytes and records every request, including wrong/duplicate
keys. It does not decide thresholds, suppress reads or generate notifications.
"""
import json
from pathlib import Path
import sys


def state(root):
    path = root / 'reads.jsonl'
    return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []


def read(root, key):
    with (root / 'reads.jsonl').open('a') as log:
        log.write(json.dumps(key) + '\n')
    return json.loads((root / 'source.json').read_text())[key]


def serve(root):
    for line in sys.stdin:
        request = json.loads(line)
        if 'id' not in request:
            continue
        if request['method'] == 'initialize':
            result = dict(protocolVersion='2025-03-26', capabilities=dict(tools={}),
                          serverInfo=dict(name='monitor-sandbox', version='1'))
        elif request['method'] == 'tools/list':
            result = dict(tools=[dict(name='read', description='Read a monitoring source by key',
                inputSchema=dict(type='object', properties=dict(text=dict(type='string', minLength=1)),
                                 required=['text'], additionalProperties=False))])
        elif request['method'] == 'tools/call':
            params = request['params']
            if params['name'] != 'read':
                raise ValueError('unknown Tool')
            value = read(root, params['arguments']['text'])
            result = dict(content=[dict(type='text', text=json.dumps(value, ensure_ascii=False))], isError=False)
        else:
            raise ValueError('unknown method')
        print(json.dumps(dict(jsonrpc='2.0', id=request['id'], result=result)), flush=True)


if __name__ == '__main__':
    serve(Path(sys.argv[1]).resolve())
