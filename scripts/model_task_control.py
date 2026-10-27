#!/usr/bin/env python3
"""Finite Task stop/admission model. See docs/guides/task-control-model.md.

No dependencies or runtime I/O. BFS retains shortest counterexamples; the normal
run exhausts the finite graph, then separately checks a fair recovery suffix.
"""
import argparse
from collections import Counter, deque
from dataclasses import dataclass, replace
import json
from pathlib import Path


@dataclass(frozen=True, slots=True)
class Run:
    epoch: int = 1
    status: str = "queued"
    owner: int = 0
    lease: str = "none"
    stop: bool = False


@dataclass(frozen=True, slots=True)
class State:
    task: str = "active"
    epoch: int = 1
    roots: tuple = (Run(),)
    child: Run | None = None  # At most one Child, belonging to Root 0.
    pause: bool = False
    cancel: bool = False
    pending: bool = False
    appended: bool = False
    crashed: bool = False
    up: bool = True
    effect: str = "absent"
    attempts: int = 0


def members(s):
    return [(f"root{i}", r) for i, r in enumerate(s.roots)] + (
        [("child", s.child)] if s.child else [])


def put(s, name, run):
    if name == "child":
        return replace(s, child=run)
    roots = list(s.roots)
    roots[int(name[-1])] = run
    return replace(s, roots=tuple(roots))


def live(r):
    return r.status in ("queued", "running")


def fenced(s, r, owner):
    return (r.status == "running" and r.owner == owner and r.lease == "live"
            and s.task == "active" and s.epoch == r.epoch)


def recover(s):
    # One recoverStopsTx transaction reaches its leaf-first fixed point.
    for name, r in reversed(members(s)):
        if not live(r) or not r.stop or r.lease == "live":
            continue
        if name == "root0" and s.child and live(s.child):
            continue
        s = put(s, name, replace(r, status="cancelled", lease="none"))
        if name == "root0" and s.effect == "dispatched":
            s = replace(s, effect="unknown")
    return s


def append(s):
    if not s.pending:
        return s
    active = any(live(r) for r in s.roots)
    fresh = s.task != "active" or not active
    if fresh and any(live(r) for _, r in members(s)):
        return s  # Entire admission transaction rolled back.
    if fresh:
        assert len(s.roots) < 2
        s = replace(s, task="active", epoch=s.epoch + 1,
                    roots=s.roots + (Run(epoch=s.epoch + 1),))
    return replace(s, pending=False, appended=True)


def transitions(s, unsafe=False):
    # Lease expiry is external time, including while the Host is down.
    for name, r in members(s):
        if r.lease == "live":
            yield f"expire {name}", put(s, name, replace(r, lease="expired")), None
    if not s.up:
        yield "restart", replace(s, up=True), None
        return
    if not s.crashed:
        yield "crash", replace(s, up=False, crashed=True), None
    yield "recovery scan", recover(s), None
    if not s.appended and not s.pending:
        yield "submit frozen append", replace(s, pending=True), None
    if s.pending:
        yield "try append", append(s), None
    if s.appended:
        # task_invocation/task_request receipt reuse; no Root/instruction changes.
        yield "duplicate frozen append", s, None
    for action, status in (("pause", "paused"), ("cancel", "cancelled")):
        if getattr(s, action) or not (s.task == "active" or
                                    (s.task == "paused" and action == "cancel")):
            continue
        n = replace(s, task=status, epoch=s.epoch + 1, **{action: True})
        for name, r in members(n):
            if live(r):
                n = put(n, name, replace(r, stop=True))
        yield f"commit {action}", recover(n), None
    for name, r in members(s):
        if (live(r) and not r.stop and s.task == "active" and r.epoch == s.epoch
                and r.lease != "live" and r.owner < 2):
            yield f"claim {name}", put(s, name, replace(
                r, status="running", owner=r.owner + 1, lease="live")), None
        if r.stop and r.status == "running" and r.lease == "live":
            # SettleStop is available only after Actor joins this executor.
            n = put(s, name, replace(r, lease="none"))
            yield f"join and settle {name}", recover(n), None
        if r.owner:
            # Delivery from owner 1, possibly after stop, expiry or replacement.
            allowed = fenced(s, r, 1)
            admitted = allowed or (unsafe and r.status == "running"
                                   and r.lease == "live")
            yield f"late result owner1 {name} ({'accept' if admitted else 'reject'})", s, (
                    "stale business write" if admitted and not allowed else None)
        if fenced(s, r, r.owner) and not (name == "root0" and s.effect == "dispatched"):
            # finishTask reads the Root outcome, regardless of which Run ended.
            # Terminal Child failure does not prevent one-shot Root success.
            for status in ("completed", "failed"):
                n = put(s, name, replace(r, status=status, lease="none"))
                root = n.roots[-1]
                descendants = [n.child] if len(n.roots) == 1 and n.child else []
                unknown = len(n.roots) == 1 and n.effect == "unknown"
                if (root.status == "completed" and not any(live(x) for x in descendants)
                        and not unknown):
                    n = replace(n, task="completed")
                yield f"{status} {name}", n, None
    r = s.roots[0]
    if s.child is None and r.owner:
        allowed = fenced(s, r, 1)
        admitted = allowed or (unsafe and r.status == "running" and r.lease == "live")
        if admitted:
            yield "admit child owner1", replace(s, child=Run(epoch=r.epoch)), (
                "stale Child admission" if not allowed else None)
    if s.effect == "absent" and fenced(s, r, r.owner):
        yield "dispatch unsafe effect", replace(s, effect="dispatched", attempts=1), None
    if s.effect == "dispatched" and fenced(s, r, r.owner):
        yield "commit effect receipt", replace(s, effect="succeeded"), None
    if s.effect == "unknown":
        yield "reject unknown effect replay", s, None


