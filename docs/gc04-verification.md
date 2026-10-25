# GC-04 travel verification

`scripts/verify_gc04.py` drives the original travel continuation through the real
CLI/TUI Main, Task search/manage, Root, MCP, restricted QuickJS, Artifact,
Reporter and Delivery boundaries. Explicit `capabilities` and `terminal-input`
Fake profiles separate driver plans from original user messages. This proves
mechanics; it does not grade language understanding, itinerary quality or fresh
travel information. `semantic_score=null`, `eligible_for_grading=false`,
`release_ready=false`. Model, real account/VM and current L2 gaps remain with
[ERII-223](https://multica.ai/erii-studio/issues/01a16936-335f-72a4-a7f4-618823a87151).

```sh
make build
python3 scripts/verify_gc04.py --binary bin/erii --evidence evidence/gc04.json
python3 scripts/verify_gc04.py --binary bin/erii --evidence evidence/gc04.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc04.json --evidence evidence/gc04-preview.json
python3 -m unittest discover -s scripts -p 'test_gc04.py'
```

The `security` suite gate runs all six original locked `m6-v1` variants, each
for five serial rounds. `--variant-jobs` bounds independent variants (1, 2 or 4)
and never reduces rounds. Wording, noise, order, scoring and protection rules
stay unchanged. The new lock binds exactly the 24 GC-04 source blocks from
manifest SHA256
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.

| Boundary | Observed proof and limit |
| --- | --- |
| Launch / Doctor | Disposable config/data, explicit Fake probe/select, synthetic embedding and read-only MCP peers, real QuickJS. No online account or implicit provider. |
| Existing work | Public Main Task creation completes France and pauses a held NZ Root. Two imported confirmed itinerary Artifacts remain byte-for-byte unchanged. Fixture IDs map to Host-created Task references; no database seeding. |
| Original journey | The unchanged context and three events appear once, in variant order, in Conversation and frozen requests. Event 2 reaches Main while France's actual Root is held, as an unchanged text part alongside a separate driver part in one atomic input. Event 3 uses a real PTY TUI after Root completion. Separate driver inputs perform explicit mechanical operations. |
| Search / continuation | Real `task_search` uses the known fixture's completed status filter and France query to return one Task. It does not prove natural intent selection or unfiltered search ranking. Append activates that Task at epoch 2 with one new Root; concurrent Steering stays in that Root. Later classification uses epoch 3 and a new Root. No third Task or Child is created. |
| Steering / sources | The first Root waits on a real MCP read. Main receives the original text part and commits the scripted append before releasing it. Its subsequent frozen request carries Steering and executes the explicit revised plan. World data comes from the configured MCP Tool, including preferences, hotel, confirmed bookings, weather, opening and transit. Synthetic sampling is labelled with the frozen case clock; this is not a live sample. |
| ETA / artifacts | JavaScript derives 19:50 from the explicit 20:00 deadline minus a 10-minute driver margin, then 19:15 from the Tool's 35-minute return. Python datetime independently recomputes both and the 25-minute outbound leg. Museum arrival uses its opening time; route excludes countryside. Both reopened JSON Artifacts distinguish confirmed / needs_booking / no_booking and retain constraints, source and confirmed references. Classification links the prior immutable Artifact and grants it through the public attachment/Task boundary. Natural-language constraint extraction and travel suitability remain ungraded. |
| Restart / replay | Real SIGKILL after a committed input generation and after Reporter commit. Accepted generation results and the committed report are reused. Duplicate original ingress and another drain preserve exact state, Tool attempts, Artifacts and deliveries. |
| Pause / old epoch | NZ's held Root ends cancelled with `task_paused`; releasing its old MCP hold cannot revive it. NZ remains unchanged throughout France continuation. This is a bounded pause/late-completion trace; general cancellation/old-owner races remain in Task-control conformance. |
| Delivery | Both published itinerary references enter TaskUpdate and Main's runtime requests; Main replies have one durable Delivery each. This Fake's replies do not prove user-facing itinerary explanations. |
| Security / applicability | The frozen three-turn script requests planning/classification, with no purchase, change, cancellation or payment. Tool allowlist, empty Approval/Automation and explicit `booking_actions=[]` prove no such action. Visa/flights absent from the frozen world remain unknown. No booking-disconnect, approval transfer, `unknown` effect, Child or Automation is executed by this journey. Existing required Tools/Code Mode/Task-control gates retain those cross-cutting contracts; they are not travel execution evidence. Real booking/visa/live-map/traffic/return-journey semantics require the parent's model/L2 evaluation. |
| Cleanup / identity | Source SHA and tracked source hashes before/after, binary SHA/build metadata and Python/Node/Go/platform identity. Native requests, Tasks, Run tree, Tools, Approval, Outbox, Artifact bytes and Delivery are retained. All owned processes join, embedding peers stop and disposable roots disappear. |

`--validate` requires 30 ordered receipts, all locks and source/binary/cleanup
identity. It recomputes assertions from native records rather than trusting
stored booleans. Parent Code Mode returns, native Artifact save arguments,
CLI-reopened bytes and descriptor hashes must agree. Twenty-five corruption
probes call the real observer. Seven synchronize native returns, save arguments,
Artifact bytes/descriptors and replay copies, then reject late ETA, missing
sampling, countryside, changed confirmed bookings, conflated groups, wrong
transit and invented purchase using the independent locked-world oracle.
Other probes reject wrong Task/epoch, revived paused Root, lost Steering,
pre-publication dispatch, missing report/delivery/access, false restart,
duplicate attempts and replay drift. Receipts are inspectable consistency
evidence, not execution attestation; exact-SHA independent review remains required.

| Original frozen Golden source lines | Mechanical coverage / semantic boundary |
| --- | --- |
| 201, 203, 211 | Original journey/capability envelope; the deterministic world only specifies France/NZ continuation. Visa, flights, live map/traffic and return travel remain model/L2 scope. |
| 205, 206 | Existing Task search/append/new Root, immutable anchor and confirmed sources; revised plan retains its preceding Artifact reference. |
| 207 | Labelled synthetic weather/opening/transit source and concurrent Steering before Root settlement; current live sampling remains unexecuted. |
| 208 | No action requested or dispatched; no purchase/approval/unknown claim. Dedicated required side-effect conformance remains unchanged. |
| 209 | CLI-reopened itinerary with route, classifications, provenance and ETA; offline rendering and usability remain ungraded. |
| 590, 592, 610, 612, 613, 614 | Original clock/world and all three unchanged inputs across six variants and five serial rounds. |
| 616, 618, 619, 620, 621, 623 | Real filtered search and read-only MCP world; fixture values are reused without changing Golden input. |
| 625, 626, 627, 628 | No third Task, epoch 2 continuation, excluded countryside, independently computed ETA, three booking groups and unchanged Oct 3–4 sources. |

The public `gc04_native_manifest` points to full local JSON under
`.gc04-native/` with byte count and SHA256. Validation and preview require those
bytes and reject path traversal, symlink, hash and summary drift. The pinned CI
uploader excludes hidden files. Retain the full local evidence and executable
for the same-host Reviewer; a manifest alone is inconclusive.

The existing `unscripted_cases.py freeze --case GC-04` entry submits only the
original context/events through the production Pi Bridge using an
operator-prepared profile. It sends no driver plan, expected answer or credential.
Its existing-Task/held-Steering adapter remains incomplete and input fragments
remain ineligible for grading. See [unscripted operations](guides/unscripted-evals.md).
This finite mechanical delivery does not certify the parent release.
