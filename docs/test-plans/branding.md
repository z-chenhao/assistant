# Steward 品牌：测试方案

[功能说明](../features/branding.md) · [技术方案](../designs/branding.md) · [主索引](../feature-map.md)

## 范围与证据

F-041 覆盖项目身份和实际 Web 操作，不包含真实模型语义效果评估。使用隔离数据根与受控 peer；真实 macOS Guest 的平台兼容性须在对应平台验证，Linux 软件检查不代表其完成。

## 需求场景矩阵

| 需求 ID | 场景 | 可观察结果 | 已有入口 | 覆盖缺口 |
| --- | --- | --- | --- | --- |
| F-041 | 构建、安装、默认根和部署覆盖 | Steward 产品名、Ghost Agent 名；`steward` 命令、`STEWARD_*`、独立凭据服务、数据根与事件命名空间一致，固定依赖可构建 | `make test lint`、foundation 系统门禁 | 原生 macOS 包需平台验证 |
| F-041 | 登录、首次设置与欢迎页 | 新标识、同源图片字体、模型未就绪禁止发送 | channels 浏览器驱动 | 审美由所有者审阅截图 |
| F-041 | 延迟 bundle、输入、附件、失败提交、live 更新 | 保留节点、草稿、焦点和 source_id，无自动重发 | channels 浏览器驱动 | 无 |
| F-041 | 桌面/移动任务、设置、Trace 与 Context | 共享视觉、原功能可操作、无外部资源请求；基础样式不覆盖页面密度，移动抽屉导航完整可见、页面无横向溢出 | channels 与 local-provider 门禁 | 审美由所有者审阅截图 |
| F-041 | 模型详情与记录筛选 | 模型名和相同 ID 只显示一次，标题与状态对齐，320px 下详情不溢出，数据库筛选字段与提交按钮底边对齐 | channels 浏览器驱动 | 用截图审查视觉密度与阅读节奏 |
| F-041 | 中文与英文混排、减少动态效果、发送未确认 | 中文回退可读、输入至少 16px；减少动画后状态文字仍可见，失败保留草稿与附件 | channels 浏览器驱动及隔离实际界面检查 | macOS 优先苹方，其他平台有同源思源黑体回退；非跨平台像素一致承诺 |
| F-041 | Main/Worker、工具、审批、取消、恢复 | 新身份内冻结与重放契约有效 | 九项 core 门禁、operations、相关 race 检查 | 不改写冻结记录，不提供其他身份的兼容入口 |

设置精简由 channels 的 page-transitions 场景覆盖：1400／390px × 明暗主题核对标题与字段对齐、分组无外框、能力目录的名称／状态与详情帮助可达；正常 Ready 只保留辅助文字。原有 Preferences 中英文／触摸／键盘／自动保存／原生回退矩阵继续运行。Tasks 保留当前执行记录／任务配置表格和既有过滤、分页及详情路线，不恢复旧卡片布局。

## 已有入口与完成判定

使用[产品测试入口](product.md)；`npm run build --prefix web` 更新嵌入资源，`npm run check --prefix web` 验证生成资源一致。命令与截图的执行报告保存在现有 evidence 流程，不新增 runner。源码名称检查使用 `rg`；不将旧 Git 历史视作当前产品资源。

界面验证须核对 Chat 的 Computer/Worker 真数据读取、失败重试、互斥展开与草稿保护、主题持久化、两级导航焦点恢复、完整帧头像暂停、轻微漂浮与静态首帧回退。界面验证使用隔离数据根，不在个人工作空间发送测试请求。

`channels_browser.mjs` 比较 Chat 头像在悬停前、问候动画期间和移开指针后的尺寸，并验证问候沿用同一个画布。

界面语言回归：`go test ./internal/interaction/web` 的 locale 测试覆盖浏览器中文仍默认英文、显式中文及切回英文、无效选择不改 Cookie、现有认证/CSRF 边界、实时片段语言与 ETag，以及用户内容、原始参数和审批冻结标识不变。静态界面文案必须有中文条目；真实浏览器核对保存/刷新/跨页、动态控件和移动排版。

`tests/system/channels_preferences.mjs` 由现有 channels 驱动的 preferences 阶段执行。可用 `python3 tests/system/verify_channels.py --binary bin/steward --evidence evidence/preferences/report.json --preferences-only` 单独运行；重跑使用新的 evidence 目录。覆盖 1400/390/320px × 中英文 × 明暗主题的闭合和展开、值宽度、44px 触摸目标、右缘对齐及无布局偏移；检查键盘浏览／确认／取消、帮助与列表互斥、触摸、定位翻转和缩放。鼠标帮助使用未聚焦、未点击的 1px 连续移动路径，逐点确认提示可见并记录坐标；覆盖全部布局组合、保存拒绝反馈出现和窄容器自然换行，避免直接跳到提示掩盖触发器间隙。真实 POST 验证保存、Cookie 读回、刷新及跨页；受控故障验证单请求锁、明确拒绝与重试、认证拒绝、断网、15 秒超时、错误确认、响应损坏但 Cookie 已应用和 Cookie 未保留时不假报成功。回归返回、确定性的 persisted pageshow、无 JS／初始化失败／无 Popover 的原生路径，以及相邻设置和通知表单。CSS 200% 缩放不替代 Owner 在 Mac mini 上的原生缩放与 VoiceOver 实测。JSON／原生响应、非法值、Cookie 属性、认证与 CSRF 由 locale 测试补充。


