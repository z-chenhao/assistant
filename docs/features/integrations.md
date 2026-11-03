# MCP 与真实外部能力

将配置的外部工具纳入同一权限、审批和恢复体系。

[需求主索引](../feature-map.md) · [技术方案](../designs/integrations.md) · [测试方案](../test-plans/integrations.md)

## 功能说明

操作员可把允许的本地 stdio 工具加入 Agent 工作面。

用户能连接实际数据源与服务，支撑研究、日程、监控等具体工作。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-024"></a>

### F-024 · 受限本地 MCP 工具接入

**需求**：操作员可把允许的本地 stdio 工具加入 Agent 工作面。

**当前实现**：启动服务器、tools/list、tools/call、schema 指纹与工具允许列表已实现。

**边界与剩余工作**：当前子进程受限、无继承凭据/网络；这不是通用远程认证 MCP 客户端。

**验收目标**：真实协议验证发现/调用、漂移拒绝、取消回收与外部效果审批。

<a id="f-025"></a>

### F-025 · 可认证的真实服务接入

**需求**：用户能连接实际数据源与服务，支撑研究、日程、监控等具体工作。

**当前实现**：已有本地 MCP 与配置型 Guest 浏览器基础；GitHub 精确 PR 监听可从 SecretStore 解析 PAT，经 `/user` 验证身份后读取评审及当前提交的 CI。凭据失效、来源失效、限流与协议错误通过安全状态码诊断；替换同一 SecretStore 引用中的 PAT 后下轮重试，无需将密钥复制到状态库。

**边界与剩余工作**：此接入只读 GitHub.com 的一个明确 PR，不提供 Agent 服务工具、远程 OAuth MCP、自动签发/刷新 PAT 或授权写动作；统一连接与其他服务仍待实现。真实账户权限、PAT 到期恢复及实际平台结果需 Live 验证。

**验收目标**：在明确服务上完成真实读取/授权动作/失效恢复；无凭据泄漏，审批及 unknown 语义保持一致。

<a id="f-042"></a>

### F-042 · 可靠的环境事件监听

**依赖**：F-025 服务接入与 F-020 渠道。完成状态以主索引为准。

**需求**：用户授权监听实际工作来源后，外部评审、CI 等变化能可靠进入助手，断连恢复后仍能追溯接收与处理情况。

**当前实现**：显式配置一个 GitHub.com 仓库/PR 后，前台 Web/TUI/Automation 运行时持续读取 review comments、已提交 reviews 与当前 PR head 的已完成 check-runs。完整分页扫描后，将来源版本身份、观察内容、健康及补读成功时间原子保存到 `state.db`，通过 `github status/events` 查看；重复和乱序不重复保存同一来源版本，自身份的评论/评审不回流。

**边界与剩余工作**：未绑定的观察只供本机诊断；本机所有者可将当前精确 PR 显式关联到已有 Task，只有绑定后新观察才交 Main 作为不可信外部证据，再由 Main 判断是否在原目标内追加工作。事件不能创建新 Task 或扩大权限；移除/切换配置与明确撤权持久停止旧绑定，暂时轮询故障保留已接纳观察；GitHub 因果 Main/Task 结果复用 F-020 回原渠道/话题，显式通知偏好仍优先。补读重扫 GitHub 仍保留的当前资源；不保证已删除评论、扫描间消失的中间修改、旧 head 的 CI 或普通 issue comments/legacy commit statuses。每类至多扫描 20 页、每页 100 项；总规范化内容至多 8 MiB，超过界限显示 `HISTORY_LIMIT` 并保留原成功边界，不能称为完整审计流。

**验收目标**：重复接收不重复接单；断连、重启、乱序和来源权限撤销后按明确边界恢复或停止；监听中断不展示为正常。接收信息不授予任务执行或外部动作权限。

## 如何使用

当前在配置中指定本地 MCP 可执行文件、独立目录和允许的工具及权限。启动时纳入冻结注册表，Agent 通过同一工具发现和执行入口使用。

GitHub 监听使用以下显式配置；不要把 PAT 字面量写入 YAML。PAT 需要所选仓库的 Pull requests 和 Checks 读取权限，并能通过 `/user` 返回当前用户身份；SecretStore 支持操作系统凭据引用或显式环境引用。配置示例同 [config.example.yaml](../../config.example.yaml)。