def invariant(s):
    assert s.task in ("active", "paused", "completed", "cancelled")
    assert sum(live(r) for r in s.roots) <= 1, "multiple nonterminal Roots"
    assert len(s.roots) <= 2 and all(r.owner <= 2 for _, r in members(s))
    assert s.attempts <= 1, "unsafe effect replayed"
    assert s.effect != "unknown" or s.attempts == 1


def recovery_suffix(s):
    """Witness under quiescent user control, eventual expiry/resources and scans.

    Stop already committed; no more controls/claims/business work are scheduled.
    Restart, expire all remaining leases, scan to fixed point, retry append.
    This is separate from safety and is not a claim about arbitrary scheduling.
    """
    s = replace(s, up=True)
    for name, r in members(s):
        if r.lease == "live":
            s = put(s, name, replace(r, lease="expired"))
    return append(recover(s))


def explore(unsafe=False):
    initial = State()
    parents = {initial: None}
    depth = {initial: 0}
    queue = deque([initial])
    edges = liveness = 0
    max_depth = 0
    coverage = Counter()
    max_task_epoch = max_owner_epoch = 0
    while queue:
        s = queue.popleft()
        max_depth = max(max_depth, depth[s])
        max_task_epoch = max(max_task_epoch, s.epoch)
        max_owner_epoch = max(max_owner_epoch, *(r.owner for _, r in members(s)))
        if s.pending and s.task in ("paused", "cancelled"):
            end = recovery_suffix(s)
            assert end.appended and not end.pending and end.task == "active"
            invariant(end)
            liveness += 1
        for action, n, violation in transitions(s, unsafe):
            edges += 1
            coverage[action] += 1
            try:
                assert not violation, violation
                invariant(n)
            except AssertionError as error:
                trace = [action]
                cursor = s
                while parents[cursor]:
                    cursor, step = parents[cursor]
                    trace.append(step)
                return dict(result="COUNTEREXAMPLE", violation=str(error),
                            trace=list(reversed(trace)), states=len(parents),
                            counterexample_length=len(trace), transitions=edges,
                            max_depth=max_depth, exhaustive=False)
            if n not in parents:
                parents[n] = (s, action)
                depth[n] = depth[s] + 1
                queue.append(n)
    return dict(result="PASS", states=len(parents), transitions=edges,
                max_depth=max_depth, exhaustive=True,
                recovery_suffixes=liveness,
                observed_task_epoch_max=max_task_epoch,
                observed_owner_epoch_max=max_owner_epoch,
                transition_coverage=dict(sorted(coverage.items())),
                bounds=dict(tasks=1, roots=2, children=1, owner_epochs_per_run=2,
                            crash_restart_pairs=1, frozen_appends=1,
                            pauses=1, cancels=1, unsafe_effects=1),
                fairness="quiescent controls; eventual restart, lease expiry, resources, scan and append retry")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--unsafe", action="store_true", help="remove business/Child fencing")
    parser.add_argument("--evidence", type=Path)
    args = parser.parse_args()
    result = explore(args.unsafe)
    rendered = json.dumps(result, indent=2) + "\n"
    if args.evidence:
        args.evidence.parent.mkdir(parents=True, exist_ok=True)
        args.evidence.write_text(rendered)
    print(rendered, end="")
    return 0 if result["result"] == "PASS" else 1


if __name__ == "__main__":
    raise SystemExit(main())
