# GC-11 model trial mechanics

The driver sends the locked synthetic context and original two user events
through the production CLI, Context Builder and pinned Pi Bridge. It never
logs in, adds/tests/selects a profile or supplies an answer/Tool plan. Actual
model quality, all semantic metrics and current L2 remain unverified.

## Offline contract

```sh
python3 scripts/verify_local_provider.py --binary bin/erii --gc11-driver-only --evidence evidence/gc11-driver-offline.json
python3 scripts/prepare_release_eval.py --input evidence/gc11-driver-offline.json --evidence evidence/gc11-driver-preview.json
```

The same contract runs in the ordinary local-provider CI gate. An owned HTTP
model peer drives real Main/MCP/Artifact operations. It derives observations
from actual Tool bytes, without recommendation Gold. Both Artifacts reopen
through the CLI. Store readback checks two original turns/replies/deliveries,
frozen profile/Capability, no Task/Automation/effect/approval, completed calls
and duplicate-input equality. A reused installation rejects before model HTTP.
Processes/server join and the disposable root is removed. HTTP-fixture reports
stay explicitly ineligible for model-quality grades or a comparison winner.

## One approved model trial

The owner first prepares **one fresh dedicated Erii installation per
arm/variant/repetition**, through native login/Test/Select controls. Do not copy
another installation's databases or credentials. Read `model list` again after
activation: selection can increment the profile version. The driver requires
the exact active ID/version and limits. Use a clean immutable source checkout;
preserve runtime/user edits by creating an isolated verification checkout.

The owned root contains `data/`, `provider/mcp-data.json`, `config.json` and
`erii-eval.json`, without symlinks. Provider data is the locked `gc11-v1.json`
case's `world` under `{"insurance_quotes": ...}`. Config must be this JSON,
with approved absolute paths substituted:

```json
{
  "version": 1,
  "app": {"data_root": "TRIAL_ROOT/data"},
  "llm": {"provider_bridge_command": "node",
    "provider_bridge_script": "SOURCE_ROOT/provider-bridge/bridge.mjs",
    "provider_bridge_version": "pi-ai-1.3.0-v1"},
  "mcp": [{"name": "fixture", "command": "SOURCE_ROOT/bin/erii",
    "args": ["-test-mcp-server", "TRIAL_ROOT/provider"],
    "directory": "TRIAL_ROOT/provider", "tools": {"read": "read_only"}}]
}
```

The Tool peer is explicitly synthetic even for an unscripted model trial:
this is L1, not official insurance research/current L2. Richer Tool, channel,
Computer and workspace configurations reject before model admission. Credentials
stay in the configured native SecretStore; model/API-key environment variables
from the coding agent are not inherited.

The exact non-secret manifest `erii-eval.json` is:

```json
{
  "version": 1, "issue": "ERII-30", "execution_kind": "live_model",
  "arm": "local", "variant_id": "wording-1", "repetition": 1,
  "profile_id": "APPROVED_ACTIVE_PROFILE_ID", "profile_version": 2,
  "limits": {"context_window": 8192, "max_output_tokens": 1024},
  "turn_timeout_seconds": 300, "config_sha256": "SHA256_OF_CONFIG_JSON"
}
```

Use approved common limits, rather than assuming the example numbers apply.
Unknown manifest fields, source/config changes, effects, symlinks, stale
profiles and prior domain work reject. Execution kind records operator
provenance; it is not independent proof of a genuine model. The offline harness
always declares `offline_loopback_fixture`. No secret belongs in these files.

```sh
python3 scripts/gc11_driver.py --binary bin/erii --trial /APPROVED/TRIAL_ROOT/erii-eval.json --evidence evidence/trials/local-wording-1-1.json
python3 scripts/prepare_release_eval.py --input evidence/trials/local-wording-1-1.json --evidence evidence/judge/local-wording-1-1.json
python3 scripts/gc11_driver.py --binary bin/erii --compare evidence/trials --evidence evidence/comparison-preflight.json
```

Each trial joins all foreground CLI processes and deliberately retains the
approved installation for operator inspection. It does not delete account
metadata or external SecretStore entries. After evidence retention, the owner
cleans that installation through ordinary operations. The offline harness
removes its own disposable root. Failures return nonzero, retain state/diagnostic
evidence once admission begins, and remove prior output before validation.

## Frozen condition preflight

Exactly two arms × six variants × five independent trials are required.
Preflight recomputes assertions, original inputs, source/replay witnesses, raw
initial Context and Tool schemas from state. It rejects missing, duplicate,
reused-Run, dirty, unjoined, graded or drifted receipts. Source/binary/driver/
Bridge/fixture/configuration/limits/Capability conditions must match. Only the
verified owned Agent namespace and installation paths are normalized; every
permission/schema remains exact. Model snapshots stay separate per arm,
including provider/model/wire/reasoning/compatibility metadata.

Within each arm, all thirty trials must have one canonical model identity.
Compare every generation snapshot, requiring a valid local endpoint SHA-256
identity and retaining endpoint hashes, model, wire,
reasoning, defaults, limits, compatibility, capabilities and any future model
fields. Only installation-generated snapshot/profile/auth-reference IDs,
timestamps and profile version counters are normalized across installations;
each trial still checks its exact approved profile ID/version and frozen
snapshot. Raw snapshots remain separately recorded. Local and account arms
may use different models; neither arm may switch models between trials.
Preflight emits the two canonical identities for inspection and rejects drift
before claiming matching conditions. Synthetic diagnostic cohorts and unit
tests do not produce real-model execution or semantic grades.

Latency, completed-reply time, usage, calls and CLI-host child CPU/hardware are retained.
External model/GPU resource measurements remain unknown. Fixture seed zero does
not pretend that an unsupported provider seed was set; actual generation defaults
are retained and compared without forcing unsupported parameters.
Logical generations, recorded generation/Tool dispatch attempts are distinguished
from provider-internal network retries, which are not measured by the driver.
Streaming first-useful-feedback latency remains unknown. Empty Tool/Artifact
inventories are valid when the model uses the supplied source directly; the
driver does not add obligations absent from the Golden Case. Generated Artifacts
must reopen and match their stored bytes/hash. Judge packets allow only Goal,
permitted Fixture, visible dialogue, Artifact summaries and assertions. Offline
receipts remain ineligible even though their CLI path is real. Conditions
matching does not assign scores, pass@1/pass^5, variance or a winner.

Approved profiles/environment and actual Judge/human review remain due; the
consolidated input request is in [current live preflight](v1-live-preflight.md).
GC-12 §8.13 now has maintained scripted mail-and-calendar mechanics. GC-08 §8.9
also has a maintained wedding checkpoint; the next uncovered exact candidate is
GC-14 §8.15. These observations do not complete release acceptance.
