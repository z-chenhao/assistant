# Foundation feature map

Run `make verify` from the repository root. The stable mechanics live in
`scripts/verify.py`; the harness uses the built binary through the same CLI
boundary as a user, then independently reads SQLite and JSONL evidence.

| Feature | How to reach it | Authoritative entry point | Inspectable evidence |
| --- | --- | --- | --- |
| Help/doctor | `bin/personal-agent -help` | `cmd/personal-agent/main.go:run` | Usage output; no data created |
| Startup diagnostics | `bin/personal-agent [-config path]` | `internal/agent/sqlite.Store.Diagnostics` | `setup_required`, null active profile, ingress disabled; matching stored identity |
| Diagnostic command | `bin/personal-agent [-config path] status` | `cmd/personal-agent/main.go:run` | One JSON object, exit zero |
| Versioned configuration | `-config` and `ERII_DATA_ROOT`/`ERII_LOG_LEVEL` | `internal/config.Load` | Invalid input rejected without value echo; focused Go boundary tests |
| SQLite bootstrap | Either startup command | `internal/agent/sqlite.Open` | Schema v1, WAL, one persisted Agent row |
| Migration safety | Start against prior/future versions | `internal/agent/sqlite.migrate` | Future schema rejected without row changes; Go test proves rollback and synthetic prior-schema upgrade |
| Local observability | Inspect configured `logs/` | `internal/observability.Telemetry.Start` | JSONL records and correlated startup/database spans |
| Shutdown/restart | SIGTERM, SIGKILL, then `status` | `cmd/personal-agent/main.go:main` | Clean SIGTERM, identical diagnostics and SQLite dump after both restart paths |

Launch uses an isolated temporary root and a different working directory.
Doctor checks the exact binary, CLI help, and clean initial state. Drive covers
the paths above. Evidence is `evidence/verification.json`. Cleanup always reaps
the exact child process and removes temporary state; the evidence report stays.
