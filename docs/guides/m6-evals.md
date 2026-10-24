# M6 personal and Computer verification

ERII-26 adds the exact M6 synthetic worlds from Golden Cases §8.2–8.8,
§8.11, §8.14, §8.16 and the desktop portion of §8.19. Each case runs three
wordings, two 10–30% irrelevant-Conversation variants and one world-key order
variant. `m6-v1.lock.json` freezes the dataset, scoring policy, normative source,
documents, images, source regions and three valid H.264 MOV inputs.

ERII-30 extends GC-02 through its fourth turn and the implemented M7 Automation
boundary without changing these locked inputs. Its historical fixture annotation
deferring Automation no longer describes the executed coverage. Reports record
the extension explicitly and leave other missing M7 journeys unverified.

Run the portable gate with:

```sh
make build
ERII_VERIFY_CASE_JOBS=4 make verify VERIFY_SHARD=personal VERIFY_JOBS=1
```

This runs 66 case/variant journeys plus six isolated red-flag variants, five consecutive times. The suite also retains the current
Core, Memory, Computer, approval, recovery, Bridge and Sidecar gates. Core
conformance now runs SC-08 and SC-11 against the implemented SQLite stores and
executors; the previous Python-only Memory/DesktopLease simulations are removed.
There is no domain schema change in this slice. Existing migration, CAS, fencing
and kill/restart gates remain mandatory.

For focused diagnosis, use the same driver:

```sh
python3 scripts/run_m6_cases.py --binary bin/erii \
  --case GC-13 --evidence evidence/m6-booking.json
```

The runner owns all five verification stages:

| Stage | Observed boundary |
| --- | --- |
| Launch | Fresh isolated roots and real CLI processes; explicit scripted Bridge, embedding HTTP Fake and allowlisted stdio MCP data peer |
| Doctor | Pinned Node/QuickJS, configured model Probe/activation, sealed disposable Computer base, Chromium/CDP and loopback browser peer |
| Drive | Main/Worker Agent Loop → ToolExecutor → QuickJS; real Memory control and recall; Computer actions retain approval and DesktopLease |
| Evidence | Independent SQLite snapshots, exact Tool/Approval parameters, Conversation/events, CLI-opened bytes and hashes, screenshots, external sandbox state |
| Cleanup | Every owned process joined, browser closed and temporary state removed; exported evidence retained |

Chromium's Playwright pipe can become ready before `DevToolsActivePort` exists.
The shared Browser readiness helper waits up to five seconds for a valid port,
within the existing 15-second Doctor deadline. Missing or partially written port
files are retried; other filesystem errors fail immediately. Readiness regressions
run in `npm test --prefix guest-browser`. Cleanup preserves a primary journey
failure and attaches any additional cleanup error; a bad cleanup exit still fails
an otherwise successful journey.

`m6-cases-1…5.json` hold each journey's state and assertions.
`m6-cases-N-assets/` retain the actual opened source/output bytes and screenshots,
indexed by `evidence.exported_artifacts`. The `.m6-evidence.json` marker identifies
runner-owned output; an existing unmarked directory is rejected. Never use a
personal data directory as the evidence destination. All data are synthetic.
`m6-summary.json` requires the same binary, dataset, locks, complete case/variant
set and assertion inventory across all five reports. Missing, skipped, drifted
or failed results cannot pass. A failed run clears the previous success report.

| Case | Mechanical evidence | Proof limits |
| --- | --- | --- |
| GC-01 | Exact health fields, source/time metadata, preserved three input events, low-confidence Recall, no Task, invented vitals or external writes; isolated Turn-2 red-flag replacement routes to urgent evaluation with no lifestyle advice | Unscripted red-flag recognition and advice quality need semantic model evaluation |
| GC-02 | Four ordered turns, unanswered interview question, exploratory constraint, real supersede/deletion; no Task or Automation before the request; one Main Automation Tool call, last-Sunday 20:00 Asia/Shanghai schedule, first fire Nov29 20:00 +08, duplicate input creates no new generation/schedule; future context excludes forgotten content | Interview/constraint selection is scripted and ungraded; no claim of a real model choosing the plan |
| GC-03 | Real corrected Memory, old recall excluded; 54,995.60 USD, 370,120.39 CNY, 520,000 liquid; 30/35/40万 scenarios; unknown tax | Financial explanation quality ungraded; no trading adapter enabled |
| GC-04 | Two existing travel Tasks, NZ paused, completed France append creates one new Root, steering accepted during a held Root, confirmed source bytes unchanged, booking groups; shared Recall/Main publication barriers choose the maintained legal ordering before Root dispatch and before releasing held work | Itinerary selection/explanation ungraded |
| GC-05 | Scoped stable career preference, exact offers, five verification topics, no resignation decision | Ranking and interview guidance ungraded |
| GC-06 | Real progress supersede/deletion, three correction topics, no invented refund or permanent deficit | Spoken interaction, English/Chinese style and tutoring quality ungraded |
| GC-07 | Real browser network-control sandbox: observe disconnected Ethernet, denied repair has no effect, approved repair verifies four paths and one protocol | Sandbox state is synthetic; no claim to repairing a real home network/native macOS Settings |
| GC-10 | Current city/sample metadata; cached weather older than two hours requires a second read before Artifact generation | Clothing recommendation quality ungraded |
| GC-13 | Real confirmation page and order state: denial leaves zero orders, lost post-submit transport acknowledgement produces `unknown`, a later binding reads `order_781`, exactly one submission | Original invocation remains `unknown`; read-only business reconciliation does not fabricate a provider receipt or rewrite its status |
| GC-15 | Real readable PNGs, selected pixels decoded by explicit Fake Bridge, valid regions tied to immutable source hashes; masked year remains null; CLI-openable `visa_check.md` | Portable extraction values are an explicit OCR Fake, not model vision-quality evidence |
| GC-18 desktop | Three separate completed Tasks; later confirmed notes override proposals, standalone design Artifact, exact 416/128/16 CSV counts; three real videos loaded in browser and organized with scoped Artifact descriptors | Desktop organization only; semantic synthesis, missing-information aggregation and video editing quality ungraded |

