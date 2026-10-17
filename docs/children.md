# Direct Child delegation

ERII-17 adds `builtin__agent__run` v1 to new Worker capabilities. A Root or
Child can delegate explicit work to one general Worker. Main continues to use
Task tools. Every Child executes the same `RunAgentLoop` kernel.

```json
{"task":"Compare these two sources and return the material differences.","artifactRefs":[]}
```

The model receives only the final business result:

```json
{"text":"The material differences are …","artifactRefs":[]}
```

`task` is required, nonblank, at most 8192 characters. Optional `artifactRefs`
contains at most 127 opaque accessible references. All other properties, nulls
and duplicate keys fail validation. No model, agent, Skill, Tool, permission,
output schema, timeout, label, internal ID or replay key can be selected.

| Boundary | Rule | Durable evidence |
| --- | --- | --- |
| Main | Tool hidden, `can_spawn_agents=0`; forged call returns `NOT_FOUND` | Main Capability Snapshot, failed invocation, no Child |
| Parent Worker | Tool visible and `can_spawn_agents=1`; active Task/epoch and live claim required | Frozen capability and Run claim |
| Admission | Exactly one Child per accepted direct call; creates no Task | `agent_invocation`, unique direct invocation key, `run_tree`, `run_ready` Outbox |
| Ancestry | Same Agent, Task, Task epoch, Root and ModelSnapshot; depth = Parent depth + 1 | Immutable Run identities and `child_initial` SQL trigger |
| Capability | Derive from Parent's frozen specs; retain only work permissions, scope and frozen Skills | Child Capability Snapshot is a subset; no live config lookup |
| Context | Core safety + general Worker rules + explicit task/references | Child model request; no Parent messages, identity, Task steering or Conversation copy |
| Result | Final text and selected ArtifactRefs to direct Parent only | Child `worker_result`, Parent invocation result and Artifact pins |
| Approval | Each external effect has its own frozen invocation approval | Separate approval IDs, argument hashes and invocation owners |

Fixed reductions exclude Task tools and permissions for Task control, user
delivery, Conversation/Session control, Agent configuration, credentials and
Memory writes. Unknown future permissions are excluded. Work permissions are
`read_only`, `external_effect`, `artifact_write` and `spawn_agents`.
Child text above 16 KiB uses the existing Tool output normalization: the full
text becomes an Artifact inside the invocation terminal transaction, while the
API still returns only a short `text` plus `artifactRefs`.
Artifact publish selects outputs for the Parent; it does not send them to the
user. The Root selects final Task artifacts and Main owns final delivery.

The AgentService admission operation is `agent/sqlite.Runs.StartChildRun`.
It runs inside the ordinary `ToolStore.Accept` transaction. The invocation,
Child, Child capability, claim, Root counters, Artifact input grants and Outbox
commit together. Validation failures use the same durable invocation lifecycle
and stable Host Tool errors. Parameter drift fails with
`NON_DETERMINISTIC_REPLAY`; repeated admission reuses the same Child and does
not increment counters. Calls with identical arguments but different provider
call IDs create distinct Children.

`tools.AgentToolExecutor` joins that Child through the existing Host Tool
executor. Pending work becomes `waiting(child_run)`. The Parent Run becomes
`waiting(resource)` and releases its lease, so recursive trees cannot consume
all execution slots with waiting ancestors. On terminal Child commit, a
`run_ready` Outbox event targets only the direct Parent. Recovery scans the
same authoritative Child state, including completion before Parent suspension.
No Child result is sent directly to Main or the user. Child model failures
return a Host Tool error for Parent judgment; Host does not automatically retry
the Child. `unknown` external effects remain `UNKNOWN_SIDE_EFFECT`, never
become successful Agent results and never create a replacement Child.

A tree waiting entirely on approvals releases every lease and lets `chat work`
exit. Run `approval list`, approve/deny the exact frozen action, then run
`chat work` again. This resumes the same Child and Parent invocation. Approval
of a Parent action never approves a Child action, even with identical arguments.

Task pause/cancel uses the existing Task epoch and stop pipeline. New admissions
and late model/tool results immediately fail fencing. Descendants settle before
ancestors. A later append creates a new Root; it never revives old Runs.

## Configuration and upgrade

