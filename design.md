---
name: Steward
description: "Steward Web 的视觉与交互规范"
colors:
  light-background: "#f9f9f9"
  light-surface: "#fcfcfc"
  light-text: "#212121"
  light-muted-text: "#666666"
  light-divider: "#dfdfdf"
  light-control-border: "#858585"
  light-focus: "#767676"
  dark-background: "#212121"
  dark-surface: "#282828"
  dark-text: "#ededed"
  dark-muted-text: "#afafaf"
  dark-divider: "#3e3e3e"
  dark-control-border: "#777777"
  dark-focus: "#9f9f9f"
typography:
  page-title:
    fontFamily: "Inter, PingFang SC, Source Han Sans SC, Microsoft YaHei, sans-serif"
    fontSize: "1.375rem"
    fontWeight: 600
    lineHeight: 1.35
  section-title:
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: 1.5
  ui:
    fontSize: "0.875rem"
    lineHeight: 1.6
  metadata:
    fontSize: "0.8125rem"
    lineHeight: 1.65
  reply:
    fontFamily: "Source Serif 4, Georgia, PingFang SC, Source Han Sans SC, Microsoft YaHei, sans-serif"
    fontSize: "16px"
    lineHeight: 1.75
rounded:
  button: "9px"
  surface: "12px"
spacing:
  workspace-gutter: "40px"
  compact-gutter: "20px"
  section: "20px"
  compact-section: "16px"
  action-gap: "10px"
---

# Steward 界面设计规范

本文件是产品界面设计的统一入口，适用于 Chat、Task、Dev、Settings 及未来新增页面。修改页面结构、导航或公共控件时遵守本规范；具体功能行为与限制见 [Feature Map](docs/feature-map.md)，实现边界见[渠道方案](docs/designs/channels.md)，品牌资源见[品牌方案](docs/designs/branding.md)。

## Colors

浅色与深色共用相同的语义。主操作使用高对比填充，普通操作使用透明底和细边框。状态同时使用文字，颜色提供辅助识别。内容分隔线、输入边界与键盘焦点各自使用对应的颜色。实际色值维护在 [共享样式](web/src/foundation.css)。

Chat 发送按钮是单色主操作的例外：明暗主题均使用 `#B84F2F` 橙色背景和白色图标，样式限定在消息输入区。

## Typography

操作页依次使用页面标题、区块标题、常规界面和辅助文字四档。在根字号为 16px 时分别为 **22、16、14 和 13px**。表单标签、状态、帮助文案应按角色选档。窄屏普通文本输入和下拉控件使用 16px。Chat 欢迎语和回复采用阅读排版。字标使用 Michroma，代码使用 JetBrains Mono。


## 设计原则

- 导航层级表达用户所在的位置，同一层级始终承担同一种职责。
- 常驻区域只放经常使用、跨页面需要的入口；个人偏好集中在 Settings。
- 页面优先呈现可操作的设置项、真实状态和必要反馈，详细说明按需展开。
- 控件反映真实能力。未实现的功能不占分类，不显示没有作用的开关。
- 界面调整保留原有授权、版本校验、失败反馈和数据边界。

## 两级导航

### 一级：模块切换

一级侧栏在各模块之间保持相同位置和次序。

| 位置 | 入口 | 职责 |
| --- | --- | --- |
| 上方 | Chat | 持续对话、输入与用户交付 |
| 上方 | Task | 任务列表与详情 |
| 上方 | Dev | 执行检查与开发者数据 |
| 左下角 | Settings | 偏好与配置，默认打开 Preferences |

一级入口以模块为单位选中。进入 Settings 或 Dev 时保留一级侧栏，在其右侧展开模块内部的二级栏。跨分类和详情页面切换不移动一级入口。模型修复等具体操作直接链接到相应配置页。

侧栏底部只保留 Settings 入口。工作台侧栏不放宣传语、主题快捷切换或头像动效按钮；外观和动效通过 Settings → Preferences 管理。

### 二级：模块内部分类

