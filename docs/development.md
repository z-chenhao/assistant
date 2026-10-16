# Runnable foundation and local Provider Bridge

The foundation includes strict YAML, SQLite, Outbox/Inbox, CloudEvents/NATS,
model metadata, immutable snapshots, and a versioned stdio Provider Bridge.
The local-provider M1 slice connects pinned Pi AI to Chat Completions endpoints
through the CLI, with native Keychain or explicit environment credentials.
See [Provider Bridge setup and operations](provider-bridge.md) for the runnable
local path. Current ChatGPT OAuth is implemented through the CLI. Main Runs, the reusable Agent Loop, ordered Conversation and the first pure Direct Tool are now runnable through [the Main CLI](main-loop.md). The [TUI](tui-and-evals.md) and [Task/Root delegation with controlled pause/cancel](tasks.md) are runnable. Child Runs and general Durable Replay remain later slices. Full product Golden Cases remain `not_applicable`; Main and Task scripted L0 fragments are reported separately.

## Commands and evidence

Use Go 1.26.2+, Node 24.21.0, Python 3.10+ with standard-library SQLite, and a C compiler for
the Go race detector. The application uses pure Go SQLite. Initial module
fetching and `npm ci --prefix provider-bridge --ignore-scripts` need network access; checks subsequently need no online services,
accounts, installed broker executable, containers, or fixed ports.

| Command | Evidence |
| --- | --- |
| `make build` | `bin/personal-agent`, the only product process entry point |
| `make test` | Real temporary SQLite, migrations, transaction rollback, duplicate/late/out-of-order events, Fake and real EventBus contracts, broker restart, process kills |
| `make test-race` | Those tests with Go's race detector |
| `make lint` | `gofmt`, `go vet`, module checksums |
| `make verify` | Real CLI metadata/probe/select/restart and cancelled setup, Bridge and metadata/security contracts five times, relay outage recovery, JetStream wire contract, seven SIGKILL boundaries, independent state readback |

GitHub Actions runs all four checks on Ubuntu for PRs and pushes to `main`.
The integration tests own an embedded file-backed JetStream server bound to
loopback on an assigned port. They shut it down, wait for termination, and remove
its temporary storage. Crash tests kill only their exact Go-test child PID.
The CLI harness also owns and reaps its exact executable child. All state is
isolated from user configuration and removed on success or failure.

`make verify` removes old reports and writes ignored JSON evidence:

- `evidence/verification.json`: diagnostics, persisted Agent/profile rows,
  canonical JSON, rejected metadata, logs/spans, bootstrap/model kill/restart,
  future-schema rejection, Provider Probe/activation/degraded states, and cleanup assertions.
- `evidence/main.json`: concurrent Main ingress, nine SIGKILL boundaries, stable
  steering, profile switching, semantic/Tool/usage/delivery readback and scripted
  GC-01/09/10 Main fragments.
- `evidence/local-provider.json`: real Node/Pi HTTP CLI setup, credential safety, failed re-test and usage readback.
- `evidence/pi-bridge.tap`: owned endpoint contract results.
- `evidence/models.jsonl`: Go test events for five runs of Bridge/auth/catalog/
  generation/cancellation/error contracts, migration/rollback, activation CAS,
  immutable snapshots, and credential safety assertions.
- `evidence/events.json`: seven process-kill boundaries and SQLite state after
  reopening, before recovery handling.
- `evidence/relay.json`: CLI launch during broker outage, frozen event identity,
  recovery, delivery marker, graceful shutdown, and stable Agent identity.

A nonzero command means verification failed even if an earlier, narrower report
exists. Paths in reports describe temporary state already removed. The feature
map is `scripts/FEATURES.md`.

## CLI and configuration

```sh
make build
./bin/personal-agent -config config.example.yaml status
./bin/personal-agent -config config.example.yaml
# Ctrl-C stops the foreground process.
./bin/personal-agent -config config.example.yaml relay
# Ctrl-C stops the internal event relay.
```

`status` opens/migrates SQLite, prints one JSON object, and exits. With no
command on a terminal, startup opens [setup and conversation](tui-and-evals.md).
With redirected stdin it prints the same diagnostic object and waits for
SIGINT/SIGTERM. Explicit `tui` also opens the terminal adapter. These paths do
not connect to NATS. `relay` explicitly runs only the internal event publisher;
it prints diagnostics, connects to the configured broker, and drains pending
Outbox events in bounded batches until stopped. It does not enable a model or
Agent ingress. There is no production consumer yet; transactional Inbox handling
is exercised through its real Go API by the integration harness.

