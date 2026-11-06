# Developer Memory：只读记忆投影

[功能与需求](../features/memory.md) · [测试方案](../test-plans/memory.md) · [主索引](../feature-map.md)

## 需求、使用者与边界

本机所有者需要浏览真实记忆及来源；Web 是现有所有者读取权的消费者。原 Database 通用元数据浏览器及其专用三库连接池已移除，领域存储、Agent Tool、维护队列、召回与加密格式不变。没有数据库迁移或新的公开持久化模型。

## 读取机制

[Memory 私有投影](../../internal/memory/inspection.go) 定义有界查询、记忆项、来源与维护回执；[Memory Store 读取](../../internal/memory/sqlite/inspection.go) 使用现有连接池和 Vault。[Runs 所有者读取](../../internal/agent/sqlite/memory_inspection.go) 从配置派生当前用户／Agent 与隐私 epoch、floor，调用存储并校验每条来源。App 只负责组合，Web 不能指定作用域。

Store 在只读事务中读取作用域内、未删除、隐私 floor 之后的记录及替代关系，关闭事务后访问 SecretStore。只解密当前有效正文；被替代正文始终不可见。reserved sealed: 表示不能降级成普通文本。搜索采用 Unicode 文本的小写字面包含匹配；当前本机语料在内存中扫描，5 秒读取预算内失败则整体失败，不返回截断的成功搜索。此实现适合当前个人记忆规模，大规模索引优化未稳定为新协议。

分页按创建时间／ID 降序，游标绑定所有者、隐私 floor 和搜索／过滤／视图条件；游标只表示位置，不提供授权。输出前重读元数据指纹，检查成员、密文表示、来源和替代关系是否改变，再重新检查所有者及隐私 epoch。遗忘、纠正、加密保护或切换配置发生在读取期间，不能返回旧结果。明文只存在于请求内存和所有者响应，不写入日志、工具结果、事件或持久搜索索引。

## 来源和维护记录

Conversation／Task 引用复用既有证据可见性校验；来源正文复用有界 evidence 读取，来源按每页最多 20 条显式续读，且请求的 source 必须属于该记忆当前可见来源页。feedback 仅映射到当前 Agent 的确切 builtin__memory_manage ToolInvocation，或已保存的本机控制请求，不推断关联。替代链接仅指向当前查询边界内仍可见的未删除元数据。

维护页只投影当前 Agent 的 remember／forget 回执，不解密 payload；applied／rejected／pending 来自已保存的时间字段，错误保持维护管线的脱敏代码。记录分页有界，来源 Run 另受隐私 floor 约束。

## 页面与共享组件

列表与 Traces 共用 compact 密度、过滤栏、表格和响应式布局；详情直接消费共享 [sheet](../../web/src/components/ui/sheet.ts)、[split-view](../../web/src/components/ui/split-view.ts) 和 [resize-handle](../../web/src/components/ui/resize-handle.ts)。Memory 页面只拥有筛选与选择状态，不复制控件皮肤。颜色沿用已有类型／状态语义；层级与状态始终有文字。浏览器只持久化宽度比例，不保存记忆正文、来源正文或搜索结果。

列表链接渐进增强为非模态半屏详情，支持来源选择、前后条目、展开、独立页面、返回与焦点恢复。原生导航和无脚本表单保持有效。旧 Database 页面与 API 直接移除，不保留泛型存储检查接口；新接口是私有所有者读投影，尚未作为第三方稳定契约。

## 取舍

复用领域读取和现有 Vault，避免另建解密或维护路径。首次实现选择清晰的全量候选扫描与输出前复查，避免引入落盘明文索引。没有通用 SQL、任意表投影、正文编辑、分类推断、召回统计或数据写入入口。后续大规模检索或可编辑管理需要独立证据与生命周期设计。
