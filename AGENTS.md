# Repository Guidelines

## Project Structure & Module Organization

Steward is a personal agent with a Go execution core. `cmd/steward/` is the process entry point; `internal/` contains domain packages and adapters such as `event/nats` and `memory/sqlite`. Keep `internal/app/` focused on composition.

`web/src/` holds TypeScript/React components and CSS; embedded assets live in `internal/interaction/web/assets/`. Node sidecars live in `provider-bridge/` and `runtime/javascript/`; browser checks live in `guest-browser/`. Go tests sit beside source files. `tests/system/`, `tests/harness/`, and `tests/fixtures/` hold system checks, Python harness tests, and fixtures. Operational helpers belong in `scripts/`.

## Build, Test, and Development Commands

Use Go 1.26.10+, Node 24.21.0 (`.node-version`), Python 3.10.12+, and a C compiler.

- `make setup`: install pinned development dependencies.
- `make build`: compile `bin/steward`; `make run`: build and start locally.
- `make test`: run Go, Node, browser, frontend-output, and Python harness checks.
- `make lint`: validate documentation, formatting, types, workflows, shell syntax, and dependencies.
- `make audit`: query current Go/npm advisories (network required; separate from deterministic regressions).
- `make test-e2e E2E_EVIDENCE=evidence/my-change`: run core deterministic system flows; use a fresh evidence directory per run.
- `make test-full` / `make test-race`: run extended system coverage / race checks.
- `python3 scripts/check_docs.py`: validate documentation-only changes.

## Coding Style & Naming Conventions

Format Go with `gofmt` (tabs); use lowercase package names and define interfaces in consuming packages. Keep adapters in named domain subpackages; avoid generic service/repository layers. Follow nearby JavaScript/TypeScript conventions: two-space indentation, camelCase functions, PascalCase React components, and kebab-case component filenames. Linting uses `go vet`, TypeScript, and Node syntax checks.

## Testing Guidelines

Use Go `testing` (`*_test.go`), Node `node:test` (`*.test.mjs`), and Python `unittest` (`test_*.py`). Run focused checks first, then `make test lint` and relevant system gates for cross-module changes. Coverage follows observable scenarios, without a mandated percentage. Exercise real stores/protocols for persistence changes and failure/restart/race paths for replay or concurrency. Report skipped and unrun checks explicitly. See the [testing guide](docs/test-plans/product.md).

Close Python SQLite connections explicitly with `contextlib.closing`; the connection context manager only commits or rolls back transactions. Use a read transaction when a predicate needs a consistent snapshot across tables.

## Commit & Pull Request Guidelines

Recent history favors imperative subjects with `feat:`, `refactor:`, or `chore:` prefixes and optional scopes, e.g. `feat(web): improve task navigation`. Keep changes focused. Follow the PR template: describe behavior, link an existing issue, provide exact validation commands/evidence, and identify residual risks. Update the [Feature Map](docs/feature-map.md), owning feature, design, and test plan when behavior changes.

## Security & Agent Instructions

Follow the [contribution workflow](docs/features/engineering.md) and [runtime contracts](docs/designs/runtime-contracts.md). See [governance](GOVERNANCE.md) for branch and release ownership. Preserve approval, frozen-snapshot, and durable-replay guarantees. Store credentials only in SecretStore. Tests must use disposable `STEWARD_DATA_ROOT` directories. Publishing releases requires explicit authorization.

Keep the README and favicon derived from the current Ghost atlas; `web/artwork/source/` contains build inputs, not public assets. Preserve third-party notices in native packages. Delete remote branches only after checking their exact head against merged PR evidence; retain unmerged work for review.
