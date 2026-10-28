# 开发、验证与交付

[主索引](../feature-map.md) · [技术方案](../designs/engineering.md) · [测试方案](../test-plans/engineering.md)

## 功能与需求

<a id="f-038"></a>

### F-038 · 标准开发与显式发布流程

开发者能安装固定依赖、构建、按风险验证并准备可审查交付。现有 Makefile、测试 runner、CI 和原生打包入口已实现；成功的软件回归不代表真实模型效果或发布授权。完成状态以主索引为准。

验收目标：可复现构建，相关回归保持真实断言，失败不被当作通过；发布继续需要明确授权。

## 如何使用

以下维护开发命令与迭代流程。当前 CI 技术组织见并列技术方案，测试层级与执行说明统一在 [产品测试方案](../test-plans/product.md)。

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="ci-local-use"></a>

### Local use

```sh
make test lint
make test-e2e E2E_EVIDENCE=evidence/my-change
make test-full E2E_EVIDENCE=evidence/full-check
make test-race
```

The system runner refuses to overwrite a previous summary. Select a fresh
output directory when rerunning; retain a failed run when diagnosing it. Use
`python3 tests/system/runner.py --list --suite full` to inspect the runnable
software inventory. Results are ignored local outputs, not committed audit ledgers.

<a id="contributing-contributing-to-erii"></a>

## Contributing to Erii

Start with [AGENTS.md](../../AGENTS.md), the code being changed and its nearby tests.
Use the [testing guide](../test-plans/product.md) for test ownership and commands, and the
[feature designs and runtime contracts](../designs/product-overview.md#contract-0-文档效力与实现规则) for product protocols and invariants.

<a id="contributing-development"></a>

### Development

Use Go 1.26.2+, Node 24.21.0, Python 3.10.12+ and a C compiler. On Linux,
coding isolation requires Bubblewrap and user namespaces. Install dependencies:

```sh
make setup
make build
make run
```

On a clean Ubuntu host, browser system libraries can be installed with
`python3 scripts/setup.py --part browser --with-deps`. Node must match
`.node-version`. Keep npm, Go and Playwright's native download caches; do not
share mutable `node_modules`. Setup uses frozen npm lockfiles, ignores lifecycle
scripts and verifies cached packages. Use `SETUP_FLAGS='--mode offline'` when
all required downloads are already cached. Offline audit output does not prove
current advisory coverage. Do not disable TLS, checksum or signature verification.

<a id="contributing-product-work-first"></a>

### Product work first

Start from a user problem and an observable result. Fix product defects with
focused regressions. Standalone CI tuning, new verification frameworks, Golden
Case expansion and clause-audit work are paused unless explicitly requested by
the owner. Do not create a new script when an existing test or command can prove
the behavior. An explicitly requested engineering-governance change is in scope.

Multica is retired. Old ERII tasks and historical coordination notes do not
authorize new work. Link an issue if one exists; a branch or PR does not require
an issue identifier. Do not recreate the old task hierarchy in another system.

<a id="contributing-feature-documentation"></a>

### Feature documentation

Use the [Feature Map](../feature-map.md) as the product requirement index.
Register each requirement with a stable ID, a short description, completion
status and a link to its feature document. Feature documents explain behavior,
requirements, acceptance and current usage, then link a technical design and a
parallel test plan. Keep simple test plans small by referencing existing checks.

Keep architecture and flow sections current. Append architecture/flow decisions,
alternatives, reasons and compatibility effects to the design's changelog;
do not leave obsolete diagrams in the current design or invent historical dates.
Clearly distinguish implemented behavior, accepted plans and open proposals.
Test plans map requirement IDs to observable scenarios, existing entry points
and coverage gaps. Execution reports remain separate from plans.

Update the index, feature usage, design and test plan with the implementation.
Do not mark a requirement complete merely because a schema, renderer, fixture
or test file exists. See the [maintenance rules](../feature-map.md#文档维护规则)
and templates for the exact structure. Cross-feature protocols live in the runtime contracts and owning designs;
historical decisions remain in their design changelogs.

<a id="contributing-working-on-a-change"></a>

### Working on a change

1. Define the concrete behavior and the evidence needed to prove it.
2. Inspect the implementation, callers, closest instructions and existing tests.
3. Reproduce a bug where practical. Preserve unrelated user files and changes.
4. Implement one complete change. Keep approval, credentials, data integrity and
   replay guarantees intact. Ask about actual product/protocol conflicts.
5. Run focused checks while editing, then the proportional gates below.
6. Review the diff and remove temporary outputs. Update current documentation.
7. Report the behavior, checks/results and remaining limitations. Distinguish
   failed, skipped and unrun checks; never turn scripted responses into model scores.

| Change | Evidence |
| --- | --- |
| Documentation | Inspect content and run `python3 scripts/check_docs.py` |
| Local logic | A focused existing unit regression or necessary new one |
| State/store/adapter | Real store or protocol integration tests |
| Approval, cancellation, concurrency, replay | Relevant failure/restart tests and race detector |
| CLI/UI/API | Drive the real interface and inspect resulting state |
| Cross-module change | `make test lint`, relevant `make test-e2e` gates |
| Release | Extended regression, race checks, native packages and relevant live compatibility checks |

Go package tests currently mix unit and component integration checks; `go test
./...` is not called a pure UT suite. The nine deterministic core system gates
run in PR CI. Extended system coverage and the full race inventory run separately
on a weekly schedule or manual request. See [CI](../designs/engineering.md#ci-continuous-integration-and-delivery).

<a id="contributing-runtime-and-delivery"></a>

### Runtime and delivery

`ERII_DATA_ROOT` selects an explicit data directory. Tests use disposable roots;
never use a personal application root as a fixture. `doctor` is read-only;
`status` can initialize or migrate state. Credentials belong in SecretStore,
never YAML, source, test outputs or logs. No command selects a paid/default/Fake
model implicitly. Packaging uses `sh scripts/package.sh /absolute/output`.

Keep PRs focused. Describe the user behavior or engineering problem, resulting
change and relevant validation. CI success is software regression evidence;
it does not certify real-model quality, real-account login or release acceptance.
Creating a version tag or publishing a release requires an explicit request.
