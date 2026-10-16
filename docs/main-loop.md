# Main Loop CLI and recovery

ERII-9 adds the Main execution path. Set up and explicitly select a probed local
or ChatGPT account profile using [the provider commands](provider-bridge.md).
Then use the same configured data root for all commands:

```sh
printf '%s' 'Hello Erii' | bin/personal-agent -config local.yaml chat send greeting-1
bin/personal-agent -config local.yaml chat work
bin/personal-agent -config local.yaml chat history
```

`chat send [REQUEST_KEY]` reads up to 16 KiB of text from stdin. It returns a
durable acceptance receipt, not a model answer. Omit the key to generate a fresh
one; retain a supplied key when retrying an uncertain ingress response. The same
key and content return the original receipt, including after Run completion.
Changed content under that key fails closed. Unconfigured/unavailable profiles
close ingress. No implicit model or Fake is selected.

`chat work` drains accepted work in the foreground. Run it in one terminal while
other terminals submit steering. It waits for a live ownership lease to expire
or its owner to finish; it never runs a second Main concurrently. When currently
accepted work ends it exits. Invoke it again for later inputs. The default
no-command startup opens the [TUI](tui-and-evals.md) on a terminal; redirected
stdin retains diagnostics/wait behavior. The TUI uses the same Actor and
executor, including recovery and Steering while Main is busy.

`chat history` returns ordered semantic user/assistant messages. It also fills
missing accepted Conversation intents before reading. This is an intentional
recovery operation, not a read-only SQLite diagnostic. Tool calls/results and
partial model text appear only in Run context/RunTrace. A failed model produces
a durable failed Run and a safe CLI error; the Host does not retry it. New user
input creates a new Run after the old one ends.

## Ownership and persistence

Run creation freezes the ModelSnapshot, capability/tool schemas, and role prompt
fragments with the Run and Outbox event in one state transaction. An immutable
history boundary selects earlier semantic history from `conversation.db` on
first execution. The resulting context and each provider request are persisted
before dispatch. Steering remains separate ordered messages, injected after a
tool batch or before publishing a final answer. Steering arriving during a
final generation causes another model step before that answer is published.
A recovered accepted exchange keeps its frozen context until its result is checkpointed, before newer steering is injected. Once a reply intent is committed, later input follows that reply and continues
the same active Run until it reaches its terminal boundary.

State migration v7 adds Runs, claims, capability snapshots, input and semantic
write intents, model generations, ToolInvocations, reply/delivery records, and
RunTrace. It preserves prior v1–v6 metadata and events. `conversation.db` has an
independent v1 forward migration, WAL, foreign keys, busy timeout, monotonic
per-Conversation sequences and immutable idempotent message IDs. An incompatible
future version fails startup. Memory storage remains a later slice.

| Run state | Legal next states in this slice |
| --- | --- |
| `queued` | `running` on Claim |
| `running` | `running` on Checkpoint/controlled release, `waiting`, `completed`, `failed` |
| `waiting` | `running` on an eligible wake/Claim |
| `completed`, `failed` | Terminal; another request creates another Run |

The Main foundation reserves the baseline's cancelled state.
[The Task slice](tasks.md) now supplies create/search/append; controlled user
pause/cancel uses the controlled [Task stop path](tasks.md). Waiting reasons use the baseline enum. Resource waits
are recoverable; user-input waits become eligible when new steering arrives.
Approval wakes and resource waits use the durable Tool path in `docs/tools.md`; other external-event integrations arrive with their respective slices.

Claims use an opaque token, monotonic epoch and five-second lease, renewed once
per second. All Run, generation-acceptance and Tool mutations verify ownership;
Checkpoints and terminal transitions additionally use Run version CAS. Takeover
rejects old claims even when a slow response arrives afterward. SQLite busy/locked
transactions receive at most three attempts with bounded exponential backoff;
other write failures stop at a recoverable boundary. SQLite's configured busy
timeout also applies. Model errors and Tool business errors are not retried.

