# Terminal setup and M1 Case Runner

Build with `make build` after installing the pinned Bridge dependencies. Launch
`bin/personal-agent -config config.example.yaml` on a terminal, or explicitly use
the `tui` command. With redirected stdin, no-command startup retains the JSON
diagnostic/wait behavior used by infrastructure harnesses. `status`, `relay`,
and existing model/chat CLI commands retain their contracts.

The terminal is line-oriented and uses the existing Model control plane and
Actor. It adds no provider, Tool, execution loop, database, or migration.
Existing state v7 and Conversation v1 open unchanged; the prior migration tests
continue to cover empty and older databases. No schema bump is required.

## Setup and repair

Fresh state opens a blocking connection picker:

1. Local / OpenAI-compatible endpoint: enter the URL and an optional **environment
   variable name**, never a key value. Erii imports that environment credential
   into SecretStore. Choose a discovered model or manually enter an ID when
   discovery is unavailable. Enter conservative context/output bounds; these
   are candidates until Probe succeeds.
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
python3 scripts/verify_tui.py --binary bin/personal-agent --evidence evidence/tui.json
python3 scripts/run_cases.py --binary bin/personal-agent --evidence evidence/cases.json
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
These gaps stay visible per result. Existing local delivery rows remain pending
intents. No full release or 80/85-point product acceptance is claimed here.
