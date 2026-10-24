# Frozen baseline-07 audit (ERII-237)

[baseline-07.json](baseline-07.json) disposes of **80 exact source blocks**,
from technical baseline §6.6.4 original L1112 through §8 original L1248.
The coordinator-sealed ADR006 manifest locates them at L1122–1258 with
unchanged section/content identities. The 362 authored sub-obligations cover
Claim/Wake/cancel, quotas and monetary consent, reference implementation
choices, Code Mode acceptance scenarios, Skills and the first three Security
clauses. No adjacent clause is added and no product protocol is changed.

There are 56 partial mechanical rows, seven evidence gaps, 15 background rows
and two policy rows. Duties retain 332 required-v1, six policy, 23 background
and one explicitly conditional future statement. All **338 required/policy
duties remain routed**. The single deferred statement is the optional future
near-limit system hint; it does not defer current quota protection. Reference
table left columns are historical context; right-side product choices remain
mandatory. Optional prefix caching creates no mandatory cache-hit target.
No requirement is marked verified; no semantic scores are awarded.

## Frozen identity and provenance

| Input | Identity |
| --- | --- |
| Original manifest | SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`; 380773 bytes |
| Original baseline | `d11e543ea32efdf784478f5959ea6565af87b646` |
| Original package digest | `e4c7b80cf8886c25307f3b225c1970d4ff7234ca5f7e6a0cacb36ac4a0c5ace3` |
| ADR006 manifest | SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `d05cf38d322f6c69ad05e499d0caacafc0c520bec0e728170f05a3cf517fcd63` |
| Audited code base | `77cc7e07597a4dc6e901cad701147f9e25d9aa6d` |
| Technical baseline file | SHA256 `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

The ledger retains both exact package lists and original/current line mapping.
The existing validator independently pins both digests. Strong local checking
compares all 1588 resealed source identities, each assigned once. The coordinator
receipt is comment `01a16eef-49cd-7829-933f-1575886adc28`, referencing parent
receipts `01a16e21-9bcb-7ae6-bf4a-4e466fdfbe5b` and
`01a16e27-7505-7a40-bd57-3f4b287bc177`. Their existing seal is consumed; this
PR neither reseals changed text nor grants an architecture deviation.
Changed or ambiguous content rejects and needs coordinator resealing.

Each row preserves full source text, authored obligations and scopes, current
code/test/harness hashes, actual merged PR provenance, native observation
references, limits and exact remaining duties. The eight merged PRs are
12/15/16/17/18/20/23/29; ancestry and audited blobs were checked locally.
The 42 indexed references are navigation evidence, not a claim that every
indexed test ran. Original ERII-30's 122 partial/gap reviews and earlier package
ledgers stay separate and unchanged.

## Maintained verification path

```sh
make audit-baseline-07
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load the frozen ledger and current checkout without network,
accounts or VM. Drive uses the existing `release_audit.py` CLI. Evidence is
`evidence/baseline-07.json`; inspect `package_audit.reviews`, dispositions,
remaining duties and routes. Cleanup removes only disposable output. Missing,
duplicate, changed blocks, stale references, unsupported exemptions, original
location drift, invented PASS and unrouted duties reject. Invalid reruns remove
stale reports; output cannot overlap an input or retained native directory.
`--require-complete` still exits **2**: this audit cannot certify release.

Stronger read-only checking uses independently retained inputs:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-07.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-07.json
```

`manifest_checked`, `native_bytes_checked` and `git_provenance_checked` report
which optional inputs were actually checked. Without them these flags are
false. Git checking does not fetch; shallow CI does not pretend to verify
unavailable history. Hashes establish integrity, not semantic completeness;
independent acceptance review must inspect the authored duties and their limits.

## Observed native evidence

Raw evidence stays under the registered durable evidence root:
`ERII-237/77cc7e07597a4dc6e901cad701147f9e25d9aa6d/audit-20261025/`.
Receipts retain commands, source SHA, timestamps, exits, sizes and SHA256;
CLI receipts also bind the actual locally built binary. Environment versions
are retained. Dependencies came from the offline lockfile cache. No historical
ZIP or complete CI artifact was downloaded. Multi-MiB traces remain local.

