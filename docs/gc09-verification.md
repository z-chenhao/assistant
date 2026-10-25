# GC-09 agenda verification

ERII-255 owns the original 17 frozen source blocks. This entry proves scripted
L0/L1 mechanics, with no semantic grade or v1 release claim.

```sh
make build
python3 scripts/verify_suite.py --binary bin/erii --gate gc09 --evidence evidence/gc09
python3 scripts/verify_gc09.py --binary bin/erii --evidence /path/to/gc09.json --validate
python3 scripts/prepare_release_eval.py --input /path/to/gc09.json --evidence evidence/gc09-preview.json
python3 -m unittest discover -s scripts -p 'test_gc09.py'
```

The runtime `gc09` gate executes six unchanged M1 variants, each in five
consecutive repetitions. It reuses `run_cases.drive` for the exact short
Main-only agenda and separately locked `gc09-steering-v1` same-day update.
Three original wordings, two 10–30% noise variants and one source-key ordering
variant retain every calendar entry, E1–E4, T1–T2 and the original event.
The explicit golden Fake recognizes bounded numbered output; it is not a
semantic evaluator of arbitrary prose. E1/T1 precedes noon, preparation precedes
the interview, E4 needs checking without another reply, and E2/E3 are deferred.
Steering omits completed items, moves preparation around personal time, and
recovers after actual step-zero context-freeze SIGKILL with frozen snapshots.
Duplicate ingress and replay leave complete readback identical.

An additional isolated instance drives the same original input through three
actual stdio MCP connectors (calendar, mail, work). Returned bytes must match
the locked world. Write Tools are available behind normal Host permissions but
never invoked. The driver kills the CLI after initial delivery commit, restarts
it, and requires all completed reads/generations to remain identical without
another connector call. It observes zero Task, schedule, Approval and Artifact
for the original short request. No real account or network source is used.

The separately locked `gc09-boundary-v1` inputs explicitly request two background
preparations. This supplemental request does not alter the original Gold's
requirement that the short agenda create no Task. Main creates two ongoing Tasks;
the harness holds both accepted Worker generations at the maintained
`before_worker_generation` barrier and captures both Roots running together.
It holds Main outside a transaction at `after_delivery_commit` until Reporter
queues both results. On release, exactly one follow-up Main asks one combined
question with both Task references. Distinct supplied answers go through
`task_search`/`task_manage(append)` to their original Tasks and new epoch-2 Roots.
Actual model requests contain only each Root's own answer. Old completed Roots
stay immutable. Both answer results return in one Main delivery. This proves
aggregation of an available batch; it does not claim an unbounded wait window
for Workers that become blocked at different times.

`--validate` recomputes all 30 native receipts, source/Model/Capability identity,
actual connector results, Worker/Task/update/inbox mapping, completed generation
to visible delivery agreement, duplicate ingress and replay equality. Thirteen
corruption probes reject source/output/Steering drift, false restart, connector
write, answer misrouting, stale epoch, revived old Root, missing actual input,
split batch, altered completed call, unexpected effect and forged model output.
Stored all-true flags cannot override recomputation.

The compact public `gc09_native_manifest` binds size and SHA256 of complete raw
receipts under `.gc09-native/`. The strict reader rejects summary drift, unsafe
paths, symlinks, missing/truncated bytes and duplicate JSON keys. Keep native
bytes and the binary in durable local evidence for independent review; the
existing CI uploader excludes hidden directories. Source SHA and tracked source
hashes, binary hash/build, environment versions, times and owned-process and
disposable-root cleanup accompany receipts. Hash consistency is not execution
attestation. Judge preparation recomputes the native report and emits no Judge
packets, scores or winner. Failed runs remain inconclusive and cannot reuse a
stale success manifest.

| Frozen source lines | Mechanical coverage and remaining boundary |
| --- | --- |
| 271, 273, 281 | Daily agenda and three connector reads. Source freshness and model prioritization remain ungraded. |
| 275, 276, 277 | E1/T1 priority/deadline, E2/E3 deferral, E4 check, bounded output; no mail/calendar write, approval, cancellation, schedule or Artifact. |
| 278 | Two concurrent Root Workers, one batched question, preserved Task refs, distinct answer routing to new Roots. Scripted scheduling/language does not prove unscripted judgment. |
| 279 | Existing same-day Steering and context-freeze recovery omit completed work. No active Worker exists during that original synchronous Steering. |
| 753, 755, 771, 773, 775, 776, 777, 778, 779 | Unchanged original world/clock/event/Gold across all six variants and five repetitions. |

No Child Run, consented external effect or unknown side-effect outcome exists in
this Case. Child approval inheritance, cancellation of active work and late
cancelled-owner effects are not claimed by these receipts. Unsolicited Task
cancellation is rejected by the exact Tool inventory. Model semantics and current
L2 remain associated with parent ERII-223, with `semantic_score=null`,
`eligible_for_grading=false` and `release_ready=false`. The maintained
`unscripted_cases.py freeze --case GC-09` entry uses the production Pi Bridge and
an operator-prepared profile for original user inputs only; full connector and
concurrent Worker adapters remain incomplete. No credential is borrowed.

The original `run_cases.py` runtime `cases` gate and `test_agenda_case.py` remain
in place. Their six agenda/Steering preview packets remain ineligible for grading.
They alone do not certify the supplemental Worker or connector path.