```json
{"readiness":"setup_required","agent_ingress_enabled":false,"agent_id":"<UUIDv7>","active_model_profile_id":null,"config_version":1,"schema_version":10,"state_database":"/tmp/erii-local/state.db"}
```

Flags precede commands. `-help` creates no state. SIGINT/SIGTERM close the relay,
database, and logs; errors print to stderr and exit nonzero. Known transient
broker connection/publish failures retry at the configured interval. Invalid
stream configuration, malformed events, and unexpected database errors stop the
relay with pending intent retained; repair the cause and rerun `relay`. Shutdown
can interrupt marking a successful publish; the next relay may redeliver it.

Configuration contains exactly one YAML document with explicit `version: 1`.
Unknown/duplicate fields, invalid types, relative data roots, invalid levels or
NATS values fail before storage opens. Errors do not echo configuration values.
Without a file, version-1 defaults apply. Environment overrides apply last;
empty overrides are invalid.

| YAML key | Override | Default / accepted values |
| --- | --- | --- |
| `version` | None | `1` |
| `app.data_root` | `ERII_DATA_ROOT` | `$HOME/.local/share/erii`; absolute path |
| `app.log_level` | `ERII_LOG_LEVEL` | `info`; debug/info/warn/error |
| `nats.url` | `ERII_NATS_URL` | `nats://127.0.0.1:4222`; nats/tls host:port, no userinfo/path/query/fragment |
| `nats.stream_prefix` | `ERII_NATS_STREAM_PREFIX` | `erii`; 1–64 ASCII letters/digits/underscores/hyphens |
| `nats.timeout` | None | `2s`; positive, at most 1 minute |
| `nats.ack_wait` | None | `30s`; positive |
| `nats.retry_interval` | None | `1s`; positive |

Use a dedicated stream prefix per data root on a shared broker. Broker
credentials are not supported in this slice; never put secrets in URLs or YAML.
No provider or credential backend is implicitly enabled. An unavailable broker cannot turn
`setup_required` into `ready` or replace the persisted Agent identity.

## Unverified model metadata

Add a candidate through stdin, then read it from a new process using the same
configured data root. These commands do not contact the endpoint or NATS:

```sh
./bin/personal-agent -config config.example.yaml model add <<'JSON'
{
  "name": "Local candidate",
  "provider_kind": "openai_compatible",
  "endpoint": "http://127.0.0.1:8000/v1",
  "wire_api": "openai_completions",
  "model_id": "local-model",
  "context_window": 8192,
  "max_output_tokens": 1024,
  "compatibility": {"tool_result_format": "openai"},
  "capabilities": {"text_input": true}
}
JSON
./bin/personal-agent -config config.example.yaml model list
./bin/personal-agent -config config.example.yaml status
```

`model add URL MODEL_ID [AUTH_ID]` provides a concise local setup path.
With no arguments, `model add` accepts exactly one metadata JSON object, at most 64 KiB, with
snake_case fields. Unknown fields (including credential fields and user-supplied
status/verification/IDs/versions) fail without echoing values. HTTP(S) endpoints
must omit userinfo, queries and fragments. Names, labels, model IDs, and endpoint
paths are metadata: never put credentials in them. Output contains a UUIDv7,
UTC timestamps at microsecond precision, version 1 and `unverified` status.
Optional generation defaults support temperature [0,2] and top_p [0,1]; output
tokens must be positive and not exceed the context window. These are candidate
metadata, not verified capability claims or permission to exceed Host
execution limits. `openai_account` metadata requires `openai_responses`, an
existing OAuth connection, and no custom endpoint.

`model list` returns an array and never creates a default profile. `status`
reports `setup_required`, null Active Profile and closed ingress until selection.
`model test` and `model select` use an explicitly configured real Bridge or explicit test injection. See
[Provider Bridge operations](provider-bridge.md) for the versioned protocol,
Probe and activation flow, readiness states and remaining real-provider work.
`model login [ENV_NAME]` registers an API key through Keychain or an environment reference.
No paid, local, or Fake model is connected by adding metadata.

