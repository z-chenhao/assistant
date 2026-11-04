# Developer Database 数据检查

从本机工作台检查已保存的元数据，沿明确关系定位执行、消息、记忆与文件。

[需求主索引](../feature-map.md) · [技术方案](../designs/database.md) · [测试方案](../test-plans/database.md)

## 需求与验收

<a id="f-033"></a>

### F-033 · Web Database 检查入口

**需求**：用户能检查执行、对话、记忆和文件元数据，理解数据关系。

**当前实现**：Developer → Database 提供 Executions、Messages、Memory 和 Artifacts 四类只读投影。可分页浏览或按确切引用查找，再沿关联进入相关记录、Task 或 Observability。无需模型推理、embedding 或记忆解密；页面支持中文、英文、移动浏览和无 JavaScript 操作。

**边界**：入口使用本机工作台现有认证和启用的 Web 绑定，只展示当前所有者及 Agent 可见的固定元数据。消息正文、Memory 明文或密文、凭据、主机路径及执行请求快照不进入此投影。页面不接受 SQL、数据库路径或写入操作。安装配置仍需能正常启动工作台。

**验收目标**：从真实用户入口形成记录后，验证四类分页、精确查找、关联跳转、作用域、敏感内容控制及重启读取；浏览前后执行与领域记录保持不变。

## 如何使用

打开工作台的 **Developer → Database**，选择记录类型。默认每页 20 条，使用 **Older records** 继续读取。查找指定对象时，在对应类型中输入已知的 `run:...`、`conversation:...`、`memory:...` 或 `artifact:...` 引用，再选择 **Find record**。空白查找返回该类型列表；Reset 清除查找条件。

展开 Metadata 查看字段，点击关系链接查看相应记录。执行记录的 **Inspect execution** 打开既有 Observability；Task 关联打开原任务详情。记录内容不因界面语言变化而改写。

| 类型 | 展示的元数据 | 关联 |
| --- | --- | --- |
| Executions | 角色、状态、深度、Task 代次、版本和起止时间 | 原 Task、父执行、根执行及 Observability |
| Messages | 角色、来源类型、顺序号、创建时间和内容部分数 | 已接纳输入或已提交回复的实际执行、显式附件 |
| Memory | 层级、作用域类型、存储形式、来源数量、版本、删除时间和替代状态 | 替代谱系、可见消息/Task/Worker 来源、确切记忆工具调用所属执行 |
| Artifacts | 名称、媒体类型、字节数、SHA-256、可见范围、版本、保存期限和引用数量 | 生成执行/Task，以及明确引用它的消息、Task、执行或工具所属执行 |

已删除和被替代对象保留元数据供本机所有者审计，其旧内容不会被读取或自动送入模型。文件的 `stored` 表示元数据未标记删除；保存期限是存储值，实际内容读取继续由 Artifact 的保留与访问规则裁决。

每条记录最多展示 16 个已保存的来源、附件或持有者关联，另保留直接父级、生成者或替代关系。计数包含原记录保存的全部条目，可见链接可能更少。未知、缺失或越权目标不会成为可用链接；关系不会自动递归展开，也不授予访问权。

### 只读 JSON 入口

已通过工作台认证的本机客户端可读取：

```text
GET /api/developer/database?collection=executions
GET /api/developer/database?collection=messages&limit=20
GET /api/developer/database?collection=memories&ref=memory:OPAQUE_REF
```

`collection` 允许 `executions/messages/memories/artifacts`，省略时为 executions。`limit` 允许 1–50，默认 20。返回 `collection/items/next`；每条记录包含 `ref/status/created_at/fields/links`。有 next 时将其作为相同类型的 `cursor` 请求下一页。Messages 按保存的顺序号倒序，其他类型按创建时间与引用倒序；相同位置使用引用稳定区分。

`ref` 与 cursor 不同时使用。游标绑定类型和当前 Agent，不是权限凭据。错误或重复参数返回 400，不存在和越权引用均返回 404；数据源不可读返回 503。失败不伪装成空列表，响应不带底层 SQL、存储位置或原始错误。

## 实现与验证依据

实现与取舍见[技术方案](../designs/database.md)，验收入口及证明范围见[测试方案](../test-plans/database.md)。此功能是本机元数据检查，不代表模型对数据内容的理解质量。
