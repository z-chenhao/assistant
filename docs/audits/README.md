# Bounded baseline audits


## Maintaining shared references (ERII-369)

The five ledgers `baseline-12`, `baseline-13`, `golden-common-01`,
`golden-common-02` and `golden-common-03` now have explicit
`reference_maintenance` records. Their entire original content is independently
sealed in `release_package.REFERENCE_LEDGER_ORIGINALS`, with the merged ledger's
commit/path, byte hash and canonical JSON hash. Original frozen blocks, reference
hashes, `audited_base`, native observations, merged PRs, limits and gaps remain
unchanged. The current record covers the 14 references affected by GC-01 plus the CI
workflow changed to supply the required Git history. Each snapshot binds its
explicit committed SHA; the workflow is the first changed reference exercised
by this repair itself. This does **not** validate
GC-01 PR #65's changed bytes; that branch must refresh after merging this repair.

A normal committed shared-code change must fail the audit until explicitly
refreshed. From that same branch, prepare each affected ledger through the
maintained command (repeat for the five names above):

```sh
python3 scripts/maintain_audit_references.py \
  --ledger docs/audits/baseline-12.json \
  --source-sha "$(git rev-parse HEAD)" \
  --reason 'Recheck committed shared-code identities; native reruns remain required' \
  --output evidence/baseline-12-proposal.json
# Inspect the proposal, then explicitly replace only this ledger.
cp evidence/baseline-12-proposal.json docs/audits/baseline-12.json
make audit-baseline-12
```

The command discovers drift and retains the previously explicit reference scope.
`--reference PATH[#SYMBOL]` may additionally select an unchanged reference.
It writes only a disposable proposal under `evidence/`, removes stale output on
failure, and never overwrites an input. Capture requires a committed source
revision descending from the original merged ledger and reachable from HEAD.
The original merged ledger must remain an ancestor of HEAD. Tests need its
reachable history; CI fetches history only in the two Python regression jobs.

Maintenance envelope version 2 uses the explicit binding
`source_commit_tree_and_HEAD_blob`. It retains the exact source SHA and base64
Git commit/path-tree object bodies (no duplicate blob bodies). Every audit
reconstructs those objects in a disposable bare object store with native Git,
checks their object IDs, resolves each exact file path/mode, and compares its blob
with both current HEAD's committed entry and the current file bytes. SHA256 and
symbol checks remain mandatory. The original merged ledger's raw/canonical seals
and Git blob checks are unchanged. Object-store cleanup is automatic; no network
fetch or write to the user's Git objects occurs.

A source commit need not be an ancestor of the squash result: squash preserves
file objects while replacing commit ancestry. The full source commit/tree proof
remains independently checkable even when a fresh clone has **no** source commit
object. This is an explicit file-identity binding, not an ancestry claim or a
fallback on missing evidence. All audits always check the proof, even before
merge; missing/corrupt proof objects or a HEAD/blob/mode mismatch reject. The
unreleased version-1 envelope is rejected and must be explicitly refreshed.
The source SHA, reference hashes and all original historical identities in these
five migrated records are retained.

`audit_package` independently validates the original seal, merged ledger blob,
source commit/tree proof, HEAD blob, current bytes and symbol on **every** maintained audit,
including the ordinary CLI. Removing the envelope and rewriting original
hashes still fails the independent seal. Its report exposes both reference hashes,
`current_reference_git_checked` for the explicit snapshot and per-reference
`current_git_blob_checked` / `historical_git_blob_checked` flags plus
`source_git_proof_checked`. All old observations remain readable but have
`observation_applicability.status=historical_only` and
`current_native_revalidated=false`. A refresh runs no native commands. It never
proves changed behavior, inherits PASS, fills a gap or certifies release. New
native observations require separate execution and receipts in the consumer's
acceptance evidence. Existing proof limits and routes still apply.

`--verify-git` additionally retains the original audited-source ancestry, source
blob and all historical reference/merged-PR checks. It is deliberately stricter
than reference maintenance. Some existing `audited_base` commits predate squash
merges and are not ancestors of main: that strong mode still rejects them.
Maintenance does not silently turn historical identity checks into ancestry
proof. Missing original manifests/native roots or rejected full ancestry remain
explicit evidence gaps. Normal reference maintenance is not a repair of those
gaps. `--require-complete` still exits 2; scores remain null.

The isolated Git fixtures in `test_release_reference_maintenance.py` reproduce
normal edits and a second edit, then reject forged hashes/SHAs/blobs, changed old
receipts/limits/gaps/PRs, absent provenance/revalidation/invalidation, missing
symbols, uncommitted bytes, duplicate frozen blocks and ambiguous source units.
Run `python3 -m unittest discover -s scripts -p 'test_release_*.py'`.

The maintained delivery-history harness drives all five real audit CLIs through
the registered squash shape, a fresh clone with pre-squash source commits absent,
rejected consumer drift, explicit consumer refresh, a real merge from advanced
main, and another squash/fresh clone with the newly captured source absent:

```sh
python3 scripts/verify_audit_references.py --evidence evidence/audit-reference-history.json
```

