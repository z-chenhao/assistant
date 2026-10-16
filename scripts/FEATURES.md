# Foundation feature map

Run `make verify` from the repository root. `scripts/verify.py` drives the built
CLI and independently reads SQLite/JSONL. `internal/event/integration` owns the
JetStream and process-kill mechanics. No online account or installed broker is
required. Each harness launches disposable state, checks readiness, drives the
real boundary, records evidence, and always reaps its processes/removes state.

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Help/doctor | `bin/personal-agent -help` | `cmd/personal-agent:run` | Usage; no state created |
| Startup diagnostics | `bin/personal-agent [-config path] status`; no-command with redirected stdin | `agent/sqlite.Store.Diagnostics` | setup_required, null profile, closed ingress; stored identity |
| Diagnostic command | `bin/personal-agent [-config path] status` | `cmd/personal-agent:run` | JSON object, zero exit |
| Configuration | `-config`, `ERII_*` overrides | `config.Load` | Invalid fields/URLs/durations rejected without value echo |
| SQLite bootstrap | Either startup path | `agent/sqlite.Open` | Schema v12, WAL, one Agent, one atomic fresh-bootstrap event |
| Migration safety | Empty, real v1/v2/v3/v4/v5/v6, future schema | `agent/sqlite.migrate` | Prior Agent/config/timestamps, model metadata/snapshots and Outbox/Inbox preserved; future rejected; failed v3/v4/v5/v6/v7 DDL rolls back |
| Model metadata | `bin/personal-agent model add` (stdin JSON), `model list`, `status` | `llm/sqlite.Store.AddProfile/ListProfiles` | New process and SIGKILL/restart preserve profile; unverified/version 1; independent SQLite/canonical JSON readback; ingress stays closed |
| Metadata validation | `model add` | `llm.ProfileInput.Validate`, CLI decoder | Unknown/credential fields, credential-bearing URLs, invalid enums, dangling/mismatched auth and pre-Probe activation rejected without value echo or writes |
| Metadata CAS | Host storage, no CLI update surface | `llm/sqlite.Store.UpdateProfile/UpdateAuthState` | One winner across DB connections; stale CAS and event failures leave metadata/Outbox unchanged; profile edits reset verification |
| Secret references | `model login [ENV_NAME]`; trusted Host only | `llm.RegisterAuth`, `llm/secret`, `llm/memory` test Fake | Put before metadata; database failure cleans new reference even after cancellation; cleanup failure explicit; DB/WAL/SHM and output exclude credential material |
| Immutable model snapshot | Main Run creation and trusted transaction fixture | `llm/sqlite.Store.FreezeProfile/Snapshot` | Ready-only, frozen profile version and canonical endpoint hash; no CredentialRef; auth/profile changes preserve snapshot; update/delete/REPLACE rejected; outer transaction rollback removes snapshot/event |
| Provider Bridge | Explicit `-test-bridge success` | `llm/bridge.Open`, `llm/fake.Serve` | Version handshake, long-lived stdio JSON-RPC, sanitized errors, process crash/restart, orderly shutdown/reap |
| Catalog and auth fixtures | `model catalog`; Bridge contracts | `ModelCatalog`, `ProviderAuth` | Candidate catalog and refresh; UI-neutral auth events, continue/refresh/status/logout |
| Capability Probe | `-test-bridge success model test ID` | `llm.Control.Test`, `bridge.Client.Run`, `llm.Prober.Run` | Actual stream, Tool Call/Result continuation, cancellation and clipping; tool-less model rejected; no implicit provider |
| Atomic selection | `-test-bridge success model select ID PROFILE_VERSION CONFIG_VERSION` | `llm.Control.Select`, `llm/sqlite.Store.Select` | Version-bound Probe; profile/config/Outbox rollback; concurrent config CAS; v3 migration preserves auth/profile/snapshot/events |
| Effective readiness | `status`, foreground startup | `agent.EffectiveReadiness`, `llm.Control.Available` | Explicit test injection required after restart; absent/invalid profiles, credentials and incompatible Bridge close ingress; ready profile survives SIGKILL/restart |
| Cancelled setup | `model cancel` | CLI read-only diagnostics path | setup_required and identical SQLite dump; no default provider |
| Generation contract | Bridge tests repeated by `make verify` | `LLMProvider.Generate/Continue/Cancel/Dispose` | Sequence dedup/drift/gaps, one completion, immutable snapshot/session identity, partial-crash rejection, received completion before EOF, clipping and bounded errors |
| Burst delivery with slow sinks | Real subprocess and mailbox regressions repeated by `make verify` | `bridge.eventStream.accept`, `bridge.Client.generate` | 129-event slow sink; full 4096-event burst, duplicates and pre-ack events retained; clipping before buffering; concurrent catalog/cancel/Close progress while sink waits; overflow and changed duplicates fail closed |
| Generation rejection and explicit cancellation | Real lifecycle peer regressions repeated by `make verify` | `bridge.Client.generate/Cancel`, `bridge.eventStream.cancel` | Host rejection cancels accepted work or retires peer with original error and no replay; provider terminal error skips cleanup; ack-only Cancel settles matching local call, interrupts pre-ack wait and fences late/queued completion; independent catalog and ledger prove state |
| Cancel before dispatch | Real subprocess race and held-registration regressions repeated by `make verify` | `bridge.Client.exchange/Cancel`, `eventStream.dispatchDone` | Valid 900,000-byte Generate/Continue requests race cancellation after registration; peer observes exactly one start and cancel, no active work or replay; dispatch wait respects deadline and leaves catalog/Close available |
| Stalled Bridge and pre-ack cancellation | Bridge transport regressions repeated by `make verify` | `bridge.Client.exchange/write/cancelGeneration` | Real subprocess stops reading 512 KiB request; caller/write-admission deadlines and forced Close remain bounded; cancellation before start/continuation acknowledgement reaches peer; late ack cannot revive generation |
| Restart admission and internal RPC timeout | Bridge lifecycle regressions repeated by `make verify` | `bridge.Client.ensure/startProcess/Close/cleanupGeneration` | Concurrent restart waiters respect caller deadlines; single/concurrent Close interrupts initialization and reaps it; internal start/continuation timeout cancels accepted work while caller stays live; failed cancellation retires peer; no automatic generation replay |
| Failed Probe cancellation cleanup | Active-profile retest regressions repeated by `make verify` | `llm.Control.failedProbe`, `llm/sqlite.Store.InvalidateProbe` | Unacknowledged internal cancellation retires real subprocess, revokes tested proof and degrades active config with events; replacement catalog leaves ingress closed; actual caller cancellation preserves prior state |
| Legacy metadata execution limits | `model list`, `model test/select`; v3 migration fixture | `ProfileInput.Validate/ValidateActivation` | Above-limit v3 metadata remains unchanged/readable; one such row does not block listing; Probe/selection/readiness still enforce current Host limits |
| Local Pi setup | Configure `llm`; `model discover URL`, `model add URL MODEL_ID [AUTH_ID]`, `model catalog ID/test ID/select ID ...` | `provider-bridge/bridge.mjs`, `bridge.OpenLocal`, `llm.Control` | `evidence/local-provider.json`: all five probe behaviors, manual ID/catalog independence, activation and restart, keyed/keyless headers, failed retest closes ingress |
| Local HTTP security/errors | Node contracts in `make test`/`make verify` | Pi transport wrapper | `evidence/pi-bridge.tap`: redirects rejected, no fabricated key, bounded retries, sanitized 400/401/404/413/429/5xx, malformed/partial SSE, cancel/dispose |
| Malformed Tool arguments | Actual Node/HTTP contracts in `make test`/`make verify` | `generate` Tool Call normalization | Incomplete JSON and array/null/scalar arguments reject without invented calls, raw diagnostics, completion or retry; explicit `{}` remains valid |
| Session disposal | `LLMProvider.Dispose`; real Node and deterministic lifecycle peers | `bridge.Client.Dispose`, generation admission fence | Generate/Continue settle after ack; queued completion and late admission fenced; already published results and unrelated sessions preserved; dispatch precedes disposal; failed cleanup retires peer |
| Streaming generation deadline | Actual trickling HTTP response in Node contracts | Bridge-owned 30-second timer | Body consumption terminates, HTTP closes, one sanitized PROVIDER_UNAVAILABLE event, no completion or retry; catalog remains available |
| Frozen endpoint and usage | Model generation and v4 migration fixtures | `llm/sqlite.Resolver/RecordUsage`, schema v5 | Prior endpoint survives profile edit/restart; atomic failed migration; immutable usage/snapshot attribution and no invented token counts |
| Actual Pi process crash | `make test`, repeated in `make verify` | `bridge.TestActualPiBridgeClippingCrashAndRestart` | Kill Node after delta, no partial result, same Bridge restart/catalog, clipping and token readback |
| Native credentials | `ERII_TEST_KEYCHAIN=1 go test ./internal/llm/secret -run TestKeychainOwnedRoundTrip -v -timeout 30s` | Security.framework backend | Owned synthetic credential add/read/update/delete; optional macOS check, CI uses Fakes/env references |
| Local observability | Configured `logs/` | `observability.Telemetry.Start` | JSONL logs and correlated startup/database spans |
| Shutdown/restart | SIGTERM/SIGKILL then `status` | `cmd/personal-agent:main` | Stable identity, timestamps, SQLite dump |
| Transactional Outbox | `Store.Transaction` + `event/sqlite.Enqueue` | `event/sqlite.Enqueue` | Domain/event rollback together; unique ID rejects duplicate/drift |
| Event relay | `bin/personal-agent [-config path] relay` | `event.Relay`, `event.Publisher.Flush` | Actual CLI offline launch, broker recovery, same event ID, SQLite publication marker, clean shutdown |
| CloudEvents transport | `EventBus.Publish/Subscribe` | `event/nats.Open`, `event/fake.New` | Shared contract; structured wire JSON/extensions; arbitrary JSON rejected; durable redelivery |
| Transactional Inbox | `Store.Events().Handle` | `event/sqlite.Store.Handle` | Domain+Inbox commit before ack; rollback, ack loss, consumer scope, concurrent duplicates |
| Late/out-of-order snapshots | Domain handler on received CloudEvent | `event/sqlite:TestDuplicateLateAndOutOfOrderSnapshots` | New version wins; old epoch/version ignored; duplicates skip effects |
| Process crash recovery | `make verify` fault suite | `event/integration:TestProcessCrashBoundaries` | Seven SIGKILL checkpoints, full SQLite close/reopen, state readback, one handled effect |
| Broker restart | Integration contract suite | `event/integration:TestNATSBrokerRestartPreservesAcceptedUnackedEvent` | Same file store and durable cursor retain accepted/unacked event |

