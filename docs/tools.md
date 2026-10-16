# Tool execution and approval

Every Direct Tool enters `tool.Executor`: persisted provider response → frozen
admission → input validation → security/approval → bounded dispatch → normalized
result commit. Future Runtime facades must use this same entry point. Code Mode,
Child Runs, real MCP transport and Computer actions are not implemented by this
slice. No new production provider or implicit Fake is enabled.

`approval list` shows the local user's approval requests, complete final arguments,
frozen hash, status and expiry. To decide the displayed action:

```sh
personal-agent approval list
personal-agent approval approve APPROVAL_ID FROZEN_ARGUMENTS_HASH
# Or: personal-agent approval deny APPROVAL_ID FROZEN_ARGUMENTS_HASH
personal-agent chat work
personal-agent approval reconcile
```

These are local user control commands. The principal comes from the private data
root's Agent identity; neither a model nor tool arguments can select it. Future
remote channels must authenticate the principal before calling the same decision
boundary. IDs, hashes and provider keys do not enter model-visible tool schemas.
Approval is bound to one invocation, principal and request hash. A matching
callback can repeat after completion; a conflicting decision fails. Expiry,
denial and steering fail closed. New Main input invalidates every unexecuted
approval in that Main Run; Task append does the same for its active Root. This
includes approval granted before dispatch. A changed action requires a new
invocation and approval. Already dispatched effects cannot be revoked by steering.

| Invocation transition | Meaning |
| --- | --- |
| queued → running | Frozen arguments, policy, key and accepted Outbox event already committed; dispatch checks approval and resources atomically |
| queued/running → waiting(approval/resource/retry_backoff) | Persisted wake condition; executor and Run workers are released |
| waiting → running | Approval decision/expiry, available capacity, or due transient retry |
| running → succeeded/failed | Independent normalized result committed before end event; final batch delivery retains original call order |
| running → unknown | No trustworthy outcome for an unsafe dispatched call |
| unknown → succeeded/failed | Trusted provider query supplies authoritative outcome; evidence recorded with result |
| queued/waiting/running → cancelled/unknown | Controlled Task stop joins or fences owners; unexecuted calls cancel, unsafe dispatched calls stay unknown |

`unknown` is never dispatched again, even when a query later says absent.
`approval reconcile` queries unknown calls by their original key and may settle
only authoritative success/failure. It does not revive Runs, fabricate a new
approval, or send another action. The scheduler also runs this query-only scan. The CLI prints each unknown or
reconciled action with its observed status and safe result/receipt.
A Root with any unresolved effect in its current tree reports `unknown` and keeps
its Task active. A previously delivered unknown message is a historical observation; reconciliation
does not rewrite Conversation history or claim a new user delivery.
An oversized, inconsistent or nonterminal recovery query result is inconclusive.
The executor persists `unknown` before delivering `UNKNOWN_SIDE_EFFECT` to the
Worker, so the call remains available for later authoritative reconciliation.

## Provider contract

A registered definition declares `Spec` metadata and one execution boundary:

- `Execute(ctx, arguments)` is for read-only tools; trusted internal Task tools
  use `HostStore` under the same invocation and Run claim.
- `Invoke(ctx, Request)` is for external effects. `Request.IdempotencyKey` comes
  only from committed Host admission. The provider must use this key wherever its
  external system supports idempotency.
- `Query(ctx, Request)` returns `succeeded`, `failed`, `absent` or `unknown` from
  authoritative external state. `query_before_retry` requires it.

Every effective policy fixes version, risk, replay strategy, timeout, output byte
limit, provider concurrency scope and a versioned conflict-key generator (or
explicit `none`). An external tool must declare high risk, a bounded timeout and
output limit, provider scope and conflict policy. Missing replay policy means
`never`. Effectful tools cannot claim `safe`.

