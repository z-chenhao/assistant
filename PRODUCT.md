# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

项目所有者日常使用的 personal agent，主要通过持续对话安排研究、开发、学习、生活跟进与资产续费核对。这些使用场景不是已接入外部账户的承诺。

## Product Purpose

接续同一个 Agent 的长期对话，委派后台任务，查看已交付文件，并审阅需要确认的具体动作。2026-11-02 用户确认默认接上最近对话，旁边查看近期活动、任务进展和输出成果。

## Capabilities and Constraints

依据 docs/feature-map.md、docs/features/channels.md 和现有 Web 路由：已有 Conversation、Tasks 列表与详情、附件、冻结审批、模型保存/测试/显式选用、通知偏好、能力配置与只读 Developer 检查。每个 Agent 一个 Conversation，任务控制由 Main 理解；批准不等于已执行，未知外部结果不能自动重发。

最新授权已从隔离 Demo 推进到正式 Web 重构：保留真实 Conversation、Task、Approval、Artifact、模型/账户、通知、能力与 Developer 流程，使用已确认 Demo 的视觉。Chat 中按需读取只读 Worker 执行与 Computer 画面；不宣称已有多会话、独立文件库、完整记忆管理页或通用富 UI。测试使用隔离数据根与明确的脚本模型；本地现有 Web 保持其原数据与凭据，不自动选择模型或连接外部账户。

## Brand Commitments

最新请求指定参考用户 Chrome 中的 ChatGPT 与其 Jarvis Dots，采用非纯黑、非纯白的黑白主色。品牌名由用户改为 Steward，标语为 “Your personal AI steward.”；纯文字字标使用接近 SpaceX 的舒展几何字形，最新字体要求：Logo 维持 Michroma，英文界面使用 Inter、英文回复正文使用 Source Serif 4、代码使用 JetBrains Mono；macOS 中文优先苹方，跨平台中文回退思源黑体。Newsreader 作为英文正文备选，不额外下载。Agent 形象更新为用户给定的绒感小幽灵，提供黑色与柔白两版适配主题；保留胶囊眼睛、笑脸、圆润下摆。角色轻微上下漂浮、偶尔眨眼；挥手使用预生成的完整无损帧，身体不缩放，手臂保持连贯。运行时不使用网格或 WebGL，可暂停并跟随系统减少动效。Agent 名称为 Ghost；本节为当前品牌基准。用户明确要求熟悉的对话产品结构，作为本轮方向。

## Evidence on Hand

2026-11-02 原生 Chrome 实际观察：ChatGPT 首页的应用导航、侧栏、居中输入；Jarvis 的持续消息流、可开合资料、近期活动和输出内容。仅记录结构观察，不复制真实私人聊天、文件、账号或设备信息到 Demo。功能依据为现有仓库文档和 internal/interaction/web 路由/模板。


## 本轮补充确认

用户要求字体、组件也与参考保持一致；组件基于实际 Chrome 观察及 OpenAI 官方公开 Apps SDK UI 的类型、控件、圆角与间距证据核对，不声称其公开 SDK 等于当前 Web 私有实现。用户进一步明确“只需要一个侧边栏”，因此移除左侧独立应用图标栏与右侧常驻资料栏；只有一个左导航，活动、任务和文件在主内容区展开，继续对话时保留草稿。此条取代先前常驻右栏安排。

产品名统一为 Steward，Agent 名为 Ghost；命令为 `steward`，环境变量使用 `STEWARD_*`，默认数据根为 `~/.local/share/steward`，Go 模块为 `github.com/z-chenhao/assistant`。全项目使用单一身份，不保留其他品牌的别名或配置读取回退；不改写已冻结的执行记录。

界面语言默认英文，不自动跟随浏览器语言；设置中可显式切换简体中文并在当前浏览器持久化。此项只影响产品界面和状态文案，不改写对话、任务内容、文件、原始证据或 Agent 执行语言。

最新确认：头像改用完整无损帧播放，保留黑白形象和轻微上下漂浮。导航仅 Chat/Tasks/Settings/Developer；Chat 顶部居中两行显示 Agent 名称 Ghost、动态头像，下方持续对话。右上角 Computer 与当前 Worker 入口分别展开只读实时桌面和 Worker 执行记录；两者互斥使用同一详情区域。Worker 包括进行中与已结束执行，按实际开始时间（未开始时取创建时间）倒排，状态取后端。Task 保留独立列表与详情。

Steward 是产品品牌，Ghost 是当前 Agent 的显示名称。持续对话和欢迎空态只在顶部保留一个动态头像；每条回复与用户消息不重复显示可见头像或名称，读屏仍保留发言者标识。

本机 Web 省略 token_ref 时直接进入，配置 token_ref 时保留口令模式；实际监听、peer 与同源校验以及渠道绑定和动作审批继续生效。Chat 右上角 Computer / Worker 改为纵向上下排列。

待确认入口统一放在 Chat 右上角 Computer / Worker 下方；仍按真实待审批数出现、点击定位原审批，不在左上角重复展示。

Web task intent: the owner primarily uses Tasks to understand upcoming Agent work and its configuration, and wants to edit scheduling instructions/rules manually. Capability setup should use friendly forms by default, with advanced configuration available when needed. Preserve the distinction between saved scheduling intent, dispatched Tasks, frozen actions and delivered results.
