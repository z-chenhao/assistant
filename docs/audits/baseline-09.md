# Frozen baseline-09 audit (ERII-239)

[baseline-09.json](baseline-09.json) disposes of **80 exact source blocks** from
technical baseline §9.6.5 original L1518 to §11.6 original L1728. The existing
ADR006 seal moves them to L1528–1738 with identical section/content hashes and
order. This package covers Capability discovery, CUA file transfer and output,
Conversation/RunTrace/context ownership, Compaction, hybrid recall, Forget,
Memory storage, search and the beginning of the Curator contract.

The ledger retains 284 authored sub-obligations: 256 required-v1, five policy,
12 background and 11 explicitly deferred duties. All **261 mandatory/policy
duties remain routed**. There are 65 partial mechanical rows, two evidence gaps,
one policy row, ten background rows and two deferred rows. Table headers and
separators remain inspectable. Future Capabilities and future Provider migration
do not exempt current execution, privacy or storage duties. No requirement is
certified, no historical partial/gap is promoted, and no semantic score is given.

## Identity and provenance

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Original baseline | `d11e543ea32efdf784478f5959ea6565af87b646` |
| Original package digest | `e4c26626df402e3f0f7e9ecf731901b9b17e33fa25cd62049a350d6148fdab53` |
| ADR006 manifest | 385378 bytes; SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `680dd54a6cbc5587ceb348b70adda1956f38a0b03ffe9e622204e587991c5f46` |
| Audited code base | `87fecc5ba864aa0235bd711eb32c6ceaead19491` |
| Technical baseline file | SHA256 `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

Coordinator comment `01a16f2f-98eb-77a0-b200-0dae49700dab` authorizes consuming
the existing seal. Its parent reseal/independent-check receipts are
`01a16e21-9bcb-7ae6-bf4a-4e466fdfbe5b` and
`01a16e27-7505-7a40-bd57-3f4b287bc177`. The consumer independently pins both
package digests. Strong mode checks all 1588 sealed source assignments exactly
once against the current inventory. Changed or ambiguous content rejects and
requires coordinator resealing; this audit makes no normative change.

Each row retains full source text, old/sealed/current locations, scoped duties,
observations, limits and exact remaining routes. The 45 current file references
are byte-bound navigation evidence, not a claim that every indexed test ran.
The seven merged PRs are 18/20/22/23/26/32/35; full local Git ancestry and audited
blobs were checked. A merged PR proves provenance, not product conformance.
The historical ERII-30 ledger and all preceding package ledgers remain unchanged.

## Maintained verification

```sh
make audit-baseline-09
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load the ledger and sources without network, accounts or VM.
Drive invokes the maintained `release_audit.py` CLI. Evidence is
`evidence/baseline-09.json`; inspect `package_audit.reviews`, scopes, routes and
observation receipts. Cleanup removes disposable reports only. The inherited
rejection suite covers missing/duplicate/out-of-package/changed blocks, stale
references, unmerged PRs, native byte/path tampering, unsafe output paths and
invented PASS. Package regressions also protect original lines, mandatory routes,
exemption reasons and the distinct token/Curator gaps. Invalid reruns remove stale
output. `--require-complete` still exits **2**.

