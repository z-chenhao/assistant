# Frozen baseline-10 audit (ERII-240)

[baseline-10.json](baseline-10.json) disposes of **80 exact source blocks** from
technical baseline §11.6 original L1729 through §14.2 original L1880. ADR006 moves
them to L1739–1890 with unchanged section/content identities and order. The scope
covers remaining Memory maintenance duties, EventQueue, authoritative state,
Observability, user progress, technology choices and module organization.

The ledger retains **318 sub-obligations**: 281 required-v1, 13 policy, 15
background and nine explicitly deferred. All **294 mandatory/policy duties stay
routed**. There are 70 partial mechanical rows, one evidence-gap row, five policy
rows and four background rows. Table formatting and example fences remain
inspectable. Mixed future migrations never exempt current storage or adapter
duties. No requirement is certified, historical partial/gap dispositions remain
unchanged, and no semantic score is assigned.

## Identity and provenance

| Input | Identity |
| --- | --- |
| Original manifest | 380773 bytes; SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556` |
| Original baseline | `d11e543ea32efdf784478f5959ea6565af87b646` |
| Original package digest | `d312f7fa24d257858397fcee94246eb2c01910fb249ec971c7a874829c31ef0f` |
| ADR006 manifest | 385378 bytes; SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `33ebc44545bf9d3718771a4303266cca26f1923c1f234ee2e30474b5067fa911` |
| Audited code base | `4ea6156fda1065e29ec746718c8a24c1117d8f9c` |
| Technical baseline file | SHA256 `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

Coordinator comment `01a16f5d-78c8-7945-b674-0d22a9060ce6` authorizes consuming
the existing ADR006 seal. Parent reseal and independent-check receipts remain
`01a16e21-9bcb-7ae6-bf4a-4e466fdfbe5b` and
`01a16e27-7505-7a40-bd57-3f4b287bc177`. The validator pins both package digests
independently of authored dispositions. Strong mode checks the full 1588-block
assignment exactly once. Changed/ambiguous source content rejects and requires
coordinator resealing; line shifts need a unique section/content hash.

Each row binds full text, original/sealed/current locations, scoped duties,
current implementation/test references, observations, proof limits and remaining
routes. The 87 current file references bind audited bytes. An indexed test is
navigation evidence unless a native observation records its execution. The 17
merged PRs are 2/6/8/10/12/14/16/18/22/23/25/26/29/31/33/35/45. Full local Git
ancestry and audited blobs were checked. Merge provenance does not certify a
requirement. Previous package ledgers and the ERII-30 ledger remain unchanged.

## Maintained verification

```sh
make audit-baseline-10
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load the exact sources and ledger offline. Drive invokes the
existing `release_audit.py` CLI. Evidence is `evidence/baseline-10.json`; inspect
`package_audit.reviews`, routes and observations. Cleanup removes disposable
reports only. Inherited corruption tests reject missing, duplicated, extra,
changed or ambiguous blocks, stale references, unmerged PRs, unsafe paths,
native-byte drift and invented PASS. Package regressions protect both seals,
original locations, mandatory routes and the distinct lifecycle/progress/layout/
trace gaps. Failed reruns remove stale output. `--require-complete` still exits
**2**; completing this audit is separate from release acceptance.

Same-host read-only strong verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-10.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-10-strong.json
```

Only this mode sets `manifest_checked`, `native_bytes_checked` and
`git_provenance_checked` true. Shallow CI does not claim unavailable full-history
or same-host evidence. Hashes prove integrity; independent review must judge the
semantic completeness of every authored duty.

## Observations and limits

Full raw evidence remains in the registered durable root under
`ERII-240/4ea6156fda1065e29ec746718c8a24c1117d8f9c/audit-20261025/`.
Receipts retain actual commands, source SHA, timestamps, exits, sizes and hashes.
CLI observations bind the built binary. Locked offline setup reused native
caches. No historical ZIP, owner credential or live VM was accessed.

| Observation | Result and practical limit |
| --- | --- |
| Focused Go race contracts | 53 passing tests/subtests in selected EventQueue, Memory, Task and SQLite paths. Covers transaction rollback, Inbox/ack ambiguity, concurrent deduplication, actual disk-backed JetStream wire format/restart, poison isolation, claim fences, Memory receipts and read-only progress. Two packages have no tests; the binary-dependent production lifecycle case initially skipped and was then run separately with the built binary. This is not the full race inventory. |
| Production event lifecycle | A separate real-binary race invocation supplies the maintained fixture configuration. Checks startup order, real JetStream and QuickJS handshake, authoritative recovery, broker outage/reconnect, Inbox deduplication and SIGTERM claim release. Deterministic model peer; no current cloud deployment. |
| Secret and local spans | Three passing race tests cover explicit environment-reference rejection, correlated local spans/level filtering and bounded concurrent JSON log rotation. No owner Keychain round trip or cross-process trace propagation is claimed. |
| Memory CLI | Twelve maintained scenarios cover observation, explicit controls, encryption, Worker refusal, commit/restart, Forget barriers, new-source lineage, opt-out and immutable history. They do not implement automatic demotion/eviction or establish different-text semantic conflict resolution. |
| Delivery CLI | Twenty real SIGKILL/restart scenarios through CLI/PTY cover reply/intent/enqueue/checkpoint/send/ack boundaries. Additional cases cover transient retry, accepted ambiguity, query-only unknown, approve/deny and duplicate callbacks. Ordinary progress creates no delivery. No actual channel account or semantic quality claim. |
| Operations CLI | Real CLI health/trace/report projections, sensitive sentinel scan, SIGKILL lock release, encrypted backup/restore, frozen approval and duplicate-drain stability. Metrics are bounded count projections; local diagnostic spans are distinct from durable RunTrace and full distributed tracing. |
| Source inspection | Read-only callers and module inventory finds zero production `.Progress(...)` callers; current calls are tests. Curator source has observe/remember/forget and promotion but no automatic demotion/eviction procedure. The engine tree places SecretStore under `llm`, and `Get` returns bytes. These are scoped source findings, not runtime product PASS or a proof of credential leakage. |

