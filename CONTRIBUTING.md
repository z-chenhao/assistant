# Contributing to Erii

Read [AGENTS.md](AGENTS.md) and the relevant specification before changing behavior.
Use the smallest complete change, preserve existing work, and include inspectable
evidence. [Development](docs/development.md) maps the modules and commands.

## Local workflow

```sh
make setup
make run
make test
make test-race
make lint
make verify
```

Use focused checks while editing, then proportional gates before review. Reproduce
bugs first. Update the affected [feature-map entry](scripts/FEATURES.md) and drive
the real CLI/UI/API boundary. Never claim an unrun check passed or turn historical
results into a new release claim.

Frontend source belongs in `web/`; Go web handlers and checked-in generated assets
belong in `internal/interaction/web/`. Rebuild the assets with
`npm run build --prefix web`. Keep model credentials and execution policy in Go.

Write feature guides in `docs/guides/`, approved decisions in `docs/adr/`, and
dated measurements in `docs/reports/`. Keep generated evidence out of Git. Update
[the documentation index](docs/README.md) when adding a guide. `make lint` checks
local links and the frontend build checks freshness.

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
