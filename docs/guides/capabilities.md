# MCP, Skills and controlled coding

These optional adapters enter the existing registry and ToolExecutor. Main and
Root receive the same configured work surface; Root still lacks Task control and
user delivery. Neither role receives Skill management. No second discovery API,
execution loop, approval pipeline or model credential channel is introduced.

Use `config.example.yaml` for configuration. Paths are operator configuration;
frozen ToolSpecs represent their scopes with hashes. A `resource_hash` binds
each new ToolSpec to its configured scope. Descriptions and hashes are part of
frozen `tools_json`; Skill resources also determine `skill_set_hash`.
Descriptions reach the Pi Bridge. Full Skill bodies enter context only on view.

## MCP transport

Configure an absolute executable, argument vector, dedicated provider directory,
server name and tool allowlist. No shell, inherited credentials, proxy settings,
user HOME or network access is available to the server. Its executable, system
runtime files and configured provider directory are the only file-read scopes;
only that directory is writable. Put interpreted server code/dependencies there
if using a system interpreter. The provider directory must not overlap domain
data, Skills, the coding checkout or another provider directory. Scope checks
resolve aliases through existing ancestors even before new data directories
exist. The command must be self-contained or use
the exposed system runtime; arbitrary host package paths are unavailable.

The adapter implements MCP `2025-03-26` newline JSON-RPC stdio initialization,
initialized notification, bounded paginated `tools/list`, and `tools/call` text
results. Each operation owns a short-lived initialized session; every call
rediscovers its advertised tool and rejects schema/description drift before
dispatch. Unknown allowlist entries, duplicate/invalid names, oversized frames,
unsupported schemas, server requests, malformed replies and unsupported content
fail closed. Notifications are bounded; they never update a running catalog.
See the [MCP transport specification](https://modelcontextprotocol.io/specification/2025-03-26/basic/transports)
and [tools specification](https://modelcontextprotocol.io/specification/2025-03-26/server/tools).

The operator must classify each exposed tool as `read_only` or `external_effect`.
Provider annotations cannot grant permissions. Reads use `safe`; effects use
`high` risk and `never` replay because standard MCP has no authoritative receipt
query/idempotency contract. Every effect requires its own frozen approval.
Transport loss or a malformed result after dispatch is `unknown`; no retry or
claimed success follows. A call result must be an object with a non-null content
array, text entries with explicit string text, and a boolean `isError` when
present. Empty content arrays and explicit empty text are valid.
Raw provider errors are discarded. Cancellation terminates and collects the
session's process group. The executor supplies timeout, audit, conflict, quota,
result ordering and Artifact spillover semantics. A server-wide conflict key
serializes calls to the same configured provider across Runs.

HTTP/SSE, authenticated external MCP connections and production website access
are not implemented. The local stdio adapter exercises real structured protocol
calls against deterministic external-system Fakes without opening host network
or credential access. Future Guest-connected adapters must retain the same
registry, frozen policy and invocation lifecycle.

## Skills

The configured catalog belongs to this data root's local Agent. `agent` and
`workspace` identify the curated scope; neither grants cross-Agent access. Each
directory contains frontmatter `name`/`description` in `SKILL.md` and optional
references/scripts/assets. Names match their directories. Symlinks, malformed
metadata and oversized catalogs are rejected. Hashes include every catalog file,
not only the summary. No full body is embedded in the startup description.

`builtin__skill__list({})` returns name, description, scope and content version.
`builtin__skill__view({name, version, path?})` defaults to `SKILL.md`; relative
references must already exist in the catalog. A wrong version or changed file
returns `NON_DETERMINISTIC_REPLAY`; paths cannot escape the configured root.
Non-UTF-8 assets use the common binary-result Artifact path, preserving bytes
without placing invalid text in model history.
Changing a reference or scope changes the frozen set hash. A restarted old Run
fails compatibility selection if its catalog no longer matches. Restore the old
catalog to continue it; do not silently switch versions. Ordinary Main/Worker
tools cannot manage Skills. The trusted [Experience Reviewer](memory-maintenance.md)
owns the separate propose/verify/promote/retire lifecycle.

## Coding fixture boundary

Provision an exclusive disposable Git checkout, with AGENTS.md and no shared
writers. Preserve the user's original checkout separately. This interim adapter
is for bounded controlled repositories, not execution of hostile workloads or a
replacement for the M6 Lume VM/Guest ComputerProvider. The current maximum is
4,096 walked entries, 64 KiB per exposed file, 8 MiB per test/metadata copy.
Shared worktrees, alternates, symlinks and hard-linked exposed files are denied.
Do not configure a checkout containing credentials. Configuration rejects scope
overlap with Erii data, Skills and provider directories.

| Tool | Behavior | Permission / replay |
| --- | --- | --- |
| `builtin__workspace__read` | Relative regular file, text and SHA-256 | read_only / safe |
| `builtin__workspace__search` | Bounded literal search or file listing | read_only / safe |
| `builtin__workspace__git` | Fixed status/diff commands only | read_only / safe |
| `builtin__workspace__test` | Fixed `go test -race ./...` in an isolated disposable copy | read_only / safe |
| `builtin__workspace__edit` | One existing clean tracked file; matching `before_hash` and AGENTS.md `rules_hash` | external_effect / high / never |

`audit` mode omits edit entirely, including from Worker discovery. `edit` mode
requires an exact approval and rejects protected paths, AGENTS.md, dirty/staged
files, assume-unchanged/skip-worktree index entries, invalid links and stale
hashes. Index flags are inspected in the private metadata copy and never cleared
on the original index. A temporary sibling is synced and renamed
atomically. This first adapter conservatively rejects a second edit to an already
dirty file, including its own earlier edit. There are no create/delete/commit,
reset/checkout, arbitrary shell, environment, mount or command parameters. Human
integration of the reviewed diff remains outside this tool surface. A crash after
rename but before result commit stays `unknown`; the file is not edited again.
An exclusive checkout is required because filesystem CAS cannot fence an
uncooperative external writer between the final preimage check and rename.

Git runs against a disposable metadata copy without repository config, hooks,
external diff/textconv, filters, fsmonitor, replacement objects or submodule
recursion. The original index, dirty files and hooks remain byte-identical.
Transient metadata locks are excluded. Automatic maintenance is disabled for
read commands and fixture bootstrap so no detached Git writer owns test state.
Tests copy regular source files without `.git`, then run under macOS Seatbelt
(`sandbox-exec`) or Linux bubblewrap namespaces with host user files, network,
keychain/Mach services and inherited secrets unavailable. Writable cache/temp
files and test mutations stay in that copy. The source checkout is not mounted
into the test sandbox. Output is bounded and temporary paths are normalized.
Tests returning nonzero report `passed:false`; they do not imply success just
because the Tool completed. A missing sandbox fails closed; there is no host
execution fallback. Process groups are collected and disposable files removed.

Linux requires bubblewrap and permission for unprivileged user namespaces. CI
installs the distribution package and enables namespaces only on its disposable
runner. macOS requires the system sandbox and Xcode/Command Line Tools. No new Go
or Node production dependency is added. Kernel sandboxing is an interim isolation
boundary. [Computer operations](computer.md) and [Computer Use](computer-use.md)
own VM lifecycle/resource isolation and the separately recorded live compatibility
evidence; this coding fixture does not establish those results.

## Reproduction and evidence

```sh
make build
python3 scripts/verify_capabilities.py --binary bin/erii --evidence evidence/capabilities.json
go test -race ./internal/tool/... ./internal/skill ./internal/computer/... ./internal/config
```

The harness launches private data/provider/Skills/checkout roots, doctors an
explicit Fake model, drives ordinary chat/Task/approval CLI commands, reads SQLite
and file hashes, kills after a real edit before result commit, restarts, and
cleans every child and disposable root. `make verify` repeats it five times.
The coding fixture starts with a real failing `--status paused` regression,
edits only its CLI through a Worker, then passes its real race tests. Audit reads
the deliberately defective retry ordering without changing any byte. Go
regressions additionally probe host credential reads/writes, environment/network
denial, link escapes, catalog drift and executable Git config.

This extends the M3 Tool/Approval/Artifact fragments in `verify_tools.py` and
`verify_artifacts.py`; it does not assign a semantic Golden score. GC-19 is a
coding prerequisite here. This fixture marks GC-20 Child/recovery and full
GC-19/20 Durable cases `not_applicable`; the [Core gate](core-evals.md) covers
their additional mechanics. Production Computer has [separate verification](computer.md).
The added GC-11 mechanical fragment reads three actual provider quote records,
preserves oral-cashback uncertainty and reopens the saved comparison Artifact.
Its deterministic scripted model does not establish live-model reasoning quality.

No new tables or domain states are needed: schema v12 already freezes arbitrary
ToolSpecs, Skill set hashes, policy and invocation outcomes. Empty/prior schema
migration tests remain in the existing SQLite gate. Old capabilities without
these optional fields keep their old tools; enabling an adapter never expands an
already frozen Run. Changing a configured scope fails old-Run selection closed.