| Current ChatGPT sign-in | `model login-chatgpt [ACCOUNT_ID]`, `model accounts` | `account.Manager.Login`, `provider-bridge/account.mjs` | `evidence/account.json`: actual CLI Browser OAuth, PKCE, random loopback, identity validation, separate account/client registration, masked labels and stable private device ID |
| Account model discovery | `model account-catalog ACCOUNT_ID`, `model account-add ACCOUNT_ID MODEL_ID` | `accountCatalog`, existing Pi Responses generation/Probe | Dynamic account catalog/visibility, rejected hidden IDs, actual Tool continuation/cancel/clipping, current trusted endpoint only |
| Credential lifecycle | `model refresh ACCOUNT_ID`, `model logout ACCOUNT_ID`, startup recovery | `account.Manager`, `llm/sqlite.AccountState`, schema v6 | Six-process single-flight, rotation failure, stale CAS, logout/proof/readiness revocation, owned Keychain or explicit memory-socket fixture, SIGKILL/restart, no rotating replay |
| Account final acceptance | `go test ./internal/agent/sqlite -run TestAccountFinalAcceptance -v`; repeated by `make verify` | `account.Manager.Observer`, `llm/sqlite.RecordUsage`, `bridge.Client.generate` | Actual Go/Node/Pi result paused after Guard; second Host/DB logout, reauthorization or auth loss rejects text/completion/usage success at transaction boundary; refresh retains acceptance; storage failure publishes no completion |
| Cancellation during account acceptance | `go test -race ./internal/agent/sqlite -run TestAccountCancellationAtFinalAcceptance -v`; repeated by `make verify` | `bridge.eventStream.finalize/cancel`, `bridge.Client.generate`, `llm/sqlite.RecordUsage` | Generate/Continue × Cancel/Dispose/context × observer/open-transaction/post-commit barriers; pre-commit cancellation stores cancellation, clears result and suppresses completion; post-commit ack preserves accepted result; existing auth fence and local cancellation contracts retained; held-commit deadline regression drives independent catalog RPC |
| Account Tool namespace replay | Node account continuation contract; CLI account Probe | `provider-bridge.context`, Pi Responses | Captured continuation wire retains fixed `erii` namespace, arguments and matching call/output IDs; local peer rejects omission |
| Deferred account device login | Not offered | ADR 001 / baseline §5.4 | Explicitly deferred by user; never counted as passing |

