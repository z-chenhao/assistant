# Frozen baseline-12 audit (ERII-242)

[baseline-12.json](baseline-12.json) retains **80 exact source blocks**, from
technical baseline §16.3 original L1985 through §17.M5 original L2143. The
approved ADR006 seal locates them at L1995–2153. All 80 source/section/content
identities are unchanged; only line locations moved. No previous package's
conclusion is inherited.

The ledger records **452 sub-obligations**: 429 required-v1, 16 policy, four
background and three explicitly deferred duties. All **445 mandatory/policy
duties remain routed**. The four background rows explain a list introduction,
table header or separator. Device Authorization is explicitly deferred by
§5.4/ADR001 and the M1 completion clause; the surrounding Browser OAuth, PKCE,
refresh and real-model duties remain current. There are 76 partial mechanical
rows and four background rows. None certifies product conformance or release.
The historical release ledger and previous package ledgers remain unchanged.

## Source and evidence identity

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Original package digest | `81e65f75d0be884021e2fbf9fc2a13a99613e2bd0cd40d52e6a26b8a3c63cfab` |
| ADR006 manifest | 385378 bytes; SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `aece4cdba5e23a174caf8c4ee1f1b6ae65679bd3b17bd0274304446d21154d65` |
| Audited code base | `dd18b3e9a918afa20065d25fb1f6c98110343238` |
| Technical baseline SHA256 | `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

Admission clarification is comment `01a16faf-e9eb-71fc-97a3-ba9ad52b238e` on
ERII-242. The parent seal receipt is `01a16e21-9bcb-7ae6-bf4a-4e466fdfbe5b`;
independent identity verification is `01a16e27-7505-7a40-bd57-3f4b287bc177`.
Both package digests are independently pinned in the maintained validator.
Strong mode checks all 1588 current manifest assignments, unique identities,
current source and reference bytes, native files and Git ancestry. A content
change or ambiguous match rejects and requires coordinator resealing.

The 86 current references bind exact file bytes at the audited base. They index
implementation and maintained tests; an indexed test is not necessarily executed.
Metadata for 22 actual merged PRs is reused from retained preceding ledgers,
then strong mode verifies each merge commit's ancestry and each current blob.
No historical ZIP was downloaded. Merge provenance is separate from behavior
and independent acceptance evidence.

The configuration fence is one source block, with **28 full-key duties and
mappings** to the current YAML, environment, database or fixed implementation.
The 14 minimum-interface rows each retain a method/responsibility mapping.
`ActorStore.Claim`, concrete Store methods and transactional package functions
are distinguished from the literal consumer interfaces. Names alone do not
establish either a functional failure or an approved architecture deviation.

## Maintained verification

```sh
make audit-baseline-12
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load frozen sources and the authored ledger offline. Drive uses
the existing `release_audit.py` CLI. Evidence is `evidence/baseline-12.json`;
inspect `package_audit.reviews`, `config_mapping`, `interface_mapping`, remaining
routes and observations. Cleanup removes disposable output only. Regression
checks reject missing/duplicate/extra/drifted blocks, missing configuration keys
or interface mappings, altered original locations/hashes, stale references,
unsafe native paths, unmerged provenance, missing mandatory routes and invented
PASS. Failed reruns remove stale disposable reports.

The CLI with `--require-complete` still exits **2**, with `status=inconclusive`,
`release_ready=false` and zero verified requirements. Completing this audit
does not satisfy the release validator's unmet source, evaluation or human gates.

