# 文件产物与多模态输入：测试方案

[功能与验收目标](../features/artifacts-media.md) · [技术方案](../designs/artifacts-media.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-012](../features/artifacts-media.md#f-012) | 验证读回字节与哈希、作用域、保留期、并发保存、删除和重启恢复。 | 保护已接通的限定范围；Live/配置另行验证 |
| [F-013](../features/artifacts-media.md#f-013) | 通过真实消息入口上传图片，在最终供应商请求中验证图片存在，并验证无权限/不支持模型的行为。 | 实际浏览器/CLI → Artifact → Pi → HTTP，区分视觉与纯文本 |
| [F-014](../features/artifacts-media.md#f-014) | 逐格式验证可获取的内容、原始文件关联、失败反馈和实际任务输出；质量由独立评测衡量。 | 必须通过目标入口补齐闭环；不能只直接调用底层方法 |

## 层级与关键路径

文件存储断言真实字节/hash；图片通过 Web/CLI 用户入口到最终 HTTP 请求检查，不能只测直接构造 GenerationRequest。音视频/文档逐格式验证实际解析链，不把文件名或 fixture 元数据当内容理解。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 执行记录 |
| --- | --- | --- |
| [tests/system/verify_artifacts.py](../../tests/system/verify_artifacts.py) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 结果保存于对应 evidence，不在计划中推定 |
| [internal/llm/bridge/images_test.go](../../internal/llm/bridge/images_test.go) | 已有相关回归入口；具体断言以实现为准，不代表覆盖全部目标 | 结果保存于对应 evidence，不在计划中推定 |

### F-013 回归路径

- `internal/agent/messages_test.go`：真实 user、runtime、assistant、Tool 图片的选择边界及旧引用模式。
- `internal/agent/sqlite/images_test.go`：通过 Submit/Steering/Generation 接入实际 SQLite 和图片字节，验证显式 grant、未选择/未授权 ref 拒绝、过期 claim、纯文本回退、旧 policy 恢复的请求字节一致、已完成结果不再读取图片。
- `internal/llm/bridge/images_test.go`：真实 Pi HTTP 图像 envelope、不落盘像素、去重/图片数量上限、格式/损坏/字节及像素上限、极窄图片的实际 JPEG 尺寸。
- `internal/interaction/web/model_actions_test.go`：图片选项仅形成显式候选声明；既有模型 Probe 检查继续保护声明与观察证明双条件。
- `tests/system/verify_local_provider.py` 的用户图片场景启动真实事件 broker、Web、Actor 与 Node/Pi。Chromium 实际上传 PNG，HTTP peer 检查收到 JPEG 字节；纯文本模式无图片字节且生成上下文包含限制说明。CLI save/send-parts 后在 HTTP 响应前 SIGKILL，恢复保持 generation id、request hash 和图片内容 hash。现有 screenshot 场景继续证明工具图片选择。
- 浏览器证据含桌面与移动截图。所有图片为合成运输素材，HTTP peer 只验证协议与字节，不对 OCR、视觉推理或真实模型质量评分；真实账户视觉质量另行 Live/eval。

- **F-014 剩余覆盖**：按实际用户场景明确支持格式，补解析、转写或媒体工具及错误反馈；不以合成文件证明理解质量。

## 详细测试入口与证明边界

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="artifacts-verification"></a>

### Verification

For focused receipt recovery, add `--recovery-only` to the command below. It
launches approved Worker effects, kills after the effect or terminal unknown
commit, reconciles through the CLI, verifies bytes/hash and caller readback,
replays without a second effect, and removes all disposable state.

`python3 tests/system/verify_artifacts.py --binary bin/erii --evidence
evidence/artifacts.json` owns Launch/Doctor/Drive/Evidence/Cleanup, using the
explicit `artifacts` model and binary/large-output Tool Fakes. It drives actual
CLI save/open/delete, large input refs, Task output publication, eight concurrent
CLI writers, forged/traversal refs and six SIGKILL boundaries. The system runner selects it; repeat count is explicit. Go contract tests add cross-user/Agent/Task/Run checks,
child-shaped writers, symlinks/tampering, quota, expiry/reference/dry-run checks,
migration rollback, failed Tool commit and stale claim fencing. Race detection
is separate (`make test-race`).

GC-15's versioned fragment preserves source hashes and reopens `visa_check.md`
through Task → Root → Main final delivery. Synthetic PNGs carry fixture fields
in metadata; they are binary transport assets, not OCR evidence. Scripted text
is not semantically graded; this Artifact fragment keeps the full case
`not_applicable`. [M6 evidence](product.md) covers additional image/privacy
mechanics without establishing model vision quality. Original M1/M2 fixtures
and locks stay unchanged. [Code Mode replay](../designs/codemode.md) and [Computer transfer](../features/computer-use.md)
are implemented and verified separately from this Artifact fixture.

`manage` accepts only fields for its selected operation. `publish` selects the
current Run's final outputs; it does not send a message or upload a file. Main's
final AssistantMessage passes those semantic references to the existing Delivery
Composer and versioned UISpec (including Web's OpenIntelligentUI table). Workers
return only to their direct Parent. Neither tool accepts ownership or filesystem
paths. Search filters before counting accessible matches, so older matches are
not hidden behind 50 unrelated files.

The five old Direct Tool names are removed. See
[ERII-192 compatibility](../designs/tools-approvals.md#decision-005-005--consolidated-model-visible-tools) before upgrading
an installation with active Runs. The authenticated `artifact` CLI is unchanged.

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。

## 工程基础重构回归

F-012/F-039：流阻塞期间另一个 state owner 能扫描回收且不删除活跃 stage；进程被杀后锁自动释放，现有恢复场景回收 orphan；publish 整组预校验与提交时版本/授权重检；继续运行配额并发、Tool deadline、unknown 效应和最终 envelope 上限回归。入口：`internal/agent/sqlite/foundation_test.go`、`artifacts_test.go`、`artifact_recovery_test.go` 与既有 artifacts system gate。