Reports: `evidence/verification.json`, `evidence/models.jsonl`, `evidence/events.json`,
`evidence/relay.json`. `make test-race` separately checks shared-memory access;
SQL uniqueness/transaction and protocol tests prove their respective invariants.
This includes the local-provider M1 slice, with no general Durable Replay or exactly-once claim.

## Main slice (ERII-9)

`scripts/verify_main.py --binary bin/personal-agent --evidence evidence/main.json`
owns Launch/Doctor/Drive/Evidence/Cleanup. It runs from `make verify`, creates
disposable state, starts only explicit Fakes, and reaps all owned children.
See [Main operations](../docs/main-loop.md) for recovery and limits.

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Durable ingress | `chat send [REQUEST_KEY]`, stdin text | `agent.Actor.Submit`, `agent/sqlite.Runs.Submit` | 16 concurrent CLI processes, one Main, stable receipt, drift rejection, setup gate |
| Execute/recover Main | `chat work` | `agent.Actor.Drain`, `agent.RunAgentLoop/ContinueAgentLoop` | Nine SIGKILL boundaries; expired claim automatically reclaimed; terminal Runs not replayed |
| Stable steering | Submit while `chat work` is active | Loop model/tool/reply boundaries | Separate ordered user messages; stale final answer overtaken; no second Main |
| Context and model freeze | Create Run, switch profile, restart worker | `MainContext`, `Runs.Initialize/Generation` | Frozen role fragments, history boundary, request/schema hashes; new profile only on a new Run |
| Conversation | `chat history` | `conversation/sqlite.Append/List`, `Runs.Flush` | Independent DB, monotonic sequence, idempotent immutable IDs, no Tool trace; cross-DB missing writes repaired |
| Direct Tool | Model requests `builtin__text__echo` | `tool.Registry`, `tool.Executor`, bound invocation Store | Real pure execution, schema rejection, absent Tool error, per-exchange uniqueness, immediate completion; slow/fast order regression |
| Claims/CAS/wait/failure | Host execution boundary | `Runs.Claim/Checkpoint/Wait/Complete/Fail` | Old-token/epoch rejection, version conflict, invalid transition, terminal irreversibility, auth/usage/result atomic rollback |
| Result/trace events | Host EventSink | Loop + Executor, `Runs.Trace` | Persist completion before message/tool end; ordered trace envelope; no streaming EventQueue writes |
| Graceful shutdown | SIGTERM during a generation | Actor controlled release | Accepted model result retained, unclaimed nonterminal Run, no business cancellation, recovery reuses result |
| Local/account Main path | Configured local endpoint or account profile | Real CLI -> Go Actor/Loop -> Node/Pi -> Tool -> Conversation | `local-provider.json` and `account.json`, actual System Prompt and Tool continuation; no online account required |
| Main L0 fragments | `make verify`, `evals/fixtures/m1-v2.json` | `case_runner.py`, `run_cases.py`, actual TUI | 18 GC-01/09/10 wording/noise/order fragments; normative facts; fixed scenario clock; locked fixtures/scoring; full Cases not_applicable and semantic scores null |
| M1 storage migration | Startup from empty or v6 | `agent/sqlite` v7, `conversation/sqlite` v1 | Prior identity/config/events retained; partial DDL rolls back; future schemas rejected |

