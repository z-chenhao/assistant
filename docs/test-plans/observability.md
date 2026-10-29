# Developer Observability 执行检查器：测试方案

[功能与验收目标](../features/observability.md) · [技术方案](../designs/observability.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

本方案回答如何证明需求完成，不是测试执行报告。已有入口只表示测试存在，不能推导本次已通过或真实模型效果已达标。

以功能文档中的需求为验收源；配置依赖、待实现分支、平台限制和未验证项单独报告。具体命令及 core/extended 编排以 [Testing](product.md) 和 [runner](../../tests/system/runner.py) 为准。

## 需求场景矩阵

| 需求 | 需要验证的可观察行为 | 当前重点 |
| --- | --- | --- |
| [F-031](../features/observability.md#f-031) | 真实工作能从入口定位到 Main、Task、Root 和 Child；Root 不被误标为 Main 的 Child。 | 已有真实 Actor/SQLite 与浏览器入口；逐轮内容不是本行完成范围 |
| [F-034](../features/observability.md#f-034) | UI 与被冻结的 Host 请求逐项一致，工具调用/结果对应正确，来源和省略可追溯，长文本可完整查看。 | 已有冻结记录/API 比对、真实压缩恢复与浏览器入口；无发送边界观测或模型质量结论 |
| [F-035](../features/observability.md#f-035) | HTTP peer 接收的请求与观测记录对应，差异明确标记为脱敏或媒体引用；请求头凭据不可见。 | 已有目标 HTTP/API 与浏览器入口，限制和执行结果分开记录 |
| [F-036](../features/observability.md#f-036) | 受控中断恢复后，UI 不把复用结果显示成新外部调用；未知结果、缺失 usage 和未 dispatch 等待如实展示。 | 已有目标 HTTP/API 与浏览器入口，限制和执行结果分开记录 |

## 层级与关键路径

F-031 复用真实 Actor/store 和现有 Web。测试用显式 scripted provider 决定调用，ToolInvocation、Task 和递归 Child 由真实执行链产生；页面读取真实保存的数据，不为 Inspector 新建执行引擎。F-034 沿用同一真实执行与冻结存储；F-035/F-036 通过生产 Pi/HTTP peer、既有故障恢复入口及新的 Trace 查询覆盖发送边界与时间线。

- 单元/组件：规则、实际 SQLite 和协议边界在所属模块验证。
- 系统：用真实 CLI/Web/Actor/Bridge 链路证明目标行为；允许受控外部 peer，不能伪造产品内部接受结果。
- Live/质量：涉及真实账户、Guest 平台或语义质量时单独记录环境与证据；不适用项说明理由，不用软件回归代替。

## 首版具体场景

下表维护整组功能的验收用例；F-031 的已有入口见下节，F-034 的具体证据也列在下节；F-035/F-036 的生产 HTTP peer、恢复、只读与浏览器证据也列在下节。

| 场景 | 关联需求 | 输入 / 操作 | 预期断言 | 层级 |
| --- | --- | --- | --- | --- |
| O-01 执行导航 | F-031 | 创建普通 Main 与包含 Root/递归 Child 的 Task，按时间和状态查找 | 列表可定位记录，关系与实际数据库一致；Task 创建关联与 Parent/Child 关系区分 | SQLite 集成 + 浏览器 |
| O-02 逐轮消息 | F-034 | 驱动两次以上模型请求，包含普通文本、工具调用和工具结果 | 消息顺序、角色、完整正文、Tool Schema、参数及结果与冻结记录一致；不存在的角色不伪造展示 | Bridge/store 集成 + 浏览器 |
| O-03 来源与压缩 | F-034 | 加入 Skill、召回、追加指令并触发上下文压缩 | 可追溯来源和省略记录；当前没有细粒度来源的数据明确显示未知，不以猜测补齐 | SQLite 集成 + 浏览器 |
| O-04 相邻轮次 Diff | F-034 | 在追加和工具续轮前后查看 Diff | 明确标识新增、移除与变化内容，原始快照可单独查看，查询不改写历史 | 组件 + 浏览器 |
| O-05 最终供应商请求 | F-035 | 受控 HTTP peer 接收经真实 Bridge/SDK 转换的请求 | system 合并、工具命名空间等转换能在最终请求视图核对；脱敏和媒体替换项有明确标记 | 真实 Bridge + HTTP peer |
| O-06 图片与凭据 | F-035 | 使用合成图片与仅用于测试的凭据发送一次请求 | 观测无 Authorization/Token/图片 Base64；媒体引用、尺寸和哈希可核对，不能伪称脱敏记录为逐字原始包 | 发送边界集成 |
| O-07 等待与恢复 | F-036 | 先等待审批或 Child，再在持久化边界中断/重启 | 等待不显示为已 dispatch；恢复复用的结果与新发送尝试区分，效果次数和回执不变 | 故障/恢复系统测试 |
| O-08 失败与不确定 | F-036 | 制造模型失败、工具失败、未知外部效果和缺失 usage | 如实显示失败/unknown/缺失值；不补零、不显示虚构成功，不由浏览操作触发重试 | 协议/SQLite 集成 + 浏览器 |
| O-09 有界查询 | F-031 / F-034 | 加入较多记录和长消息，分页、切换、展开完整内容 | 列表有界、详情可继续读取；截断有提示，完整正文有可达入口；不一次加载全部数据库 | 查询集成 + 浏览器 |
| O-10 只读与访问边界 | 全部 | 未认证访问、重复刷新、含 HTML 的消息与敏感上下文查看 | 未授权拒绝；消息安全转义；受控访问；查询前后领域记录、审批和外部效果不变 | 认证/查询集成 + 浏览器 |

真实线上模型不是这些机制检查的必需条件；产品实例中的敏感上下文不能复制进测试数据。未来若增加外部导出、重放按钮或评测入口，需另行定义其访问和副作用边界。

## 已有检查与缺失覆盖

| 入口 | 已有证据用途 | 本次执行状态 |
| --- | --- | --- |
| [internal/agent/sqlite/inspection_test.go](../../internal/agent/sqlite/inspection_test.go) | 真实 Main/Task/Root/递归 Child、原始提交因果、选择与分页、相同时间游标、角色/状态/UTC 边界、Agent 隔离、只读并发、schema 29→30 回滚与冻结数据保持 | 执行结果在对应日志维护 |
| [internal/observability/sqlite/inspection_test.go](../../internal/observability/sqlite/inspection_test.go) | 数据库层拒绝检查器写入，缺失状态文件不能由读操作创建；读取失败不成为 Runtime 启动依赖 | 执行结果在对应日志维护 |
| [internal/interaction/web/inspection_test.go](../../internal/interaction/web/inspection_test.go) | 认证、无执行 runtime 依赖、参数校验、标题转义、规范页面/旧链接跳转、无 POST 动作、脱敏失败 | 执行结果在对应日志维护 |
| [verify_channels.py](../../tests/system/verify_channels.py) 的 inspection_relationships / [channels_browser.mjs](../../tests/system/channels_browser.mjs) | CLI 设置 → 实际 Web Main 创建 Task 与递归 Child → API 与数据库关系比对 → 浏览器导航/角色状态时间筛选/桌面移动/无 JS → 逐轮内容与实际冻结 JSON 比对、Diff 和长 Unicode 正文续读 → 原始领域计数和冻结哈希不变 → SIGKILL 重启不重跑 | 随既有 channels gate 运行，不新增脚本或 gate |
| [context_inspection_test.go](../../internal/agent/sqlite/context_inspection_test.go) / [context_test.go](../../internal/agent/sqlite/context_test.go) | 真实 Main/Root/递归 Child、语义摘要恢复各轮的完整 messages/tools/response 与冻结记录逐项一致；原请求/效果与归档隐私不变 | 执行结果在对应日志维护 |
| [observability/sqlite/context_test.go](../../internal/observability/sqlite/context_test.go) | 长中文/emoji/NUL、8192 字节续读、40 项分页、来源隔离、重复键 Diff、增加/移除/改变/移动、未知响应与压缩/召回原记录、独立并发读取 | 执行结果在对应日志维护 |
| [web/context_test.go](../../internal/interaction/web/context_test.go) | 同一授权边界、参数/偏移校验、完整模板含 step 0 的前后链接、安全转义、无 POST、no-store 与 CSP | 执行结果在对应日志维护 |
| [traces_test.go](../../internal/agent/sqlite/traces_test.go) | 真实 Main/Root/递归 Child 的统一查询、输入输出、角色与 Agent/trace 隔离、缺失网络证据不猜测、同时间 100 节点分页及跨 trace 游标拒绝 | 执行结果在对应日志维护 |
| [web/traces_test.go](../../internal/interaction/web/traces_test.go) | 树/时间线与详情模板、授权/绑定、参数验证、转义、真实 parent 分支、页外 parent 与几何上限 | 执行结果在对应日志维护 |
| [bridge/observations_test.go](../../internal/llm/bridge/observations_test.go) / [images_test.go](../../internal/llm/bridge/images_test.go) | 可选诊断不改变 generation.event 序号；跨 registration 丢弃、上限、凭据、异常 sink；实际 Pi 收到图片而观测仅含安全引用/hash | 执行结果在对应日志维护 |
| [Node capture tests](../../provider-bridge/test/observability.test.mjs) | Node 侧 opt-in、凭据/像素替换、尺寸/hash、超限标记与 callback 故障隔离 | 执行结果在对应日志维护 |
| [verify_channels.py](../../tests/system/verify_channels.py) 的 native_trace_inspection | 生产 Bridge + 独立 HTTP peer；实际重试、最终 JSON 比对、usage/cost 缺失、完成后重启不重发；桌面/移动/无 JS、详情滚动/history、只读账本 | 随现有 channels gate 运行，截图为受控 peer 下的真实产品页面 |

- **F-031 证明范围**：O-01 导航、O-09 的关系元数据分页、O-10 的认证/只读/转义已有入口。缺少来源记录时明确未知；标题裁切、跨页状态变化和树片段有说明。截图不证明模型质量。
- **F-034 证明范围**：O-02/03/04/09/10 由实际冻结记录比对、语义压缩/恢复检查、SQL 边界测试和真实浏览器共同覆盖。完整内容是保存的 Host 请求和已接纳结果；旧来源缺失明确未知，压缩归档仅显示引用，不授予新 Artifact 访问。
- **F-035 证明范围**：生产 Go/Node/Pi 请求与独立 HTTP peer 收到的 JSON 比对；503 重试与 attempt 对应。实际图片发送测试核对 Artifact/预览 hash 证据且无 Base64；Node 与 Host 验证凭据移除、限额与诊断故障隔离。Capture 本身 best-effort，不能用空记录证明没有网络调用。
- **F-036 证明范围**：统一树/时间线、当前状态、唯一 generation 的已有 token/cost；同一接纳结果在 checkpoint 前 SIGKILL 后只调用一次 provider，并显示 reuse 事件。旧细粒度时间缺失保持未知，tool unknown 不触发重试；等待和未知效果仍由既有 tools/children/recovery gates 保护。

## 执行与完成判定

使用隔离配置与可丢弃数据，保留产品版本、实际命令、环境、结果和清理情况。区分通过、失败、跳过、未执行以及未知外部结果。

需求完成需要对应入口可用、正常与相关失败/恢复场景有证据、使用说明与技术方案一致。报告沿用现有 evidence/流程；不为本方案建立另一套测试平台或复制历史评测资产。

HTTP 诊断写入以真实 SQLite 锁验证：60ms 的短业务写锁释放后保存记录；持续锁定时在预算内放弃，调用方不等待诊断锁。`go test -race ./internal/observability/sqlite -run TestRecorder` 覆盖两条边界。原 native Pi/HTTP 回归保留完整 attempts/headers 数量断言，失败输出已接收请求数、诊断阶段和 generation 状态计数。

## 页面重构验收

真实 channels 浏览器 action 从列表进入独立 Trace 详情，验证树/时间线对齐导航、节点类型 Tab、折叠 metadata、完整 Context/JSON/来源与相邻 Compare 前后内容，返回列表和旧 deep link 保留参数；长 UTF-8 正文、桌面/移动端、无 JavaScript 与原 API 的读取边界继续覆盖。`TestTraceDetailDoesNotDependOnGlobalList` 明确让全局列表失败而详情成功，防止拆页后仍耦合列表查询。沿用同一 SQLite/HTTP peer 证据，不引入第二套执行或评测系统。
