# Account monetary authorization

Accepted for ERII-18 on 2026-10-17 by the project owner. Baseline §6.6.6 is
normative. This resolves the previously unspecified price/limit policy.

Use an explicitly configured account monetary boundary and provider/model price
version. Token counts remain accounting and safety observations. They never
authorize spending. No model-visible budget or capability query is added.

The Host freezes pricing and policy in an immutable companion to each new
ModelSnapshot. Children share that snapshot. The generation admission transaction
reserves a conservative monetary projection from frozen context/output bounds;
usage acceptance settles it atomically with the provider observation and model
result. Missing usage retains its reservation as unknown actual cost. Existing
snapshots migrate as unpriced/unconfigured, without retrospective valuation.
Explicit setup Probes have usage-only snapshots and remain visible unpriced
control-plane observations; they are not silently valued as business Runs.

When committed account spend plus the next reservation reaches the boundary,
the Host holds dispatch and creates one durable approval owned by the Root.
All descendants join it. Its frozen action explicitly grants the remaining
bounded Run batch model spending beyond the account boundary, without another
monetary ceiling. It grants no Child Tool effect. A separate synchronous Main
Run uses its own approval; a Worker grant never authorizes Main work.

The grant is scoped to the exact Run identity, account, currency, frozen price
snapshot and Task epoch. New Roots require their own grant if the account
boundary is reached. Approval requires local ownership and the displayed frozen
hash. Denial fails held model calls with `SPEND_AUTHORIZATION_DENIED`. Pause or
cancel invalidates pending grants; old epochs cannot decide or accept usage.
Pending monetary approvals do not expire on process exit or release/reacquire.

Policy absence, missing prices and missing usage are visible states. They never
become guessed rates or zero-cost observations. Zero prices require every rate
to be written explicitly. No automatic currency conversion is supported. A
configured account cannot switch currency, and a pricing version cannot be
reused with different rates. Configuration changes affect new Runs only.

This is private Host/control-plane infrastructure. It reuses the existing
generation, Task stop, security notification, CLI/TUI approval decision and
ToolInvocation boundaries. Monetary consent is a different frozen action from
tool-effect consent, so it has a typed ledger rather than a fabricated Tool call.
It does not introduce another Agent Loop, effect execution path, model tool,
pricing service, token allowance or billing integration.

Tradeoff: projections deliberately reserve the full frozen model context/output
bound, rather than predicting tokens from text. This can prompt earlier than
actual usage requires. It prevents concurrent calls from independently consuming
the same authorization headroom. Historical unpriced usage remains unknown;
the local spend view and approval action disclose those call counts. Reported
usage exceeding a reservation is recorded as observed and affects subsequent
admission; a provider that ignores its model bounds cannot be given a guaranteed
real-world billing ceiling by this local mechanism.

Migration 015, `TestSpend*`, `TestSpendRequiresExplicitIntegerPrices`, and
`scripts/verify_spend.py` prove conversion, frozen metadata, migration rollback,
cross-process coalescing, restart, denial, cancellation, usage rollback and
independent effect approval. The synthetic fixture is not market pricing or
model-quality evidence. M5 JavaScript/Durable Replay remains a separate gate.