DeliveryRequests use the M3 Delivery Outbox below. No scored product-Case or M5
Durable Replay claim.

## Delivery slice (ERII-15)

`scripts/verify_delivery.py` extends the maintained MainHarness/Terminal mechanics
for isolated Launch, fixture/schema/readiness Doctor, real CLI/PTY Drive, direct
SQLite snapshots and transcripts as Evidence, and child/PTY/data Cleanup.
`make verify` runs it five consecutive times; `evidence/delivery-N.json` records
20 SIGKILL/restart cases per run and deterministic transport/approval failures.

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Semantic delivery and retained history | TUI, `/history`, `delivery work/list`, `chat work` | `Runs.DeliverReply`, `delivery.Service`, `delivery/terminal.Adapter` | Main and Root final replies accepted once into the terminal projection; PTY transcript and destination/content readback; duplicate drains/callbacks preserve receipts |
| Recoverable cross-database commit | Main/Root reply, kill/restart | `Runs.PrepareReply/Flush/DeliverReply` | Deterministic reply/delivery IDs; Conversation precedes enqueue and checkpoint; ten before/after boundaries per role; no duplicate logical generation or semantic message |
| Delivery retry and reconciliation | Live TUI polling or `delivery work`, explicit `-test-delivery` | `delivery.Service.Drain`, `delivery/sqlite.Store` | Known transient backoff and exhaustion; accepted-send query; unknown stays query-only; terminal sends never rewrite Conversation; late-ack fencing and concurrent-consumer Go tests |
| Security approval notification | `/approve ID HASH`, `/deny ID HASH`; CLI `approval` | `toolStore.Prepare`, `Runs.Decide`, `tui.terminal.command` | One Security notice without Main inference; frozen hash callback wakes original Root; duplicate decision produces no notice; existing rejection/expiry/cancel/parameter-drift gates remain required |
| Notification policy | Root completion/failure; read-only `/status` | `replyIntent`, `notification`, `delivery.Request.Validate` | Only completion/failure/needs_input/approval accepted; progress cannot enqueue; Worker results still return through Main |
| Delivery persistence and configuration | Startup, delivery YAML policy | State schema v12, `config.Load` | Every prior schema migrates; populated v11 receipts/keys/status/attempts survive; failed migration rolls back; legal/illegal states, atomic event enqueue, bounded config |

