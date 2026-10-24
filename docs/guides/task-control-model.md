# Task stop and admission model

This private engineering model maps the existing SQLite protocol. It adds no
runtime state, schema, Tool surface or extension API. Run it offline:

```sh
python3 scripts/model_task_control.py --evidence evidence/task-model.json
python3 scripts/model_task_control.py --unsafe --evidence evidence/task-model-unsafe.json
python3 -m unittest discover -s scripts -p 'test_model_task_control.py'
go test ./internal/agent/sqlite -run '^TestTaskControlModel' -count=1
```

The normal command exits 0 only after exhausting the finite reachable graph.
`--unsafe` removes Task/owner fencing from business result and Child admission;
it must exit 1 with a shortest counterexample. It does not mutate Go source.
`make test` includes the model regressions. Required race, process recovery,
five-run gates and complete CI remain separate requirements.

## States and atomic boundaries

```mermaid
stateDiagram-v2
    [*] --> Active
    Active --> Stopping: pause/cancel transaction; epoch +1
    Stopping --> Stopping: append rolls back with ErrBusy
    Stopping --> Settled: owners join OR leases expire; leaf-first scan
    Settled --> Active: append transaction; instruction + epoch + new Root
    Active --> Active: append to live Root; instruction + frozen receipt
    Active --> Completed: Root succeeded; no live descendants or unknown effects
    Active --> Active: Root failed; Task stays active
    Completed --> Active: append creates new Root
```

`Stopping` and `Settled` both have Task status `paused` or `cancelled`.
They distinguish whether the old tree still has nonterminal Runs. Each Run has
an independent claim epoch and lease; Task epoch fences business writes as soon
as stop commits. An unchanged Task status on repeated stop does not increment
its epoch. The model bounds pause and cancel to one accepted change each;
receipt dedup is modeled separately for append.

| Model step | Implementation / SQL atomic unit | Important rejection or retained state |
| --- | --- | --- |
| Initial Root | [`createRoot`](../../internal/agent/sqlite/tasks.go), accepted Task creation transaction | Task, Root, snapshots, claims, tree and Outbox commit together; model starts after commit |
| Dispatch | [`tools.ExecuteBuiltin`](../../internal/agent/sqlite/builtins.go) reads persisted invocation name | Automation, Code Mode, Computer, Child, Memory manage/search keep their existing executors; Task and Artifact use the existing transactional fallback |
| Append / duplicate | `executeTaskAdmission` / `executeBuiltin` in `tasks.go`; `task_invocation`, `task_request`, Task CAS, instruction, Root and Outbox in one `Runs.write` | Same frozen action reuses receipt; fresh append checks all nonterminal tree members; ErrBusy rolls back before waiting outside the transaction; Artifact fallback never returns this admission ErrBusy |
| Pause / cancel | [`stopTask`](../../internal/agent/sqlite/task_control.go): automation disable, Task CAS/epoch increment, Run/Tool desired state, `recoverStopsTx`; caller stores receipt/Outbox in the same transaction | No signal-before-commit gap; old Run keeps its old Task epoch; no business result salvaged |
| Claim / expire | [`Claim`, `Renew`, `fence`](../../internal/agent/sqlite/runs.go): claim token, epoch increment, expiry and Run status transaction | Claim rejects stop intent, expired/stale ownership and inactive/mismatched Task; renewal uses ownership only, so a stopping owner can join safely |
| Late result / Child | `fence`, [`AcceptGeneration`](../../internal/agent/sqlite/generation.go), [`tools.Accept`](../../internal/agent/sqlite/tools.go), [`StartChildRun`](../../internal/agent/sqlite/children.go) | Old owner/Task epoch rejects writes and admission; usage/result and Tool/Child insertion roll back atomically |
| Join / recovery scan | Actor joins current executor before `SettleStop`; ownership validation, claim release and `recoverStopsTx` transaction | Live owners cannot be scanned away; expired/unowned leaves settle first; parents wait for descendants; repeated scan is idempotent |
| Unsafe effect | `recoverStopsTx` updates pending Tool status using `attempt_count` and `replay_policy`; existing [`ToolExecutor`](../../internal/tool/executor.go) | Dispatched unsafe effect becomes `unknown`; no replay; prior successful receipts stay successful; unexecuted/safe work cancels |
| Completed / failed Run | `Runs.terminal`, [`finishTask`](../../internal/agent/sqlite/reporter.go) | Read the Root outcome on each terminal transition; Root success completes a one-shot Task after all descendants end, including failed Children; unknown effects keep it active; Root failure leaves Task active; terminal Runs never revive |
| Crash / restart | Existing claim persistence plus recovery scan | Crash changes Host availability only; leases do not disappear with the process; recovery must still wait for expiry |

