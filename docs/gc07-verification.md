# GC-07 network journey verification

ERII-253 owns the 20 original GC-07 source blocks in the owner-frozen
`replan-work-packages.json` (SHA256
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`).
The block identities and original Golden/M6/scoring bytes are pinned in
[`gc07-v1.lock.json`](../evals/fixtures/gc07-v1.lock.json). No original input,
assertion, scoring policy or variant changes.

```sh
make setup
make build
python3 scripts/verify_gc07.py --binary bin/erii --evidence evidence/gc07.json
python3 scripts/verify_gc07.py --binary bin/erii --evidence evidence/gc07.json --validate
```

Launch creates isolated stores, an explicit selected Fake profile, loopback
embedding/Lume/SSH peers and real Chromium. Doctor checks the frozen source,
model readiness, sealed VM fixture base and browser publication. The existing
production CLI, TUI, Main/Task/Root/Child, Memory, QuickJS, ToolExecutor, Security,
Computer and Artifact boundaries perform the work. The external peer never
writes domain stores. Case clock/timezone are fixture data; Host leases use
wall time. Cleanup joins owned processes and removes disposable state.

All six original variants (three wordings, two noise, one reordered background)
run five consecutive times. CI uses the maintained fixed `wording` and `noise`
parts, each holding three whole five-round series. `--ci-part` binds native
validation to that exact 15-row selection; full validation still requires all
30 rows. The fail-closed Delivery join requires both parts and all original
control rounds on the same source, binary, fixtures and workflow attempt.
Separate explicit scripted plans leave each original
user event intact. Main/TUI receive the two original turns, with observation
between diagnosis and the authorization turn. Scripted choices are not model
quality evidence.

| Original obligation | Mechanical observation |
| --- | --- |
| Current topology and device configuration | A stale topology is superseded through Memory; Root and Child use the same scoped recall. The page exposes the original ISP/bandwidth/router/DHCP/DNS/port/device list, Ethernet/interface IP and disconnected physical link. |
| Observe before action; impact and approval | Actual AX/screenshot and scoped DOM reads precede clicks. Buttons describe gateway interruption. Frozen approvals bind navigation and each click; denied repair has zero attempts and no external change. |
| Verification, rollback and clean configuration | A failed repair leaves the gateway offline and two protocols. Explicit approved rollback restores the original state. Repair with a lost transport acknowledgement is reconciled through a new read: gateway, ordinary website, proxy and Apple TV paths all pass, one protocol remains. The external action ledger has exactly failure, rollback and repair. |
| Secrets and shared desktop | Synthetic credential/key sentinels are absent from persisted Memory/Trace/Artifacts. Independent Task observation waits at the desktop barrier with zero attempts; it resumes after cancellation releases the old owner. Child capability reduction excludes Task/Memory control. |
| No fabricated restoration | `UNKNOWN_SIDE_EFFECT` stays unknown with one attempt. The report distinguishes sandbox state readback from provider acknowledgement and live verification. Original disconnected/failure observations never report restored state. |
| Recovery and delivery | SIGKILL after generation commit and after the unknown effect/Tool commit reuses completed calls; duplicate input preserves native state and action ledger. Old cancelled Task epoch cannot write or revive. Screenshots and JSON report reopen through the CLI; native Tool returns, Artifact bytes/hashes, Task/Run tree and Main delivery intents are retained. |

The public JSON is a small hash/size-bound manifest. Complete native evidence,
including actual Artifact bytes, is streamed losslessly into gzip beside it in
`.gc07-native/`; compressed hash/size and bounded decompressed size are checked; the pinned
CI uploader excludes that hidden directory. Validation requires native bytes,
recomputes assertions and corruption probes, and rejects missing/duplicate/
reordered rounds, changed input/source/binary, failed cleanup, false grades,
contradictory native returns, stale topology, bad rollback, denied effects,
unknown retries, revived owners and changed Artifact bytes. Hashes bind bytes;
they do not establish independent trust. Reviewer uses a clean exact-SHA checkout.

Task creation uses the maintained Recall publication barriers: freeze Root
Recall, publish Main, then dispatch Root. The desktop probe keeps both Tasks
and cancellation inside one foreground Actor, with separate observed barriers
for each Task. This preserves the existing fail-closed concurrent-pending-append
protocol; it adds no storage retry or runtime change. The maintained Recall
conformance gate covers the rejected ordering. A forced GC-07 reproduction
retains the same storage rejection with one pending assistant vector and zero
Worker model requests. Successful journey evidence describes the controlled
legal ordering, rather than success under every interleaving.

Unexpected failures retain full compressed native state locally and publish a
bounded structural summary (under 32 KiB) with Run/claim/Code/Computer states.
It excludes raw model requests and Tool result contents, so a failed hosted run
remains diagnosable within the transfer budget.

Registering this gate changes `verify_suite.py`. The `baseline-13` and
`golden-common-02` reference-maintenance envelopes explicitly refresh that
committed file identity through `maintain_audit_references.py`. Their original
content, frozen blocks, assertions, thresholds and historical observations are
unchanged. This reference-only refresh supplies no new native PASS.

The result is `mechanical_pass`, with `semantic_score: null`,
`eligible_for_grading: false` and `release_ready: false`. The maintained
`unscripted_cases.py freeze --case GC-07` entry supplies original user inputs
only. Its conversation fragment still requires an approved device/network
adapter before real-model diagnostics can be evaluated. It never receives this
harness's plans or expected answers.

ERII-223 retains semantic quality, constrained grading, actual macOS/home-network
and current L2 compatibility, approved live environments/profiles and human
review. No live account is borrowed and no unexecuted check passes. The fixture
only establishes synthetic Ethernet/network state through real Chromium and the
production browser adapter, with explicit Fake Lume/SSH lifecycle. Generic VPN,
subscription, native Settings and unrelated device workflows are not claimed by
this frozen Ethernet scenario. ERII-253 completion does not certify v1 release.
