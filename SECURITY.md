# Security policy

Steward handles private conversations, credentials and potentially consequential
tool actions. Preserve approval, isolation, frozen-snapshot and durable-replay
boundaries. Keep credentials in SecretStore or explicit environment references.
Do not attach real data roots, authentication URLs or unredacted logs to issues.

## Report a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/z-chenhao/steward/security/advisories/new)
when the repository offers the reporting form. If it is unavailable, open an
issue requesting a private security contact **without vulnerability details**;
do not publish the exploit or sensitive evidence in that request. The repository
owner must enable the private reporting setting; source changes cannot enable it.

Include the affected revision/platform, impact, minimal disposable reproduction
and any proposed fix. Do not test against another person's data or services.
There is no guaranteed response time or automatic bounty program.

## Supported fixes and checks

Security fixes target current `main` and newly prepared releases. Historical
branches have no promised backport window. Use the current minimum Go toolchain
from `go.mod`; rebuilding with an older toolchain can retain standard-library
vulnerabilities even when application dependencies are unchanged.

`make audit` queries the current Go vulnerability database and npm advisories.
The dependency audit workflow runs weekly, manually and for dependency PRs;
Dependabot proposes reviewed updates. Scans are time-bounded evidence, not a
security guarantee. High/critical npm findings and reachable Go findings fail
the audit; assess non-reachable package/module advisories separately.

See [runtime contracts](docs/designs/runtime-contracts.md),
[approval boundaries](docs/designs/tools-approvals.md) and
[backup/recovery](docs/features/operations.md) for the supported operating model.
