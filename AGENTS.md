# Erii — Repository Instructions

Build the smallest complete change that satisfies the request, prove it with inspectable evidence, and continue through verification without waiting for routine approval.

This file is a compact router and a list of non-obvious invariants. It does not replace the technical baseline, Golden Cases, tests, or code.

## 1. Read only the context the task needs

Do not read every design document before every edit. Start with the target code, nearby tests, and the closest `AGENTS.md`. Use `rg` and `rg --files` to find the relevant specification.

| Task | Read |
|---|---|
| Architecture, protocol, state, storage, security, replay, or milestone work | Relevant sections of `docs/technical-design.md` |
| Product behavior, eval fixtures, user-visible claims, or release gates | Relevant cases plus scoring and hard-failure rules in `evals/golden-cases.md` |
| Model-visible Tool changes | Baseline Tool, Security, Coding Agent Contract, and Definition of Done sections |
| Small local refactor or bug fix | Target implementation, callers, tests, and only the specification needed to resolve intent |

The baseline defines intended semantics. Tests and code show current reality. If they disagree, identify the exact conflict. Do not invent a compatibility path or silently choose one side.

Ask for a decision only when the conflict changes product behavior, a public/model-visible protocol, data compatibility, security, or Durable Replay. Continue safe read-only diagnosis while waiting.

Do not edit `sources/` reference material if that directory exists.

## 2. Non-negotiable product boundaries

- Use one reusable Agent Loop Core. `Main` and `Worker` are roles. `Root Worker` and `Child Worker` are positions in a Run tree, not separate loops.
- Main owns user conversation, Task decisions, and final delivery. Main does not create Child Runs and does not receive `agent.run()`.
- Main creates or appends a Task through `task_manage` when work is asynchronous, long-running, or needs parallel Child Runs. Short synchronous work stays in Main.
- Model-visible Task tools are only `task_search` and `task_manage`.
- `agent.run()` always creates one Child Run. It never creates a Task. A Child may call `agent.run()` again within Host limits.
- `agent.run()` starts a general Worker. The caller cannot select an agent, model, Skill, Tool set, permission set, output schema, timeout, label, or replay key.
- Parent and Child exchange only an explicit `task`, accessible Artifact references, and the final result/error. They do not share mutable run context or peer messaging.
- A Child inherits the Parent Worker’s frozen ModelSnapshot and Capability Snapshot, then applies fixed reductions. A Child cannot gain Task control, user delivery, credentials, long-term Memory writes, or any capability absent from its Parent.
- Worker results return only to the direct Parent. Workers never message the user.
- Do not add `workflow.*`, `parallel.*`, agent handles, sibling messaging, a budget query tool, or a second capability discovery API.
- Use standard JavaScript for sequencing and concurrency. The Host remains the real concurrency and resource limit.
- Direct Tools, `tools.*`, `cua.*`, and `agent.run()` share ToolInvocation, approval, error, Artifact, cancellation, and replay semantics. Do not build a second execution pipeline.

The MVP JavaScript boundary is:

```ts
interface AgentRunRequest {
  task: string;
  artifactRefs?: readonly ArtifactRef[];
}

interface AgentResult {
  readonly text: string;
  readonly artifactRefs: readonly ArtifactRef[];
}
```

Do not expose Run IDs, usage, traces, status unions, partial results, or JSON-constrained business output through `agent.run()`.

## 3. Runtime invariants that must not drift

- Task states are `active`, `paused`, `completed`, and `cancelled`. A failed Run does not create a `failed` Task state.
- Pause is a controlled, recoverable stop. Cancel propagates down the Run tree. A late result from an old epoch or owner is rejected by fencing.
- A user request to “continue” normally uses `task_manage(append)` and creates a new Root Run. Do not revive the old Run.
- The Host retries only known transient infrastructure failures. JavaScript or the Parent decides what to do with timeouts, model failures, or inadequate results.
- Durable Replay uses fixed source and frozen snapshots. Reuse completed calls. Reattach or continue incomplete calls. Never retry an external side effect whose outcome is `unknown`.
- The trusted Runtime creates call keys. Models and JavaScript never create internal IDs, `call_key`, or provider idempotency keys. Parameter drift under one key fails closed.
- A Host-accepted unawaited call continues after the script ends. Its result stays internal and does not enter the finished script output.
- An external side effect requires the configured approval for that exact frozen action. Approval does not transfer from Parent to Child.
- With no active ModelProfile, enter `setup_required`. Support a local OpenAI-compatible URL and Sign in with ChatGPT. Never enable an implicit paid/default/Fake provider.
- Freeze ModelSnapshot at Run creation. Profile changes affect only new Runs. A Root tree and its Durable Replay never switch model, endpoint, wire protocol, Tool schema, or compatibility snapshot.
- Secrets live only in the credential store. Never put credentials in SQLite, YAML, prompts, Artifacts, logs, traces, or errors.