Physical stdout may redisplay retained history on reconnect. No generic external
exactly-once guarantee, Web/Feishu/UISpec implementation, or full Golden Case
score is claimed. Operations and state details: `docs/delivery.md`.

## Terminal slice (ERII-10)

`scripts/verify_tui.py` owns launch/doctor/drive/evidence/cleanup through a real
PTY and the existing local/account HTTP peers. `make verify` runs it five times.
See [terminal operations and proof limits](../docs/tui-and-evals.md).

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Fresh setup | Default launch on terminal or `tui` | `tui.Session`, existing `llm.Control` | Actual PTY discovery/manual URL or browser account setup → Probe → explicit selection → ready |
| Terminal conversation | Plain text, `/history` | `agent.Actor.Submit/Drain`, semantic Conversation | Actual Pi Main/Tool/reply for both paths; busy Steering enters one Main; no raw Tool trace |
| Profile controls | `/models`, `/test ID`, `/select ID`, `/setup`, `/login [ID]`, `/logout ID` | Existing Model control plane and account Manager | Dynamic account IDs; versioned activation; logout/degraded gate; new Run model switch with old Snapshot retained |
| Cancel and readiness | `/cancel`, Ctrl-C, SIGTERM, unavailable Bridge | `tui.Session`, `agent.EffectiveReadiness` | Fresh setup stays closed, active profile retained; login cancels without credentials; prompt input and executor joined; five consecutive runs |
| Terminal recovery | Restart TUI after actual SIGKILL | Existing Main recovery and Conversation intents | Identical semantic state for both providers; held generation with Steering reused; no duplicate reply |
| Terminal input byte boundary | Default launch; `verify_terminal_input.py` in `make verify` | `tui.OpenInput`, bounded `readInput`, same Actor | 12 actual PTY cases: exact 16 KiB ASCII/UTF-8, overflow rejected without partial Main rows, recovery, editing/EOF, echo on/off, signal shutdown, flags/termios restored; macOS review reproducer retained as before/after evidence |
| Eval result/scoring protocol | `run_cases.py`; `make test` scorer regressions | `case_runner.py`, locked `scoring-v1.json` | Real TUI drives 18 fragments; full evidence and snapshots; missing grades null, hard/mechanical failure cannot be overridden, thresholds enforced |

## Task delegation slice (ERII-11)

