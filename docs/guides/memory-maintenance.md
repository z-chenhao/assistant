# Memory, context and verified experience

Enable maintenance explicitly with `memory.encryption_key_ref`, an existing
SecretStore reference to a 32-byte AES key. Store the key in the native credential
store or use an explicit `env:NAME` reference. YAML contains only the reference.
Keep the same key across process restarts and backups; key loss fails closed.
There is no implicit credential backend or per-record approval.

```yaml
memory:
  encryption_key_ref: env:ERII_MEMORY_KEY
context:
  threshold_percentage: 0.75
  recent_messages: 6
  summary_bytes: 2048
```

The Curator runs for the lifetime of TUI/default service/foreground `chat work`,
including idle service time, and is joined before databases close on shutdown. `memory work` drains a bounded serial batch for maintenance/diagnosis;
`memory status` shows durable receipts, applied/rejected state and sanitized error
codes. Transient failures stay pending with internal retry backoff. Permanent
conflicts are recorded as rejected, not retried forever or announced to the user.
The shared Event Outbox provides wakeups; the SQLite journal owns acceptance and
recovery. A crash between the Memory commit and journal acknowledgement reuses
the same event and its Memory receipt.

Main always discovers `builtin__memory__manage` with `remember`/`forget`. The Host resolves
scope and provenance. Without `memory.encryption_key_ref`, it returns
`PROVIDER_UNAVAILABLE` without accepting a mutation or changing privacy state.
Main and Workers always discover `builtin__memory__search`; it requires an
explicit embedding configuration. Without that configuration, search returns
`PROVIDER_UNAVAILABLE`, and automatic recall is disabled. Tool descriptions name
the missing configuration. No provider or encryption credential is implicit. Workers retain only read-only search; their results become
Curator source observations. The authenticated local equivalent is:

```sh
# Each command reads one JSON object on stdin. Reuse REQUEST_KEY to recover a receipt.
bin/erii -config config.yaml memory manage REQUEST_KEY
bin/erii -config config.yaml memory work
bin/erii -config config.yaml memory status
```

Remember input: `{"operation":"remember","content":"I prefer unsweetened tea"}`.
Correction adds `"supersedes":"memory:REF"` from an authorized search result.
Forget input: `{"operation":"forget","memory_ref":"memory:REF"}`.
Acceptance is durable intent, not a claim that asynchronous merging has finished.
For a durable opt-out, use `{"operation":"forget","category":"health"}` (also
finance, identity, relationships, goals, learning, preferences, facts, or all).
This tombstones current matching records and blocks future automatic extraction
in that category. `remember` with a category resumes future extraction; it does
not restore tombstones. Explicit remember of one fact can override an opt-out for
that fact. Event-frozen deletion targets and idempotent policy receipts prevent a
replayed old policy event from deleting fresh evidence or undoing a later choice.
Unknown/cross-scope references are rejected. Reusing a control key with changed
parameters conflicts. Memory writes never use Security Approval.

Automatic extraction conservatively retains bounded first-person reports from
user Conversation. Attributed Worker results use `User reported: ...`; untrusted
Worker material never counts toward Core promotion. Questions, long logs and
credential-bearing input are excluded. Exploratory and transient statements stay
Recall. Identical independent user evidence can promote a stable statement after
three sources. Explicit remember can establish Core, but exploratory language
remains Recall. Corrections create immutable successors; competing successors
conflict. This deterministic extractor preserves reports rather than inventing
diagnoses or certainty; it is not a general semantic fact normalizer.

With maintenance enabled, every derived Memory record is encrypted with AES-GCM
bound to record identity and scope; deduplication uses keyed hashes. This avoids
relying on a heuristic sensitivity classifier for storage protection. Persistent
FTS/vector tables contain no private plaintext. Authorized retrieval decrypts a
scoped working set into an in-memory SQLite index with temporary files disabled,
then uses the same FTS5/vector RRF adapter and the selected local embedding
provider. Encrypted Memory requires a literal loopback embedding endpoint; remote
embedding fails closed before any private input is sent. Existing remote-only
recall installations without maintenance keep their configured provider.

Existing Memory is sealed before enabling the configured maintenance provider;
derived FTS is rebuilt and old database/WAL pages are cleaned. A persisted cleanup
marker resumes cleanup after a crash. Original Conversation and frozen Run audit
records retain their existing private-file storage policy; this is not full disk
encryption. Back up all three databases together with the credential-store key.
Private working sets are rebuilt per search; this prioritizes confidentiality and
protocol consistency over an unmeasured performance claim.

Forget follows [ADR 003](../adr/003-memory-forget-revokes-unfinished-context.md). It
revokes unfinished context at acceptance, before asynchronous tombstones apply.
It deliberately stops every unfinished Run in the Agent scope and prevents old
automatic context from being imported into new Runs. Older Tasks are hidden from
model search and append is denied; a fresh self-contained Task can use new
independent input. Immutable audit remains.
Fresh chat input accepted while the revoked Main is stopping binds to a separate
queued Main and keeps that receipt on retries. Its execution waits for the old
owner to join or expire and settle. A fresh explicit remember of identical content
creates a successor with its own timestamp and source, so unrelated Forget does
not hide the newly supplied evidence. Old evidence remains excluded. State schema
20 migrates the Main admission index without rewriting stored input or snapshots;
all processes sharing the data root must use the current binary after migration.
An explicit Forget issued by a Main Tool can stop that Main before another model
reply; the durable receipt is visible through the control/status boundary.

