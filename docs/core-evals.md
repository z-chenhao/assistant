# Core MVP deterministic gate

ERII-21 extends the existing Case Runner and verification inventory. Run:

```sh
npm ci --prefix provider-bridge --ignore-scripts
npm ci --prefix runtime/javascript --ignore-scripts
make test
make test-race
make lint
make verify
```

Linux needs bubblewrap and working unprivileged user namespaces. macOS uses
Seatbelt. There is no unsandboxed coding fallback. Node is pinned in CI to
24.21.0. No online account, production data or model credentials are needed.
For one maintained reproduction use:

```sh
python3 scripts/run_core_cases.py --binary bin/personal-agent --evidence evidence/core-probe.json --case GC-17
python3 scripts/run_core_cases.py --binary bin/personal-agent --evidence evidence/core-probe.json --case GC-19
python3 scripts/run_core_cases.py --binary bin/personal-agent --evidence evidence/core-probe.json --case GC-20
```

Omit `--case` to run all variants and SC contracts. Five repetitions are serial
inside `make verify`'s codemode shard. Existing security, approval, delivery,
provider and cancellation gates retain all previous repetitions. CI installs
bubblewrap in the codemode shard as well as the providers shard. The Code Mode shard
job deadline is 60 minutes to accommodate 90 isolated case executions; the other
verification shards retain 30 minutes. Ubuntu measured 491–517 seconds per full
18-variant Core run; the former 45-minute job ended during the fifth run after
four passing runs. No repetition or assertion is removed. Individual
Tool, process and assertion deadlines are unchanged. The full Go race package
budget is explicitly 15 minutes (within the existing 20-minute CI job): Ubuntu
hit the former implicit 10-minute package alarm while the current test was only
one second old, in fresh SQLite migration parsing. No data race was reported;
this is a cumulative suite budget, not an extension of a test or lease deadline.

## Exact fixtures and observed boundaries

`evals/fixtures/core-v1.lock.json` freezes the policy, generator, CSV, hidden
row-level answers, five attachments and coding repositories. Each execution
records its full file lock, variant digest, seed, fixture clock, observed times,
binary digest, model/capability snapshots, Run tree, Task, Tool/Approval/Outbox,
Code Ledger/Store/events, provider dispatch ledger, effect ledger, and Artifact
bytes/hash. Clocks in scenario context are frozen; leases use real UTC time.

GC-17 uses seed1701 and exact category/severity counts, duplicate pairs and edge
rows. Gold labels never enter the Child task. The scripted classifier reads text;
its output is compared with the separate Gold table. All six input Artifacts are
saved through ToolExecutor. Each Child gets explicit accessible references and
its own row. `feedback_analysis.csv` preserves all 100 IDs, includes source row
references and attachment references, and is reopened through the Artifact CLI.
This is deterministic classification evidence, not evidence of model quality.

The crash checkpoint waits for 37 completed Children and eight Children actually
waiting on explicit Fake effect approvals, rather than eight merely queued
admissions. Both the exact owned Sidecar and Go PID receive SIGKILL. The eight
approvals are boundary fixture controls, not feedback business requirements.
After restart the 45 accepted identities remain stable and only remaining work
runs. An independent provider dispatch ledger proves the completed 37 calls ran
once. F055 is admitted after restart; the explicit `core` test runtime forwards
one altered request under its existing **Runtime-generated** key, records
`NON_DETERMINISTIC_REPLAY`, then forwards original parameters. No model or script
creates a key. The source, snapshots and production Runtime remain unchanged.
The injector is enabled only by `-test-bridge core`.

GC-19 uses the maintained isolated coding harness. The audit reads AGENTS.md,
searches actual source, records symbols/order/trigger/impact, and compares every
workspace/Git byte. The edit adds the existing service's status flag and a
regression test, runs the real sandbox `go test -race ./...`, and verifies exactly
the CLI and its test changed. The dirty sentinel, Git index and hooks survive.
All wording/noise/order context is sent through the real Conversation ingress.

