# 全仓工程审查与重构记录

[工程方案](../designs/engineering.md) · [测试总方案](../test-plans/product.md)

审查基线：`ffab679`，工作区初始干净；日期：2026-11-03。覆盖 Go 核心与适配器、Web、Provider Bridge、CodeMode、系统测试、构建配置及 Agent 指令。检查结合调用关系、116 条可用 Git 提交、实际测试与覆盖率；不把静态未引用直接判定为可删除。

## 结论与证据

| 范围 | 证据与判断 |
| --- | --- |
| 架构与模块边界 | 66 个 Go 包、247 条仓库内依赖；组合根 `internal/app` 依赖 44 包，`internal/agent/sqlite` 依赖 31 包。`c52d7d5` / `3e41a2a` 已拆出领域适配器；Run/Tool/Task 同一事务及 fencing 是有意保留的一致性边界，不按包大小拆事务。 |
| 循环依赖 | `go list ./...` 成功；34 个 JS/TS 模块的本地静态 import 未发现环。Python AST（包括函数内 import）在基线发现 `verify_channels → verify_local_provider → verify_channels`，原来依赖延迟导入避免初始化错误；提取共用 peer 后已无该环。 |
| 重复与冗余 | `internal/interaction/web/server.go` 每次请求建立同一资源路由表，且资源处理混入认证入口；`internal/tool/mcp/client.go:Definitions` 内联发现、冻结校验、RPC 调用与返回解析。适合局部职责提取，无需通用框架。 |
| 死代码与动态引用 | `templates/*.html` 是编译期嵌入的完整模板集合。`diffLink` 的模板调用在 `4595a69` 移除后只剩 FuncMap 注册与测试；已移除失效注册/helper，保留通过真实 URL 测试分页兼容性。其余模板 helper、prompt、字体资源、平台实现不能根据 Go 文本引用数删除。 |
| 历史配置与实现 | `docs/designs/engineering.md` 的原目录示例包含 `secret/keyring`、`llm/providerbridge`、`computer/lume` 等不存在的包，已与当前结构对齐。32 份迁移、旧快照、legacy context、旧书签重定向均有恢复或兼容路径，保留。 |
| 错误与恢复 | Staticcheck 指出 `internal/agent/sqlite/scheduler_test.go` 的第二次 `Next` 错误被块内变量遮蔽；`internal/computer/macosvm/provider.go` 有两个立即被覆盖的 IP 赋值，但对应状态读取承担不确定副作用保护，必须保留调用。 |
| 过时 API | 离线 SSH fixture 手动构造 ECDSA 坐标触发 4 个弃用诊断；改用标准私钥解析，保持同一确定性密钥和 SSH 身份竞争测试。不是生产凭据或配置更改。 |
| 测试质量 | Go 全包测试通过。跨包合并语句覆盖率为 **69.3%**（仅本次 Go 测试进程）；`agent/sqlite` 73.0%、Web 67.6%、App 26.7%。低覆盖的组合根另有 CLI/E2E 检查，子进程与系统测试不计入此值；`[no test files]` 不等于未被集成测试覆盖。 |
| 测试环境隔离 | `tests/harness/test_setup.py:SetupTests.setUp` 继承宿主 `NPM_CONFIG_AUDIT=false`，真实本地 registry 测试收到 1 次请求而非预期 5 次。仅测试 fixture 显式启用自身审计请求，保留原断言和生产 setup 配置语义。 |
| 异步测试边界 | `internal/agent/sqlite/responsive_test.go:279` 构造的恢复点遗漏输入消费，且在允许先响应输入的 `ModelPhase` 要求首个请求已含异步结果，与既有响应式契约不符；原版本单核 race 重复 10 次有 8 次失败。`tests/system/verify_channels.py:91` 的多表读取不在同一事务，受控并发写入可让旧实现误判全部完成；原 E2E 也出现事件数 110→112 的只读假失败。修正测试前提与快照读取，保留全部结果/权限/只读断言。 |
| AGENTS / Skills | 根 `AGENTS.md` 的命令、版本、事务/重放约束与当前代码一致；`tests/fixtures/coding-v1/AGENTS.md` 是 coding 隔离测试输入，不是全仓指令。仓库没有独立 `SKILL.md`；运行时 skill catalog 的锁、作用域、哈希和容量限制有测试，不能删除外部动态加载路径。 |

## 按优先级执行

评分按实际风险和收益排序；“性能”没有基准数据时不承诺加速倍数。