| Observation | Result and practical limit |
| --- | --- |
| Focused Go race contracts | 181 passing tests/subtests across selected SQLite, Skill and Tool contracts. Actual storage/runtime with deterministic peers. This is not the complete Go race inventory. |
| JavaScript | All 19 maintained QuickJS tests passed, including pending-call rejection at 129, no ambient time/network/import, stable call keys, replay ordering and unawaited Host calls. These tests expose the capacity gap rather than waive it. |
| Scheduling CLI/PTY | One Task/Root/100 unique Children, peak 16, 98 results/two fixture failures; SIGKILL after 25 admissions preserves identities; an independent Task progresses beside 16 live grandchildren; cancel and restart do not revive the old tree. |
| Monetary consent CLI | Nine groups: one Root approval for 100 items; actual-overrun result reuse; tool-effect approval remains independent through kill/restart; denial prevents model/effect dispatch; Main cancellation invalidates Root consent; reservation survives SIGKILL; unpriced/unconfigured/missing usage remain explicit. |
| Fixed-source recovery CLI | Sidecar SIGKILL, Go SIGKILL and SIGTERM at 37 completed plus eight active; 45 admitted identities reattach and 100 finish; completed provider dispatch counts do not increase; suspended activeVMs=0. |
| Child CLI/PTY | Eight check groups plus six SIGKILL boundaries; direct Parent results, recursive Child, scoped/large Artifacts, per-invocation approval, unknown-effect preservation and cancellation. |
| Skill discovery CLI | Progressive summary/view, real binary asset bytes, Worker management/Task control refusal, frozen Skill hash, rejected approval before provider dispatch, unknown MCP effect never retried. |

Each harness reports cleanup of owned processes and disposable state. The
six-hour approval test advances a controlled logical clock; it is not a real
six-hour soak. Pure `cua` long waits, all combinations of unawaited calls and
recovery, 100 large outputs, complete UI privacy and real model grouping of
missing information remain unproven. No real model/account/Guest compatibility,
semantic quality, full performance baseline or five consecutive protected
release runs is claimed. The report remains `inconclusive`,
`release_ready=false`, `verified_requirements=0`.

## Finite gap routes

| Route | Exact remaining scope and exit |
| --- | --- |
| [ERII-343](https://multica.ai/erii-studio/issues/01a16ef3-43c0-78cd-977f-840dce140018) | Original L1120: `code_stop_intent` invalidates token/owner but does not increase `execution_epoch`; claim does. Establish the specified first-stage epoch change and bottom-up stable/unknown settlement with migration/race/restart evidence, or obtain owner ADR and coordinator reseal. Existing token protection is not alleged broken. |
| [ERII-344](https://multica.ai/erii-studio/issues/01a16ef3-47a8-7ad5-a3ef-3fb5975a05a8) | Original L1128–1129: no configurable per-execution cumulative Host Call limit of 10000/50000; pendingCalls=128 rejects a large parallel cell. Verify separate cumulative counting and bounded transparent queuing at 100/1000 items, replay, cancellation and memory boundaries. No unbounded pending queue or weakened tests. |
| [ERII-345](https://multica.ai/erii-studio/issues/01a16ef3-4aea-7e6d-93a7-bf158e28d3ff) | Original L1172/L1193: code placement/dependency diagram differs from current `agent/sqlite` composition. Resolve every named responsibility and dependency with precise mappings; an intentional normative change needs owner ADR and resealing. Do not create empty wrappers. ERII-342 owns a different §3 API naming scope. |
| [ERII-346](https://multica.ai/erii-studio/issues/01a16ef3-4dfc-7c22-b841-2da0e323cd8b) | Original L1218: collect all eight named performance metric groups under fixed comparable conditions, with units/samples/noise. Model/machine thresholds need the specified measured inputs and owner freeze; one RSS sample or total fixture time is insufficient. |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Original frozen dual-arm actual-model evaluations: missing-information aggregation, single Main question, quota/spend refusal completion, Skill experience quality. Native state plus independent Judge; no Fake semantic scores. |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | Current UI/Guest/channel observation with named human review, precise approval/state/Artifacts and cleanup; includes aggregate counts, hidden internals and pure `cua` waiting/recovery. |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Per-duty final current-head evidence join, protocol/management equivalence, required CI/repetitions/independent acceptance. Unobserved duties remain unmet; a discovered implementation fault needs a finite repair, not a fabricated report PASS. |

The four new records are Backlog proposals, awaiting coordinator registration
and admission. Their sources, exclusions, deduplication and independent exit
are in each record. They do not authorize repairs in this audit PR. Existing
ERII-338 RPC/Outcome and ERII-341 Tool-event gaps remain intact. Audit completion
and any child being Done do not certify v1 or close its original release gates.
