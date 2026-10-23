# v1 release audit — ERII-30

Release status: **inconclusive**. The v1 completion contract is not satisfied.
ERII-4 must remain `in_progress`; ERII-30 must not be accepted as a completed
release. This document and its ledger are an initial source/evidence inventory,
not a claim that every normative requirement has passed.

The current synchronized base is `68150f1ec567c1babae7d03b3c6ef91d301966bc` (human-merged #33); ERII-29 #31 remains the prior release dependency.
ERII-5–29 each have a merged implementation PR. The ledger records GitHub's
actual merge commit, actor, time and URL, queried on 2026-10-21. A merge records
integration, not semantic quality, exact-SHA review or final human acceptance.

## Reproduce and inspect

```sh
python3 scripts/release_audit.py --evidence evidence/release-audit.json
make release-audit
```

The first command validates the source digests and code/test/document/PR
references, then emits an inspectable inventory. `make release-audit` emits the
same report and deliberately exits 2 because completion is inconclusive.
Exit 0 from the inventory command means only that indexing succeeded.

[`v1-release-ledger.json`](v1-release-ledger.json) routes all baseline §§0–19
and Golden §§1–16 to candidate implementation files, tests, merged PRs and
maintained evidence producers. The report preserves each paragraph, list item,
table row and fenced contract with its original section, line and SHA-256.
It includes descriptive/future/reference text so no automated keyword filter
can silently discard a mandatory clause. Thirty-one blocks now have exact-hash
individual dispositions (`partial_mechanical` or `evidence_gap`); the other
blocks remain `pending_individual_review`. Some blocks contain several obligations;
each review must check every subclause, distinguish v1 requirements from explicit
deferrals, and link the exact executed assertion before closing it. Shared
section mappings are navigation aids, not per-clause conformance proof.

The inventory does not consume or certify test reports, invoke a model, infer
semantic grades, accept historical L2 as current, or modify product data.
Changing a normative source requires deliberate re-audit and digest update.
A missing file/symbol/section/PR reference fails the inventory; a failed rerun
removes its previous report. Unit tests exercise these rejection paths and
the nonzero completion boundary through the actual CLI.

## Confirmed coverage gaps

| Gap | Inspectable current state | Required next evidence |
| --- | --- | --- |
| GC-02 semantic quality | The existing six-variant driver now processes all four turns in order and creates one monthly Automation through the Main Tool; it asserts no early Task/schedule, first fire 2026-11-29 20:00 +08 and duplicate-input idempotency | Unscripted interview/constraint selection and semantic grades remain unverified; scripted scheduling evidence is not a product Case score |
| GC-08 | `verify_channels.py` uses earlier Task continuation; exact wedding checklist is not driven | C1→C2 dependency, 128 guests, unknown tables, hotel draft/frozen recipient/body/attachment; C3 stays lower priority |
| GC-09 | Six exact §8.10 CLI/TUI agenda variants now retain full source/request/state/replay and enforce every scripted agenda assertion | Broader §7 multi-connector/Worker question aggregation and Steering, actual model quality and constrained grades remain required |
| GC-11 | Six locked variants now drive both original turns, three exact quotes/services/weights, actual MCP reads, user-visible scripted responses and two CLI-opened Artifacts; duplicate input creates no work | Unscripted recommendation, source selection and semantic calibration remain unverified; Fake output is not a product score |
| GC-12 | `verify_gc12.py` drives six complete mechanical journeys plus denial/lost-response, five serial rounds, with independent mail/calendar readback | Actual model evaluation, real approved account actions, complete protected acceptance and constrained grades remain required |
| GC-14 | `verify_automation.py` and channel checks drive exact threshold days | Complete case variants, input/stop/currency evidence and semantic evaluation; only Oct 18/21 notify |
| GC-16 | Child admission/isolation/aggregation fragments exist | D1–D5 research journey; earliest primary source Atlas 2024-03-01, rename vs extension, Aster excluded, independent report |
| All 20 semantic gates | `case_runner.py`, `core_report.py`, `m6_report.py` correctly leave semantic/product metrics null for scripted Fakes | Constrained Judge O/F/P/A/T/S/R/C, all cases ≥80, suite mean ≥85; pass@1/pass^5, variance and hard-failure priority |
| Local/account comparison | Provider contracts use deterministic HTTP peers | Unscripted model runs with identical fixture/Context Builder/Capability/tools/limits and separately reported snapshots, quality, latency and resources |
| Current L2 | `computer-provider-l2-result.json` and `computer-l2-result.json` are dated historical observations | Current real login, VM/CUA/Jianying/channel observations with time, model, failure class, inspectable artifacts and human review |
| Web build dependency verification | A CLI-scoped watcher 2.6.0 override removes the braces/micromatch chain; clean install and full npm audit report zero findings, and the embedded-asset check produces identical bytes | Keep native build/UI verification; remove the override when a stable Tailwind CLI ships a safe watcher dependency and the same gates pass |

GC-01/03–07/10/13/15/17–20 have stronger mechanical journeys, but that does
not establish all normative assertions or product scores. At M7, missing
whole-case portions are release gaps; an older `not_applicable` label cannot
waive them. No scoring policy, fixture, hard assertion or baseline is relaxed.

The original four npm high entries came from one Tailwind CLI build chain.
[GHSA-vfj7-8cjw-p6xm](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm)
affects braces through 3.0.3 with no patched release. Tailwind CLI 4.3.3 pins
watcher 2.5.1; a normal transitive update cannot remove that chain. The scoped
`@tailwindcss/cli` override selects published watcher 2.6.0, which uses picomatch
instead of micromatch/braces. Upstream has already adopted 2.6.0 in
[PR 20381](https://github.com/tailwindlabs/tailwindcss/pull/20381).
Only the build dependency graph/lock changes; the pinned Tailwind compiler and
runtime dependencies remain unchanged. `npm run check` verifies identical
embedded JavaScript/CSS/licenses. Watcher and picomatch retain MIT licensing;
detect-libc 2.x's Apache-2.0 license was already present through Lightning CSS.
No canary, forced downgrade or new production dependency is introduced.

Reproduce the dependency checks with:

```sh
npm ci --prefix web --ignore-scripts
npm audit --prefix web --json
npm run check --prefix web
```

The audit is a dated registry observation; offline CI keeps deterministic
installation, generated-asset freshness and real Web UI gates. A clean audit
does not establish complete application security or current L2 acceptance.

## Existing system and recovery evidence

GC-02 now reaches the same M7 Automation boundary within its existing locked
wording/noise/order journeys, rather than deferring Turn 4. Its Automation clock
uses the fixed Case time; Host leases still use wall time. The driver retains
all four original user events, Memory supersede/deletion, an independent CLI
schedule readback and before/after/replay state. The five-round M6 aggregate
requires all six monthly assertions; omitting one in every round fails closed.
No fixture, scoring policy or production protocol changed. Remaining M7
journeys are unverified rather than treated as release-time not applicable.

The next exact-head run, 39575536939 on `980dc12`, passed all functional shards
but exceeded the race job's ten-minute cap. Its static SQLite partitions had
measured costs near 330/379/449 seconds; the last partition was incomplete and
the disposable-storage cleanup guard failed. Neither race nor the aggregate
was green. `ci_race.py` now balances placement using retained Ubuntu root
durations, while discovery/verdicts still require every root exactly once.
The hints cannot omit new tests or resurrect removed tests. Bridge and the
timing-sensitive Child approval test stay isolated; CPU admission, race flags,
all deadlines and cleanup checks are unchanged. See [CI operations](ci.md)
for reproduction and evidence. A balanced estimate alone is not a CI PASS.

Exact-head CI run 39572139854 exposed an intermittent pause/restart failure:
`task-control` attempt 5 exited with `MODEL_INCOMPATIBLE` before
`before_stop_settle_commit`. Deterministic regression tests reproduced a
crash/storage-failure window after `context_compaction` commits but before
`model_generation` admission. The old steering guard checked only generation
records; it could append new input to an already frozen compaction step.

ERII-30 repairs both Main steering and Root Task append by deferring input when
either record freezes the current step. Recovery uses the original context;
pending input is applied at the next step before final delivery. The two named
regressions in the feature map assert preserved context, ordered input, one
final reply/Root result and no duplicate adoption. The maintained CLI harness
adds actual SIGKILL at `after_compaction_commit`; SC-04 includes both tests in
its five-run conformance inventory. This preserves existing frozen-context,
snapshot and replay semantics; no migration or protocol deviation is introduced.

`scripts/core_conformance.py:TESTS` maps SC-01–12 to named Go tests and rejects
missing/skipped results. `verify_spend.py` supplies the additional SC-13 gate.
`verify_suite.py:SHARDS` is the authoritative offline inventory and preserves
five serial repetitions. `core_report.py` and `m6_report.py` validate the exact
variant/assertion/fixture/binary inventory and cleanup. These are mechanical
gates, independent of semantic grading and L2 review.

| Obligation | Maintained evidence producer | Proof boundary |
| --- | --- | --- |
| Fixed-source replay, 37 completed + 8 in flight, F055 drift | `verify_code_recovery.py`, `run_core_cases.py` | Actual Sidecar/Host kills, persisted ledger and call counts; inspect all five reports |
| Pause/cancel, late epochs, lost wake, recursive grandchildren | `verify_task_control.py`, Core conformance | Stored terminal/epoch/tree state and race tests |
| Frozen approval, deny, transient retry, unknown never resubmitted | `verify_tools.py`, `verify_codemode.py`, booking journey | External sandbox ledger + ToolInvocation; no exactly-once claim |
| Memory correction/Forget, successor Run and dispatch fencing | `verify_memory_maintenance.py`, `verify_recall.py` | Real encrypted/scoped stores, provenance and pending-generation rejection |
| Backup/upgrade/capacity and production close | `verify_operations.py`, `verify_capacity.py`, event integration | Cold authenticated restore with hash readback, wrong-key/tamper rejection, fenced cutover, v25 upgrade and 1000-Child cycles |
| Real VM recovery | `lume_live.py`, `verify_computer.py`, `verify_computer_use.py` | Offline peers are not real macOS; historical L2 is identified separately |

Use [operations.md](guides/operations.md) for startup/close, poison handling, cold
backup, independent credential/VM retention, isolated restore, cutover and
upgrade. A backup excludes SecretStore and VM storage. Retire the source and
reconcile external outcomes before activation. Never reissue an `unknown`
effect or revive a terminal Run. Approved deviations are ADR 001 (current
account auth/deferred device flow), ADR 002 (explicit monetary authorization)
and ADR 003 (Forget revokes unfinished work); no new deviation is approved here.

## GC-11 checkpoint

```sh
make build
python3 scripts/verify_capabilities.py --binary bin/erii --scenario quotes --scenario-jobs 2 --evidence evidence/gc11.json
python3 scripts/prepare_release_eval.py --input evidence/gc11.json --evidence evidence/gc11-eval-preparation.json
python3 -m unittest discover -s scripts -p 'test_*.py'
python3 scripts/release_audit.py --evidence evidence/release-audit.json
```

The `gc11-v1` lock binds source, policy and exact fixture. The existing five-round
capabilities CI gate now includes every GC-11 variant, preserving the coding,
Skill/MCP and SIGKILL scenarios. Each quote variant owns isolated configuration,
databases and provider state. Doctor activates only the explicit Fake. Drive
preserves two ordered original events with separate inspectable scripted plans;
the real Main, Tool executor and stdio MCP run through the CLI. Independent
readback asserts six exact read/save/echo calls, original three-quote source,
two visible replies/deliveries, no Task/approval/purchase effect and unchanged
state after a repeated second input. Both Artifacts are opened through the CLI
and their bytes/hashes retained before joined cleanup removes the temporary root.
The records include Run tree, snapshots, ToolInvocation, Approval, Outbox and
Artifact state. `test_quote_case.py` rejects missing turns, services, uncertainty,
source transport, response bytes, deliveries, unauthorized effects and replay drift.

The first eleven GC-11 individual reviews bind exact source blocks to code,
regressions, this evidence command and PR #32's **unmerged; independent review required** disposition.
Each lists observed mechanics and all remaining semantic or broader obligations;
for example, no purchase during comparison cannot prove purchase approval, and
the car-insurance weights do not prove device/budget personalization in other
purchases. The audit validates block hashes, references, duplicates, required
remaining-work notes and dispositions. Reviewed blocks are not individually
verified requirements: the verified count stays zero, release_ready stays false
and scores stay null. Source or reference drift fails closed.

The first checkpoint CI on `6a837f4` passed all functional rounds, including
the new providers/GC-11 gate, but the delivery job exceeded ten minutes and
Ubuntu checks failed. Retained `suite-delivery.json` reports clean stable
source, passed=true and all ten rounds; its 517.6-second wall time equals the
two series' sum. The dispatch allowlist excluded delivery even though CI
requested two workers. The bounded executor now honors that setting for these
independent series, preserving every round and timeout. A real-subprocess
mutual-admission regression fails on the old code. The failed receipt remains
evidence; only successful required checks on the later exact head satisfy CI.

[Evaluation preparation and current live preflight](v1-live-preflight.md) provides
the executable packet/condition preparation and exact existing VM/channel
commands. Judge input is allowlisted and excludes raw plans/traces; scripted
previews are ineligible for grading. No model comparison or current L2 run is
claimed. The [unscripted GC-11 driver](gc11-evaluation.md) now supplies production
CLI/Bridge trial mechanics and a strict sixty-receipt frozen-condition preflight.
The ordinary local-provider gate runs its real HTTP/CLI/MCP/Artifact contract.
It sends only original events and fixture context, retains profile/Capability,
state/usage and source/replay witnesses, checks actual generated Artifacts and
rejects reused installations. Fake HTTP observations remain semantically ungraded.
Eight driver regressions cover original prompts, ownership/configuration,
independent Run inventory, all cross-variant conditions, failed/dirty/drifted
receipts, actual CLI stale-output removal and termination of an owned child
session. Judge preparation accepts these
receipts through the same allowlist and keeps offline previews ineligible.

Three additional source-block reviews cover baseline frozen ModelSnapshot,
Golden fixed configuration/snapshots and identical-condition model comparison.
Their dispositions remain partial/evidence_gap: two Main Runs do not prove
Root/Child inheritance or active-run profile switching, and preflight mechanics
do not establish actual model sampling, semantic grades, GPU resources or a
completed live comparison. Those fourteen records were the GC-11 checkpoint inventory. After GC-12 and
GC-09, the ledger has 31 partial/evidence-gap dispositions, 1,552 pending blocks
and zero certified requirements. The next exact whole-case candidate is GC-08;
its dependency and draft-approval journey still needs maintained verification.

## Handoff and remaining work

The user's 2026-10-21 coordination authorizes sequential bounded checkpoints
inside this existing child and sole PR #32. No new child or competing PR is
needed. ERII-30 remains incomplete until
those results and individual normative reviews are available. The user's later
2026-10-21 review instruction authorizes independent review of the implemented
changes and evidence while these required release results remain missing.
Ready/In Review means reviewable, not accepted: the Verifier must assess the
original contract and report BLOCKED for missing required evidence. A prior
Verifier PASS is not a prerequisite to requesting review. No reviewer PASS,
CI result or human acceptance is fabricated.

Retain exact commands, exit codes, source/binary hashes, complete reports and
cleanup results with the issue. The inventory never labels an unrun command
passed. `make test`, `make test-race`, `make lint` and `make verify` are distinct
checks. CI green is not full v1 acceptance. The final milestone/child/PR and
recovery report must be refreshed on ERII-4 after all gaps close; parent
`in_review` is permitted only then and `done` requires human acceptance.
