# ERII-365 shared progress rule repair

Observed on 2026-10-24 UTC. This is a delivery receipt for the shared Multica
skill, not an Erii product capability or release certification.

`ensure_progress` now selects the sole enabled rule among matching markers.
Disabled history no longer causes `DUPLICATE_PROGRESS_RULES`. Multiple enabled
matches still fail closed. If matching history has no enabled rule, the first
historical record is returned without mutation. Expiry, manual suppression and
platform loop limits are not reset. Marker matching and the no-match creation
path, including target and Builder filter, remain unchanged.

## Authority and delivery

The authoritative workspace skill is `agent-delivery`, ID
`fa86952a-e68e-4413-b63b-3a7cceb0a600`, in workspace
`d20328fd-2cd3-4184-a4fc-8ca713fd5bc3`. Its config is `{}` and advertises no Git
origin. No separate maintained helper checkout was found in the inspected local
project and skill directories. All ten installed supporting files initially
matched the workspace's content hashes.

The two files were updated in place with `multica skill files upsert`, preserving
the skill ID and existing Builder/Reviewer bindings. The skill body, config and
other eight supporting files were unchanged. A fresh `skill get --with-content`
matched the candidate bytes and supplied the readback used for verification.
The [patch](progress-selection.patch) freezes the exact source change for Git
review. It is not a second installed helper or a new source of authority.

| File | Existing file ID | New SHA256 |
| --- | --- | --- |
| `scripts/delivery.py` | `319a1bca-bba4-4ac6-be7c-74c7f87f0e1a` | `c8a1b9f650d5724c7840a37b2d26fde7159d2d5115dcf3488296f22ed2bdf44c` |
| `scripts/test_delivery.py` | `ab9211ed-5831-4849-ad5a-3a572d026c31` | `45937ff5a8acd4dd93d12ecbb50c6223d7a4b71acfaf014dc92bce44d1970279` |

## Evidence

The [manifest](manifest.json) records original and new hashes, commands, exit
codes, versions, local artifact sizes and SHA256 values. Full evidence is retained
at its `evidence_root` outside the temporary task checkout. Commands below ran
from the retained `candidate` and, after publication, `readback` directories.

| Check | Observed result |
| --- | --- |
| New mixed-history regression against original helper | Exit 1; four subcases reproduce `DUPLICATE_PROGRESS_RULES` |
| Captured ERII-242 rules replay, both orders | Original stops; fixed helper selects `01a169b1-8859-7aa6-843e-87db54e961f0`; one list call, no writes |
| `python3 -m unittest discover -s scripts -p test_delivery.py` | Exit 0; 90 tests, one pre-existing skip for retired workflow recovery |
| `python3 scripts/model_check.py` | Exit 0; `BOUNDED_SAFETY_PASS`, 204 states; liveness not proven |
| `python3 scripts/delivery.py doctor --self-test` | Exit 0; `DOCTOR_PASS`; also passed against fresh workspace readback |
| Skill creator `quick_validate.py` | Exit 0 using isolated PyYAML 6.0.3; initial system-Python attempt lacked PyYAML and is retained |
| ERII-242 wakeup snapshots before/after | Identical; no consumer rule or task was resumed |

The regressions cover history ordering and list/envelope responses, multiple
enabled rejection, disabled/expired/loop/max-fire records, unrelated markers and
the original creation contract. No Erii product or CI policy was changed. Local
evidence snapshots are verification inputs, not additional runtime installations.

## Independent acceptance still required

The current Builder Run retains its original installed helper. It was not edited.
Workspace readback proves publication, but cannot prove that a later Run receives
the corrected files. Reviewer must hash its actual installed files against the
table before accepting distribution, independently inspect the frozen patch and
run the shared regressions and doctor. A mismatch is an unresolved distribution
gap; do not copy the candidate into the runtime to make this check pass.

CI and independent review for this receipt must bind to its PR head SHA. Neither
this report nor ERII-242's historical CI is a PASS for that head. Only the
coordinator may resume ERII-242 after this issue is Done, preserving its original
PR/branch and requiring a new SHA, CI and review. `release_ready=false`.
