# 006 — Explicit Task completion policy

Status: create policy accepted by the authenticated owner in ERII-225, comment
`01a16954-32dd-7a14-a571-cd8ba4d33ed3`. ERII-226 implements this finite contract.
The explicit-completion interface conflict below remains undecided.

## Decision and boundary

Expose optional `completionPolicy: "once" | "ongoing"` only on the existing
Main-only `builtin__task__manage` create arm. Omission keeps `once`. Ordinary
Root success completes a once Task after its final descendant, while ongoing
stays active. Both report the Root result once. Failure and unknown effects
leave either policy active. An ongoing Task without executable work starts no
new Root until append or an existing Automation supplies work.

Persist the choice atomically with Task, immutable anchor, Root, frozen
snapshots, receipt and Outbox. The existing policy trigger prevents later edits.
Append, pause, cancel, owner leases, old-epoch rejection and the single Loop/Tool
pipeline remain the mechanisms. Automation already uses the same ongoing
policy. No new Task state, scheduler, Agent Team or Workflow domain is needed.

The consumer is Main's current Task tool. The schema is a model-visible contract;
storage IDs, policy persistence and Run ownership remain private. No policy
setter or general scheduling configuration is exposed.

## Compatibility

No migration or backfill: schema v9 already added this column, with `once` for
prior Tasks. Empty and historical migrations retain their existing gates.
Explicit once normalizes to omission before request hashing, retaining legacy
invocation/source dedup identity. Ongoing is a different action even when the
brief matches. Invalid, null, duplicate or cross-arm policy fields fail closed.

The registry's v1 Spec format stays v1; the extended schema and description are
frozen into new capability snapshots. Exact Spec selection rejects an older
Main capability as `MODEL_INCOMPATIBLE`; upgrading never replaces a Run's schema.
Drain existing Main Runs with their original binary before upgrading. Stored
results, Task policy, anchors and terminal Runs are not rewritten. New Runs
receive the current schema; existing Worker snapshots have no Task tool.

## Explicit-completion conflict

ERII-226 says explicit completion can still end a Task. The current baseline
§3.9 and implementation expose only create/append/pause/cancel. The database
permits active → completed, but no callable explicit-completion path exists.
Adding a `complete` operation changes the approved model-visible union. This
requires an owner decision against ERII-225 before final acceptance; this change
does not invent that operation or treat cancel as completion.

## Verification

Contract and real SQLite tests cover default/explicit-once dedup, separate ongoing
identity, frozen prior schema rejection, child-tree completion, append, controls,
late writes and immutable policy. `scripts/verify_tasks.py` drives the actual CLI,
reads stored state, kills before/after Task and Root-result commit, restarts and
checks completed-call reuse. Existing five-run control and GC-08 fragment gates
remain required. Scripted fixtures prove mechanical behavior, not semantic Golden
Case quality or v1 release acceptance.
