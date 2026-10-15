# Erii
A durable Personal Agent named Erii with tools, memory, task delegation, and recoverable execution.

The runnable foundation initializes SQLite and reports `setup_required`. It
includes transactional Outbox/Inbox, CloudEvents, and an optional NATS relay.
It persists model metadata and provides a versioned stdio Provider Bridge with
deterministic fixtures. Explicit `-test-bridge success` enables CLI capability
tests and atomic model selection without an account. Saved test profiles cannot
enable ingress without that injection. Real providers, login and Agent Runs are
M1 work; this build does not accept conversations or tasks.

With Go 1.26.2+ and Python 3.10+:

```sh
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