`scripts/verify_tasks.py` extends the existing CLI/PTY launch, doctor, drive,
evidence and cleanup harness. `scripts/run_task_cases.py` uses the shared Case
result/scoring and Terminal mechanics. Both run in `make verify` without accounts.
See [Task operations and proof limits](../docs/tasks.md).

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Task discovery and mutation | Natural-language Main delegation; canonical Task tools | `task` schemas/validators; accepted `ToolInvocation` -> `tools.ExecuteBuiltin` | Strict union/null/unknown-field checks, defaults/50 limit, scope and hybrid ranking, ambiguous real CLI search, duplicate source/call receipts |
| Atomic delegation | Main `task_manage(create)` | `Runs.createRoot` and state transaction | Task/anchor/Root/snapshots/Outbox and receipt readback; before/after SIGKILL |
| Append and continuation | Main searches then `task_manage(append)` | `TaskInstruction`, Root Steering and `createRoot` | Active frozen-generation injection, 16 concurrent instructions, immutable anchor, new Root/epoch after completion, no terminal revival |
| Root concurrency / responsive Main | Same TUI or `chat work` | Actor scheduler, shared Loop, database lease admission | Eight Tasks/four Worker leases, reserved Main slot, live PTY receives acceptance and new answer while Root is held |
| Actual provider Task path | Local endpoint through Node/Pi | Same Task/Root/Reporter path, production Bridge conversion | `local-provider.json`: Task union reaches HTTP, Root has no Task controls/Conversation, runtime-framed follow-up returns through Main |
| Root result / Reporter | Root completes or fails | `finishTask`, TaskUpdate/Outbox, `Report`, Main Inbox | One update and consumer receipt per Root, failure leaves active Task; four result/report SIGKILL boundaries; repeated drain unchanged |
| Batched follow-up / user priority | Concurrent completions while Main runs | `promoteFollowups`, `Steering` | Go contracts: terminal-commit/idle-scan races, one batch, no current-Main injection; overtaking input defers queued update before its first generation |
| Runtime provenance | Main receives a Task update | Typed `AgentMessage`, provider conversion | `origin:runtime`, untrusted JSON boundary, named safe details; unknown type/provenance fail closed; no Worker Conversation/System entry |
| Read-only activity | `/status`, `task_search` | `Runs.Progress/Activity`, `searchTasks` | Progress read/write contract creates no update, model wake or delivery; PTY status shows completed work |
| M2 due fragments | `run_task_cases.py` | Locked `m2-v2` fixture and shared scoring protocol | 24 GC-02/04/08/18 fragments; SC-01/04/05/10/12 applicable invariants; pause/cancel followed by new Root; semantic scores remain null; Child/Artifact behavior not_applicable |
| M2 storage | Startup from empty or v8 | State schema v9 | Every prior version migrates; v8 rollback/retention; old frozen capabilities still selected; Task/Run epoch fencing and audit immutability |

## Task controls (ERII-12)

`scripts/verify_task_control.py` reuses the CLI/PTY harness for Launch, Doctor,
Drive, Evidence and Cleanup. `make verify` repeats it five times. Reports retain
actual PTY output and persisted state; all processes and disposable databases
are cleaned up. See [Task operations](../docs/tasks.md) for transitions and limits.

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Controlled pause/cancel | Main searches then `task_manage(pause/cancel)` | `stopTask`, Actor execution context, `SettleStop/recoverStopsTx` | `task-control-{1..5}.json`: accepted provider generation cancelled; status/epoch/desired state persisted; exact Root reason; no stale result/followup; three SIGKILL boundaries |
| Continue stopped work | Main `task_manage(append)` | `executeBuiltin`, `createRoot`, immutable instruction/receipt | New Root and epoch after paused/cancelled/completed; one nonterminal Root; preserved anchor, committed Tool/model results and progress; duplicate and concurrent controls |
| Epoch/owner fencing | Current Task tools and Host persistence boundary | `fence`, `Claim`, `AcceptGeneration`, `Trace` | Go contracts repeated five times: old checkpoint/progress/generation/Tool/reply/terminal/trace writes rejected; usage rollback; out-of-order response cannot change new Task |
| Tree stop and completion | Internal stored tree relationships | `recoverStopsTx`, `finishTask` | Go synthetic-tree contracts: leaf-before-parent, waiting descendants, safe cancellation/unsafe unknown, no unknown replay, last descendant releases one-shot completion, ongoing stays active; Child admission remains M4 |
| Shutdown versus user stop | SIGTERM/context cancellation; later `chat work` | Shared Loop, Actor release | Current generation retained and same Root reclaimed after shutdown; Task/epoch untouched; deterministic scan/claim/after-claim cancellation barriers preserve accepted work and return normal cancellation (`TestLoopShutdown*`); user stop gives terminal Root and explicit reason |

