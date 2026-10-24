# Frozen baseline-08 audit (ERII-238)

[baseline-08.json](baseline-08.json) records **80 exact source blocks**, from
technical baseline §8 original L1249 through §9.6.5 original L1516. The existing
ADR006 seal moves them to L1259–1526 without changing content identities. The
402 authored duties cover the remaining Security clauses, logical Computer and
Instance, durable lifecycle, image compatibility, three Computer Use layers,
Inventory, Surface observations, App/Browser/Tab, waits and the first standard
Capability. The clause after the final table row is outside this package.

There are 56 partial mechanical rows, 13 evidence gaps, five policy rows, four
background rows and two deferred rows. Duties retain 366 required-v1, 18 policy,
seven background and 11 explicitly optional/future statements. All **384
required/policy duties remain routed**. Warm memory snapshots and future cloud
Providers do not exempt Durable recovery or current safety. Optional advanced
Security modes do not require a new configuration mechanism. The financial
clause preserves explicit confirmation for each real transaction while deferring
integration-specific rules; this audit does not add financial capabilities.
No requirement is certified, and no semantic score is awarded.

## Identity and provenance

| Input | Identity |
| --- | --- |
| Original manifest | SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`; 380773 bytes |
| Original baseline | `d11e543ea32efdf784478f5959ea6565af87b646` |
| Original package digest | `22f68315002ca724f5ce132ee02b9a8c1678f19d4decc3101c42bfa541cb30ee` |
| ADR006 manifest | SHA256 `f26df8fbb927746198612c9f5aa8921df6c51274953bff0cc0b0a83c20e93837` |
| Sealed package digest | `12528dae09407c3daa20caf9c1d1c5166920d70b50a3346b4ab875cd93d077e8` |
| Audited code base | `8855d64cf9c2ea154a02404e44229545c963eb09` |
| Technical baseline file | SHA256 `bbb7e5db3f0bd6aa672e8704e7329d0ecd6d067769c9e49bf3997af5d9268a67` |

The ledger keeps both exact package lists, original/current lines and full text.
The existing validator pins both package digests independently. Strong checking
compares the entire 1588-block sealed assignment for uniqueness and current
source coverage. Coordinator comment `01a16f0a-e895-7146-8f4a-e636d96066a2`
authorizes consuming the existing seal, whose parent receipts are
`01a16e21-9bcb-7ae6-bf4a-4e466fdfbe5b` and
`01a16e27-7505-7a40-bd57-3f4b287bc177`. No new resealing or approved deviation
is inferred here. Changed or ambiguous source identities reject.

Each row binds authored duties and scopes, current code/test/harness bytes,
merged PR provenance, observations, limits and exact remaining routes. The
41 references are navigation evidence, not a claim that every indexed test ran.
The eight actual merged PRs are 12/15/16/18/20/23/25/26; audited blobs and merge
ancestry were checked locally. The historical ERII-30 ledger and all preceding
package ledgers remain unchanged. Audit progress does not erase their gaps.

## Maintained verification

```sh
make audit-baseline-08
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

Launch/Doctor load the ledger and current sources without accounts, network or
VM. Drive invokes the existing `release_audit.py` CLI. Evidence is
`evidence/baseline-08.json`; inspect `package_audit.reviews`, routes, scopes and
observation receipts. Cleanup removes disposable reports only. The consumer
rejects missing, duplicate, changed or out-of-package blocks, stale references,
original-line drift, unrouted mandatory duties, unexplained exemptions and
invented PASS. Invalid reruns remove stale output; output cannot overlap inputs
or retained native evidence. `--require-complete` still exits **2**.

Read-only strong verification uses independently retained inputs:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-08.json \
  --work-packages /absolute/durable/work-packages-adr006-20261024.json \
  --native-root /absolute/durable/audit-20261025 \
  --verify-git --evidence evidence/baseline-08-strong.json
