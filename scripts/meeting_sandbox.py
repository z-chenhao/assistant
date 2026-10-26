#!/usr/bin/env python3
"""Explicit stdio MCP fixture for GC-12; no network or real accounts.

One frozen invite action sends a mail AND creates a calendar entry in the
independent remote SQLite Fake. No deduplication masks duplicate Host dispatch.
It does not implement live mail/calendar integration or a cross-provider saga.
"""
import json
from pathlib import Path
import sqlite3
import sys

FIELDS = ('action', 'recipient', 'subject', 'body', 'start', 'end', 'timezone')
INVITE_SCHEMA = dict(type='object', properties={k: dict(type='string', minLength=1) for k in FIELDS},
                     required=list(FIELDS), additionalProperties=False)
INVITE_SCHEMA['properties']['action'] = dict(enum=['send_mail_and_create_calendar'])


def state(root):
    path = root / 'remote.db'
    if not path.exists():
        return dict(mail=[], calendar=[], attempts=[])
    with sqlite3.connect(path) as db:
        return {name: [json.loads(r[0]) for r in db.execute('SELECT payload FROM '+name+' ORDER BY rowid')]
                for name in ('mail', 'calendar', 'attempts')}


def invite(root, args):
    # Valid independent service boundary; intentionally no key/ON CONFLICT.
    # Incorrect recipient/time is recorded, so assertions can detect it rather
    # than being rescued by a Fake that knows the Gold.
    if set(args) != set(FIELDS) or any(not isinstance(v, str) or not v for v in args.values()):
        raise ValueError('invalid invitation')
    if args['action'] != 'send_mail_and_create_calendar':
        raise ValueError('unknown invitation action')
    with sqlite3.connect(root / 'remote.db') as db:
        for name in ('mail', 'calendar', 'attempts'):
            db.execute('CREATE TABLE IF NOT EXISTS '+name+'(payload TEXT NOT NULL)')
        db.execute('INSERT INTO attempts VALUES(?)', (json.dumps(args),))
        mail = dict(recipient=args['recipient'], subject=args['subject'], body=args['body'], status='sent')
        calendar = dict(attendee=args['recipient'], title=args['subject'], start=args['start'],
                        end=args['end'], timezone=args['timezone'], status='confirmed')
        for name, value in (('mail', mail), ('calendar', calendar)):
            db.execute('INSERT INTO '+name+' VALUES(?)', (json.dumps(value),))
    return dict(mail=mail, calendar=calendar)


def serve(root):
    for line in sys.stdin:
        req = json.loads(line)
        if 'id' not in req:
            continue
        method = req['method']
        if method == 'initialize':
            result = dict(protocolVersion='2025-03-26', capabilities=dict(tools={}),
                          serverInfo=dict(name='meeting-sandbox', version='1'))
        elif method == 'tools/list':
            result = dict(tools=[dict(name='read', description='Read meeting source or actual remote state',
                inputSchema=dict(type='object', properties=dict(resource=dict(enum=['source', 'status'])),
                                 required=['resource'], additionalProperties=False)),
                dict(name='invite', description='Send one mail and create one calendar entry, with exact frozen parameters',
                     inputSchema=INVITE_SCHEMA)])
        elif method == 'tools/call':
            p = req['params']
            if p['name'] == 'read':
                value = json.loads((root/'source.json').read_text()) if p['arguments']['resource'] == 'source' else state(root)
            elif p['name'] == 'invite':
                value = invite(root, p['arguments'])
                # A lost reply after the two remote effects proves unknown is
                # query-only. Host must not dispatch this never-replay Tool again.
                if (root/'drop-reply').exists():
                    return
            else:
                raise ValueError('unknown Tool')
            result = dict(content=[dict(type='text', text=json.dumps(value, ensure_ascii=False))], isError=False)
        else:
            raise ValueError('unknown method')
        print(json.dumps(dict(jsonrpc='2.0', id=req['id'], result=result)), flush=True)


if __name__ == '__main__':
    serve(Path(sys.argv[1]).resolve())