二级栏位于一级栏与内容区之间，展示模块名称及内部分类。

| 模块 | 当前二级分类 |
| --- | --- |
| Settings | Preferences、Notifications、Resources、Capabilities |
| Dev | Traces、Database |
| Task | 当前没有独立分类，不展开空二级栏 |

二级分类以当前分类为单位选中，详情继承所属分类。一级模块选中态与二级分类选中态同时保留。分类搜索只筛选二级项目，不影响一级入口。

Task、Dev 或其他模块未来增加内部分类时复用同一结构。列表筛选、数据库集合和详情标签属于内容区，不提升为一级模块或额外导航层。

### 内容区

内容位于导航右侧，按实际可用宽度响应布局。正文标题、设置分组和主操作遵循相同的对齐线。Settings 使用居中窄列；Task 和 Dev 可按内容宽度展示列表、详情和检查面板。

## 尺寸与响应式

桌面一级栏宽 224px、二级栏宽 216px，由共享变量控制；模块没有二级分类时，内容只避让一级栏。Settings 内容容器最大宽度为 680px，包含左右留白。两级栏共用折叠／展开行为，桌面折叠为 64px 图标栏，保留四个模块入口、选中态和展开按钮；同一浏览器的折叠偏好跨模块、分类和刷新保持一致，并在首屏样式前应用。

视口宽度不大于 760px 时，导航收进同一个抽屉。存在二级分类时，一级保持窄列，二级占据剩余宽度；两级入口和选中态同时可达。抽屉与内容不横向溢出。资源列表按自身可用宽度切换行布局，不能只根据视口宽度判断；桌面双侧栏留下的窄列也必须正常展示模型名与操作。操作页按钮、普通输入、选择器及折叠入口的触控高度至少 44px。

导航打开后焦点进入抽屉，Tab 在整个导航容器内循环；Escape、关闭按钮或遮罩关闭后焦点返回打开按钮。无 JavaScript 时保留原生链接和静态可达的导航。

## 设置页与控件

设置项使用紧凑分组：圆角容器、细分隔线、左侧名称与必要说明、右侧值或操作。避免每个设置项各占一张大卡片，不在普通设置页堆叠实现细节。

- 下拉使用统一的紧凑圆角外观、选中标记和键盘操作；需要搜索时提供筛选与空结果。Escape 或点击外部关闭，关闭后保留合理焦点。桌面与窄屏使用同一套交互和尺寸规则；无 JavaScript 或浮层能力不可用时保留原生选择器。
- 输入框和多行文本保持统一标签、边框和间距。必填、无效和服务端错误在当前流程中说明，失败保留已填内容。
- 普通布尔偏好使用有名称的开关。涉及模型提供方授权等明确同意时，保留原有确认控件。
- 主操作使用实色按钮，次操作使用低强调按钮；危险操作使用危险色，并清楚说明作用范围。
- 退出账户等有影响的操作使用确认对话框，默认聚焦取消。取消和 Escape 不提交请求，确认只提交一次。
- 本地外观与动效偏好立即应用并保存在当前浏览器；语言确认不同值后自动保存，失败保留原生表单路径。能力配置修改后自动保存；服务端先校验最终候选，失败保留输入和重试入口。无 JavaScript 时保留原生提交。

下拉交互统一由 [Base UI 选择器](web/src/components/ui/select-view.tsx) 提供；服务端表单通过 [原生值适配器](web/src/components/ui/select.ts) 接入，React 富组件直接消费同一选择器。按钮、选择器、选项浮层、复选框与 Switch 的样式统一维护在[公共控件样式](web/src/components/ui/controls.css)。Settings、Task、Dev、Chat 富组件与后续页面复用同一组件体系；页面只负责布局、验证与保存，不能另建下拉或开关实现。单个布尔配置使用原生 checkbox 的 switch 语义，多选复选框与明确授权确认保留各自语义。开关视觉尺寸不扩张，点击和键盘焦点区域至少 44px。

### 状态与账户操作