Same-host read-only strong verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-12.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-12-strong.json
```

Only this mode claims manifest/native-byte/full-Git checks. Shallow CI cannot
check unavailable history or same-host files. Independent review must assess
semantic completeness of authored duties; hash integrity cannot establish it.

## Native observations and limits

Commands, source SHA, timestamps, exit codes, sizes and SHA256 remain under the
registered durable root in
`ERII-242/dd18b3e9a918afa20065d25fb1f6c98110343238/audit-20261025/`.
Package setup used the maintained offline entry point and native npm cache;
mutable `node_modules` were not shared. No owner credential, actual model
account or macOS Guest was accessed.

| Observation | Observed result and limit |
| --- | --- |
| Selected Go race contracts | 120 passing tests/subtests, six packages, zero skips. Covers Host error normalization, strict configuration, Code admission/replay/store/wake/old owner, Main/Root shutdown, delivery, Child boundaries and Event Outbox/Inbox. This is a selected inventory, not the full race suite or protected five-run series. |
| Delivery CLI | Maintained CLI/PTY harness runs 20 actual SIGKILL/restart windows around Conversation append, Delivery enqueue, Checkpoint, transport and acknowledgement. Covers retries, ambiguous/query-only unknown, approval allow/deny, provider-free delivery and independent Fake receipts. Scripted replies carry no semantic score. |
| Code recovery CLI | Actual Sidecar SIGKILL, Go SIGKILL and SIGTERM at 37 completed plus eight accepted of 100 calls. Reuses the 37 completed provider generations, reattaches all 45 accepted identities, completes 100 Children, checks suspended VM release and cancelled-tree recovery. One run of three variants is not the original protected five-run series. |
| Startup CLI | Source and native archive install, real PTY first setup/chat, persistent local Broker crash/restart, reinstallation and cleanup. Uses an explicit Fake Bridge. It does not prove actual ChatGPT Browser login, the literal 8000/v1 endpoint or current Web/Guest compatibility. |
| Synchronous error probe | An ephemeral Go driver calls the maintained `Sidecar.Start` with a harmless Host fixture returning `tool.Failure(PROVIDER_UNAVAILABLE)`. Actual QuickJS `load()` catch returns `code=PROVIDER_UNAVAILABLE`, `retryableType=undefined`. The asynchronous path copies error properties. The probe source is retained beside raw output; it is a counterexample, not a PASS script or a production change. |

All maintained harnesses join owned children/PTYs and remove disposable state.
The temporary probe starts no external service and Sidecar.Start joins its owned
process. No full release suite, current L2, actual model semantics, protected
five-run series or named human review is claimed.

## Finite gap routes

Three new unassigned Backlog proposals retain exact source hashes and independent
exit conditions. Deduplication inspected all 74 existing parent children before
creation. No other Agent was dispatched. Existing semantic owners are reused
within their frozen responsibilities; this PR implements no product repair.

| Route | Remaining scope |
| --- | --- |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | 当前release-head机械/安全/恢复/CI及独立review最终join；未满足保持unmet。 |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | 实际模型错误处置、Main委派/汇总、Provider兼容与原完整双臂评测。 |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | 当前真实登录、Web/Guest/渠道与具名人审；Fake不能替代。 |
| [ERII-362](https://multica.ai/erii-studio/issues/01a16fb4-2bd8-720a-8d4b-a5d59c245604) | 同步Store Host Error缺少boolean retryable及可选安全details；限定同步边界。 |
| [ERII-363](https://multica.ai/erii-studio/issues/01a16fb4-2ec5-7050-a0f7-c4b2c4d46245) | 28个完整配置键与YAML/环境/DB/固定策略的逐项处置；不默许同名外的等价性。 |
| [ERII-364](https://multica.ai/erii-studio/issues/01a16fb4-318c-746b-afef-7dc620a43eff) | 14行逐方法消费接口与Fake合同；不为满足名字加空包装。 |
| [ERII-355](https://multica.ai/erii-studio/issues/01a16f63-a273-7eb8-872c-3c581d91144b) | Worker进度写入与只读活动展示之间的缺口。 |
| [ERII-344](https://multica.ai/erii-studio/issues/01a16ef3-47a8-7ad5-a3ef-3fb5975a05a8) | 可配置累计Code Host Call和1000项透明排队；pending=128不是累计上限。 |
| [ERII-336](https://multica.ai/erii-studio/issues/01a16dd9-72c5-702d-8049-2d3df3556ef2) | Code Mode发现输出的TypeScript声明。 |
| [ERII-338](https://multica.ai/erii-studio/issues/01a16dfe-85d9-757b-9e52-2d0de021e380) | RPC heartbeat/事件身份/interrupted/Outcome；不从重放成功推协议全满足。 |
| [ERII-360](https://multica.ai/erii-studio/issues/01a16f97-7ec3-760d-a825-29978b811f82) | Active Profile降级后保留引用与字面ready不变量的差异。 |
| [ERII-357](https://multica.ai/erii-studio/issues/01a16f68-a44e-7b55-99e4-6ea2a800addc) | 分布式追踪与本地startup/database span的边界。 |
| [ERII-359](https://multica.ai/erii-studio/issues/01a16f97-7bed-7ca3-82fc-3a585d2da78e) | 旧Main撤销中与queued successor共存的唯一性合同。 |
| [ERII-351](https://multica.ai/erii-studio/issues/01a16f1a-5c83-794f-87df-07fefac2e3c3) | Provider生命周期/状态/能力与ComputerUseRuntime职责。 |
| [ERII-343](https://multica.ai/erii-studio/issues/01a16ef3-43c0-78cd-977f-840dce140018) | Code cancel execution_epoch原子失效与两阶段收敛。 |

Each remaining duty retains its precise route and proof limit in the ledger.
New proposals require coordinator registration/admission before implementation.
Neither a route being Done nor this audit PR being merged certifies v1 release.
