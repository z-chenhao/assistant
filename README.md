# Erii
A durable Personal Agent named Erii with tools, memory, task delegation, and recoverable execution.

The first runnable foundation is available. It initializes SQLite and reports
`setup_required`; it does not connect to a model or accept agent work. **M0 is
not complete.**

With Go 1.26.2+ and Python 3.10+:

```sh
make build
./bin/personal-agent -config config.example.yaml status
./bin/personal-agent -config config.example.yaml
# Ctrl-C stops the foreground process.
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