| 顺序 | 改进 | 正确性/稳定性 | 复杂度/维护收益 | 性能价值 | 验证 |
| --- | --- | --- | --- | --- | --- |
| 1 · P2 | 修复调度测试错误遮蔽；清理无效 IP 赋值；迁移离线 ECDSA API | 恢复失败断言，保留恢复状态读取 | 小而独立 | 无性能声明 | 调度器、Computer/SSH 测试及相关 race |
| 1a · P2 | 隔离 npm 审计测试的宿主偏好 | 防止环境设置造成假失败 | fixture 明确拥有本地请求 | 无性能声明 | 带 `NPM_CONFIG_AUDIT=false` 的既有审计测试 |
| 1b · P2 | 修正异步恢复 fixture；渠道断言使用一致读取快照 | 消除可复现的测试竞态，不改生产调度 | 恢复点与等待条件准确表达契约 | 无性能声明 | 单核 race 重复、受控 SQLite 交错、完整 race 与核心 E2E |
| 2 · P2 | 将多处复用的 HTTP 模型测试 peer 提取到独立 fixture 模块，解除两个 gate 的环 | 不改协议或场景 | 消除测试模块相互依赖 | 无性能声明 | AST/import 检查、Python harness、受影响系统 gate |
| 3 · P2 | MCP 按注册、每次调用的冻结检查、响应解析划分私有函数 | 保持 `Ambiguous`、取消、沙箱和审批语义 | 缩短捕获大闭包，便于审查错误路径 | 无性能声明 | 真实 stdio、恶意返回、漂移、取消回归 |
| 4 · P3 | 资源路由归 `assets.go`，复用固定目录；清理失效模板 helper | 保持 URL、HTTP 状态、安全检查顺序 | 减少入口职责和死注册 | 消除每请求路由表构造 | Web 包及实际浏览器/E2E |
| 5 · P3 | 修正文档模块示例，记录验证和限制 | 不影响运行时 | 防止 Agent 沿失效目录查找 | 不适用 | 文档检查、diff 自审 |

每批修改后执行定向验证与 diff 自审；最后运行 `make test lint`、相关 E2E 与 race。公共接口、生产配置、冻结协议和 schema 均不在修改范围内。

## 保留与后续决策

- 不拆分权威写事务、不统一不同领域的只读游标格式、不合并带不同隐私策略的 Memory/Conversation Store；收益不足以抵消一致性与兼容风险。架构调整或接口变更需先确认。
- 不清理旧快照和迁移，不修改 CodeMode sidecar/prelude 字节：运行时指纹参与已接纳任务的重放。
- Staticcheck 基线有 86 条诊断，首轮修复后剩余 79 条。追加审查已修正其中 32 条（26 处 HTTP 状态常量、3 个私有错误变量命名、3 处简单表达式）；另 47 条来自 30 处既有对外错误文案，逐行记录 ST1005 兼容例外并保留全部文本。运行 v0.8.2 默认检查已无未处理诊断；没有全局关闭规则，也不把兼容例外计为运行时缺陷修复。
- 界面轮询和精确向量检索的性能价值需生产规模数据；当前仅有本地测试，不据此改轮询周期、缓存或检索算法。
- macOS VM、Keychain、真实账户与付费模型未获本环境运行证据；确定性 fixture 只证明软件机制。首轮仅运行核心与相关 gate；追加审查补跑完整 `make test-full`，结果见后文。后续优先补目标 macOS 环境验证，以及组合根的失败/恢复场景；不为覆盖率数字添加镜像实现的测试。

## 首轮执行与验证记录

原始输出保留在忽略目录 `evidence/engineering-review/`，不将运行日志纳入源码。各批已分别自审；验证结果区分初次失败、基线复现和修正后门禁。

