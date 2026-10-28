# 安装、启动与部署

[主索引](../feature-map.md) · [技术方案](../designs/installation.md) · [测试方案](../test-plans/installation.md)

## 功能与需求

<a id="f-037"></a>

### F-037 · 安装、首次启动与显式部署

用户能从源码或原生包安装，完成 Doctor、首次模型连接和终端启动，并按需使用显式 Web/外部 broker 配置。当前安装、版本化目录与托管本地 broker 已实现；平台和依赖条件见下文。是否完成以主索引为准。

验收目标：真实安装、首条输入、工具回复、退出、崩溃重启及重装保持数据；未就绪模型不隐式选择默认提供商。

## 如何使用

按下方安装步骤操作；模型详细设置见 [模型连接](model-connections.md)，Web 配置见 [渠道](channels.md)。

## 操作说明与配置细节

迁入内容按职责归属在此维护。既有编号用于追溯协议，不代表所有设计目标均已实现；完成状态以需求主索引为准。

<a id="getting-started-install-and-start-erii"></a>

## Install and start Erii

<a id="getting-started-install"></a>

### Install

From the repository checkout, install Go 1.26.2+ and Node.js 24.21.0, then run:

```sh
./install.sh
~/.local/bin/erii
```

No Python, browser download, YAML editing, broker installation or frontend build
is needed for terminal conversation. The source installer needs Go and npm while
building. A prebuilt native archive needs only Node: unpack the archive and run
its `install.sh`. Maintainers produce archives with `sh scripts/package.sh "$PWD/dist"`.
Until a release is published, use the checkout installer.

The installer creates a versioned runtime in `~/.local/lib/erii/releases/`, a
`current` symlink, and the `~/.local/bin/erii` launcher. It checks the new runtime
before switching the launcher. It refuses to overwrite unrelated executables or
unmanaged installation directories. It never edits your shell profile.

For another installation location:

```sh
./install.sh --prefix /absolute/installation-prefix
/absolute/installation-prefix/bin/erii
```

Add the prefix's `bin` directory to your shell's `PATH` if you want the short
`erii` command. Rerun the installer to upgrade. Prior runtime releases stay in
`lib/erii/releases/`; remove old releases only after stopping Erii and confirming
they are not the `current` target. User data is separate and survives reinstall.

<a id="getting-started-connect-a-model-and-chat"></a>

### Connect a model and chat

Bare `erii` opens guided setup on a terminal. Choose a local OpenAI-compatible
endpoint or Sign in with ChatGPT. Follow the prompts to discover or enter a model,
probe its capabilities, and explicitly confirm selection. Without a selected
model, Erii stays `setup_required` and accepts no Agent work.

After selection, type your message. `/models` lists profiles, `/setup` repairs a
connection, and `/quit` or Ctrl-C closes the foreground application. Within a
setup prompt, `/cancel` returns to conversation before `/quit`; Ctrl-C exits
directly. No daemon is
installed. Existing Task and Run recovery uses the same durable state on restart.

ChatGPT credentials require the native macOS Keychain build. On other platforms,
use an unauthenticated local endpoint or an explicit API-key environment reference.
Never put credentials in configuration files. See [provider setup](model-connections.md)
for authentication requirements and model privacy details.

<a id="getting-started-diagnose"></a>

### Diagnose

```sh
erii doctor
erii status
```

`doctor` checks the Node version and installed runtime/dependency files without
opening user databases or contacting a provider. `status` opens the control plane
and reports persisted model readiness. It can initialize or migrate state.
`erii start` is the explicit form of terminal startup and requires a terminal.
With redirected input, use an explicit control command such as `status`.

Default startup manages an embedded JetStream server on a random **loopback-only**
port. Its durable notification storage is under the data root's `broker/` directory.
SQLite remains authoritative. A data-root lock prevents two managed brokers from
using the same directory. Shutdown drains the application before stopping the
broker; restarting after a crash reopens the same durable stores.

<a id="getting-started-advanced-deployment"></a>

### Advanced deployment

The default data directory is `~/.local/share/erii`. Override it explicitly with
`ERII_DATA_ROOT=/absolute/path erii`. Do not use a temporary directory for real work.

An explicit config file or `ERII_NATS_URL` opts into your configured external
broker instead of the managed local broker:

```sh
erii -config /absolute/erii.yaml start
```

With a config file, only its configured model bridge and optional runtimes are
enabled. The no-config installation defaults do not override that capability choice.
Bundled runtime discovery applies to `start`, `doctor` and `status`. Low-level
execution commands such as `chat work` keep their explicit configuration contract;
they never infer another provider while recovering existing work. Use the terminal
for ordinary conversation or supply `-config` for scripted execution.

Copy only the needed sections from [config.example.yaml](../../config.example.yaml).
The example's `/tmp` data root is for disposable development. Flags precede commands.
The browser surface remains `erii -config /absolute/erii.yaml web serve`; it needs
its existing loopback listener, SecretStore token reference and broker configuration.
See [channels](channels.md) and [operations](operations.md).

The underlying Go entry point is `cmd/erii`; local builds produce `bin/erii`.
The previous `personal-agent` command/path is replaced; existing databases and
model snapshots do not require a migration for this executable rename.
