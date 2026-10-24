GC-12 uses the exact §8.13 source and original two inputs through the maintained
ChannelHarness, production Main/Context Builder, Security, ToolInvocation and
stdio MCP boundaries. It is an explicitly scripted model with independent local
mail/calendar state. It provides no semantic score, live compatibility evidence,
real model comparison, L2 or release acceptance.

Run the complete offline checkpoint:

```sh
make build
python3 scripts/verify_gc12.py --binary bin/erii --evidence evidence/gc12.json
python3 scripts/verify_suite.py --binary bin/erii --evidence evidence/gc12-series --shard communication --jobs 1
python3 scripts/prepare_release_eval.py --input evidence/gc12.json --evidence evidence/gc12-judge.json
```

The communication gate runs five consecutive complete rounds, each containing
six positive variants and denial/lost-response safety probes. A mechanical pass
does not fill the original protected acceptance series or eight semantic grades.
The lock binds the source fixture and unchanged Golden specification.

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
