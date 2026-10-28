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

### F-013 · 用户图片视觉输入与本地 OCR

**需求**：用户上传图片后，选定视觉模型能实际收到授权图片内容；纯文本模型也能显式识别图片文字并保留来源。

**当前实现**：Web 上传和 CLI `chat send-parts` 的显式用户图片进入冻结的 ImageRefs 投影；通过视觉 Probe 的选定模型会在实际供应商请求中收到授权缩图。Settings 可显式声明并测试图片输入能力。Main/Worker 默认获得 `builtin__image_ocr`，可用本地 Tesseract 提取授权图片中的文字、词框和引擎置信分数，完整结果保存为关联原图的 Artifact。

**边界**：支持 PNG/JPEG，单图最多 8 MiB、约 3355 万像素（32 × 1024²）。工作集最多保留 8 张不同图片，激进上下文重试最多 4 张；省略的像素保留引用与说明。文本模型保留引用并明确说明未看到图片，不读取或发送像素。格式不支持、图片损坏或越界在供应商调用前失败，不把文件名当作图像理解。真实模型的理解质量独立评测。

**验收目标**：通过真实消息入口上传图片，在最终供应商请求中验证图片存在，并验证无权限/不支持模型的行为；纯文本模型通过真实 OCR 工具收到文字，已提交 OCR 结果在重启后复用。

<a id="f-014"></a>

### F-014 · 音视频与文档内容处理

**需求**：用户交付音视频或文档后，Agent 能获取适合任务的内容和来源。

**当前实现**：附件运输与保存已有；消息投影主要提供引用，未形成统一解析/转写链路。

**边界与剩余工作**：按实际用户场景明确支持格式，补解析、转写或媒体工具及错误反馈；不以合成文件证明理解质量。

**验收目标**：逐格式验证可获取的内容、原始文件关联、失败反馈和实际任务输出；质量由独立评测衡量。

## 如何使用

Web 的附件选择器可随文字提交文件。CLI 用 `erii artifact save NAME MEDIA_TYPE` 从 stdin 保存，随后用 `erii artifact describe REF` 或 `erii artifact open REF` 检查和读取。

在 Settings 添加模型时，按需勾选 **Enable image input (verified when tested)**，执行 Test connection，通过后再 Select model。Saved models 显示 Image input verified；未启用时显示 Text only。随后随消息上传 PNG/JPEG 即可。CLI 的模型 JSON 输入使用 `capabilities.image_input: true`，图片 ContentPart 必须为 `{"type":"image","artifact_ref":"artifact:..."}`；普通 `file` 不自动加载像素。

新 Run 才启用这条用户图片投影；升级前的活动 Run 沿用其冻结表示。正常停止旧进程后统一升级，不混跑不同投影版本。音视频和复杂文档仍由 F-014 跟踪，文件运输不等于内容理解。

### 图片文字识别（F-013 增强）

上传 PNG/JPEG 后可要求 Agent「识别图片中的文字」。模型通过 `builtin__image_ocr({artifactRef})` 显式调用本地 OCR，Direct Tool 和 Code Mode 使用同一执行路径。视觉模型也可使用此工具；OCR 不改变所选模型，不调用另一家视觉服务。支持英文、简体中文和繁体中文语言包，以返回的 `languages` 为本次实际使用的语言。

先安装运行依赖，再启动 Erii：

```sh
# Ubuntu / Debian，简体中文和英文
sudo apt-get install tesseract-ocr tesseract-ocr-eng tesseract-ocr-chi-sim bubblewrap
# macOS
brew install tesseract tesseract-lang
erii doctor
```

繁体中文在 Debian/Ubuntu 另装 `tesseract-ocr-chi-tra`。自定义语言目录可在启动前设置绝对路径 `TESSDATA_PREFIX`。只使用已安装的 eng/chi_sim/chi_tra；缺少某语言包不会推定能够识别该语言。Doctor 报告依赖与语言包，缺少 OCR 不阻止普通对话，调用会明确失败 `PROVIDER_UNAVAILABLE`。Linux 还需要启用非特权 user namespaces；macOS 使用 Seatbelt。没有无沙箱执行回退。

结果标记 `method: "ocr"`，包含 `source_ref`、原图尺寸、文字、词框与 `document_ref`。坐标以原图左上角为原点，单位为像素；`confidence` 是 0–100 引擎分数，不是正确率概率。模型预览最多 2048 字节文字、16 个词框，完整 JSON 可用 Artifact 工具按范围读取。识别最多保留 4096 个词和 128 KiB 文字，截断会标记 `truncated`；空白图可成功返回空文字。

OCR 能提供截图中的文字证据，不能证明布局、颜色、图表关系或场景理解。处理原图，沿用 8 MiB/32 × 1024² 像素限制；调用总期限为 30 秒。没有自动发布、向用户发送文件或写入长期 Memory。文字会作为普通 Tool 结果进入当前模型上下文，依旧属于不可信的图片内容。

引擎及语言包身份随新 Run 冻结。安装、升级或变更语言目录后重启并建立新 Run；旧冻结调用不会换引擎继续执行。升级前完成 Main 与需保留的旧 Run，Worker Task 可暂停后由新 Main append 接续。已经提交的结果复用原始文字与 Artifact，不重新识别。

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
through `artifact save`, then passed by reference. A verified image-capable profile
receives explicitly typed user image parts through the scoped pixel envelope on
new Runs; text-only profiles receive reference
hints and an explicit no-pixels notice. Other files remain references and can
request bounded text reads. Transport does not establish model understanding quality. [M6 fixtures](../test-plans/product.md) exercise
image/source-region and media handling with explicit Fakes; [Computer Use](computer-use.md)
provides scoped file transfer. Those paths have separate proof limits.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/artifacts-media.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/artifacts-media.md)。
