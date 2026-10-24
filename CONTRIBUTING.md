# Contributing to Erii

Read [AGENTS.md](AGENTS.md) and the relevant specification before changing behavior.
Use focused checks while editing and complete the proportional gates before review.
Reproduce bugs first. Update the affected [feature-map entry](scripts/FEATURES.md)
and use the [verification guide](docs/guides/verification.md) to drive the real boundary.

## Prerequisites and setup

Use Go 1.26.2+, Node 24.21.0, Python 3.10.12+ with SQLite and safe tar extraction
filters, and a C compiler for Go's race detector. Cold dependency installation
needs network access. Once installed, tests use deterministic peers and disposable
state; they need no online account or separately installed broker.

```sh
make setup
make build
make run
```

On Ubuntu, install browser system libraries with
`python3 scripts/setup.py --part browser --with-deps` after package setup.
Coding sandbox tests also need Bubblewrap; CI provisions it through
[ci-inputs](.github/actions/ci-inputs/action.yml).

### Repeated and offline setup

`make setup` is the maintained development/test installation entry point. CI uses
the same `scripts/setup.py` in two parts: packages once, then browsers only in
jobs that need them. CI selects its consumed package roots with `--packages`;
local setup still installs all four by default. Checks never reinstall dependencies. Source/release installer
tests deliberately install into their own isolated destinations; those are real
installation contracts, not redundant development setup.

```sh
make setup SETUP_FLAGS=--measure-network
make setup SETUP_FLAGS='--mode offline --measure-network'
# Select a disposable cold cache, or a retained download cache shared by worktrees:
npm_config_cache=/absolute/path/npm-cache make setup SETUP_FLAGS=--measure-network
```

Keep npm's native download cache (default from `npm config get cache`) and
Playwright's native browser cache (or an absolute `PLAYWRIGHT_BROWSERS_PATH`).
Do not copy or share `node_modules`. Local setup runs `npm ci` against all four
lockfiles; CI installs its selected lockfiles, retaining the existing `--ignore-scripts` policy and online audit.
`--prefer-offline` reuses content first; online mode fetches missing objects.
Offline mode never falls back to online. npm verifies cached tarballs with its
native integrity checks. Its offline "0 vulnerabilities" output does **not**
establish advisory coverage; run online setup/audit for current advisories.

`evidence/setup.json` records Node/npm/platform/architecture, each lock's SHA256,
actual cache locations, stage timings and failure. Node must match `.node-version`.
The cache is content addressed rather than partitioned by lock digest: npm uses
the current lock/integrity and platform constraints for every installation, so
unchanged objects remain reusable across lock changes. Playwright uses its pinned
package's platform/revision mapping. A browser Doctor checks install markers,
executability, both Chromium variants' reported version against `browsers.json`,
real page creation, and FFmpeg execution. This is a compatibility/smoke check,
not a cryptographic attestation of the entire browser tree. Browser garbage
collection is disabled during setup so another worktree's revision stays intact.
Go retains its native module/build caches; the copied offline CI SDK and private
product workspace caches keep their existing isolation contracts.

`--measure-network` additionally records request attempts, distinct successful
URL objects and response-body bytes by category, without credentials, headers or
URL queries. It observes Node `http(s).request/get`, as used by the pinned npm and
Playwright downloaders. It excludes TLS/IP overhead, sent bytes, undici/fetch,
browser subprocess traffic, apt and Go. Do not call it whole-machine egress.
Cold/warm describe cache state, not how many times an install command ran. A cold
npm cache with a warm browser cache is a partial cold measurement; label it.

Each stage (including native cross-worktree cache lock waits) has a 180-second
deadline, configurable with `--timeout` up to 600 seconds. HTTP attempts use a
30-second timeout with no npm transport retries. Playwright retains its native
bounded mirror fallback under the stage deadline. Timeout kills the owned process
group. Same-worktree setup serializes under `evidence/setup.lock`; separate
worktrees install privately and can concurrently read the native download caches.

If offline setup reports a missing/corrupt npm object, run online setup to fill
only missing objects, then retry offline. Lock/manifest conflicts require fixing
the lockfile, not deleting it or using `npm install`. A missing browser fails
offline immediately; online setup installs its locked revision. For a damaged or
incompatible browser, stop its consumers and remove only the named revision
directory shown by the Doctor, then run online setup. Do not wipe shared caches.
`--with-deps` is online only; offline hosts must already have OS browser libraries.
Inspect the failed stage and logs when its deadline expires; no required check is
skipped and no cache failure silently authorizes different dependencies.

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
