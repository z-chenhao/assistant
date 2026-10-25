# GC-02 goals verification

`scripts/verify_gc02.py` joins the frozen GC-02 journey to the existing application
boundaries. It proves deterministic mechanics, with explicit scripted plans through
the existing capabilities Fake. Raw input uses a separately probed terminal-input
Fake profile; new Main Runs switch explicitly selected profiles, while each
Run and recurring Root retain frozen snapshots. It does not grade interview quality, understanding,
value judgments, goal recommendations or actual model/connector compatibility.
`semantic_score=null`, `eligible_for_grading=false`, `release_ready=false`.
Remaining product/model/current L2 acceptance belongs to
[ERII-223](https://multica.ai/erii-studio/issues/01a16936-335f-72a4-a7f4-618823a87151).

```sh
make build
python3 scripts/verify_gc02.py --binary bin/erii --evidence evidence/gc02.json
python3 scripts/verify_gc02.py --binary bin/erii --evidence evidence/gc02.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc02.json --evidence evidence/gc02-preview.json
python3 -m unittest discover -s scripts -p 'test_gc02.py'
```

All six locked `m6-v1` wording/noise/order variants run five consecutive rounds.
`--variant-jobs 1|2|4` only bounds independent variants; rounds remain serial.
The maintained suite runs the same complete gate in `communication`. No option
reduces counts or changes the original fixtures, scoring or protection rules.

| Boundary | Evidence and limit |
| --- | --- |
| Launch / Doctor | Disposable databases, explicit profile add/probe/select, deterministic embedding HTTP and MCP peers, restricted QuickJS; no account or implicit provider |
| Original input | All four original events and context retained in order through actual CLI and PTY TUI; scripted driver messages are separate and visible. Fake replies to natural input do not demonstrate understanding. |
| September to October continuity | Original confirmed/exploratory facts, unanswered question and initial Core/Recall content; MCP read via ToolExecutor and reopened resume Artifact. Unanswered income remains null until Turn 2. |
| Goal revision | Main Memory Tool accepts the constraint. Curator supersedes the original exploratory record with the current exploratory goal; original Conversation and encrypted history remain. No Task or Automation before the reminder request. |
| Memory restart | SIGKILL after correction commits in memory.db, before state acknowledgement; one receipt/record after restart. |
| Reminder | One Main Automation Tool call, cron `0 20 * * 0L`, `Asia/Shanghai`, first fire `2026-11-29T12:00:00Z`; duplicate ingress changes no native state. |
| Cross-month review | Nov29 and Dec27 each admit a fresh Root on one ongoing Task. Each Root searches current Memory and publishes a CLI-reopened Artifact, including current exploration and family/income constraints. A fixture support scaffold carries the conflict, dependency and next step with live source references and no priority/decision; its advice quality remains ungraded. No corrected content/Memory ID is baked into the schedule. |
| Fire/report recovery | SIGKILL after first fire commit and second report commit; completed model/Tool rows reused, one update and Main delivery per occurrence, repeat drains equal. Scenario clock advances only Automation, never Host leases. |
| Cancel | Persist Jan31 fire, cancel the schedule before admission; pending fire is skipped with no Root, report or delivery. Old ingress cannot renew it. |
| Forget protection | Supplemental probe freezes a goal-dependent generation, kills the process, forgets the current lineage, then submits unrelated input. Old owner is cancelled, fresh Main completes, later Recall/context exclude the forgotten content. |
| Delivery / security | Workers report only through Main. Native Approval and external-effect inventories remain empty; no employer/family/third-party message. No Child is required for this small review. Unknown effects and Child approval transfer are not fabricated as GC-02 evidence. |
| Identity / Cleanup | Source SHA/file hashes before and after, binary digest/build metadata, tool/environment versions, native state/requests/Memory/Artifacts/Outbox, joined process groups and removed roots; deterministic peers shut down. |

`validate_report` recomputes assertions from all 30 ordered native receipts, checks
locks/source/binary identity and cleanup, and refuses semantic grades. Negative
probes change original input, source Tool identity, unanswered answer, supersede
lineage, restart, early/duplicate schedule, zone/fire time, missing month,
Tool attempts, Artifact bytes/rehashed value ranking, cancelled fire, old owner,
deleted Recall and replay. Stored assertion flags cannot override observation.
Consistency checking does not replace independent exact-SHA review and rerunning.
Failures retain per-variant `.failure.json` plus an inconclusive main report.

The continuation lock retains manifest SHA256
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`
and exactly these 24 original blocks:

| Frozen Golden source lines | Proof / remaining semantic boundary |
| --- | --- |
| 173, 183, 515, 517, 532 | Original historical world and full input journey; real cross-month model continuity ungraded |
| 175, 177, 178, 179, 180, 181 | Exploratory version/constraint readback, unanswered resume, no ranking, real supersede, opt-in tracking; interview quality/conflict explanation remain model evaluation |
| 534, 535, 536, 537, 539 | Four unchanged event inputs plus separate scripted operations; no deterministic reply is claimed as an unscripted answer |
| 541, 542, 543, 544 | Resume Artifact, current Memory, one zoned schedule, immutable Conversation |
| 546, 548, 549, 550 | Exploration stays Recall, exact first fire, no external actions; independent development/startup never marked decided in structured readback |

For real model work, the existing `unscripted_cases.py freeze --case GC-02`
producer supplies the original six-variant user context/events through production
Pi Bridge with an operator-prepared profile. It sends no scripted plans, expected
answers or assertions. See [unscripted operations](guides/unscripted-evals.md).
Its Memory/schedule/logical-clock adapter remains incomplete: these input fragments
are ineligible for grading. This PR does not close that parent release gap.

The public `--evidence` file is a small `gc02_native_manifest`. Full native JSON
lives beside it under `.gc02-native/`, with a relative path, byte count and
SHA256 in the manifest. `--validate` and the preparation CLI require those bytes,
reject unsafe/symlink paths and hash/summary drift, then recompute every assertion.
A downloaded manifest without native bytes is inconclusive, never a passed Case.
The pinned CI uploader defaults `include-hidden-files=false`: it uploads the
small manifest, logs and source-bound suite report, and excludes raw GC-02 state.
Builder retains the complete directory and executable in the registered durable
local evidence root for same-host independent review; CI summaries alone do not
replace that evidence or a clean exact-SHA rerun.
