# Production recovery and operations

The runtime state is authoritative. JetStream is an at-least-once wakeup bus.
Restarts reuse accepted model results, frozen Model/Capability Snapshots,
ToolInvocation identities and fixed-source Code Mode ledgers. There is no
exactly-once guarantee for external effects. `unknown` stays query-only or
requires an operator decision; a backup cannot reveal effects performed after
its recovery point.

## Run and diagnose

`erii -config CONFIG web serve`, `tui`, and `automation work` own the
foreground production lifecycle. Setup remains available without a usable
profile. The broker and durable consumer initialize before the Tool Registry,
ComputerProvider and Sidecar handshake. The existing Outbox/Inbox and recovery
scan start before execution and work ingress. Runtime publication uses an atomic
core reference, so a Setup UI never captures an obsolete Actor after activation.
Interactive model selection wakes startup immediately and waits for actual
runtime readiness before completing the Web response or printing the TUI input
prompt. The wait is bounded at 15 seconds and stops during shutdown.

The scan is the same `Runs.Recover` used by `Next`: due Automation admission,
Code execution claim/wake reconciliation, Approval expiry, query-only unknown
outcomes, controlled stops, immutable conversation/result intents and Main
follow-ups. Run scheduling reclaims expired leases through the existing CAS and
epoch fences. Artifact startup reconciliation and channel delivery recovery
retain their existing boundaries. See [replay](codemode.md),
[Computer](computer.md), [channels](channels.md) and [delivery](delivery.md).

SIGTERM stops ingress and joins accepted executions at durable boundaries. It
stops Memory maintenance, drains internal wakeups with a two-second bound, closes
the consumer/broker, then closes the Provider and databases. It never cancels a
Task. Sidecar executions remain per invocation; the startup handshake process
receives `runtime.shutdown` and is joined before ingress. No idle VM is retained
by the handshake. A broker disconnect retains intents, closes new work ingress,
and reconnects without failing accepted work. Setup/progress endpoints remain
available. Explicit one-shot CLI commands (`chat work`, `delivery work`, etc.)
can inspect/recover the local authoritative state without a running broker.
Only `-test-bridge` selects a Fake; `-test-events` additionally drives the real
NATS lifecycle in isolated CI.

```sh
erii -config CONFIG ops health
erii -config CONFIG ops broker
erii -config CONFIG ops trace RUN_ID
erii -config CONFIG ops report > bug-report.json
```

`health` contains counts of live/expired claims, pending Code wakes, conversation
intents, due Automation/Approval, pending/unknown delivery and Tool outcomes,
active Root queue ages/service tickets, circuit breakers and configured scheduler
limits. Root detail is limited to 100 rows; traces return the latest 1000 points.
`broker` reads retained messages/bytes, consumer pending/redelivery and DLQ count.
These are operator commands; they are never model Tools, Web progress data,
notifications or model-visible budget discovery. Querying them never drives an
Agent or sends a message.

`report` is a bounded allowlist of counts, limits and opaque correlation IDs.
`trace` includes Run/Parent/Root IDs, sequence, event kind and Tool call ID; it
omits the event text. Never attach raw SQLite, logs, full RunTrace, prompts,
Tool arguments/results, endpoints, account/destination identifiers, config,
Artifacts or archives to a bug report. Reports remain local until an operator
explicitly shares them. Credentials are resolved only by SecretStore. Structured
logs contain fixed messages/codes and correlation metadata. App/trace files each
rotate at 16 MiB with two backups (96 MiB total by default); rotation is serialized
across CLI processes. Raw domain audit rows are retained, not silently pruned.

## Broker authentication, bounds and DLQ

Use a dedicated `stream_prefix` per data root. Literal loopback can use an
unauthenticated development broker. Every remote broker requires `tls://` with
normal certificate verification and `token_ref` resolved through SecretStore.
URLs reject embedded credentials, query strings and fragments. Never place the
token itself in YAML, command arguments, SQLite or reports. Missing/invalid
credentials fail closed with a constant error, without the token or provider
error. Rotate the referenced token in SecretStore and restart the service.

Defaults: event Stream `_events_v1`, File storage, DiscardNew, seven days,
1 GiB, 1,000,000 events, 1 MiB/event. Durable consumers permit 32 unacknowledged
messages and pull one at a time. AckWait defaults to 30 seconds (10 ms–5 min),
retry interval to one second (1 ms–1 min). Ingress/recovery does not depend on
broker deduplication or global event ordering.