The abstraction serializes whole committed transactions, as `Runs.write` does.
Queued/waiting Runs share one unowned runnable category. Resources are available
at claim/admission; no profile, approval, memory-privacy, capacity or automation
state is explored. Owner identity is its per-Run epoch; token uniqueness is
assumed. Old-result and old-Child attempts use owner 1, including after owner 2
replaces it. Child belongs only to the first Root. The unsafe-effect slot belongs
to that Root and covers stop recovery only, with no successful terminal Run
while its effect is still dispatched. Main, model execution and I/O are
abstracted to persisted boundary events. This is a protocol model, not a second
execution implementation or a proof of these excluded subsystems.

## Bounds, safety and progress

Bounds are one Task, at most two Root identities, one Child, two owner epochs
per Run, one crash/restart pair, one frozen append, one pause, one cancel and one
unsafe effect dispatch. Task epoch starts at 1 and advances at accepted control
or fresh append boundaries. Rejected operations and duplicate receipts are
self-loops. BFS memoizes states and has no depth cutoff; `max_depth` is the
greatest shortest-path depth, not an arbitrary execution-length limit. Reports
give actual state/edge counts, attained epochs and per-action coverage. Changing
the abstraction can change those counts; the executable report is authoritative.

After correcting Root-outcome completion, the normal model exhausts 26,595
states and 121,610 transitions, with greatest shortest-path depth 21 and 2,637
recovery suffixes. Observed Task epoch reaches 4 and owner epoch reaches 2.
The unsafe variant still returns the three-step fencing counterexample below.

Safety checks one nonterminal Root, stale business/Child rejection, bounded
ownership, four legal Task states and at most one unsafe dispatch. Leaf-first
settlement and atomic append/receipt reuse are modeled transitions; the Go and
CLI tests must independently validate that mapping. The model does not prove
SQLite isolation, implementation completeness or an external exactly-once claim.

Progress is checked separately for **every reached state with a committed stop
and pending append**. Its recovery suffix restarts the Host, expires remaining
leases, scans leaves to a fixed point, then retries append. It requires new
controls to cease, leases eventually to expire (no endless renewal), resources
eventually to be available, and recovery scans/append retries to run fairly.
Every such suffix must admit append with a new Root. This finite suffix check
does not prove liveness under arbitrary infinite scheduling or outside the
stated bounds. Crash before/after transaction commit is represented by the two
adjacent committed states; real kill/restart gates validate that abstraction.

## Counterexample and implementation evidence

`test_one_shot_completion_uses_root_outcome_after_child_drains` and
`TestTaskControlModelCompletionUsesRootOutcome` pair eight traces: both Root
outcomes, both Child outcomes, and both ending orders. For example,
`claim root0 → admit child owner1 → claim child → completed root0 → failed child`
must complete the Task and publish one completed Root update. The Go regression
uses the existing synthetic Child admission fixture, the real Agent Loop for
success, `Runs.Fail` for failure, and SQLite readback. Admission itself is not
proven by this fixture. A failed Root stays active even if its Child succeeds.
The unsafe-effect slot still belongs only to Root 0: an unknown effect in the
current epoch prevents completion; an old epoch's unknown does not prevent a
fresh Root from completing.

The unsafe model finds `claim root0 → commit pause → accept old-owner result`.
The Task epoch changes while the old owner still has a live lease. Checking only
ownership would accept that result. The normal model rejects it.
`TestTaskControlModelCounterexampleRejectsOldOwner` repeats this trace through
real model-result persistence, `Progress` and `ToolStore.Accept`, for pause and
cancel. It proves no stale progress, invocation or Child survives rejection.
The earlier `TestTaskControlPauseFencesLateWritesAndContinueNewRoot` also checks
late generation/usage rollback, reply, terminal and trace writes after stop.

For mutation evidence, use a disposable Go overlay removing only the Task
predicate from `fence` and run the new regression. It must fail with
`model counterexample admitted stale write`; rerun without the overlay and keep
both logs. Do not commit the overlay or a weakened production predicate.
Process-level evidence remains the maintained `task-control`, `children`,
`tools` and `code-recovery` gates, each with its original five repetitions.
