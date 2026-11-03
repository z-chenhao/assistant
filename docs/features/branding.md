# Steward 品牌与个人工作空间

Your personal AI steward.

[主索引](../feature-map.md) · [技术方案](../designs/branding.md) · [测试方案](../test-plans/branding.md)

## 功能说明

Steward 是拥有对话、工具、记忆与委派任务的私人助手。产品名为 Steward，当前 Agent 名为 Ghost。字标使用 Michroma，形象为黑白小幽灵。正式 Web 使用非纯黑纯白的单色界面，英文界面为 Inter，回复正文为 Source Serif 4，中文优先 PingFang SC、回退自托管思源黑体；代码为 JetBrains Mono。图片与字体全部同源嵌入。

## 需求与验收

<a id="f-041"></a>

### F-041：品牌、身份与工作空间

- 单个 256px 侧栏；首页接续真实持续对话，采用居中正文、无卡片回复、浅灰用户气泡与多行输入。Computer/Worker 详情按需查看实时画面和执行历史；成果仍归实际任务及其交付消息。
- Conversation、Tasks、Settings & capabilities、Developer 共用颜色、字体和组件。模型未就绪时显示配置入口并禁用发送。
- 浅色用深色幽灵，深色用浅色幽灵；可暂停动效，尊重 reduced-motion、页面可见性和离屏状态。动画只表达角色反馈，不伪造任务状态。
- 实际发送、附件、审批、冻结哈希、任务接续、分页与实时更新维持既有执行链路。未确认发送保留草稿与附件，不自动重发。
- 品牌贯穿 Web、终端、授权页、安装器与打包说明。命令为 `steward`，环境变量使用 `STEWARD_*`，默认数据根为 `~/.local/share/steward`，模块为 `github.com/z-chenhao/assistant`。不提供其他品牌的命令、路径、协议或配置别名。

## 如何使用

构建后运行 `bin/steward web serve`（需 Web 配置）或正常启动应用。默认数据根为 `~/.local/share/steward`。在首页对话并审阅具体审批；Tasks 可查看摘要、历史、关联消息与成果，Settings 管理模型、账户、通知和能力，Developer 查看真实执行与冻结上下文。Chat 右上角 Computer/Worker 展开即时查询的只读详情；已完成的 Worker 不等同于最终用户交付或文件成果。

外观、侧栏折叠及头像动效偏好保存在当前浏览器；它们不更改 Agent 执行策略。没有新增多会话、独立记忆浏览器或虚构的文件库。完整操作见[安装](installation.md)及[Web 功能](channels.md)。

### 界面语言

默认界面为英文，不根据浏览器语言自动切换。进入 Settings & capabilities → Preferences → Interface language，可选择简体中文并保存；中文路径为“设置与能力 → 偏好设置 → 界面语言”。偏好仅对当前浏览器有效，刷新和跨页面导航后保留，登录页及实时片段也按该偏好呈现。仅翻译产品界面、状态和诊断说明；对话内容、任务标题与摘要、文件、模型名、冻结参数及原始执行证据保留原文。
