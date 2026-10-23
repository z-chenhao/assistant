# Contributing to Erii

Read [AGENTS.md](AGENTS.md) and the relevant specification before changing behavior.
Use focused checks while editing and complete the proportional gates before review.
Reproduce bugs first. Update the affected [feature-map entry](scripts/FEATURES.md)
and use the [verification guide](docs/guides/verification.md) to drive the real boundary.

## Prerequisites and setup

Use Go 1.26.2+, Node 24.21.0, Python 3.10.12+ with SQLite and safe tar extraction
filters, and a C compiler for Go's race detector. Source dependency installation
needs network access. Once installed, tests use deterministic peers and disposable
state; they need no online account or separately installed broker.

```sh
make setup
make build
make run
```

On Ubuntu, install browser system libraries with
`node guest-browser/node_modules/playwright-core/cli.js install --with-deps chromium`.
Coding sandbox tests also need Bubblewrap; CI provisions it through
[ci-inputs](.github/actions/ci-inputs/action.yml).

## Layout and ownership

| Path | Owns |
| --- | --- |
| `cmd/erii` | Only Go process entry point, CLI parsing and presentation |
| `internal/app` | Composition, dependency lifecycle and shutdown |
| `internal/agent` | Shared Main/Worker loop, ownership and scheduling contracts |
| `internal/agent/sqlite` | Durable Run/Task state, fencing, replay and migrations |
| `internal/interaction` | HTTP/Feishu adapters and normalized ingress |
| `web` | Frontend TypeScript, styles, build tooling and upstream notices |
| `provider-bridge` | Pinned Pi model/auth bridge; no domain database or Tool access |
| `runtime/javascript` | Restricted QuickJS sidecar; no model credentials |
| `guest-browser` | Browser readiness and deterministic browser contract tests |
| `internal/startup` | Optional local broker deployment and process ownership |
| `scripts` | Reusable verification and CI mechanics; [feature map](scripts/FEATURES.md) |
| `evals` | Golden Cases, locked fixtures and evaluation infrastructure |

Interfaces belong to their consumers. Named subpackages implement storage and
providers. Keep one Agent Loop and one Tool execution pipeline. CLI command
handlers live in focused files under `cmd/erii`; `main.go` owns validation,
application admission and shutdown. The private
`agentLoop` phase dispatcher in `internal/agent/loop.go` handles model, tools,
reply and settlement; the Actor owns leases, cancellation and foreground joins.

## Checks and evidence

| Command | Required evidence |
| --- | --- |
| `make test` | Frontend freshness, Go/Node contracts and Python harness regressions |
| `make test-race` | Audited complete Go race inventory |
| `make lint` | Local documentation links, shell syntax, formatting, vet, module integrity, Node/TypeScript checks |
| `make verify` | Complete offline CLI, protocol, restart and Golden Case fragment gates |
| `python3 scripts/verify_startup.py --evidence evidence/startup.json` | Source and native archive installation, real PTY setup/chat, local broker crash/restart, reinstall and cleanup |

`evidence/`, `bin/`, `dist/` and dependency directories are ignored. Every harness
owns its temporary state and exact child processes. Evidence describes observations
and limitations, including unexecuted Live Shadow work. See the feature map before
inventing a new drive script. [CI operations](docs/ci.md) documents required shards,
exact-SHA artifacts and fail-closed joins.

## Working on a change

1. Define the observable result, non-goals and evidence that will prove completion.
2. Inspect the entry point, callers, state owner, failure path and nearby tests.
   Read only the relevant baseline/Golden Cases. Identify exact conflicts between
   specification and code; ask when a choice changes product behavior, protocol,
   compatibility, security or Durable Replay. Continue safe diagnosis meanwhile.
3. Reproduce a bug before editing, or record why a safe reproduction is infeasible.
4. Build one complete slice through the real boundary. Include required migration,
   deterministic Fake, regression and minimal documentation. Do not edit `sources/`.
5. Drive the CLI/UI/API or restart path and inspect authoritative output/state.
   Run the risk-based checks below, fix failures caused by the change and rerun
   affected checks. A build or an agent's self-report alone is not proof.
