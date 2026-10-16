# Erii
A durable Personal Agent named Erii with tools, memory, task delegation, and recoverable execution.

The runnable foundation initializes SQLite and reports `setup_required`. It
includes transactional Outbox/Inbox, CloudEvents, and an optional NATS relay.
It connects local OpenAI-compatible models through a pinned Pi AI Node Bridge.
The CLI supports discovery, manual model IDs, capability probes, and explicit
activation, with optional Keychain/environment credentials. Deterministic
fixtures remain available through explicit `-test-bridge` injection.
ChatGPT account login, refresh, model discovery and logout are available through the CLI (see docs/provider-bridge.md). The terminal now supports setup, model controls and conversation through the same Main Agent Loop as `chat send`, `chat work` and `chat history`. See [terminal setup and M1 evals](docs/tui-and-evals.md) and [Main recovery](docs/main-loop.md). Tasks and Workers remain later slices.

With Go 1.26.2+, Node 24.21.0, and Python 3.10+:

```sh
npm ci --prefix provider-bridge --ignore-scripts
make build
./bin/personal-agent -config config.example.yaml status
./bin/personal-agent -config config.example.yaml
# Ctrl-C stops the foreground process.
# With a local JetStream broker, explicitly run the internal event publisher:
./bin/personal-agent -config config.example.yaml relay
```

```sh
make test
make test-race
make lint
make verify
```

See [local development and storage notes](docs/development.md) and the
[verification feature map](scripts/FEATURES.md) for configuration, evidence,
cleanup, and the precise limits of this slice.

See [local provider setup](docs/provider-bridge.md#connect-a-local-model) for the actual model path.

Task delegation, discovery, append and durable follow-up: [operations and limits](docs/tasks.md).

Tool policy, frozen approvals, recovery and deterministic CLI evidence: [docs/tools.md](docs/tools.md).
