# First runnable foundation

This slice implements process startup, strict versioned YAML configuration,
structured local logs and spans, forward SQLite migrations, and inspectable
`setup_required` diagnostics. It is a subset of M0 in technical-design.md
§17. No model is active and no agent ingress exists.

The sole process entry point is `cmd/personal-agent`. `internal/app` assembles
configuration, observability, and the Agent SQLite store and closes them in
reverse order. The Agent module owns readiness and its stored configuration.
There is no generic persistence interface or extension framework.

## Prerequisites and commands

- Go 1.26.2 or newer, matching `go.mod`.
- Python 3.10+ with the standard `sqlite3` module for the process harness.
- A C compiler supported by Go for `make test-race` (the application itself
  uses a pure Go SQLite driver).

The initial Go module download needs network access. After dependencies are
cached, all checks use local files, real temporary SQLite, and no accounts,
provider endpoints, broker, VM, or external services. There are no external
service integrations in this slice requiring Fakes.

| Command | Evidence |
| --- | --- |
| `make build` | `bin/personal-agent`, the only Go executable |
| `make test` | Configuration boundaries, stored readiness constraints, UUIDv7 identity, UTC microseconds, migration rollback and prior-version upgrade |
| `make test-race` | The Go tests with the race detector |
| `make lint` | `gofmt` check, `go vet`, module checksum verification |
| `make verify` | Real CLI launch, help, diagnostics, stored state, stop/restart, kill/restart, future-schema rejection, local logs/spans, and cleanup |

GitHub Actions runs all four checks on Ubuntu for pull requests and pushes to
`main`. The workflow reads the Go version from `go.mod`.

`make verify` writes `evidence/verification.json`. It removes an old report
before running, launches only its exact child binary, uses an isolated temporary
data root and working directory, and reaps children and removes temporary state
on success or failure. Its report contains observed startup diagnostics, a stored
row, log/span records and counts, and each passed assertion. Paths in that report describe
temporary state that has already been removed. The report and build output are
ignored by Git. See `scripts/FEATURES.md` for the entry point of each feature.

## Configuration and CLI

```sh
make build
./bin/personal-agent -config config.example.yaml status
./bin/personal-agent -config config.example.yaml
```

`status` initializes/migrates the configured database, prints one JSON object on
stdout, then exits zero. It is a diagnostic startup path, not a read-only
database inspector. With no command, the process prints the same object and
waits in the foreground. SIGINT/Ctrl-C and SIGTERM close the database and log
files and exit zero. Errors go to stderr with a nonzero exit code. Flags precede
the optional `status` command. `-help` creates no application state.

```json
{"readiness":"setup_required","agent_ingress_enabled":false,"agent_id":"<UUIDv7>","active_model_profile_id":null,"config_version":1,"schema_version":1,"state_database":"/tmp/erii-local/state.db"}
```

Without `-config`, built-in version-1 defaults use
`$HOME/.local/share/erii` and log level `info`. An explicit configuration path
must exist. The file must contain exactly one YAML document, an explicit
`version: 1`, and only known fields. Missing optional app fields inherit defaults.
Unknown keys, duplicate keys, unsupported versions, invalid types and log levels,
and relative data roots fail before storage opens. Parser errors do not echo
configuration values.

| YAML key | Deployment override | Accepted value |
| --- | --- | --- |
| `version` | None | `1` |
| `app.data_root` | `ERII_DATA_ROOT` | Absolute path |
| `app.log_level` | `ERII_LOG_LEVEL` | `debug`, `info`, `warn`, `error` |

Environment overrides apply after file decoding. Empty overrides are rejected.
Configuration cannot contain credentials, model endpoints, or active profiles
in this slice. There is no implicit Fake, paid provider, or network connection.

## Stored state and migrations

Only `state.db` is created. Conversation, memory, Artifact storage, and later
domain tables are deferred. New directories have mode `0700`; new database and
log files have mode `0600`. Existing permissions are preserved. Database paths
are independent of the process working directory and are URL-escaped before
being passed to SQLite.

Each SQLite connection enables WAL, foreign keys, and a 5-second busy timeout.
Concurrent first-open WAL setup can return `SQLITE_BUSY` without invoking the
busy handler. Connection initialization alone retries that known transient error
within a 5-second context; migrations and commits are never automatically retried.
The store limits itself to one open connection. An immediate transaction
serializes migration version checks and DDL. Schema versions use SQLite
`PRAGMA user_version`. Version 0 (including a newly created empty file) upgrades
to version 1. Version 1 creates only the bootstrap `agent_config` table. The
bootstrap transaction inserts one Agent identity and UTC microsecond timestamps
once; subsequent startups preserve every stored field and timestamp. Schema
constraints reject an active profile, `ready`, an invalid singleton, or invalid
configuration version/timestamps in this slice. There is no update API yet;
future aggregate updates must introduce CAS with their behavior.

Migration failure rolls back both DDL and the version marker. Newer unsupported
versions fail startup without applying migrations or rewriting stored rows.
There is no downgrade or automatic database deletion. Resolve a future-version
error by running the matching newer binary or restoring a separately verified
backup. Stop the process before copying a database; use SQLite's backup API for
online backup. Do not copy just `state.db` while a process may have WAL writes.

There is no previously released schema. A synthetic two-version test proves
forward upgrade and preservation of prior rows without introducing speculative
production migrations. Kill/restart verification covers **committed bootstrap
state**, not later Task/Run or Durable Replay behavior.

## Local observability

`logs/app.jsonl` uses `log/slog` JSON records with a PID and configured severity.
Startup logs carry `trace_id`/`span_id`. `logs/traces.jsonl` contains local
`app.startup` and child `sqlite.open_and_migrate` spans with random 128-bit trace
IDs, 64-bit span IDs, parent IDs, UTC start times, elapsed microseconds, and an
`ok`/`error` outcome. Spans omit raw errors and configuration payloads. Failed
database startup still records error spans when telemetry can be opened.

These are process diagnostics, not OpenTelemetry export, distributed context
propagation, metrics, or durable RunTrace. Writes are best-effort, files append
without rotation, and a hard kill can lose diagnostic output. Inspect these
files locally; stdout remains suitable for JSON parsing.

## Scope and dependencies

Direct dependencies are pinned: `modernc.org/sqlite` for SQLite without CGo,
`go.yaml.in/yaml/v3` for strict YAML, and `github.com/google/uuid` for UUIDv7.
They have permissive BSD-3-Clause/MIT/Apache-2.0 licensing (see each module's
license files) and no account or service cost. The SQLite translation adds
transitive modules and build size; there is no second storage implementation
or ORM. Dependency checksums are committed in `go.sum`.

NATS, Outbox/Inbox, ModelProfile activation, authentication/SecretStore, model
bridge, Task/Run, tools, memory, UI, and later milestones are not implemented.
The full M0 completion conditions therefore remain unmet. Product Golden Cases
and SC-01–SC-12 depend on later capabilities and are `not_applicable` to this
slice, not passed. No Durable or Security milestone claim is made.

For manual experiments, choose a dedicated disposable data root and stop your
process before removing that directory. The automated harness needs no manual
cleanup and never uses your configured application data root.
