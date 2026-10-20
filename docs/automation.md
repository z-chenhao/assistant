# Automation operations

Automation persists user intent separately from Task. The scheduler writes one
`automation_fire` and `ai.erii.automation_fired.v1` Outbox event atomically. It
never runs a model. Actor's ordinary `Next` recovery consumes authoritative fire
rows, writes its Inbox receipt, creates an ongoing Task if needed, freezes the
current model/capabilities in a new Root, and emits `run_ready` in one transaction.
Execution uses the existing RunClaim, Agent Loop, ToolInvocation, approval and
Main Reporter/Delivery path. Lost or repeated queue wakes cannot duplicate work.

Run the foreground scheduler/executor with an active, tested profile:

```sh
personal-agent automation work
```

Keep that command running under the installation's process supervisor. It scans
at startup and at one-second intervals while idle. SIGINT/SIGTERM stops and joins
its work at existing recoverable boundaries; process exit does not cancel intent.
TUI/CLI ingress may run in another process against the same configured data root.
`chat work` also scans due Automation during its normal Actor drain.
`automation tick` only persists due events and exits; it does not execute them.
No scheduler process is implicitly detached by the application.

Create from a JSON object on stdin, with a stable request key for retries:

```sh
personal-agent automation manage monthly-review < review.json
personal-agent automation list
```

`review.json`:

```json
{"operation":"create","instruction":"Remind me to review my long-term goals.","triggerType":"cron","triggerSpec":"0 20 * * 0L","timezone":"Asia/Shanghai"}
```

`at` uses an RFC3339 timestamp with explicit offset. Every schedule stores an
explicit IANA timezone (`UTC` works; process-local `Local` is rejected). Cron has
five numeric fields: minute, hour, day of month, month, weekday (0=Sunday through
6=Saturday). Lists, ranges and positive steps work; `0L` through `6L` in the
weekday field select that month's last weekday. Restricted day-of-month and
weekday fields use ordinary cron OR semantics; wildcard steps retain wildcard
semantics (for example `0 8 */1 * 0` still means Sunday). See the
[crontab day-field rule](https://man7.org/linux/man-pages/man5/crontab.5.html). A new schedule fires strictly
after creation/edit time. Nonexistent DST wall minutes are skipped; both distinct
UTC occurrences of a repeated wall minute fire. Invalid calendars fail closed.

Missed occurrences coalesce to the original persisted due occurrence; cron's
next fire advances strictly past the recovery scan time. This deliberately avoids
replaying every missed day. Already accepted fires remain durable and execute
in order, with at most one live Root for the ongoing Task. A failed Root leaves
the Task active. A completed Root also leaves an Automation Task active.

Update supplies `operation: "update"`, `automationRef`, current `version` from
list, and the complete new instruction/schedule/timezone/optional threshold.
Cancel supplies only `operation: "cancel"`, `automationRef` and `version`.
Each schedule mutation is CAS; stale versions return `INVALID_ARGUMENT` without
writes. Scheduler bookkeeping and observations also advance the visible version;
read list again after a conflict. Edit increments an independent schedule version,
invalidating queued fires and suppressing stale results. Current in-flight steps
keep their existing snapshots and approval boundaries. Cancel disables future
fires and uses the existing Task cancel/epoch mechanism for the current tree.
Task pause/cancel also disables its Automation. A stopped Task cannot be restarted
by a delayed fire or schedule edit; create a new Automation to establish new intent.
Completed one-shot `at` intent retains `enabled=true, next_fire_at=NULL` for audit;
it never fires twice. Cancellation is explicit and disables it.

## Threshold monitoring

A create/update may include `below`, a nonnegative integer threshold (maximum
10^12, same unit as observations). The instruction specifies the real data source
and business units. This is not a price-fetching service: the Root uses its
existing scoped Tools to obtain evidence, then calls
`builtin__automation__observe({"value":1750})`. The Host binds the call to that
Root's persisted fire; the model cannot choose identities, dates or replay keys.
Only the linked Root may write an observation. Main and other Roots/Children
cannot update that state. Claims and schedule versions fence late writes.

The Host records one authoritative value per fire. Repeated sources returning
that value are idempotent; a different value under the same fire is
`NON_DETERMINISTIC_REPLAY`. A later observation can change the state from outside
to inside the threshold, permitting one completion notice. Further below-threshold
values remain quiet until an outside value resets it. Old/out-of-order fires do
not reset newer monitor state. No valid observation produces one `needs_input`
update rather than an invented measurement. Failures/unknown effects still use
the existing meaningful-notification path. Successful quiet observations create
no Main turn or DeliveryRequest. Workers never deliver directly to the user.

## Tool contract and compatibility

| Tool | Exposure | Permission | Replay | Errors |
| --- | --- | --- | --- | --- |
| `builtin__automation__manage` | Main | `task_control` | safe; frozen invocation and source/action receipt | invalid request/CAS: `INVALID_ARGUMENT`; missing ref: `NOT_FOUND`; Worker: hidden/denied |
| `builtin__automation__observe` | Only monitoring Automation Root; Main/ordinary Roots/Children hidden | `automation_observe` | safe; immutable per-fire observation | invalid value: `INVALID_ARGUMENT`; wrong role/fire: `PERMISSION_DENIED`; value drift: `NON_DETERMINISTIC_REPLAY` |

The shared ToolExecutor enforces frozen schemas and exact action approval for
any downstream external effect. Automation grants no credentials or effect
approval. Existing frozen capability snapshots gain no Tools. New Root trees
freeze the active profile at admission, not when the schedule or fire was created.

Migration 023 adds Automation, fire identities, source/action receipts, due and
pending indexes, version/transition/audit constraints. Empty and all prior schemas
upgrade forward. Populated v22 migration and partial-DDL rollback have dedicated
tests. Back up the whole data root with the process stopped before upgrading;
older binaries reject schema 23. No credentials enter these records.

## Verification

```sh
make build
python3 scripts/verify_automation.py --binary bin/personal-agent --evidence evidence/automation.json
go test -race ./internal/automation ./internal/agent/sqlite -run 'TestAutomation|TestCalendar|TestRejectMalformed' -count=5
```

The reusable harness owns isolated Fake profile setup, CLI/model Tool ingress,
fixed clocks, state/Tool/snapshot/delivery readback, actual SIGKILL at four
fire/Outbox/admission boundaries, restart and stable duplicate drain. It joins
children and deletes disposable state. `make verify` runs it five consecutive
times. `evals/fixtures/automation-v1.json` pins GC-02/14 scheduling fragments;
GC-08 covers reminders and delayed cancellation. These are deterministic L0/L1
protocol checks. They do not award full Golden Case semantic/model scores or
claim channels and all M7 hardening are implemented.
