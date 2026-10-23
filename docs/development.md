# Development

Start with [CONTRIBUTING](../CONTRIBUTING.md) for repository conventions and
[getting started](guides/getting-started.md) for the user installation path.

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
[ci-inputs](../.github/actions/ci-inputs/action.yml).

### Repeated and offline setup

`make setup` is the maintained development/test installation entry point. CI uses
the same `scripts/setup.py` in two parts: packages once, then browsers only in
jobs that need them. Checks never reinstall dependencies. Source/release installer
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
Do not copy or share `node_modules`. Every setup runs `npm ci` against all four
lockfiles, retaining the existing `--ignore-scripts` policy and online audit.
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
