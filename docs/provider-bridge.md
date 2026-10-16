# Provider Bridge and local model control

The Go Host runs a separate Node stdio Bridge using exact dependency
`@earendil-works/pi-ai@1.3.0` (MIT), with a checked-in transitive lockfile.
Only Pi's Chat Completions adapter is imported. Installation downloads the Pi
SDK dependency tree; it adds no hosted service or account cost. Its SDK still
brings other provider packages transitively, although Erii does not enable them.
OAuth, Main/Worker Runs, Conversation recovery, and general Durable Replay
remain later slices. This is the local-provider part of M1, not complete M1.

## Connect a local model

Install Node 24.21.0 and run `npm ci --prefix provider-bridge --ignore-scripts`.
Add this explicit configuration, using the absolute repository path:

```yaml
version: 1
llm:
  provider_bridge_command: node
  provider_bridge_script: /absolute/path/to/erii/provider-bridge/bridge.mjs
  provider_bridge_version: pi-ai-1.3.0-v1
```

Use an isolated `ERII_DATA_ROOT` while evaluating the CLI. The Bridge is optional;
unconfigured startup still provides setup/diagnostics without a provider.

```sh
bin/personal-agent -config local.yaml model discover http://127.0.0.1:8000/v1
bin/personal-agent -config local.yaml model add http://127.0.0.1:8000/v1 MODEL_ID
bin/personal-agent -config local.yaml model catalog PROFILE_ID
bin/personal-agent -config local.yaml model test PROFILE_ID
bin/personal-agent -config local.yaml model select PROFILE_ID 1 1
bin/personal-agent -config local.yaml status
```

`add URL MODEL_ID [AUTH_ID]` creates an unverified candidate with conservative
8,192 context/1,024 output token limits. The existing `model add` JSON form allows
explicit token limits and other candidate metadata. If discovery is unavailable,
manually supply the ID to `add`; catalog support is never an activation condition.
Inspect profile/config versions with `model list` and `status` before selecting.
Every mandatory Probe behavior must pass. Optional compatibility, reasoning,
modalities, and sampling claims remain disabled until individually verified.
Responses API is rejected on the local path. Unsupported frozen settings fail
closed instead of being silently dropped. Clipping Probe uses an eight-byte Host
output limit while keeping the frozen model token limit, so it actually tests
clipping rather than depending on a model exceeding its requested token limit.

API keys are optional. On macOS with cgo, `model login` reads a key from stdin
and writes it through Security.framework Keychain APIs (no credential process
arguments). It returns an AuthConnection ID to use with `add`. On any platform,
`model login ENV_NAME` explicitly registers a SecretStore environment reference;
`ENV_NAME` must be present in each Host process using that connection. Pass the
returned AuthConnection ID as the last argument to `add URL MODEL_ID AUTH_ID`
or in the JSON metadata form. Missing credentials fail closed. Neither method
prints or stores the key in SQLite. Do not enter credentials in metadata fields.
The native backend is unavailable without macOS/cgo; it never falls back to a file.

Privacy: model requests, conversation content, and Tool schemas go only to the
configured endpoint. `localhost` resolves from the Go Host network namespace,
not the Computer Guest. This notice is also printed before CLI activation.
The Host retrieves only the referenced API key for each catalog/generation call
and sends it in a private, transient connection RPC envelope. The Node process
inherits PATH only, receives no domain DB paths or Tool/Computer handles, and
keeps no credential file. This is a logical process boundary, not an OS sandbox.
There is no provider fallback. All redirects are rejected, including same-origin
redirects, so Authorization cannot be forwarded. The SDK constructor requires a
nonempty key; a transport wrapper always deletes its placeholder Authorization
and sends a Bearer header only when the user supplied an actual key.

