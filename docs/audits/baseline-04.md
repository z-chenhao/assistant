# Frozen baseline-04 audit (ERII-234)

[baseline-04.json](baseline-04.json) retains all **80 exact source blocks** and
224 authored sub-obligations from the owner's work-package manifest. The range
starts at technical baseline §5.4 original L541 and ends at §6.2.2 original L692.
It includes account state and acceptance fences, Provider boundaries, model
failures, Tool naming, progressive discovery, Surface handles and Store writes.
It does not absorb adjacent clauses or implement product fixes.

There are 63 partial mechanical dispositions, four evidence gaps, two policy
rows and 11 background rows. Every mandatory sub-obligation remains routed for
release evidence or a finite repair. Mixed clauses explicitly retain their
background, optional design permissions and deferred Go Adapter alternative.
The Device Authorization reference describes the existing ADR 001 deferral;
it does not claim device-code delivery. No requirement is marked verified.

## Frozen identity

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Manifest baseline | `d11e543ea32efdf784478f5959ea6565af87b646` |
| Canonical `packages.baseline-04.blocks` digest | `2e56b4c2b855c9c6a81e667f316e92444e2890a8a35568a36cdce566fb07639e` |
| Audited code base | `2de83045c121cf457eb943270de758c17210dfaa` |
| Current technical baseline file | SHA256 `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

The package digest is pinned in the maintained validator independently of the
authored reviews. Local verification also checks the original manifest's full
1585-block assignment for uniqueness. ADR 006 / PR 45 changed Task text outside
this package. All 80 section/content hashes still match uniquely; current lines
are original lines plus 10. Changed or ambiguous content fails and requires
coordinator resealing with an explicit old/new mapping. This audit changes no
specification text and does not replace the original whole-file hashes.

The 122 ERII-30 partial/gap reviews remain separate and unchanged. This package
may overlap historical reviews; counts must not be added as verified coverage.
PR metadata records actual merged provenance, not proof of clause conformance.
`references` binds current file bytes; indexed tests are distinct from executed
tests. The retained Go JSON stream identifies 197 passing tests and subtests.

## Run and inspect

```sh
make audit-baseline-04
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch and Doctor need only Python, this checkout and its frozen ledger. Drive
uses the existing `release_audit.py` CLI. Evidence is the read-back report at
`evidence/baseline-04.json`, especially `package_audit.reviews`. Cleanup can
remove that disposable report. No network, account, model or VM is used by the
audit command. Output cannot overlap an input or retained native directory;
invalid reruns remove stale reports. `--require-complete` still exits **2**.
Audit completion cannot certify release.

To check independently acquired original inputs, retained observation bytes and
full Git history, run:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-04.json \
  --work-packages /absolute/durable/work-packages.json \
  --native-root /absolute/durable/audit-20261024 \
  --verify-git --evidence evidence/baseline-04.json
```

Inspect `manifest_checked`, `native_bytes_checked` and `git_provenance_checked`.
Without those inputs their flags remain false. Git verification checks audited
blobs and actual merged PR ancestry without fetching; missing history rejects.
Byte integrity cannot prove semantic completeness. Independent review must
judge the authored sub-obligations and their evidence.

## Native observations and limits

Raw observations remain outside the ephemeral worktree under the registered
durable evidence root:
`ERII-234/2de83045c121cf457eb943270de758c17210dfaa/audit-20261024/`.
The ledger retains exact commands, source SHA, start times, exits, sizes and
SHA256 for seven observation groups. CLI receipts also bind the binary bytes.
Dependencies were installed from the native offline npm cache; no historical
ZIP was downloaded. Keep multi-MiB traces local.

| Observation | Result and boundary |
| --- | --- |
| Focused Go contracts | 197 passing tests/subtests across LLM, Tool, SQLite, QuickJS and Computer packages; real SQLite, deterministic peers; not a full race or release suite |
| Provider contracts | 57 passing Node/Pi tests, including OAuth, dynamic catalog, continuation, redirects, transient retry bounds, cancellation and incomplete-stream rejection |
| JavaScript contracts | 19 passing actual QuickJS tests, including forbidden globals/imports, canonical aliases, catchable errors, Store, fresh cells, unawaited calls and replay drift |
| Account CLI | Nine checks through Go → Node → Pi → local OAuth/Responses fixture, six-process refresh, auth-loss cleanup, final usage and token scans; no live subscription claim |
| Local Provider CLI | 17 checks through production Probe/select/generation, role-separated Task/Root/Main continuation, usage and failed-retest admission; local HTTP peer, not current Qwen quality or image-mode evidence |
| Code Mode CLI | 17 checks with real QuickJS, nested Tool/Store/approval and two SIGKILL boundaries, invocation/event/effect readback and cleanup; not the complete 37+8 recovery matrix |
| Computer Use CLI | Eight checks of production adapter contracts against explicit HTTP/SSH peers, inventory, binding, lease/revision refusal, observations, scoped transfers and cleanup; not a current Guest/L2 observation |

These are mechanisms and inspectable observations. They supply no semantic
Golden score, live account compatibility, current VM proof or named human
acceptance. The report stays `inconclusive`, `release_ready=false`, with zero
verified requirements. Required five-round and release gates remain unchanged.

## Routed work

| Route | Exact remaining work | Independent exit |
| --- | --- | --- |
| [ERII-335](https://multica.ai/erii-studio/issues/01a16dd9-6f96-7e35-88a3-c5f794a8e958) | Provider process permission refusal and single SecretStore ownership; actual Host-push credential envelopes versus the specified Secret RPC | Use secret-free fixtures through the production launcher, retain allowed/denied native observations and any minimal repair or approved ADR, with independent review. Minimal environment and temporary cwd do not prove file isolation. No leak is alleged. |
| [ERII-336](https://multica.ai/erii-studio/issues/01a16dd9-72c5-702d-8049-2d3df3556ef2) | Ordinary `describeTool` output types, namespace explanation and minimum lifecycle/function declarations | Replace unjustified `Promise<unknown>` only with trusted frozen declarations; inspect through actual QuickJS discovery and preserve permission/compatibility contracts. Current top-level text also omits the explicit no-rollback warning. |
| [ERII-337](https://multica.ai/erii-studio/issues/01a16de0-3fc6-73db-a1a2-4ce56d52bb20) | Ordinary Run terminal session disposal | Connect shared Main/Root/Child termination to corresponding Provider session cleanup and verify terminal, wait/pause, cancellation and restart boundaries. `Dispose` exists and has tests, but production calls currently occur only in Probe cleanup. |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Actual authorized local/account compatibility, Main decisions and model compliance with discovery/observation policies | Preserve the original frozen dual-arm evaluations, native Run/Tool/Artifact state and independent Judge evidence; missing resources remain inconclusive. |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | Current Web/Feishu and real Guest inventory, binding, operations and lease recovery | Current approved environment, native observations/Artifacts, cleanup and named human review. Historical snapshots and deterministic peers cannot substitute. |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Remaining per-obligation current-head protocol, security, replay, race, repetition and CI evidence join | Consume complete actual receipts with independent review; route newly observed implementation gaps separately and retain unmet duties until resolved. |

Each row's `remaining` field identifies its exact sub-obligation and route.
New repair records are Backlog proposals for coordinator admission; this audit
does not dispatch them or grant implementation approval. A child being Done or
this audit PR merging does not establish v1 release acceptance.
