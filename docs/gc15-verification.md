# GC-15 verification boundary

ERII-324 owns the 16 original GC-15 blocks in the ERII-223 frozen package
manifest (`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`).
The source map is `evals/fixtures/gc15-v1.lock.json`: overview lines
355, 357, 359–363, 365 and journey lines 910, 912, 921, 923, 925–928.
Golden source, original M6 fixture, PNG pixels and region manifest retain their
hashes. No wording, noise, order, rubric or release gate changes.

```sh
python3 scripts/verify_gc15.py --binary bin/erii --evidence evidence/gc15.json
python3 scripts/verify_gc15.py --binary bin/erii --evidence evidence/gc15.json --validate
python3 scripts/verify_suite.py --binary bin/erii --gate gc15 --evidence evidence/gc15-focused
python3 scripts/prepare_release_eval.py --input evidence/gc15.json --evidence evidence/gc15-preview.json
python3 -m unittest discover -s scripts -p 'test_gc15.py'
```

The driver runs all six locked M6 wording/noise/order variants with five serial
repetitions per variant. At most two fixture workers run locally by default. Its
`security/multimodal` registration adds one finite Required Ubuntu 24.04 job
for GC-15. The original finance/cases/protection parts stay unchanged. Every job
retains two gate workers, four fixture workers, the ten-minute deadline and all
existing safety series; no runner class, per-job resources or join count changes.
Original M6 and Artifact fragment gates remain required. Native IDs must be disjoint across all 30 fresh journeys; restart and
repeated ingress retain IDs within each journey.

The initial combined cases attempt (run `40431324510`, job `121949923085`,
SHA `cf73fbe97e40a7fa6b3663e29f6b383a10a56338`) was cancelled at the
ten-minute limit: GC-05 took 312.34s, GC-04 345.53s, and GC-15 was cancelled
after its remaining 228.14s. It remains failed evidence. The dedicated GC-15
part removes the new serial queue behind the two existing cases; it claims no performance
certification and cannot waive any missing/failed part.

| Frozen duty | Observed mechanical evidence |
| --- | --- |
| Original input and files | Real `chat send-parts` receives unchanged context/event text and imported PNG ArtifactRefs; frozen requests retain input and media hints. The scripted driver is a separate message. |
| Visible, computed and unknown | Region Fake supplies locked extraction metadata. QuickJS computes nine inclusive days, checks the visa window and 15-day duration, preserves `01` single entry, and leaves masked return year and order/2026 consistency null with a clarification request. Python independently checks dates and fields. |
| Artifact integrity and regions | CLI reopens both PNGs and `visa_check.md`; bytes, hashes, sizes, descriptors, regions and refs bind to native save/return/publication. Host selection loads actual pixels into the frozen image-capable request; the existing Fake decodes the image envelope. No OCR inference is claimed. |
| Access and privacy | Pre-input model describe fails `NOT_FOUND` without a grant. Input grants bind to the receiving Run. No long-term Memory record/evidence or Skill candidate; normal Conversation observation events have empty payloads. Captured CLI/crash stderr excludes source fields and encoded pixels. Required Artifact conformance separately covers foreign Agent/Task, media kinds, expiry and path attacks. |
| Final output | One selected Markdown report separates facts, calculation, unknown/clarification and region provenance. Native save bytes, return, reopened bytes and exactly one assistant attachment agree. Input PNGs are not selected for publication. |
| Recovery and deduplication | SIGKILL after input generation, inside Artifact metadata transaction and after nested save commit. Raw SQLite observation follows the transaction crash without another App inside the held transaction. Rolled-back metadata is absent; recovered save has one object/reference, completed calls/generations are reused, duplicate ingress returns its receipt, drift fails, and settled native state replays exactly. |
| Cleanup and identity | Isolated roots removed, owned processes joined, embedding HTTP peer closed. Source SHA/working hashes before/after, binary SHA256/build info and platform/Python/Node/Go versions accompany raw state. |

Two interrupted safe script attempts resume under the same invocation: script
attempt count is exactly three, rolled-back local save has two attempts.
Publication and each model generation execute once. Completed save never
dispatches again. This recovers known local state; it does not authorize retry
of an unknown external effect.

`--validate` reconstructs assertions from native state and reopened bytes.
Corruption probes include synchronized return/save/Artifact mutations (wrong
stay, guessed year, multiple entries, validity/duration confusion), changed
regions/pixels, missing grants/publication/delivery, Memory/log leakage, unknown
Tool status, duplicate attempts, false crash/rollback and replay drift. Tests
reject copied/relabelled repetitions, changed identities, incomplete/duplicate/
reordered inventories, forged all-true flags, missing cleanup, unsafe paths,
symlinks, altered hashes and semantic-score promotion.

Public JSON is a compact manifest. Native bytes remain beside it under
`.gc15-native/` for same-host recomputation. Hashes alone do not establish trust.
The pinned CI uploader excludes hidden raw bytes. Failed local attempts remain
retained and cannot be replaced by later success.

## Proof limits and remaining ownership

This is synchronous Main work: no Task, Root, Child, Automation, external
submission or Approval is requested. The driver asserts absence. Existing
Task/Child cancel, old-epoch, approval rejection and unknown-effect conformance
gates remain required; this Case does not claim those inapplicable actions.
Other documents/audio/video in the overview have transport coverage in the
existing Artifact gate, not extraction-quality evidence from this visa fixture.

`unscripted_cases.py freeze --case GC-15` supplies unchanged original inputs to
the production bridge as an ungraded conversation fragment. Full unscripted
multimodal ingress/OCR/region adapter, actual local/account model comparisons,
semantic dimensions, current L2 personal documents/media and human privacy
review remain on **ERII-223**. No account or personal dataset is borrowed.
`semantic_score=null`, `eligible_for_grading=false`, `release_ready=false`;
preview constructs no Judge packets or scores. This is not v1 release acceptance.