标题旁展示必要状态。Resources 的模型列表默认收起配置，当前模型及连接状态在行内标明。展开一行即可查看连接标识。行尾保留连接和选用两个位置，自定义兼容端点未验证时显示「测试连接」，通过后显示「连接已验证」；ChatGPT 账户模型不显示测试操作。候选模型只提供「使用此模型」，选用后显示「当前模型」。首次成功连接自动用于对话，后续添加不覆盖当前选择。「测试连接」使用次要按钮。已有有效验证的当前模型可从连接位置选择「恢复使用当前模型」。共享账户作为分组标题，模型直接列在对应账户下，直连模型独立分组。账户操作单独展开，重连和退出在同一操作行。登录进度归发起页面，首次渲染与实时更新采用同一规则。授权窗口成功打开后，原页面保留简短的等待状态和取消入口。弹窗被拦截时，原页面提供手动继续链接。


## 文案、状态与动效

同级设置页保留可见的页面标题和一致的对齐线，Resources 的 Models 是标题下的区块。脚本加载前后应保持导航、设置分组及正文位置稳定；脚本专属设置预留位置，下载失败时恢复原生表单和导航。

连接检查反馈只对应检查时的字段；修改普通字段或导入高级 YAML 后清除旧反馈，自动校验并保存；不显示额外测试和保存按钮。通知偏好修改后撤下此前保存提示，明确显示未保存，仍须显式保存。

页面标题和字段名称优先使用用户能理解的名称。帮助、YAML、连接详情和诊断放进 Advanced；影响当前决定的限制直接显示。

启用状态与连接检查结果分别表达：默认启用显示“已启用（默认）”；准备中、不可用、需要检查和重启后生效使用各自的真实状态。Memory 与检索共用主入口，语义搜索属于其高级子设置。

外观和头像动效统一位于 Preferences。关闭头像动效后展示静态首帧，仍保留真实状态文字。系统开启“减少动态效果”时优先遵循系统偏好，关闭动效并解释开关为何不可用；动效不能成为获取状态的唯一方式。

品牌字标、色彩、字体与图片沿用现有[品牌资源](docs/designs/branding.md)，品牌宣传语可用于 README 和产品介绍，不占据常驻工作台导航。

## 验收

界面变更检查桌面、窄屏、明暗主题、键盘与无 JavaScript 的主要路线。导航检查两级选中态、模块切换、折叠恢复和焦点；设置检查输入保留、真实持久化、保存失败与取消确认。实际测试入口见[渠道测试方案](docs/test-plans/channels.md)和[品牌测试方案](docs/test-plans/branding.md)。


### 自动回归与评审分工

| 已确认的规则 | 回归落点 |
| --- | --- |
| 启用状态独立于连接检查证明；异常和重启状态不能被“默认开启”掩盖 | [状态单元测试](internal/interaction/web/capability_status_test.go)，中英文状态及有／无检查证明 |
| 默认密钥复用、关闭后保留数据身份、无效保存不落盘、陈旧版本拒绝 | [配置组件测试](internal/config/setup/defaults_test.go)、[Web 保存回归](internal/interaction/web/capability_server_test.go) |
| 一级入口常驻、Settings 在底部、二级栏不覆盖内容、手机导航可退出 | [真实浏览器回归](tests/system/channels_capabilities.mjs)，320／390／760／761／1024／1400px，保留布局测量证据 |
| 首屏主题与折叠偏好、存储不可用、脚本下载失败回退 | [首屏 UT](web/appearance.test.mjs) 与 channels 的 page-transitions 场景 |
| 同级标题、模型窄列、跨模块折叠、慢脚本前后布局、通知未保存反馈 | channels 的 page-transitions 场景检查实际几何与持久化，保留截图 |
| 普通及高级配置修改后旧连接提示失效，重新测试恢复新反馈 | channels 的 capability-setup 场景 |
| 外观与头像动效只在偏好设置管理，系统减少动效变化保留用户选择 | 同一浏览器场景验证持久化、系统偏好往返及侧栏控件边界 |
| 下拉可搜索／键盘操作，确认取消零提交、确认只提交一次 | 同一浏览器模块及现有账户集成场景 |

