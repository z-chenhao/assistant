#!/usr/bin/env python3
"""Read-only GC-10 source: return locked cache/current bytes and log reads.

No ranking, recommendation, network, time source or domain-database access.
"""
import json
from pathlib import Path
import sys


def state(root):
    path = root/'reads.jsonl'
    return [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []


def serve(root):
    for line in sys.stdin:
        req = json.loads(line)
        if 'id' not in req:
            continue
        method = req['method']
        if method == 'initialize':
            result = dict(protocolVersion='2025-03-26',capabilities=dict(tools={}),serverInfo=dict(name='local-sandbox',version='1'))
        elif method == 'tools/list':
            result = dict(tools=[dict(name='weather',description='Read sampled weather for a location/time; refresh bypasses cache',inputSchema=dict(
                type='object',properties=dict(location=dict(type='string'),at=dict(type='string'),refresh=dict(type='boolean')),
                required=['location','at','refresh'],additionalProperties=False))])
        elif method == 'tools/call':
            args = req['params']['arguments']
            source = json.loads((root/'source.json').read_text())
            sample = next(s for s in source if s['location'] == args['location'] and s['at'] == args['at'])
            value = sample['current' if args['refresh'] else 'cache']
            with (root/'reads.jsonl').open('a') as log:
                log.write(json.dumps(dict(arguments=args,result=value),ensure_ascii=False)+'\n')
            result = dict(content=[dict(type='text',text=json.dumps(value,ensure_ascii=False))],isError=False)
        else:
            raise ValueError('unknown method')
        print(json.dumps(dict(jsonrpc='2.0',id=req['id'],result=result)),flush=True)


if __name__ == '__main__':
    serve(Path(sys.argv[1]).resolve())