The initial combined Go command used the nonexistent baseline directory
`internal/secret/...` and exited 1. Its log/receipt is retained. The corrected
current-path contracts and a separate `internal/llm/secret` invocation passed;
the failed command is never reported as a successful observation.

All CLI harnesses report joining owned processes/PTYs/peers and removing disposable
state. The production event test owns its isolated broker and temporary root.
No complete release suite, protected five-run series, actual-model quality,
current Guest compatibility or named human review is claimed. Output stays
`inconclusive`, `release_ready=false`, `verified_requirements=0`.

## Finite gap routes

| Route | Remaining scope and independent exit |
| --- | --- |
| [ERII-354](https://multica.ai/erii-studio/issues/01a16f63-9fb3-78d6-95f9-a5b3b51fdef2) | Original L1731/L1738: implement or explicitly resolve automatic demotion/eviction with frozen policy, immutable provenance, Forget/privacy barriers, migration/CAS/race and kill/restart evidence. Explicit Forget cannot stand in for background aging. |
| [ERII-355](https://multica.ai/erii-studio/issues/01a16f63-a273-7eb8-872c-3c581d91144b) | Original L1797/L1798/L1800: connect meaningful production Worker updates to Progress Store and read-only presentation, including current stage and completed/total fields. Prove no notification, Main CustomMessage, trace leakage or stale-owner writes. |
| [ERII-356](https://multica.ai/erii-studio/issues/01a16f63-a532-743c-a2cf-034c08160575) | Original L1830/L1879/L1880: reconcile the normative module tree, llm/secret ownership and controlled Credential contract with current nested packages/byte interface. Example package names alone require no wrappers. Any accepted deviation needs owner ADR and coordinator reseal. |
| [ERII-357](https://multica.ai/erii-studio/issues/01a16f68-a44e-7b55-99e4-6ea2a800addc) | Original L1791: reconcile or implement distributed tracing across the actual process boundary. Preserve private Run/Tool correlation, failure/cancel/restart/concurrency and redaction; local startup spans cannot silently certify cross-process propagation. |
| [ERII-353](https://multica.ai/erii-studio/issues/01a16f33-2af9-73ad-af32-b0c29774632d) | Reuse the preceding package's different-text fact conflict repair; current serial/fingerprint handling and explicit supersedes do not complete automatic semantic merging. |
| [ERII-335](https://multica.ai/erii-studio/issues/01a16dd9-6f96-7e35-88a3-c5f794a8e958) | Native Bridge permission and Secret RPC proof; directory separation alone is insufficient. |
| [ERII-340](https://multica.ai/erii-studio/issues/01a16e1c-85c6-7b24-8ca6-89cca9f6caa5) | Frozen idempotency-key derivation and replay compatibility; do not silently normalize the observed specification/implementation difference. |
| [ERII-342](https://multica.ai/erii-studio/issues/01a16e46-7f12-7af5-b0f8-c5070324cf06) | Existing §3.8/§3.11 domain API and Context Builder naming reconciliation. This task does not own the AgentService dependency. |
| [ERII-345](https://multica.ai/erii-studio/issues/01a16ef3-4aea-7e6d-93a7-bf158e28d3ff) | Code Mode persistence/ownership and dependency mapping. The §14.2 AgentService duty reuses this task's frozen §6.6.8 duty; no duplicate loop or empty wrapper. |
| [ERII-351](https://multica.ai/erii-studio/issues/01a16f1a-5c83-794f-87df-07fefac2e3c3) | Existing ComputerProvider lifecycle/state/capability responsibility reconciliation. |
| [ERII-352](https://multica.ai/erii-studio/issues/01a16f33-2755-76d6-a96f-5ac3e3e8f661) | Existing frozen token-budget versus byte-estimation threshold repair; current context duties remain unmet until actual proof. |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Actual-model Memory evidence/recall judgment, Main failure interpretation and meaningful progress, model/ChatGPT compatibility; original frozen dual-arm evaluations and independent Judge receipts. |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | Current approved Guest/channels and named human observations, exact approval, native state/Artifact and cleanup; deterministic peers do not substitute. |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Final current-release per-duty implementation, migration, replay, race, protected-five-run, native CI and independent-review join. Remaining duties stay unmet; discovered faults require finite repair rather than changing this audit to verified. |

The §14.2 current L1884 / original L1874 duty “Code Mode through AgentService
creates only Child Runs” routes to ERII-345, whose existing frozen scope includes
§6.6.8 current L1203 / original L1193, SHA256
`86b2574208ec60f8111bee492a57dd166a9c1e4b04571b0ea3bb9bae35715548`.
That block in [baseline-07](baseline-07.json) explicitly requires the same
AgentService Child-creation dependency. The remaining duty records this identity
and its peer obligation in `scope_reuse`; it does not add a source block to this
80-block audit or enlarge either downstream task's contract. The regression
checks the exact route against that frozen peer duty and all documented issue
links against the ledger's canonical routes.

The four new records are unassigned Backlog proposals awaiting coordinator
registration/admission. Deduplication inspected all 64 existing parent children
and then the newly proposed scopes. Finishing this package or a repair does not
certify v1 release acceptance.