6. Inspect the diff and remove temporary artifacts, debug code, dead paths and
   unrelated changes. Documentation must match observed behavior.
7. Report outcome, exact checks/results, cleanup, risk/proof gaps and unresolved
   decisions (`None` when empty). An unavailable required check is `inconclusive`.

Proceed autonomously with scoped reversible edits, local inspection, disposable
services, Fakes and tests. Preserve user changes; never overwrite, revert,
reorder or include unrelated edits. Do not use destructive Git commands, bypass
hooks, swallow failures, weaken tests or relax assertions. Ask only for a real
product decision, missing credential, permission boundary, destructive operation,
production/external side effect or material scope expansion. Explain material
license, cost, security or architecture impact before adding a production dependency;
add one only for a current requirement.

### Match verification to risk

| Change | Minimum evidence |
| --- | --- |
| Docs, comments, local naming | Inspect diff/rendered text; format when relevant |
| Logic or bug fix | Focused regression and important boundaries |
| State, schema, persistence | Legal/illegal transitions, empty/prior migration, CAS/conflicts |
| Tool, approval, side effect | Contract, frozen parameters, rejection/retry, crash before/after effect |
| Concurrency, cancellation, replay | Duplicate/late/out-of-order events, old epoch, race detector, kill/restart |
| UI, CLI, API, Artifact | Running user path and inspectable output |
| Performance | Same environment/input/harness before and after; samples and noise |
| Milestone/release | All required checks, fault injection and due Golden Cases |

Every external dependency needs a deterministic CI Fake. Real accounts, websites
and macOS VM compatibility belong to release-time Live Shadow evidence.
`M0 → M7` are runnable slices: M0–M5 is Core MVP; M6–M7 completes the defined
first release. An earlier red milestone blocks an alternative architecture.
Each milestone includes migrations, Fakes, tests, CLI reproduction, operations
notes, kill/restart evidence and due Golden Cases. Internal M2/M4 builds must not
claim unimplemented Durable/Security semantics. Golden Cases require 80+ each,
85+ suite average, no hard failures; security, approval, cancellation, Durable
Replay and coding workspace protection require five consecutive passes. Record
an approved architecture deviation in an ADR and update the baseline before merge.

### Maintain the verification path

Use an existing harness before writing new drive mechanics. Each important UI,
CLI, API and Computer workflow needs isolated Launch, dependency/Fake/state
Doctor, Drive through the real boundary, inspectable Evidence and joined-process
Cleanup that preserves user data. Keep the feature map current in the same change.
Promote repeated mechanical actions into reusable commands. Harness observations
do not replace code review, measured performance, security or replay tests.
Prove one real task end to end before widening parallelism or unattended automation.

Parallel slices need independent scope/file ownership, concrete inputs and finish
conditions, their own checks/output and one named integrator. Do not parallelize
an unknown root cause, shared state machine, migration or competing implementations
of one protocol. The integrator inspects diffs and evidence. Improve weak
verification before adding agents.

### Find history and exact-source evidence

Check the source identity first; a local checkout may predate `origin/main`:

```sh
git status --short --branch
git rev-parse HEAD origin/main
git log -S 'search text' --oneline -- path/to/file
git log --follow --oneline -- path/to/file
gh pr view PR_NUMBER --json url,headRefOid,mergeCommit,statusCheckRollup
gh run list --commit FULL_SHA --workflow ci.yml --limit 5 --json databaseId,headSha,attempt,status,conclusion
```