For same-host read-only strong verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-09.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-09-strong.json
```

Only this mode sets `manifest_checked`, `native_bytes_checked` and
`git_provenance_checked` true. Shallow CI does not claim to inspect unavailable
history or local observations. Hashes prove integrity; independent review must
judge whether the authored duties fully express the source clauses.

## Observations and limits

Full raw evidence remains in the registered durable root under
`ERII-239/87fecc5ba864aa0235bd711eb32c6ceaead19491/audit-20261025/`.
Receipts retain actual commands, source SHA, timestamps, exits, sizes and hashes;
CLI observations also bind the built binary. Offline locked setup and environment
versions are retained. No historical ZIP was fetched.

| Observation | Result and practical limit |
| --- | --- |
| Focused Go race contracts | 53 passing tests/subtests in selected context, recall, Memory, Conversation and SQLite contracts. Two package-level skips mean no tests in the interface/Fake packages; no test case skipped. Covers prior schemas, concurrent supersede, rollback, domain ranking, frozen per-turn recall, Forget and fresh successor ownership. This is not the full Go race inventory. |
| Surface contracts | Four passing tests: CUA revision/capability/transfer, frozen mutation approval and unconfigured Memory rejection. This is a selected supplement, not all Computer contracts. |
| Recall CLI | Six scenarios: ordinary retrieval, actual SIGKILL/restart after recall freeze, provider failure, multipart/current and prior schema, and deterministic concurrent append failure. State and request readback use real SQLite and CLI, with explicit HTTP embedding and model peers. |
| Memory maintenance CLI | Twelve scenarios cover noisy reports, Main control/Worker refusal, immutable explicit correction/encryption, Curator commit/restart, frozen and queued Forget successors, post-barrier fresh remember provenance, long-context original retention, verified Skill lifecycle and persistent opt-out. Explicit corrections do not prove automatic semantic conflict merging. |
| CUA CLI and QuickJS | Eight CUA check groups retain separate transfer/overwrite/export/upload approvals, file bytes, registration/scope refusal, navigation expiry, denied input, traversal and selected-image context. All 19 QuickJS tests passed. Peers represent SSH/Browser dependencies; no current Guest compatibility is claimed. |
| Curator gap probe | Two successive current-home-city statements produce two live automatic records with no supersedes link. The probe reuses maintained CLI harness mechanics and retains source and state. Exit zero means the gap was reproduced, not product PASS; it does not decide which statement is true. |
| Transfer boundary correction | Frozen Guest helper read and write each accept 8,388,608 bytes and reject 8,388,609 bytes with JSON `PERMISSION_DENIED`. All four subprocesses exit zero, so exit status alone does not prove acceptance. This executes the actual helper in disposable local files, without Host/SSH/VM transport. Host `WriteFile` is source-inspected only. |

The transfer observation and probe source are retained in the native root's
`transfer-repair-20261025/` subdirectory; earlier raw files remain unchanged.
The probe extracts the helper from the audited Git revision and verifies that
both Host and Guest bytes still match this checkout. Its receipt binds
`internal/computer/macosvm/use.go` (`WriteFile`, lines 394–397) and
`internal/computer/macosvm/transfer.py` (write/read guards, lines 33/50/52).
The Guest helper SHA256 is
`432b3ef5a1f436c189f7d876dc8294cb175c3f746385d0aba7f8727e31607007`.
These source-bound observations correct the previous erroneous 64 MiB description;
the current Host and Guest raw-file limit is **8 MiB**. Product limits are unchanged.

All CLI harnesses/probe report cleanup of their children, HTTP peers and
disposable databases/workspaces. Compaction uses a fixed three-byte-per-token
approximation, not observed model token accounting. Core is a ranking tie-break;
no static Core prefix/cache-hit measurement is claimed. Encrypted Memory builds
FTS/vector working sets in private in-memory SQLite; index rebuild performance
was not measured. Host transfer buffering stays outside JS/RunTrace. The helper's
local 8 MiB boundary observation does not establish end-to-end Host/SSH/Guest
large-file behavior, memory usage, disconnect handling or arbitrary file sizes;
those checks remain unexecuted here.

No actual-model quality, live account, current macOS Guest, full protected five
runs or complete release suite is claimed. Output remains `inconclusive`,
`release_ready=false`, `verified_requirements=0`.

## Finite gap routes

| Route | Remaining duties and independent exit |
| --- | --- |
| [ERII-352](https://multica.ai/erii-studio/issues/01a16f33-2755-76d6-a96f-5ac3e3e8f661) | Original L1589/L1591 and related L1595: reconcile the normative token threshold with the current fixed JSON-byte estimate. Preserve frozen context/replay; prove threshold boundaries, multilingual/tool/image budgets, context-length retry, migrations, CAS/race and restart. Normative changes require owner ADR/coordinator reseal. One finite PR with exact-SHA CI and independent review. |
| [ERII-353](https://multica.ai/erii-studio/issues/01a16f33-2af9-73ad-af32-b0c29774632d) | Original L1728 and Memory merge duties: source-aware conflict handling for different-text automatic facts. Preserve uncertain evidence and immutable provenance, Worker read-only scope, event idempotency, concurrent Child deduplication and Forget. Exit includes the reproduced two-city fixture, explicit/implicit corrections, competing sources, migrations, race/restart, exact-SHA CI and independent review. Semantic quality still needs original real-model/Judge evidence. |
| [ERII-349](https://multica.ai/erii-studio/issues/01a16f0e-9633-7a2e-ad59-5daf19a28f93) | Original L1521: verify precise Capability declarations, guidance and safety against the actual facade/Host/Driver methods and optional parameters. Reuse the preceding package's finite API repair; no duplicate issue. |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Real-model discovery/selection, opaque reference use, output choice, recall relevance, stable Memory judgment and summary fidelity. Exit remains the original frozen dual-arm evaluations with native state and independent Judge; scripted fragments cannot supply scores. |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | Current approved Guest browser download/chooser/upload/Artifact and compatibility observations with named human review, exact approval and cleanup. Local peers cannot substitute. |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Final current-head per-duty implementation/test/migration/replay/race/approval/protected-five-run/native-CI and independent-review join. Every remaining duty stays unmet until actual evidence resolves it; route implementation faults into finite repairs. |

The two new records are unassigned Backlog proposals awaiting coordinator
registration/admission. Deduplication inspected all 64 existing parent children.
Finishing this package or one repair does not certify the v1 release.
