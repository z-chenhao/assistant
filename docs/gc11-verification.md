# GC-11 quote comparison verification

ERII-257 owns exactly the 23 GC-11 blocks in replan manifest
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`.
`gc11-boundary-v1.lock.json` binds their original lines/hashes and the unchanged
Golden Cases, scoring and locked quote fixtures. This gate proves deterministic
mechanics. Recommendation quality, real insurance research and semantic grades
remain unverified under parent ERII-223.

```sh
make build
python3 scripts/verify_gc11.py --binary bin/erii --evidence evidence/gc11-boundary.json
python3 scripts/verify_gc11.py --binary bin/erii --evidence evidence/gc11-boundary.json --validate
python3 scripts/prepare_release_eval.py --input evidence/gc11-boundary.json --evidence evidence/gc11-boundary-preview.json
```

The suite's communication/channels `gc11` gate calls this same entry point.
After ERII-378, the fixed parts are gc01, gc02, gc09, gc10 and channels; GC-11
stays in channels. Their fail-closed join must reconstruct all nine gates and
thirteen suite rows, including the original GC-12 five-round series. The shared
repair's PASS does not certify this combined consumer inventory. This gate
retains all six original wording/noise/order variants and five serial repetitions
per variant. Two independent fixture workers use the existing pool; select
`--variant-jobs 1` for serial execution. CI resources, deadlines and worker limits
remain unchanged. GC-11 joins the long native series ahead of short gates.
The original capability gate still runs its existing five rounds and all six
quote variants. This entry adds recovery and independent receipt validation.

Launch reuses `verify_capabilities.prepare` and its isolated stores, explicit
Fake Bridge and sandboxed stdio MCP. Doctor activates only the explicit fixture.
Drive uses `quote_case.drive`, retaining the exact original context, events,
timestamps, noise and ordering. Explicit plans and scripted answer bytes are
separately labelled; they prove transport and cannot measure model judgment.
Evidence comes from actual CLI ingress/history, ToolInvocation, frozen model
requests, ArtifactStore, Conversation and Delivery. Cleanup joins owned children
and removes each disposable root. A failed drive retains diagnostic state.

| Frozen source | Evidence / proof boundary |
| --- | --- |
| Journey and capability labels, lines 299, 309 | Locked original two-turn quote comparison; six unchanged variants; research/personalization quality remains semantic |
| Official source priority, line 303 | Exact locked quote source transported by two read-only MCP calls; B's oral promise remains unwritten; live official-source research is unexecuted |
| Decisive differences and unknowns, line 304 | Scripted report includes coverage equality, services, exclusions and unknown quality; bytes reopen through the actual Artifact CLI; judgment is ungraded |
| Unverified cashback, lines 305, 821 | B's 5038.49 paid-cashback scenario and 5738.49 unpaid scenario remain distinct in source, saved bytes and visible reply |
| Personal conditions, line 306 | Locked 45:35:20 weights preserved through context and report; no popular ranking or invented user equipment/budget |
| No comparison purchase, line 307 | Only read, Artifact save and echo calls; actual permissions and replay policies checked; zero approval, Task, Child, automation or external-effect log |
| Fixture table and shared cover/weights, lines 803, 805–809, 811 | Exact three premiums, written/oral cashback flags, all services, coverage equality and weights; no fixture/scoring changes |
| Original script, lines 813, 815–816 | Both causal user events enter CLI and frozen model requests, with one reply/delivery per turn |
| Gold, lines 818, 820, 822–823 | Scripted bytes cover common protection, conditional B or certainty-oriented C, written terms/payment/refund conditions and complaint route without guaranteed recovery; quality and real enforceability remain ungraded |

Every journey is killed after its first committed quote read and then after its
first committed Delivery intent (still pending transport). Restart must reuse
completed calls and generations, and deliver exactly once. Duplicate ingress and
changed parameters under the same key cannot create new work. Final native state
must equal replay and post-rejection state. This synchronous comparison authorizes
no purchase. Task cancellation, obsolete Task epochs and unknown external effects
are inapplicable here; buying transfers to GC-13's approval boundary. No cross-Case
release invariant is certified by this gate.

The validator recomputes all assertions and 22 native corruption probes. It binds
Artifact references to their actual save invocations and owning Runs, checks
stored size/hash against reopened bytes, frozen snapshots, usage, released claims,
source-before/after working-file hashes, binary SHA256/Go build identity, environment,
timestamps and cleanup. Native Run, ToolInvocation, generation, message, Delivery
and Artifact identities must be disjoint across all 30 journeys. Restart/replay
reuse the same identities within one journey. Copying six real journeys and
renumbering them as 30, even with resealed manifests, must fail both the actual
validator CLI and the CI native-receipt consumer.

A small public manifest binds retained `.gc11-native/` bytes by size and SHA256.
Keep those bytes and the copied-repetition rejection diagnostic locally for
independent review. Hashes bind consistency, not authorship. Missing, tampered,
symlinked, escaped, reordered, duplicated, graded or unclean receipts fail closed.
The preview emits no Judge packets, scores or winner. `semantic_score` stays null;
`release_ready` and `eligible_for_grading` stay false.

The existing [unscripted GC-11 trial](gc11-evaluation.md) remains the maintained
model path: `gc11_driver.py --trial` sends the original two prompts without plans
or answers, through an owner-prepared dedicated profile and production Pi Bridge.
Its offline loopback test proves mechanics only. Approved profiles, independent
semantic grades and current L2 official sources remain tracked by ERII-223 and
[the live preflight](v1-live-preflight.md); this issue does not borrow credentials
or certify v1 release acceptance.
