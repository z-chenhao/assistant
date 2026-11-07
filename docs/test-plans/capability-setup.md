# 能力连接与就绪引导：测试方案

[功能与验收目标](../features/capability-setup.md) · [技术方案](../designs/capability-setup.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 自动默认资源验收

- `internal/config/setup/defaults_test.go`：并发启动复用同一密钥；密钥缺失不替换；自动、关闭、重新启用跨重启保存；自定义错误密钥不能覆盖默认加密身份。
- `internal/interaction/web/capability_status_test.go`：自动与自定义均显示 Enabled／已启用，异常、准备和重启状态独立；`tests/system/channels_capabilities.mjs` 验证模式切换与保存。人工检查 Auto／自动选中态、短标签无截断及明暗主题桌面／窄屏截图。
- `internal/interaction/web/capability_forms_test.go`：四类自动能力的普通表单往返与关闭；原高级字段保留用例继续运行。
- `internal/memory/sqlite/local_test.go`：无 embedding 服务的中文加密记忆搜索、重启、scope、明文不落盘和遗忘。
- 原生 SecretStore 的实际解锁与 macOS 首次下载/启动需使用真实登录会话验收；Linux 的内存凭据 fixture 不能替代这项证据。


## 范围与证据

证明 F-030 的真实 Settings → 测试 → 保存 → 明确重启 → 新 Run 闭环。诊断与软件回归不认证外部模型语义质量、真实账户授权或 native macOS Guest 兼容。

## 需求场景矩阵

| 场景 | 可观察行为 | 验证入口 |
| --- | --- | --- |
| 五个主入口／六类资源 | 默认 Memory 已启用、检索收进高级设置；普通 Save 校验失败不落盘、修正后成功，关闭记忆同时关闭检索 | channels 的 capability-settings 场景 |
| 提交安全 | 未认证 401、伪造 Origin 403、未知详情 404、超限 413、未验证 Save 409 且文件不变 | 真实 HTTP + 浏览器 |
| 证明约束 | 精确候选、完整源文件版本、过期证明拒绝，探测中外部变更丢弃结果 | config/setup 组件测试 |
| 文件保护 | 0600、保留注释/其他 section、并发 Manager 至多一个成功、拒绝 symlink/删除源、无残留临时文件 | config/setup + discovery |
| 数据边界 | 拒绝直接密钥轮换、embedding 身份变更不等于索引迁移、Computer bootstrap 结构正确 | config/setup 组件测试 |
| 有界真实探测 | 固定 embedding 文本；实际沙箱 printf；MCP tools list 但无业务调用；实际 sidecar 握手 + sealed base 校验但无 VM | owned HTTP/stdio/Computer peers + 组件测试 |
| 生命周期 | 保存前后旧快照一致；重启后新 Run 包含新 provider；旧快照仍一致 | channels 场景读取实际 SQLite 状态 |
| 可访问操作 | 原生无 JavaScript 字段表单、320px 无横向溢出、浏览器无脚本错误 | channels browser 模块 |

## 维护入口

- `go test -race ./internal/config/... ./internal/interaction/web ./internal/computer/macosvm`：配置/文件并发、真实沙箱与协议适配器、共享 Computer 校验。
- `make test lint`：代码/组件、现有 Node/Web/测试基础设施、文档与静态检查。
- `make build` 后运行既有 `tests/system/runner.py --gate channels --binary bin/steward --evidence /absolute/path`；capability-settings 已并入该 gate，不增加 runner 或 CI gate。
- 跨模块回归按 [Testing](product.md) 执行 core，以及涉及能力/Computer/recall 的现有 extended gates。

具体结果保存在每次运行的 evidence，本文定义验收而非替代执行报告。owned peers 是明确的受控外部服务，不伪造产品内部 Run/验证通过状态。测试在可丢弃根目录创建配置和资源，结束后回收进程/监听器；从不访问个人数据目录。

## 平台与剩余证明范围

Linux 验证了内核沙箱与受控 Lume 协议、tiny sealed base 和真实 Node runtime。Computer 诊断不启动 Guest，因此不以这份证据声称验证了 macOS/arm64 的实际桌面操作；该边界由 [Computer 测试方案](computer-use.md) 保持。真实外部 MCP/embedding 服务可通过同一 owner 入口按实际凭据测试，未配置时不自动补齐服务或选择默认模型。

普通表单补充：`capability_forms_test.go` 与 `capability_server_test.go` 验证六类往返、未展示字段保留、别名回退、MCP 顺序／权限、明文凭据拒绝、版本冲突、精确候选证明与转义。维护中的 channels_capabilities 浏览器场景覆盖默认记忆、中文／移动端、Preferences 布局、自动保存的失败／成功／修改后重新校验／保存，MCP 原生增删行、无 JavaScript 字段表单。真实工作区／Computer 诊断若因本机环境不可用而失败，必须记录未完成范围并拒绝无效保存，不能用 UI 成功状态替代。

状态与保存回归：`TestDefaultMemoryHasOneEnabledEntryAndPreferenceLayout` 和 `TestPreferenceStyleSaveChecksBeforePersisting` 验证默认启用、一处 Memory 入口、联合关闭、密钥保留、无效连接拒绝与版本冲突。channels 的 `default_capability_settings` 使用显式隔离 SecretStore 启动真实应用，验证默认密钥跨进程重用并输出新设置页截图。


`TestCapabilityStatusDistinguishesConfigurationFromConnectionProof` 以表驱动验证关闭、自动／自定义启用、准备中、不支持、不可用、已保存待重启及未保存候选；各状态分别使用有／无连接检查证明和中英文渲染，防止再次把默认启用误写成未验证，或用启用文案掩盖异常。

维护中的 capability-setup 浏览器场景还验证六类真实字段的失败修复、自动保存、MCP 权限与原生行编辑，无脚本提交；逐页覆盖 1400／390／320px 和明暗主题，测量布尔行标签与开关同排、无横向溢出；断言不包含 YAML 编辑、导入、模板与配置文件路径。服务端回归验证 guided 忽略附带整份文档、字段草稿保留和版本冲突。配置和探测仍使用真实 Manager 及 owned peers。
