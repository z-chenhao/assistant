# Current release evaluation preflight

ERII-30 continues in PR #32. Independent review of its implementation and
evidence does not satisfy the original release acceptance contract.
These commands prepare or inspect approved
test infrastructure; none grants permission to use a personal account, incur
model cost, provision a VM or send a real channel message. Historical L2
receipts remain historical. Current scores, comparison winner and human
acceptance remain unknown.

## Offline Judge and model comparison preparation

```sh
python3 scripts/verify_capabilities.py --binary bin/personal-agent --scenario quotes --scenario-jobs 2 --evidence evidence/gc11.json
python3 scripts/prepare_release_eval.py --input evidence/gc11.json --evidence evidence/gc11-eval-preparation.json
```

The second command validates all six variants, fixture/response bytes and
required mechanical assertions. It produces six allowlisted Judge packet
previews. Only Goal, permitted Fixture, user-visible dialogue, final Artifact
summary and programmatic results enter `judge_input`. Original user wording
comes from the locked fixture; raw plan-bearing messages, generation requests,
Run traces and hidden Gold are excluded. Tests inject plan/trace sentinels and
require their absence. Every preview is ineligible for grading because the
responses are scripted. No model is invoked and no score is assigned.

The output separately specifies two unscripted comparison arms and the equal
conditions to freeze: source/binary/config, fixture, Context Builder,
Capability/Tool/Skill versions, limits, seed, clock and initial stores. Only
model/profile identity may differ. Each arm needs five actual runs of each
variant, separate snapshot and latency/resource receipts, mechanical failures
and eight-dimension dispositions before pass@1/pass^5/variance can be reported.
O/F/P/A/C semantic review cannot award T/S/R mechanically or override a hard
failure. The [production-CLI GC-11 driver](gc11-evaluation.md) now executes
individual unscripted trials and validates a sixty-receipt comparison cohort.
It omits all `Capabilities:` plans and answer templates. Actual approved model
runs, constrained grades and current L2 remain unexecuted.

## Exact environment inputs and commands

Set these variables only to installation-owner-approved **dedicated** paths.
Do not paste credentials into a terminal transcript or issue comment. The
profiles and credential references must already exist in the test installation.

```sh
: "${ERII_TEST_CONFIG:?approved absolute Erii configuration path}"
: "${ERII_TEST_DATA_ROOT:?approved isolated installation data root}"
: "${ERII_L2_ROOT:?approved existing owned Lume root}"
: "${ERII_L2_PRODUCT_CONFIG:?approved Lume product-live.json path}"
ERII_DATA_ROOT="$ERII_TEST_DATA_ROOT" bin/personal-agent -config "$ERII_TEST_CONFIG" status
ERII_DATA_ROOT="$ERII_TEST_DATA_ROOT" bin/personal-agent -config "$ERII_TEST_CONFIG" model list
python3 scripts/lume_live.py --root "$ERII_L2_ROOT" --profile product doctor
```

`status` and `model list` inspect readiness and non-secret metadata; they do not
test/select a paid profile. Capture local endpoint/model/wire protocol and
account connection/model/profile **IDs and versions**, reasoning/context/output
limits, allowed cost, and Judge identity through private configuration. Account
login, if needed, is performed by the owner in the authenticated Settings/TUI
flow; runtime-owner credentials are never borrowed. The approved configuration
must identify its real Bridge, isolated NATS stream, Tool/Skill roots and secret
store references. Do not dump configuration or auth output into public evidence.

After approval and a successful doctor, the existing VM workflow is:

```sh
python3 scripts/lume_live.py --root "$ERII_L2_ROOT" --profile product serve
# Separate owned foreground execution; collect/join it after the following drive.
python3 scripts/verify_computer.py --binary bin/personal-agent --live-root "$ERII_L2_ROOT" --live-config "$ERII_L2_PRODUCT_CONFIG" --evidence "$ERII_L2_ROOT/evidence/erii30-current-computer.json"
```

The root must satisfy [ownership, pinned Lume, Guest permissions and shutdown
rules](computer-l2.md) and the sealed clean-base/product configuration in
[Computer operations](computer.md). The lifecycle drive uses a deterministic
model with a **real VM**; it does not certify unscripted model quality or current
Jianying compatibility. Re-run the application matrix separately, recording
Guest/application/Driver versions, current sampling time, actual screenshots,
failure class and a named human reviewer. Stop and join the owned server; do
not power off to claim a clean checkpoint or touch an existing personal VM.

The current real channel uses the production foreground command:

```sh
ERII_DATA_ROOT="$ERII_TEST_DATA_ROOT" bin/personal-agent -config "$ERII_TEST_CONFIG" web serve
```

The owner must specify loopback Web vs enrolled Feishu test p2p tenant/app/open
ID/destination, credential **references**, allowed read-only work and the exact
recipient boundary. This command alone is only Launch; current real-login,
message/delivery/approval observations and named human review must be captured
before L2 is complete. Stop/join it after readback. Never send a real message
while the approved channel or recipient is unknown; the existing Fake channel
gate is not a substitute. Independent packet/audit/fixture work continues while
these live inputs are unavailable.

The single consolidated input request is: approve the dedicated config/data
paths, local/account profile IDs+versions and limits/cost, Judge/reviewer,
existing owned VM/product config and Guest/app permissions, and Web/Feishu test
channel/recipient scope. Supply secret values only through the credential store.