| Unified Tool feature | Reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Frozen approval and decision recovery | `approval list`, `approval approve/deny ID HASH`, `chat work` | `tool.Executor`, `sqlite.tools.Prepare/Start`, `Runs.Decide` | `evidence/tools-{1..5}.json`: persisted waiting with no active claim, CLI restart, exact hash, duplicate callback, denial and no early effect |
| Steering invalidates granted/unexecuted action | `chat send` or `task_manage(append)` while approval waits | `Runs.Submit`, `tools.ExecuteBuiltin`, `invalidateApprovals` | CLI GC-12 fragment changes Monday to Tuesday; only new frozen Tuesday action executes; Go expiry/wrong principal/hash/immutability regressions |
| External effect recovery and reconciliation | `chat work`, `approval reconcile` | `Executor.executeOne`, `Runs.ReconcileUnknown` | Nine real SIGKILL/restart scenarios per harness run; separate Fake ledger, stable keys, query-before-retry and never; oversized recovery receipt persists unknown before Worker delivery, keeps Task active, reaches Main and later reconciles without dispatch; persisted Worker regressions also cover inconsistent/nonterminal query evidence; unknown absence does not authorize retry; cancel of an approval waiter never dispatches |
| Tool capacity and independent results | Parallel model Tool batch | `sqlite.tools.Start/Defer/Finish`, `Executor.Execute` | `TestToolQuotasAndDynamicConflictKeys` (all five scopes and intersecting keys), `TestToolResultsCommitIndependentlyAndDeliverInDeclarationOrder`, race detector |
| Host errors and schema/policy boundary | Direct Tool results | `tool.NewRegistry/Validate/Normalize` | Hidden Worker Tools return NOT_FOUND before validation; strict schema, forged fields/duplicate keys rejected; bounded transient retry and sanitized raw provider errors; no model credentials available to adapters |

Tool harness lifecycle: **Launch** `MainHarness` with explicit Fake provider/tool;
**Doctor** model add/test/select and ingress readback; **Drive** ordinary chat and
approval CLI commands; **Evidence** domain state, approval records, Outbox and
separate external ledger plus kill checkpoints; **Cleanup** join every child and
remove isolated roots. `make verify` runs five consecutive repetitions. Full
GC-11/12/13 product scenarios, Code Mode and Artifact spillover are not claimed by
these Tool fragments. Operational details: `docs/tools.md`.

## Artifact slice (ERII-14)

`scripts/verify_artifacts.py --binary bin/personal-agent --evidence evidence/artifacts.json`
owns Launch (isolated data root + explicit Fakes), Doctor (selected profile,
schema/readiness), Drive (CLI and normal Main/Task/Root Tool path), Evidence
(independent SQLite, bytes/hash readback, history, trace and SIGKILL residue),
and Cleanup (join children and remove disposable roots). `make verify` repeats
SC-09/recovery five times. See [Artifact operations](../docs/artifacts.md).

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Binary save/open and audit | `artifact save NAME MIME` stdin; `artifact open/describe REF` | `artifact/sqlite.Put/Open/Describe` | Exact bytes/SHA-256 across processes, opaque path-free descriptor, malformed/forged/traversal rejection |
| Immutable lifecycle | `artifact delete REF VERSION`; `artifact cleanup [apply]` | `artifact/sqlite.Delete/Cleanup/Recover` | Tombstone/CAS, expired/tampered/symlink rejection, quota/ref guards, dry-run, active/paused Task reads, retention after Task end |
| Reference inputs | `chat send-parts KEY` stdin parts; Task create/append `artifactRefs` | `Runs.Submit`, `tools.executeBuiltin` | Same transaction validates/pins refs, bounded text read, original hash unchanged, forged input creates no ingress |
| Large/binary Tool results | Normal Executor result; `chat work`, `approval reconcile` recovery | `tools.FinishResult/ReconcileResult`, `Runs.ReconcileUnknown/storeOutput`, `tool.RecoverResult` | Result ref + metadata atomic commit, no payload in history; both binary Query recovery paths through `verify_artifacts.py --recovery-only` (also in the full gate), exact receipt bytes/hash and caller/committed/replayed equality, real post-effect SIGKILL with no second effect; race regressions for scope, frozen deadline/cancel/stream closure, invalid evidence/refs/metadata, read and commit failure retaining unknown, and terminal Run immutability |
| Final output selection | `builtin__artifact__publish` | `PublishTx`, `PrepareReply`, `taskUpdate/promoteFollowups` | Draft omitted; explicit Root refs in update; Main preserves selected media/file part types and bounds final refs to the Conversation envelope; GC-15 transport fragment reopens report |
| Terminal Artifact delivery | TUI conversation and `/history` | `tui.terminal.history` | Owned PTY sees final attachment ref; CLI open verifies delivered bytes; clean terminal shutdown |
| Task visibility and parallel outputs | `builtin__artifact__list/read` in scoped Worker; Host child-shaped contract fixtures | `DescribeTx`, Run-derived scope | Same Task read, cross-user/Agent/Task/Run denial, eight child-shaped writers cannot overwrite same display name |
| Object publication/recovery | Real save Tool during `chat work` | `PutTx`, startup `Recover` | Six SIGKILL boundaries, orphan cleanup, one final object, committed ref reused with no second attempt |
| Schema v11 migration | Startup on empty/v10 DB | `agent/sqlite.migrate` | Prior identity/config retained, partial DDL rollback, prior v1–v9 regressions retained |

