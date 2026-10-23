# Terminal setup and conversation verification

Build with `make build` after installing the pinned Bridge dependencies. Launch
`bin/erii -config config.example.yaml` on a terminal, or explicitly use
the `tui` command. With redirected stdin, no-command startup retains the JSON
diagnostic/wait behavior used by infrastructure harnesses. `status`, `relay`,
and existing model/chat CLI commands retain their contracts.

The terminal is line-oriented and uses the existing Model control plane and
Actor. It adds no provider, Tool, execution loop, database, or migration.
The terminal uses the shared storage migrations. Empty and prior schemas are
covered by the state owner's upgrade tests.

Terminal input uses a noncanonical, bounded reader on macOS and Linux; it does
not depend on the kernel's smaller canonical line buffer. A line may contain up
to **16,384 UTF-8 bytes**. Oversized or malformed UTF-8 lines are rejected in
full, drained to the delimiter, and followed by a fresh prompt. Their prefixes
never reach Main or settings. Backspace/Delete removes a rune; Ctrl-U clears the
line (including an overflow); Ctrl-D on an empty line exits. Echo follows the
terminal's original setting. Normal exit, Ctrl-C and SIGTERM restore terminal
flags, control characters and timing settings. Polling and bounded reads allow
cancellation without changing the shared stdin/stdout nonblocking flag.

## Setup and repair

Fresh state opens a blocking connection picker:

1. Local / OpenAI-compatible endpoint: enter the URL and an optional **environment
   variable name**, never a key value. Erii imports that environment credential
   into SecretStore. Choose a discovered model or manually enter an ID when
   discovery is unavailable. Enter conservative context/output bounds; these
   are candidates until Probe succeeds. Discovered models are numbered: enter
   a number or model ID, or press Enter when only one model is listed. Invalid
   numeric bounds retry the same field instead of discarding setup.
2. OpenAI — Sign in with ChatGPT: open the private browser URL displayed on this
   terminal. Complete the browser flow, then choose an ID returned by this
   account's catalog. For reauthorization, provide the existing account ID.
   Hidden or unavailable models cannot be manually activated through this path.

Both paths require the actual streaming, Tool call/result continuation,
cancellation, and clipping Probe. The terminal then asks for explicit `yes`
before activating the tested profile through existing version checks. The
privacy disclosure appears before confirmation. Text is the supported input
mode; additional modalities and reasoning settings remain unverified and are
not offered as confirmed settings.

Each Probe check has its own 25-second deadline so a cold local model can load
and think. This leaves cleanup headroom before the existing 30-second Bridge
generation bound. The terminal names each check as it starts. A deadline reports the
failed check and elapsed limit, rather than presenting it as user cancellation.
The clipping check needs only a short answer longer than eight bytes; it does
not request thousands of repeated tokens. Configured context/output limits stay
unchanged. Failed setup prints the saved profile ID and exact `/test ID` and
`/select ID` recovery commands. A failed retest still revokes prior proof and
closes ingress. Ctrl-C exits during a check; `/cancel` applies at input prompts.

