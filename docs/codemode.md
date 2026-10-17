# JavaScript Code Mode runtime (ERII-19)

The configured `builtin__codemode__execute` Tool runs one fresh QuickJS VM per
invocation through the normal Agent Loop and ToolExecutor. Its `source` argument
contains the raw async JavaScript body: top-level `await` and `return` work.
There is no second Agent Loop or JavaScript interpreter. Every nested Tool,
Agent, and Computer operation creates an ordinary fenced ToolInvocation.

## Launch and verify

Install Node **24.21.0**, then install the exact WASM dependency:

```sh
npm ci --prefix runtime/javascript --ignore-scripts
make build
python3 scripts/verify_codemode.py --binary bin/personal-agent --evidence evidence/codemode.json
```

The reusable harness owns Launch, Doctor, Drive, Evidence, and Cleanup. It
creates temporary configuration and databases, activates the explicit
`codemode` model fixture, then drives `chat send`/`chat work`. The model fixture
selects requests; the real Node/Pi/QuickJS VM executes the JavaScript. The
Computer Fake is injected only with this test fixture. Production has no Fake
Computer fallback. Its future Driver remains M6 work.

The report includes exact explicit outputs, nested invocation identities,
source/Store/event readback, a real Child Worker result with inherited frozen
model identity, approval rejection without Provider dispatch, schema version,
before/after-effect SIGKILL recovery with one durable identity and one external
receipt, and cleanup. `make verify` runs this harness five times. `make test` includes
real Sidecar transport, Promise, resource, cancellation, and security fixtures.

To enable Code Mode in an ordinary installation:

```yaml
version: 1
javascript:
  command: /absolute/path/to/node
  script: /absolute/path/to/erii/runtime/javascript/src/sidecar.mjs
```

Both paths must be trusted and outside an editable coding workspace. There is
no implicit executable discovery or default model. Capability snapshots freeze
the versioned ToolSpec for each new Run. Existing snapshots retain their
original Tool surface. A Sidecar protocol failure produces a sanitized Tool
error, never a raw exception or inherited credential.

## Script contract

```javascript
const matching = await searchTools("echo", {limit: 10});
const detail = await describeTool(matching[0].name);
const results = await Promise.allSettled([1, 2, 3].map(n =>
  tools[matching[0].name]({text: String(n)})
));
store("summary", {count: results.length});
text(load("summary"));
return results.map(result => result.status);
```

`ALL_TOOLS` contains canonical names and short descriptions. Complete schemas
remain behind discovery. Canonical bracket calls always work; an escaped alias
is available only when unambiguous. Discovery searches only the frozen Run
view. Main has no `agent` global and cannot invoke the hidden Agent Tool.
Workers with the frozen capability get exactly
`agent.run({task, artifactRefs?}) -> Promise<{text, artifactRefs}>`. No Run ID,
usage, output schema, agent selector or control handle enters this API.

The CUA facade provides inventory, explicit binding, and lightweight Surface
proxies. Binding alone emits no output. `documentation()` and `capabilities`
resolve through the same nested Executor. Surface handles stay within one cell;
Store rejects them, including nested handles. The Fake exposes read-only
observations and documentation; it does not pretend to implement GUI actions.

Only explicit `text()`, `image()` and top-level `return` reach the parent Tool
result. Console and nested business results stay internal. Errors reject
Promises with normalized `code`, `message`, and `retryable` fields. `exit()`
ends successfully. A script failure does not roll back accepted work.

`store(key, value)` commits synchronously to invocation-local JSON state before
returning. Writes survive later script failure and process restart. Reads
return copies; a missing key returns `undefined`. Keys are 1–256 bytes, each
JSON value is at most 256 KiB, and total stored keys/values at most 1 MiB.
Repeated writes use a versioned last-write-wins update. Other cells and Child
Runs do not inherit this state. Large data belongs in Artifacts.

## Isolation and transport

Go starts a Node Sidecar with an explicit environment containing only PATH,
TZ, and LANG. It passes source, frozen accessible Tools, identities and limits;
it passes no credentials, model Provider or database handles. One execution
gets one Worker thread and one QuickJS-WASI VM. Worker termination is awaited;
the Go adapter closes pipes and reaps its exact child process before returning.
The process is currently scoped to `Start`; it is not a resident shared pool.

The Pi 1.3.0 VM/Prelude is vendored with its MIT notice and exact npm integrity.
Erii retains Pi's Promise scheduling, JSON boundary, output controls and
intrinsic lockdown. Its checked Prelude adaptations prohibit time/random and
provide synchronous Host-backed Store plus the CUA/Agent facades. Node is the
host; QuickJS is the script boundary. Scripts have no Node objects, filesystem,
network, timers, module loader, credentials, or real-time/random APIs.

The versioned JSON-RPC 2.0 stdio protocol implements initialize, start, cancel,
dispose and shutdown plus host discovery/invoke/store requests. Envelopes carry
execution, parent and Run identity, monotonic sequence and UTC time. The Go
adapter rejects forged identities, unknown methods, sequence drift and a final
RPC result without a matching authoritative terminal event. Exact duplicate
events can be acknowledged again. The sink durably records events before ack.
Nested admission atomically inserts CodeCall, ToolInvocation, Child admission
when applicable, counters and Outbox using the existing transaction.

Fixed production limits are: 30s cell deadline, 32 MiB QuickJS heap, 100,000
interrupt polls, 64 KiB source/output, 256 output items, 128 pending Host calls,
and 1 MiB RPC messages. Go ToolExecutor retains its actual concurrency,
approval, conflict, timeout and retry policies. Limits from untrusted sources
cannot expand them. Parameter drift for an admitted nested identity fails
closed. External effects retain `safe/query_before_retry/never` semantics.

## Recovery boundary and remaining work

A waiting nested invocation suspends and disposes the VM; its existing
ToolInvocation and Child Run remain durable. On an ordinary scheduler wake,
the current slice can re-run source and reuse the stable ordinal's accepted
invocation. Host-accepted unawaited calls continue internally; they do not add
outputs to an already finished script.

This is the ERII-19 runtime slice, **not complete Durable Replay or M5**.
ERII-20 owns dedicated execution claims/epochs, source-position call keys,
ordered result-delivery replay, wake drain, orphan-call completion, comprehensive
37-completed/8-active Sidecar and Go kill/restart proof, and frozen runtime
compatibility across recovery. Until those gates pass, do not use this slice as
an unattended long-running recovery guarantee. ERII-21 owns full due Golden
Case scoring; the runtime harness does not award semantic scores.