公共控件的复用边界也属于自动门禁：`npm run check --prefix web` 在检查构建资源前运行 `controls-policy.mjs`，扫描新增和已有页面，拦截页面直接操纵 Popover、私建 listbox／option，以及在共享样式外另写选择器／开关皮肤。普通原生 select、switch 消费者、容器布局及既有语言回退标记仍可使用；提示等其他公共浮层允许在共享组件层实现。

`npm test --prefix web` 验证门禁的正反例，以及选项过滤、禁用项、空列表、方向键循环、Home／End 和过滤后的活动项边界。`make test` 包含这些检查；现有 core `channels` 门禁实际调用 Task 计划页场景，验证手动保存、触摸、原生回退、冲突草稿及保存不提前派发任务。

代码变更沿用 [PR CI](.github/workflows/ci.yml)：`make test lint` 运行单元／组件与资源一致性检查，`make test-e2e` 的 core 集合包含 `channels`，运行上述浏览器场景并保留失败截图与测量。纯文档变更沿用文档校验，不新增重复 CI 或发布步骤。

自动回归约束已确认的行为和布局关系。信息密度、说明是否冗余和整体视觉协调由评审结合本规范及桌面／移动端截图核对；截图是评审证据，不把生成了截图等同于像素视觉测试通过。新增规则时同步更新对应层级的回归，不锁死 CSS 类名、整页 HTML 或无关的绝对坐标。

模型连接页只出现一次所选账户，目录模型的图片能力默认由元数据确定，不提供图片开关。模型连接、加载与选用按钮共用次要按钮样式，各操作组保持明确间距，不显示技术性选用说明或许可证页脚。共享字标和界面字体采用同源版本化预加载与私有缓存，原生切页时避免字体闪回。

Chat 输入框在聚焦时只增强原有圆角边界，使用内侧描边保留键盘焦点，不增加外围白框。

### Chat 消息呈现

Ghost 回复左对齐，使用主题中的灰色气泡；用户消息右对齐，背景与发送按钮同为 #B84F2F，文字使用白色。正文支持安全 Markdown，artifact 链接走认证下载，Mermaid 按需渲染并在失败时保留源码。桌面输入字号 14px，移动端保持 16px 以避免输入放大。请求处理中只显示三点动效，状态名称保留为辅助标签，系统减少动态效果时显示静态点；不将接纳回执误称为已读。Computer 与 Worker 入口固定在视口，滚动末端不跟随页面移动。

能力的 Advanced 优先提供 YAML 文件导入、示例下载及简短指引；文本编辑器折叠为审核和修改用途。浏览器标签页 Ghost 使用现有图集导出的黑／白图标，跟随实际页面主题。


Intelligent UI uses OpenIntelligentUI native component contracts, SVG helpers and form controls as the foundation, with Steward typography, neutral surfaces, restrained chart colors and compact spacing. Normal interactive answers do not repeat a Text version control; text fallback appears on failure or without JavaScript. Action groups use explicit gaps and wrap on narrow screens. Generated HTML/SVG diagrams, calculators and simulations follow generateSandboxedUi with compact answer layouts, explicit theme tokens and bounded content sizing. Native UI and the final written reply remain separate bubbles. Standard conversation Markdown is independent of this component contract.

Intelligent UI operations live in a compact options menu. Native tables, charts, typed forms and cards share state and control styling; fullscreen, dialogs and pinned views preserve the same content instance. The page contains content, controls and necessary state only, with no feature introductions or implementation explanations.

全局基础组件采用 shadcn/ui 的可组合结构和 Base UI 的按钮、搜索选择器及菜单交互。组件映射现有主题，保持页面字体、字号、导航、内容列、消息气泡和品牌色；只统一控件内部留白、边界、圆角与交互状态。原生表单与脚本增强共用皮肤，语言和能力自动保存、通知和任务计划显式保存的业务语义保持。
