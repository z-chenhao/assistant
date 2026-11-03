# Steward 品牌：技术方案

[功能说明](../features/branding.md) · [测试方案](../test-plans/branding.md) · [主索引](../feature-map.md)

核对日期：2026-11-03。Steward 是产品品牌，Ghost 是当前 Agent 的名称。当前代码与自托管资源共同定义品牌实现。

## 当前架构与流程

Go 继续提供认证、页面、原生表单与局部更新；React 只增强 Host-owned UISpec。`foundation.css` 定义 Demo 的配色和自托管字体；shell、workspace、conversation、observability、avatar 分别负责局部表现。`appearance.js` 为同源外部脚本，在 CSS 前应用本地明暗偏好，不放宽 CSP。

单侧栏仅 Chat、Tasks、Settings、Developer。Chat 顶部居中两行显示名称和动态头像，右上角 Computer/Worker 打开互斥只读详情面板，关闭/隐藏停止画面轮询；Task 保留独立列表与详情。面板不替换对话和 composer，后台更新不强行拉回阅读位置。

任务详情继续显示目标、审批、进展、关联交付、附件与历史；没有将技术执行细节挤入默认首页，也没有虚构多会话或文件索引。发送、附件、source_id、草稿、冻结审批哈希、分页及已有 live region ID 不变。

## 资源与动效

Michroma 字标、Inter UI、Source Serif 4 回复、Source Han Sans SC 中文回退和 JetBrains Mono 代码字体在 `/brand/` 同源提供，OFL 通知在 `/licenses.txt`。使用 Demo 已确认的黑白幽灵 PNG，身体、手臂、五官离线烘焙为完整无损 WebP 帧；运行时 Canvas2D 播放，身体不做整体缩放，不加载 WebGL。轻微整体现浮动与固定影子保留；暂停、reduced-motion、隐藏或离屏停止当前挥手。资源来源和完整生成提示保存在 `web/licenses/steward-artwork.md` 及相邻 provenance JSON。

## 身份与边界

当前只有所有者使用开发环境，没有需要迁移的发布消费者。品牌贯穿表现层、启动器、配置、凭据服务与内部协议；不维护双命名和回退路径。

| 范围 | 当前标识 |
| --- | --- |
| 产品 / Agent | Steward / Ghost |
| 字标 / 标语 | Steward / Your personal AI steward. |
| Go 模块 | `github.com/z-chenhao/assistant`（与仓库地址一致） |
| 命令入口 / 构建输出 | `cmd/steward` / `bin/steward` |
| 环境变量 / 默认数据根 | `STEWARD_*` / `~/.local/share/steward` |
| 安装目录 / 归档 | `lib/steward` / `steward-<platform>-<arch>.tar.gz` |
| 协议与凭据 | 使用各自已有的 Steward 命名空间与独立 SecretStore 服务 |

身份文本属于产品策略；冻结快照、审批、持久化、重放和工具权限仍是执行机制，语义不因命名而改变。新的执行使用 Ghost 身份提示；已有冻结记录不被重新解释或改写。品牌资源和 Web 控件仍为私有实现，没有新增公共设计 API、第二执行链路或通用品牌配置系统。

本次是开发阶段的破坏性身份重置：只接受当前命令、环境变量、默认路径与命名空间，不提供旧配置发现、迁移工具、凭据读取回退或资源别名。不会主动删除应用数据或扩大凭据权限。选择直接统一身份，放弃兼容层，以免在没有实际消费者时长期维护两套概念。

## Changelog

| 日期 | 关联需求 | 决策 | 依据及取舍 |
| --- | --- | --- | --- |
| 2026-11-03 | F-041 | 全项目统一 Steward / Ghost；移除不使用的品牌图片、字体和资源路由；新提示同步身份 | 所有者确认没有现有用户并要求彻底清理。命名同步到 CLI、配置、安装、内部协议与测试；不增加兼容或迁移分支。 |

### 2026-11-02 · 界面语言

使用现有原生设置流程新增 `/settings/preferences` 与认证后的 `POST /settings/language`。只接收 `en`、`zh-CN`，以路径 `/`、SameSite Strict、有效期一年的 `steward-language` 浏览器 Cookie 持久化。未设置或无效时一律英文；不使用 Accept-Language。服务端预编译两份不可变模板，显式 `tr` 仅作用于应用自有界面文案；客户端从同一私有 JSON 文案表读取。没有运行时 DOM 扫描或外部翻译服务。实时片段和日期展示随语言变化；正文、标识、字段值、冻结哈希及机器协议不变。此方案优先支持无 JS 的原生切换，未引入通用翻译平台或账户级语言同步。