The Bridge's Host observer commits generation results, usage and the completion
Outbox event together under the existing auth/cancellation acceptance gate,
before publishing completion. Recovery reuses completed generations. An
incomplete model call may run again with the frozen request; no partial response
becomes a semantic message. Profile switches affect only new Runs, including
after restart. Credentials remain outside snapshots/context/results.

Main replies commit in this order:

1. Save an immutable reply intent with stable message/delivery IDs in `state.db`.
2. Idempotently append the ordered intents to `conversation.db`.
3. Enqueue DeliveryRequest and its Delivery Outbox in state.db.
4. Advance the reply Checkpoint after both durable writes.
5. Complete the Run, or process newly accepted steering.

There is no cross-database transaction. Restart fills missing writes using the
saved IDs. The `settle` Checkpoint prevents another generation if the process
dies between reply commit and Run completion. Any executor may flush immutable
semantic intents, but only the current claim may create them or advance a Run.
The M3 [Delivery Outbox](delivery.md) drains requests into the durable terminal
inbox, independently of model readiness. Reading semantic history alone does not
imply a message was accepted by the channel.

SIGINT/SIGTERM stops admission to the current executor and waits for its current
model/Tool step to reach a durable boundary. Model calls have a 45-second Host
deadline; the Node Bridge also owns its existing stream deadline. The executor
releases the claim while retaining the nonterminal Run state, without cancelling business work. Run `chat work` again to
continue. SIGKILL leaves the lease to expire; a newly started `chat work` waits
and reclaims automatically. No broker is needed for this bounded CLI drain:
state is authoritative, and Outbox notifications remain available to the relay.

## Initial Direct Tool contract

| Tool | Input | Output | Permission | Replay | Limits |
| --- | --- | --- | --- | --- | --- |
| `builtin__text__echo` v1 | `{ "text": string }`, no extra fields | The same text | `read_only`, Main only in this slice | `safe` | 4096 input bytes; 10-second deadline; 16 KiB result |

The registry describes the frozen schema and resolves only Host-registered
definitions. The single Executor accepts/persists each invocation before
execution, validates arguments, applies bounded Host quotas and conflict keys, and commits each
completion immediately. Model results use original declaration order even when
calls finish in a different order. Direct calls are unique by Run, provider
exchange and call ID; a provider may reuse a call ID in a later exchange.
Each invocation has its own Host-generated ID and idempotency key.

Invalid echo arguments return `INVALID_ARGUMENT`. Unavailable tools return
`NOT_FOUND`; these safe error objects contain only code/message/retryable.
The registry requires explicit effect metadata and the durable approval path
for external effects (see `docs/tools.md`). No `agent.run()`, generic external transport, or
bypass is exposed here. Unknown tools retain replay policy `never`; recovery
can finish their rejection without executing a provider.

## Evidence and limits

Run `make test`, `make test-race`, `make lint` and `make verify`. The feature map
is [scripts/FEATURES.md](../scripts/FEATURES.md). `verify_main.py` owns isolated
launch, readiness, concurrent CLI input, drive, direct SQLite readback, nine
SIGKILL boundaries, SIGTERM, profile switching and cleanup. Its explicit
`-test-bridge main` and `-test-main-hold` hooks are never selected by production
configuration. `evidence/main.json` includes frozen requests/results and final
history, ToolInvocation, usage and delivery rows. The local/account harnesses
also execute this path through actual Node/Pi against owned HTTP fixtures.

GC-01/09/10 Main-only L0 fragments use versioned inputs and scripted Fake
responses. Reports keep their full Cases `not_applicable`, with no semantic
score. They prove protocol assertions, not real-model quality, live account
eligibility or current restaurant information. Full M1, Security/Approval,
Memory/compaction, Worker/Child and M5 JavaScript Durable Replay remain unclaimed.
M1 currently stops with a context-length error instead of silently trimming
history; semantic compaction is M6. Live account inference remains an unexecuted
L2 requirement, and Device Authorization remains deferred by ADR 001.
