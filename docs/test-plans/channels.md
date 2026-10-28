# 终端、Web 与飞书：测试方案

[功能与验收目标](../features/channels.md) · [技术方案](../designs/channels.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-017](../features/channels.md#f-017) | 验证真实终端首条输入、后台工作时追加、退出和重启。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-018](../features/channels.md#f-018) | 真实浏览器验证设置、发消息、附件、审批和已有对话展示。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-019](../features/channels.md#f-019) | 长任务与输入草稿并行时，回复/进度更新可见，焦点、草稿及附件不被覆盖。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |
| [F-020](../features/channels.md#f-020) | 验证真实绑定、token 生命周期、通知偏好、撤销、重启与不确定投递；未知结果不盲目重发。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

实际浏览器/PTY/签名回调分别驱动；输入草稿、附件与焦点必须在后台更新时保留。飞书失效 token、撤销绑定和发送结果未知单独验证；本地回执不冒充远端核实。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [tests/system/verify_channels.py](../../tests/system/verify_channels.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_tui.py](../../tests/system/verify_tui.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |
| [tests/system/verify_delivery.py](../../tests/system/verify_delivery.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 本次未执行 |

- **F-019 剩余覆盖**：接通增量状态/消息读取与前端更新；具体传输方式在实施时确定，保留权威提交边界。
- **F-020 剩余覆盖**：补凭据刷新/失效诊断体验；不确定远端投递目前只能依赖已保存本地回执，可信远端核实能力仍待定义。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="channels-verification-and-evidence"></a>

### Verification and evidence

```sh
make test
make test-race
make lint
make test-full
python3 tests/system/verify_channels.py --binary bin/erii --evidence evidence/channels.json
python3 tests/system/verify_channels.py --binary bin/erii --evidence evidence/channel-leases.json --delivery-lease-only
python3 tests/system/verify_channels.py --binary bin/erii --evidence evidence/components.json --ui-components-only
```

`verify_channels.py` owns Launch (private root, local platform Fake and real Web
process), Doctor (auth and real browser setup), Drive (HTTP/UI/CLI boundaries),
Evidence (SQLite/Conversation/API/platform ledgers and browser screenshots), and
Cleanup (joined children, stopped Fake server and removed disposable state).
CI selection follows the current engineering design; repetitions require an explicit runner option.
The pinned Browser dependency is the repository's existing `playwright-core`.
Screenshots and DOM evidence are under `evidence/channels-ui/`.

The harness covers concurrent callback/source retries, identity/auth/recipient
rejection, input commit and delivery SIGKILL windows, approval Steering plus
restart and callback races, inbound/outbound attachment bytes, unknown-send
non-retry, and GC-08/09/12/14 channel/state fragments. It also covers
slow sends/uploads, cumulative multipart duration beyond the default lease,
competing live/recovery consumers, lost ownership cancellation, late settlement
and renewed ingress after restart. The existing platform Fake owns deterministic
response delays and an independent send-attempt ledger. Unit/contract tests cover
migration rollback, CAS/revocation, route/projection freezing, encryption,
credential redirect refusal, session expiry, XSS and unknown/invalid-component
fallback. The channel harness also drives Web account login through the real Manager/Bridge
against the existing local OAuth Fake, actual Probe/Select, hidden-model rejection,
restart and logout, then scans persisted files for credential values. Existing
suites retain replay, cancellation, account lifecycle and
Artifact access coverage.

Scripted-policy transport fragments are not full product scores. Complete 20-case
semantic grading and live Feishu/ChatGPT authorization remain release evidence;
this slice does not claim M7 or the complete first release is finished. No live
external account or write is required by CI.

Feishu transport shapes follow the official [message API](https://open.feishu.cn/document/server-docs/im-v1/message/create),
[resource API](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1),
and [callback encryption guide](https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscription-configure-/encrypt-key-encryption-configuration-case).

<a id="delivery-inspectable-verification"></a>

### Inspectable verification

The system runner selects `tests/system/verify_delivery.py` for delivery regression. It reuses
`MainHarness` and `Terminal` for Launch, Doctor, Drive, Evidence and Cleanup.
Each run captures 20 actual SIGKILL/restart cases: Main and Root final replies at
Conversation append, delivery commit, checkpoint, send and acknowledgement
boundaries. Root acceptance is delivered before faulting its final Main follow-up.
Database snapshots prove one semantic message/receipt, correct destination,
retired outbox, reused generations, and read-only duplicate scans. PTY transcripts
prove final replies and frozen-hash approvals reach the real terminal surface.

The explicit `-test-delivery` Fake requires `-test-bridge` and supports
`retry-once`, `unavailable`, `ambiguous`, and `unknown`. It wraps the same durable
terminal acceptance boundary. Recovery evidence covers bounded retries, query-only
unknown, accepted-send reconciliation, and duplicate approval callbacks without
unnecessary Main inference. Go tests additionally cover concurrent consumers,
claim fencing, identity drift, atomic event failures, legal/illegal transitions,
and populated v11 migration. These are M3 conformance fragments, not full product
Golden Case scores or a claim that M5 Durable Replay is complete.

<a id="web-components-build-and-verify"></a>

### Build and verify

```sh
npm ci --prefix web --ignore-scripts
npm run build --prefix web
make build
npm run check --prefix web
npm run lint --prefix web
python3 tests/system/verify_channels.py --binary bin/erii --evidence evidence/components.json --ui-components-only
```

Commit component source, lockfile and generated assets together. `make test`
rejects stale embedded assets; CI installs the same pinned dependencies. Go-only
builds consume those checked-in assets and need no frontend server or network.
The maintained channel browser action proves actual React composition, native
table cells and fallback, frozen approval fields, artwork loading, responsive
layout, CSP-compatible execution and local-only frontend requests. Its complete
action also drives model setup, file upload and actual message submission.
It deliberately holds the local JavaScript bundle while entering a draft and
selecting an attachment through the usable server form, then releases it and
checks text, file bytes, focus, selection and source identity before submitting.
The composer snapshots and synchronously mounts in one browser task so early
input cannot be lost during progressive enhancement.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。