公共控件复用门禁通过 `npm run check --prefix web` 在构建资源检查前执行，正反例与过滤／键盘状态 UT 通过 `npm test --prefix web` 执行；两者均属于现有 `make test`／`Ubuntu checks`。新页面私建下拉运行逻辑、模板选项列表或开关皮肤时应失败并报告文件位置，原生表单与容器布局应通过。

Task 计划页由 `verify_channels.py` 的 `task_control_components` 实际调用现有 schedule 浏览器阶段，结果写入 `task-controls-ui/schedule-browser.json`。覆盖 1400／720／320px、搜索与空结果、多个选择器互斥、依赖字段、开关键盘和触摸、显式保存与刷新、无效时间修正后重试、原生提交、陈旧版本草稿，以及保存配置不提前派发任务。

公共控件在 Preferences、能力配置、Task 计划及 Dev Trace 筛选中检查搜索／空结果、方向键／Home／End 浏览、确认、Escape／Tab 取消、跨控件互斥、44px 触控与无脚本原生提交。普通布尔开关保留原有 checked、disabled、表单值和保存时机；能力与模型声明、计划启用以及外观动效沿用现有真实浏览器场景。

渠道浏览器的 setup 阶段检查模型同意说明在桌面与窄屏无需聚焦即可阅读、选用按钮保留关联描述，以及语言帮助的 Escape 关闭与焦点保持。native-trace 阶段检查 720px 筛选控件的 16px 字号与 44px 高度、树中每项状态的可读文字；capability-setup 阶段检查 720px 普通文本框与 Advanced YAML 字号、输入边界色和 320px 折叠入口的触控高度。

精修验证沿用渠道浏览器与 Web 模块测试：四项导航、切页后标题焦点、最新消息入口、动态 composer 留白、真实待审批计数到卡片、320/390px 布局、44px 手机控件、黑白完整帧播放与暂停。Source Han 子集逐包验证 SHA-256/cmap，全集覆盖保留30926字符；不以 macOS 苹方的截图冒充非 macOS 冷加载性能测量。

全项目身份检查包括 CLI 帮助与 TUI、OAuth loopback 回调、固定工具命名空间、新执行身份提示、安装归档和链接。通过既有 provider-bridge、Go context/prompt、startup、Web、foundation 与 TUI 回归验证；提示字节哈希仅随已审查的身份文案改变而更新，冻结记录和 Child 作用域断言继续生效。

品牌资源一致性：`make test-browser` 同时运行 `web/artwork/icons.mjs --check`，校验源/atlas 哈希及 README/favicon PNG 与正式首帧的像素一致性。Go Web 测试验证资源路由；文档检查覆盖 HTML picture/img 的本地资源引用。


Preferences 的头像动效开关由 channels 的 `default_capability_settings` 通过真实浏览器验证：关闭后返回 Chat，头像停在首帧且漂浮暂停；跨页／刷新保持关闭，重新打开恢复偏好。独立 reduced-motion 浏览器上下文验证开关关闭、不可用并显示原因。侧栏不再包含宣传语、主题按钮或暂停按钮，外观与动效仅从偏好设置调整。


系统减少动效还验证运行时往返切换：解除系统限制恢复用户原有选择；用户主动关闭后，再次开启／解除系统减少动效仍保持关闭，不覆盖浏览器保存的偏好。

跨页稳定性由 core channels 的 `page_transitions` 调用现有浏览器驱动：中英文、明暗主题、320／390／760／761／780／800／820／1024／1400px 检查模型名、操作、同级标题及无溢出；折叠后跨模块并刷新保持状态。拦住真实 app.js 下载，比较导航、设置组和正文增强前后几何；下载失败验证原生回退。通知编辑验证旧保存提示失效、草稿不自动落盘及显式提交持久化。证据写入 `page-transitions-ui/page-transitions.json` 和截图。`web/appearance.test.mjs` 验证明暗优先级、桌面／手机折叠、不可用存储和下载失败；这些检查沿用 `make test lint`／`make test-e2e` 及必需的 `Ubuntu checks`。

channels 的 components 浏览器场景同时验证 dark/light 切换后的 favicon 路径；实际资源像素仍由 icons.mjs --check 验证。


全 Web 文案与问号帮助：`tests/system/channels_help.mjs` 由维护中的 Apps 浏览器门禁执行，覆盖 Apps 无冗余副标题／部署入口、连接可用性在操作处表达、原有 Preferences 连续 hover 路径、通知选择器与定时开关的可访问名称、阅读帮助不修改表单、任务容器内浮层、动态替换、200% 缩放、触摸切换及无 JavaScript 回退。原 Preferences 完整矩阵继续由 channels 验证；Root 因果边界指引改为主动打开帮助后核对。实际错误、权限、截断、保存及授权确认仍按各功能验收。