Context is measured conservatively at three UTF-8 bytes per token, including
serialized Tool specifications. Compaction starts above
`context_window * threshold_percentage`. It removes complete older Tool exchanges,
then makes bounded extractive summaries of older semantic messages. Current user
input and static rules stay intact. Recent complete exchanges are retained within
the budget; reducible older history is summarized if the preferred recent count
would exceed it. Summaries carry
untrusted runtime origin and source references; they never become user instructions
or new recall evidence. Original Conversation/Run checkpoint content remains intact.
Derived summaries live in `conversation.db`; frozen manifests live in `state.db`.
Irreducible current input fails explicitly. A context-length rejection gets one
explicit Loop recovery attempt with a lower threshold, a new generation/step and
the same ModelSnapshot. The failed exchange remains audit evidence; no Tool is
rerun by this recovery. Policy freezes at Run creation; Children inherit it.

The Experience Reviewer is a trusted local control workflow. Ordinary Main and
Worker Skill capabilities remain list/view. `skill manage` reads a JSON operation:
`propose`, `verify`, `promote`, `reject`, `retire` or `list`. A proposal includes
`name`, `description`, a short reusable `procedure`, `reason` (`correction`,
`recovered_failure`, `success`) and accessible `source_refs`.

Verification supplies a `candidate_ref`, current `version` and `verification_ref`
(`invocation:REF`). It must resolve to a successful real workspace test invocation
in a completed authorized Run, reporting an untruncated passing `go test -race ./...`.
Ordinary success requires two independent Runs; correction/recovered-failure
candidates require one. Correction candidates require an original user source;
recovered-failure candidates require a failed Run source. Later independent
verification Runs add their own provenance without rewriting the candidate.
The procedure is immutable and binds all verification
records. Promotion is impossible before that gate. Publication is staged, protects
existing files and is recoverable if the process dies before the status commit.
Retirement retains an audit copy. New catalog versions affect new Runs; existing
frozen resources fail closed on drift. Raw logs and secrets are rejected as Skill
procedures. Mechanical evidence validates the test provenance, not the semantic
quality of the written method; model-quality review remains a release check.

Skill frontmatter opens and closes with an unindented `---` delimiter line.
The same punctuation inside a name or description is metadata, and body text
after the closing delimiter remains intact. Publication validates generated bytes
with the catalog's parser and size limit before writing or publishing a candidate;
unreadable metadata cannot advance it to promoted. The writer format and resource
hashes are unchanged, so no data migration is needed. The lifecycle harness covers
punctuation-bearing metadata through promotion, new-process startup, catalog
list/view, retirement and another startup.

Each configured catalog admits at most 128 active Skills. Pending publication
and retained retirement directories do not consume active slots or enter model
summaries. The loader enumerates directory entries in batches of 128, retains at
most 128 active entries, and keeps the existing per-file/per-Skill size limits.
Publication validates the full active catalog, then admits a new Skill under an
exclusive OS lock on the catalog directory. Readers share that lock; retirement
uses the same writer boundary. There is no persistent lock file, and process
exit releases ownership. A contended writer fails closed and can be retried.

At capacity, `skill manage promote` returns `RESOURCE_LIMIT`; the candidate stays
verified at its current version. Retirement preserves its audit copy and frees
one slot, after which that same candidate/version can be promoted. An identical
already-published Skill succeeds even at capacity, recovering a crash between
directory rename and candidate status commit. A partial pending `SKILL.md` can
be rewritten on retry; unexpected staged resources or links are rejected. No
audit copies are removed to make room. These changes need no migration and do
not change valid Skill bytes or frozen resource hashes. The lifecycle harness
proves last-slot publication, full rejection with unchanged candidate state,
retirement, retry and fresh-process readback with more than 128 retained entries.
That cardinality action selects an explicit 131,072-token local Fake profile so
the 128 summaries and list results fit; earlier small-profile compaction and
context-budget assertions run unchanged before that selection.

```sh
make build
python3 scripts/verify_memory_maintenance.py --binary bin/erii --evidence evidence/memory-maintenance.json
go test -race ./internal/context ./internal/memory/... ./internal/conversation/...
go test -race ./internal/agent/sqlite -run 'TestForget|TestContextLength|TestExperience'
```

The harness owns isolated Launch, profile/Fake Doctor, CLI/Tool Drive, database and
request Evidence, and process/thread/filesystem Cleanup. It covers health/goal/
finance/learning fragments, correction, encryption, restart, privacy revocation,
long history provenance and verified Skill promotion/retirement. It does not score
full GC-01–07, Computer cases or release-time Live Shadow behavior.