## MCP, Skills and coding fixture (ERII-16)

`scripts/verify_capabilities.py --binary bin/personal-agent --evidence evidence/capabilities.json`
owns Launch (private data/provider/Skills/checkout roots), Doctor (explicit Fake
model add/test/select, validated scopes), Drive (ordinary chat/Task/approval CLI),
Evidence (SQLite, real test output, full tree hashes and SIGKILL/restart), and
Cleanup (join every process and remove disposable roots). `make verify` repeats
five times. See [capability operations](../docs/capabilities.md).

| Feature | How to reach it | Authoritative entry point | Evidence |
| --- | --- | --- | --- |
| Structured MCP stdio | Configure allowlisted `mcp`, normal Direct Tool call | `tool/mcp.Definitions`, common Executor | Real initialize/list/call; schema drift, malformed metadata/params, cancellation, provider process host-read/write/network/env denial; frozen approval rejection, unknown has one external ledger entry and no retry |
| Configured scope isolation | Optional MCP/Skills/workspace configuration at CLI startup | `config.Load/overlaps` | Root, shared-provider and domain-data overlap rejection; symlink alias resolved even before a new data directory exists |
| GC-11 structured quote fragment | MCP fixture lookup, then ordinary Artifact save/open | stdio provider data, `builtin__artifact__save`, ArtifactStore | Three actual quote records; oral cashback remains conditional (5038.49 versus 5738.49); saved report bytes/hash reopened through CLI |
| Progressive Skills | `builtin__skill__list/view` in Main or Root | `skill.Definitions`, `skill.SetHash`, Run capability freeze | Summary excludes body; view returns actual text or binary Artifact; exact asset bytes reopened through CLI; reference/scope/version hashes and drift rejection; traversal/link denial; ordinary Worker management NOT_FOUND |
| Coding edit/test | `workspace.mode: edit`, normal Main/Task/Root calls | `workspace.Definitions`, common Executor/Security, `computer/sandbox.Command` | Real red missing-status test, exact approved Worker edit, real green race test; only CLI file changes; dirty sentinel/index/hooks preserved; protected edit rejection and old preimage denial |
| Coding audit and Git | `workspace.mode: audit`, read/search/git | `workspace.read/git`, registry selection | Audit returns actual retry file/symbol evidence; full tree hashes unchanged; hidden edit and Task control NOT_FOUND; Git filters/textconv/fsmonitor never execute |
| Coding crash boundary | SIGKILL before result commit after rename; `chat work` restart | Existing ToolStore finish boundary and never-replay policy | One edit, one attempt, persisted unknown; no repeat write or claimed success |
| Interim Computer isolation | Fixed test command and dedicated MCP executable, Host-only scopes | Seatbelt on macOS, bubblewrap on Linux | Go tests attempt host-secret read/write and network access; source checkout unchanged by test mutation; missing boundary fails closed; VM/Guest production adapter remains M6 |