```

`manifest_checked`, `native_bytes_checked` and `git_provenance_checked` are true
only when those optional inputs were checked. Normal shallow CI does not claim
to verify unavailable history or same-host files. Hashes prove integrity;
independent review must judge semantic completeness of the authored duties.

## Observed evidence and limits

Full raw evidence remains under the registered durable root:
`ERII-238/8855d64cf9c2ea154a02404e44229545c963eb09/audit-20261025/`.
Receipts retain actual commands, source SHA, timestamps, exits, sizes and hashes.
CLI receipts bind the locally built binary. Environment versions and successful
offline dependency setup are retained. No historical ZIP was downloaded.

| Observation | Result and practical limit |
| --- | --- |
| Focused Go race contracts | 86 passing tests/subtests, no skips, across selected Computer, macOS adapter/peer and SQLite Tool/approval contracts. Includes whole-unit publication process death, corrupt checkpoints, compatibility rejection, state CAS/upgrade, read barriers and stale owners. This is not the full Go race inventory. |
| Durable Computer CLI | Seven check groups: five repeated checkpoint/restore observations followed by active-claim recovery boundaries; stable Computer identity, increasing epochs and unchanged disk/NVRAM/config/Artifact bytes. Four SIGKILL boundaries and two Task cancellation boundaries retain frozen cleanup identity. HTTP/SSH peers represent external dependencies. These are not five complete protected release runs or a real Host reboot. |
| Computer Use CLI | Eight groups cover Inventory/binding, independent observations, action/reobserve/stale refs/wait timeout, separately approved file import/overwrite/export/upload, registered files across cells/instances, cross-run refusal, navigation expiry, denied input and path traversal. Chosen screenshot Artifact alone enters model context; other screenshots remain internal. |
| Real local Chromium | Eight tests passed: seven CDP readiness boundaries plus a real browser contract covering Inventory, locators, input, navigation expiry, download and controlled chooser. Browser/temporary state cleanup completed. Host Chromium is not the current macOS Guest runtime. |
| Real QuickJS | All 19 maintained tests passed, including forbidden globals, fresh cells, synchronous Store and nested Surface Handle persistence refusal, private bridge tokens and structured CUA errors. Does not prove every public argument is honored. |

Both CLI harnesses report cleanup of owned processes and disposable state.
Memory maintenance is indexed statically, without a new execution claim.
The retained ERII-229 `report.md` and `manifest.json` were read and hashed as
historical input; `historical_inputs` records their exact identities. Their
readiness interruption, missing browser fixture and absent human-review result
remain gaps. They are not a current native observation and are not certified by
`native_bytes_checked`. The stopped clone and interrupted ledger were untouched.

No live model/account/Guest compatibility, full current Jianying acceptance,
semantic quality, named human review, complete performance baseline, complete
release suite or five consecutive protected runs is claimed. Output stays
`inconclusive`, `release_ready=false`, `verified_requirements=0`.

## Finite gap routes

| Route | Exact scope and independent exit |
| --- | --- |
| [ERII-347](https://multica.ai/erii-studio/issues/01a16f0e-9047-71b1-8d54-b5d5ff158cdd) | Original L1252/L1270/L1306/L1331: ordinary `SSHGuest.Ready` still invokes `permissions grant`. Separate read-only readiness from explicitly authorized provisioning; prove existing permissions pass, missing permissions refuse without grant, and signature/login/failure/cancel/restart retain the boundary. Do not resume ERII-229's interrupted work. |
| [ERII-348](https://multica.ai/erii-studio/issues/01a16f0e-934e-72d5-bb40-ac815bf49005) | Original L1269/L1316/L1372/L1516: current Guest synthetic browser runtime/owned marker/LaunchAgents are missing. Reuse maintained deployment/Doctor/Drive/Evidence/Cleanup, fix identities/hashes and reject partial/wrong fixtures. Exit needs approved current Guest native observations; Host Chromium cannot substitute. |
| [ERII-349](https://multica.ai/erii-studio/issues/01a16f0e-9633-7a2e-ad59-5daf19a28f93) | Original L1339/L1382/L1435/L1443/L1470: `getTab` browser option and some creation/pointer/input options are ignored or narrowed; `paste` rejects md/html, key duration is not forwarded and optional methods lack precise absence disclosure. Map every signature through the actual facade/Host/Driver, settle undefined option semantics, and preserve return/compatibility evidence. A bare numeric index binds the latest observation and cannot carry old revision identity; resolve the original old-index wording explicitly. Protocol changes need owner ADR and coordinator reseal. |
| [ERII-350](https://multica.ai/erii-studio/issues/01a16f0e-9950-7853-8af7-d9ee9c2c342a) | Original L1471–1472: new Tabs have execution Handles but no observed Run-owned temporary Tab retention/cleanup policy or explicit cross-Run stable-reference retention. Implement approved Resource Policy with user-Tab protection, ownership, cancellation, unknown close outcomes and restart evidence. Whole-browser disk persistence is insufficient. |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Original frozen dual-arm real-model choices and judgments: structured Tools before GUI, AX/semantic actions before coordinates, result verification and interpreting new observations. Native state and independent Judge remain required. |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | Current isolated Guest/clean base/fixed Driver permissions, actual lifecycle and Browser, all Jianying compatibility groups, UI progress/privacy and named human review. Consume finite repairs; preserve unmet external conditions and actual cleanup evidence. |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Final per-duty current-head contract/merged PR/native CI/migration/race/kill-restart/approval/protected-five-run and independent-review join. Unobserved duties remain unmet; an implementation fault needs a finite repair, not a report-generated PASS. |

The four new records are Backlog proposals, unassigned and awaiting coordinator
registration/admission. Their sources, exclusions, semantic deduplication and
independent exits are retained in each record. The audit changes neither
Computer behavior nor release gates. Completing this package or a gap child
does not certify v1 release.
