# 005 — Consolidated model-visible Tools

Status: accepted for the user-requested ERII-192 Tool consolidation.

Artifact exposes exactly `builtin__artifact__manage` and
`builtin__artifact__search`. Manage keeps strict operation-specific fields for
save/read/describe/publish; search browses or filters scoped metadata. The coarse
manage permission remains local `artifact_write`, with operation validation
before execution and before publication. Neither Tool grants external writes.
Run-derived scopes, immutable bytes, atomic result/publication commits, replay
keys, approval boundaries and Child reductions use the existing pipeline.

The former echo builtin only returned model data. Remove it from production;
explicit offline bridges use `mcp__fixture__echo`. Do not reinterpret a pure Tool
as sending a message. User output stays Main AssistantMessage → Delivery/Outbox →
Composer/UISpec, including the existing OpenIntelligentUI native table. No new
send Tool, Worker delivery permission or arbitrary generated UI is introduced.

Memory search and Main-only manage remain discoverable without configuration.
Their descriptions identify required configuration; calls return
`PROVIDER_UNAVAILABLE` without a mutation, privacy change, implicit provider or
credential. Unconfigured search freezes a disabled identity, so normal turns do
not attempt automatic recall. Configured recall still freezes its real provider
identity, and configured maintenance retains encryption and Curator receipts.
Worker-only `agent.run` remains singular and uses the same ToolInvocation path.

## Compatibility and upgrade

No database schema changes or stored snapshot rewrites are required. This is a
model-visible contract break: the five old Artifact Tool names and production
echo are not aliases. Registry selection compares each frozen Spec exactly.
A Run with a removed/changed Spec fails `MODEL_INCOMPATIBLE`; it never silently
uses the new schema. Existing invocation results and audit records remain intact.

Drain or pause work before upgrading. Finish an active frozen Run with its
original binary/configuration when needed. After upgrading, use a new Main Run
or `task_manage(append)` to continue work with a new Root and new frozen view.
Do not revive or mutate the old Run. Model/embedding configuration changes follow
the same frozen-snapshot rule; they apply to newly created Runs.

## Verification

Schema tests cover all operations, cross-arm/duplicate/null/ownership rejection,
and removal of legacy names. Real SQLite tests cover bytes, metadata filtering
before the result limit, foreign scope denial, publication, replay reuse and
unconfigured Memory without mutation. Maintained CLI harnesses inspect actual
production HTTP projection, nested Code Mode calls, real CUA/Child behavior,
Artifact kill/restart recovery, configured Memory and channel delivery. See the
ERII-192 entries in `scripts/FEATURES.md` for commands and evidence ownership.