The numbered picker and explicit connection/model steps follow
[Pi 1.3.0's model selection flow](https://github.com/earendil-works/pi/blob/v1.3.0/packages/coding-agent/docs/models.md).
Erii keeps its line-oriented terminal, SecretStore, mandatory Probe and explicit
version-checked activation.

Use `/cancel` at any setup prompt. A fresh installation stays `setup_required`;
an existing active profile stays selected. Candidate metadata or an account
connection already created remains available for explicit repair/logout.
Ctrl-C cancels a pending browser login and closes the terminal. Login URLs are
private interaction data: do not paste them into logs or reports. Device
Authorization remains deferred under ADR 001.

| Command | Behavior |
| --- | --- |
| `/setup` | Add/repair a connection and probe/select its model |
| `/models` | Show profile IDs, model IDs, and status |
| `/test ID` | Explicitly rerun Probe; failed tests revoke existing proof |
| `/select ID` | Confirm activation of an already tested profile; versions are read before confirmation |
| `/login [ACCOUNT_ID]` | New ChatGPT login or reauthorization of the same account |
| `/logout ACCOUNT_ID` | Existing credential/session cleanup and proof revocation |
| `/history` | Redisplay stored semantic conversation |
| `/status` | Current effective readiness |
| `/quit` | Close the terminal and release foreground work |

`ready` accepts ordinary lines through `Actor.Submit`. The terminal drains the
same durable Main work, shows busy/idle, and reads replies from Conversation.
Input remains available while Main runs: new lines become ordered Steering in
the active Main, rather than a second concurrent executor. Profile changes
affect new Runs only. Startup reclaims unfinished Main work when ready and
reopens existing history. `degraded` retains history and settings while rejecting
new work. Raw streaming text and Tool traces are not conversation output.
SIGTERM/Ctrl-C cancel the adapter, join its input/executor goroutines, and close
the application. Unfinished durable work remains under the existing recovery
rules; this is not a Task pause/cancel feature.

## Reproduce and inspect

```sh
python3 scripts/verify_tui.py --binary bin/erii --evidence evidence/tui.json
python3 scripts/verify_terminal_input.py --binary bin/erii --evidence evidence/terminal-input.json
python3 scripts/run_cases.py --binary bin/erii --evidence evidence/cases.json
```

For an opt-in, read-only local model Live Shadow, use the maintained provider
harness. It discovers the exact model, tests it through the real CLI/Pi boundary
in a disposable data root, records proof and content-free timing/error rows,
and removes that state. It never selects a model or modifies your installation:

```sh
python3 scripts/verify_local_provider.py --binary bin/erii \
  --live-endpoint http://127.0.0.1:8000/v1 \
  --live-model mlx-community/Qwen3.6-35B-A3B-4bit \
  --context-tokens 262144 --output-tokens 32768 \
  --evidence evidence/local-live-probe.json
```

The terminal harness owns a Unix PTY, isolated data roots, an HTTP local-model
peer, a browser OAuth/Responses peer and an in-memory SecretStore socket. Both
provider paths execute the production Go → pinned Node/Pi path. It captures
redacted terminal transcripts and independently reads SQLite, generations,
snapshots, pure ToolInvocations, usage, delivery intents, and Conversation. It
drives discovery/manual setup, login cancellation, setup cancellation, degraded
ingress, SIGKILL/reopen, busy Steering and model freezing. Every child, PTY,
server, and disposable database is cleaned up before returning. Authentication
URLs and credentials never enter the evidence artifact. `make verify` repeats
the terminal cancellation/security/recovery boundaries five times.

It also retains an 11-second cold-start HTTP fixture, single-model Enter and
numbered account selection, hidden account-model rejection, numeric field
repair, fixed stage progress and failed-test recovery commands. These deterministic
checks require no live local model or online account.

The input harness adds 12 real PTY scenarios: the macOS review reproduction,
ASCII and multibyte byte boundaries, large overflow and next-line recovery,
malformed input, echo on/off, rune editing, Ctrl-U/Ctrl-D, partial-line signal
shutdown, unchanged shared file flags, and terminal settings readback. The
kernel's transient `PENDIN` retype status is excluded from the settings comparison;
all configuration flags, control characters and speeds are compared. The test
driver handles partial writes and drains output while pasting, so terminal echo
cannot hide dropped input or block the harness. The explicit `terminal-input`
Fake gives a short reply to isolate input limits from Tool argument/output limits;
the original Main Fake and both real Bridge fixtures retain their existing coverage.

`scripts/terminal_driver.py` owns terminal mechanics; `verify_main.py` owns
durable state readback. `case_runner.py` owns provider-independent variants,
assertions and result/scoring semantics. `run_cases.py` supplies the real
terminal driver. This keeps fixture/result policy separate from the adapter.

## Fixture and scoring contract

`evals/fixtures/m1-v2.json` copies the synthetic persona and GC-01/09/10 facts
and events from Golden Cases §8. It replaces the earlier `main-v1.json`, which
substituted different goals/times. The full fixture remains attached to each
run's digest. Each case has three wording variants, two irrelevant Conversation
noise variants (10–30% added characters), and an order variant that changes
only noncausal object-key ordering. Each variant starts in an isolated empty
database, with explicit synthetic context and timestamped input events.

The **scenario clock** is frozen in that explicit context and advances only at
the specified event times. The model has no wall-clock Tool. Host leases,
database timestamps and telemetry keep real wall time; result `started_at`,
`finished_at`, and latency describe the actual execution. Runtime-created IDs
remain Host-owned; results retain the actual immutable snapshots and map the
synthetic persona through explicit Conversation context rather than replacing
runtime identities. Every variant records seed 0, fixture/dataset/policy digests,
input events, binary/Bridge provenance, model/capability snapshots, assertions,
metrics, user-visible transcript, state and RunTrace.

`evals/scoring-v1.json` freezes the normative O/F/P/A/T/S/R/C weights,
0–4 range, 80-point Case threshold, 85-point suite mean and hard-failure policy.
The reusable scorer renormalizes only explicitly applicable dimensions; missing
grades never pass. Mechanical failures take precedence over semantic grades;
hard failures score zero. `m1-v2.lock.json` pins fixture, policy and normative
source bytes. Intentional changes require a reviewed version/lock update.

The `golden` Bridge is an **explicit scripted Fake**. Its response depends on
fixture context and turn ordinal, not exact prompt spelling. It verifies
protocol, persistence and fixture/assertion plumbing. It does not demonstrate
understanding, recommendation quality, or noise resistance by a real model.
Successful results are `fragment_pass`, with `final_score: null`. Full Cases
and later capabilities are `not_applicable`; zero scored Cases and null suite
scores/pass@1/pass^5 prevent a scripted run from being mistaken for a product
quality pass. Deterministic repetitions prove mechanical boundaries, not model
sampling stability. A real-model five-run comparison and semantic Judge have
not executed. L2 Live Shadow remains **inconclusive**.

M1 supplies facts as explicit context. It does not claim Health/Weather/Calendar/
email connectors, stale-weather refresh, Memory, Automation, Task/Worker/Child,
general M3 approval/security, external delivery or M5 JavaScript Durable Replay.
These gaps stay visible per result. M3 local transport and approval evidence is
maintained separately in [Delivery operations](delivery.md). No full release or 80/85-point product acceptance is claimed here.
