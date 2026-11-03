# Steward Web reconstruction

Mode: Operate. Visual authority: the current production templates, shared CSS tokens and self-hosted Steward/Ghost assets.

Current users: the owner's personal agent workspace. Keep the authenticated, single ongoing conversation, bounded history, durable send acknowledgement, attachments, frozen approvals, task list/detail/progress/milestones/artifacts, models/accounts, notifications, capability editors, and developer inspection.

Use the approved soft monochrome tokens, Michroma wordmark, Inter UI, Source Serif 4 reply prose, PingFang SC / Source Han Sans SC Chinese and JetBrains Mono code. One 256px sidebar; centered 768px conversation; document-style replies, warm-neutral user bubbles and multiline composer. Chat has a centered two-row name/avatar header and continuous conversation. Computer and Worker open mutually exclusive read-only detail panes, never another navigation sidebar. Finished tasks lead to their real delivered artifacts. No synthetic recents, global memory browser or fictional file library.

Mechanism remains server-owned HTML plus bounded live-region patches and existing React UISpec islands. Theme, sidebar visibility and avatar motion are local presentation preferences. Existing HTTP paths, POST fields, source IDs, frozen hashes, authentication and replay semantics stay stable. Scope is private Web presentation; no new shared/public execution contract.

Domain-required complexity: readiness gating; draft/attachment retention on uncertain send; pagination; approvals and task-scoped return links; safe offline feedback; focus and scroll preservation; technical detail inspection. Rejected speculative complexity: new conversation store, universal navigation framework, new artifact index, memory CRUD, fabricated task counts and pretend actions.

Narrow reversible implementation: replace presentation CSS/templates, add local shell controls and demand-loaded task peek using `/api/updates`; retain route and projection behavior. Keep developer detail as a specialist page instead of imposing it on the home conversation. Product, command, configuration, storage and internal namespaces use a single Steward identity. No alternate brand compatibility routes or data migrations are provided.

## Interface language

English is always the first-use default, independent of Accept-Language. Settings → Preferences offers English and Simplified Chinese through a native authenticated form. A validated browser cookie selects immutable locale-specific templates; frontend controls import the same private JSON catalog. Live fragments use the selected locale. This is UI-only: never translate stored messages, replies, task titles/summaries, files, frozen arguments, raw technical evidence, protocol values or model identifiers. Retain the existing brand/fonts/layout.

Latest approved refinement: exactly four navigation entries (Chat/Tasks/Settings/Developer); Chat identity above continuous conversation, with Computer and current Worker at top right. Worker history includes ongoing and finished executions ordered by execution time; Task keeps its independent list and detail. Computer is an on-demand read-only desktop viewer, not a second navigation sidebar or input/control channel. Its separate Driver session cannot change agent capture receipts or provision a VM; it rejects epoch changes. Source Han uses disjoint unicode-range subsets with full original coverage.

Agent identity refinement: product brand remains Steward; the Chat header identifies the assistant as Ghost in the UI typeface. Keep its animated avatar only in the header, including the welcome state. Messages use alignment, bubbles and spacing instead of repeated visible avatars/names, with screen-reader speaker labels retained.

Chat controls: Computer above Worker, then pending approvals, in a vertical stack at top right. Local Web opens directly without a workspace token when token_ref is omitted; configured token installations keep their explicit protection.

Layout refinement: Task and Developer use their actual content-container width to stack below 760px. Chat viewers use the existing modal below 1280px so the navigation and a detail pane cannot crush prose/composer width. Keep the selected tab visible within its own horizontal strip, preserve keyboard focus across navigation and trace fragment replacement, and show capability examples before editing. These remain private presentation choices; routes and frozen execution semantics stay unchanged.

Task priority: reveal saved upcoming work and how it is configured, including schedules before first dispatch. Owner-native schedule editing uses real instructions, enabled state, friendly frequency/date/time, saved IANA zone and optional threshold. It does not rewrite immutable Task objectives/policies or invent structured future steps. Capability settings lead with six native guided forms, retaining advanced YAML and the exact test/save proof contract.
