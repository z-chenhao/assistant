# Documentation

Read the guide for the task at hand. The technical baseline defines intended
semantics; feature guides describe the implemented paths and their proof limits.

## Use Erii

- [Install and start](guides/getting-started.md)
- [Model connections and ChatGPT sign-in](guides/provider-bridge.md)
- [Terminal setup and conversation](guides/tui-and-evals.md)
- [Web and Feishu](guides/channels.md)
- [Tasks and follow-up](guides/tasks.md), [Child Runs](guides/children.md)
- [Tools and approvals](guides/tools.md), [Code Mode](guides/codemode.md)
- [Memory maintenance](guides/memory-maintenance.md), [recall](guides/recall.md)
- [Artifacts](guides/artifacts.md), [delivery](guides/delivery.md)
- [Automation](guides/automation.md), [capabilities](guides/capabilities.md)
- [Computer lifecycle](guides/computer.md), [Computer Use](guides/computer-use.md)
- [Operations, backups and recovery](guides/operations.md)

## Develop and verify

- [Contributing, development commands and layout](../CONTRIBUTING.md)
- [End-to-end verification](guides/verification.md)
- [Verification feature map](../scripts/FEATURES.md)
- [Main loop and recovery](guides/main-loop.md)
- [Core conformance](guides/core-evals.md), [personal-assistant evaluation](guides/m6-evals.md)
- [CI operations](ci.md), [Golden Cases](../evals/golden-cases.md)

## Architecture and historical evidence

- [Technical baseline](technical-design.md)
- [Architecture decisions](adr/README.md)
- [Recorded reports](reports/README.md)

New feature instructions belong in `guides/`. Approved architecture decisions
belong in `adr/`. Dated measurements and compatibility results belong in
`reports/`; they are historical observations, not current release certification.
Generated local verification output stays in ignored `evidence/`. Update this
index and the feature map when changing a user path. `make lint` checks local
Markdown destinations and heading anchors, including repository AGENTS links, so moves
cannot silently break navigation. Code examples and runtime-injected policy are
not repository navigation.