Run after committing the repair: it validates the committed HEAD tree. The report
records source/base/squash/merge IDs and 26 command outcomes, including the
post-squash baseline-12 regression. Adjacent raw reports are retained for manual
runs. `RegisteredSquashAuditTests` includes the same path in the release/Python
regressions; its disposable repositories and reports are cleaned. Neither the
harness nor tests mutate the caller's branch or any remote reference. This
mechanical history fixture certifies no product or release behavior.
Remove disposable `evidence/*-proposal.json` and audit reports after inspecting
or retaining them; never remove the original ledger or native evidence root.

[golden-common-03](golden-common-03.md) records ERII-246's final 18 Golden
common blocks: 13 reference links and five evaluation-boundary clauses. Its
36 scoped duties preserve every background/deferred reason and all 21 mandatory
routes. Use `make audit-golden-common-03`; audit completion certifies no release.

[golden-common-02](golden-common-02.md) records ERII-245's 80 exact blocks from
GC-05's overview through the first three product references. Its 476 duties retain
all 467 mandatory/policy routes, three field maps, SC-01–13 and current ungraded
native observations. Use `make audit-golden-common-02`. ERII-367/368 own metric
and SC-13 consumer mechanisms; this audit certifies no product or release.

[golden-common-01](golden-common-01.md) records ERII-244's 80 unchanged Golden
common blocks through the GC-04 overview row. Its 359 scoped duties preserve
all 302 mandatory duties, 25 finite owners and current ungraded native
observations. Use `make audit-golden-common-01`; no product score or release
is certified. ERII-366 owns clock/seed preparation; actual sampling and final
reports remain downstream consumers.

[baseline-13](baseline-13.md) records ERII-243's 58-block M5–M7, acceptance and
deferred-choice audit. Its 352 scoped duties retain all 324 required/policy routes,
the independently audited ADR006 Task lifecycle change, current native observations
and 19 existing finite owners. Use `make audit-baseline-13`; no product requirement
or release is certified.

[baseline-12](baseline-12.md) records ERII-242's 80-block error, configuration,
startup, interface and M0–M5 audit. It retains 452 scoped duties, all 445
mandatory/policy routes, 28 configuration keys, 14 method maps and three finite
gap proposals. Use `make audit-baseline-12`; no product requirement is certified.

[baseline-11](baseline-11.md) records ERII-241's 80-block module, policy and
persistence audit. It retains 487 scoped duties, all 480 mandatory/policy routes,
19 field maps, the changed ADR006 Task identity and four finite gap proposals.
Use `make audit-baseline-11`; no product requirement is certified.

[baseline-10](baseline-10.md) records ERII-240's 80-block Memory, EventQueue,
progress and module audit. Its 318 scoped duties retain all 294 mandatory/policy
routes, current native observations and four finite gap proposals. Use
`make audit-baseline-10`; no product requirement is certified.

[baseline-09](baseline-09.md) records ERII-239's 80-block CUA transfer, context
and Memory audit. Its 284 scoped duties retain all 261 mandatory/policy routes,
current native observations and two finite gap proposals. Use
`make audit-baseline-09`; no product requirement is certified.

[baseline-08](baseline-08.md) records ERII-238’s 80-block Security, Computer and
CUA audit. Its 402 scoped duties retain 384 mandatory routes, current mechanical
receipts and five finite gap proposals. Use `make audit-baseline-08`; no product
requirement is certified.

[baseline-07](baseline-07.md) records ERII-237's 80-block Claim/Wake/cancel,
quota, monetary consent, Skill and Security audit. Its 362 scoped duties retain
all mandatory routes, native receipt limits and five finite gap proposals. Use
`make audit-baseline-07`; zero requirements are certified.

[baseline-06](baseline-06.md) records ERII-236's separate 80-block audit of
Loop messages/events, Child API, shared Tool execution and frozen replay.
Its per-obligation evidence and finite gap routes do not certify release or
alter previous package ledgers.

[baseline-05](baseline-05.md) records ERII-235's separate 80-block audit of Store
isolation, the JavaScript Host protocol, ToolInvocation and Main runtime messages.
Its 363 sub-obligations retain explicit scopes, evidence limits and finite routes.
Use `make audit-baseline-05`; no requirement is certified.

[baseline-02](baseline-02.md) records ERII-232's Task, prompt, Run and Automation
audit: 83 current blocks after the owner-approved [ADR006 reseal](baseline-02-reseal.md).
Original identities and remaining duties stay visible; no requirement is certified.
Run `make audit-baseline-02` through the same maintained entry point.

[baseline-03](baseline-03.md) records ERII-233's 80 frozen blocks, their 347
sub-obligations, current evidence and remaining routes. Use `make audit-baseline-03`.
Its completed dispositions do not certify product conformance or release.

[baseline-04](baseline-04.md) records ERII-234's separate 80-block audit of
account, Bridge, model failure and Code Mode contracts. Its ledger and current
observations do not change baseline-01 or the preserved historical ledger.