Treat any change to these points as a protocol change. Update the baseline, migration/compatibility plan, and conformance tests together.

## 4. Work autonomously to a verifiable finish

For implementation requests, continue through code, focused tests, inspection, and fixes. Do not stop after the first plausible patch.

You may autonomously:

- inspect the repository and local Git state;
- make scoped, reversible edits;
- run disposable local tests, linters, Fakes, and development services;
- fix failures caused by the requested change and rerun affected checks;
- create temporary local evidence and remove it before handoff.

Stop and ask only for a real product decision, missing credential, permission boundary, destructive operation, production/external side effect, or material expansion of scope. Do not ask for routine permission to read files, edit in scope, or run safe local checks.

Protect the workspace:

- Preserve all user changes. Do not overwrite, revert, reorder, or include unrelated edits.
- Do not use destructive Git commands or bypass hooks.
- Do not weaken tests, swallow errors, or relax assertions to obtain a green run.
- Add a production dependency only when it solves a current requirement. Explain material license, cost, security, or architecture impact before proceeding.

## 5. The iteration loop

1. **Define done.** State the observable behavior, invariant, or fixed fixture. Name the evidence that will prove it.
2. **Interview the repo.** Inspect the entry point, callers, state owner, failure path, and nearest tests. Ask the user only for facts the repository cannot reveal.
3. **Reproduce first.** For a bug, create a repeatable failure or record why a safe reproducer is infeasible.
4. **Build one vertical slice.** Connect the real entry point to domain logic, persistence/tool boundary, and observable result. Include the migration, Fake, test, and minimal documentation the slice requires.
5. **Verify the real thing.** Run the CLI, read back stored state, inspect the API response, drive the live UI, or perform a real kill/restart cycle as appropriate.
6. **Review and clean.** Inspect the diff. Remove debug code, temporary Artifacts, dead paths, duplicate abstractions, and unrelated changes.
7. **Report evidence.** Distinguish verified facts from inference and unverified gaps.

“It compiles,” “the code looks correct,” and an Agent’s self-report are not proof. If a required check cannot run, call the result `inconclusive` and name the missing evidence.

## 6. Keep changes narrow and architecture explicit

Use the project structure defined in the baseline:

- `cmd/erii` is the only Go process entry point.
- `internal/app` is a composition root with no business logic.
- Define an interface in the module that uses it. Put implementations in named subpackages such as `memory/sqlite` or `event/nats`.
- Do not create generic `ports`, `adapters`, `controller`, `service`, or `repository` hierarchies.
- Do not introduce remote RPC, public `pkg/`, multi-model routing, a Workflow domain, or cloud service splits for hypothetical future needs.
- The model-provider runtime has no Tool, Computer, or domain-database access.
- The JavaScript runtime has no model credentials, arbitrary network/filesystem access, dynamic imports, or non-replayable time/random sources.
- Models and Tool providers never write domain databases directly.

Prefer the smallest coherent implementation. Do not keep old and new paths, temporary compatibility layers, backup files, or speculative abstractions unless an approved migration requires them.

## 7. Verification should match the risk

Do not run the full suite for a typo. Do not use a narrow unit test to approve a replay or side-effect change.

| Change | Minimum evidence |
|---|---|
| Docs, comments, local naming | Inspect rendered text or diff; run a formatter if relevant |
| Pure logic or bug fix | Focused unit/regression tests, including important boundaries |
| State, schema, or persistence | Legal/illegal transitions, migration from empty and prior schema, CAS/conflict behavior |
| Tool, approval, or side effect | Contract test, frozen parameters, rejection path, retry policy, crash before/after side effect |
| Concurrency, cancel, or replay | Duplicate/late/out-of-order events, old epoch, race detector, kill/restart evidence |
| UI, CLI, API, or Artifact | Exercise the running user path and capture inspectable output |
| Performance | Same environment, input, and harness before/after; report noise and samples |
| Milestone or release claim | Full required checks, fault injection, and all Golden Cases due at that milestone |

M0 must establish real, documented equivalents of:

```sh
make test        # deterministic; no online accounts
make test-race   # Go race detector
make lint        # formatting, lint, static checks
make verify      # milestone integration/protocol gates
```

Until these commands exist, inspect and use the repository’s actual commands. Never claim an example command ran.

Every external dependency must have a deterministic Fake for CI. Real accounts, websites, and macOS VM compatibility belong in release-time Live Shadow runs, not required CI.

### Verification harness is product infrastructure

Do not make a human relay observations between the running product and the Agent. Give the Agent a stable way to launch, drive, observe, and clean up each important product surface.

For each primary UI, CLI, API, and Computer workflow, maintain a project-local verification harness with:

