# 能力连接与就绪引导

让用户发现、配置、验证和修复可选能力。

[需求主索引](../feature-map.md) · [技术方案](../designs/capability-setup.md) · [测试方案](../test-plans/capability-setup.md)

## 默认能力与设置

正常安装启动无需先填写能力配置。Memory 自动生成 256 位密钥并保存在系统 SecretStore；搜索默认使用本地全文检索；Skills 自动准备私有空目录。Computer 在 Apple silicon Mac 首次使用时自动下载固定版本的 Lume 和 macOS 镜像、校验并准备私有 Guest。其他平台显示不支持自动 macOS 桌面，本地文件与命令仍可用。

Settings / Capabilities 提供“自动（推荐）／自定义／关闭”。自定义资源优先；关闭保留已有数据。保存后重启，新 Run 使用新配置；正在执行和恢复的 Run 继续校验原冻结能力。显式 `-config` 部署保留现有配置语义，可通过 `defaults` 选择自动资源。


## 功能说明

Settings → Capabilities 保留 Memory、MCP tools、Skills、Files and commands、Computer 五个入口。Memory 包含存储与检索，可选语义检索放在 Memory 的高级设置中。默认开启显示“已启用（默认）”，自定义配置显示“已启用”；准备中、异常和重启后生效单独呈现，连接检查证明不作为启用状态。页面与 Preferences 共用左侧导航、居中窄列、紧凑分组行和可搜索下拉，配置后直接保存。

## 需求与验收

<a id="f-030"></a>

### F-030 · 统一能力设置与诊断体验

**需求**：用户能在设置中看见记忆、embedding、MCP、Skills、代码工作区及 Computer 是否已连接，并完成配置验证。

**当前实现**：底层保留六类资源的独立验证与迁移边界，用户只需在五个主入口配置。普通 Save 自动执行有界检查并保存；自定义连接可单独检查。YAML、来源路径、检查时间和详细说明放在高级设置。关闭 Memory 同时关闭检索并清除自定义 embedding 连接，保留数据和密钥引用；重新启用恢复默认本地检索。保存仍绑定成功验证的精确候选和未变化的源文件。新配置经明确重启后供新 Run 使用，运行中的配置和旧快照不会热改。

**边界**：模型与通知继续使用各自设置页；不增加模型可见的能力发现 API。自定义语义检索、Skill catalog、MCP 和 Computer 连接仍可由用户提供；自动资源走默认准备流程。诊断验证连接与资源边界，不评价模型质量，不代替 Guest 首次运行时的身份检查，也不代办账户授权或数据迁移。

**验收**：未配置 → 编辑 → 失败可修复 → 成功测试 → 保存待重启 → 重启加载 → 新 Run 使用新能力；旧能力快照保持不变。关闭 JavaScript 后仍可完成表单操作，窄屏无横向溢出。

## 如何使用

1. 打开 Settings → Capabilities，选择一项能力，按说明准备自己的服务、已有凭据引用和私有路径。
2. 启用该能力并填写普通字段；MCP 使用 provider／允许工具行，Computer 按连接、Guest、兼容与运行时分组。需要未展示字段时再展开 Advanced YAML。
3. 点击 Test settings（高级编辑器为 Test configuration）。失败后根据诊断修改并再次测试；测试不会保存文件。成功后可以刷新页面继续查看尚未保存的候选。
4. 点击 Save tested settings（高级编辑器为 Save verified configuration）。修改候选、其他设置保存、外部文件修改、测试结果超过五分钟或进程重启都会要求重新验证。过期页面不能覆盖已观察到的新配置。
5. 按原启动方式停止并重启 Steward。页面显示的“启动时已配置”和“待重启”用于区分当前配置与保存配置。新的请求创建新快照；不兼容的旧快照继续按既有运行时规则拒绝恢复，不会自动升级。

凭据只填写 `env:NAME` 或 `credential:NAME` 引用；真实值放 SecretStore/进程环境，不放 YAML、命令参数或截图。新环境引用需先提供给 Steward 进程，再测试。保留 Memory 的原始 32-byte 密钥和备份；更换引用须指向同一密钥。embedding endpoint/model/dimensions 的变化需要离线重建索引，单纯连接测试不会迁移已有向量。先前已停用能力的数据也不会被删除；再次启用必须保留原身份。

清空 Memory、embedding、MCP、Skills 或 Computer 配置可停用它；清空 Files and commands 会恢复默认本地工作区，不删除工作区。启用 Computer 前先清空显式本地执行覆盖；Computer 开启后文件工具与桌面工具共用 Guest。

## 诊断范围

| 能力 | 实际检查 | 不执行 |
| --- | --- | --- |
| Memory | SecretStore 中 32-byte 密钥可用，完成固定文本 HMAC | 读写用户记忆、密钥轮换 |
| Memory search | 向所选端点发送固定诊断句，校验模型、维度与向量 | 发送对话、选择隐式模型、重建索引 |
| MCP | 现有内核沙箱中 initialize / tools list，核对 allowlist | tools call 或业务副作用 |
| Skills | 实际 catalog、SKILL.md 与有界资源校验 | 执行 Skill 内容 |
| Files and commands | 私有工作区中用现有内核沙箱运行固定 printf | 用户提交的 shell、网络访问 |
| Computer | Node sidecar 握手、Lume 版本/独立存储、sealed base 完整性、凭据引用 | 下载、克隆、启动、停止 VM 或访问 Guest |

## 配置来源与恢复

显式 `-config` 文件是编辑目标；未指定时读取 `STEWARD_DATA_ROOT/config.yaml`，默认是 `~/.local/share/steward/config.yaml`。文件不存在则沿用内置默认值，首次成功保存才创建 0600 文件。显式文件必须是普通文件，设置页拒绝符号链接；其他 section 和注释保留。

设置页使用完整配置校验，路径必须遵守各能力的隔离要求，配置文件也不能放进 Agent 或 provider 可读写的工作目录。手工把整个文件改坏导致进程无法启动时，需先从终端修复文件；Web 不能修复一个尚未启动的进程。只读 `doctor` 仍不读取凭据或探测服务，页面上的 Test 是用户主动发起的独立操作。

## 详细操作参考

- [MCP、Skills 与工作区约定](../designs/integrations.md)
- [记忆维护操作](memory-context.md)
- [Computer 生命周期](../designs/computer-use.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/capability-setup.md)；检查和平台证明范围见 [测试方案](../test-plans/capability-setup.md)。