The transport retries connection failures or HTTP 429/502/503/504 at most twice,
with 50/100 ms delays, before a response stream is accepted. It does not retry
partial streams, malformed events, auth errors, or model incompatibility. Requests
use a 30-second SDK bound and the Host Probe a ten-second total bound; cancellation
and cleanup retain the existing one-second bound. The error body is discarded,
except for an allowlisted machine code read from at most 8 KiB for HTTP 400/422
context-length classification. SDK console sinks are disabled because malformed
SSE parsing can otherwise print provider chunks. Errors contain stable codes only.

Readiness checks the saved proof, current credential availability, and Bridge RPC
health. It does not require catalog support or certify current endpoint health;
actual inference and explicit re-tests observe provider availability. Failed
re-tests revoke proof and close ingress. A crashed Bridge is restarted only for
the next independent operation. No generation is automatically replayed.

Schema v5 retains immutable endpoint locations separately from mutable profiles.
Resolution uses the frozen endpoint hash; editing a profile cannot redirect an
old snapshot. Migration backfills endpoints available in prior-schema profiles.
An endpoint lost through historical edits before v5 cannot be reconstructed and
fails closed. Snapshots and credentials retain their existing boundaries.
`provider_usage` stores the snapshot, generation/Run IDs, observed token counts,
Host elapsed milliseconds, and normalized failure. Missing token usage is NULL,
including cancellation/crash; zero is recorded only if supplied by the provider.
Local monetary cost is zero. This is accounting, not a durable generation-result
ledger. A Host crash can lose an in-flight usage record; Run recovery is later work.

## Verification and operations

`make test` runs Go tests and Node contracts against an owned HTTP endpoint through
actual subprocesses. `make verify` drives the CLI, reads SQLite independently,
and writes `evidence/local-provider.json` and `evidence/pi-bridge.tap`. The harness
owns launch (ephemeral HTTP port and data root), doctor (binary, Node, installed Pi),
drive (discover/add/test/select/status, keyed/manual/failure variants), evidence
(usage, failures, snapshot IDs), and cleanup (reap children, close endpoint, delete
state). Go's actual Pi crash test kills the exact Node child after a text delta,
rejects partial output, restarts the same Bridge, and reads the catalog back.
Security/cancellation contracts repeat five times through `make verify`.

For optional owned native Keychain read/update/delete verification on macOS:

```sh
ERII_TEST_KEYCHAIN=1 go test ./internal/llm/secret -run TestKeychainOwnedRoundTrip -v -timeout 30s
```

CI uses deterministic Fakes/environment references and requires no OS credential
store or real model account. Real Qwen/vLLM/SGLang/LM Studio compatibility remains
release-time Live Shadow evidence. Product Golden Cases remain not_applicable
until their Main/Conversation slices; no product score is claimed here.

## Deterministic fixtures

## Reproduce the CLI path

Use an isolated absolute `ERII_DATA_ROOT`. Run `make build`, then provide metadata
through `model add` as documented in `development.md`. Copy its `profile_id`.
Profiles are unverified candidates, and metadata cannot activate one.

```sh
bin/personal-agent -test-bridge success model catalog
bin/personal-agent -test-bridge success model test PROFILE_ID
bin/personal-agent -test-bridge success model select PROFILE_ID 1 1
bin/personal-agent -test-bridge success status
bin/personal-agent status
```

`select` requires the expected profile version and Agent config version, visible
in `model list` and `status`. It increments both versions atomically, commits
ready/verified metadata, retains the stable AuthConnection reference, and creates
both activation Outbox events. Test alone stores Probe evidence and opens no
ingress. Concurrent selections have one config-CAS winner. Metadata edits or auth
version changes invalidate the prior Probe. A failed re-test revokes only the
tested version's existing proof, marks it invalid/unavailable/reauth_required,
and degrades an active config atomically with events. Cancellation by the Test
caller preserves prior state. Failure of the Probe's internal cancellation check
revokes prior proof even when the error includes `GENERATION_CANCELLED`; a
responsive replacement Bridge alone cannot restore readiness. A failed commit
changes neither the profile, Probe version, config nor events. Existing SecretStore registration
writes credentials first and removes a new orphan on failed metadata commit.
SecretStore/SQLite still have no distributed transaction: a hard crash between
those commits can orphan a credential; reconciliation is later work.

