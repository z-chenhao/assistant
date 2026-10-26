ERII-258 owns the 19 original GC-12 blocks in the frozen replan manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.
`gc12-boundary-v1.lock.json` records their exact source lines/hashes and pins
the unchanged Golden specification, scoring, original fixture and original lock.
GC-12 uses the exact §8.13 source and original two inputs through the maintained
ChannelHarness, production Main/Context Builder, Security, ToolInvocation and
stdio MCP boundaries. It is an explicitly scripted model with independent local
mail/calendar state. It provides no semantic score, live compatibility evidence,
real model comparison, L2 or release acceptance.

Run the complete offline checkpoint:

```sh
make build
python3 scripts/verify_gc12.py --binary bin/erii --evidence evidence/gc12.json
python3 scripts/verify_gc12.py --binary bin/erii --evidence evidence/gc12.json --validate --probe-rejections
python3 scripts/verify_suite.py --binary bin/erii --evidence evidence/gc12-series --shard communication --gate gc12 --jobs 1
python3 scripts/verify_gc12.py --binary bin/erii --evidence evidence/gc12-series --validate-series
python3 scripts/prepare_release_eval.py --input evidence/gc12.json --evidence evidence/gc12-judge.json
```

The communication gate runs five consecutive complete rounds, each containing
six positive variants and denial/lost-response safety probes. A mechanical pass
does not fill the original protected acceptance series or eight semantic grades.
The lock binds the source fixture and unchanged Golden specification.
The focused command retains all five rounds but cannot claim the complete
communication inventory. Required CI still runs its fixed parts and full join.

| Frozen blocks | Mechanically observed boundary |
| --- | --- |
| 313, 842–844 | Original two inputs; Monday frozen before notice, Steering durable before SIGKILL, new Tuesday approval after restart |
| 317, 848 | MCP source contacts/recent thread/common slots, exact Alex Chen recipient and Tuesday 11:00–11:30 +08; ambiguous/missing-slot no-write Fake regressions |
| 318–319, 849–850 | Recipient/body/timezone/time/action frozen; native request hashes recomputed, obsolete approval invalidated, wrong hash rejected, concurrent/repeated callbacks produce one mail and one event |
| 320–321, 851 | Short scripted body, actual remote status readback and one completion; model style and unauthorized-commitment judgment remain ungraded |
| 315, 323, 827, 829, 840, 846 | Normative labels and exact source/fixture identities; these confer no semantic score |

Every aggregate records source SHA and tracked source hashes before/after,
binary SHA256/Go build identity, OS/Python/Node/Go versions, native checkpoints,
model/capability snapshots and cleanup. `--validate`, Judge preparation and the
CI collector recompute native observations. They reject missing/reordered
variants, changed source/binary/fixture, incomplete cleanup, altered pending or
completed cuts, frozen hashes, generation snapshots and replay state. The CI
manifest carries native execution IDs and rejects reused Run, ToolInvocation,
generation, Delivery or message IDs across all five rounds. Restart snapshots
within one journey retain their original IDs. Hashes prove consistency;
independent exact-SHA review and required CI remain mandatory.
The optional rejection probe corrupts real passing bytes in 13 ways and requires
both the actual validator CLI and native CI consumer to reject them. It also
submits a copied complete round to the CI join and requires identity rejection.
Full diagnostics stay beside the input report in its `.rejections` directory.

The CLI removes the previous aggregate before fixture validation or startup.
An unsuccessful rerun exits nonzero and leaves no current passing aggregate;
variant diagnostics remain available, and Judge preparation rejects the missing
report. To run the success → same-path startup failure → Judge rejection
regression through the actual product CLI:

```sh
ERII_GC12_TEST_BINARY=bin/erii python3 -m unittest discover -s scripts -p test_gc12.py
```

Without that explicit binary, the Python regression seeds the complete synthetic
observation fixture and exercises the same failure and consumer CLI boundaries.
It does not require a product installation or claim a new product run.

Launch creates a disposable installation, explicit model Fixture, bound fake
channel and stdio MCP peer. Doctor selects the explicit profile and discovers
only the configured read-only source/status Tool and external-effect invitation
Tool. The MCP peer is copied into its own provider directory; neither process
sandbox nor model permissions are widened. macOS uses the Xcode Python binary
directly, avoiding the system launcher's blocked xcodebuild dependency. Linux
uses `/usr/bin/python3` inside the existing sandbox.

Drive submits the original request, reads the exact contacts/recent thread and
common slots through MCP, and holds Monday's frozen approval before channel
transport. It persists “不要周一。”, rejects obsolete approval, SIGKILLs that
server and starts the same installation again. The new frozen action identifies
Alex Chen, Tuesday 11:00–11:30 +08, subject, short body and both actions. Wrong
hashes reject; concurrent signed callbacks and repeated source IDs approve once.

The Fake exposes one explicit invitation operation that atomically records one
mail and one calendar entry in separate tables in its independent `remote.db`.
This models a combined invitation endpoint, not two production providers or a
distributed transaction. It deliberately has no deduplication: duplicate dispatch
creates duplicate rows and fails the harness. The model selects from the supplied
source, then reads remote status before reporting completion. Identity ambiguity
and missing common slots have deterministic no-write regressions.

Evidence retains original input, complete native state/generations/frozen
snapshots/approvals/Tool results, separate mail/calendar records, readbacks,
channel receipts, hashes and unchanged semantic state after replay/restart. No
Artifacts are needed: the empty Artifact inventory is recorded. The interrupted
old approval notice remains `unknown` and query-only. Its reconciliation version
and timestamp can change; all IDs, payloads, statuses, attempts and semantic
outputs remain exact. This is not a second mail/calendar effect.

Denial creates neither external record. Dropping the reply after remote commit
leaves the native effect `unknown` with one attempt; separate read-only status
can observe the records, but cannot convert the never-replay Tool to succeeded
or dispatch it again. Repeated approvals and restarts preserve that state.

Cleanup joins every owned Web/Bridge/MCP process and HTTP peer and removes only
the disposable installation. Failed runs retain diagnostics before removal.
Judge preparation admits only complete ungraded six-variant receipts and
allowlists source, visible dialogue, empty Artifact summary and assertions.
Every packet is ineligible for grading; missing real-model/L2/human evidence
remains INCONCLUSIVE. The final release-completion command remains nonzero.

This short synchronous journey stays in Main. It creates no Task, Root/Child
Run or Artifact; their empty inventories are evidence, and Task cancellation or
old Task epochs are inapplicable. Steering and obsolete approval, process
restart, duplicate callbacks and unknown effects are exercised here.

The maintained `verify_unscripted_cases.py` gate freezes and runs these original inputs through the production CLI/bridge with an explicit loopback peer, retaining the ungraded input-fragment boundary.

Subsequent model evaluation uses `unscripted_cases.py freeze --case GC-12 --variant base`
with an explicitly configured, approved evaluation profile and isolated root
following [the unscripted guide](guides/unscripted-evals.md). The maintained
entry exposes the original inputs; its connected mail/calendar adapter is still
missing and it remains ineligible for whole-case grading. ERII-223 retains that
semantic gap, current L2 real-account/channel evidence and release acceptance.
No credentials or accounts are borrowed. `semantic_score=null` and
`release_ready=false` throughout this mechanical gate.
