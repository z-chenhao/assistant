# End-to-end verification

Use the maintained suite for architecture migrations and refactors. Its gates
launch isolated instances, check dependencies, drive real CLI/UI/API boundaries,
read back authoritative state and artifacts, then join owned processes and remove
fixture data. Tests use deterministic peers and need no online account. See the
[feature map](../../scripts/FEATURES.md) for exact invariants and live proof limits.

## Run and inspect

Install the [development prerequisites](../../CONTRIBUTING.md#prerequisites-and-setup),
then run:

```sh
make build
python3 scripts/verify_suite.py --list
python3 scripts/verify_suite.py --list --shard security
make verify
```

The JSON catalogue comes from the execution inventory itself. Each entry names
its shard, gate, script, repetition count and reports. Every standalone `verify*`
and `run*cases` harness must be registered; `make test` rejects inventory drift.

| Capability | Shard | Named gates |
| --- | --- | --- |
| Persistence, recovery and operations | runtime | foundation, operations, capacity, events |
| Conversation, Tasks and delegation | runtime | main, tasks, children, task-control, scheduling, spend |
| Proactive work and task scenarios | runtime | automation, cases, task-cases |
| JavaScript, replay and coding | codemode | codemode, code-recovery, core-cases |
| Computer lifecycle and input | codemode | computer, computer-use |
| Memory, Tools and Artifact security | security | memory-maintenance, recall, tools, artifacts, gc03, gc04, gc05, gc06 |
| Delivery, Web and Feishu | delivery | delivery, channels |
| Exact health, goals, communication, monitoring and research journeys | communication | gc01, gc02, gc12, gc08, gc14, gc16, gc09, gc10 |
| Personal and multimodal journeys | personal | m6-cases |
| Install, model setup and credentials | providers | startup, local-provider, account-initial, account, models |
| Terminal and capability contracts | providers | tui, terminal-input, capabilities, pi-bridge |
| Unscripted evaluation entry mechanism | providers | unscripted-entry |
| Audit reference identity through squash and consumer merges | providers | audit-references |

A shard runs its complete inventory:

```sh
make verify VERIFY_SHARD=security VERIFY_EVIDENCE=evidence/security
```

For a focused change, select named gates without inventing new drive mechanics:

```sh
python3 scripts/verify_suite.py --binary bin/erii --gate main --gate artifacts --evidence evidence/focused-check
```

Selection preserves every chosen gate's full repetition count, including five
consecutive safety rounds. Unknown, duplicate and out-of-shard names fail before
launch. `--gate` cannot combine with CI part selectors. Reports live under
`EVIDENCE/focused/SELECTION_HASH/`, have `complete_inventory: false`, and cannot
replace complete shard verdicts or Code Mode/M6 summaries. A failed round stops
that series; later success does not erase the failure.

## Evidence and failure diagnosis

Complete runs write `suite-all.json` or `suite-SHARD.json`, per-shard timings,
per-gate logs, harness profiles and gate-specific JSON reports beneath the chosen
evidence directory. Focused runs write `suite-focused.json` in their selection
directory. Reports bind source revision/diff and binary digest, record exit codes
and owned-process drainage, and fail if tracked source or binary changes during
the run. Commit new files before collecting review evidence; revision/diff binding
does not fingerprint untracked files.

Read the first failing gate's log and report before rerunning. Fix the cause and
use a fresh evidence directory to retain the failed attempt. Every harness owns
cleanup on success, failure and interruption; `owned_group_drained: false` fails
the suite. Do not run against a user data root or manually relay UI observations.

## Complete acceptance

```sh
make test
make test-race
make lint
make verify
```

CI runs the same complete inventory, partitioning only expensive Code Mode/M6
worlds. Exact-source joins reject incomplete, unstable or mismatched reports.
See [CI operations](../ci.md) for topology, timings and native-package release
checks. Focused success is change-specific evidence, never a whole-project pass.

Offline conformance proves protocol behavior, including real process death and
restart. Semantic scores, actual model accounts and macOS Guest compatibility
require separate [Golden Case](../../evals/golden-cases.md) and Live Shadow
evidence. Do not infer those results from a green deterministic suite.
