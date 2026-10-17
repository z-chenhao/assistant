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

ERII-17 retains the existing four Worker leases plus reserved Main execution
slot. Configurable Child concurrency/fairness and the larger M4 batch scenario
belong to the next scheduling slice. M5 adds CodeCall and JavaScript `agent.run()`
using this same admission and ToolInvocation path. This slice does not claim
M4 completion, JavaScript execution, general Durable Replay or full GC-16/17/18/20
product-case scores.

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
