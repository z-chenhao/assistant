# Isolated cancellation experiment

These two workflow replicas preserve the Required/control and candidate workload
commands and job topology from `7300e9506ee8b1342e9c3730444b06f0ace415a2`.
They run only for `docs/ci-cancel-probe.md` pushes on this diagnostic branch.
The production PR and its five complete before/after pairs stay unchanged.

Freeze five ordinary doc-only workload commits followed by a sixth doc-only
commit whose message contains `[stop-cancel-profile]`. Push the preparation
commit first; do not push the frozen workload heads manually. Run
`python3 scripts/ci_cancel_cohort.py --head FULL_SHA_1 --head FULL_SHA_2 --head FULL_SHA_3 --head FULL_SHA_4 --head FULL_SHA_5 --head FULL_STOP_SHA --output evidence/cancellation-cohort.json`.

The controller waits until both real Code Mode phases have been active for
thirty observed seconds and at least ten jobs are running, then issues exactly
one recorded superseding push. An unknown push, early test failure, ambiguous
run or unexpected attempt stops without retry. All source/attempt/job API
snapshots and push start/ack intervals remain inspectable. The final stop-marker
push cancels the fifth sample and admits no new workload. It never changes the
Required workflow, PR readiness, protection or test inventory.

Cancelled samples measure job stopping; they are not passing test evidence.
Signal receipt, owned-group cleanup and state/port cleanup must be assessed
separately using phase artifacts and the maintained real-Core cancellation probe.
Client/API clock skew and one-second API timestamp precision remain limitations.