Trusted storage provides profile CAS updates that increment version and clear
verification. Auth state CAS preserves stable connection/provider/credential
identity. Writes enqueue only IDs/versions in the same SQLite transaction.
The snapshot API accepts the caller's transaction so the future Run owner can
commit a Run and its snapshot together. It requires a ready, verified profile
and connected authentication when referenced. Storage-only fixtures mark a
profile ready directly; Bridge contracts and the CLI harness separately exercise
the real Probe/selection boundary. No test creates a runnable Agent Run yet.
Snapshots freeze profile version, model, wire API, compatibility, capabilities,
generation parameters, token limits and canonical endpoint hash. They reference
the AuthConnection ID, never CredentialRef or secret material; update, delete
and SQL REPLACE fail. Profile changes and auth refresh metadata leave them intact.

Model JSON uses sorted object keys, compact Go JSON string escaping, finite
IEEE-754 numbers with negative zero normalized, and snake_case keys. The typed
generation/compatibility/capability documents contain no large integers or
arbitrary extensions. Endpoint hashes are SHA-256 over canonical
`{"endpoint":"<exact validated URL>"}`; URL spelling changes change the hash.
This is a restricted metadata encoding, not a general RFC 8785 implementation.

`llm.SecretStore.Get/Put/Delete` is a trusted Host interface. The deterministic,
copy-owning `llm/memory` implementation is used only in tests, never wired into
the executable. `llm.RegisterAuth` writes a new secret first, then atomically
stores the connection reference/event; database failure deletes that newly
generated credential using a bounded context independent of request cancellation.
Backend error text never escapes. Cleanup failure is explicit and requires
reconciliation. Connection metadata serialization omits CredentialRef.
Current OAuth and refresh/logout are implemented with native macOS Keychain; see docs/provider-bridge.md.
A crash between SecretStore and SQLite can leave an orphan credential; these
stores cannot commit atomically, and crash-time orphan reconciliation is not
implemented here. Account OAuth uses the current official protocol in the Provider Bridge.

## SQLite migrations and transaction ownership

`state.db` owns Agent config, Outbox/Inbox, `auth_connection`, `model_profile`,
and `model_snapshot`. Database paths
are absolute and URL-escaped. New directories use `0700`, new DB/log files use
`0600`; existing permissions remain unchanged. Each connection enables WAL,
foreign keys, and a 5-second busy timeout. The store has one connection and uses
immediate transactions. Only connection initialization retries `SQLITE_BUSY`;
domain commits and migrations are never automatically replayed.

Forward migrations and `PRAGMA user_version` commit together. Empty databases
apply v1 (Agent config), v2 (Outbox/Inbox), v3 (model metadata), and v4 (Provider
Probe and activation), and v5 (immutable endpoint bindings and snapshot usage). Real v1/v2/v3/v4 databases upgrade without changing identity,
prior metadata/snapshots or existing Outbox/Inbox history. Failed DDL rolls back
to the actual prior version. Newer schema
versions fail closed; failed DDL rolls back. There is no downgrade or deletion.

Fresh bootstrap commits its singleton Agent and `ai.erii.agent.initialized.v1`
Outbox event in one transaction. Repeated startup preserves all rows, timestamps,
and identity. Schema v4 preserves v3 metadata/snapshots and replaces the original
closed-only Agent config. `model_probe` binds observed results to profile/auth
versions and Bridge identity. Only verified selection may set a ready active
profile; stale config/profile/auth versions roll back the entire activation.

Domain modules use `Store.Transaction(ctx, func(tx *sql.Tx) error)` and call
`event/sqlite.Enqueue(ctx, tx, cloudEvent)` on that **same** transaction before
returning. Event insertion errors must be returned so the domain write rolls
back. A duplicate event ID fails closed, including parameter drift. SQL remains
inside trusted domain storage; no model or Tool provider accesses these APIs.

Outbox records have unique event IDs and Outbox IDs, subject/type, structured
JSON, UTC-microsecond occurrence/publication times, and a durable attempt count.
A partial index covers unpublished rows. Pending scans use insertion order;
publish happens outside SQL transactions. Attempts are recorded before publish;
`published_at` is set only after JetStream acknowledges acceptance. A process
kill or ambiguous publish retains the same frozen event and event ID for retry.

`event/sqlite.Store.Handle` inserts `(consumer,event_id)` and applies a SQL
callback in one transaction. A duplicate skips the callback. A handler failure
rolls back both records and does not ack. A successful commit precedes broker
ack. An ack error can return `handled=true`: the effect is committed, so
redelivery skips it and retries acknowledgement. Consumer names are stable
logical handler identities; changing a name requests independent handling.
Callbacks must use the supplied transaction, perform SQL effects only, and never
call the Store again while holding its single connection.

## EventBus contract and ordering

