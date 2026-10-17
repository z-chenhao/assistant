# Task delegation and Root recovery

The M2 slices add Main-only Task discovery, create/append/pause/cancel,
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

The later M3 slices supply authoritative Artifact validation, external effects,
approvals and delivery. ERII-17 adds [Direct Child delegation](children.md) to
new Worker capabilities. JavaScript and general Durable Replay remain later work.

## Controlled stop and continuation

| Current Task | pause | cancel | append |
| --- | --- | --- | --- |
| active | paused, epoch +1 | cancelled, epoch +1 | Stable Steering if a Root is active; otherwise new Root, epoch +1 |
| paused | Same receipt state, no epoch change | cancelled, epoch +1 | active, new Root, epoch +1 |
| completed | INVALID_ARGUMENT | INVALID_ARGUMENT | active, new Root, epoch +1 |
| cancelled | INVALID_ARGUMENT | Same receipt state, no epoch change | active, new Root, epoch +1 |

Stop acceptance atomically persists Task status/epoch, Run and invocation desired
state, receipt and Outbox. This immediately closes business-write and new-work
admission. The owner observes intent every 100 ms, cancels its safe provider/Tool
context, joins the current step, then settles the old Run as `cancelled` with
`task_paused` or `task_cancelled`. Already committed exchanges, Tool results,
instructions and progress remain available to the new Root as framed Task data.
An in-flight response arriving after acceptance is rejected, including its usage
write. Stop/progress events do not wake Main with a Worker business result.

Cancellation covers all current Task tree members and their invocations. The
scanner settles leaves before parents; live owners retain only lease/stop
authority, while expired or unclaimed work settles without a business claim.
Every claim-bound business write checks both ownership and the Task epoch.
An append racing stop waits outside the database transaction for the old tree
boundary, then atomically saves instruction/reactivation/new Root. The database
retains its one nonterminal Root constraint. A call interrupted during admission
remains recoverable under its frozen ToolInvocation and creates no partial append.

Only the same terminal Root's result is reported after its last descendant ends.
Host-owned `completion_policy` defaults to `once`; `ongoing` keeps the Task active
when one Root ends. The current model-visible create Tool makes one-shot Tasks;
no new policy argument is exposed. Future Automation uses the Host policy.
The original M2 tests retain stored topology and conservative unsafe-stop
fixtures. The M3 Tool harness verifies approvals and external reconciliation;
the [Child harness](children.md) exercises actual admission, recursive execution
and cancellation. No external exactly-once guarantee is made.

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
The final idle scan also rechecks durable updates and pending Inbox entries,
so a Root/Main commit between scans cannot strand a completion.
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

State schema v9 extends v8 with immutable completion policy, invocation stop
intent and legal stop/epoch transitions, without rewriting existing Runs,
snapshots or Conversation. Frozen capabilities retain their exact schemas.
Forward tests cover empty and every v1–v8 database, including v8 rollback;
existing identity/config/event checks remain in place. Future schemas fail fast.

`make test`, `make test-race`, `make lint`, and `make verify` run the deterministic
checks. `scripts/verify_tasks.py` owns isolated launch, doctor, CLI/PTY drive,
SQLite evidence and cleanup. It kills actual processes before/after Task,
result, Reporter and follow-up commits, then reclaims work and checks receipts,
model reuse, one completion per Root and delivery dedup. `evidence/tasks.json`
contains before/after state and terminal output. `evidence/local-provider.json`
also drives Task creation, independent Root execution and runtime-framed Main
follow-up through the actual Node/Pi Bridge and owned HTTP peer. `evidence/task-cases.json`
contains 24 locked `m2-v2` GC-02/04/08/18 Task fragments across wording, noise and
key order variants, including GC-04 pause/continuation and GC-08 cancel/continuation.
SC-01/04/05/10/12 receive applicable contracts. `verify_task_control.py` adds live
PTY cancellation and three real SIGKILL boundaries around stop acceptance and
settlement, repeated five times by `make verify`; evidence is
`evidence/task-control-{1..5}.json`. Go contracts cover transition rejection,
duplicate and concurrent append/pause/cancel, stale results/claims, synthetic
descendant propagation, retained results, one-shot/ongoing policy and shutdown.
Scripted fragments receive no semantic score. Live-model quality,
online accounts and full product Golden Case gates remain unexecuted.

Shutdown joins the currently owned work at a stable boundary. SIGKILL recovery
waits for the five-second lease to expire. Re-run `chat work` or restart the TUI;
never edit the database to revive terminal Runs. No background child is required
to survive the command or test harness.
