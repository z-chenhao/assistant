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
