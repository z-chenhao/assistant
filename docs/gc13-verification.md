# GC-13 booking verification

ERII-259 owns exactly the 19 GC-13 blocks in replan manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.
The Case lock retains the original Golden Cases, scoring, world and six
wording/noise/order variants. Five repetitions run serially per variant.

```sh
make build
python3 scripts/verify_gc13.py --binary bin/erii --evidence evidence/gc13.json
python3 scripts/verify_gc13.py --binary bin/erii --evidence evidence/gc13.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc13.json --evidence evidence/gc13-preview.json
```

Launch/Doctor reuse the isolated Computer harness, explicit Fake model/Lume/SSH
and real Chromium with the production browser adapter. Drive uses actual CLI
input, TUI approval input, Main Task creation, Root/Child, shared ToolInvocation,
frozen Approval, Artifact, Conversation and Delivery boundaries. The original
input is delivered before the final page, and approval arrives after it. The
scripted Child transports the original world to its direct Parent; it does not
prove restaurant selection quality. Main cannot create a Child.

| Frozen source lines | Native mechanical proof / remaining semantics |
| --- | --- |
| 327, 337 | Logged-in sandbox browser journey; real login and current site/VM compatibility remain L2 |
| 329, 331 | AX/screenshot plus DOM readback of R1 湖滨店, date, time, two people, 280 each, 560 total, no deposit and final selector; cancellation terms explicitly say `not specified in fixture` |
| 332 | Denied submit has zero dispatch and zero order; CAPTCHA/password probe returns a user handoff without entering a secret |
| 333 | Submit commits `order_781`, driver acknowledgement is dropped, invocation stays unknown; a new CLI process reads My Orders |
| 334 | Promotion cannot alter the frozen target; separate default 99 CNY addon probe is observed and removed before submission |
| 335 | Report Artifact and Main delivery contain the confirmation obtained from the order table, never from click acknowledgement |
| 855, 857, 866, 868–871 | Original three restaurants and request remain locked; user prompt, final page, later user approval, accepted order and lost acknowledgement are causal steps |
| 873, 875–877 | Approval freezes branch/date/time/party/prices/deposit/terms/addon; only one exact R1 order, one attempted accepted submit, no unknown retry |

The added CAPTCHA/addon/terms surfaces are deterministic sandbox controls, not
new Golden inputs or claims about a real restaurant. Handoff, choice and report
content are explicit scripted plans. Unscripted model judgment is ungraded.

Every journey kills the actual process after generation commit and after the
unknown Tool commit. Restart reuses completed work and preserves unknown. A
second Task is cancelled at desktop admission; its old epoch cannot submit.
A later read-only observation proves the desktop can be acquired and checkpointed.
Duplicate input reuses its receipt; changed parameters under the same input key
are rejected. Final native state and external orders must survive replay unchanged.
Cleanup joins owned process groups and removes disposable roots. Failure bytes
stay in the local hidden evidence directory with a bounded public summary. Browser
transport failures retain method, selector, elapsed time and bounded process diagnostics.

The validator recomputes 47 native assertions, 23 corruption probes, exact source and
binary/build/environment identity, six reopened PNGs and the report bytes, and
rejects copied execution identities across all thirty journeys. The small public
manifest binds compressed native bytes in `.gc13-native/`; it cannot independently
certify completion. Preserve those bytes locally for independent review. Hashes
prove consistency, not authorship.

CI runs the same entry point in six fixed variant parts: `gc13-wording-1`,
`gc13-wording-2`, `gc13-wording-3`, `gc13-noise-1`, `gc13-noise-2` and
`gc13-order-1`, selected with the corresponding `--ci-part` variant ID. Each
contains one original variant and all five serial repetitions. The original
full CLI still requires all thirty receipts. A part cannot validate as a full
report or under another selector. Both collectors recompute identical native
oracles; the fail-closed join and CI comparison reject reused execution
identities across parts and require fifteen logical gates and nineteen suite
rows. The original M6 booking fragments and five-round series remain required.

The e430e69 job 121867994536 (run 40407022499, attempt 1) hit its existing ten-minute
deadline after 521.66 seconds of verification. The two-part b5c6e25 attempt
40411110600 passed wording, but noise job 121882430790 was cancelled after 520.32
seconds with exit 130 and owned processes drained. Its retained log contained no
per-round progress, so it does not identify an internal stall or prove a product
assertion failure. The six-part placement removes concurrent variants from each
GC-13 runner and adds four finite Ubuntu setup jobs relative to b5c6e25 (five
relative to the original unsplit gate). Every existing timeout, per-job
four-fixture/two-gate limit, input, assertion, repetition and cleanup requirement
remains unchanged. No credentials or additional accounts are used.

Compact `gc13-*.progress.jsonl` records identify each variant, repetition,
timestamp and entered stage, including started/completed boundaries. CI retains
these files and bounded failure summaries even when verification is cancelled.
They are diagnostics, never substitutes for completed native receipts. Large
native bytes remain local; the compact upload does not recursively include them.

`unscripted_cases.py freeze --case GC-13` remains the original input-only model
entry; it contains no plans or answers. A connected owner-approved booking
adapter/profile, real login/VM/current L2 and independent semantic grades remain
under ERII-223 and [live preflight](v1-live-preflight.md). This mechanical package
keeps `semantic_score=null`, `eligible_for_grading=false`, `release_ready=false`.
Its completion does not certify the v1 release.
