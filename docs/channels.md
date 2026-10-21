# Web, Feishu and safe delivery

ERII-28 adds authenticated interaction and rendering to the existing
`InboundEvent -> Actor -> Main/Worker -> ToolInvocation -> Conversation ->
DeliveryRequest/Outbox -> Adapter` path. There is one Conversation per Agent.
No channel creates its own reasoning loop, Task state machine or approval store.

## Launch and identity enrollment

Configure the real Provider Bridge as in `config.example.yaml`. Without an
active, tested profile the Web setup page remains available and ingress stays
`setup_required`. Neither startup nor a missing credential selects a Fake.

```yaml
web:
  listen: 127.0.0.1:8787
  token_ref: env:ERII_WEB_TOKEN
feishu:
  app_id: your-app-id
  tenant_key: your-tenant-key
  verification_token_ref: env:ERII_FEISHU_VERIFICATION_TOKEN
  encrypt_key_ref: env:ERII_FEISHU_ENCRYPT_KEY
  access_token_ref: env:ERII_FEISHU_ACCESS_TOKEN
```

These are SecretStore references, never literal tokens. Web access tokens need
at least 24 bytes. Set credentials through the installation's credential store
or its explicit environment-reference backend. Feishu's configured access token
must remain valid; rotation/refresh belongs to the credential owner. A missing
or expired credential never falls back to another account or recipient.

The installation owner enrolls the exact Feishu tenant/app/open ID and p2p chat
through the OS-authenticated CLI. A callback cannot enroll itself. Group chats
and bot senders are rejected to avoid broadening access to the personal history.

```sh
printf '%s' '{"op":"bind","channel":"feishu","subject":"your-tenant-key:your-app-id:your-open-id","destination":"your-chat-id","enabled":true,"version":0}' | bin/personal-agent -config /absolute/config.yaml channel manage
bin/personal-agent -config /absolute/config.yaml web serve
```

`web serve` prints its loopback URL, enrolls the local browser identity once,
and runs in the foreground until SIGINT/SIGTERM. It owns HTTP ingress, scans the
same Actor/Automation claims and independently drains the same Delivery Outbox,
so one busy Worker cannot delay another Run’s approval notice. It joins its
work on shutdown. An explicitly revoked Web binding stays revoked on restart.
`channel manage` also accepts `list`, `preference`, `prefer` and versioned `bind`
updates. Set `enabled:false` with the current binding version to revoke access.
There is no destination migration: enroll a new destination and select it for
future work. Frozen deliveries keep their original destination. A changed Feishu app/tenant
cannot acquire an old binding’s pending delivery.

Web uses a process-local, one-hour HttpOnly/SameSite=Strict session after token
login, or a Bearer token for API access. It rejects DNS rebinding, cross-site
POSTs and invalid identities, provides no CORS access, and serves a restrictive
CSP. Auth URLs and device-code events stay ephemeral and are visible only in the
authenticated settings surface. Account login, reconnect and logout use the existing Manager/Bridge
lifecycle. The settings picker retains connected account metadata across restart.
Account model additions use its authenticated visible-model catalog. Credentials never enter Conversation, projections or evidence.

## User surfaces and callback contract

- `/`: shared semantic conversation, attachments and pending frozen approvals.
- `/settings`: local/account model metadata, explicit Test/Select and future
  notification preference. Selecting a provider requires explicit consent.
- `/progress`: read-only Task summaries and aggregate Child counts; no raw Tool,
  model, Run ID or Child trace. Pause/continue/cancel stays in conversation.
- `GET /api/conversation`, `/api/progress`, `/api/status`, `/api/auth`: authenticated
  projections. `POST /api/message` accepts only `source_id` and semantic `content`.
  `POST /upload` stores a local Artifact; `/artifact?ref=...` resolves only through
  the authenticated owner scope and downloads as an attachment, never active HTML.
- `POST /feishu/callback`: Feishu v2 `im.message.receive_v1`, signed exact bytes,
  five-minute timestamp freshness, configured token/app/tenant and bound owner/chat.
  AES-256-CBC encrypted envelopes are supported. URL verification may omit signature
  headers and must authenticate with the configured verification token. It creates
  no input, Run or binding. Publish only this callback through a TLS reverse proxy;
  keep the Web control surface on loopback.

