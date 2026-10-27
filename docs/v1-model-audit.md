# Model connection audit — ERII-30

This is a bounded individual review of baseline §5.2, §5.3 and the seven
normative bullets in §5.6. It does not certify model compatibility or release
acceptance. Each source block has its own line, digest, observed assertions,
remaining obligations and code/test/evidence references in the
[release ledger](v1-release-ledger.json). The existing §5 snapshot review also
includes the named Main/Child profile-switch regressions. No normative source,
locked evaluation fixture, score or product protocol changes in this increment.

## Observed boundaries

| Source | Code and executed evidence | Limits of that evidence |
| --- | --- | --- |
| §5.2 profile, snapshot and credentials | Typed model validation; profile/config CAS and atomic Outbox; immutable snapshot and endpoint readback after edit/restart; auth failure/rollback and synthetic-secret redaction tests | The schema fence contains multiple obligations. These checks do not certify every field, real credential backend, account refresh or all provider metadata. |
| §5.2 request consistency and limits | Main profile switch during generation; Child admission after profile switch; frozen context-length retry without another Tool execution; exact Host activation bounds | Named Go tests use deterministic providers. They do not establish a real multi-provider Root tree across every recovery boundary. Above-limit diagnostic metadata deliberately remains readable. |
| §5.3 local connection and Probe | Real Go CLI → Node/Pi → owned HTTP peer: discover/add/test/select, restart, Tool continuation, manual ID without catalog, optional API key, failed retest closing ingress | The endpoint is scripted. No actual Qwen, vLLM, SGLang or LM Studio compatibility, TLS deployment or model quality is inferred. Local Responses remains rejected before network access. |
| §5.3 conservative optional capabilities | Activation retains observed text/Tool/streaming and explicitly declared, observed image capability; unsupported sampling/reasoning/wire contracts reject before network | There is no positive reasoning/sampling compatibility Probe. Unsupported optional settings are disabled or rejected, not certified. |
| §5.3 image proof | CLI-selected image profile sends screenshot pixels only after explicit selection; selected reference counts `[0,0,1]`; text-only wire counts `[0,0,0]`; failed image retest degrades readiness | Computer mechanics use the maintained Fake. Synthetic color responses and transport checks do not prove a real model can interpret screenshots. |
| §5.3 untrusted HTTP | Node contracts inspect headers and requests; redirect refusal, bounded retries, malformed/partial/oversized stream rejection, safe errors and empty stderr; a separate cross-origin HTTP receiver gets zero requests from keyed catalog and generation calls | The cross-origin regression covers a 307 between two owned loopback origins. It does not establish all live proxy/TLS configurations or every redirect status. |
| §5.6 fault and replay | Profile switches affect new Main/Child admissions; endpoint/usage survive restart; missing usage stays unknown; context-length retry preserves snapshot and Tool results | No live outage/recovery or complete same-condition local/account comparison. This tranche does not establish host/GPU resource accounting or every four-zero-rate configuration path. |

All dispositions remain `partial_mechanical` or `evidence_gap`.
`individually_verified_requirements` stays zero, `release_ready` stays false,
and all semantic scores remain null. An indexed block is not a passed clause.
The separate §5.1 Wizard, §5.4 account lifecycle, §5.5 Bridge isolation and
§5.6 reference-link blocks remain outside this tranche.

## Maintained reproduction

Use the repository's pinned Node and Go versions. The recorded local dependency
install used the package-manager cache with `--offline`; no shared mutable
`node_modules` or owner account was used. Build the normal binary and drive the
maintained complete local-provider command:

```sh
make build
npm test --prefix provider-bridge
python3 scripts/verify_local_provider.py --binary bin/erii --evidence evidence/model-audit/local-provider.json
go test ./internal/llm/bridge -count=1 -json
python3 -m unittest discover -s scripts -p 'test_release_audit.py'
python3 scripts/check_docs.py
python3 scripts/release_audit.py --evidence evidence/model-audit/release-audit.json
python3 scripts/release_audit.py --require-complete --evidence evidence/model-audit/incomplete.json
```

The final command must exit **2**, preserving the incomplete release result.
The cross-origin test uses the existing Bridge fixture plus a second owned
HTTP server. It verifies the configured origin receives the explicit key,
catalog and generation each make one attempt, neither completes through a
redirect, and the other origin receives no request. A disposable mutation from
`redirect: 'manual'` to `redirect: 'follow'` must fail this named test; its log
and exit code are retained separately from passing checks. The temporary copy
is removed without changing the production Bridge.

The local-provider command includes the existing screenshot projection and
GC-11 driver contract. Its scripted GC-11 receipt remains ungraded. Its cleanup
receipt records joined CLI children, stopped HTTP peers and removed disposable
state; it does not retain or touch a user installation.

The Issue evidence manifest records the exact focused Go test expression,
source/binary identity, package results and five named race tests:

- `TestConcurrentProfileCASAcrossConnections`
- `TestActivationConcurrentConfigCAS`
- `TestChildFrozenModelAndCapabilitiesAfterProfileSwitch`
- `TestLoopSteeringAtBoundaryAndProfileSwitchOnlyNewRuns`
- `TestContextLengthRetryKeepsSnapshotAndDoesNotReplayTools`

This audit and test-only tranche uses these proportional checks. Earlier full
`make test`, `make test-race`, `make lint` and `make verify` receipts retain their
original SHA; they are not relabeled as reruns for this audit.

## Remaining release work

Current L2, real login, named human review, all semantic/protected acceptance,
local/account comparison, remaining whole-case/long-running variants and the
individual normative backlog are still due. The unresolved Task default-mode
conflict and the prior opaque Ubuntu `run storage unavailable` failure remain
release gaps. See the [release audit](v1-release-audit.md) and
[live preflight](v1-live-preflight.md). No checkpoint PASS can waive them.
