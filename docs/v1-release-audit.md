# v1 release audit — ERII-30

Release status: **inconclusive**. The v1 completion contract is not satisfied.
ERII-4 must remain `in_progress`; ERII-30 must not be accepted as a completed
release. This document and its ledger are an initial source/evidence inventory,
not a claim that every normative requirement has passed.

The inspected base is `ae2dda0a420d24ffbf45553ee10dec67d2b05ead` (ERII-29 #31).
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
can silently discard a mandatory clause. Each block remains
`pending_individual_review`. Some blocks contain several obligations; a future
review must check every subclause, distinguish v1 requirements from explicit
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
| GC-02 Turn 4 | `run_m6_cases.py` explicitly returns `automation: not_applicable; M7`; `verify_automation.py` tests the monthly date separately | One complete personal journey through the actual Automation tool, with no duplicate schedule and first fire 2026-11-29 20:00 +08:00 |
| GC-08 | `verify_channels.py` uses earlier Task continuation; exact wedding checklist is not driven | C1→C2 dependency, 128 guests, unknown tables, hotel draft/frozen recipient/body/attachment; C3 stays lower priority |
| GC-09 | Main agenda and Web transport fragments exist | Exact full M7 case record, required variants, notification/Steering and semantic grades |
| GC-11 | `verify_capabilities.py` exercises the quote/Artifact fragment | Complete two-turn three-quote recommendation; conditional oral cashback, written evidence, no purchase |
| GC-12 | `verify_channels.py` explicitly labels an approval fragment; Tool tests exercise frozen hashes | Exact Alex Chen/Tuesday 11:00–11:30 journey; obsolete Monday approval invalidated; one mail AND one calendar event and completion notice |
| GC-14 | `verify_automation.py` and channel checks drive exact threshold days | Complete case variants, input/stop/currency evidence and semantic evaluation; only Oct 18/21 notify |
| GC-16 | Child admission/isolation/aggregation fragments exist | D1–D5 research journey; earliest primary source Atlas 2024-03-01, rename vs extension, Aster excluded, independent report |
| All 20 semantic gates | `case_runner.py`, `core_report.py`, `m6_report.py` correctly leave semantic/product metrics null for scripted Fakes | Constrained Judge O/F/P/A/T/S/R/C, all cases ≥80, suite mean ≥85; pass@1/pass^5, variance and hard-failure priority |
| Local/account comparison | Provider contracts use deterministic HTTP peers | Unscripted model runs with identical fixture/Context Builder/Capability/tools/limits and separately reported snapshots, quality, latency and resources |
| Current L2 | `computer-provider-l2-result.json` and `computer-l2-result.json` are dated historical observations | Current real login, VM/CUA/Jianying/channel observations with time, model, failure class, inspectable artifacts and human review |
| Dependency audit | 2026-10-21 `npm audit --prefix internal/interaction/web/client --json`: four high-severity entries in one build dependency chain | Assess and repair braces → micromatch → @parcel/watcher → @tailwindcss/cli; no force upgrade in this audit |

GC-01/03–07/10/13/15/17–20 have stronger mechanical journeys, but that does
not establish all normative assertions or product scores. At M7, missing
whole-case portions are release gaps; an older `not_applicable` label cannot
waive them. No scoring policy, fixture, hard assertion or baseline is relaxed.

The dependency finding concerns the Tailwind CLI build chain, not proof of a
runtime exploit in the embedded application. The registry report identifies
`GHSA-vfj7-8cjw-p6xm` in braces. Establish actual exposure and a bounded fix
before assigning a release disposition. The audit does not run a forced major
upgrade or change dependency locks.

## Existing system and recovery evidence

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

Use [operations.md](operations.md) for startup/close, poison handling, cold
backup, independent credential/VM retention, isolated restore, cutover and
upgrade. A backup excludes SecretStore and VM storage. Retire the source and
reconcile external outcomes before activation. Never reissue an `unknown`
effect or revive a terminal Run. Approved deviations are ADR 001 (current
account auth/deferred device flow), ADR 002 (explicit monetary authorization)
and ADR 003 (Forget revokes unfinished work); no new deviation is approved here.

## Handoff and remaining work

The parent has been asked to update its ledger before dispatching bounded
whole-case, semantic-comparison and current-L2 work. This run creates no new
implementation child and activates none. ERII-30 remains incomplete until
those results and individual normative reviews are available. Its draft PR
must stay draft; no reviewer PASS, CI result or human acceptance is fabricated.

Retain exact commands, exit codes, source/binary hashes, complete reports and
cleanup results with the issue. The inventory never labels an unrun command
passed. `make test`, `make test-race`, `make lint` and `make verify` are distinct
checks. CI green is not full v1 acceptance. The final milestone/child/PR and
recovery report must be refreshed on ERII-4 after all gaps close; parent
`in_review` is permitted only then and `done` requires human acceptance.
