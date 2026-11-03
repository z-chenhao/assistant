# 开源项目治理审查 · 2026-11-03

[工程方案](../designs/engineering.md) · [治理规则](../../GOVERNANCE.md) · [上一轮代码审查](2026-11-03-engineering.md)

基线为 `ffab679` 及上一轮已验证的工作区重构。用户随后明确要求覆盖品牌过渡产物、README、CI/CD、远端分支与无效 PR，并选择 Apache-2.0。此次不改变执行协议、审批、schema 或冻结运行时字节。

## 发现与处置

| 优先级 | 位置 / 证据 | 改进 |
| --- | --- | --- |
| P1 | `go.mod` 使用 Go 1.26.2；govulncheck v1.9.0 报告 28 项标准库可达漏洞，最高修复版本为 1.26.10。 | 升级同系列安全补丁到 1.26.10，统一当前开发文档。四份 npm 锁文件审计均无已知漏洞。 |
| P1 | README 明确未选择项目许可证；原生包不携带项目及编译链接的 Go 依赖 notices。 | 经用户选择加入 Apache-2.0、NOTICE、第三方说明。安装器按实际链接包收集模块及工具链许可证；release 安装保留这些文件。 |
| P2 | README/favicon 指向独立简化 SVG；真实 UI 使用完整 Ghost atlas；两份 1,953,532 字节的无五官烘焙源图也被 embed。 | 移除 SVG；源图移至 `web/artwork/source/`，退出运行时 embed。两张静态 PNG 共 99,526 字节，从正式 atlas 首帧导出，README 明暗图和 favicon 共用它们。 |
| P2 | 品牌 provenance 分散在四份 JSON，包含未采用的实验；Web BRIEF/DESIGN 与当前 CSS/功能设计重复。 | 保留选定输入及四个实际使用的 atlas，在 `web/artwork/manifest.json` 统一维护；合并字体元数据，删除废弃实验与重复草稿。运行时主题、尺寸、降级和暂停行为保持。 |
| P2 | `scripts/check_docs.py` 未检查 HTML picture/img；损坏 README 图片引用的回归先失败。 | 增加 HTML 本地资源检查；真实 Chromium 校验静态图与正式首帧的解码像素。 |
| P2 | README 仅有两条开发命令、旧仓库 URL；贡献入口只有链接，无首次贡献路径和 GitHub 问题模板。 | 补齐安装前提、使用方式、平台边界、命令和目录地图；加入贡献、支持、行为准则、治理、Issue 表单、CODEOWNERS、EditorConfig。保留旧 Go import 路径。 |
| P2 | PR CI 显式检出 head，未验证 GitHub 合并结果；无独立持续依赖审计与 workflow lint。 | 改为默认合并结果 checkout，保留 `Ubuntu checks` 状态；NUL 分隔路径分类、actionlint、Dependabot 和独立依赖审计。 |
| P2 | 原生平台检查主要发生在打包流程，draft 创建前未复验下载后的 checksum。 | 扩展回归增加 macOS CLI/startup/credential adapter/offline VM 契约；draft 创建复验 checksums，发布仍需显式授权。 |
| P2 | 实际四平台构建暴露 Linux 打包缺少 Bubblewrap，两个 doctor 契约失败；Actions 日志报告 setup-node 的 Node 20 已弃用。 | Linux 打包补齐必要依赖；核对官方兼容性后固定 setup-node v7.1.0、upload-artifact v7.0.2、download-artifact v8.0.2 的完整 SHA。手动与 tag 构建均重新下载产物并验证 checksum。 |
| P2 | race 的记录器短锁用例与 SQLite 压力分片竞争 CPU；首次 CI 仅输出分组失败，具体错误需下载 artifact。 | 在压力分片前独立执行 Bridge 和记录器；保持时限、断言与完整清单，失败摘要直接进入日志。将审查中已使用的固定版 Staticcheck 纳入常规 lint，保留逐行兼容例外。 |
| P2 | 远端 33 个分支，22 个 head 与已合并 PR 精确相符；唯一开放 PR #113 仍采用已被 #116/#117 取代的 Collie 视觉。 | 保存完整 Git bundle；带 SHA lease 原子删除 22 个已合并分支。关闭 #113，并删除其被取代的主题分支；保留独有的旧实验工作供审核。 |
| 权限限制 | 私密漏洞报告关闭，合并后自动删分支关闭；修改接口均返回 HTTP 403。 | 不声称已启用。SECURITY.md 明确报告入口不可用时不公开漏洞细节；需仓库管理员在 Settings 中启用这两项。 |

## 结构边界

Go 领域、adapter、组合根和权威事务边界继续保留；不为目录整齐拆开原子事务。`web/src`、`web/artwork/source` 和 Go embedded assets 分别承担界面源码、可重建素材和运行时产物。当前设计归功能/设计/测试文档；有日期的审查记录归 `docs/reviews/`。源图和四个动态使用的 atlas 不是死代码，字体许可、生产保护与兼容 import 不删除。