The Fake chooses explicit Tool plans. JavaScript computes/assembles fixture
outputs from real Tool results; this establishes operation and persistence,
not the ability of an unscripted model to choose those plans. All semantic
dimensions, product `pass@1`, product `pass^5`, mean and minimum scores remain
null. Mechanical repetition metrics are separately labelled. The M6 product
gate is **inconclusive**, never a full Golden Case or milestone PASS.

## External data adapter contracts

The portable personal cases use the existing configured stdio MCP adapter,
not a second Tool pipeline or an implicit production data provider. The
Host allowlists only `read: read_only`; effects are absent. `mcp-data.json` is
the synthetic external world, not an authoritative domain database.

| Boundary | Required production contract | Deterministic equivalent |
| --- | --- | --- |
| Health read | Authenticated subject/scope; explicit relevant field projection; source and sampling time; unavailable fields unknown; no invented vitals or diagnosis; credentials remain in SecretStore | `health:age,sex,diagnoses,current_red_flags` returns only the normative four fields and synthetic subject/source/time |
| Weather | Explicit location, sampling time/timezone and fields; reject absent/unavailable evidence; refresh a cache older than two hours | `weather:cached` at 05:59, then `weather:refresh` at the fixed 08:10 scenario time in 杭州 |
| Other supplied context | Preserve source identity, confirmed/exploratory distinction and unknown values; authorize retrieval before sharing with Worker | Exact isolated `world` documents and scoped Memory/Artifact references |
| OCR/vision | Read only accessible immutable images; return original dimensions/hash and source rectangles; masked fields null; user-provided trip year cannot reveal a masked order year | Locked source-region manifest plus actual image-envelope decoding and negative region/hash/mask tests |

No health/weather online adapter is selected automatically. To connect a real
service, use the existing MCP allowlist and credential boundary documented in
`capabilities.md`; validate these contracts against that service before making
freshness, health or OCR quality claims. A server's annotations never grant
effect permission or retry safety.

## Optional real decoding and Live Shadow

On macOS, the maintained fixture tool can independently run actual Vision OCR:

```sh
python3 scripts/m6_sources.py --evidence evidence/m6-vision.json
```

It compiles the local Swift tool, decodes each PNG, checks visible values against
their source rectangles, checks that the masked region reveals no year, records
actual observations/confidence and joins/removes the temporary executable.
This is separate from portable CI. Regenerate synthetic PNGs into an empty
directory with `swift scripts/m6_assets.swift make DIRECTORY`; review images and
regions and deliberately update the versioned lock after any fixture change.
The existing `lume_media.swift` tool generates/inspects real synthetic movies;
its `clip PATH` action creates one additional three-second H.264 input.

Current L2 is not implied by historical results. Lume lifecycle/identity evidence
is recorded in `computer-provider-l2-result.json`, CUA/native/browser evidence in
`computer-l2.md`, and Jianying compatibility in `computer-l2-result.json`.
`m6-summary.json` identifies these as prior evidence with `current_run: not_run`.
Use the existing `lume_live.py` and `verify_computer_use.py` live commands for a
new release-time observation; do not silently convert offline peers into a
macOS VM or Jianying compatibility verdict.

M6 supports `--scenario-jobs 1|2|4` (or `ERII_VERIFY_CASE_JOBS`), using the maintained owned-process fixture pool. CI selects four; travel uses an isolated capacity-two group for its simultaneous Main/Root processes. Five rounds remain serial; `m6-summary.json` requires all 330 variants and unchanged assertion inventories. `m6-cases-N.progress.json` records bounded progress; the full state report is written after join, including failure. See [CI latency budget](../ci.md#latency-budget).


CI uses seven fixed parts from `m6_report.PARTS`. Context/Memory/network/booking
are separate; GC-18's six variants form three independent pairs. Every pair runs
five consecutive times. `verify_suite.py --shard personal --personal-part NAME`
produces partial evidence only. `ci_personal.py --parts DIRECTORY --evidence OUT`
requires all seven clean exact-SHA/build/driver/fixture parts and reconstructs the
canonical five full reports before the unchanged M6 aggregate can pass. The join
also rejects missing, duplicate, reordered, failed and substituted variants.
Full state/Artifact bytes stay in each part's artifact; the joined reports retain
the original export paths and hashes. No current L2 or semantic score is added.


Travel reuses `verify_recall.drain_with_publication_barrier`: Root freezes its
recall before Main publishes, then Main completes before Root model dispatch.
While the real MCP read is held, a second Main pauses/steers the Task and
publishes before held work resumes. Reports assert that no new Worker model
request crossed the first publication barrier, including when earlier Roots
already ran. The Recall gate still forces the opposite pending-append order,
requires the fail-closed error and separately proves explicit later recovery.
The successful M6 journey is evidence for the recorded legal ordering, not an
all-interleavings success claim. Runtime errors, deadlines and retry rules are
unchanged.
