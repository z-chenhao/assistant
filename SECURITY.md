# Security

Keep credentials out of issues, logs, prompts, YAML and artifacts. Configuration
uses SecretStore references. Never upload a real data root or authentication URL
as a bug reproduction.

For a suspected vulnerability, use GitHub private vulnerability reporting when
available, or contact the repository maintainers privately before opening a public
issue. Include the affected revision, a minimal disposable reproduction, impact
and observed behavior. Do not exercise the issue against another person's data.

The [technical baseline](docs/technical-design.md) defines approval, capability,
credential and replay boundaries. [Operations](docs/guides/operations.md) describes
backup and recovery. Offline test success does not establish live-provider or
Computer compatibility.
