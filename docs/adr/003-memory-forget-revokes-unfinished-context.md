# Forget revokes unfinished context

Accepted for ERII-23 on 2026-10-18 by the project owner: “选择 A：Forget 应覆盖未完成 Run；审计记录可以保留，但被忘记内容不得继续进入模型上下文”.

A durable Forget intent is a privacy barrier. It takes precedence over reusing
frozen recall, compacted context and completed recall calls in unfinished work.
The Host retains immutable requests/results as audit evidence. It never changes
a frozen request, snapshot, CodeCall argument hash or internal call key to remove
content in place.

The initial implementation invalidates all unfinished Runs in the same Agent
scope. Semantic dependencies can exist in copied Tool results, Child results and
summaries; checking only the latest `run_recall` row would miss them. A monotonically
increasing privacy epoch is committed with the accepted mutation. Business writes,
model-result acceptance and Code Mode callbacks must match the epoch frozen at
Run creation. Existing stop recovery joins live owners, stops descendants before
parents, and preserves uncertain effects as `unknown`. Task status remains intact.
The Host does not revive an old Run or retry an unknown effect.

Provider dispatch holds a short state transaction while writing its request
frame. Forget acceptance and request dispatch therefore have a serialized order
across local processes. A request already dispatched cannot be withdrawn from a
provider; its late result is rejected after revocation. A queued request cannot
dispatch after the barrier. This does not claim erasure from an external model.

New Runs do not automatically import Conversation or recall evidence from before
the scope's barrier. This intentionally conservative rule also excludes unrelated
older context from automatic assembly. Originals remain available in explicit
local history/audit; a user can provide fresh independent evidence or remember
again. Older Tasks are excluded from model Task search and cannot append/create a new
Root across the barrier. The user can create a fresh Task with independent current
input. This prevents old anchors/instructions from silently reintroducing revoked
material.

Input accepted after the barrier owns a new queued Main with its own frozen
snapshots and immutable receipt. It never joins the revoked Main. Both scheduler
and direct Claim wait for revoked Main owners to join or expire and settle before
executing the successor. A later Forget revokes that queued successor as usual.
State schema 20 replaces the single nonterminal Main index with one Main whose
desired state is running; stopped audit rows may coexist with its queued successor.
The migration preserves all input, claim and snapshot records. Use the current
binary for all processes after migration; older binaries do not enforce this
successor admission fence.

A newer explicit remember with an independent evidence source creates an
immutable successor even when its content and tier deduplicate. Its timestamp
and source belong only to the fresh evidence; the old record stays audit-only.
Receipt replay, repeated sources and out-of-order older evidence do not renew a
fact. Scope privacy filtering and lineage/source suppression still apply.

Memory deletion retains tombstones, invalidates the whole supersession lineage,
and suppresses re-extraction of its evidence sources. Category opt-outs persist
with CAS versions; replay uses frozen deletion targets and immutable policy
receipts so an old event cannot revoke fresh explicit evidence or undo a later choice. The background Curator is
serial and receipt-based. No maintenance delivery or Approval is created.

State schema 19 adds the event journal, privacy epoch, frozen context policy,
compaction manifests and Experience Reviewer lifecycle. Existing Runs receive a
fixed default compaction policy. Accepted schema-18 generations are explicitly
marked to retain their original pre-compaction request representation on replay;
they still obey the privacy barrier. Model/Capability Snapshots do not change.
Conversation schema 3 retains derived summaries separately from original messages.
Memory schema 2 adds evidence, idempotent receipts and suppression records.

Verification lives in `scripts/verify_memory_maintenance.py`, repeated five times
by `make verify`, plus epoch/dispatch, migration, Curator, context and lifecycle
regressions. The harness kills the owner before model dispatch and after Memory
commit, then verifies stored state and fresh model requests. It does not establish
semantic model quality or selective dependency tracking.
