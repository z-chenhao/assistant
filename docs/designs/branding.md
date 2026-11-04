# Steward 品牌：技术方案

[功能说明](../features/branding.md) · [测试方案](../test-plans/branding.md) · [主索引](../feature-map.md)

Steward 是产品品牌，Ghost 是当前 Agent 的名称。当前代码与自托管资源共同定义品牌实现。

## 当前架构与流程

Go 继续提供认证、页面、原生表单与局部更新；React 只增强 Host-owned UISpec。`foundation.css` 定义 Demo 的配色和自托管字体；shell、workspace、conversation、observability、avatar 分别负责局部表现。`appearance.js` 为同源外部脚本，在 CSS 前应用本地明暗偏好，不放宽 CSP。

单侧栏仅 Chat、Tasks、Settings、Developer。Chat 顶部居中显示名称和动态头像，真实就绪/工作状态在头像下方同轴居中，以绿/蓝/琥珀/红/灰小圆点区分就绪、工作、需配置、模型异常与未知，保留明确文字和明暗主题对比，右上角 Computer/Worker 打开互斥只读详情面板，关闭/隐藏停止画面轮询；Task 保留独立列表与详情。面板不替换对话和 composer，后台更新不强行拉回阅读位置。

任务详情继续显示目标、审批、进展、关联交付、附件与历史；没有将技术执行细节挤入默认首页，也没有虚构多会话或文件索引。发送、附件、source_id、草稿、冻结审批哈希、分页及已有 live region ID 不变。

## 资源与动效

Michroma 字标、Inter UI、Source Serif 4 回复、Source Han Sans SC 中文回退和 JetBrains Mono 代码字体在 `/brand/` 同源提供，OFL 通知在 `/licenses.txt`。使用 Demo 已确认的黑白幽灵 PNG，身体、手臂、五官离线烘焙为完整无损 WebP 帧；运行时 Canvas2D 播放，身体不做整体缩放，不加载 WebGL。轻微整体现浮动与固定影子保留；暂停、reduced-motion、隐藏或离屏停止当前挥手。资源来源和完整生成提示保存在 `web/licenses/steward-artwork.md` 与 `web/artwork/manifest.json`。

README、favicon 与 Web 静态图均来自正式 256px atlas 的第 0 帧，通过 `node web/artwork/icons.mjs` 导出；`--check` 比较解码像素，防止手绘替代图漂移。两份无五官 PNG 是动画烘焙输入，位于 `web/artwork/source/`，不进入 Go embed 或公开路由。两种主题、两种帧尺寸均有运行时消费者，继续保留。品牌维护入口为当前 CSS、本方案及正式资源清单。

## 身份与边界

当前只有所有者使用开发环境，没有需要迁移的发布消费者。品牌贯穿表现层、启动器、配置、凭据服务与内部协议；不维护双命名和回退路径。

| 范围 | 当前标识 |
| --- | --- |
| 产品 / Agent | Steward / Ghost |
| 字标 / 标语 | Steward / Your personal AI steward. |
| Go 模块 | `github.com/z-chenhao/assistant`（保留既有 import 路径；仓库现为 `z-chenhao/steward`） |
| 命令入口 / 构建输出 | `cmd/steward` / `bin/steward` |
| 环境变量 / 默认数据根 | `STEWARD_*` / `~/.local/share/steward` |
| 安装目录 / 归档 | `lib/steward` / `steward-<platform>-<arch>.tar.gz` |
| 协议与凭据 | 使用各自已有的 Steward 命名空间与独立 SecretStore 服务 |

身份文本属于产品策略；冻结快照、审批、持久化、重放和工具权限仍是执行机制，语义不因命名而改变。新的执行使用 Ghost 身份提示；已有冻结记录不被重新解释或改写。品牌资源和 Web 控件仍为私有实现，没有新增公共设计 API、第二执行链路或通用品牌配置系统。

当前身份边界：只接受当前命令、环境变量、默认路径与命名空间，不提供旧配置发现、迁移工具、凭据读取回退或资源别名。不会主动删除应用数据或扩大凭据权限。选择直接统一身份，放弃兼容层，以免在没有实际消费者时长期维护两套概念。

## 界面语言

使用现有原生设置流程新增 `/settings/preferences` 与认证后的 `POST /settings/language`。只接收 `en`、`zh-CN`，以路径 `/`、SameSite Strict、有效期一年的 `steward-language` 浏览器 Cookie 持久化。未设置或无效时一律英文；不使用 Accept-Language。服务端预编译两份不可变模板，显式 `tr` 仅作用于应用自有界面文案；客户端从同一私有 JSON 文案表读取。没有运行时 DOM 扫描或外部翻译服务。实时片段和日期展示随语言变化；正文、标识、字段值、冻结哈希及机器协议不变。此方案优先支持无 JS 的原生切换，未引入通用翻译平台或账户级语言同步。

Preferences 使用局部 `.preference-group` 与 `.preference-row` 组织单层圆角设置组，左侧名称／帮助，右侧语言触发器；保留读屏 h1 和当前导航标签。默认窄屏同行，长内容或放大时按根目录 DESIGN.md 有序换行。颜色、字体来自既有 token，触摸操作高度至少 44px。列表只显示真实语言与当前项勾号，无自动保存说明或正常界面的 Save。样式限定本项，不修改相邻表单密度。

`preferences.ts` 渐进增强原生 form/select，初始化完成后隐藏原生控件与保存按钮；原生 select 仍是唯一提交值源。支持 Popover API 时使用顶层非模态 combobox/listbox，焦点留在触发器，活动项与选中项可区分；定位随滚动／resize／visual viewport 更新，空间不足翻转或内部滚动。缺少浮层能力时仍通过原生 select 的 change 自动保存；缺少脚本／fetch 或初始化失败时保留原生选择与显式保存。方向键浏览不提交，确认不同值才提交；Escape、Tab、外部操作取消浏览，Tab 不抢焦点。

同一 POST `/settings/language` 在显式 `Accept: application/json` 时返回包含已设置语言的 JSON 200；非法值返回 JSON 400 且不设置 Cookie。认证和同源／CSRF 仍在既有路由边界执行。普通原生表单沿用 303。增强端每页只允许一笔请求，无队列：忙碌时保留焦点并锁定选择，明确拒绝回到此前值，400 可显式重试；认证拒绝提供重新打开设置入口。15 秒超时、网络中断、5xx、错误类型或不匹配响应均视为结果未确认，先 GET 核对，不自动重发。

成功导航携带目标语言与焦点 fragment，服务端在 GET 读回 Cookie 后才显示成功；不一致则提示检查浏览器 Cookie 设置。整页使用服务端翻译，不扫描改写 DOM。成功／错误反馈显示在语言组内。普通进入页面不抢焦点；pagehide 使旧响应失效，BFcache 返回通过 GET 重新核对，不重放写入。

帮助默认提供原生 details/summary；`settings.ts` 就绪后替换为具有 aria-describedby 的按钮与文本 tooltip。悬停或聚焦显示，点击固定显示／关闭，移入仍可阅读；Escape 关闭直到再次进入或激活，外部操作关闭。帮助与语言列表互斥。无 JS 时保留原生展开，不提供 Escape 关闭。相邻设置继续沿用原有原生提交保护。