- 基线：Go、Provider Bridge、CodeMode 通过；`make test lint` 在浏览器阶段因缺少锁定 Chromium 终止，不能计为全绿。后续通过现有 `python3 scripts/setup.py --part browser` 安装锁定版本并通过 doctor；未替换版本或绕过下载校验。
- Go 覆盖率：`go test -coverpkg=./internal/...,./cmd/... -coverprofile=evidence/engineering-review/coverage.out ./...` 通过；结果为测试基线，不包含系统测试覆盖。
- 批 1：调度器、`computer/macosvm/...` 定向测试通过；自审确认三个 `state` 调用及原错误分支均保留，ECDSA 标量完全不变。
- 批 2：提取前后的 `Endpoint` AST 完全相同，四个 gate 导入通过；复用现有断言，无新测试框架。
- 批 3：MCP 包真实 stdio 回归通过；自审确认所有返回码、空/null 区别、冻结哈希与 `Ambiguous` 判定一致。
- 批 4：新增资源请求边界用例先在原实现通过，再验证提取后的实现；覆盖公开资源、未知路径、路径穿越、DNS rebinding、CSRF、方法限制及安全响应头。旧 diff 链接仍通过真实 URL 解析回归。
- 批 5：npm 审计测试在宿主 `NPM_CONFIG_AUDIT=false` 下通过。异步恢复测试补齐输入消费后从 settle 边界恢复，`GOMAXPROCS=1 go test -race -count=10 ./internal/agent/sqlite -run '^TestDeferredToolRecoveryRetainsOutcomeAndAuthority$'` 全部通过；正常/unknown 两种结果及单次调用、单次交付断言未放宽。
- 批 6：真实 SQLite WAL 受控交错在两个读取间提交新投递与 Run 终态：旧读取返回“全部完成”，修正后仍观察到 running 并继续等待。`rows()` 与 `facts()` 显式开启只读事务，无固定 sleep 或事件过滤；源码中保留原只读对比。
- 初次全量门禁如实保留：核心 E2E 前 8 项通过，channels 只读对比失败；race 的恢复用例失败并触发其他 SQLite 分片取消。两处问题均在未修改的 `ffab679` 检出，随后修正测试状态/读取边界，而非修改运行时契约。
- 首轮 `make test lint` 通过（`test-lint-final.log`）：Go 全包、Provider Bridge 58/58、CodeMode 19/19、浏览器 8/8、Python harness 96 通过/1 项 macOS Seatbelt 跳过；文档、gofmt、vet、TypeScript、前端产物、Shell 与依赖校验通过。`go mod tidy -diff` 无差异。
- 补充系统 gate：capabilities、computer、tui 通过；`focused-e2e/summary.json` 记录 `passed=true`、`source_stable=true`，所有受管进程组已回收。
- 首轮 `make test-e2e E2E_EVIDENCE=evidence/engineering-review/e2e-final` 通过：startup、main、tasks、children、tools、code-recovery、local-provider、account、channels **9/9**；汇总 `passed=true`、`source_stable=true`，受管进程组全部回收。
- 首轮 `make test-race` 通过：覆盖 66 个 Go 包，SQLite 285 个顶层用例完整且无重复分片；无 race 报告，`source_stable=true`、`selection_complete=true`，全部进程组回收。证据归档至 `race-final/inventory.json`，初次失败另存 `race-first/`。
- 首轮交付仅包含局部实现、测试及文档变更；未修改生产配置、公共接口、schema、锁定依赖或 sidecar 字节。原生平台/真实服务与大规模性能结论仍按上述限制保留，不以确定性测试替代。

## 追加审查与问题处置

用户要求处理全部剩余问题后，继续检查静态诊断、测试资源生命周期和此前未执行的扩展场景。原有外部行为、接口、生产配置及冻结协议约束继续适用。

| 优先级 | 位置与证据 | 处置 |
| --- | --- | --- |
| P2 | `tests/system/verify_channels.py:ChannelHarness.rows` 在成功和查询失败后都留下可继续执行 SQL 的连接；Python 的连接上下文只管理事务，不会关闭连接。扫描发现 18 个系统测试模块共有 77 处同类写法。 | 显式使用 `contextlib.closing` 管理连接，并保留内层连接上下文的提交/回滚行为。AST 核对 77 处事务体保持一致，未让游标逃出关闭边界。 |
| P2 | 上轮跨快照读取证据只保存在忽略目录，缺少持续回归。 | 新增 `tests/harness/test_channels.py`：两个真实 WAL 连接在查询之间提交新投递及 Run 终态，验证旧投递列表不会与新 Run 状态混合；同时验证成功/失败均关闭连接。关闭回归先失败，再经修正通过。 |
| P2 | `internal/agent/sqlite/scheduler.go:schedulerCTE` 的 live CTE 被内联，按 Agent 统计容量时反复扫描历史 Run；容量三批同规模任务耗时约 48/85/129 秒。 | 单条 SQL 内物化一次活跃租约集合，使所有额度检查复用同一语句快照；不改变排序、fencing、额度、schema 或公共接口。最终源码容量测试约 29/31/33 秒，全部安全断言通过。 |
| P3 | HTTP 调用中 26 处数字状态码、`skill`/`mcp`/`tui` 的 3 个私有错误变量、布尔比较/变量初始化/测试格式化各 1 处。 | 使用等值标准常量、惯例私有命名和直接表达式。错误值、HTTP 状态、调用顺序与公开符号保持不变，定向 Go 测试及 race 通过。 |
| 兼容例外 | 7 个 Go 文件中的既有用户提示和导出错误文案触发 47 条 ST1005 诊断。 | 在 30 处源位置说明保留原因；比较全部 `errors.New` 文本确认未改变。其他诊断规则保持启用。 |
| 有意设计 | 写事务边界、领域隐私策略、历史迁移、冻结快照、动态资源与 Skill 发现。 | 经调用、历史与回归核对继续保留。没有发现要求改变这些边界的具体故障，不为“清零问题”删除兼容或保护机制。 |
| 环境验证 | macOS Keychain/VM/Seatbelt、真实账号与付费模型。 | Linux 环境不能提供这些平台或服务的真实执行证据。它们是验证范围限制；继续保留平台实现，需对应环境补验。 |

