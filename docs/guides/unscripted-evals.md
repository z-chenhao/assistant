# Unscripted evaluation entry

`scripts/unscripted_cases.py` sends locked Case inputs through production
`chat send` / `chat work`, the shared Agent Loop and pinned Pi Bridge, using an
explicitly selected ModelProfile. It never imports scripted Tool plans, copies
credentials, chooses a model, approves an action or assigns a semantic score.
ERII-230 completion does not certify any whole Case or v1 release.

## Support matrix

```sh
python3 scripts/unscripted_cases.py matrix --output evidence/unscripted-support
```

The command emits the authoritative 20-row matrix with individual gaps.

| Cases | Executable boundary | Missing full adapter |
| --- | --- | --- |
| GC-01–07 | Locked `m6-v1` context and events, six variants | Sources, durable Memory, correction/forget, files, schedules and approvals as applicable |
| GC-09 | Locked `m1-v2` context and events, six variants | Full Inbox/calendar and interruption journey |
| GC-10 | Locked `m6-v1` context and events, six variants | Fresh local sources and stale-source adversary |
| GC-13 | Locked `m6-v1` context and events, six variants | Booking browser, approval and unknown outcome |
| GC-15 | Locked `m6-v1` context and events, six variants | Native files, multimodal evidence and Artifacts |
| GC-18 | Locked `m6-v1` context and events, six variants | Files, concurrent Tasks, pause/append and delivery |
| GC-12 | Original two inputs and allowed contacts/slots; `--variant base` plus two wordings and three source-noise/order variants | Connected mail/calendar and pre-approval Steering; current entry is an ungraded conversation fragment |
| GC-08, 11, 14, 16, 17, 19, 20 | Missing; freeze returns `ineligible`, exit 2 | Non-scripted fixture and maintained lifecycle adapter |

Every current execution has `scope=input_fragment`, `eligible_for_grading=false`
and `final_score=null`. A profile cannot upgrade coverage. A text mention of a
file is not file ingress. Fixture time is explicit context, not a virtual Host
clock. M2 scripted delegation strings are excluded. PR32's unmerged GC-11 driver
remains historical provenance; it is not copied or used as an implementation
baseline. Future Case work should extend this common producer with its maintained
fixture boundary. Missing adversarial variants/assertions remain missing.

## Prepare, freeze and run

Use a new dedicated directory for each case/variant/repetition. Prepare its
profile through ordinary [model controls](provider-bridge.md), with
`ERII_DATA_ROOT=/absolute/eval-root/data` and the same explicit configuration
used below. Local and ChatGPT profiles use production controls and SecretStore;
the runner does not authenticate, probe or select a profile. Never point it at
personal state. No selected profile returns `setup_required`, exit 2, with no
model request or automatic fallback.

Supply a JSON object accepted by the production YAML loader. Write the actual
absolute bridge path in `config.json`:

```json
{
  "version": 1,
  "llm": {
    "provider_bridge_command": "node",
    "provider_bridge_script": "/absolute/erii/provider-bridge/bridge.mjs",
    "provider_bridge_version": "pi-ai-1.3.0-v1"
  }
}
```

The input adapter also allows `context`, `children`, `scheduler`, `tools`,
`artifact`, `spend` and `javascript`. It rejects external adapters, Computer,
workspace access, shared Memory/embedding, Skills and data-root overrides.
Incomplete fixture adapters cannot enter the execution path.

```sh
python3 scripts/unscripted_cases.py freeze \
  --binary bin/erii --root /absolute/eval-root --config /absolute/config.json \
  --case GC-09 --variant noise-1 --arm local --repetition 1 \
  --seconds 120 --output evidence/gc09-freeze

python3 scripts/unscripted_cases.py run \
  --binary bin/erii --root /absolute/eval-root --config /absolute/config.json \
  --plan evidence/gc09-freeze/plan.json --plan-sha256 PINNED_PLAN_FILE_SHA256 \
  --output evidence/gc09-observation
```

Record the plan file's byte SHA-256 at freeze and pass that digest to run.
Output directories must be new and outside the prepared root. A lock excludes
another runner using that root. Existing Runs/inputs reject; there is no resume
or automatic retry. Retain failed state and prepare a fresh root for another
trial. The evaluator owns exclusive access, including ordinary CLI processes
that do not acquire the runner lock.

Freeze binds actual tracked source bytes and HEAD, binary/configuration bytes,
selected profile/version, fixture lock/digest, original inputs, wording/noise/
order, fixture seed, arm, repetition and wall deadline. Run rejects drift before
ingress and checks profile/configuration before each event. The production Host
freezes Model/Capability/Tool/Context policy at Run admission. Native requests
and snapshots are retained; the producer checks the model against the frozen
profile and Tool/model consistency within each Run. Observed Context/Capability/
Tool hashes are labeled observations, not a pre-approved cross-model comparison
plan. Provider RNG seed control is unsupported. Hashes bind bytes, not the truth
of a live-model claim; independent acquisition and review remain required.

The wall budget includes CLI work and terminates owned groups on expiry. Host
limits and monetary configuration are retained verbatim. No approval is granted
automatically. Native spend snapshots, reservations and usage show pricing and
costs. Missing pricing remains **unpriced**; token/wall limits do not represent
monetary authorization. CPU/RSS observations remain null, pending a full adapter.

## Native receipts and Judge

`native.json` retains Conversation, Runs, frozen snapshots, model requests and
results, ToolInvocations including errors, approvals, Task state, Artifact
metadata/publications, trace, delivery and native usage/spend rows. Available
Artifact bytes are copied with size/hash checks. External-effect adapters are
disabled; no external success ledger is fabricated. `observation.json` binds
files and records execution IDs, CLI statuses, assertions, costs, missing
coverage and cleanup. Auth/partial/waiting Runs remain failures; a drained CLI
alone cannot establish success.

`judge-input.json` contains exactly Goal, allowed Fixture, visible Conversation,
final published Artifact summaries and programmatic assertions. Gold, required
text, forbidden claims and scripted plans are excluded. It is a fragment packet,
not the full Case Goal/assertion inventory. The
[strict release consumer](release-evidence.md) refuses observation records. It
still requires whole-case native trials, reviewed comparison conditions and
assertions, all hard-failure checks, mechanical dimensions and an independent
Judge. This producer does not write or certify a release pack.

Owned processes are joined on success, failure, timeout and signal. The root
and receipts, including incomplete state, remain for review. The runner never
deletes operator data. Remove that dedicated root only after retaining needed
evidence. Keep full receipts locally in the durable evidence root.

## Offline mechanism verification

```sh
python3 -m unittest discover -s scripts -p 'test_unscripted_cases.py'
python3 scripts/verify_unscripted_cases.py --binary bin/erii \
  --evidence evidence/unscripted-mechanism.json
```

The verifier owns Launch (private roots and the maintained HTTP loopback peer),
Doctor (real Probe/Select), Drive (actual producer CLI), Evidence (native readback
and hashes), Cleanup (processes, listener and temporary roots). It covers missing
profile/adapter, plan/config/model drift, native Tool/usage flow, strict-consumer
rejection, auth failure, partial stream and an HTTP request exceeding the wall
budget. No semantic scores are produced. `make verify` includes it once in the
providers shard. Unit contracts cover all 72 supported input variants and native
model/Tool drift.
