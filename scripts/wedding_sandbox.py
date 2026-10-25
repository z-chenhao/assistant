#!/usr/bin/env python3
"""GC-08 stdio peer: immutable source and an independent mail-attempt ledger.

No network, credentials, domain DB access or deduplication. A wrong/duplicate
send remains visible rather than being corrected by a peer that knows Gold.
"""
import json
from pathlib import Path
import sqlite3
import sys


def state(root):
    path = root/'remote.db'
    if not path.exists(): return []
    with sqlite3.connect(path) as db:
        return [json.loads(r[0]) for r in db.execute('SELECT payload FROM mail ORDER BY rowid')]


def send(root, args):
    if set(args) != {'recipient','subject','body','attachments'} or not all(
            isinstance(args[k],str) and args[k] for k in ('recipient','subject','body')) or not isinstance(args['attachments'],list):
        raise ValueError('invalid mail')
    with sqlite3.connect(root/'remote.db') as db:
        db.execute('CREATE TABLE IF NOT EXISTS mail(payload TEXT NOT NULL)')
        db.execute('INSERT INTO mail VALUES(?)',(json.dumps(args),))
    return dict(status='sent')


def serve(root):
    for line in sys.stdin:
        req=json.loads(line)
        if 'id' not in req: continue
        if req['method']=='initialize':
            result=dict(protocolVersion='2025-03-26',capabilities=dict(tools={}),serverInfo=dict(name='wedding-sandbox',version='1'))
        elif req['method']=='tools/list':
            fields={k:dict(type='string',minLength=1) for k in ('recipient','subject','body')}
            fields['attachments']=dict(type='array',items=dict(type='string'))
            result=dict(tools=[dict(name='read',description='Read wedding source',inputSchema=dict(type='object',
                properties=dict(resource=dict(enum=['source'])),required=['resource'],additionalProperties=False)),
                dict(name='send',description='Send the frozen mail only after approval',inputSchema=dict(type='object',
                     properties=fields,required=list(fields),additionalProperties=False))])
        elif req['method']=='tools/call':
            p=req['params']
            if p['name']=='read': value=json.loads((root/'source.json').read_text())
            elif p['name']=='send':
                value=send(root,p['arguments'])
                if (root/'drop-reply').exists(): return  # Committed effect, lost acknowledgement.
            else: raise ValueError('unknown Tool')
            result=dict(content=[dict(type='text',text=json.dumps(value,ensure_ascii=False))],isError=False)
        else: raise ValueError('unknown method')
        print(json.dumps(dict(jsonrpc='2.0',id=req['id'],result=result)),flush=True)


if __name__=='__main__': serve(Path(sys.argv[1]).resolve())
