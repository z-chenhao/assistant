# Steward 品牌：测试方案

[功能说明](../features/branding.md) · [技术方案](../designs/branding.md) · [主索引](../feature-map.md)

## 范围与证据

F-041 覆盖项目身份和实际 Web 操作，不包含真实模型语义效果评估。使用隔离数据根与受控 peer；真实 macOS Keychain/Guest 的平台兼容性须在对应平台验证，Linux 软件检查不代表其完成。

## 需求场景矩阵

| 需求 ID | 场景 | 可观察结果 | 已有入口 | 覆盖缺口 |
| --- | --- | --- | --- | --- |
| F-041 | 构建、安装、默认根和部署覆盖 | Steward 产品名、Ghost Agent 名；`steward` 命令、`STEWARD_*`、独立凭据服务、数据根与事件命名空间一致，固定依赖可构建 | `make test lint`、foundation 系统门禁 | 原生 macOS 包需平台验证 |
| F-041 | 登录、首次设置与欢迎页 | 新标识、同源图片字体、模型未就绪禁止发送 | channels 浏览器驱动 | 审美由所有者审阅截图 |
| F-041 | 延迟 bundle、输入、附件、失败提交、live 更新 | 保留节点、草稿、焦点和 source_id，无自动重发 | channels 浏览器驱动 | 无 |
| F-041 | 桌面/移动任务、设置、Trace 与 Context | 共享视觉、原功能可操作、无外部资源请求；基础样式不覆盖页面密度，移动抽屉导航完整可见、页面无横向溢出 | channels 与 local-provider 门禁 | 审美由所有者审阅截图 |
| F-041 | 中文与英文混排、减少动态效果、发送未确认 | 中文回退可读、输入至少 16px；减少动画后状态文字仍可见，失败保留草稿与附件 | channels 浏览器驱动及隔离实际界面检查 | macOS 优先苹方，其他平台有同源思源黑体回退；非跨平台像素一致承诺 |
| F-041 | Main/Worker、工具、审批、取消、恢复 | 新身份内冻结与重放契约有效 | 九项 core 门禁、operations、相关 race 检查 | 不改写冻结记录，不提供其他身份的兼容入口 |

## 已有入口与完成判定

使用[产品测试入口](product.md)；`npm run build --prefix web` 更新嵌入资源，`npm run check --prefix web` 验证生成资源一致。命令与截图的执行报告保存在现有 evidence 流程，不新增 runner。源码名称检查使用 `rg`；不将旧 Git 历史视作当前产品资源。

界面验证须核对 Chat 的 Computer/Worker 真数据读取、失败重试、互斥展开与草稿保护、主题持久化、两级导航焦点恢复、完整帧头像暂停、轻微漂浮与静态首帧回退。界面验证使用隔离数据根，不在个人工作空间发送测试请求。

界面语言回归：`go test ./internal/interaction/web` 的 locale 测试覆盖浏览器中文仍默认英文、显式中文及切回英文、无效选择不改 Cookie、现有认证/CSRF 边界、实时片段语言与 ETag，以及用户内容、原始参数和审批冻结标识不变。静态界面文案必须有中文条目；真实浏览器核对保存/刷新/跨页、动态控件和移动排版。


精修验证沿用渠道浏览器与 Web 模块测试：四项导航、切页后标题焦点、最新消息入口、动态 composer 留白、真实待审批计数到卡片、320/390px 布局、44px 手机控件、黑白完整帧播放与暂停。Source Han 子集逐包验证 SHA-256/cmap，全集覆盖保留30926字符；不以 macOS 苹方的截图冒充非 macOS 冷加载性能测量。

全项目身份检查包括 CLI 帮助与 TUI、OAuth loopback 回调、固定工具命名空间、新执行身份提示、安装归档和链接。通过既有 provider-bridge、Go context/prompt、startup、Web、foundation 与 TUI 回归验证；提示字节哈希仅随已审查的身份文案改变而更新，冻结记录和 Child 作用域断言继续生效。

品牌资源一致性：`make test-browser` 同时运行 `web/artwork/icons.mjs --check`，校验源/atlas 哈希及 README/favicon PNG 与正式首帧的像素一致性。Go Web 测试验证资源路由；文档检查覆盖 HTML picture/img 的本地资源引用。


Preferences 的头像动效开关由 channels 的 `default_capability_settings` 通过真实浏览器验证：关闭后返回 Chat，头像停在首帧且漂浮暂停；跨页／刷新保持关闭，重新打开恢复偏好。独立 reduced-motion 浏览器上下文验证开关关闭、不可用并显示原因。侧栏不再包含宣传语、主题按钮或暂停按钮，外观与动效仅从偏好设置调整。


系统减少动效还验证运行时往返切换：解除系统限制恢复用户原有选择；用户主动关闭后，再次开启／解除系统减少动效仍保持关闭，不覆盖浏览器保存的偏好。