Provision broker storage for both configured Stream reservations plus operating
headroom. Reconnect reads and strictly validates existing Stream configuration;
it does not request another reservation or silently alter retention or access.

After five unsuccessful consumer deliveries, the adapter durably writes a
content-free quarantine receipt before acknowledging the original message. A
crash between those writes can duplicate the receipt; its consumer/stream
sequence is its stable identity. The receipt contains only consumer, stream
sequence, attempt count, body SHA-256 and a fixed failure code. Raw bodies and
headers stay in the original protected stream. The independent `_dlq_v1` Stream
retains receipts for 30 days, at most 64 MiB/10,000 entries/4 KiB per entry.
Configure `poison_attempts` (2–100) for both consumption and non-transient Outbox
publication failures. Malformed stored envelopes quarantine immediately. Healthy
Outbox rows are served before previously attempted rows. Broker outages do not
increment poison failures or exhaust their threshold. No DLQ event notifies the
user. The local structured warning and `ops broker`/`ops health` are the developer
alert surface. A full broker/DLQ retains authoritative intent and requires
operator remediation; it does not authorize dropping records or replaying effects.

```sh
# Requeue a valid internal wakeup using its displayed attempt count as a fence.
erii -config CONFIG ops retry-event EVENT_ID ATTEMPTS
```

This operation cannot retry a Tool or channel side effect and refuses malformed
envelopes and stale decisions. Broker receipt inspection is a privileged broker
operation using the displayed stream sequence, not a user channel. Diagnose the
handler first; authoritative recovery scans continue independently.

Existing unbounded v1 Streams fail validation instead of silently acquiring new
retention policy. Stop Erii processes, back up state, and explicitly run:

```sh
erii -config CONFIG ops upgrade-broker
```

The command verifies the owned subject/storage/retention contract and refuses
bounds that would immediately discard retained messages by count/bytes/age.
It preserves messages, subjects and durable positions. Set a supported larger
retention window or settle/export old wakeups before retrying. Normal `broker`
and serving commands validate policy; they never silently alter existing bounds.

## Cold encrypted backup and isolated restore

Stop **every** process using the data root, including temporary CLI commands and
external database writers. Every Erii process holds a shared OS lock. Backup
requires the exclusive lock and refuses a live instance; process death releases
the lock. Do not copy live WAL files. The backup verifies all three databases,
checkpoints their WALs, checks foreign keys and live Artifact hashes, then streams
a consistent archive directly into encryption. No plaintext staging archive is
created. Symlinks, hard links and special files in the snapshot are refused.

Create/escrow a dedicated 32-byte encryption key in SecretStore first. A reference
may use `env:NAME` for a controlled deployment, or the OS credential store. The
archive never contains key material, and is useless if its key is lost. The
archive uses a fresh 256-bit salt, HMAC-SHA256 key derivation and AES-256-GCM
records, with authenticated order/length and a final marker. Truncation, append,
corruption and a wrong key fail closed. Maximum snapshot size is 64 GiB/65,536
files, with an 8 MiB manifest. The private manifest records version, database
schemas, sizes and SHA-256 hashes. Its CLI response is local operational data,
not the shareable bug report.

```sh
erii -config SOURCE_CONFIG ops backup /private/backups/erii.erii KEY_REF
erii -config ISOLATED_CONFIG ops restore /private/backups/erii.erii KEY_REF
# Inspect/retire the source and reconcile post-backup external outcomes first.
erii -config ISOLATED_CONFIG ops activate source-retired-and-outcomes-reconciled
erii -config ISOLATED_CONFIG status
```

The archive includes `state.db`, `conversation.db`, `memory.db` and immutable
`artifacts/` bytes, including tombstone/claim/approval/replay state. It excludes
credentials, logs, auth locks, configuration, workspace/Skill directories,
Computer VM/checkpoint storage and external provider ledgers. Back up those
resources separately using their owners' policy. A new machine must provision
the same credential references and Memory decryption key separately; missing
credentials enter repairable setup, never a paid/Fake fallback. Memory's existing
encrypted material remains encrypted. Sensitive Conversation/Artifact material
is encrypted in the archive; its restored at-rest representation follows the
original storage policy.