PR identity, source SHA and run **attempt** must match the evidence being assessed.
Read [the verification reports](docs/guides/verification.md#evidence-and-failure-diagnosis)
and [CI artifact identities](docs/ci.md) before downloading. Keep complete evidence
under the registered durable local root, outside disposable worktrees. A small
manifest records issue/repository/full SHA/run/attempt, commands, exit codes,
versions/profile, relative paths, sizes and SHA-256. Same-host review verifies
hashes and reuses raw evidence read-only, while independently checking a clean
exact-SHA checkout. Missing evidence is inconclusive, never a passed check.

[ERII-206 / PR #37](https://github.com/z-chenhao/erii/pull/37) owns the maintained
collector and exact-artifact cache; reuse its documented entry point once that
change is present in your revision. [ERII-207 / PR #38](https://github.com/z-chenhao/erii/pull/38)
owns locked setup/cache diagnostics. Neither is assumed available in older
checkouts. Reuse package-manager downloads and pinned browser binaries; do not
share mutable `node_modules`. Default remote delivery is a compact manifest and
necessary diagnostics, at most 1 MiB per run. Do not recursively bundle dependencies,
browsers, prior ZIPs or historical logs. Required final exports remain required;
prepare them once with explicit size/purpose. Retain raw evidence until the issue
and review/repair resolve; worktree cleanup must not remove it.

### Turn repeated corrections into infrastructure

Prefer, in order: an architecture/API that prevents the bad state; compiler/types/
lint/CI; regression/fault test/harness; focused Skill or nested instructions; a root
rule only for repeated cross-repository invariants. Review comments discover the
missing control. Remove bad examples and stale workarounds. Keep Skill descriptions
short and route to relevant resources. Audit root instructions for stale/duplicate/
contradictory rules and remove rules made redundant by stronger controls.

Use short active sentences, stable terms and concrete evidence. Cross-agent briefs
state goal, inputs, constraints, evidence and stop condition. Use tables for exact
mappings and diagrams for state/ownership/branches. Review aids are disposable
unless requested as product artifacts. Do not omit uncertainty to shorten a report.

## Frontend changes

```sh
npm run build --prefix web
npm run check --prefix web
npm run lint --prefix web
python3 scripts/verify_channels.py --binary bin/erii --evidence evidence/channels.json --ui-components-only
```

Commit generated assets with the source. The Go binary embeds the local bundle;
`make test` rejects stale output. Keep credentials, model calls, Task control and
approval authority in the Host. See [frontend provenance and contracts](web/README.md).

## Configuration and data

`erii start` supplies installed bridge/sidecar paths and manages a durable loopback
broker on an assigned port. `erii -config /absolute/config.yaml start` and
`ERII_NATS_URL` retain the explicit external-broker deployment. Configuration is
strict version-1 YAML; unknown fields or invalid values fail closed.

`ERII_DATA_ROOT` overrides the default `~/.local/share/erii`. Keep disposable test
roots separate from user state. `status` can migrate databases; `doctor` only
checks installation files and executable versions. Production credentials live in
SecretStore. YAML contains references, never keys or tokens.

[Model setup](docs/guides/provider-bridge.md), [operations and backups](docs/guides/operations.md),
and [the example config](config.example.yaml) describe advanced controls.
No command implicitly enables a paid, default or Fake model. Test injection stays
explicit and never becomes the production fallback.

## Pull requests and CI

Use an owning issue identifier in the branch and PR title. Describe the behavior,
affected feature-map entries, exact checks, observed artifacts, cleanup and gaps.
Keep the PR draft until implementation and required checks are complete. The
stable `Ubuntu checks` aggregate requires every offline shard; docs-only changes
do not skip safety gates. See [CI operations](docs/ci.md).

## Native packages

```sh
sh scripts/package.sh "$PWD/dist"
```

A package contains the native `erii` binary, lockfile-installed model and JavaScript
runtimes, the installer, and user documentation. Node remains an explicit runtime
prerequisite. Native packaging preserves the macOS credential-store build.
The package workflow builds Ubuntu and macOS archives and retains checksums.
A version tag creates a draft release for maintainers to inspect before publishing.
Do not tag or publish a release as part of an ordinary refactor.

## Branch maintenance

```sh
python3 scripts/clean_branches.py
python3 scripts/clean_branches.py --apply
```

The default only plans branch changes. Apply removes branches whose tips are
ancestors of `origin/main` or match the exact head of a merged GitHub PR, excluding
main, every checked-out worktree branch and open PR heads. Local deletion also
requires Git's merged/upstream guard; there is no `-D` fallback. Remote deletion
uses an exact-tip lease. Unique local snapshots and branches without merge proof
stay for inspection. Stale remote-tracking refs are pruned after apply. The command
writes an audit report to `evidence/branches.json`.
