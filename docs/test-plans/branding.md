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
| F-041 | 桌面/移动任务、设置、Trace 与 Context | 共享视觉、原功能可操作、无外部资源请求 | channels 与 local-provider 门禁 | 无 |
| F-041 | Main/Worker、工具、审批、取消、恢复 | 新身份内冻结与重放契约有效 | 九项 core 门禁、operations、相关 race 检查 | 不验证旧身份数据迁移 |

## 已有入口与完成判定

使用[产品测试入口](product.md)；`npm run build --prefix web` 更新嵌入资源，`npm run check --prefix web` 验证生成资源一致。命令与截图的执行报告保存在现有 evidence 流程，不新增 runner。源码名称检查使用 `rg`；不将旧 Git 历史视作当前产品资源。
