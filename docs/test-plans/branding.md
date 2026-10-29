# Collie 品牌：测试方案

[功能说明](../features/branding.md) · [技术方案](../designs/branding.md) · [主索引](../feature-map.md)

## 范围与证据

F-041 覆盖项目身份和实际 Web 操作，不包含真实模型语义效果评估。使用隔离数据根与受控 peer；真实 macOS Keychain/Guest 的平台兼容性须在对应平台验证，Linux 软件检查不代表其完成。

## 需求场景矩阵

| 需求 ID | 场景 | 可观察结果 | 已有入口 | 覆盖缺口 |
| --- | --- | --- | --- | --- |
| F-041 | 构建、安装、默认根和部署覆盖 | 只有 Collie 命令与新目录，固定依赖可构建 | `make test lint`、foundation 系统门禁 | 原生 macOS 包需平台验证 |
| F-041 | 登录、首次设置与欢迎页 | 新标识、同源图片字体、模型未就绪禁止发送 | channels 浏览器驱动 | 审美由所有者审阅截图 |
| F-041 | 延迟 bundle、输入、附件、失败提交、live 更新 | 保留节点、草稿、焦点和 source_id，无自动重发 | channels 浏览器驱动 | 无 |
| F-041 | 桌面/移动任务、设置、Trace 与 Context | 共享视觉、原功能可操作、无外部资源请求；原生/React 控件共享样式、Trace 使用固定 SVG 图标；基础样式不覆盖页面密度，320px 窄屏导航完整可见、页面无横向溢出 | channels 与 local-provider 门禁 | 审美由所有者审阅截图 |
| F-041 | 中文与英文混排、减少动态效果、发送未确认 | 中文回退可读、输入至少 16px；减少动画后状态文字仍可见，失败保留草稿与附件；附件 picker 的键盘焦点可见，移动筛选控件字号至少 16px，普通页面正文直接呈现 | channels 浏览器驱动及隔离实际界面检查 | 中文字体随操作系统供应，非跨平台像素一致承诺 |
| F-041 | Main/Worker、工具、审批、取消、恢复 | 新身份内冻结与重放契约有效 | 九项 core 门禁、operations、相关 race 检查 | 不验证旧身份数据迁移 |

## 已有入口与完成判定

使用[产品测试入口](product.md)；`npm run build --prefix web` 更新嵌入资源，`npm run check --prefix web` 验证生成资源一致。命令与截图的执行报告保存在现有 evidence 流程，不新增 runner。源码名称检查使用 `rg`；不将旧 Git 历史视作当前产品资源。

## 低饱和复查的浏览器检查

通过真实 owner 页面检查欢迎、模型/能力设置、任务、审批和 Trace；同时检查鼠标/键盘焦点、320px 与 390px 的输入字号和溢出、reduced motion、无 JavaScript 表单及延迟 bundle 下草稿/附件保持。色值计算记录在本轮 evidence，单独报告测量配色对；不把低饱和等同于低文字对比，也不把静态截图代替真实操作验证。继续使用既有 channels 入口，不为视觉调整增加 UT、CI gate 或评测脚本。