`internal/event` owns `EventBus.Publish/Subscribe`, `Delivery.Ack`, and the
publisher's Outbox interface. No NATS types cross that boundary. The official
CloudEvents SDK validates/roundtrips structured JSON and preserves extensions.
The Erii profile requires CloudEvents 1.0, JSON data, UTC time, and a nonempty subject.
Host-created events use globally unique UUIDv7 IDs across all sources; this is
why Inbox keys use `event_id` rather than a source-local ID. `subject` is the
aggregate key. The adapter hashes it into a safe NATS subject suffix without
changing the CloudEvent. Content-Type is `application/cloudevents+json`.
See the [CloudEvents JSON specification](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/formats/json-format.md).

The versioned stream is `<prefix>_events_v1`, matching
`<prefix>.v1.events.>`. It uses file storage, explicit publish acknowledgements,
no age expiry, and DiscardNew so capacity pressure rejects new publications
instead of evicting old ones. Durable pull consumers deliver from the beginning,
use explicit acknowledgements, and allow one outstanding message. Creation
validates existing stream/consumer settings instead of updating them silently.
Closing a subscription preserves its durable cursor. Configure a compatible
ack wait when reopening a consumer.

NATS reconnect buffering is disabled. The SQLite relay retries known transient
broker errors and reopens failed connections. No broker deduplication window is
a correctness dependency. Delivery is **at least once**, with atomic local
Inbox/domain handling. It does not provide global exactly-once delivery,
external-side-effect deduplication, global ordering, or future Run/epoch fencing.
See [JetStream consumers and acknowledgements](https://docs.nats.io/nats-concepts/jetstream/consumers).

Use one relay per data root. The foundation does not promise distributed
publisher ordering. Consumers own aggregate version/epoch guards: deduplication
alone cannot reject a distinct but stale event. The fixture applies versioned
snapshots, accepts a newer version, ignores old epochs/lower versions, and records
them in the Inbox. It proves that pattern, not a general reordering engine for
incremental events. Domain code must define its own necessary ordering policy.

## Recovery and operations

| SIGKILL boundary | Observed recovery invariant |
| --- | --- |
| Before domain commit | Domain change and event both absent |
| After domain commit | Domain change and event both survive |
| Before broker publish | Pending intent publishes after restart |
| After publish, before Outbox marker | Same event can publish twice; one Inbox effect |
| Before Inbox commit | Inbox/effect both roll back; redelivery handles once |
| After Inbox commit, before ack | Both survive; redelivery skips the effect |
| After ack | Effect survives and durable consumer does not replay the message |

Tests close every SQLite connection, kill the child, reopen the real database,
and inspect state. A separate test restarts the broker on the same file store
and verifies accepted, unacknowledged delivery survives. The deterministic Fake
can fail publish/ack and explicitly redeliver without timer-based retry.

Observe `event_outbox WHERE published_at IS NULL` and `attempt_count` to diagnose
backlog. Relay warnings omit URLs, event data, and raw provider errors. Existing
local JSONL startup/database spans remain correlated and best-effort; they are
not distributed tracing or durable RunTrace. Logs append without rotation.

Pending intent is retained indefinitely. Malformed broker messages are rejected
without ack; consumers currently retry indefinitely. DLQ routing, bounded poison
message handling/alerts, Outbox/Inbox/stream retention and compaction, broker
authentication, and multi-node operations remain later work. Storage grows until
operationally managed. There is no automatic pruning or broker reconstruction
after permanent storage loss. Back up SQLite and JetStream appropriately; this
slice proves process interruption, not power-loss or disaster recovery.

Stop processes before filesystem backups, or use SQLite's backup API. Never copy
only `state.db` while WAL writes are possible. Never remove user data to repair a
migration error. Use the matching binary or a separately verified backup.

Pinned direct additions are `cloudevents/sdk-go/v2` and `nats.go` (Apache-2.0),
and `nats-server/v2` (Apache-2.0, test harness only). The SDK avoids a custom wire
format parser; the official client avoids a second broker protocol. The test
server and its dependencies add build/download cost but no service/account cost;
they are absent from the product executable. Checksums live in `go.sum`.

Unified Tool policy, local approval commands, effect recovery and the five-run security harness are documented in [tools.md](tools.md).

The Ubuntu job has a bounded 25-minute timeout for the five-run Tool recovery gate and the existing Bridge/account/restart checks. The Tool gate adds 40 real SIGKILL/restarts; it retains every assertion and repetition.