GC-20 is a controlled Go/TypeScript repository. Its legacy JSON state migration
is intentionally a small fixture, not an additional product database or a
replacement for Erii's SQLite migration tests. Up/down, repeated upgrade, empty
state and malformed-state preservation are exercised by real fixture tests.
The Root applies the migration implementation; Go is killed after that Tool
result commits while the scheduler implementation is still incomplete. Recovery reuses that
completed edit. An independent Child reads current cancellation source and
reports its nonrecursive defect. Only Root edits the shared service. The final
sandbox tests cover migration, Root/Child admission, old epoch rejection and
recursive cancellation. Node executes the TypeScript fixture contract tests;
the actual Sidecar protocol/resource/cancel/replay tests remain in `make test`.
A separate real Erii Root/Child/grandchild tree, in its own isolated database
and Agent, is cancelled through Main's
Task tool and read back after restart. The final engineering database is read
again and must still contain exactly one Task. No Child messages the user.

## Scores and scope

`core-cases-1.json` through `core-cases-5.json` contain per-variant assertions.
`core-summary.json` recomputes `hard_assertions.pass@1` and
`hard_assertions.pass^5` from all 90 results. It rejects missing/duplicate variants,
changed fixtures, binaries or assertion inventories, skipped conformance, and
incomplete cleanup.
A reported score or a passing string cannot override a false hard assertion.

O/F/P/A/T/S/R/C semantic grades and product pass metrics remain `null` when no
constrained Judge ran, matching `evals/scoring-v1.json`. Programmatic evidence
is inspectable by dimension via the requirement map below; it is not assigned
invented 100-point product scores. Suite mean/min are also null. No live model,
L2 observation, real Memory maintenance or real DesktopLease is claimed.
SC-08/11 execute private deterministic boundary Fakes and explicitly record the
real adapters as due M6. Full future case portions remain `not_applicable`.
These reports cannot by themselves establish the full product release gate.

## Core requirement-to-test and PR map

Paths below are reports relative to `evidence/`; repeated reports use `-1`…`-5`.
Each named check must actually execute; missing/skipped Go tests fail the gate.

