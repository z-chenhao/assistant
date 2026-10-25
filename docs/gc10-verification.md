# GC-10 local-life verification

ERII-256 owns the 16 GC-10 blocks in the approved replan manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.
`gc10-boundary-v1.lock.json` pins those exact source lines/hashes, the original
Golden Cases/scoring, and the unchanged M1/M6 fixtures. This is deterministic
mechanical evidence. It does not certify clothing judgment, live weather,
recommendation quality, a Golden Case score or release readiness.

```sh
make build
python3 scripts/verify_gc10.py --binary bin/erii --evidence evidence/gc10.json
python3 scripts/verify_gc10.py --binary bin/erii --evidence evidence/gc10.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc10.json --evidence evidence/gc10-preview.json
```

The maintained suite runs this entry as the communication/channels `gc10` gate.
The CI collector independently recomputes the native receipt before emitting its
compact manifest. It preserves all six variants and five serial repetitions per
variant. Two independent fixture workers share the existing bounded fixture pool;
`--variant-jobs 1` selects serial execution. Each variant owns its temporary root,
processes and failure file. Joined results retain canonical inventory order.
The existing CI job deadline and gate-worker limits are unchanged. GC-10 joins
GC-09 ahead of short communication gates in the existing bounded queue; this
avoids leaving a complete native series in the deadline tail. There is no
reduced success mode.

The original M6 context and event pass unchanged through CLI ingress. The
explicit `local` test Bridge reads weather through an actual sandboxed, read-only
stdio MCP Tool. The sandbox returns supplied data and logs every read; it does
not produce recommendations. Main's ToolInvocation, frozen generation requests,
Conversation and Delivery are read back from native databases. The original M1
fragment and M6 Code Mode fragment remain covered by their existing gates.

Separate locked continuations exercise location and time changes, an exact
freshness boundary, source unavailability and significant cost. They do not
replace or edit the original event. The location change enters through the real
PTY TUI. Every source includes location, sampling time and availability.

| Frozen source / applicable assertion | Native evidence |
| --- | --- |
| Journey, lines 285 and 792 | Original Hangzhou 08:10 input, driving, 18:30–19:10 walk and 21:00 departure; all three locked wordings, both noise variants and reversed world keys |
| Current location/time and timestamp, line 289 | Exact MCP arguments/results and source log; current location and sampling timestamp reach the delivered response |
| Travel, outdoor duration and existing schedule, lines 290 and 797 | Short reply includes driving, the full walk interval and evening temperature drop; scripted clothing is explicitly ungraded |
| Short Main response without Task, lines 291 and 798 | Six completed Main Runs, one delivery per event, bounded replies, zero Task/Child/Approval/Artifact/automation rows |
| Recompute after location/time changes, line 292 | Same-time Shanghai change rejects a newer Hangzhou cache; evening Shanghai change rejects its morning cache; exact two-hour-old Hangzhou sample remains valid |
| Cost/irreversibility escalation, line 293 | Request with 300/person and nonrefundable 200 deposit stops at GC-13 prebooking checks and frozen-action approval requirement; no booking/payment Tool is invoked |
| Original clothing and stale refresh, lines 796 and 799 | Original fresh weather is unchanged; stale by one second over two hours forces a second read; short sleeve/light trousers/thin sweatshirt output, no heavy coat; unavailable refresh cannot reuse old temperature |
| Fixture/Gold/capability labels, lines 287, 295, 783, 785, 794 | Lock verifies all 16 original block identities; labels confer no semantic PASS |

Every variant is killed after the first committed weather Tool result, restarted,
and killed again after the initial committed Delivery. Completed calls and
generations must survive unchanged, with exactly one source request per actual
Tool execution. Duplicate ingress returns the same receipt, changed parameters
under its key fail, and a final drain leaves native state/source logs identical.
This exercises fixed completed-call reuse in this synchronous journey. It does
not claim Task cancellation, late old Task epochs or unknown external-effect
recovery: this Case creates no Task or authorized external effect. Those release
requirements retain their existing owners.

Each report has source SHA and working-file hashes before/after, binary hash and
Go build identity, OS/Python/Node/Go versions, timestamps, raw native state and
process/root cleanup observations. `--validate` recomputes every assertion and
17 native corruption probes, rejects incomplete/reordered/graded receipts, and
checks the current source/binary identity. A small public manifest binds hidden
`.gc10-native/` bytes by size and SHA256; these bytes must be retained for review.
Hash binding proves consistency, not authorship. Independent exact-SHA review
and required CI remain mandatory. The preview produces no Judge packets.

For subsequent real-model evaluation, the existing input adapter is:

```sh
python3 scripts/unscripted_cases.py matrix
python3 scripts/unscripted_cases.py freeze --help
```

Select `--case GC-10` with an explicitly configured approved local/account
profile and dedicated evaluation root, following
[the unscripted guide](guides/unscripted-evals.md). Its original M6 inputs match
this gate, without Gold/required text or scripted Tool plans. Its current adapter
is still a conversation fragment: connected source freshness/location/failure
and cost continuations need a production-profile adapter before whole-case
semantic grading. This remains ineligible for grading. ERII-223 tracks that gap,
current L2 weather/search evidence, model comparison and release scores. No live
profile or account is borrowed by this gate. `semantic_score` stays null and
`release_ready` stays false.
