# M0 Provider Bridge and model control

This is the deterministic M0 skeleton. It performs no provider network access,
OAuth, billing or Agent execution. It adds no dependencies. Real Pi integration,
its pinned dependency/version, production SecretStore, credential RPC and live
provider/login contracts remain M1 work. Product Golden Cases remain
`not_applicable` until their milestone. This does not claim M1 or Durable Replay.

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
and degrades an active config atomically with events. Cancelled tests preserve
prior state. A failed commit changes neither the
profile, Probe version, config nor events. Existing SecretStore registration
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
| `models.list`, `models.refresh` | Candidate catalog, never proof of capability |
| `model.probe` | Provider-side connection check; the Host also runs behavioral generation checks |
| `generation.start`, `generation.continue` | Accepted request with immutable ModelSnapshot, Host-created generation/run IDs, and `session_id = run_id` |
| `generation.event` | Notification with generation ID, monotonic sequence and text/tool/completion/error kind |
| `generation.cancel` | Cancellation acknowledgement; Host discards partial accumulated result |
| `session.dispose` | Explicit end of the isolated provider session |

The Host accepts a matching duplicate sequence once, rejects changed content under
one sequence and rejects gaps/out-of-order new events. A completion is delivered
once per generation call. Complete frames already received before process EOF
are drained before reporting a crash; incomplete generation returns an explicit
error with no partial result. Only the next independent operation restarts and
re-handshakes the same Bridge. There is no durable generation-result ledger yet;
M1 will persist model-call/Conversation completion and recovery decisions.

Frames are bounded to 1 MiB; generation output to the smaller of 64 KiB and four
bytes per requested output token, on a UTF-8 boundary. This is a byte safety bound,
not tokenization or a context-limit estimate. There are at most 4096 distinct
sequence events and 32 Tool Calls (64 KiB of arguments per call). Host RPC calls
have a five-second bound, including writer admission and pipe I/O; behavioral
Probe has a ten-second bound. Writer admission observes cancellation, and pipe
deadlines interrupt a blocked write without leaking a writer goroutine. A partial
frame terminates and reaps that Bridge instead of reusing a corrupted stream.
Cancellation and cleanup have independent one-second contexts. Close can abandon
its graceful shutdown attempt and terminate a peer that is not reading.
Cancellation after complete dispatch sends `generation.cancel` even before the
start acknowledgement. A late acknowledgement cannot revive that generation.
Failed cancellation cleanup terminates the Bridge and returns the cancellation
cause together with the sanitized cleanup error; it cannot certify a successful
Probe. Oversized/malformed frames,
unknown event kinds and sequence drift fail closed. Transport failure is
`PROVIDER_UNAVAILABLE`; invalid wire data is `INVALID_PROVIDER_RESPONSE`.

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
