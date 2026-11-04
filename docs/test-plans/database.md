# Developer Database 数据检查：测试方案

[功能与验收目标](../features/database.md) · [技术方案](../designs/database.md) · [主索引](../feature-map.md) · [统一测试入口](product.md)

## 范围与证明边界

F-033 验证本机所有者从真实 Web 入口浏览元数据、定位关联对象和检查存储状态。此功能不触发模型推理，不以回归数据证明模型质量，也不开放内容解密或数据库修复。

## 需求场景矩阵

| 场景 | 必须证明 |
| --- | --- |
| 四类投影 | 执行、消息、Memory、Artifact 均有非测试入口；返回允许的元数据 |
| 分页/精确查询 | 默认与上限有效；相同时间稳定继续；无重复或遗漏；游标不能跨类型/Agent；确切引用只返回一个对象 |
| 关联 | Main 输入/回复、Task/父/根执行、附件、Memory 来源/替代、文件持有者均沿持久关系定位可见目标 |
| 归属 | 外用户、外 Agent、未知和缺失对象不能被读取或成为可用链接；已知外引用与未知引用同样拒绝 |
| 内容与敏感字段 | 消息正文、Memory 新旧明文/密文、存储 key/路径、凭据和 checkpoint 均不返回；存储值和链接转义 |
| 原生隐私边界 | 有加密 Memory、没有解密密钥或 embedding 时仍可检查元数据，不解密旧值 |
| 只读与恢复 | SQLite 连接本身拒绝写入；页面/API 无写入口；完整浏览前后领域记录一致；重启后结果仍可读 |
| UI | 中英文、确切引用查找、分页和关联跳转；390px 页面不横向扩张，表格可局部滚动；无 JavaScript 可操作 |
| 故障 | 认证/绑定失效、重复/未知参数、坏游标、取消、不可读数据库和非法类型得到有界且安全的错误 |

## 已有入口

- `go test ./internal/database/... ./internal/interaction/web ./internal/app`：真实 SQLite 只读池及投影组件、作用域、分页、关联、Web 认证与安全错误。
- `go test -race ./internal/database/... ./internal/interaction/web`：相关 Go 并发边界。
- `python3 tests/system/verify_channels.py --binary bin/steward --database-only --evidence evidence/database/report.json`：独立聚焦验收；也纳入既有 channels 核心门禁。
- `make test lint` 与[系统 runner](../../tests/system/runner.py)：原有行为、模板、生成资产、类型、文档和完整相关系统检查。

系统场景通过原有 CLI/Main/Bridge 接纳消息、创建 Task/Child、保存及附加文件，并由真实记忆工具和 Curator 形成加密 Memory。它为多记录分页工作负载选择显式 32K 测试模型窗口，独立的小窗口裁决保持原入口。随后移除记忆密钥和配置，经真实 HTTP 遍历元数据、核对确切结果和关系，再驱动 Chromium 完成分页、查找、执行检查、中文、移动与无脚本操作。领域记录来自这些入口，不通过 SQL 注入接受结果。

浏览前后独立读取三库的相关领域表并比较摘要，确保没有新执行、输入、审批、调用或内容修改；重启后再次读取加密 Memory 元数据。单元测试另构造外作用域、旧明文、删除和替代记录，验证投影不会泄露原值。测试使用私有临时数据根，关闭全部 SQLite 连接并回收浏览器/服务进程。

## 完成判定

功能、架构、使用方式、Feature Map 与验收入口须一致；执行证据保留在对应 PR、CI 或忽略的 evidence/。测试存在不等于某个版本已通过。真实大库性能、跨库全局一致视图和其他安装环境不由小型回归数据证明。
