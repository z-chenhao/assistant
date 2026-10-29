# 自然语言对话与共享循环

统一处理用户对话、工具续轮、追加输入和最终回复。

[需求主索引](../feature-map.md) · [技术方案](../designs/conversation.md) · [测试方案](../test-plans/conversation.md)

## 功能说明

用户能连续对话、追问和纠正要求，短时工作在 Main 中完成。

本文描述用户行为和范围；逐项完成状态以主索引为准。当前能力依据代码静态盘点，配置可用性与真实服务/模型质量需分别验证。

## 需求与验收

<a id="f-003"></a>

### F-003 · 可恢复的 Main 对话

**需求**：用户能连续对话、追问和纠正要求，短时工作在 Main 中完成。

**当前实现**：Actor 管理输入与执行权；共享 Loop 在 model/tools/reply/settle 阶段推进，保存模型交换和回复。

**边界与剩余工作**：本需求不包含实时 Web 推送或高质量语义压缩，分别由 F-019、F-011 跟踪。

**验收目标**：验证输入去重、Steering、工具续轮、单 Main 所有权与中断后的回复恢复。

## 如何使用

启动 `collie` 后直接输入需求。命令行可将文本经 stdin 交给 `collie chat send REQUEST_KEY`，再由 `collie chat work` 推进；`collie chat history` 读取语义对话。

用户通过自然语言追加背景或纠正要求。耗时或需要独立 Worker 的工作由 Main 显式建立 Task，参见任务功能。

## 详细操作参考

- [对话 CLI 与恢复细节](#main-loop-main-loop-cli-and-recovery)
- [安装与部署详情](installation.md)

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="contract-1-产品定位"></a>

### 1. 产品定位

Collie 是一个通用私人助理，目标能力对标 Muse、dots、Manus Cue、Grok Bot、Hermes Agent，并参考 Pi 1.2.0 的 Agent Harness 设计。

<a id="contract-11-当前阶段"></a>

#### 1.1 当前阶段

- 一期只做好一个 Agent。
- 一期采用本地部署，不依赖云端运行时。
- 首次启动必须完成模型连接与主模型选择；一期同时支持本地 OpenAI-compatible 模型端点和 OpenAI “Sign in with ChatGPT” 订阅登录。
- Agent 核心由宿主 Go Host 执行；需要桌面能力时，在与宿主隔离的 macOS Guest 中安装和操作应用，由 Computer Use 或配置的工具访问。
- 一期采用模块化单体；模块边界和协议提前抽象，以便未来拆分为云服务。

<a id="contract-12-长期形态"></a>

#### 1.2 长期形态

- 一个用户可以创建多个 Agent。
- 多个 Agent 彼此平等，均由用户直接调用，不设置默认的上下级关系。
- 本地与云端使用相同的核心协议和接口，仅替换底层 Provider 与部署组件。

<a id="contract-13-用户交互原则"></a>

#### 1.3 用户交互原则

- 用户与 Agent 的主要交互永远是自然语言对话，而不是任务管理页面。
- Web 用于用户在电脑前交代和查看工作。
- TUI 用于研发、测试与调试。
- 飞书是未来便携下达任务和接收通知的主要渠道，默认通知到飞书，但可配置。
- 用户只感知 Agent 是“空闲”还是“忙碌”，以及当前有哪些持续进行中的工作。
- 用户通过对话追加指令、暂停、恢复或取消工作；进度页面只提供读取能力。
- 原始模型调用、工具调用和工具返回轨迹仅对研发调试开放。

<a id="main-loop-main-loop-cli-and-recovery"></a>

## Main Loop CLI and recovery

Main and Worker use one shared Agent Loop. Set up and explicitly select a probed local
or ChatGPT account profile using [the provider commands](model-connections.md).
Then use the same configured data root for all commands:

```sh
printf '%s' 'Hello Collie' | bin/collie -config local.yaml chat send greeting-1
bin/collie -config local.yaml chat work
bin/collie -config local.yaml chat history
```

`chat send [REQUEST_KEY]` reads up to 16 KiB of text from stdin. It returns a
durable acceptance receipt, not a model answer. Omit the key to generate a fresh
one; retain a supplied key when retrying an uncertain ingress response. The same
key and content return the original receipt, including after Run completion.
Changed content under that key fails closed. Unconfigured/unavailable profiles
close ingress. No implicit model or Fake is selected.

`chat work` drains accepted work in the foreground. Run it in one terminal while
other terminals submit steering. It waits for a live ownership lease to expire
or its owner to finish; it never runs a second Main concurrently. When currently
accepted work ends it exits. Invoke it again for later inputs. The default
no-command startup opens the [TUI](../test-plans/product.md) on a terminal; redirected
stdin retains diagnostics/wait behavior. The TUI uses the same Actor and
executor, including recovery and Steering while Main is busy.

`chat history` returns ordered semantic user/assistant messages. It also fills
missing accepted Conversation intents before reading. This is an intentional
recovery operation, not a read-only SQLite diagnostic. Tool calls/results and
partial model text appear only in Run context/RunTrace. A failed model produces
a durable failed Run and a safe CLI error; the Host does not retry it. New user
input creates a new Run after the old one ends.

## 实现与验证依据

代码入口、最新架构与取舍见 [技术方案](../designs/conversation.md)；已有检查、未覆盖场景和验收方法见 [测试方案](../test-plans/conversation.md)。
