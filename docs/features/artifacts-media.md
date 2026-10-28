# 文件产物与多模态输入

管理附件和交付物，并补齐文件内容进入模型的链路。

[需求主索引](../feature-map.md) · [技术方案](../designs/artifacts-media.md) · [测试方案](../test-plans/artifacts-media.md)

## 功能说明

用户能上传、保存、搜索、下载交付物，引用具有访问边界。

用户上传图片后，选定视觉模型能实际收到授权图片内容。

用户交付音视频或文档后，Agent 能获取适合任务的内容和来源。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-012"></a>

### F-012 · 可重新打开的文件产物

**需求**：用户能上传、保存、搜索、下载交付物，引用具有访问边界。

**当前实现**：Artifact 存储、哈希校验、发布选择、Task 引用与渠道文件入口已实现。

**边界与剩余工作**：保存文件不等于理解内容；发布仅选择当前 Run 最终输出，不授予外部发送权限。

**验收目标**：验证读回字节与哈希、作用域、保留期、并发保存、删除和重启恢复。

<a id="f-013"></a>

### F-013 · 用户图片到模型视觉输入

**需求**：用户上传图片后，选定视觉模型能实际收到授权图片内容。

**当前实现**：工具结果/Code Mode 图片有显式 ImageRefs、缩图和 Bridge 传图；用户消息图片目前主要是引用提示。

**边界与剩余工作**：接通用户图片与视觉能力设置，保留授权、大小及数量约束。

**验收目标**：通过真实消息入口上传图片，在最终供应商请求中验证图片存在，并验证无权限/不支持模型的行为。

<a id="f-014"></a>

### F-014 · 音视频与文档内容处理

**需求**：用户交付音视频或文档后，Agent 能获取适合任务的内容和来源。

**当前实现**：附件运输与保存已有；消息投影主要提供引用，未形成统一解析/转写链路。

**边界与剩余工作**：按实际用户场景明确支持格式，补解析、转写或媒体工具及错误反馈；不以合成文件证明理解质量。

**验收目标**：逐格式验证可获取的内容、原始文件关联、失败反馈和实际任务输出；质量由独立评测衡量。

## 如何使用

Web 的附件选择器可随文字提交文件。CLI 用 `erii artifact save NAME MEDIA_TYPE` 从 stdin 保存，随后用 `erii artifact describe REF` 或 `erii artifact open REF` 检查和读取。

请把当前附件能力理解为可靠存储和传递。图片、音视频和复杂文档的自动理解仍受 F-013/F-014 的链路与模型能力限制。

## 详细操作参考

- [文件操作详情](#artifacts-artifact-storage-and-operations-erii-14)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="artifacts-artifact-storage-and-operations-erii-14"></a>

## Artifact storage and operations (ERII-14)

Artifacts reopen through opaque `artifact:` references. Only the Store resolves
references to storage keys. Files live beneath `app.data_root/artifacts`;
metadata, references, publications and tombstones live in state.db schema v11.
The forward migration preserves schema v10 data and frozen Run capabilities.

This installation has one local user principal (`local`), tied to its data root
and OS account. This is not a multi-user server or a bearer-reference API. The
Host constructs user/Agent/Task/Run scope. The CLI acts as that local user;
models never choose scope. Cross-user and cross-Agent access fail. Worker
outputs are immutable, Run-owned and readable within the same Task. Main
inputs require explicit grants; Task create/append validate and pin accessible
refs. Same Task visibility does not inject any file contents into context.

<a id="artifacts-cli-boundary"></a>

### CLI boundary

```sh
printf 'report text' | bin/erii artifact save report.md text/markdown
bin/erii artifact describe ARTIFACT_REF
bin/erii artifact open ARTIFACT_REF > ./download.md
bin/erii artifact delete ARTIFACT_REF VERSION
bin/erii artifact cleanup
bin/erii artifact cleanup apply
```

`save` streams stdin and accepts a display name and MIME type. It accepts no
source path or URL. `open` verifies size/SHA-256 before emitting bytes to stdout.
`describe` returns metadata without paths, storage keys, owner IDs or download
URLs. The local user may inspect expired/deleted metadata for audit; content
reads and all Worker operations reject expired/deleted refs. Active/paused Task
ownership or explicit Task attachment suspends expiry; the frozen per-object
retention duration runs for at least that duration after the last associated
Task ends. Descriptors omit expiry while a Task protects the object. Delete checks
version and references, then tombstones metadata. Recovery unlinks deleted
bytes on the next startup; cleanup apply also unlinks after commit. Audit
metadata remains. Referenced objects cannot be deleted.

`chat send-parts [REQUEST_KEY]` reads a JSON array on stdin, bounded to 16 KiB.
Text uses `{ "type": "text", "text": "..." }`; image/audio/video/file parts use
`{ "type": "file", "artifact_ref": "..." }` with their semantic type. The Host
validates and pins each input before accepting it. Large files are uploaded
through `artifact save`, then passed by reference. The current text Provider
receives reference hints and can request bounded text reads. Reference transport
does not establish visual understanding. [M6 fixtures](../test-plans/product.md) exercise
image/source-region and media handling with explicit Fakes; [Computer Use](computer-use.md)
provides scoped file transfer. Those paths have separate proof limits.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/artifacts-media.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/artifacts-media.md)。
