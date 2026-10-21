# Erii

Erii is a personal agent with conversation, tools, memory, delegated tasks, and
recoverable execution. Use it in the terminal or through the local Web interface.

## Install and start

From a checkout, with **Go 1.26.2+** and **Node.js 24.21.0** installed:

```sh
./install.sh
~/.local/bin/erii
```

The installer builds Erii, installs its pinned model and JavaScript runtimes, and
creates the `erii` command under `~/.local/bin`. It never edits your shell profile
or selects a model. If that directory is on your `PATH`, just run `erii`.

First launch guides you through a **local OpenAI-compatible model** or
**Sign in with ChatGPT**. Erii starts its local event broker for you. Your data
stays in `~/.local/share/erii`. Type `/quit` or press Ctrl-C to stop.

```sh
erii doctor       # Check the installation without opening user databases
erii status       # Read model/setup diagnostics
erii start        # Explicit foreground launch
```

A native release archive includes the compiled application and both runtimes;
unpack it and run its `install.sh` with Node installed. Release packaging is
available to maintainers; no published release is required for source installation.
See [installation and first conversation](docs/guides/getting-started.md) for
requirements, upgrades, custom paths, and advanced configurations.

## Develop

```sh
make setup         # Install all pinned development/test dependencies and Chromium
make run           # Build and launch Erii
make test
make test-race
make lint
make verify
```

The frontend lives in [`web/`](web/README.md); Go HTTP handlers and embedded assets
live in `internal/interaction/web/`. The product runs as one Go application with
local Node sidecars. There is no separate frontend server to deploy.

- [Documentation](docs/README.md) — setup, feature guides, architecture, operations
- [Contributing](CONTRIBUTING.md) — layout, checks, releases, branch maintenance
- [Verification feature map](scripts/FEATURES.md) — real entry points and evidence
- [Technical baseline](docs/technical-design.md) — intended semantics and boundaries

Offline tests prove deterministic behavior. Real model accounts, macOS Computer
compatibility, and product-quality Golden Case scores require separate Live
Shadow evidence; a green offline build does not establish those results.
