# Erii — Repository Instructions

Build the smallest complete change that satisfies the request, prove it with inspectable evidence, and continue through verification without waiting for routine approval.

Start at [README](README.md) for installation and [CONTRIBUTING](CONTRIBUTING.md) for the implementation workflow, risk-based checks, review and cleanup. Use the [feature map](scripts/FEATURES.md) to find maintained verification entry points. These instructions retain product invariants; the baseline defines their full semantics.

## 1. Read only the context the task needs

Do not read every design document before every edit. Start with the target code, nearby tests, and the closest `AGENTS.md`. Use `rg` and `rg --files` to find the relevant specification.

| Task | Read |
|---|---|
| Architecture, protocol, state, storage, security, replay, or milestone work | Relevant sections of [technical baseline](docs/technical-design.md) |
| Product behavior, eval fixtures, user-visible claims, or release gates | Relevant cases plus scoring and hard-failure rules in [Golden Cases](evals/golden-cases.md) |
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

## 4. Architecture boundaries

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

## 5. Delivery and instruction maintenance

Follow [the contribution workflow](CONTRIBUTING.md#working-on-a-change) through verification and final evidence. Preserve user changes, never weaken checks, and ask only for a real decision or permission boundary.

Keep this file a router and a list of cross-repository product invariants. Put mechanical controls in code, lint or tests, module guidance in the nearest nested `AGENTS.md`, and general workflow in CONTRIBUTING. Do not edit the auto-managed runtime block below or include it in a product commit.