```yaml
children:
  max_depth: 8
  max_descendants: 4096
  max_tool_calls: 10000
```

Depth excludes the Root at depth zero. Descendants count all admitted Children
including terminal ones. Tool calls count first Worker invocation admissions
across the tree; replay does not consume another call. Beyond a ceiling, calls
return nonretryable `RESOURCE_LIMIT`; models receive no budget-query interface.
Deployment maxima are depth 16, descendants 10000, calls 50000. Limits are
frozen in `run_tree` for each new Root. Changing configuration affects only new
trees. These are safety ceilings; no small tree token/cost/time budget is added.

Migration 013 upgrades empty and existing schema 012 databases forward. Existing
Run capabilities and ModelSnapshots remain immutable. Old Roots without the
Agent Tool do not gain it on upgrade. Existing Root counters initialize from
stored Runs/invocations. Downgrades fail closed; back up the data root before
upgrading. No credentials or external account are needed for deterministic CI.

ERII-18 adds fair queues and the larger Direct Tool batch scenario described
below. M5 adds CodeCall and JavaScript `agent.run()`
using this same admission and ToolInvocation path. This slice does not claim
M4 completion, JavaScript execution, general Durable Replay or full GC-16/17/18/20
product-case scores.

## Fair scheduling and batch evidence (ERII-18)

`scheduler.child_concurrency` defaults to 16 active Child leases per Root tree.
Global, local-user, Agent and frozen provider account/endpoint limits default to
32. Every concurrency setting accepts 1–32. Main has a separate reserved slot.
The single-user data root makes the user limit an aggregate Worker limit.
All processes sharing a data root must use the same deployment configuration.
Tool provider/run/tool limits and workspace conflict keys still apply at the
shared ToolInvocation boundary. A busy workspace serializes only its conflicting
Tools. Logical Computer allocation/binding remains an M6 integration.

Run rows form the queue. Each successful Worker claim atomically checks live
leases and advances its Root's durable service ticket. New trees enter at the
current queue tail. The least recently served eligible tree goes first; runnable
descendants go first within that tree. No capacity query reaches the model.
Saturation leaves accepted Children queued. Expired leases lose capacity and
ownership together. Waiting ancestors hold no lease. Child joins coalesce until
the current batch settles; independent ready Tools and resolved approvals can
wake the Parent earlier. The model receives the joined Tool batch. Effects
retain their individual frozen approvals.

`scheduler.max_worker_turns` defaults to 256, with a deployment ceiling of 512.
It is frozen in `run_tree` at Root creation, including for descendants. Exceeding
it fails that Worker with `RESOURCE_LIMIT`; the Parent can aggregate other items.
Depth, descendant and tree Tool-call ceilings retain their existing defaults.
There is no cumulative business-failure cutoff or tree token/cost/time cutoff.
Model calls retain a 45-second timeout and Tools retain their frozen timeout.
These are per-operation watchdogs, not a total Task duration limit.

Provider usage acceptance also updates a durable infrastructure-error streak.
Five consecutive unavailable/rate-limited responses open a 30-second scheduling
cooldown by default (`infrastructure_failures` and `cooldown_seconds`). Successful
responses and known business errors reset the streak. A cooldown pauses new
leases without retrying model/business failures or failing queued items. Other
providers and Main remain schedulable. The existing Tool pipeline retries only
its known transient infrastructure outcomes; `unknown` effects stay uncertain.

Migration 014 upgrades schema 013 forward, preserving frozen Runs, capabilities
and tree counters. Existing trees receive a 256-turn ceiling. New Roots freeze
the configured value. `scheduler_tree` stores service order; `scheduler_provider`
stores infrastructure health. Read-only `run_tree_usage`, `task_usage` and
`account_usage` views aggregate immutable provider observations across the tree,
Roots and actual account/endpoint. Omitted token usage remains explicit through
`missing_token_usage`; it is never represented as zero monetary cost.

The TUI `/status` reads aggregate queued/running/succeeded/failed/waiting counts
for the current Task epoch. Cancelled Children count as failed in this display.
Reads create no notifications, Task updates or Main model turns. Root prompt
guidance asks it to collect available results and deduplicate genuine blockers;
the Fake fixture tests transport and aggregation, not real-model judgment.

```sh
python3 scripts/verify_scheduling.py --binary bin/personal-agent --evidence evidence/scheduling.json
```

