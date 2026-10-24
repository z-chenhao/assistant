# Release evidence decisions

`scripts/release_audit.py` inventories the baseline and Golden Case sources.
`scripts/release_decision.py` consumes independently acquired evidence for a
specific release SHA. These are private repository tools. Neither invokes a
model, authenticates an account, grants access, publishes a release or changes
Task state. ERII-224 completion does not certify v1.

## Inventory and history

```sh
python3 scripts/release_audit.py --evidence evidence/release-audit.json
make release-audit
```

The second command requires completeness and deliberately exits 2. Inventory
has no certification path. The [original ledger](../v1-release-ledger.json) is
preserved byte for byte from ERII-30 checkpoint
`38faf6144d9ae0dc5e5d38aaf32d06052820d683`, including all 122
`partial_mechanical`/`evidence_gap` reviews. Full paragraph/list/table/fence text,
source/section/line/hash and remaining obligations remain in every inventory.
The ledger's PR32 references describe an unmerged checkpoint.

Some referenced code only exists on that checkpoint. The pinned
[legacy reference index](../v1-legacy-references.json) records those exact
references and historical file hashes. It allows offline navigation validation
without importing unmerged product changes or fetching Git objects in CI.
It is historical provenance, never proof of current code. The decision path
requires current Git blobs, native observations and new independent reviews.
Changes to normative source hashes fail closed and require deliberate re-audit.
The current [source binding](../v1-release-source-bindings.json) explicitly pins
both current sources and the immutable historical ledger. The audit and decision
entry points use that binding. Only historical review line positions may move:
each original section/content hash must still match exactly one current block.
A changed or ambiguous reviewed block rejects instead of inheriting its old
review. All other historical review fields and remaining obligations survive;
new clauses stay pending individual review. `--ledger` accepts an explicit ledger
without this rebinding and still enforces its exact source hashes. The binding
is an inventory update, never release acceptance.

## Trust and acquisition

An authorized release evaluator acquires the original native reports, raw
observations, authenticated GitHub metadata, model/Judge receipts and named
reviews. Keep the complete pack in a dedicated durable local directory. Obtain
its final SHA-256 through the independent evaluator/review channel. Supply that
hash explicitly; do not calculate a hash from an untrusted attachment and treat
the result as authentication.

Hashes detect substitution and bind evidence. They cannot prove that an actor
is human, an external model ran, a screenshot is genuine, an assertion covers
the whole normative clause, or a file came from GitHub. Those facts belong to
authenticated acquisition and independent review. The pack's authority must
check them before providing the trusted digest. A self-authored PASS, fixture,
approval word, merged-PR count or green CI is insufficient. There is no
automatic importer that upgrades historical reports to current receipts.

```sh
python3 scripts/release_decision.py \
  --pack /absolute/durable/release-pack/pack.json \
  --pack-sha256 TRUSTED_DIGEST \
  --evidence evidence/release-decision.json --require-complete
```

Run from a checkout of the exact release SHA. Output must be under repository
`evidence/`, outside the input directory. No input is modified. Every referenced
file uses a relative `path` and byte `sha256`; missing files, traversal, symlinks,
changed bytes and duplicate JSON keys fail closed. An invalid rerun replaces a
prior success with a rejected report. Exit 0 with `--require-complete` means
all supplied evidence gates passed. Exit 2 means inconclusive; exit 1 means
rejected evidence or an observed failure. Without the flag, exit 0 may describe
an inconclusive report: inspect `status` and `release_ready`.

## Private pack contents

The pack has version 1, an author, and a binding containing the exact
`release_sha`, both normative source hashes, `scoring_sha256` and
`suite_sha256`. All structured receipts carry this same binding. The suite is
hash-bound to an independent acceptance-mode scope review; it is not a policy
language. It freezes the assertion inventories, applicability and six variants
for each of the 20 Cases, both canonical model identities, SC-01–12 and the five
protected safety topics. Scope reviewers must check all subclauses, including
descriptive source blocks, rather than dropping requirements by keyword.

The executable synthetic pack builder in
`scripts/test_release_decision.py:PackFixture` shows the exact private layout.
Its files are marked synthetic and are only consumer mechanism tests. Never
submit them as actual product evidence. It is intentionally not an evaluation
producer or a public schema. The fields consumed by the two private scripts
are authoritative; changing those fields requires corresponding producer and
consumer verification.