`Outcome.Ambiguous` takes priority over all retry flags. A provider may set
`Transient` only for a known infrastructure failure **before any effect**, using
`PROVIDER_UNAVAILABLE`. The Host allows three total attempts in one invocation
with persisted 50/100 ms backoff. Business errors and ordinary timeouts do not
retry. Uncertain external timeout/cancellation becomes `unknown`.

| Replay strategy | Incomplete running call after owner replacement |
| --- | --- |
| safe | Repeat the read with the same invocation/key |
| query_before_retry | Query by key; reuse a confirmed result, retry only confirmed absence, otherwise persist unknown |
| never (including omitted) | Persist unknown without dispatch; later query-only reconciliation may settle it |

Tool preparation checks the frozen Run capability before argument validation;
inaccessible Tool names return `NOT_FOUND` without revealing their schema.
Provider errors are reduced to the documented Host codes. Messages contain only
the safe code; no raw provider message/details, internal IDs, credentials, paths
or SQL cross this boundary. Only `PROVIDER_UNAVAILABLE` has `retryable=true`.
Successful tool output is trusted provider data; adapters must not return secrets.
External tool adapters never receive model-provider credentials.

The registry validates a bounded JSON Schema vocabulary: type, properties,
required, additionalProperties, items, oneOf, enum, const, numeric bounds, string
length and array length. Unsupported keywords reject registration. Arguments
reject duplicate keys, excessive depth/size, invalid types and bounds before
provider execution. Provider-specific semantic validation remains required.

## Capacity and recovery

`tools` configuration sets global/Agent/Run/provider/tool concurrency and approval
TTL. Defaults are in `config.example.yaml`. Admission freezes dynamic conflict
keys after schema validation. SQLite dispatch transactions arbitrate limits and
intersecting keys across processes. Only live fenced Run owners retain capacity.
A live attempt cannot be redispatched by its own owner. Shutdown during a
scheduler scan or claim returns normal cancellation. Already owned local setup
reaches a stable release boundary without starting another model call. Recovery requires a new
Run claim; stale owners cannot commit a result.

Migration 10 upgrades empty and all prior schemas. Old immutable Model/Capability
Snapshots and Tool schemas remain unchanged. Capability format 1 and its existing
read-only/internal ToolSpecs keep the fixed policy-v1 defaults: low risk, 10 s,
16 KiB (Task search 64 KiB), builtin provider and no conflicts. These are the same
pre-M3 execution limits. New Capability Snapshots use format 2. Admission stores
the effective policy and keys independently; changing a frozen Spec fails closed.
No credentials are added to storage.

This is not an exactly-once guarantee for external systems. Providers with no
idempotency or authoritative query can leave an outcome unknown indefinitely.
Do not retry such an action manually without resolving the original outcome.

## Deterministic evidence

`make verify` runs `scripts/verify_tools.py` five times. Each run launches isolated
real CLI processes with explicit `-test-bridge approval`/`approval-never` fixtures,
proves setup readiness, drives decisions and steering, kills Go at four execution
boundaries for both unsafe strategies, injects an oversized external receipt after
a Worker effect commit/SIGKILL and later reconciles the original call, reads back
invocation/approval/Outbox and a separate Fake external ledger, then joins children
and removes temporary data.
Reports are `evidence/tools-{1..5}.json`. The fixture tool is unavailable without
explicit test injection. The Fake ledger is not a domain database and has no
access to Run/Task state.

Focused Go tests cover expiry/denial, wrong principal/hash, duplicate decisions,
argument drift guards, unsupported schema rejection, resource limits/conflicts,
independent out-of-order completion, unknown reconciliation and migration
rollback. `go test -race ./...` covers their concurrent boundaries. These are
SC-06/07 and GC-11/12/13 approval/recovery **fragments**; full real purchase,
calendar and booking integrations remain due in their later slices. Artifact
spillover and remote delivery are separate M3 slices; excess inline output fails
`RESOURCE_LIMIT` in this slice.