[baseline-01.json](baseline-01.json) disposes of the **80 exact blocks** assigned
to ERII-231 in the owner's frozen work-package manifest. It records 257 authored
sub-obligations: 48 blocks have partial mechanical evidence, 17 have evidence
gaps, seven describe policy, five provide background and three explicitly defer
long-term features. No requirement is certified as implemented or verified.

Each row retains source/section/original line/hash and full source text, current
line, sub-obligations, observations, current code/test/harness references, actual
merged PR provenance, native observation references, proof limits and remaining
duties. `references` binds current file bytes, with symbols where applicable;
there is no fallback to historical code. An indexed maintenance test is not
necessarily an executed test. The raw Go JSON stream identifies the actual 171
passing tests, including subtests.

The original [ERII-30 ledger](../v1-release-ledger.json) and its 122 partial/gap
reviews remain unchanged. Package reviews are reported separately, including
overlap with historical reviews. Counts never imply newly verified requirements.
The inventory and release decision retain their existing proof limits.

## Frozen scope and changes

The original manifest is 380773 bytes, SHA256
`0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`, from
baseline `d11e543ea32efdf784478f5959ea6565af87b646`. Its full 1585-block assignment
was checked locally for uniqueness. The canonical JSON hash of
`packages.baseline-01.blocks` is
`b95fc9fa9861ff62d5551f820ae56dac90f9ad33ecdbd2ad2ef743fcc315db95`.
The validator pins this digest independently of the authored dispositions.

ADR 006 / merged PR 45 changed Task clauses after this package's last block.
All 80 section/content hashes still match uniquely, from §0 L10 through §3.5
L164. The recorded current whole-file hash does not replace the frozen
manifest's whole-file hash. Current inventory has 1588 blocks after approved
additions. A changed/ambiguous package block rejects and needs coordinator
resealing with an explicit old/new hash mapping. Line moves require a unique
unchanged section/content hash.

## Run and inspect

```sh
make audit-baseline-01
python3 -m unittest discover -s scripts -p 'test_release_*.py'
```

The first command uses the maintained `release_audit.py` entry point and contacts
no network, account, model or VM. Doctor checks frozen identities, current
reference bytes/symbols, complete dispositions and finite gap routes. Drive reads
the ledger through the CLI. Evidence is `evidence/baseline-01.json`; inspect
`package_audit` and `source_units`. Cleanup may remove that disposable output.
Output cannot overlap an input or retained native directory. Invalid reruns
delete stale disposable reports. `--require-complete` still exits 2:
finishing this audit is not release acceptance.

For an independently acquired original manifest, retained native bytes and
full Git history, strengthen the read-only verification:

```sh
python3 scripts/release_audit.py \
  --package-ledger docs/audits/baseline-01.json \
  --work-packages /absolute/durable/work-packages.json \
  --native-root /absolute/durable/audit-20261024 \
  --verify-git --evidence evidence/baseline-01.json
```

The three inputs set explicit `manifest_checked`, `native_bytes_checked` and
`git_provenance_checked` flags. Without them the flags stay false: a shallow
CI checkout cannot inspect unavailable history or same-host files.
`--verify-git` checks audited blobs and merged PR ancestry without fetching;
missing objects reject. Byte hashes prove integrity, not the truth of a model
or VM observation. Independent review must judge semantic completeness of the
sub-obligations; the parser cannot derive that from counts or keywords.

Native observations remain under the registered durable evidence root in
`ERII-231/fd2a3534f5b0602c5bf9376c14912f45b1ff4a5c/audit-20261024/`.
The ledger records commands, source SHA, timestamps, sizes and hashes. Contracts
use real SQLite with deterministic peers. `verify_children.py` drives the actual
CLI/PTY, scoped Artifacts, approval recovery, direct Parent results, cancellation
and six SIGKILL/restart boundaries, then joins processes and removes disposable
state. Its scripted provider earns no semantic scores. Keep the multi-MiB trace
local; publish only a compact manifest.

## Remaining evidence

| Existing route | Remaining scope | Independent exit |
| --- | --- | --- |
| [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909) | Real-model Task decisions, continuation, recall, aggregation and startup/delegation chain | Original frozen dual-arm evaluations, native state and independent Judge receipts; unavailable resources stay inconclusive. ERII-230 delivered the entry point, not executions. |
| [ERII-331](https://multica.ai/erii-studio/issues/01a16bd2-481b-7404-8db1-971204d4f839) | Current Web/Feishu, isolated Guest, desktop/browser/Jianying, conflict and nonconflict operation, named human observations | Current SHA/approved environment/native state/Artifact/cleanup and named review; ERII-229's snapshot and deterministic peers cannot substitute. |
| [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420) | Final normative join, protocol/version compatibility, Task status truth table and unobserved standalone Run `external_event` wake | Retain unmet duties until current native evidence and independent review resolve them. Observed implementation conflicts need separate finite issues; this audit does not repair the product. |

These bounded release children are reused by semantics. Each row's `remaining`
field identifies its exact duties. Background/deferred rows retain explicit
reasons. Neither this PR nor a child being Done grants v1 release acceptance.