## 远端证据与保留项

- `main` 已有 active 的 Protect main ruleset：禁止删除/非快进、必须经 PR、要求最新基线及 `Ubuntu checks`。本轮保留这些保护；未降低 gate。
- [#113](https://github.com/z-chenhao/steward/pull/113) 的关闭依据是已合并 [#116](https://github.com/z-chenhao/steward/pull/116)、[#117](https://github.com/z-chenhao/steward/pull/117) 明确采用另一套正式品牌；不是因为 PR 年龄或冲突。
- 仍保留 9 个未合并实验分支：5 个 `ERII-141-*` profile 分支，以及 `agent/builder/erii-{30,227,254,326}`。它们包含独有提交，部分修改运行时或测试；尚未证明每项改动已被吸收，不按名称或年龄删除。
- 删除前的 33 refs、完整历史和 SHA 保存在忽略目录 `evidence/open-source-review/remote-before.bundle`；`branch-audit.json`、`deleted-branches.json` 和两个 cleanup 日志记录具体动作。

## 验证记录

原始输出保存在 `evidence/open-source-review/`，包括初次失败和修复后结果；不提交运行日志、个人数据或历史评测产物。

- HTML 图片检测回归先失败，修正后通过；正式静态图已从当前 atlas 导出。
- Go 1.26.10 的 `make audit` 通过：无可达 Go 漏洞，四份 npm 锁文件均为 0 项；仅余 GO-2026-5932：`golang.org/x/crypto/openpgp` 不受维护；本项目未导入该包，使用的是 SSH 等其他包。没有为隐藏模块提示删除实际使用的密码库。
- Go 1.26.10 下 `make test lint` 通过；包含 HTML 图片回归、正式首帧像素验证、工作流 actionlint 以及全部既有软件检查。
- 首次 [native packages](https://github.com/z-chenhao/steward/actions/runs/41631074356) 的 Linux amd64/arm64 因缺少 Bubblewrap 失败。修正后 [四平台打包及下载校验](https://github.com/z-chenhao/steward/actions/runs/41631572727) 全部通过；Linux/macOS 分别携带 24/25 个实际链接 Go 模块的 notices，手动运行没有创建 release。
- [核心 CI · 93ec10a](https://github.com/z-chenhao/steward/actions/runs/41634844523) 与 [依赖审计](https://github.com/z-chenhao/steward/actions/runs/41634844320) 通过。前一提交的核心 artifact 已逐项核对 startup、main、tasks、children、tools、code-recovery、local-provider、account、channels **9/9**；安装、浏览器、许可证保留及清理断言均通过。
- [首次扩展运行](https://github.com/z-chenhao/steward/actions/runs/41631568765) 在 capacity 报模型元数据持久化失败；race 的诊断短锁测试耗时 210 ms，超过既有 100 ms 写入预算，随后取消其他分片。原用例独立 `go test -race -count=30` 通过。调整测试调度后，本地 `make test-race` 完整通过 **66 包、285 个 SQLite 顶层用例**，源码稳定、清单完整、进程组全部回收；没有扩大生产预算或放宽断言。
- 容量定位使用临时诊断构建：本地常规 CPU 三轮、单核三轮，以及 [GitHub runner 三轮](https://github.com/z-chenhao/steward/actions/runs/41635203837) 均通过，共 **27,000 个子任务**。每轮仍为 2,994 完成、6 个预设业务失败，无遗留工作租约。诊断代码未进入交付分支；临时引用已在 Git bundle 备份后清理。
- [修正后的完整扩展与 race 运行 · 93ec10a](https://github.com/z-chenhao/steward/actions/runs/41634836812) 保留其执行结果与原始 artifacts；[PR #118](https://github.com/z-chenhao/steward/pull/118) 记录最终验收结论与最新门禁。后续日志展示、静态检查入口及本报告的整理不改变产品代码，不能将不同提交的测试记录混为一次运行。

## 尚未关闭的事项

1. **容量失败的根因尚未确认。** 九轮诊断没有重现首次远端持久化错误；不能宣称已修复，也不据此删除有界写入和恢复保护。后续若再现，应保留具体失败位置、上下文状态及存储等待证据，再决定优化；生产时限或架构调整须另行确认。
2. **管理员设置仍受权限限制。** 私密漏洞报告及合并后自动删除分支的写接口均返回 403，需管理员在仓库 Settings 启用；已有主分支保护保持。
3. **真实集成验证仍有边界。** 原生 macOS 契约已运行，但真实 Keychain round-trip、Guest、个人账号和付费模型未做实机验收。9 个独有实验分支继续保留，直到能逐项证明已吸收或由维护者作出取舍。
