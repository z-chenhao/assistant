# 模型连接与账户

连接、验证和选择模型，并管理 ChatGPT 账户。

[需求主索引](../feature-map.md) · [技术方案](../designs/model-connections.md) · [测试方案](../test-plans/model-connections.md)

## 功能说明

用户能连接 OpenAI-compatible 端点，测试后显式选择活动模型。

用户能登录、重新认证和退出 ChatGPT 账户，凭据可安全持久化。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-001"></a>

### F-001 · 本地模型连接与选择

**需求**：用户能连接 OpenAI-compatible 端点，测试后显式选择活动模型。

**当前实现**：CLI/TUI/Web 有连接、Test、Select 入口，Run 冻结模型快照。

**边界与剩余工作**：无活动模型时保持 setup_required；不隐式选取收费或 Fake 模型。

**验收目标**：连接、测试、选择、工具续轮和重启均沿真实 Provider Bridge 验证。

<a id="f-002"></a>

### F-002 · ChatGPT 登录与跨平台凭据

**需求**：用户能登录、重新认证和退出 ChatGPT 账户，凭据可安全持久化。

**当前实现**：账户生命周期和 macOS Keychain 已实现；非 macOS 原生凭据写入返回错误。

**边界与剩余工作**：补齐支持平台的凭据后端及相应设置反馈；不能把环境变量 API Key 视为 OAuth 凭据写入实现。

**验收目标**：在每个声明支持的平台完成登录、刷新、退出、进程中断恢复和凭据隔离验证。

## 如何使用

