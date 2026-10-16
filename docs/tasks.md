# Task delegation and Root recovery

ERII-11 is the first M2 slice. It adds Main-only Task discovery, create/append,
independent Root execution and durable Main follow-ups. Use a selected, probed
model through the existing TUI or `chat send` / `chat work` commands. There is
one shared Agent Loop and one ToolInvocation pipeline.

A self-contained delegation brief belongs in `task_manage(create).task`. Main
must include necessary background, constraints, completion criteria and expected
outputs. The Host generates an opaque TaskRef and a title; the receipt means
accepted, not finished. Short synchronous questions remain in Main.

| Canonical tool | Input | Output | Visibility / permission | Recovery |
| --- | --- | --- | --- | --- |
| `builtin__task__search` v1 | Optional `query`, `scope`, `status[]`, `limit` | `TaskSummary[]` | Main, `read_only` | `safe`; bounded 64 KiB result |
| `builtin__task__manage` v1 | Discriminated `create/append/pause/cancel` union | `{taskRef,status,accepted:true}` | Main, `task_control` | `safe`; transactional receipt dedup |

The exact schemas and strict validators live in `internal/task/task.go`.
Unknown fields, nulls, duplicate fields, wrong union arms, invalid limits and
unknown statuses return `INVALID_ARGUMENT`. Missing or inaccessible Task or
Artifact references return `NOT_FOUND`. Responses expose no Run/invocation IDs,
credentials, SQL, budgets or internal traces. Infrastructure write failures stop
at a recoverable boundary instead of becoming a misleading Tool success.

`pause` and `cancel` retain their canonical schema arms but return
`PERMISSION_DENIED` in this internal slice. Controlled pause/cancel, desired-state
recovery and ongoing-Task end policy belong to ERII-12. No nonempty ArtifactRef
is admitted before M3 supplies an authoritative ArtifactStore. No Child/JS Tool,
external effect, Approval or external channel delivery is enabled here. This is
not a claim that the full M2 or Durable Replay milestone is complete.

## Ownership and acceptance

Task + immutable anchor + first Root + frozen model/capability snapshots +
`run_ready` Outbox commit together. The same transaction stores a receipt keyed
by the accepted invocation, and by consumed source message plus normalized
action. Runtime derives both keys; models supply neither. Two identical calls
from one source return the same receipt. Distinct actions from that source can
create independent Tasks. Replaying the accepted Tool after an uncertain commit
reuses the saved receipt instead of creating another Task or instruction.

`append` retains the anchor and stores an ordered TaskInstruction. An active
Root consumes it at a model/tool/reply boundary. An already accepted model
exchange keeps its frozen request until its saved result is recovered. A late
instruction suppresses that provisional final result and causes another model
step. With no active Root, append reactivates the Task, increments `task_epoch`
and creates a new Root with current model/config snapshots. Earlier Roots stay
terminal. Previous Task instructions and authoritative prior business results remain available to the new Root; results retain runtime provenance.

Each Root receives safety, compact identity and Worker rules plus the Task
anchor/instructions. It receives no Conversation, Main checkpoint or mutable
peer context. The role-derived Tool snapshot removes both Task controls and
user delivery. Main has no `agent.run()`. Host lease admission bounds execution
to four Roots and reserves a separate Main slot, including across CLI processes.
The database still enforces one active Main per Agent and one Root per Task.

## Results, Reporter and priority

The Root's natural-language result is stored privately. Its terminal Run,
one-shot Task completion, semantic TaskUpdate and Outbox commit atomically.
Failed Roots leave their Task active; failures are not a Task state. Root
business failures do not stop unrelated Tasks. Every claim-bound Worker write
checks the current Task epoch and active state as well as the Run lease.

The durable Reporter scans authoritative updates; queue delivery is only a wake.
A stable event ID derives the Main request ID. Reporter consumer dedup and the
Main Inbox entry commit in one transaction. Repeated scans or wakes cannot
create another update, Main entry or delivery. Old epochs are consumed without
injection. Current available updates are assigned together to one new Main only
after its predecessor ends. If user input overtakes a queued follow-up before
its first generation, the runtime defers that batch and handles the user first.
Once a generation has been accepted, its context stays frozen; later user input
uses the shared Loop's normal stable Steering boundary.

A typed runtime `custom/task_update` retains message ID, time, origin and named
`TaskUpdateDetails`. Its provider conversion retains `origin:runtime` and a
JSON-framed untrusted-data boundary. It never becomes a System message or user
Conversation entry. Unknown custom types and malformed runtime provenance fail
closed. Main alone writes the eventual semantic answer and DeliveryRequest.
The TUI observes committed Main replies while Roots continue; it does not stream
Worker messages directly to the user.

Progress is a separate fenced projection. `Runs.Progress` accepts a meaningful
Host update, never infers one from low-level Tool activity. It creates no
TaskUpdate, Inbox, queue wake or delivery. `/status` reads recent titles, status
and progress without invoking a model. Search also returns the saved projection.

## Search policy

Without a query, scope defaults to `active`; with a nonempty query, to `all`.
Default limit is 20; the maximum is 50. Status filtering is additional to scope.
`recent` uses `tasks.recent_days` (default 30) and `tasks.recent_limit` (default
200), configured at startup. These restrict discovery, not database retention.
Sorting is stable by status relevance, combined text/semantic relevance,
updated time and opaque reference. Multiple plausible candidates remain visible
for Main to clarify; the Host does not silently append to the first match.

The local ranker combines lexical evidence with TF-IDF-weighted co-occurrence
similarity from the Agent's own Task anchors/instructions. Related terms learned
from the same corpus can retrieve documents without exact query overlap. It
requires no model, network or embedding dependency. Sparse/new corpora offer
limited semantic recall; multilingual semantic quality is unverified. Search
uses only the calling Agent/Conversation scope. Returned summaries are bounded
independently of the retained anchors, so all 50 rows fit the Tool result limit.

## Migration and verification

State schema v8 extends v7 without rewriting existing Runs, snapshots or
Conversation. Frozen v7 capability views continue through the same registry's
spec-checked selection. Forward tests cover empty and every v1–v7 database;
existing identity/config/event checks remain in place. Future schemas fail fast.

`make test`, `make test-race`, `make lint`, and `make verify` run the deterministic
checks. `scripts/verify_tasks.py` owns isolated launch, doctor, CLI/PTY drive,
SQLite evidence and cleanup. It kills actual processes before/after Task,
result, Reporter and follow-up commits, then reclaims work and checks receipts,
model reuse, one completion per Root and delivery dedup. `evidence/tasks.json`
contains before/after state and terminal output. `evidence/local-provider.json`
also drives Task creation, independent Root execution and runtime-framed Main
follow-up through the actual Node/Pi Bridge and owned HTTP peer. `evidence/task-cases.json`
contains 24 locked GC-02/04/08/18 Task fragments across wording, noise and key
order variants. SC-01/10/12 receive the applicable contracts; SC-04/05 remain
ERII-12 work. Scripted fragments receive no semantic score. Live-model quality,
online accounts and full product Golden Case gates remain unexecuted.

Shutdown joins the currently owned work at a stable boundary. SIGKILL recovery
waits for the five-second lease to expire. Re-run `chat work` or restart the TUI;
never edit the database to revive terminal Runs. No background child is required
to survive the command or test harness.
