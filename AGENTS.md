# Repository Guidelines

## Project Structure & Module Organization

Steward’s Go entry point is `cmd/steward/`; domain packages and adapters live in `internal/`. Keep `internal/app/` focused on composition. React/TypeScript sources live in `web/src/`; generated assets in `internal/interaction/web/assets/`. Model protocols live in `internal/llm/provider/`; the isolated RPC client/server share `internal/llm/bridge/`. The Node execution sidecar lives in `runtime/javascript/`; `provider-bridge/` retains the compatibility launcher and protocol fixtures. `guest-browser/` contains browser checks; `tests/{system,harness,fixtures}/` holds system flows, Python regressions, and fixtures. Operational helpers belong in `scripts/`.

## Build, Test, and Development Commands

Use Go 1.26.10+, Node 24.21.0 (`.node-version`), Python 3.10.12+, bash, and a C compiler.

- `make setup`: install pinned dependencies.
- `make build` / `make run`: compile `bin/steward` / build and start locally.
- `make test lint`: run Go, Node, browser, frontend-output, Python, formatting, type, documentation, and workflow checks.
- `make test-prompts`: check Main/Root/Child system ordering, frozen inheritance and Root progress projection.
- `make test-e2e E2E_EVIDENCE=evidence/my-change`: run core system flows; always choose a fresh evidence directory.
- `make test-full` / `make test-race`: extended system coverage / Go race checks.
- `make audit`: query current Go/npm advisories; requires network access.
- `python3 scripts/check_docs.py`: validate documentation-only changes.

## Coding Style & Naming Conventions

Use `gofmt`, tabs, lowercase Go packages, and interfaces defined by consumers. Keep adapters within domain subpackages. JavaScript/TypeScript uses two-space indentation, camelCase functions, PascalCase React components, and kebab-case component filenames. Linting includes `go vet`, pinned Staticcheck, TypeScript, and Node syntax checks.

## Testing Guidelines

Use Go `testing` (`*_test.go`, beside sources), Node `node:test` (`*.test.mjs`), and Python `unittest` (`test_*.py`). Coverage targets observable scenarios, without a fixed percentage. Run focused checks first; cross-module changes require `make test lint` and relevant system gates. Follow the [testing guide](docs/test-plans/product.md); report skipped/unrun checks.

Use disposable `STEWARD_DATA_ROOT` directories. Close Python SQLite connections with `contextlib.closing`; use read transactions for consistent cross-table predicates.

## Commit & Pull Request Guidelines

Use imperative subjects with customary prefixes, e.g. `feat(web): improve task navigation`, `fix:`, or `refactor:`. Keep PRs focused. Follow the [PR template](.github/pull_request_template.md): describe behavior, link existing issues, record exact validation/evidence, and identify compatibility implications and residual risks. Small fixes need no issue.

Update the [Feature Map](docs/feature-map.md), owning feature, design, and test plan when behavior changes. [Documentation policy](docs/README.md#强制维护规则) requires current topics; keep development logs and execution results in PRs or CI artifacts.

## Security & Agent Instructions

Follow the [engineering workflow](docs/features/engineering.md), [runtime contracts](docs/designs/runtime-contracts.md), and [governance](GOVERNANCE.md). Preserve approvals, frozen snapshots, and durable replay. Store credentials only in SecretStore. Releases require explicit authorization; remote branch deletion requires governance checks and exact-head leases.

Derive README/favicon artwork from the current Ghost atlas; `web/artwork/source/` is build input. Preserve third-party notices.

Connector implementations must be Steward-owned. Do not add external agent SDKs, vendor plugin implementations, or introduce their runtime-specific names. Reference behavior and public protocols while implementing adapters locally; general-purpose parsing libraries are allowed. Preserve required third-party attribution for retained code.
