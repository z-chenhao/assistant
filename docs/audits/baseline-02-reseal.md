# baseline-02 — coordinator reseal required

ERII-232 is **inconclusive**. The original 80-block package cannot complete its
current-source audit: ADR 006 / merged PR 45 changed two assigned blocks. No
changed-block obligations or conformance conclusions have been inferred. The
original manifest, package assignment, historical ledger and release gates remain
unchanged. This record is a reseal request, not a completed package ledger or a
release certificate.

The [machine-readable mapping](baseline-02-reseal.json) retains both complete
old/new texts and hashes, all 78 unchanged identities with relocated lines, and
three new clauses needing explicit assignment. It binds the original 380773-byte
manifest, SHA256 `0120228762abfe54dd477bc27853b3b45338d0ea5f19363a3d3b742224f5c556`,
baseline `d11e543ea32efdf784478f5959ea6565af87b646`, and audited current source at
`c1adab4e1fb23991255dce5a6424ec4b644ab381`. That commit merges main into the
existing task branch while retaining its required starting commit.

| Frozen block | Proposed current block | Change authorized in ADR 006 |
| --- | --- | --- |
| §3.9 L224 | §3.9 L225 | Manage union adds `complete` |
| §3.9 L237, fenced TypeScript contract | §3.9 L240, fenced TypeScript contract | Adds create `completionPolicy` and complete arm |

Current §3.6 L176 and §3.9 L229–230 are new blocks, rather than replacements of
the 78 unchanged blocks. Their assignment belongs to the coordinator. They have
not been appended to ERII-232's frozen scope. The approval provenance and upgrade
boundary are in [ADR 006](../adr/006-explicit-task-completion-policy.md).

## Preserved work and evidence

Complete local evidence is retained under the registered durable root:

```text
/Users/zhuchenhao/Projects/erii/evidence/agent-delivery/ERII-232/
  c1adab4e1fb23991255dce5a6424ec4b644ab381/audit-20261024/
```

- `reseal-request.json`: old/new source texts, hashes and unchanged relocation.
- `baseline-02.pending.json`: 78 preliminary reviews, 237 authored sub-obligations,
  44 current file/symbol hash bindings, merged PR provenance and proof limits.
  The two changed blocks have no obligation/conformance disposition. This draft
  is deliberately **not registered** with `release_package.audit_package`.
- `contracts.jsonl`, `contracts-receipt.json`, `passing-tests.json`: 174 passing
  Go tests including subtests, zero skipped, across Agent, SQLite, Task and
  Automation packages. Commands and source SHA are in the receipt.
- `release-regressions.log`, `release-regressions-receipt.json`: all 39 tests from
  `python3 -m unittest discover -s scripts -p 'test_release_*.py'` passed, including
  rejection of changed/ambiguous source, stale references and unsupported claims.
- `merged-prs.json`: native GitHub metadata for PRs 8, 10, 11, 12, 16, 22, 28, 33
  and 45; each merge commit is an ancestor of the audited source commit.

The draft reuses finite release routes [ERII-330](https://multica.ai/erii-studio/issues/01a16bd2-3352-7bc8-8c8c-fc6045822909)
for real-model semantic evidence and [ERII-332](https://multica.ai/erii-studio/issues/01a16bd2-5d15-7af2-bb7f-7e5ff3c3c420)
for the final current-source evidence join. Neither route being Done certifies
this package. Candidate naming differences (`CreateRun`/`ClaimRun`/`CheckpointRun`,
Context Builder, RunScheduler and RunExecutor versus the current private
constructors/Actor) remain explicit in the draft. They need independent judgment;
this audit neither repairs them nor silently treats them as approved deviations.

Go contracts used deterministic peers and real SQLite. They do not supply
real-model scores, current Live Shadow observations, CLI kill/restart evidence or
five consecutive protected runs. No native CLI harness, full suite, race suite,
exact-SHA CI or independent acceptance review was run for this incomplete audit.
The 78 preliminary reviews still require a complete ledger, maintained-entry-point
validation and independent review after resealing. Their count is not a verified
requirement count.

## Required next action

Coordinator must preserve the old manifest and explicitly reseal the two
old/new identities in the mapping, deciding ownership of the three additions
and recording any affected cross-package assignment. Return the authoritative
new manifest digest, exact package assignment and authorization reference before
Builder resumes those blocks. Do not replace the old hashes in place or relax
the package validator. Reuse unchanged notes only by unique section/content hash,
then recheck their current reference bytes and evidence identities.

There is no acceptance-review request or product release claim at this boundary.
