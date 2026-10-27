# GC-14 monitoring verification

The maintained runner drives the exact Golden §8.15 input and six daily prices
through the production CLI, Context Builder, Automation, Actor, RunClaim,
ToolInvocation, Reporter and Delivery path. The model is explicitly scripted;
the original user input contains no Tool plan or answer. Six variants preserve
the fixture with three wordings and three ordering/noise changes.

```sh
make build
python3 scripts/verify_gc14.py --binary bin/erii --evidence evidence/gc14.json
python3 scripts/prepare_release_eval.py --input evidence/gc14.json --evidence evidence/gc14-judge.json
python3 -m unittest discover -s scripts -p 'test_gc14.py'
go test -race -count=5 ./internal/llm/fake
```

`make verify` runs all six variants in the communication shard, which provisions
the existing MCP sandbox. `--variant base` is diagnostic and cannot produce a
complete Judge packet set.

Launch creates disposable configuration, databases and a read-only stdio peer.
Doctor tests/selects an explicit fixture profile and validates the source lock.
Drive sends the original natural-language request through `chat send/work`.
Main reads the independent fixture and creates one daily 08:00 Asia/Shanghai
Automation for HKG-CDG, 2026-12-01, threshold 1800 CNY. No Task or fire exists
before the first due minute; 07:59:59 remains quiet. Six due days create six
distinct Roots on one ongoing Task. Each Root reads site-a and site-b with exact
route, date, sampling day and currency, then records both same-value observations
through ordinary ToolInvocations. The peer has no network/domain DB access or
threshold/deduplication logic; every read remains in an independent log.

Only October 18 and 21 produce a Task update and one delivered notification
each. October 19 remains below the threshold without a repeated notice. Every
day repeats scheduling/draining and compares complete native state. An actual
SIGKILL after the October 18 admission commit leaves that same queued Root,
frozen snapshots and no premature source read; restart completes it once. A
second SIGKILL at the final scan boundary plus duplicate ingress preserves all
state and reads. A profile switch before October 19 proves later admissions
freeze the new profile while all earlier Model/Capability snapshots remain
unchanged. Worker Tool schemas remain constant across the switch.

Cancellation is another original user message. Main lists and cancels the
single Automation through its normal Tool. Later ticks and duplicate cancel
input create no Root, Tool read or delivery. Prior fires, Roots, observations,
notifications and Conversation remain inspectable. Evidence contains all daily
native states, profile identities, Model/Capability snapshots, Tools, empty
Approval/Artifact inventories, fire/Inbox/Reporter/Delivery records, source log,
two killed-process exit codes and replay equality. Cleanup joins children and
removes the disposable root, including on failure.

Python mutations reject schedule offset, wrong observations/source/currency,
lost/repeated notifications, missing/duplicate fires/Tools, wrong Task and
terminal-state drift. Boundary fixtures enforce strict `<1800`, continuous-low
suppression and repeated threshold reentry. Go tests require two matching
current sources and reject missing/negative/disagreeing prices, wrong route,
flight date, sample day, currency or source. This is Fake correctness evidence,
not independent model decision quality.

Judge preparation recomputes native assertions and the variant/source lock
before allowlisting fixture, visible conversation and assertions. All previews
are ineligible and scores/winner remain null. Actual websites/models, other
monitor types, full release dimension scores, protected acceptance, current L2
and named human review remain required. The release completion gate stays
nonzero.

## ERII-323 frozen receipt boundary

ERII-323 owns the 16 GC-14 blocks in replan manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.
`evals/fixtures/gc14-boundary-v1.lock.json` binds those exact lines and hashes,
the original Golden Cases, original fixture and lock, and accepted ADR-006.
Neither the original prices/variants nor scoring or protected repetitions change.

| Frozen source | Mechanical evidence |
| --- | --- |
| Golden §7.GC-14, lines 341–351 (8 blocks) | Original input, exact route/date/CNY/Asia-Shanghai schedule, threshold reentry, explicit Main cancellation, retained history, current admission profile and frozen Model/Capability plus actual generation schemas, no unrequested Tools/Approval/Artifact |
| Golden §8.15, lines 881–906 (8 blocks) | Locked six-day source, 08:00 first fire, Oct18/21 notifications only, duplicate ticks and two same-price sources, cancel with no later Root |

The same six isolated journeys retain all previous duplicate scheduling,
SIGKILL/restart, source-order/noise and cancellation probes. The existing
five-run Automation, Task-control and Delivery safety gates remain required;
the six Fake variants are not five stochastic model trials. Existing SQLite
`TestAutomation*` tests own delayed-event/CAS, concurrent admission,
schedule-edit fencing, revoked intents and migration/storage-failure boundaries.
This Case has read-only external price reads and local Conversation delivery;
there is no external purchase/send approval to simulate. The maintained Delivery
unknown-send gates remain required without claiming GC-14 exercised a live send.

```sh
python3 scripts/verify_gc14.py --binary bin/erii --evidence evidence/gc14.json --validate
PYTHONPATH=scripts python3 -m unittest test_gc14 test_unscripted_cases test_ci_communication test_prepare_release_eval
go test -race -count=5 ./internal/agent/sqlite ./internal/llm/fake -run 'Test(Automation|Monitor)'
go test -race -count=5 ./internal/automation
```

A small `gc14_native_manifest` binds a local lossless gzip receipt by size/hash.
`--validate`, Judge preparation and the communication CI collector reopen the
native receipt and recompute its assertions. They require all six variants in
order, disjoint native Run/Tool/generation/message/delivery identities, exact
working-source bytes/SHA, actual binary hash/build metadata, environment,
cleanup and null grades. Twenty-three mutations of real receipts reject corrupted
prices, day order, duplicates, late work, lost source/history, crash records,
profile/Model/Capability/generation drift and unexpected Approval/Artifact.
Manifest regressions reject altered bytes, sizes, summaries, duplicate JSON keys,
path traversal and symlinks. Hashes establish byte identity; independent review
still establishes trust in the source and observed execution.

`unscripted_cases.py` now accepts GC-14's six existing variant IDs as an
**input fragment** through the selected production profile. It supplies only
the original request and synthetic route/date/threshold/schedule context, with
future daily prices withheld. It supplies no Tool plan, answer, Fake model or
simulated approval. Live sources, advancing clock/notifications, approved
profiles, full-model semantic scores and current L2 remain due under ERII-223
and [live preflight](v1-live-preflight.md). Reports retain
`semantic_score=null`, `eligible_for_grading=false`, `release_ready=false`.
Completing ERII-323 does not certify v1 release acceptance.