Restore writes only a new, isolated data root and never overwrites an existing
root or archive. It verifies authentication, path/type bounds, exact manifest
inventory, file hashes, integrity, foreign keys, Artifact bytes and compatible
versions before atomically publishing the root. Partial restores are removed.
Files/directories use 0600/0700. A sibling-parent maintenance lock serializes
publication. The restored root remains fenced until the explicit cutover command;
neither model work nor external effects run before activation.

Activation attests that the source is retired and the operator checked effects
after the backup recovery point against authoritative external records. Never
run two copies of one identity. Preserve provider idempotency receipts and
reconcile `unknown`; a cold snapshot cannot promise exactly-once effects or
recover work accepted only after its recovery point. The existing fixed-source
and frozen-snapshot compatibility checks still apply after restore.

## Forward upgrade rehearsal and evidence

Restore a backup into an isolated root first. All three schema versions are
checked before any forward migration; a future/incompatible database rejects
startup without partially upgrading another database. State migration 026 adds
only Outbox quarantine fields/index. Empty and every supported prior schema
follow the same forward migration path. Failed DDL rolls back version and schema
together. Never lower `user_version` or downgrade in place. Retain the old binary
and original cold backup until the rehearsal completes.

```sh
make test
make test-race
make lint
make verify
python3 scripts/verify_operations.py --binary bin/erii --evidence evidence/operations.json
python3 scripts/verify_capacity.py --binary bin/erii --evidence evidence/capacity.json
python3 scripts/measure_capacity.py --binary bin/erii --evidence evidence/capacity-samples.json
```

Real interactive acceptance shares `scripts/event_broker.py`: an owned, pinned
JetStream test peer, built from the Go integration tests, with a versioned JSON
handshake and EOF shutdown. No broker server enters the product binary. TUI and
Web OAuth fixtures exercise production broker startup and immediate first input.

`make verify` owns five consecutive operations passes and three default-limit 1000
Child batches in one reused state root. Its existing event gate adds a real CLI/NATS/Sidecar lifecycle,
authentication, poison isolation/restart, stream upgrade, broker outage and
shutdown evidence. Existing suites retain process/Sidecar kills, late/duplicate/
out-of-order events, approval/side-effect crash windows, cancel fences and
query-only unknown handling. No separate execution or replay pipeline is added.
`TestOperationsPriorSchemaRollbackAndActiveWorkUpgrade` reconstructs real v25,
retains accepted work/snapshots, rejects partial migration, upgrades and drains
the work exactly once. Operations harness cleanup joins processes and removes
its isolated roots, ciphertext archive and restored state.

The capacity sampler runs the unchanged 100-item batch/fairness/cancel/restart
harness three times by default. Compare the same machine, fixture, binary
boundary and harness; retain individual samples, medians, ranges and binary
hashes. The 1000-item test is a separate capacity observation, not a semantic
Golden Case score. Model/Tool calls keep their existing per-call watchdogs (45 s
for a model step; frozen Tool deadlines). The provider breaker counts consecutive
infrastructure errors, cools down and resets on success; business item failures
do not trip it. Existing account monetary admission remains independent of Tool
approval and does not become a Token budget. Default scheduler and safety limits
remain unchanged until measurements justify a change. Release-time live accounts,
macOS compatibility and semantic Golden Case scoring remain separately reported.

Recorded local samples are in [operations-capacity.json](../reports/operations-capacity.json).
On one Apple M4 Max (16 logical CPUs, 64 GiB), the unchanged 100-item harness had
these observations; all six samples passed with the original limits.

| Observation | Before | After |
| --- | --- | --- |
| Three samples (seconds) | 9.327, 8.726, 8.893 | 9.041, 8.971, 9.012 |
| Median (seconds) | 8.893 | 9.012 |
| Max–min (seconds) | 0.601 | 0.070 |

The median changed by about +1.3%, within the observed before range; three
samples do not establish a general performance difference. Defaults stay at
16 Children/tree, 32 global Workers and the original safety ceilings. A separate
three-cycle test reused one state root for 3000 Children: 2994 completed, six
injected business failures were aggregated, and every cycle released all Worker
leases. These are bounded deterministic capacity observations, not a production
duration or model-quality certification.
