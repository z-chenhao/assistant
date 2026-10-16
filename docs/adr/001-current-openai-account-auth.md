# ADR 001: Current OpenAI account authentication and deferred device flow

Status: accepted by 朱晨浩 on 2026-10-16 in ERII-8, comment
`01a1431f-37d1-7369-9c29-fba77578c4a5` (方案 A). Implementation remains subject
to independent review and human merge.

## Decision and evidence

Use the current OpenAI public-client subscription protocol in the Provider
Bridge. Retain Pi AI 1.3.0 for Responses generation. Its bundled login hardcodes
port 1455, checks ID-token presence rather than verified identity, and its OpenAI
catalog is static. Calling it unchanged cannot satisfy §5.4. Implement the
documented current OAuth protocol inside the existing Bridge, with random-port
loopback, expiring one-use state, PKCE S256, verified ID-token identity and a
dynamic account-specific catalog. Go does not implement provider-private wire
details. No additional production dependency is required.

Device Authorization is deferred until a supported **current-provider** grant
is established. It remains an explicit future requirement and is excluded from
this slice's completion gate. It must not be reported as tested or delivered.
Do not substitute the legacy `openai-codex` provider or Platform API keys.

Authoritative protocol references, inspected 2026-10-16:

- [Registration and sign-in](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)
- [Accounts and sessions](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)
- [Models and inference](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
- [ID-token verification](https://developers.openai.com/siwc/website#6-verify-the-id-token)
- [Preview wire restrictions](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)

## Boundary and invariants

The only consumers are the existing CLI/settings control plane and trusted
generation resolver. `account.Manager` owns lifecycle and uses a narrow private
Store/Provider boundary. This is not a model Tool, public API, general OAuth
framework or second generation pipeline. Credentials cross only the existing
trusted Host/Bridge RPC and SecretStore boundaries; no credential file is used.
UI events never contain tokens. Private authorization URLs are shown only for
the active interaction and must not be captured as logs or reports.

Each AuthConnection owns one immutable account/client registration. SQLite keeps
the issued public client ID and a hash of verified issuer/subject; labels are
masked. Access, refresh and retained ID tokens stay in OS SecretStore. The
installation device UUID is non-secret private configuration, excluded from
diagnostics, events, traces, reports and prompts.

Cross-process kernel locks serialize refresh, credential replacement, recovery
and logout for one connection. A persisted `refreshing` row observed after
obtaining the lock means its prior owner was interrupted: fail closed to
`reauth_required`, delete credentials and revoke proof. Never replay a rotating
refresh whose outcome may be unknown. Successful refresh preserves capability
proof for the same identity and updates its auth provenance atomically. Auth
loss/logout invalidates related profile proof and closes ingress atomically
with metadata/events. Remote revocation failure does not prevent local logout;
report whether revocation was confirmed.

## Migration, tradeoffs and intentionally private choices

Schema v6 adds private installation identity, immutable account registrations
and legal auth transitions. Empty and v5 migration are transactional. Existing
profiles, snapshots, usage and events are preserved. Pre-v6 OAuth metadata was
never backed by a real login; it has no validated registration and cannot become
ready by being imported. Create a new verified account connection for such a
record; do not attach a new identity to a historical snapshot.

The existing Keychain/SQLite boundary cannot perform one distributed commit.
Persist `refreshing` before rotation; persist the new SecretStore value before
committing connected metadata. A crash between those writes requires re-login,
even if rotation succeeded. Failed initial registration cleans the newly owned
secret; a process death before metadata insertion can still leave an orphaned
Keychain item, as with existing API-key registration. No automatic credential
enumeration or cross-account deletion is introduced.

Provider network destinations are fixed. Only the local fixture substitutes
network I/O; production RPC cannot choose OAuth endpoints or an account base
URL. Catalog visibility and actual returned model IDs gate account inference.
Optional reasoning/modalities remain conservative until independently verified.
No change to Main/Worker ownership, immutable snapshots or ToolInvocation.

The MVP is a single local installation, on macOS/Linux filesystems supporting
advisory locks. Account streams use HTTP with provider cache retention disabled;
logout aborts matching Bridge work and reaps its process. Another Host's final
result is rejected after auth loss: the usage write transaction rechecks the
Host-captured account epoch and status, serializing acceptance with auth mutation.
Only a committed acceptance can publish completion. Logout/reauthorization between
the observer Guard and persistence rejects the old text with `AUTH_REQUIRED`;
successful same-identity refresh preserves acceptance. Remote history/cache
deletion is not promised. Account-provider 401/403 closes readiness without switching model.

Generation registrations remain cancellable while the observer or SQLite writer
admission is pending. A private Host-only `Finalize` callback joins the existing
usage transaction to the registration's acceptance lock. Cancel/Dispose ack first
means `GENERATION_CANCELLED`, empty final output and no completion; commit first
preserves the accepted result. Auth loss is still checked in that same SQLite
transaction and rejects with `AUTH_REQUIRED`. The lock excludes observer waits,
RPC and sink callbacks; the shared RPC reader is not locked during persistence.
Waiting Cancel/Dispose calls retain their own deadlines; failed local disposal
cleanup retires the peer. An already admitted commit is allowed to settle.
No new wire field, stored schema, migration or alternate generation path is
needed. Peers and immutable snapshots retain protocol v1 compatibility.

## Verification and remaining requirements

`make test`, `make test-race`, `make lint` and `make verify` include deterministic
OAuth/ID-token fixtures, actual Pi Responses, CLI/storage readback, single-flight
refresh, failure injection, secret scans and kill/restart. The account harness
owns launch, doctor, drive, evidence and cleanup. Required auth cases repeat five
times in `make verify`. No online account is needed in CI.

Unexecuted real login/Keychain/provider Live Shadow is **inconclusive**. Device
Authorization is **deferred**, not passed. Main/Conversation Golden Cases belong
to ERII-9/10; this slice does not establish full M1 or release completion.