`model cancel` cancels setup without changing durable state. No model is chosen
automatically. Every process that uses a test profile must explicitly supply
`-test-bridge success` again. Without it, an existing active test profile reports
`degraded` with closed ingress. Incompatible Bridge versions, missing credentials,
unavailable/invalid active profiles, or stale Probe evidence also close ingress.
Diagnostics and metadata remain usable for repair. A compatible, valid active
profile reports `ready`; this readiness flag is a gate for later Agent admission,
and does not add a Conversation/Task execution endpoint in M0.

The Fake fixtures are named `success`, `incompatible`, `crash`, `partial-crash`,
`completed-crash`, `no-tools`, `bad-sequence`, `drift`, `malformed`, `auth-required`,
`raw-error`, `event-error` and `error:CODE` for the eight baseline provider codes.
Only explicit test injection starts them. Fixture names are not
ProviderKind values or saved deployment defaults. A failing Probe never selects
a different provider or retries a generation automatically.

## Private protocol v1

`internal/llm` owns consumer interfaces for the model control plane and future
Agent Loop. They are repository-internal and may evolve with conformance tests;
they are not a public plugin API. `internal/llm/bridge` implements the stdio client.
`internal/llm/fake` implements deterministic provider fixtures behind it.

One long-lived child exchanges newline-delimited JSON-RPC 2.0. The Host first
sends `bridge.initialize` with `protocol_version: 1` and rejects any different
response version. Numeric request IDs are Host-created and correlated per client.
The child gets only PATH, an isolated working-directory choice (OS temp directory),
and its explicit arguments, without application config or inherited credentials.
Child stderr and arbitrary error bodies are discarded. This is a logical process
boundary, **not an OS sandbox**; the fixtures themselves perform no filesystem,
Tool, Computer, domain database or network operations.

| RPC | Semantics |
| --- | --- |
| `bridge.initialize`, `bridge.shutdown` | Protocol handshake; orderly shutdown followed by bounded forced cleanup and process reap |
| `auth.start`, `auth.continue`, `auth.refresh`, `auth.logout`, `auth.status` | Fake auth lifecycle; UI-neutral URL, device-code, progress, workspace and input events; no actual credentials |
| `models.list`, `models.refresh` | Candidate catalog, never proof of capability; real calls carry a transient Host connection envelope |
| `model.probe` | Provider-side connection check; the Host also runs behavioral generation checks |
| `generation.start`, `generation.continue` | Real calls additionally carry a transient connection envelope; accepted request with immutable ModelSnapshot, Host-created generation/run IDs, and `session_id = run_id` |
| `generation.event` | Notification with generation ID, monotonic sequence and text/tool/completion/error kind |
| `generation.cancel` | Cancellation acknowledgement; Host discards partial accumulated result |
| `session.dispose` | Explicit end of the isolated provider session |

The Host accepts a matching duplicate sequence once, rejects changed content under
one sequence and rejects gaps/out-of-order new events. A completion is delivered
once per generation call. Complete frames already received before process EOF
are drained before reporting a crash; incomplete generation returns an explicit
error with no partial result. Only the next independent operation restarts and
re-handshakes the same Bridge. There is no durable generation-result ledger yet;
A later M1 slice will persist model-call/Conversation completion and recovery decisions.

