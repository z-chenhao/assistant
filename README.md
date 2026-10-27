# Erii

[![CI](https://github.com/z-chenhao/erii/actions/workflows/ci.yml/badge.svg)](https://github.com/z-chenhao/erii/actions/workflows/ci.yml)

A durable personal agent with conversation, tools, memory and delegated tasks.
Use Erii in the terminal or through its local Web interface. Execution can recover
after a process restart, with frozen model snapshots and explicit approvals.

[Get started](docs/guides/getting-started.md) · [Documentation](docs/README.md) ·
[Contribute](CONTRIBUTING.md) · [Verify features](docs/guides/verification.md) ·
[Security](SECURITY.md)

## Capabilities

| Capability | Guide |
| --- | --- |
| Guided local model setup or Sign in with ChatGPT | [Model connections](docs/guides/provider-bridge.md) |
| Terminal conversation, Web and Feishu | [Terminal](docs/guides/tui-and-evals.md), [channels](docs/guides/channels.md) |
| Long-running Tasks and Child Runs | [Tasks](docs/guides/tasks.md), [delegation](docs/guides/children.md) |
| Tools, approvals and recoverable JavaScript | [Tools](docs/guides/tools.md), [Code Mode](docs/guides/codemode.md) |
| Scoped memory, recall and file artifacts | [Memory](docs/guides/memory-maintenance.md), [recall](docs/guides/recall.md), [artifacts](docs/guides/artifacts.md) |
| Automation, desktop and browser actions | [Automation](docs/guides/automation.md), [Computer Use](docs/guides/computer-use.md) |
| Diagnostics, encrypted backups and recovery | [Operations](docs/guides/operations.md) |

## Install and start

Source installation requires **Go 1.26.2+**, **Node.js 24.21.0** and npm on macOS
or Linux. Dependency installation requires network access.

```sh
git clone https://github.com/z-chenhao/erii.git
cd erii
./install.sh
~/.local/bin/erii
```

The installer builds Erii and installs pinned model/JavaScript runtimes. It creates
`erii` under `~/.local/bin`; add that directory to your `PATH` to invoke it directly.
First launch guides you through model setup, probing and explicit selection.
Erii starts its local event broker and stores data in `~/.local/share/erii`.
After setup, type `/quit` to stop. Within setup, use `/cancel` to return to
conversation first. Ctrl-C exits either screen.

```sh
erii doctor       # Check installation without opening user databases
erii status       # Inspect model/setup diagnostics
erii start        # Explicit foreground terminal launch
```

A native archive contains the application and runtimes; unpack it and run its
`install.sh` with Node installed. The package workflow prepares draft releases;
source installation does not require a published archive.
[Installation details](docs/guides/getting-started.md) cover upgrades, custom paths
and advanced configuration. Computer workflows have additional macOS Guest
requirements described in their guides.

## Architecture

Erii runs as one Go application with local model and JavaScript sidecars. Main
and Worker roles share one Agent Loop. SQLite owns durable state, and JetStream
provides wakeups. The Go Host owns tools, approvals and credentials. The frontend
bundle is embedded in the application; there is no frontend service to deploy.
See the [technical baseline](docs/technical-design.md) for protocols and boundaries.

## Development and verification

```sh
make setup         # Pinned dependencies and test Chromium
make run           # Build and launch
make test
make test-race
make lint
make verify        # Complete offline end-to-end inventory
```

[Contributing](CONTRIBUTING.md) documents prerequisites, module ownership and the
review process. [Verification](docs/guides/verification.md) explains focused gates,
full runs, evidence and cleanup; the [feature map](scripts/FEATURES.md) ties each
implemented path to its authoritative entry point and proof.

Offline tests use deterministic peers and need no online accounts. Real model
compatibility, macOS Computer compatibility and semantic Golden Case scores need
separate Live Shadow evidence. A green offline build does not certify them.

## Support and licensing

Use [GitHub issues](https://github.com/z-chenhao/erii/issues) for reproducible bugs
and proposals. Include the revision, command and sanitized evidence; follow the
[security policy](SECURITY.md) for vulnerabilities.

No project-wide license has been selected yet. Dependency and vendored notices
remain with their respective components; public source availability does not
supply a project-wide open-source license.