```yaml
github:
  repository: owner/repository
  pull_number: 123
  credential_ref: env:STEWARD_GITHUB_TOKEN
  poll_seconds: 60
```

启动既有 `web serve`、TUI 或 `automation work` 的生产运行时即持续监听；不要求模型已就绪。`poll_seconds` 默认 60 秒，允许 5–3600 秒。配置变更在重启后生效；移除配置即不启动监听，当前命令不再展示旧范围为活动来源。仅配置 SecretStore 引用，不会自动选择模型。

```sh
steward -config /absolute/config.yaml github status
steward -config /absolute/config.yaml github poll
steward -config /absolute/config.yaml github events
steward -config /absolute/config.yaml github events 101
steward -config /absolute/config.yaml github stop
steward -config /absolute/config.yaml github resume
```

`poll` 执行一次真实只读扫描并返回状态；失败返回非零，同时保留安全诊断。`events` 按 sequence 倒序最多 100 项，下一页传本页最小 sequence；正文最长 16 KiB，截断时明确提示沿来源 URL 读取全文。`status` 区分 `never_checked/checking/healthy/interrupted/stale/stopped/privacy_stopped/not_configured`；超过两倍轮询间隔的旧成功/进行中扫描显示 `stale`，不冒充仍正常监听。CLI 控制可在模型或执行依赖不可用时使用。

`stop` 持久停止并 fencing 在途扫描，普通 `poll` 和重启不能恢复。Forget 同样停止原授权 epoch 的接收，并隐藏旧 epoch 观察；明确 `resume` 才更新接收意图。历史相同 event_key 不会因 resume 重新入库或暴露，只有此后尚未保存的来源版本进入当前观察。它不恢复 Task 或外部副作用。仓库名称对应的不可变 repository ID 改变时 fail closed，不把新仓库当原来源。

### 将 PR 后续反馈关联到已有任务

先配置/成功 `github poll`，确认确切仓库及 PR；再由本机所有者选择已存在、当前隐私作用域内的 Task。`task_epoch` 是所选 Task 当前执行代次，`version` 是 `github binding` 返回的关联版本（首次为 0）。关联只允许 active 或自然完成的 once Task；暂停、取消和用户显式 complete 的当前代次不能绑定。

```sh
steward -config /absolute/config.yaml github binding
steward -config /absolute/config.yaml github bind <<'JSON'
{"task_ref":"task:opaque-ref","task_epoch":1,"version":0}
JSON
```

绑定不回放已经收下的历史观察。上述旧格式绑定明确启用所有新观察。可先按一种事件类型观察匹配结果，再显式启用规则内的自动接续，见 [F-045 规则用法](automation.md#f-045)。接纳的新版本经现有单 Main/Actor 读取 JSON 证据框，判断是否需要 `task_manage append` 继续同一 Task。同一观察只接受一次不同的追加动作；相同来源与相同参数重复请求复用原回执。歧义、超范围、未知副作用或不需继续时，Main 应解释/询问用户，不建立新工作或猜测外部授权。source Main 可读已有授权数据和保存本地产物，不能直接发外部动作、创建 Automation 或写长期 Memory；真正工作交既有 Task Worker 和审批管线。

```sh
steward -config /absolute/config.yaml github unbind <<'JSON'
{"version":2}
JSON
```

使用实际 `github binding` 版本，不固定复用示例值。unbind、监听 stop、Task pause/cancel、用户显式 complete、Forget 和来源撤权会使待处理/正在推理的旧因果失效；恢复监听或普通重启不重新授权已停止的绑定。新用户输入优先建立自身的 Main，保留不可变接收回执，不被旧观察吞并。once Root 自然结束并非人工停止，原绑定可接续；人工 complete 则需要用户明确 append 进入新代次后再重新绑定。未知效果不会因为收到评审而重试。

远程 OAuth MCP、邮件/日历等不能当成现成产品开关。真实账户权限及 GitHub 保留历史的补读范围仍需独立验证。

## 详细操作参考

- [MCP、Skills 与工作区约定](../designs/integrations.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/integrations.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/integrations.md)。
