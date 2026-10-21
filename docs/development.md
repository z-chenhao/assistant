# Development

Start with [CONTRIBUTING](../CONTRIBUTING.md) for repository conventions and
[getting started](guides/getting-started.md) for the user installation path.

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
[ci-inputs](../.github/actions/ci-inputs/action.yml).

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
| `scripts` | Reusable verification and CI mechanics; [feature map](../scripts/FEATURES.md) |
| `evals` | Golden Cases, locked fixtures and evaluation infrastructure |

Interfaces belong to their consumers. Named subpackages implement storage and
providers. Keep one Agent Loop and one Tool execution pipeline. The private
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
inventing a new drive script. [CI operations](ci.md) documents required shards,
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
approval authority in the Host. See [frontend provenance and contracts](../web/README.md).

## Configuration and data

`erii start` supplies installed bridge/sidecar paths and manages a durable loopback
broker on an assigned port. `erii -config /absolute/config.yaml start` and
`ERII_NATS_URL` retain the explicit external-broker deployment. Configuration is
strict version-1 YAML; unknown fields or invalid values fail closed.

`ERII_DATA_ROOT` overrides the default `~/.local/share/erii`. Keep disposable test
roots separate from user state. `status` can migrate databases; `doctor` only
checks installation files and executable versions. Production credentials live in
SecretStore. YAML contains references, never keys or tokens.

[Model setup](guides/provider-bridge.md), [operations and backups](guides/operations.md),
and [the example config](../config.example.yaml) describe advanced controls.
No command implicitly enables a paid, default or Fake model. Test injection stays
explicit and never becomes the production fallback.
