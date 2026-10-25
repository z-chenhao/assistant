# Frozen baseline-02 audit (ERII-232)

[baseline-02.json](baseline-02.json) records **83 exact current source blocks**
and 295 authored sub-obligations. The original 80-block range was §3.5 L165 through
§3.11 L342. Owner-authorized ADR006 resealing retained 78 unchanged blocks,
replaced two Task contract blocks and assigned three new completion-policy
clauses to this package. No adjacent requirement was added independently.

There are 75 partial mechanical dispositions, four evidence gaps, two background
rows and two policy rows. Sub-obligation scope distinguishes 290 v1 duties, one
policy duty, three background items and one deferred item. Every non-exempt duty
has a finite route; each background/deferred item has an explicit reason. None
is marked verified. The original 122 ERII-30 partial/gap reviews remain unchanged
and separate; overlapping counts cannot be summed as certified coverage.

## Frozen identity

| Input | Identity |
| --- | --- |
| Immutable original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Original baseline | `d11e543ea32efdf784478f5959ea6565af87b646` |
| Resealed manifest | 385378 bytes; SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Resealed baseline-02 package digest | `7480b2bdfd3d86256d1d13a10f6b2cd9707fb62f365f07e7e244f17a0b510c69` |
| Audited code base | `3f33d942776712c3ad689c8e67f4009214d03c28` |
| Current technical baseline hash | `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

Owner comment `01a16e1d-3e2c-78fb-ba1d-5621d178a9bd` on
[ERII-232](https://multica.ai/erii-studio/issues/01a16938-d29b-7d7d-a011-673bb15a4653)
adopted the [old/new mapping](baseline-02-reseal.json) and assigned current §3.6
L176 and §3.9 L229–230 to baseline-02. The coordinator independently checked all
1588 current blocks across 36 packages: zero omissions, duplicates or stale
identities. The receipt is recorded on ERII-232 and
[ERII-223](https://multica.ai/erii-studio/issues/01a16936-335f-72a4-a7f4-618823a87151).

`original_frozen_blocks` retains all original identities. Each current row records
its source origin and original identity where applicable. The five changed/new
clauses were reviewed afresh. The validator independently pins both the new
83-block digest and original 80-block provenance. Other audits retain their
original manifest bindings. Changed or ambiguous source still rejects and needs
authorized resealing. Neither assignment nor merge provenance proves conformance.

## Run and inspect

```sh
make audit-baseline-02
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch and Doctor require Python, this checkout and the ledger. Drive uses the
existing `release_audit.py` CLI. Evidence is the read-back report at
`evidence/baseline-02.json`, especially `package_audit.reviews`. Cleanup may remove
that disposable report. The audit command needs no account, model or VM. Invalid
reruns remove stale output; output cannot overlap an input or retained native
directory. `--require-complete` still exits **2**, with null semantic scores,
`status=inconclusive`, `release_ready=false` and zero certified requirements.

For independently acquired inputs, native bytes and full Git history:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-02.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-final-20261024 \
  --verify-git --evidence evidence/baseline-02-full.json
```

This checks the sealed byte digest, all 1588 current identities assigned exactly
once, exact package ownership, native file sizes/hashes, audited Git blobs and
merged PR ancestry. Explicit report flags are `manifest_checked`,
`native_bytes_checked` and `git_provenance_checked`; absent inputs leave them
false. Missing Git objects reject without fetching. Hashes establish integrity,
not observation truth or semantic completeness.

## Native observations and limits

Full evidence remains under the registered durable root:

```text
/Users/zhuchenhao/Projects/erii/evidence/agent-delivery/ERII-232/
  3f33d942776712c3ad689c8e67f4009214d03c28/audit-final-20261024/
```

The ledger binds 56 current file/symbol references and merged PRs
8/10/11/12/16/22/28/33/45. An indexed test is distinct from an executed test.
`contracts.log` identifies 177 passing tests/subtests across Agent, SQLite, Task
and Automation packages, with zero skipped. Each observation records its exact
command, source SHA, timestamp, exit code and raw file identities.

- `verify_tasks.py`: actual create/search/append, ongoing idle behavior, explicit
  complete, rejected completion, immutable policy/anchor, receipts, follow-up
  deduplication and 14 SIGKILL boundaries, with CLI/PTY/SQLite readback.
- `verify_task_control.py`: live pause/cancel and new Root continuation, epoch
  fences, no stale follow-up and three stop-commit/settlement SIGKILL boundaries.
- `verify_automation.py`: calendar/threshold/late-fire paths, edited schedules,
  Forget provenance, Inbox/Outbox/Actor admission and fire/admission restart paths.

All three reports record owned-process cleanup and removal of disposable state.
Provider injection was explicitly deterministic. These samples provide no real
model Golden Case scores, current Live Shadow observations or consecutive-five-run
certification. Multi-MiB state stays local; publication uses a small manifest.

## Remaining routes

| Route | Remaining scope | Independent exit |
| --- | --- | --- |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Actual Task decisions, ambiguous matches, self-contained delegation, identity/context use and final expression | Frozen model arms/Golden conditions, native state and independent Judge receipts; unavailable resources remain inconclusive |
| [ERII-342](https://multica.ai/erii-studio/issues/01a16e46-7f12-7af5-b0f8-c5070324cf06) | Literal domain names versus private constructors, Runs/Actor and context functions at current L208/L213/L344/L349 | Explicit source-to-responsibility mapping and independent review; normative deviations require owner ADR/resealing |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Current-release evidence join for every mechanical/policy duty, complete error/recovery/upgrade coverage and protected repetitions | Actual current receipts and independent review; never infer completion from enums, this ledger, merged PR counts or green CI |

ERII-342 is a bounded Backlog proposal, not a dispatched implementation or waiver.
Existing ERII-338 (Code Mode RPC) and ERII-341 (Tool events) have different scope.
Every remaining duty is keyed to its authored sub-obligation; mixed optional or
deferred wording does not exempt adjacent v1 duties. Independent review must judge
whether the decomposition is complete. This PR accepts audit assets, not release.
