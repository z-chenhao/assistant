# 能力连接与就绪引导：测试方案

[功能与验收目标](../features/capability-setup.md) · [技术方案](../designs/capability-setup.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证据

证明 F-030 的真实 Settings → 测试 → 保存 → 明确重启 → 新 Run 闭环。诊断与软件回归不认证外部模型语义质量、真实账户授权或 native macOS Guest 兼容。

## 需求场景矩阵

| 场景 | 可观察行为 | 验证入口 |
| --- | --- | --- |
| 六类配置 | 列表/详情、真实未配置状态、失败后修正成功、候选刷新仍可保存 | channels 的 capability-settings 场景 |
| 提交安全 | 未认证 401、伪造 Origin 403、未知详情 404、超限 413、未验证 Save 409 且文件不变 | 真实 HTTP + 浏览器 |
| 证明约束 | 精确候选、完整源文件版本、过期证明拒绝，探测中外部变更丢弃结果 | config/setup 组件测试 |
| 文件保护 | 0600、保留注释/其他 section、并发 Manager 至多一个成功、拒绝 symlink/删除源、无残留临时文件 | config/setup + discovery |
| 数据边界 | 拒绝直接密钥轮换、embedding 身份变更不等于索引迁移、Computer bootstrap 结构正确 | config/setup 组件测试 |
| 有界真实探测 | 固定 embedding 文本；实际沙箱 printf；MCP tools list 但无业务调用；实际 sidecar 握手 + sealed base 校验但无 VM | owned HTTP/stdio/Computer peers + 组件测试 |
| 生命周期 | 保存前后旧快照一致；重启后新 Run 包含新 provider；旧快照仍一致 | channels 场景读取实际 SQLite 状态 |
| 可访问操作 | 原生无 JavaScript Test 表单、320px 无横向溢出、浏览器无脚本错误 | channels browser 模块 |

## 维护入口

- `go test -race ./internal/config/... ./internal/interaction/web ./internal/computer/macosvm`：配置/文件并发、真实沙箱与协议适配器、共享 Computer 校验。
- `make test lint`：代码/组件、现有 Node/Web/测试基础设施、文档与静态检查。
- `make build` 后运行既有 `tests/system/runner.py --gate channels --binary bin/steward --evidence /absolute/path`；capability-settings 已并入该 gate，不增加 runner 或 CI gate。
- 跨模块回归按 [Testing](product.md) 执行 core，以及涉及能力/Computer/recall 的现有 extended gates。

具体结果保存在每次运行的 evidence，本文定义验收而非替代执行报告。owned peers 是明确的受控外部服务，不伪造产品内部 Run/验证通过状态。测试在可丢弃根目录创建配置和资源，结束后回收进程/监听器；从不访问个人数据目录。

## 平台与剩余证明范围

Linux 验证了内核沙箱与受控 Lume 协议、tiny sealed base 和真实 Node runtime。Computer 诊断不启动 Guest，因此不以这份证据声称验证了 macOS/arm64 的实际桌面操作；该边界由 [Computer 测试方案](computer-use.md) 保持。真实外部 MCP/embedding 服务可通过同一 owner 入口按实际凭据测试，未配置时不自动补齐服务或选择默认模型。

普通表单补充：`capability_forms_test.go` 与 `capability_server_test.go` 验证六类往返、未展示字段保留、别名回退、MCP 顺序／权限、明文凭据拒绝、版本冲突、精确候选证明与转义。维护中的 channels_capabilities 浏览器场景覆盖主要记忆表单的失败／成功／修改后重测／保存，MCP 原生增删行、无 JavaScript 普通及高级表单。真实工作区／Computer 诊断若因本机环境不可用而失败，必须记录未完成范围并保持 Save 禁用，不能用 UI 成功状态替代。