标准安装后运行 `erii`，按引导连接本地模型或登录 ChatGPT，再执行测试和选择。Web 的 Settings 提供相应入口。连接视觉模型时显式勾选 **Enable image input (verified when tested)**，通过图片 Probe 后再选择；模型卡显示已验证图片输入或 Text only。图片发送规则见 [F-013](artifacts-media.md#f-013)。

CLI 可运行 `erii model list` 查看配置，使用 `erii model test PROFILE_ID` 测试。ChatGPT 当前完整凭据路径依赖 macOS 与 cgo；Linux 可使用明确配置的本地端点或环境变量 API Key，不能据此宣称 OAuth 已跨平台可用。

## 详细操作参考

- [模型连接操作详情](#provider-bridge-chatgpt-account-lifecycle)
- [安装与部署详情](installation.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="provider-bridge-chatgpt-account-lifecycle"></a>

### ChatGPT account lifecycle

Use the same explicit Bridge configuration shown below. On macOS with cgo,
OAuth credentials use Erii's own Keychain service. Other platforms fail closed
without a writable OS SecretStore; environment API keys cannot authenticate this
entry point. No Codex credential file is read or written.

```sh
bin/erii -config local.yaml model login-chatgpt
bin/erii -config local.yaml model accounts
bin/erii -config local.yaml model account-catalog ACCOUNT_ID
bin/erii -config local.yaml model account-add ACCOUNT_ID RETURNED_MODEL_ID
bin/erii -config local.yaml model test PROFILE_ID
bin/erii -config local.yaml model select PROFILE_ID PROFILE_VERSION CONFIG_VERSION
bin/erii -config local.yaml model refresh ACCOUNT_ID
bin/erii -config local.yaml model logout ACCOUNT_ID
```

`login-chatgpt` prints a private `open_url` JSON event. Open that URL in your
system browser to **Continue with ChatGPT**, select the account/workspace and
grant plan usage. Keep this foreground CLI running while the browser returns to
its `127.0.0.1` random-port callback. Login expires after five minutes; Ctrl-C
cancels it and closes the listener. The final JSON object contains safe account
metadata. Do not redirect login output into logs, evidence or bug reports: the
URL includes private authorization state and installation identity, and returning
sign-in may include an ID-token hint. The URL is an interaction, not diagnostics.

`accounts` is the local account picker: masked account/workspace labels and
connection IDs identify each registration. Multiple accounts have separate
credentials. For reauthorization use `model login-chatgpt ACCOUNT_ID`; it reuses
that registration's issued client ID and validates the same identity before
replacement. Logout retains this non-secret mapping and history for later login.

`account-catalog` requests the current account's `/v1/models`; only server-returned
`visibility=list` slugs can be chosen. Unknown/hidden IDs are rejected before
inference. `account-add` uses conservative text/token defaults when the catalog
omits optional metadata. Existing Test/Select still require streaming, actual
Tool Call/Result continuation, cancellation and clipping before activation.
Requests use Pi's current `openai-responses` transport to the fixed public API,
with `store=false`, `stream=true` and namespace-wrapped function tools. Unsupported
sampling, reasoning and modalities stay disabled; `response.completed` is required.

`refresh` ensures the token is fresh; it makes no network call if comfortably
before expiry. Every account request also performs this check. Kernel locks
serialize rotation and logout across local Host processes. Refresh failure or an
interrupted rotation closes readiness, removes credentials and requires explicit
reauthorization; an unknown rotating call is never replayed. Startup reconciles
interrupted auth state after obtaining the same lock. Keep all processes for one
installation on a local filesystem supporting advisory locks.

| Persisted auth state | Legal next states | Runtime behavior |
| --- | --- | --- |
| `connected` | `refreshing`, `reauth_required`, `logged_out` (metadata-only self-update is allowed) | Fresh credentials admit requests |
| `refreshing` | `connected`, `reauth_required`, `logged_out` | One lock owner rotates; new callers wait |
| `reauth_required` | `connected`, `logged_out` | Explicit validated login is required |
| `logged_out` | `connected` | Only explicit same-registration login reopens admission |

Every state transition uses version CAS and an atomic Outbox event. Auth loss
increments a persistent account epoch; old generations remain fenced even after
the same account signs in again. A successful same-account refresh changes auth
version without changing that epoch. A stale token's 401 cannot revoke a newer
credential version. Failed SecretStore/metadata commits do not reopen admission.

Logout persists closed admission, aborts the connection's Bridge work, clears
local credentials and reaps the Bridge, while retaining snapshots/history/usage.
The output reports `remote_revocation_confirmed`; if false, disconnect the app in
ChatGPT Settings to confirm remote revocation. A SecretStore deletion failure is
an error, retried on startup or another logout. Other Host processes reject final
results by rechecking their captured epoch and account status inside the usage
write transaction, serialized with auth mutations. Completion reaches the sink
only after that transaction succeeds. Logout or reauthorization between the
observer Guard and persistence records `AUTH_REQUIRED` with no accepted result
or token counts. Same-identity refresh preserves the epoch. HTTP history/cache
deletion is not promised. Account requests disable provider cache retention.

`tests/system/verify_account.py` drives this path with an owned local OAuth/Responses
server and an explicit `-test-secret-store SOCKET` in-memory fixture. This flag
is never inferred from configuration, saved metadata or missing Keychain. It
creates no credential file. The account harness and Node/Go security cases are selected by the current Makefile and system runner. `evidence/account*.json` contains safe
observations, not tokens or authorization URLs.

`go test ./internal/agent/sqlite -run TestAccountFinalAcceptance -v` drives the
real Go → Node → Pi completion path, pauses before usage persistence, then mutates
auth through a second Host/DB handle. It covers logout, reauthorization, auth loss,
same-identity refresh and storage failure. Node contracts also capture the actual
Tool continuation wire and require the fixed `erii` namespace on replay.

`go test -race ./internal/agent/sqlite -run TestAccountCancellationAtFinalAcceptance -v`
checks Generate and Continue with Cancel, Dispose and caller-context cancellation
at observer, open-transaction and post-commit barriers. Registrations remain
active through persistence. A Host-only acceptance gate serializes acknowledged
cancellation with the actual usage commit; it does not lock an observer wait,
RPC or sink. SQLite writer admission precedes this gate. A pre-commit ack rejects
success, records `GENERATION_CANCELLED` with no accepted token counts and emits
no completed event. A post-commit cancellation preserves the committed result.
Auth transaction rejection keeps `AUTH_REQUIRED`. No database migration or RPC
version change is required; the gate is excluded from JSON and snapshots.
`TestCancellationDeadlineDuringUsageCommit` holds an admitted commit, verifies
Cancel/Dispose respect their caller deadline, then drives an independent catalog
RPC before releasing storage. Failed disposal reaps its peer as usual.

[ADR 001](../designs/model-connections.md#decision-001-adr-001-current-openai-account-authentication-and-deferred-device-flow) records approved Device
Authorization deferral. It remains a future requirement, not a passing check.
Real interactive login/Keychain/provider Live Shadow remains **inconclusive**
until explicitly performed; deterministic fixtures do not establish account
eligibility or real service availability.

<a id="provider-bridge-connect-a-local-model"></a>

### Connect a local model

Install Node 24.21.0 and run `npm ci --prefix provider-bridge --ignore-scripts`.
Add this explicit configuration, using the absolute repository path:

```yaml
version: 1
llm:
  provider_bridge_command: node
  provider_bridge_script: /absolute/path/to/erii/provider-bridge/bridge.mjs
  provider_bridge_version: pi-ai-1.3.0-v1
```

Use an isolated `ERII_DATA_ROOT` while evaluating the CLI. The Bridge is optional;
unconfigured startup still provides setup/diagnostics without a provider.

```sh
bin/erii -config local.yaml model discover http://127.0.0.1:8000/v1
bin/erii -config local.yaml model add http://127.0.0.1:8000/v1 MODEL_ID
bin/erii -config local.yaml model catalog PROFILE_ID
bin/erii -config local.yaml model test PROFILE_ID
bin/erii -config local.yaml model select PROFILE_ID 1 1
bin/erii -config local.yaml status
```

`add URL MODEL_ID [AUTH_ID]` creates an unverified candidate with conservative
8,192 context/1,024 output token limits. The existing `model add` JSON form allows
explicit token limits and other candidate metadata. If discovery is unavailable,
manually supply the ID to `add`; catalog support is never an activation condition.
Inspect profile/config versions with `model list` and `status` before selecting.
Every mandatory Probe behavior must pass. Optional compatibility, reasoning,
modalities, and sampling claims remain disabled until individually verified.
Responses API is rejected on the local path. Unsupported frozen settings fail
closed instead of being silently dropped. Clipping Probe uses an eight-byte Host
output limit while keeping the frozen model token limit, so it actually tests
clipping rather than depending on a model exceeding its requested token limit.

API keys are optional. On macOS with cgo, `model login` reads a key from stdin
and writes it through Security.framework Keychain APIs (no credential process
arguments). It returns an AuthConnection ID to use with `add`. On any platform,
`model login ENV_NAME` explicitly registers a SecretStore environment reference;
`ENV_NAME` must be present in each Host process using that connection. Pass the
returned AuthConnection ID as the last argument to `add URL MODEL_ID AUTH_ID`
or in the JSON metadata form. Missing credentials fail closed. Neither method
prints or stores the key in SQLite. Do not enter credentials in metadata fields.
The native backend is unavailable without macOS/cgo; it never falls back to a file.

Privacy: model requests, conversation content, and Tool schemas go only to the
configured endpoint. `localhost` resolves from the Go Host network namespace,
not the Computer Guest. This notice is also printed before CLI activation.
The Host retrieves only the referenced API key for each catalog/generation call
and sends it in a private, transient connection RPC envelope. The Node process
inherits PATH only, receives no domain DB paths or Tool/Computer handles, and
keeps no credential file. This is a logical process boundary, not an OS sandbox.
There is no provider fallback. All redirects are rejected, including same-origin
redirects, so Authorization cannot be forwarded. The SDK constructor requires a
nonempty key; a transport wrapper always deletes its placeholder Authorization
and sends a Bearer header only when the user supplied an actual key.

The transport retries connection failures or HTTP 429/502/503/504 at most twice,
with 50/100 ms delays, before a response stream is accepted. It does not retry
partial streams, malformed events, auth errors, or model incompatibility. Requests
use a 30-second absolute Bridge deadline across fetch, retries, and response-body
consumption, independently of the SDK header timeout. A deadline aborts HTTP and
returns PROVIDER_UNAVAILABLE, with no completed result or stream retry. The Host
Probe uses a separate 25-second deadline per capability check and independent
one-second cleanup contexts. Its fixed stage diagnostics distinguish a deadline
from expected cancellation. This leaves cleanup headroom before the unchanged
30-second generation bound. Direct Cancel/Dispose RPCs retain their five-second
bound. The error body is discarded,
except for an allowlisted machine code read from at most 8 KiB for HTTP 400/422
context-length classification. SDK console sinks are disabled because malformed
SSE parsing can otherwise print provider chunks. Raw response streams are limited
to 4 MiB before SDK parsing, and normalized Tool Calls retain the Host bounds
of 32 calls and 64 KiB of arguments. Pi argument parse errors and non-object
arguments fail with INVALID_PROVIDER_RESPONSE before any Tool Call is forwarded.
Pi raw parse diagnostics are never exposed. An explicit empty object remains valid.
Provider errors contain stable codes only; Host Probe errors add fixed stage
labels and timeout durations without provider text.

Readiness checks the saved proof, current credential availability, and Bridge RPC
health. It does not require catalog support or certify current endpoint health;
actual inference and explicit re-tests observe provider availability. Failed
re-tests revoke proof and close ingress. A crashed Bridge is restarted only for
the next independent operation. No generation is automatically replayed.

Schema v5 retains immutable endpoint locations separately from mutable profiles.
Resolution uses the frozen endpoint hash; editing a profile cannot redirect an
old snapshot. Migration backfills endpoints available in prior-schema profiles.
An endpoint lost through historical edits before v5 cannot be reconstructed and
fails closed. Snapshots and credentials retain their existing boundaries.
`provider_usage` stores the snapshot, generation/Run IDs, observed token counts,
Host elapsed milliseconds, and normalized failure. Missing token usage is NULL,
including cancellation/crash; zero is recorded only if supplied by the provider.
Local monetary cost is zero. The shared Run ledger atomically accepts generation results and usage under
ownership fencing; see [Main recovery](../designs/conversation.md#main-loop-ownership-and-persistence).

<a id="provider-bridge-reproduce-the-cli-path"></a>

### Reproduce the CLI path

Use an isolated absolute `ERII_DATA_ROOT`. Run `make build`, then provide metadata
through `model add` as documented in [local model setup](#provider-bridge-connect-a-local-model). Copy its `profile_id`.
Profiles are unverified candidates, and metadata cannot activate one.

```sh
bin/erii -test-bridge success model catalog
bin/erii -test-bridge success model test PROFILE_ID
bin/erii -test-bridge success model select PROFILE_ID 1 1
bin/erii -test-bridge success status
bin/erii status
```

`select` requires the expected profile version and Agent config version, visible
in `model list` and `status`. It increments both versions atomically, commits
ready/verified metadata, retains the stable AuthConnection reference, and creates
both activation Outbox events. Test alone stores Probe evidence and opens no
ingress. Concurrent selections have one config-CAS winner. Metadata edits or auth
version changes invalidate the prior Probe. A failed re-test revokes only the
tested version's existing proof, marks it invalid/unavailable/reauth_required,
and degrades an active config atomically with events. Cancellation by the Test
caller preserves prior state. Failure of the Probe's internal cancellation check
revokes prior proof even when the error includes `GENERATION_CANCELLED`; a
responsive replacement Bridge alone cannot restore readiness. A failed commit
changes neither the profile, Probe version, config nor events. Existing SecretStore registration
writes credentials first and removes a new orphan on failed metadata commit.
SecretStore/SQLite still have no distributed transaction: a hard crash between
those commits can orphan a credential; automatic orphan enumeration/reconciliation
is not implemented.

`model cancel` cancels setup without changing durable state. No model is chosen
automatically. Every process that uses a test profile must explicitly supply
`-test-bridge success` again. Without it, an existing active test profile reports
`degraded` with closed ingress. Incompatible Bridge versions, missing credentials,
unavailable/invalid active profiles, or stale Probe evidence also close ingress.
Diagnostics and metadata remain usable for repair. A compatible, valid active
profile reports `ready`; this readiness flag is a gate for later Agent admission,
and governs the existing Main/Worker execution path.

The Fake fixtures are named `success`, `incompatible`, `crash`, `partial-crash`,
`completed-crash`, `no-tools`, `bad-sequence`, `drift`, `malformed`, `auth-required`,
`raw-error`, `event-error` and `error:CODE` for the eight baseline provider codes.
Only explicit test injection starts them. Fixture names are not
ProviderKind values or saved deployment defaults. A failing Probe never selects
a different provider or retries a generation automatically.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/model-connections.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/model-connections.md)。