Frames are bounded to 1 MiB; generation output to the smaller of 64 KiB and four
bytes per requested output token, on a UTF-8 boundary. This is a byte safety bound,
not tokenization or a context-limit estimate. There are at most 4096 distinct
sequence events and 32 Tool Calls (64 KiB of arguments per call). The reader
validates and deduplicates events independently of sink speed,
clips text before retaining it, and queues only each kind's normalized payload.
Each generation mailbox holds the full 4096-event budget plus one rejection;
matching duplicates consume no slots. Slow sinks cannot block the shared reader's
RPC replies. Queued text shares the output byte bound, Tool Calls retain the
existing count/frame/argument bounds, and sequence history stores only digests.
Sink callbacks run synchronously; cancellation is observed between callbacks.
Host RPC calls have a five-second bound per wire exchange, including writer
admission and pipe I/O; behavioral Probe has a ten-second bound. Writer admission observes cancellation, and pipe
deadlines interrupt a blocked write without leaking a writer goroutine. A partial
frame terminates and reaps that Bridge instead of reusing a corrupted stream.
Cancellation and cleanup have independent one-second contexts. Close can abandon
its graceful shutdown attempt and terminate a peer that is not reading.
Cancellation after complete dispatch sends `generation.cancel` even before the
start acknowledgement. A late acknowledgement cannot revive that generation.
Explicit Cancel acknowledgement also settles the matching local Generate or
Continue call with `GENERATION_CANCELLED`, without requiring a second terminal
notification. It interrupts a pending start acknowledgement and fences queued
and late events. Cancellation and final result publication share the process
registry lock; an old acknowledgement cannot settle a different registration.
Cancel for a registered generation waits for its start/continuation write to
finish or fail before sending the cancellation RPC. Registration alone never
permits cancellation to overtake dispatch. This wait observes the caller's
deadline and the five-second Cancel bound, holds no process lock, and does not
wait for the start acknowledgement. Close can still retire the peer independently.
Waiting for a shared restart/handshake observes each caller's context. The
lifecycle state lock never covers process I/O or reap; Close can terminate an
initializing child immediately and waits for its initializer to finish cleanup.
If an internal acknowledgement deadline expires while the caller remains live,
the dispatched start/continuation is cancelled with an independent cleanup
context. Failed cleanup forces termination. The caller receives
`PROVIDER_UNAVAILABLE`, and the generation is never replayed automatically.
Failed cancellation cleanup terminates the Bridge and returns the cancellation
cause together with the sanitized cleanup error; it cannot certify a successful
Probe. Oversized/malformed frames,
unknown event kinds and sequence drift fail closed. Transport failure is
`PROVIDER_UNAVAILABLE`; invalid wire data is `INVALID_PROVIDER_RESPONSE`.
Host validation rejection of accepted work sends bounded independent cancellation
before returning; failed cleanup retires the peer and preserves the original
sanitized rejection together with the cleanup error. An unusable reader forces
retirement. Provider terminal errors are distinct and require no extra cancel.
These paths never replay a generation automatically.

Provider errors normalize only to the baseline's eight stable error codes. No
raw response body, token, endpoint or child stderr enters error messages. The
process does not get a domain store, ToolExecutor, Computer or ArtifactStore.
No model-visible Tool was added, and model code cannot select profiles.

## Probe and activation

The Host tests one streamed reply, an actual `probe_echo` Tool Call with valid
arguments, a Tool Result continuation, acknowledged cancellation, and clipping
of long output. This test Tool has no executor or external effect. Failure of any
mandatory behavior prevents selection. The catalog and user claims cannot mark
a profile ready. Probe, selection and readiness enforce context/output limits
of 1,048,576/65,536 tokens. These are execution limits, not the persisted metadata
format: valid v3 candidates above these limits still migrate, list and read back
unchanged, alongside all other candidates. They cannot be probed or selected
until replaced or edited within limits. No profile is silently clamped.
Activation retains only observed text/tool/streaming capabilities and conservative
compatibility. Unprobed image/audio/reasoning/parallel/usage claims and optional
sampling defaults are cleared. Explicit reasoning effort is rejected until a
Probe supports it. Frozen snapshots remain unchanged by all control-plane edits.

`make verify` owns launch, doctor, CLI drive, independent SQLite readback,
SIGKILL/restart and cleanup mechanics. It also repeats Bridge/storage/security
contracts five times in `evidence/models.jsonl`. `make test-race` covers concurrent
RPC/cancellation and independent SQLite activation contenders. See
`scripts/FEATURES.md` for exact entries and `evidence/verification.json` for the
CLI evidence. No account, installed broker, persistent test process or human
observation relay is required.