Feishu text and file/image messages normalize to the same semantic ContentParts
as Web/TUI. Downloaded resources are tied to the verified source message. Feishu
approval responses use `/approve APPROVAL_ID FROZEN_HASH` or `/deny ...`; Web uses
controlled confirmation forms. Both use the same frozen-hash Security transition.
Duplicate decisions are idempotent; conflicting, invalidated or expired approvals
fail closed. Steering invalidates old approval arguments before dispatch.

Normal synchronous replies return to the latest input's bound channel. TUI input
remains TUI even if earlier Steering came from Feishu. A Main reply without user
input, Worker approval, completion, failure or needs-input notice uses the future
notification preference. `auto` prefers the first enabled Feishu binding, then
falls back to TUI. Multiple enrolled destinations can be selected explicitly in
settings. Ordinary progress never creates a notification. All routes freeze with
the semantic intent, before the first DeliveryRequest; preference changes cannot
redirect accepted messages. Revocation rejects pending deliveries to that binding.

## Deduplication, restart and uncertainty

Schema 025 adds bindings, event/source aliases, frozen message routes, preferences,
UI projections, imported-resource mappings and per-part channel receipts.
Migration is forward-only, transactional, preserves existing TUI receipts and
runs from empty or every prior supported schema.

The input transaction checks the enabled binding, canonical semantic hash and
both callback and source identities, then commits the existing `main_input`,
Conversation intent, route and Outbox event together. A new callback ID carrying
an already accepted source ID reuses its receipt. Identity or parameter drift
fails closed. Web/TUI/Feishu inputs preserve the authoritative Conversation order.

The Composer persists UISpec v1 in Delivery, never semantic Conversation or model
context. Fixed templates support escaped semantic Markdown text, tables, Artifact
cards, progress, frozen confirmation cards and field summaries. They do not execute
HTML/JS/React. Unknown versions, unknown components and invalid/render-failed
components degrade to semantic text. TUI remains semantic text/media fallback;
Feishu uses a controlled plain-text card and uploaded file attachments. Each
outbound file retains its name and exact bytes. Missing/oversized resources fail
closed rather than exposing a host path or silently sending the wrong attachment.

Local Web receipt acceptance is durable and idempotent. Feishu UUID deduplication
has a bounded provider window, so the adapter does **not** promise indefinite
idempotence. Successful parts retain authoritative response IDs before Outbox
acknowledgement. Restart reuses those receipts. A disconnected send without an
authoritative receipt becomes `unknown` and is query-only; it is never blindly
resent. Interrupted uploads can leave an unreferenced provider file, but cannot
justify repeating a user notification. The user can see the semantic message in
Web history while a remote transport remains unconfirmed.

## Verification and evidence

```sh
make test
make test-race
make lint
make verify
python3 scripts/verify_channels.py --binary bin/personal-agent --evidence evidence/channels.json
```

`verify_channels.py` owns Launch (private root, local platform Fake and real Web
process), Doctor (auth and real browser setup), Drive (HTTP/UI/CLI boundaries),
Evidence (SQLite/Conversation/API/platform ledgers and browser screenshots), and
Cleanup (joined children, stopped Fake server and removed disposable state).
It runs five consecutive times in the Delivery shard of `make verify` and CI.
The pinned Browser dependency is the repository's existing `playwright-core`.
Screenshots and DOM evidence are under `evidence/channels-ui/`.

The harness covers concurrent callback/source retries, identity/auth/recipient
rejection, input commit and delivery SIGKILL windows, approval Steering plus
restart and callback races, inbound/outbound attachment bytes, unknown-send
non-retry, and GC-08/09/12/14 channel/state fragments. Unit/contract tests cover
migration rollback, CAS/revocation, route/projection freezing, encryption,
credential redirect refusal, session expiry, XSS and unknown/invalid-component
fallback. The channel harness also drives Web account login through the real Manager/Bridge
against the existing local OAuth Fake, actual Probe/Select, hidden-model rejection,
restart and logout, then scans persisted files for credential values. Existing
suites retain replay, cancellation, account lifecycle and
Artifact access coverage.

Scripted-policy transport fragments are not full product scores. Complete 20-case
semantic grading and live Feishu/ChatGPT authorization remain release evidence;
this slice does not claim M7 or the complete first release is finished. No live
external account or write is required by CI.

Feishu transport shapes follow the official [message API](https://open.feishu.cn/document/server-docs/im-v1/message/create),
[resource API](https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/im-v1),
and [callback encryption guide](https://open.feishu.cn/document/server-docs/event-subscription-guide/event-subscription-configure-/encrypt-key-encryption-configuration-case).