| Pack field | Required evidence |
| --- | --- |
| `clauses` | One disposition per exact source identity. `verified` needs current code/test path, symbol and blob hash; merged PR receipt plus matching original GitHub metadata and reachable merge commit; native assertion/raw evidence; independent acceptance review binding all those references. Missing clauses remain `unmet`; explicit `rejected`/`expired` remain visible. |
| `trials` | Two arms × 20 Cases × six variants × five repetitions, each with a unique native execution identity, `live_model`/whole-case eligibility, cleanup, assertions and all eight hard-failure checks. Each row references separate native trial and Judge receipts, avoiding circular hashes. |
| `suite` | Reviewed assertions and dimension applicability; original input/fixture, Context Builder/context, Capability, Tool, Skill, configuration, binary, seed/timezone and limits must match for each corresponding trial in both arms. Model identities stay frozen separately. Only installation IDs/timestamps are normalized; all other fields, including future model fields and local endpoint hash, participate. |
| `systems`, `safety` | All SC-01–12. SC-03–07 and security/approval/cancellation/Durable Replay/Coding workspace protection require ordered, distinct rounds 1–5. Receipt reuse, reordered rounds and missing assertions fail closed. |
| `operations` | Native test/lint/race/Sidecar/e2e/fault reports, actual kill/restart, encrypted backup/restore and upgrade observations; each has raw evidence, command, producer and independent review. |
| `live` | Current search/login/Mail/Calendar/Feishu/browser/Computer/Jianying L2 records with model, time, failure class, environment, actual state, artifacts and named human review. Current SHA and review/observation dates at or after its commit are required. Historical L2 and prerequisite triage cannot substitute. |
| `native_ci` | Original GitHub run/job snapshots for current-SHA CI and Native packages workflows, matching repository, run/attempt IDs and job inventories; full Ubuntu needs join explicitly bound to the same source/run/attempt; four native package jobs across Linux/macOS architectures. |
| `human_acceptance` | Named member acceptance binding the complete pack excluding this final reference. It cannot be supplied by the pack author or an agent review. |

Independent review receipts name actor/type, origin, reason, subject hash,
review/expiry timestamps and raw review evidence. Only acceptance PASS can
verify its subject. Checkpoint, wrong subject, missing/unnamed/self-review or
future review rejects; rejection and expiry cannot become success. The trusted
acquirer verifies the asserted identity/type against its original source.

## Scoring and proof limits

The Judge only receives Goal, allowed Fixture, visible conversation, final
Artifact summary and exact programmatic assertions. Its native response must
contain O/F/P/A/C, with reasons and resolving evidence citations. T/S/R come
from the native mechanical receipt. Every dimension has an explicit numeric
or reviewed N/A disposition. A missing dimension rejects; deliberate null
scores remain inconclusive. Judge and trial/input hashes must match.

The consumer recomputes scores with the existing `case_runner.score`, including
N/A weight redistribution. Failed assertions and any hard failure override
Judge quality with zero. All observed runs must pass the 80 threshold; both
arms must reach mean 85 and minimum 80. Thresholds use unrounded values.
Reports retain each Case's mean/minimum, dimensions, per-variant five-run variance,
pass@1/pass^5, model identity and individual metrics. Unknown resource/latency
measurements remain null; they are not invented and keep the reporting gate
inconclusive until the required observations are supplied.

The consumer validates provenance bindings and recomputes grading, not external
reality or assertion completeness. Native producers and independent normative
review own those proofs. The source inventory is always retained separately
with its original inconclusive status. A positive combination test proves that
the consumer can converge when real obligations are supplied, not that this
project currently satisfies them. Actual missing model runs, current L2,
named human review and other release gaps remain with ERII-223.

## Verification and cleanup

```sh
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

The maintained fixture launches isolated synthetic file stores, checks native
bindings, drives positive/inconclusive/rejected CLI paths, reads reports back,
and removes its own stores and outputs. Regressions cover stale SHA/source,
model/condition drift, duplicate executions/source clauses, missing/invalid
Judge evidence, null grades, thresholds, hard-failure priority, protected
rounds, unmerged PR, historical L2, missing humans and native CI joins. They
need no network, account, model invocation or copied credentials. Operator
reports remain disposable; retain original evidence in the durable evidence
root until release review is resolved.

Negative receipt contracts call the production trial or operational-gate
validator directly. Complete composition, missing-evidence decisions and real
CLI tests still exercise the full join, including all source blocks and both
arms' 20 Cases, six variants and five repetitions. This keeps ordinary CI and
minimum-Python verification bounded without repeating unrelated valid cohorts
for every individual rejection. Assertion inventories, thresholds, repetitions
and CI time limits remain unchanged.
