"""Private deterministic M6 boundary Fakes; no production adapter claim.

These cover the full SC-08/SC-11 fixture lifecycle while Curator/Computer work is
still due M6. Real ERII-22 retrieval/storage evidence lives in verify_recall.py;
these Fakes are never registered as product tools or enabled in configuration.
"""
from dataclasses import asdict, dataclass


@dataclass
class Memory:
    owner: str
    text: str
    superseded: bool = False
    deleted: bool = False


class MemoryFake:
    def __init__(self):
        self.rows = []

    def apply(self, owner, text, supersede=None):
        if supersede is not None:
            row = self.rows[supersede]
            if row.owner != owner or row.deleted:
                raise PermissionError('inaccessible memory')
            row.superseded = True
        self.rows.append(Memory(owner, text))
        return len(self.rows) - 1

    def delete(self, owner, index):
        row = self.rows[index]
        if row.owner != owner:
            raise PermissionError('inaccessible memory')
        row.deleted = True

    def recall(self, owner, query):
        return [row.text for row in self.rows if row.owner == owner and not row.deleted
                and not row.superseded and query in row.text]


class DesktopLeaseFake:
    def __init__(self):
        self.owners = {}
        self.generations = {}
        self.effects = []

    def acquire(self, desktop, owner):
        if desktop in self.owners:
            return None
        self.generations[desktop] = self.generations.get(desktop, 0) + 1
        token = self.generations[desktop]
        self.owners[desktop] = (owner, token)
        return token

    def lose(self, desktop):
        self.owners.pop(desktop, None)

    def act(self, desktop, owner, token, text):
        if self.owners.get(desktop) != (owner, token):
            raise PermissionError('lease lost')
        self.effects.append((desktop, text))


def exercise():
    memory = MemoryFake()
    initial = memory.apply('user_eval_001', 'city:杭州')
    memory.apply('other_user', 'city:PRIVATE')
    updated = memory.apply('user_eval_001', 'city:上海', supersede=initial)
    corrected = memory.recall('user_eval_001', 'city:') == ['city:上海']
    memory.delete('user_eval_001', updated)
    deleted = memory.recall('user_eval_001', 'city:') == []
    memory.apply('user_eval_001', 'occupation:engineer')
    restrained = memory.recall('user_eval_001', 'health:') == []
    denied = False
    try:
        memory.delete('user_eval_001', 1)
    except PermissionError:
        denied = True
    lease = DesktopLeaseFake()
    old = lease.acquire('desktop-a', 'old-worker')
    blocked = lease.acquire('desktop-a', 'peer') is None
    independent = lease.acquire('desktop-b', 'peer') is not None
    lease.lose('desktop-a')
    new = lease.acquire('desktop-a', 'new-worker')
    fenced = False
    try:
        lease.act('desktop-a', 'old-worker', old, 'late click')
    except PermissionError:
        fenced = True
    lease.act('desktop-a', 'new-worker', new, 'current click')
    return {
        'SC-08': dict(state=[asdict(row) for row in memory.rows], status='boundary_fake', implementation='not_applicable until M6',
                      assertions=dict(supersede=corrected, delete_no_recall=deleted,
                                      query_restraint=restrained, owner_scope=denied)),
        'SC-11': dict(effects=lease.effects, owners=lease.owners, status='boundary_fake', implementation='not_applicable until M6',
                      assertions=dict(local_block=blocked, independent_resource=independent,
                                      old_lease_rejected=fenced, no_late_effect=lease.effects == [('desktop-a', 'current click')])),
    }
