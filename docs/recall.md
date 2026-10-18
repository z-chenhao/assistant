# Durable context recall (ERII-22)

Main, Root and Child use the same read-only `builtin__memory__search` Tool and
ToolInvocation executor. The Host supplies the local user and Run's Agent identity.
Arguments cannot choose an owner, Agent, provider or database. Worker capability
reductions retain read-only recall and grant no Memory mutation capability.

## Configuration and privacy

```yaml
embedding:
  provider: openai_compatible
  endpoint: http://127.0.0.1:11434/v1
  model: nomic-embed-text
  dimensions: 768
  # api_key_ref: reference to a credential already in SecretStore
```

This is an explicit deployment example, not a bundled/downloaded/default model.
Serve the model separately. Authorized record text and queries go to exactly this
endpoint; choose a local server for local-only processing. Remote endpoints require
HTTPS. Credentials are resolved from SecretStore only. Redirects, malformed
responses, dimension mismatch, zero/nonfinite vectors and provider failures fail
closed with sanitized errors. There is no provider switch or FTS-only success.

Without embedding configuration, ordinary chat remains available and recall is
absent. This state does not constitute configured M6 vector recall. Adding it
affects new Runs only. Existing capability snapshots gain no new Tool or automatic
recall. Removing/changing it rejects a Run that already has recall capability.
Endpoint/model/dimension are hashed into the capability; the model sees neither
credentials nor endpoint strings. Credential rotation does not change vector space.

## Authority and indexing

| Database | Version | Authority / derived data |
| --- | --- | --- |
| `state.db` | 19 | Run/Tool ledgers; immutable per-Run, per-step recall snapshots |
| `conversation.db` | 3 | Immutable original messages, monotonic sequences, frozen ownership; derived FTS5/vectors |
| `memory.db` | 2 | Facts, evidence references, supersession links, tombstones; derived FTS5/vectors |

Files use the absolute data root, private permissions and WAL. Forward migrations
are transactional and reject future schemas. The populated Conversation v1 upgrade
indexes text without rewriting original JSON, IDs, timestamps or sequences.
Existing local Conversation IDs are Agent identities and migrate to local-user
ownership. New channels bind authenticated Conversation ownership before append;
rebinding fails.

Memory.Apply is a trusted Curator boundary. It atomically saves an immutable record,
evidence and index document. Identical duplicate IDs are harmless; changed payloads
conflict. Supersession replaces only an active record in the same user and exact
scope. Competing corrections permit one successor. Forget retains a tombstone and
excludes the whole supersession lineage from retrieval. Neither storage operation changes
Conversation evidence. Explicit local history retains originals. The public Forget
control additionally applies the privacy barrier before future automatic retrieval;
see ADR 003. Automatic extraction, remember/forget and compaction are described in
[maintenance operations](memory-maintenance.md). Explicit Forget also establishes
a privacy barrier over unfinished context; immutable audit is retained.

The chosen vector adapter stores dense vectors in SQLite
`recall_document.vector_json`, partitions by authorized scope/visibility/time using
a B-tree, and exhaustively ranks that partition by cosine in Go. It adds no native
extension or production dependency. This is exact vector search, with linear cost;
it makes no ANN or sublinear-performance claim. Measure large corpora before
increasing limits. SQLite FTS5 uses unicode61/BM25. unicode61 does not segment CJK
substrings; the configured model supplies semantic recall.

Authoritative writes atomically commit text and FTS. NULL vectors are recoverable
derived work. Media-only entries retain original Artifact references but are
excluded from text search; no empty embedding request can block other recall.
Search first embeds authorized missing records and installs each
vector atomically. A crash may leave some indexed records; it never produces
partial successful recall. Both candidate paths read one coherent database
snapshot. A concurrent, not-yet-indexed append fails that search closed; a later
caller can retry. Each database pins one embedding identity. Replacing it requires
stopping execution, backing up the three databases, clearing only derived vector
columns and `recall_configuration` in an offline transaction, and configuring new
Runs. Originals remain intact; there is no online mixing or hot migration.

