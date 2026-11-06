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
  trace-agent-light: "#2965a2"
  trace-agent-dark: "#88b7e5"
  trace-generation-light: "#7845a6"
  trace-generation-dark: "#c6a2f2"
  trace-tool-light: "#9a571b"
  trace-tool-dark: "#e9b579"
  trace-http-light: "#226f75"
  trace-http-dark: "#81c7ce"
  trace-retriever-light: "#33734d"
  trace-retriever-dark: "#8fc6a1"
  syntax-key-light: "#245db8"
  syntax-key-dark: "#8eb8f7"
  syntax-string-light: "#27734a"
  syntax-string-dark: "#8bd3aa"
  syntax-number-light: "#8c4b0b"
  syntax-number-dark: "#e9b066"
  syntax-literal-light: "#914796"
  syntax-literal-dark: "#df9ce6"
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
  button: "8px"
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

浅色与深色共用相同的语义。主操作使用高对比填充，普通操作使用低强调填充或细边框。状态同时使用文字，颜色提供辅助识别。内容分隔线、输入边界与键盘焦点各自使用对应的颜色。实际色值维护在 [共享样式](web/src/foundation.css)。

Chat 发送按钮是单色主操作的例外：明暗主题均使用 `#B84F2F` 橙色背景和白色图标，样式限定在消息输入区。

## Typography

操作页依次使用页面标题、区块标题、常规界面和辅助文字四档。在根字号为 16px 时分别为 **22、16、14 和 13px**。表单标签、状态、帮助文案应按角色选档。窄屏普通文本输入和下拉控件使用 16px。Chat 欢迎语和回复采用阅读排版。字标使用 Michroma，代码使用 JetBrains Mono。


## 设计原则

- 导航层级表达用户所在的位置，同一层级始终承担同一种职责。
- 常驻区域只放经常使用、跨页面需要的入口；个人偏好集中在 Settings。
- 页面优先呈现可操作的设置项、真实状态和必要反馈，详细说明按需展开。
- 控件反映真实能力。未实现的功能不占分类，不显示没有作用的开关。
- 界面调整保留原有授权、版本校验、失败反馈和数据边界。

## 全局组件契约

本节约束整个 Web 宿主：Chat、Tasks、Developer、Settings、弹窗和 Intelligent UI 使用同一套基础组件，不能按页面保留不同的下拉、按钮或输入框体系。功能页面负责组合组件、布局和业务保存，不拥有第二套基础控件实现。组件替换本身不改变现有字体、字号、导航、内容列、消息气泡、品牌色及保存语义；这些变化需要另有明确的产品要求。

### 来源与实现归属