- `Launch` — start an isolated instance with known configuration and fixture data;
- `Doctor` — prove dependencies, ports, auth/Fakes, and test state are ready;
- `Drive` — exercise the feature through the same boundary a user or integration uses;
- `Evidence` — capture responses, state, logs, traces, screenshots, or Artifacts that prove the result;
- `Cleanup` — stop processes and remove disposable state without touching user data.

Keep a lightweight feature map beside the harness. Index each user-visible feature, how to reach it, its authoritative entry point, and the evidence that proves it works. Update the affected feature entry in the same change as behavior.

The harness proves observed behavior. It does not prove code quality, performance, security, or distributed invariants. Use types/lint/review, measured performance checks, security tests, and state/replay tests for those separate claims.

Promote stable mechanical actions into reusable commands. Do not let each Agent generate a new Playwright script, curl chain, profiling helper, or migration probe for the same operation. A reusable CLI or script owns mechanics; the Agent decides when to run it and how to interpret evidence.

Before widening parallelism or adding unattended automation, prove the harness end to end on one real task. Automation amplifies the repository’s current quality; it does not create trust by itself.

## 8. Trust before parallelism

Parallelize only when each slice has:

- independent scope or explicit file ownership;
- concrete inputs and a finish condition;
- its own verification command and inspectable output;
- one named integrator for protocol consistency and final validation.

Good parallel work includes read-only research, independent fixtures, non-overlapping modules, or competing prototypes. Do not parallelize an unknown root cause, one shared state machine, one migration, or competing implementations of the same protocol.

The integrator must inspect diffs and evidence. If one Agent cannot verify its own work reliably, improve the harness before adding more Agents.

## 9. Turn repeated corrections into infrastructure

When a failure repeats, fix it at the strongest practical layer:

1. architecture, data structure, or API that makes the bad state impossible;
2. type system, compiler, lint, dependency rule, or CI gate;
3. deterministic regression test, fault injection, or verification harness;
4. focused Skill or nested `AGENTS.md` for a repeatable judgment-heavy workflow;
5. root `AGENTS.md` only for a cross-repository invariant that cannot be enforced above;
6. one-off review comments only to discover the missing control.

Treat the repository as Agent memory. Remove bad examples and one-off workarounds before later Agents copy them. Build one paved path for common operations.

Keep Skill descriptions short and specific about when they apply. Use progressive disclosure: a Skill root should route to only the references or scripts needed for the active workflow. Do not encode a long itinerary when the model can infer routine steps.

## 10. Make output easy to supervise

Human review, not code generation, is the scarce resource. Use the simplest format that makes the result unambiguous and inspectable.

- Use short sentences, active voice, one action per procedural sentence, and one stable term per concept.
- For cross-Agent instructions, state the goal, input, constraints, evidence, and stop condition. Avoid synonyms for protocol terms.
- Use a table for exact mappings. Use a diagram for state, sequence, ownership, or multi-branch flow. Use an interactive or rendered Artifact only when it materially reduces review effort.
- Explanatory HTML, diagrams, traces, or dashboards are disposable review aids by default. Do not add them to the product or commit them unless the task requires it.
- Do not compress away uncertainty or omit evidence to make prose shorter.

Final handoff:

1. **Outcome** — what now works.
2. **Evidence** — exact checks and inspectable results.
3. **Risk** — behavior most likely to regress or any proof gap.
4. **Open** — only unresolved decisions or blockers; write `None` when empty.

Do not say a check passed if you did not run it. Do not describe “should work” as verified.

## 11. Milestones and done

- Deliver `M0 → M7` as runnable vertical slices. Do not build an alternative architecture while an earlier milestone is red.
- `M0–M5` is Core MVP. `M6–M7` completes the defined first release.
- An internal M2/M4 build must not claim unimplemented Durable or Security semantics.
- A milestone includes required migration, Fakes, tests, command-line reproduction, operations notes, kill/restart evidence, and due Golden Cases.
- A Golden Case passes only at 80+ with no hard failure. The suite average is 85+, and the lowest case is 80+.
- Security, approval, cancellation, Durable Replay, and Coding workspace protection cases must pass five consecutive runs.
- Record an approved architecture deviation in an ADR and update the baseline before merge.

A task is done when the requested behavior works, proportional verification passes, the diff is clean, documentation matches implementation, and the final handoff makes the evidence and remaining risk clear.

## 12. Maintain this file as a feedback loop

Do not grow this file after every correction.

- Add a root rule only after a repeated, cross-cutting failure.
- Prefer code, lint, tests, CI, or a focused Skill when they can enforce the rule.
- Put module-specific guidance in the nearest nested `AGENTS.md`.
- Remove a rule when stronger infrastructure makes it redundant.
- Periodically audit for stale, duplicated, contradictory, overly defensive, or model-specific instructions.
- Keep this file below the Codex project-instruction budget. Route to maintained documents instead of copying them here.