A valid multipart original can exceed the embedding adapter's 64 KiB per-request
bound. Derived indexing splits it at UTF-8 boundaries into requests of at most
64 KiB and pools normalized vectors by chunk byte length into one document vector.
Every byte is embedded; full FTS text and the original source reference remain.
Exactly cancelling vectors use the first valid chunk vector as a deterministic
fallback. No vector is saved until all chunks succeed. Pending v2 documents and
v1 migration use this same path without rewriting originals or changing schemas;
already indexed short documents retain their vectors. This document-level pooling
does not provide passage ranking. Large hits still obey the existing output budget.

## Ranking, budget and recovery

Each Provider runs FTS5 and vector ranking concurrently. Owner, Agent, visibility
and inclusive time filters apply before candidate selection. Vector candidates
require cosine >= 0.25; each path supplies at most 200 candidates. RRF is
`sum(1/(60+rank))`, starting at rank one. Memory breaks equal fused scores by Core
tier, recent timestamp and stable reference. Conversation breaks ties by recent
timestamp and stable reference. The Service queries both Providers concurrently,
alternating their domain-ranked lists under one count and serialized evidence
budget. Scores remain private to each Provider.

Tool input is `query`, optional `limit` (1–50, default 10), and optional
`time_range: {start?, end?}` using RFC3339. Output contains only `content`,
`source_type`, `source_ref`, `occurred_at`, optional `author_role`. Invalid arguments
return INVALID_ARGUMENT; provider/index faults return PROVIDER_UNAVAILABLE without
partial output. The Tool is read-only, needs no approval, and reuses the existing
ToolInvocation replay/result pipeline.

For each model turn, each Run independently queries its latest explicit user/Task
input. Root and Child never copy Parent prompt state or search results. Recall
freezes in `run_recall` before model admission with claim/epoch and version checks.
Restart uses those exact hits; new turns query the current corpus. Execution
cancellation interrupts pending recall and admits no partial/frozen model turn.
Evidence is
untrusted runtime data in a dynamic suffix, preserving the static system/identity
prefix. It never becomes a user-authored Conversation message. The suffix uses at
most one quarter of the remaining conservative byte budget, capped at 16 KiB;
hierarchical compaction and the final model-window check also apply; see maintenance operations.

## Verification

```sh
make build
python3 scripts/verify_recall.py --binary bin/personal-agent --evidence evidence/recall.json
go test -race ./internal/recall/... ./internal/memory/... ./internal/conversation/...
go test -race ./internal/agent/sqlite -run TestRecall
```

The harness owns Launch (isolated data and explicit HTTP embedding/LLM Fakes),
Doctor (profile probe/activation), Drive (CLI chat, Task, Root/Child and real Tool
calls), Evidence (both indexes, scope exclusions, provenance and requests), Cleanup
(reaped CLI children, joined HTTP thread, deleted roots). It SIGKILLs the Go process
after recall commit, changes the corpus, and proves restart reuses the old turn
while a later turn sees new data. An embedding outage proves no partial recall or
model admission; restoring the same endpoint permits recovery. `make verify`/Ubuntu
CI repeat it five times in the security shard. Fresh/pending-v2 and populated-v1
multipart CLI scenarios prove an 80,001-byte original remains unchanged, all bytes
reach bounded HTTP requests, nine generations/four Runs complete, and a repeated
drain is inert. The actual HTTP adapter regression also covers UTF-8 boundaries,
the maximum 128 × 64 KiB original, mid-chunk failure/recovery and vector reuse.

Contracts cover lexical/vector-only fusion, ranking/time boundaries, user/Agent
scope, provenance, independent concurrent retrieval, competing supersession,
deletion, mutation/index rollback, old-owner rejection and migration. This is SC-08
retrieval/storage evidence, not full automatic formation or a product-quality
score. Deployment model quality remains release-time Live Shadow evidence.

References: [SQLite FTS5](https://www.sqlite.org/fts5.html),
[Ollama OpenAI-compatible embeddings](https://github.com/ollama/ollama/blob/main/docs/api/openai-compatibility.mdx).
