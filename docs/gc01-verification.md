# GC-01 health verification

The maintained entry is `scripts/verify_gc01.py`. It proves deterministic health
journey mechanics through the application. It cannot grade clinical reasoning,
recommendations, language understanding or current Live Shadow compatibility.
`semantic_score` stays `null` and every receipt is ineligible for semantic grading.
Remaining product/model/L2 acceptance belongs to the parent release tracker
[ERII-223](https://multica.ai/erii-studio/issues/01a16936-335f-72a4-a7f4-618823a87151).

```sh
make build
python3 scripts/verify_gc01.py --binary bin/erii --evidence evidence/gc01.json
python3 scripts/verify_gc01.py --binary bin/erii --evidence evidence/gc01.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc01.json --evidence evidence/gc01-preview.json
python3 -m unittest discover -s scripts -p 'test_gc01.py'
go test -race ./internal/llm/fake
```

The command always runs all six locked wording/noise/order variants, with five
serial repetitions of each. `make verify VERIFY_SHARD=communication` runs this
same gate. Two independent variants overlap by default; `--variant-jobs 1|2|4`
changes only that bound. Each variant retains five serial repetitions, isolated
roots/peers and owned process groups. No option lowers either count.

The communication shard honors the existing `ERII_VERIFY_GATE_JOBS=2` pool.
GC-01 starts alongside the independent GC-12 series; every gate keeps its full
inventory, serial repetitions, separate evidence and cleanup. The ten-minute CI
job limit and fixture deadlines remain unchanged. See [CI operations](ci.md).

| Boundary | Observed evidence |
| --- | --- |
| Launch / Doctor | Disposable roots, explicit probe/select, the existing MCP and embedding Fakes, restricted QuickJS; no credentials or default provider |
| Original morning | Unmodified `m1-v2` inputs, actual TUI, three Main replies, bounded questions, no Task/Child/Automation, no external write, no invented vitals/diagnosis |
| Safety replacement | Original `m6-v1` Turn 2 replacement in a separate restored root; scripted immediate emergency assessment, no coffee/running recommendation; no ordinary denial submitted first |
| Seven days | Separately locked synthetic continuation on Oct16–21 after the original Oct15 morning: medication, water, food, tinnitus, exercise, correction and requested weekly summary |
| Health source | Actual MCP call through ToolExecutor; only original age/sex/diagnoses/red flags, calendar and weather, checked against native ToolInvocation rows |
| Memory / recovery | Explicit medication remember, SIGKILL after memory domain commit, one recovered receipt/record; encrypted Recall, actual supersede, new content in subsequent search/model requests |
| Summary | Deterministic source-processing cell through existing Code Mode; one published JSON Artifact reopened through the CLI, exact dates/events/current medication/hash/size, no diagnosis or causal assertion |
| Forget / unrelated work | Freeze a health-dependent request, SIGKILL, category forget, new Main input; old Run cancelled with `memory_forgotten`, one fresh delivery, corrected/original medication absent from fresh requests and later recall |
| Replay | SIGKILL after morning model completion; completed generation/Tool rows retained; duplicate ingress/work leaves complete native state equal after draining Memory maintenance |
| Evidence / cleanup | Exact source SHA and file hashes before/after, binary SHA/build metadata, environment versions, native snapshots/requests/Conversation/Tool/Approval/Artifact/Outbox/Memory, joined process groups/listener and removed roots |

The morning Fake and continuation Fake use explicitly probed profiles for **new**
Main Runs. Completed Runs retain their original snapshots. This is mechanical
fixture composition, never evidence that an unscripted model produced the
summary or recognized the red flag. No Task, Root, Child, external effect or
approval is requested by this journey. Their empty inventories are verified;
an unknown external outcome is not synthesized as a GC-01 result.

The original Golden text, `m1-v2`, `m6-v1`, scoring and protection counts remain
unchanged. `gc01-week-v1.lock.json` retains the frozen manifest identity and all
25 GC-01 source-block hashes. The continuation adds synthetic journey data;
it does not replace or narrow the original scenario.

| Frozen source lines in `evals/golden-cases.md` | Proof and limits |
| --- | --- |
| 159, 169 | Original morning plus seven-day source/Memory/Artifact journey; real connector and model quality unexecuted |
| 161, 163, 493, 495, 504, 506, 507 | Original TUI/CLI Main assertions, zero Task/Automation/external writes; helpfulness and medical explanation remain ungraded |
| 164, 497, 508 | Locked input order, relevant health fields, correction and exact Artifact chronology; clinical correctness ungraded |
| 165, 511 | Separate three-wording red-flag replacement, five repetitions per variant; scripted recognition only |
| 166 | Native supersede/forget, frozen-generation cancellation, fresh unrelated context and deleted-recall checks; separate users/Tasks are not introduced here |
| 167 | CLI-reopened seven-day summary with explicit uncertainty and empty causal claims; actual model synthesis unexecuted |
| 472, 474, 491, 494, 499, 500 | Original fixed source/context/events retained with locks and provider-request readback |
| 501, 502, 509 | Original ordinary/non-emergency inputs, no unsolicited Task/Automation, encrypted low-confidence medication Recall; inferred diagnosis quality remains ungraded |

`validate_report` recomputes all row assertions and requires all 30 ordered
receipts, current source/binary identity, locks, cleanup and null grades. Its
negative probes corrupt actual observed receipts: missing safety, changed input,
missing request context, model/delivery disagreement, duplicate Tool attempt, unsafe reply, extra health field, forged Tool ID, stale
medication, false restart, revived owner, deleted-data leak, bad Artifact hash,
rehashed causal claim and replay drift. A stored `true` assertion cannot override
the recomputed native evidence. This is consistency checking; an author receipt
does not replace independent exact-SHA review and rerunning the harness.

The existing unscripted producer accepts `--case GC-01 --gc01-journey week` or
`--case GC-01 --gc01-journey safety` when **freezing** a plan. The ordinary mode
exports the original three turns, six continuation events, forget and unrelated
input. Safety substitutes Turn 2 and stops there. Only dates/user data are sent;
no scripted plans, expected answers or assertions enter the model. Both retain
the producer's explicit-profile, budget, no-auto-approval and fail-closed drift
checks. `verify_unscripted_cases.py` drives both through the production Pi Bridge
using its maintained loopback peer. They remain ineligible input observations:
the producer's health connector and durable-memory adapter are still missing.
See [unscripted operations](guides/unscripted-evals.md) for profile/root preparation.

Keep complete JSON receipts and failure logs in the durable evidence root. Publish
only their small hash manifest. A failed drive retains a `.failure.json` partial
receipt (`*-VARIANT.failure.json`) and an inconclusive main report; it never preserves a previous pass.