| Requirement / due fragment | Authoritative check / evidence | Implementation PR |
| --- | --- | --- |
| GC-09 Main agenda / Core steering | `cases.json` GC-09; `tasks.json`; `task-control-*.json` | ERII-10 #9, ERII-11 #10, ERII-12 #11 |
| GC-11 quote transport / conditional price / Artifact | `capabilities-*.json` `GC-11_structured_quote_and_Artifact_fragment` | ERII-16 #15 |
| GC-12 frozen recipient/action, steering invalidation, duplicate callback | `tools-*.json` approve/steering; `TestToolApprovalWrongPrincipalHashDuplicateAndFrozenAdmission` | ERII-13 #12 |
| GC-13 approved transaction / unknown lookup without second submit | `tools-*.json` ambiguous/unknown/crash; `codemode-*.json` nested-effect SIGKILL | ERII-13 #12, ERII-19 #18 |
| GC-15 immutable images / scoped Artifact publication | `artifacts-*.json` `GC-15_Artifact_fragment`; OCR remains due M6 | ERII-14 #13 |
| GC-16 Child isolation/aggregation fragment | `children-*.json`, `scheduling-*.json`; research quality remains ungraded | ERII-17 #16, ERII-18 #17 |
| GC-17 exact seed1701 data / 37+8 / F055 drift | `core-cases-*.json` GC-17, `code-recovery-*.json` | ERII-20 #20, ERII-21 (this PR) |
| GC-18 multiple Tasks / append / aggregation fragment | `task-cases.json` GC-18, `tasks.json`, `delivery-*.json` | ERII-11 #10, ERII-15 #14 |
| GC-19 audit / dirty workspace / CLI regression | `core-cases-*.json` GC-19; `capabilities-*.json` | ERII-16 #15, ERII-21 (this PR) |
| GC-20 migration / independent review / recursive cancel repair | `core-cases-*.json` GC-20 | ERII-21 (this PR) |
| SC-01/02 roles / admission / narrowed capabilities / parent-only results | `core_conformance.TESTS`, `TestChild*` in each core report | ERII-17 #16 |
| SC-03/05 frozen replay / fencing / cancellation | `TestCodeReplayFrozenRequestAndAtomicAdmission`, `TestCodeManifestCannotChangeDuringRecovery`, `TestCodeClaimWakeAndOldCallbackFencing`, `TestTaskControlCancelTreeAndInvocationOutcomes` | ERII-12 #11, ERII-20 #20 |
| SC-04 controlled pause / new Root | `TestTaskControlPauseFencesLateWritesAndContinueNewRoot`, `TestTaskControlAppendWaitsOutsideTransactionForPauseBoundary` | ERII-12 #11 |
| SC-06/07 frozen approval / rejection / retry / unknown | `TestToolApproval*`, `TestToolReplayPoliciesAndEvidence`, `TestToolTimeoutAndBusinessErrorsDoNotRetry`, `TestToolUnknownReconcilesOnlyFromAuthoritativeEvidence` | ERII-13 #12 |
| SC-08 Memory boundary Fake | `conformance.boundary_fakes.SC-08`: supersede, deletion, owner scope, restraint; production adapter **not_applicable** | ERII-21 (this PR); real implementation M6 |
| SC-09 Artifact scope/hash/expiry/traversal | `TestArtifactStoreContractScopeIntegrityAndTombstone`, `TestArtifactTamperSymlinkAndTraversal` | ERII-14 #13 |
| SC-10 fair work / progress / steering | `TestSchedulerFairSecondTaskAndDurableTicket`, `TestSchedulerActivityCountsDoNotNotify`; Main ordering tests remain in full gates | ERII-11 #10, ERII-18 #17 |
| SC-11 DesktopLease boundary Fake | `conformance.boundary_fakes.SC-11`: local blocking, independent desktop, lease loss rejects old action; production adapter **not_applicable** | ERII-21 (this PR); real implementation M6 |
| SC-12 frozen model/capability / idempotent Delivery | `TestChildFrozenModelAndCapabilitiesAfterProfileSwitch`, `TestDeliveryClaimFencesLateAckAndQueriesAcceptedSend`; `delivery-*.json`, `events.json` | ERII-15 #14, ERII-17 #16 |
| SC-13 monetary authorization (existing additional gate) | unchanged `spend-*.json`, `TestSpend*` five-run gates | ERII-18 #17 |
| Empty/prior SQLite schema, CAS and rollback | `make test-race`, five-run `models.jsonl`; v1–v17 migration tests | ERII-1…20 |
| QuickJS protocol/resources/unawaited/cancel | `make test`, `codemode-*.json`, `code-recovery-*.json` | ERII-19 #18, ERII-20 #20 |

## Recovery operations

Use the same configured binary/runtime and data root after an unexpected exit.
Keep the frozen source, Prelude, runtime dependency lock and snapshots intact.
Run `status`, then `chat work`; recovery scans authoritative state rather than
trusting delivery of a wake hint. Pending approvals remain attached to their
exact frozen action. Use `approval list`; approve/deny with its stored hash.
Do not authorize a replacement action using an older approval.

Never rewrite Code Ledger rows, clear claims by hand, change source to resume an
existing execution, or reissue an `unknown` side effect. Inspect the external
system's authoritative receipt using `approval reconcile` where supported.
Unresolved uncertainty stays explicit for user disposition. A user continuation
uses Task append and a new Root rather than reviving a terminal Run.

The harness owns only its temporary directories and exact spawned processes.
It reaps children and removes disposable state on both success and failure.
Evidence reports are retained outside that state; fixture Artifacts are copied
into reports before cleanup. `core-summary.json` and each `suite-*.json` record
success only after required checks complete. CI retains exact-SHA evidence for
14 days; it is not a production backup.
