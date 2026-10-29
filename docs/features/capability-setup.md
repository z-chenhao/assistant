# 能力连接与就绪引导

让用户发现、配置、验证和修复可选能力。

[需求主索引](../feature-map.md) · [技术方案](../designs/capability-setup.md) · [测试方案](../test-plans/capability-setup.md)

## 功能说明

Settings → Capabilities 汇总 Memory、Memory search（embedding）、MCP tools、Skills、Files and commands、Computer 六类能力。列表区分未配置、已配置但未验证、连接验证成功、需要修复，以及保存后需要重启的变更。文件工具由 Computer 承担时显示 Uses Computer，不误报为工具缺失。

## 需求与验收

<a id="f-030"></a>

### F-030 · 统一能力设置与诊断体验

**需求**：用户能在设置中看见记忆、embedding、MCP、Skills、代码工作区及 Computer 是否已连接，并完成配置验证。

**当前实现**：六类能力均可从真实 Web 设置入口查看、编辑、测试和保存。编辑器仅修改当前能力的 YAML；保存要求五分钟内成功验证的同一候选配置及未变化的源文件。新配置经明确重启后供新 Run 使用，运行中的配置和旧快照不会热改。

**边界**：模型与通知继续使用各自设置页；不增加模型可见的能力发现 API。外部模型服务、Skill catalog、MCP 可执行程序、macOS/Lume 及 sealed base 仍需用户显式准备。诊断验证连接与资源边界，不评价模型质量，不代替 Guest 首次运行时的身份检查，也不代办账户授权或数据迁移。

**验收**：未配置 → 编辑 → 失败可修复 → 成功测试 → 保存待重启 → 重启加载 → 新 Run 使用新能力；旧能力快照保持不变。关闭 JavaScript 后仍可完成表单操作，窄屏无横向溢出。

## 如何使用

1. 打开 Settings → Capabilities，选择一项能力。复制 Configuration example，按 Help 准备自己的服务和私有路径。
2. 在配置框中填写该能力的字段；MCP 填 provider 列表，Computer 填 `computer` 和 `javascript` 两个 section。其他编辑器不包含顶层 section 名。
3. 点击 Test configuration。失败后根据诊断修改并再次测试；测试不会保存文件。成功后可以刷新页面继续查看尚未保存的候选。
4. 点击 Save verified configuration。修改候选、其他设置保存、外部文件修改、测试结果超过五分钟或进程重启都会要求重新验证。过期页面不能覆盖已观察到的新配置。
5. 按原启动方式停止并重启 Collie。页面显示的“启动时已配置”和“待重启”用于区分当前配置与保存配置。新的请求创建新快照；不兼容的旧快照继续按既有运行时规则拒绝恢复，不会自动升级。

凭据只填写 `env:NAME` 或 `credential:NAME` 引用；真实值放 SecretStore/进程环境，不放 YAML、命令参数或截图。新环境引用需先提供给 Collie 进程，再测试。保留 Memory 的原始 32-byte 密钥和备份；更换引用须指向同一密钥。embedding endpoint/model/dimensions 的变化需要离线重建索引，单纯连接测试不会迁移已有向量。先前已停用能力的数据也不会被删除；再次启用必须保留原身份。

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

显式 `-config` 文件是编辑目标；未指定时读取 `COLLIE_DATA_ROOT/config.yaml`，默认是 `~/.local/share/collie/config.yaml`。文件不存在则沿用内置默认值，首次成功保存才创建 0600 文件。显式文件必须是普通文件，设置页拒绝符号链接；其他 section 和注释保留。

设置页使用完整配置校验，路径必须遵守各能力的隔离要求，配置文件也不能放进 Agent 或 provider 可读写的工作目录。手工把整个文件改坏导致进程无法启动时，需先从终端修复文件；Web 不能修复一个尚未启动的进程。只读 `doctor` 仍不读取凭据或探测服务，页面上的 Test 是用户主动发起的独立操作。

## 详细操作参考

- [MCP、Skills 与工作区约定](../designs/integrations.md)
- [记忆维护操作](memory-context.md)
- [Computer 生命周期](../designs/computer-use.md)

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/capability-setup.md)；检查和平台证明范围见 [测试方案](../test-plans/capability-setup.md)。
