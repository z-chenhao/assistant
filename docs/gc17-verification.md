# GC-17 feedback verification

The existing Core runner owns the frozen GC-17 journey, including all six
original wording, noise and order variants. No Golden input, Gold distribution,
score, timeout, worker capacity or repetition changes. The supplemental lock
binds its 21 original source blocks to ERII-223's frozen manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.

```sh
make build
python3 scripts/run_core_cases.py --binary bin/erii --case GC-17 --evidence evidence/gc17.json
python3 scripts/run_core_cases.py --binary bin/erii --evidence evidence/gc17.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc17.json --evidence evidence/gc17-judge.json
python3 -m unittest discover -s scripts -p 'test_gc17.py'
make verify VERIFY_SHARD=codemode
```

For the original five consecutive GC-17 rounds alone, use the maintained CI
partition (the full `Ubuntu checks` aggregate still requires every partition):

```sh
python3 scripts/verify_suite.py --shard codemode --codemode-part GC-17 --binary bin/erii --evidence evidence/gc17-series --jobs 1
```

Launch creates private configuration, stores and an explicitly selected Core
Fake profile. Doctor checks the locked QuickJS dependency and model readiness.
Drive sends the original question, synthetic context and the exact CSV/five
attachment bytes. The harness sends no Tool plan or hidden classification Gold.
The explicit Fake Main creates one Task; Root selects a fixed Code Mode script,
saves the inputs through ArtifactStore and submits 100 ordinary Child calls.
The script returns text-parsed Child results and computes its own distributions.
The Host creates all call keys. Children inherit frozen snapshots and explicit
Artifact references, and cannot control Tasks or deliver to the user.

At exactly 37 committed results and eight active Children, the harness sends
SIGKILL to the identified owned Sidecar and Go process. The receipt records the
Go exit code and the Sidecar signal/parent identity plus a subsequent process
observation proving it is absent or a zombie; it does not invent a Sidecar exit
code. Restart reuses the 37 completed calls/generations, reattaches accepted
identities and retains the eight exact frozen approvals. The independent
effect ledger binds those eight approved effects to their idempotency keys,
arguments and results. F055's same-key parameter drift fails with
`NON_DETERMINISTIC_REPLAY`; original parameters continue successfully.

The CLI reopens `feedback_analysis.csv`. It has exactly the original 100 row IDs,
the exact category/severity distributions, distinct duplicate-text rows, row
evidence/attachment links, and unknown results for insufficient evidence.
Root's computed summary agrees with the rows. Main publishes and delivers that
same stored CSV reference once. The initial acknowledgement has no file; all
eight security approval messages remain distinct from the single Main
completion. Duplicate ingress and another drain leave complete native state
unchanged. Cleanup joins owned processes and removes private state. Failed
native observations are retained before cleanup.

`gc17.py` recomputes these assertions for CLI validation, CI part collection and
the five-round Core aggregate, and Judge preparation. It joins frozen model
requests, accepted direct calls, nested CodeCalls, Child results and final
generations, Artifact save/publication, Main generation/content parts and sent
Delivery identity. Source full SHA and working-file hashes, binary SHA256/build
information, environment and both crash/replay observations are retained.
Local CLI validation additionally checks the actual binary and freshly observed
environment. Portable CI/Judge consumers check the recorded identity and native
joins; the CI part separately binds the binary digest to its suite. Unique
execution IDs are required across variants and all five aggregate rounds.

Each ordinary runner invocation proves a positive control and runs all native
corruptions through the actual CLI, CI consumer and Judge preparation. Attacks
synchronize replay, recompute assertions and reseal compressed bytes. Wrong,
missing, duplicate, extra or early file parts; wrong reply/publication/delivery;
changed CSV, Child, epoch, model/capability, approval/effect, source key, crash,
drift and provider dispatch evidence reject. Unit regressions reject flag-only
reports and hash/size/type drift. Existing Core inventory/scoring tests isolate
those mechanics; they do not substitute their synthetic rows for native proof.

Failed assertion claims reject before decompression and native recomputation.
This preserves the original ten-minute CI job and every five-round input/check;
declared failures cannot certify a receipt. Three additional attacks forge all
claims true while retaining a wrong delivered reference, missing dispatch or a
whole Worker tree whose epoch differs from the actual completed Task,
so all three consumers must still recompute the native evidence to reject them.
Corruption construction decodes its immutable control once, then copies that
already validated JSON tree. Incoming consumer receipts always use the strict
reader. A retained before/after comparison proves the original 33 candidate payloads
were byte-identical across this construction optimization. The final oracle also
binds every Worker to the completed Task identity/epoch and adds the whole-tree
epoch attack, for 34 candidates (one control and 33 corruptions).

The Core JSON embeds size/hash-bound compressed native bytes, avoiding repeated
large uncompressed states. Complete consumer evidence stays in a hidden sibling
directory. `control.json.gz` retains the full control; each other archive retains
its control SHA256 and replacement first result. Replacing that result in the
control reconstructs every candidate exactly; the harness checks byte equality
and retains each candidate SHA256, command and outcome. This preserves all raw
information without duplicating five unchanged executions for each corruption.

`unscripted_cases.py freeze --case GC-17 --variant wording-1` freezes original
questions and allowed CSV/attachment data for an explicitly operator-prepared
production profile. `verify_unscripted_cases.py` drives this input fragment
through the real CLI/bridge with a loopback peer. It supplies no script, Gold,
automatic approval or credential. Connected full Child/Artifact/recovery runs,
actual model quality, five unscripted repeats, longer contexts, current L2 and
semantic/human grading remain due under ERII-223. Scores stay null, eligibility
and `release_ready` stay false. This Case's approved known effects do not certify
unknown-outcome or cancellation behavior; the existing dedicated conformance,
cancellation, fencing and unknown-effect gates remain required.