- 基础控件采用 shadcn/ui 的可组合结构与 Base UI 行为，适配 Steward 自有主题。不直接引入组件库整套主题、字体或全局排版重置。
- React 消费者使用 `web/src/components/ui/` 的封装；按钮归 [button.tsx](web/src/components/ui/button.tsx)，字段和结果卡片归 [answer-controls.tsx](web/src/components/ui/answer-controls.tsx)，图表基础归 [chart.tsx](web/src/components/ui/chart.tsx)。新增公共能力先补入所属共享组件，不在功能页复制实现。
- 宿主下拉统一使用 [select-view.tsx](web/src/components/ui/select-view.tsx)：少量选项采用 Base UI Select，超过 12 个非空选项才采用带搜索的 Combobox。数量是当前可调整的产品默认值；两项语言选择不得出现搜索框或为搜索预留空白。弹层按触发器与内容紧凑定宽，不给短列表强加大面板。遵循 [Base UI 的原生选用准则](https://base-ui.com/react/components/select#usage-guidelines)，不自建筛选或键盘状态机。菜单统一使用 [action-menu.tsx](web/src/components/ui/action-menu.tsx)。不得保留旧手写下拉状态机，也不得在页面直接拼装另一套浮层、选项导航和焦点管理。
- 服务端原生 select 经 [select.ts](web/src/components/ui/select.ts) 适配，React 表单经 [native-select.tsx](web/src/components/ui/native-select.tsx) 接入同一选择器。原生值、字段名、校验和无脚本回退属于表单契约，不构成第二套增强控件。
- 基础控件皮肤统一归 [controls.css](web/src/components/ui/controls.css)，颜色和字体使用 [foundation.css](web/src/foundation.css) 的语义变量。页面不得通过局部 CSS 重定义下拉或开关皮肤；其他控件变化也应落在共享的命名变体，不能用页面覆盖产生外观分叉。
- 保留采用源码和打包依赖的许可证；来源、版本及协议边界记录在所属设计方案，不添加到产品页面介绍文案中。

### 外观与状态

基础按钮、输入框和选择器使用 8px 圆角，内容容器使用 12px 圆角；桌面标准控件最小高度为 36px；Developer Traces 和 Memory 使用共享 compact 密度，桌面控件高 32px、界面字 13px、表格字 12px、辅助字 11–12px、单行高 36px。两种密度保留同一组件和字体，窄屏操作目标至少 44px。既有发送按钮、图标按钮等使用各自共享变体，不因接入组件库改动品牌造型。操作组有明确间距，窄屏可换行，不能挤压或叠放。

同一操作角色在各页面、明暗主题、普通／hover／focus／disabled／loading／error 状态下保持同一语义。主操作、次操作和危险操作用共享变体区分；不能因页面或连接状态不同而随意切换黑白按钮。颜色跟随主题变量，不给新组件单独设一套配色。

仅靠脚本提供交互的控件，在对应事件绑定和状态初始化完成前保持禁用，不能先呈现为可点击却吞掉用户操作。脚本下载失败时仍须保留可用的原生页面与表单路线。

输入框聚焦只加强已有圆角边界或内侧描边，不增加外层方形白框。保留可见键盘焦点、标签、禁用及无效状态；不能为去掉白框而移除无障碍焦点。下拉和菜单在弹窗内、200% 缩放及窄屏下仍可操作，搜索、空结果、确认、取消和焦点恢复使用同一规则。

### Intelligent UI 的组合边界

OpenIntelligentUI 的原生组件协议是基础，保留上游组件名称和参数语义。HTML、SVG 流程图／架构图／图表、计算器、模拟教学与交互控件通过标准 `generateSandboxedUi` 或具名扩展接入，不重新建立另一套渲染协议或组件主题。扩展与宿主交互仍通过受控协议，业务动作复用原 Main／Tool 授权与持久化管线。

原生组件使用同一基础控件，沙箱内容使用宿主共享主题和控件样式。正常呈现不常驻重复的 Text version；失败或无 JavaScript 时保留真实文字回退。组件与后续文字回复按顺序分成气泡，操作收进紧凑菜单。页面只出现内容、控件与必要状态，不新增组件介绍、实现细节或重复技术文案。

### 修改与验收

修改前先确认组件归属及本节约束；存在能力缺口时补充共享组件或具名扩展。任何明确获准的规则变化，应在同一变更中更新本文件及所属回归，不能只在对话、局部 CSS 或 `AGENTS.md` 中另立规范。

现有控件归属门禁随 `npm run check --prefix web` 执行；行为使用真实浏览器场景验证，视觉结合深浅主题、桌面和窄屏截图评审。具体检查与证明范围见下文[自动回归与评审分工](#自动回归与评审分工)，不通过检查文案是否存在代替行为验证。

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
| Settings | Preferences、Notifications、Resources、Apps、Capabilities |
| Dev | Traces、Memory |
| Task | 当前没有独立分类，不展开空二级栏 |

二级分类以当前分类为单位选中，详情继承所属分类。一级模块选中态与二级分类选中态同时保留。分类搜索只筛选二级项目，不影响一级入口。

Task、Dev 或其他模块未来增加内部分类时复用同一结构。列表筛选、数据库集合和详情标签属于内容区，不提升为一级模块或额外导航层。

### 内容区

内容位于导航右侧，按实际可用宽度响应布局。正文标题、设置分组和主操作遵循相同的对齐线。Settings 使用居中窄列；Task 和 Dev 可按内容宽度展示列表、详情和检查面板。

## 尺寸与响应式

桌面一级栏宽 224px、二级栏宽 216px，由共享变量控制；模块没有二级分类时，内容只避让一级栏。Settings 内容容器最大宽度为 680px，包含左右留白；Apps 沿用已确认的双列应用目录，最大宽度为 1048px，窄屏按单列展示。两级栏共用折叠／展开行为，桌面折叠为 64px 图标栏，保留四个模块入口、选中态和展开按钮；同一浏览器的折叠偏好跨模块、分类和刷新保持一致，并在首屏样式前应用。

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

### 功能优先与按需帮助

整个 Web（Chat、Tasks、Developer、Settings、弹窗与动态视图）只常驻功能、操作、实际内容和必要状态。同一页面的同一操作只保留一个明确入口；目录筛选选择后直接生效，不再要求重复点击筛选按钮，无脚本时保留原生提交。筛选的只读导航与配置的保存语义分别遵循各自契约。删除不帮助当前选择或操作的宣传、页面介绍、重复提示、实现解释及泛泛的空状态建议；不把冗余说明全部改成问号。

确实必要的操作指引、参数含义、行为边界与技术解读，收进对应字段、操作或标题旁的问号。统一复用 Preferences 的共享帮助组件，皮肤和定位归 `components/ui/`；不为各页面另写浮层。鼠标 hover、键盘聚焦可阅读，点击或触摸可展开／收起，Escape 或外部点击关闭；浮层在窄屏和缩放下保持可读，无 JavaScript 时保留原生 disclosure。动态更新后仍能使用帮助。

应用用途、任务目标／结果和诊断数据属于实际内容。真实的未配置、等待、失败、权限受限、数据截断、未保存及结果未知等状态必须清楚表达；服务部署状态不在使用页面常驻展示，不重复堆叠状态标签和同义按钮；实际操作暂不可用时，在对应操作处说明。将长指引拆为必要状态／操作和按需帮助。授权范围、费用／数据外送同意及不可逆操作的确认信息在决定处完整呈现，不藏入提示。


同级设置页保留可见的页面标题和一致的对齐线，Resources 的 Models 是标题下的区块。脚本加载前后应保持导航、设置分组及正文位置稳定；脚本专属设置预留位置，下载失败时恢复原生表单和导航。

连接检查反馈只对应检查时的字段；修改普通字段或导入高级 YAML 后清除旧反馈，自动校验并保存；不显示额外测试和保存按钮。通知偏好修改后撤下此前保存提示，明确显示未保存，仍须显式保存。

页面标题和字段名称优先使用用户能理解的名称。帮助、YAML、连接详情和诊断放进 Advanced；影响当前决定的限制直接显示。

启用状态与连接检查结果分别表达：默认启用显示“已启用（默认）”；准备中、不可用、需要检查和重启后生效使用各自的真实状态。Memory 与检索共用主入口，语义搜索属于其高级子设置。

外观和头像动效统一位于 Preferences。关闭头像动效后展示静态首帧，仍保留真实状态文字。系统开启“减少动态效果”时优先遵循系统偏好，关闭动效并解释开关为何不可用；动效不能成为获取状态的唯一方式。

品牌字标、色彩、字体与图片沿用现有[品牌资源](docs/designs/branding.md)，品牌宣传语可用于 README 和产品介绍，不占据常驻工作台导航。

## 数据列表页契约

本节是数据密集型列表的共同默认规则，当前消费者为 Developer 的 Traces 与 Memory。页面通过已有共享组件组合信息，不创建独立的控件皮肤；其他设置与阅读页面继续使用各自规格。

| 部分 | 规范 |
| --- | --- |
| 信息顺序 | 页面标题、搜索、列表操作／视图切换、筛选与结果、底部结果数与分页；不在列表前放无助于定位记录的大块说明或指标卡。 |
| 搜索与操作 | 按真实能力提供 Search、Filters、Refresh。搜索范围与过滤条件由服务端负责；改变条件从第一页开始，刷新保留当前条件。 |
| 时间区间 | 用单个紧凑触发器概括当前范围；预设和自定义起止时间在展开后选择，不把两个日期输入挤在常驻工具栏中。 |
| 桌面密度 | 共享 compact 变体：标题 18px、控件文字 13px／高度 32px、表格文字 12px／单行约 36px；常规详情正文按内容使用阅读规格。 |
| 导航与筛选 | Developer 一级 176px、二级 168px；二级标题两侧留足空间，不能裁切标签。列表筛选栏 176px，可折叠；手机默认收起，折叠触发器不能覆盖标签或其他操作。 |
| 表格内容 | 优先展示识别记录和诊断所需的真实字段；长正文单行省略，完整内容在详情中读取，ID 与低频字段后置。统计覆盖完整筛选范围，不能把当前页条数称作总数。 |
| 横向与窄屏 | 宽表在自身区域横向滚动，不撑宽整个页面；手机普通输入文字至少 16px、操作目标至少 44px。桌面检查主内容与两级导航的接缝。 |
| 展开详情 | 默认使用共享非模态侧栏，桌面半屏、手机全屏；背景列表保留上下文，支持原生详情链接、前后记录、关闭、返回与焦点恢复。分栏／侧栏调宽只保存宽度偏好。 |
| 状态与颜色 | 有效、等待、错误、被替代、缺失等状态用文字明确表达，颜色辅助识别；正文与结果始终来自真实数据，不展示没有数据来源的次数、评分或空操作。 |
| 读取与回退 | 查看与过滤不产生业务写入、模型推理或 embedding。无 JavaScript 时保留表单、分页、完整详情与来源导航。诊断 JSON 不能通过格式化丢失重复字段、精度或原始字符表示。 |

列表详情的字段和过滤项由所属功能定义；本契约不开放通用数据库表、SQL 或内部持久化结构。对已有页面采用此模式时按功能逐步改动，不用本节授权无关页面重构。

### 回归落点

交互状态由 [共享组件 UT](web/developer-controls.test.mjs)验证：拖拽取消／提交、边界与键盘、销毁后监听清理、JSON 无损回退。几何关系由现有 [列表浏览器断言](tests/system/channels_controls.mjs)在真实 Chromium 中验证，Traces／Memory 场景共同调用：行高与字体、导航留白、不同图标、时间触发器、页面溢出和 Filters／视图标签重叠。权限、筛选口径、分页、来源和 Forget 继续由所属 Go 与真实存储／浏览器回归负责。

这些测试分别进入现有 CI 的 frontend、go 和 channels-web 通道，并受必需的 `Ubuntu checks` 聚合检查约束；不新增并行规范文件或独立视觉测试框架。截图仍需人工复核明暗主题、桌面与手机，自动几何断言不能代替视觉审查。

## Developer Trace 列表与详情

用户要求保留 Steward 自有组件、Inter 字体和品牌，布局及信息密度对齐其给定的 Langfuse 参考。Trace 使用共享 compact 规格：桌面标题 18px、控件 13px/32px、表格 12px/36px，明暗主题继续使用 Steward 色值；手机输入为 16px，操作目标至少 44px。此规格是共享控件的具名密度，其他页面维持标准规格。

时间范围和 Refresh 位于全局顶栏右侧，下面依次为 Tracing 标题、全宽搜索、表格操作栏。Developer 范围的一级/二级导航分别收窄为 176px/168px，其他模块保持原宽度；内容左侧是 176px 可收起的分组筛选栏，右侧为真实查询计数直方图、单行表格与底部分页。创建时间、类型、名称、Input、Output 各自成列，身份 ID 独立且默认隐藏；列显隐保留在当前浏览器。图表统计完整筛选范围，不随分页截断。

点击名称后打开右侧非模态预览面板，桌面默认 50vw（最多 1400px），背景列表保持可操作。面板头提供全页、前后 Trace、展开和关闭；下方左树右详情直接从顶部开始，不再先放大块指标头。树按真实父子关系展示可折叠分支与连续连接线，搜索已加载节点时保留匹配祖先并临时展开。详情外边缘与树/正文分栏均可鼠标拖拽、方向键调整，宽度偏好仅存当前浏览器；窄屏改为上下排列。节点头展示类型图标、完整 UTC 时间、真实耗时、状态和 Session/Run；下面的 Preview 按 Input → Output 排列，支持 Formatted/JSON、可折叠结构化 JSON 和复制。执行事实、指标、Provider timing 与元数据折叠在详情底部。手机默认收起列表筛选；预览占满视口，树与正文上下排列。原生表单与完整详情页保留无脚本路线。

表单浮层归 `web/src/components/ui/disclosure.tsx`，使用 Base UI Popover；预览外壳归 `web/src/components/ui/sheet.ts`，使用原生非模态 dialog，Escape 与焦点恢复由共享组件管理。分隔条行为归共享 `resize-handle.ts`，分栏归 `split-view.ts`，JSON DOM 阅读归 `read-only-json.ts`；原始 pre 始终保留，片段、不完整数据或不能精确表示的数字回退到 JSON。所有控件皮肤及 compact 规格归 controls.css，页面只组合数据、布局和导航。

Trace 采用语义配色：Agent 蓝、Generation 紫、Tool 橙、HTTP 青、Retriever 绿；成功/等待/错误分别复用绿色/琥珀色/红色状态。JSON 键、字符串、数字和布尔值分别使用蓝/绿/棕/紫，值仍以文字和类型表达，颜色不单独承担含义。输入保持中性，输出仅有低强度绿色底色；导航、页面与品牌继续使用黑白灰。语义色定义在 foundation.css，明暗主题分别提供可读色值，不创建按页面分叉的控件皮肤。

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

`npm test --prefix web` 验证门禁的正反例、首屏行为和交互状态持久化。选择器的搜索、禁用项、空结果、方向键、Home／End、确认与取消由 channels 的真实浏览器场景验证；core `ui-interactions` 还覆盖弹窗内控件、表单校验、卡片布局、导出和重启恢复。`make test` 包含前端检查；core `channels` 的 Task 计划页场景验证手动保存、触摸、原生回退、冲突草稿及保存不提前派发任务。

代码变更沿用 [PR CI](.github/workflows/ci.yml)：`make test lint` 运行单元／组件与资源一致性检查，`make test-e2e` 的 core 集合包含 `channels`，运行上述浏览器场景并保留失败截图与测量。纯文档变更沿用文档校验，不新增重复 CI 或发布步骤。

自动回归约束已确认的行为和布局关系。信息密度、说明是否冗余和整体视觉协调由评审结合本规范及桌面／移动端截图核对；截图是评审证据，不把生成了截图等同于像素视觉测试通过。新增规则时同步更新对应层级的回归，不锁死 CSS 类名、整页 HTML 或无关的绝对坐标。

模型连接页只出现一次所选账户，目录模型的图片能力默认由元数据确定，不提供图片开关。模型连接、加载与选用按钮共用次要按钮样式，各操作组保持明确间距，不显示技术性选用说明或许可证页脚。共享字标和界面字体采用同源版本化预加载与私有缓存，原生切页时避免字体闪回。

Chat 输入框在聚焦时只增强原有圆角边界，使用内侧描边保留键盘焦点，不增加外围白框。

### Chat 消息呈现

Ghost 回复左对齐，使用主题中的灰色气泡；用户消息右对齐，背景与发送按钮同为 #B84F2F，文字使用白色。正文支持安全 Markdown，artifact 链接走认证下载，Mermaid 按需渲染并在失败时保留源码。桌面输入字号 14px，移动端保持 16px 以避免输入放大。请求处理中只显示三点动效，状态名称保留为辅助标签，系统减少动态效果时显示静态点；不将接纳回执误称为已读。Computer 与 Worker 入口固定在视口，滚动末端不跟随页面移动。

能力的 Advanced 优先提供 YAML 文件导入、示例下载及简短指引；文本编辑器折叠为审核和修改用途。浏览器标签页 Ghost 使用现有图集导出的黑／白图标，跟随实际页面主题。


Intelligent UI uses OpenIntelligentUI native component contracts, SVG helpers and form controls as the foundation, with Steward typography, neutral surfaces, restrained chart colors and compact spacing. Normal interactive answers do not repeat a Text version control; text fallback appears on failure or without JavaScript. Action groups use explicit gaps and wrap on narrow screens. Generated HTML/SVG diagrams, calculators and simulations follow generateSandboxedUi with compact answer layouts, explicit theme tokens and bounded content sizing. Native UI and the final written reply remain separate bubbles. Standard conversation Markdown is independent of this component contract.

Intelligent UI operations live in a compact options menu. Native tables, charts, typed forms and cards share state and control styling; fullscreen, dialogs and pinned views preserve the same content instance. The page contains content, controls and necessary state only, with no feature introductions or implementation explanations.


## 应用目录与账户连接

Apps 展示应用目录与账户连接，不展示项目初始化、服务配置按钮或常驻部署提示；常规 Composio 部署自动读取默认环境凭据，无需重复填写引用；自定义项目配置集中在 Resources → App connection service，使用者的 Connect 流程不要求填写项目密钥或 auth-config ID。授权状态自动更新，手动刷新作为补充；额外账户才要求独立别名，重连只替换失效或未完成的绑定。目录按用途分类，不设置“热门”分组；采用较宽的双列列表，每项展示原始品牌 logo、名称、一行用途和“＋”连接／“…”管理入口；移动端为单列。标题沿用 Settings 的纵向位置与字号，横向跟随目录自身内容边界。左侧“已连接的应用”显示去重的账户应用与 logo；账户操作集中于应用详情对话框，关闭后恢复焦点，无 JavaScript 时进入原生详情页。Settings 中 Apps 使用四宫格应用图标，Resources 使用服务器图标，两个入口可直接区分。目录与账户视图共享侧栏选中态、原生表单回退及全局组件契约；搜索框、图标按钮与详情弹窗使用 controls.css 的具名共享变体，普通按钮和字段继承全局皮肤。断开连接复用共享确认对话框，默认聚焦取消，Escape 不提交。

Developer 主导航使用代码括号图标，Traces 使用波形，Memory 使用记忆图标，三者不得复用同一图形。

### Developer Memory 列表与详情

Memory 沿用 Traces 的紧凑列表、176px 过滤栏和窄化的两级导航。顶部内容搜索、过滤、刷新与独立维护记录切换构成主要操作；表格优先正文、层级、形成方式、状态、来源与创建时间，不展示通用数据库集合。Core 使用已有紫色类型标记，Recall 使用蓝色，状态沿用共享语义色。

点击记忆打开共享非模态半屏详情，左侧来源与替代关系、右侧正文与选中的原始来源；分栏使用同一可拖拽与键盘调宽组件，手机上下排列。记忆正文使用普通界面阅读字体，不装扮为 JSON。真实正文、不可用及被替代元数据状态由读取结果决定，无脚本时保留原生链接和表单。
