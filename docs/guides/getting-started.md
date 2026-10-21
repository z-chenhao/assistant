# Install and start Erii

## Install

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

## Connect a model and chat

Bare `erii` opens guided setup on a terminal. Choose a local OpenAI-compatible
endpoint or Sign in with ChatGPT. Follow the prompts to discover or enter a model,
probe its capabilities, and explicitly confirm selection. Without a selected
model, Erii stays `setup_required` and accepts no Agent work.

After selection, type your message. `/models` lists profiles, `/setup` repairs a
connection, and `/quit` or Ctrl-C closes the foreground application. No daemon is
installed. Existing Task and Run recovery uses the same durable state on restart.

ChatGPT credentials require the native macOS Keychain build. On other platforms,
use an unauthenticated local endpoint or an explicit API-key environment reference.
Never put credentials in configuration files. See [provider setup](provider-bridge.md)
for authentication requirements and model privacy details.

## Diagnose

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

## Advanced deployment

The default data directory is `~/.local/share/erii`. Override it explicitly with
`ERII_DATA_ROOT=/absolute/path erii`. Do not use a temporary directory for real work.

An explicit config file or `ERII_NATS_URL` opts into your configured external
broker instead of the managed local broker:

```sh
erii -config /absolute/erii.yaml start
```

With a config file, only its configured model bridge and optional runtimes are
enabled. The no-config installation defaults do not override that capability choice.

Copy only the needed sections from [config.example.yaml](../../config.example.yaml).
The example's `/tmp` data root is for disposable development. Flags precede commands.
The browser surface remains `erii -config /absolute/erii.yaml web serve`; it needs
its existing loopback listener, SecretStore token reference and broker configuration.
See [channels](channels.md) and [operations](operations.md).

The underlying Go entry point is `cmd/erii`; local builds produce `bin/erii`.
The previous `personal-agent` command/path is replaced; existing databases and
model snapshots do not require a migration for this executable rename.