This maintained harness owns Launch, Doctor, Drive, Evidence and Cleanup. Its
100-item scenario uses four ordinary 25-call provider exchanges under the
existing 32-call Bridge limit. It proves one Task/Root/100 Child, 98 successful
items plus two failures, one deduplicated blocker and one aggregate follow-up.
It measures lease events, saturates 16 grandchildren, observes a second Task
complete, cancels the first tree, and checks repeated-drain stability. A SIGKILL
after 25 atomic admissions proves restart preserves Child identities and saved
model results. `make verify` runs five consecutive passes. Go tests additionally
cover atomic cross-process limits, fair service after reopening, expired-owner
fencing, frozen turns, circuit recovery, prior-schema upgrade and rollback.

Account-spend authorization uses explicitly configured monetary policy. See
[the approved decision](adr/002-account-monetary-authorization.md). Prices and
policy freeze with the ModelSnapshot; Children inherit them. Usage acceptance
settles each reservation exactly once in the same transaction as the model
result. Missing usage retains unknown actual cost and its conservative reserve.
No token allowance substitutes for monetary authorization.

`personal-agent -config config.yaml spend status` displays sanitized account
keys, snapshot/price versions, cumulative committed microcurrency, unknown usage
and historical unpriced call counts. It reports `configured`, `unpriced`,
`unconfigured` or `unpriced_unconfigured`; it never invents prices. The account
key is `provider_kind:auth_connection_id`, or `provider_kind:endpoint_hash` when
no authentication connection exists. Tokens remain in the usage views.
Setup Probe snapshots remain visible as unpriced control-plane observations.
This ledger is not a provider bill and cannot observe other applications' spend.

Configure `spend.accounts` as in `config.example.yaml`. A positive
`boundary_micros` is the cumulative account authorization boundary, in millionths
of the three-letter currency. Zero/absent means unconfigured. All four integer
rates per million tokens must be explicit, even when zero. Each model price
requires a version. Currency changes and rate drift under an existing version
fail closed; restart with corrected metadata. New Runs freeze new configuration;
existing Runs retain their original policy and rates.

Admission reserves the full context bound at the highest configured input/cache
rate plus the output bound at the output rate. This is deliberately conservative,
not a token prediction. Once account actual spend plus outstanding reserves and
the new projection reaches the boundary, all descendants join one Root approval.
Use the existing `approval list` and `approval approve/deny ID HASH` CLI or TUI
`/approve`/`/deny`. The displayed frozen action authorizes remaining model spend
for this Run batch beyond the account boundary without another monetary ceiling.
It grants no tool effects. An independent Main Run needs its own spend consent.
A new Root does not inherit the grant. Denial fails held model work;
pause/cancel invalidates pending approval and fences old results. Pending consent
survives process exit without expiry or duplicate prompts.

`scripts/verify_spend.py` uses explicit synthetic prices to prove the 100-item
batch, one Root approval, exact currency conversion, CLI restart, wrong-hash
rejection, denial, cancellation, and separately approved Child effect recovery
across SIGKILL. It also proves all unknown configuration/usage states. It runs
five times in `make verify`. This Direct Tool slice makes no full M4 milestone,
semantic Golden Case or M5 JavaScript/Durable Replay claim.

## Verification and operations

```sh
make build
python3 scripts/verify_children.py --binary bin/personal-agent --evidence evidence/children.json
```

The maintained harness owns Launch (isolated data root and explicit Fake), Doctor
(model add/probe/select), Drive (PTY and production CLI), Evidence (Run tree,
capabilities, model requests, results, approvals, Artifacts, counters and Outbox),
and Cleanup (reap owned processes and delete disposable state). `make verify`
runs it five times. It tests Root/Child/Grandchild, explicit Artifact passage,
forged calls, invalid parameters, inaccessible references, Child failure,
recoverable approval, live cancellation and six SIGKILL/restart boundaries.

`children-*.json` records before/after state and binary SHA256. Unit/race tests
separately cover admission rollback, simultaneous duplicate callers, old claims,
late Task epochs, profile changes, ceilings, fixed reductions and per-invocation
approvals. A successful protocol fixture is not model-quality evidence. Full
Golden Cases remain marked not applicable until their required integrations land.
