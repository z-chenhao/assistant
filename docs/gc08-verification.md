# GC-08 wedding checkpoint

The maintained CLI runner drives Golden §8.9's original three inputs and exact
clock, event, C1/C2/C3 deadlines and contacts. Six variants include three wordings
and three source order/noise changes. It selects an explicit scripted provider;
there is no answer/Tool plan added to user input and no production model claim.

```sh
make build
python3 scripts/verify_gc08.py --binary bin/erii --evidence evidence/gc08.json
python3 scripts/prepare_release_eval.py --input evidence/gc08.json --evidence evidence/gc08-judge.json
```

`make verify` includes this complete six-variant gate in its communication shard (the maintained MCP sandbox is provisioned there).
`--variant base` is a focused diagnostic, never a complete Judge input.

Launch uses an isolated MainHarness configuration and stdio MCP peer. Doctor
selects/tests an explicit fixture profile and validates the fixture/source lock.
Drive sends the three original inputs through `chat send/work`. Main reads the
fixture via MCP, creates one Task, then searches and appends that same Task twice.
The fixture's `task_wedding` is an external event alias; Host Task/Run identities
remain opaque and runtime generated. The source's clock is fixed; native audit
timestamps record actual execution time. No production clock is overridden.

The first Root saves the blocked C1→C2 checklist and lower priority C3. A new
Root saves 128 guests, completed C1, active C2 and unknown table/per-table counts.
Both results return through Reporter to Main. The third Root preserves those
facts in a new checklist with the hotel email draft and empty attachment list.
The ordinary external-effect Tool freezes that exact draft for approval. The
runner grants no send consent: the expected checkpoint is a waiting Root and
pending approval, zero provider attempts and zero remote mail. Owner CLI reopens
all three Artifacts and checks native content hashes, Task and Root ownership.

Evidence retains each turn's native Task/instruction/Run/model/Tool/Reporter/
conversation/Artifact/approval state, CLI-opened bytes, frozen call hash, wrong
hash rejection, actual SIGKILL at the pending scan boundary, and identical state
after restart plus duplicate input replay. A separate denial checkpoint verifies
APPROVAL_DENIED with zero attempts, one result per Root, terminal state and
idempotent denial replay. The independent peer has no network or domain DB
access, no Gold correction and no deduplication that could conceal a bad send.
Cleanup joins owned children and removes disposable state; failures retain their
receipts. Unit regressions reject dependency/fact/contact/hash/attempt drift and
the fixture's source-title/intent parsing errors found during development.

Judge preparation recomputes assertions and allowlists fixture, visible dialogue,
Artifact summaries and assertions. All six previews remain ineligible, scores
and comparison winner null. These observations prove scripted mechanics only.
They do not establish unscripted planning quality, weeks of event management,
calendar/reminder variants or real hotel delivery. Broader GC-08, all original
release thresholds, protected series, actual local/account comparison, current
L2/named human review and the individual normative audit remain required.