追加证据位于 `evidence/engineering-review/followup/`：`equivalence.json`、`channel-reader-before.log`、`channel-reader-after.log`、`staticcheck-final.log`、`focused-go.log`、`changed-race.log`、`test-lint-final.log` 和完整扩展套件结果。

- `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...` 通过，无未处理诊断；工具版本固定，但未添加生产依赖或新的 CI 框架。
- `make test lint` 通过：Go 全包、Provider Bridge 58/58、CodeMode 19/19、浏览器 8/8；Python harness 98 通过、1 项 macOS Seatbelt 跳过。
- 受影响 Go 模块的 `go test -race -count=1` 通过，覆盖 skill、MCP、interaction、macosvm/fixture、setup、TUI。上轮完整 66 包 race 证据继续保留。
- 性能诊断使用现有 SQLite 驱动、真实 schema 和 2,000 条历史 Run，100 次查询约 812 ms → 4.5 ms，选择结果一致；候选二进制的完整容量验证为 3,000 子任务、2,994 完成及 6 个预设业务失败，总耗时 260.93 s → 93.24 s，最终源码复验为 94.12 s。结果是本环境测量，不外推生产倍数；原始执行计划和候选构建覆盖文件保存在 `scheduler-*.log`、`production-overlay.json`、`capacity-materialized/`。
- 追加修复后的第一轮完整 `make test-full` 已通过 **26/26**，源码稳定且受管进程组全部回收；包含所有核心与扩展 gate。随后应用已验证的调度查询优化，最终版本门禁结果继续记录在下方。

- 验证过程中的失败也保留：临时 Go 诊断文件放在普通 evidence 子目录时被 `go list ./...` 当作额外包发现，导致一次构建失败；移至 `_probes/` 后恢复 66 包，重跑测试/lint/静态检查通过。另一次并行重负载验证中，未改动的 Bridge 突发消息用例超过既有 1 秒 RPC 时限；该原用例连续 10 次 race 复验通过。保留严格超时和所有断言，最终按仓库既有分组入口安排资源，避免 Bridge 与重负载门禁同时运行。失败日志分别归档为 `*-probe-discovery.log`、`race-burst-timeout/`，不将失败记作通过。

### 最终版本验收

| 检查 | 最终结果与证据 |
| --- | --- |
| 测试与 lint | `make test lint` 通过，见 `test-lint-final.log`；Python harness 98 通过、1 项 macOS Seatbelt 跳过，Go、Node、浏览器、文档、格式、类型和依赖检查通过。 |
| 静态检查 | `go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...` 通过，见 `staticcheck-final.log`；47 条既有文案诊断按上述兼容例外处理。 |
| 完整系统测试 | 最终源码 `make test-full E2E_EVIDENCE=evidence/engineering-review/followup/full-final` 通过 **26/26**，`source_stable=true`，所有受管进程组回收，见 `full-final/summary.json`。 |
| 全包竞态覆盖 | 使用既有 `tests/system/race.py` 的两个分组入口：`GOMAXPROCS=3 python3 tests/system/race.py --part sqlite` 与随后独立执行的 `python3 tests/system/race.py --part packages`。两组均通过，覆盖全部 **66 包**；SQLite **285** 个顶层用例全部通过、无遗漏或重复，无 race 报告。原有 `-race -count=1 -timeout=15m` 和测试断言不变。 |
| 证据一致性 | `race-sqlite-final/inventory.json`、`race-packages-final/inventory.json` 与完整系统测试具有相同源码提交/差异指纹，均 `source_stable=true`，所有受管进程组回收。交叉核对结果保存在 `combined-validation.json`，不是把此前失败的单次 `make test-race` 标为通过。 |
| 实测性能 | 同一容量场景的最终源码运行耗时 **94.12 s**，原查询为 **260.93 s**；三批各 1,000 子任务分别约 **29.39/31.47/33.26 s**，2,994 完成、6 个预设业务失败，最终无残留工作租约。 |

本轮已处理所有确认且可安全修正的发现。公共接口、生产配置、schema、锁定依赖、审批与重放协议保持兼容；有意保留的设计和 macOS/真实服务验证限制已列明。上述门禁完成后仅整理本报告，并再次执行文档与 diff 检查。
