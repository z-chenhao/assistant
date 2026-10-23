# GC-09 agenda verification

This is scripted whole-case mechanics for Golden §8.10, not v1 acceptance.
The existing `m1-v2` lock binds the complete source, scoring policy and Golden
document. No source, fixture, policy or hard-failure assertion is changed.

```sh
make build
python3 scripts/run_cases.py --binary bin/erii --evidence evidence/cases.json
python3 scripts/prepare_release_eval.py --input evidence/cases.json --evidence evidence/agenda-judge.json
python3 -m unittest discover -s scripts -p test_agenda_case.py
python3 scripts/release_audit.py --evidence evidence/audit.json
make release-audit
```

Launch creates isolated stores for every existing M1 variant. Doctor selects
only the explicitly requested golden Fake. The CLI accepts the full synthetic
world as Conversation input, then the original user event; the real TUI drives
Main, Context Builder, provider requests and normal Tool/delivery persistence.
There is no injected plan or answer in user input. The golden model itself is
an explicitly scripted response, so its output is not model quality evidence.

All six GC-09 variants retain three wordings, two 10–30% noise variants and
one source-key ordering variant. Every generation must contain the unchanged
source and original event. The complete three-calendar/four-email/two-work-item
world includes optional 16:00 sharing and the later T2 deadline. The bounded
numbered Fake agenda must place E1/T1 first before noon, preserve the 10:00
meeting constraint, give a real preparation interval before the 14:00 interview,
and say E4 needs checking without another reply. E2/E3 stay out of the first
two items. At most eight short items are allowed.

Evidence reads Conversation, Runs, generations, frozen Model/Capability
snapshots, Tools, usage, delivery, Task, Automation, Approval and Artifact
stores. There must be one completed Main reply, only safe echo Tools and no
new Task, schedule, approval or Artifact. No mail/calendar connector is enabled
or invoked. Repeating the accepted input through the CLI and working again must
leave the entire readback identical. Cleanup joins owned PTYs/processes and
removes the temporary root; both receipts are required by preview preparation.
The same existing runtime CI gate executes these checks on every affected build.

`test_agenda_case.py` rejects the previous keyword-only agenda, wrong priority,
late deadline/preparation, repeated mail claims, nine-item output, source/input
drift, unexpected effects/state, duplicate completion and replay drift. Judge
preparation independently recomputes both the existing M1 assertions and the new
whole-case assertions, checks the locked six-receipt inventory and cleanup, and
allowlists only source, visible dialogue and assertions. All six previews are
ineligible for grading, with null scores and winner.

The text recognizer is deliberately limited to the scripted numbered fixture;
it is not a general semantic evaluator. This check does not prove real connector
reads, refusal when a real write connector is available, priority reasoning by
an unscripted model, or broader §7 GC-09 concurrent Worker question aggregation
and later Steering. Those obligations, all release thresholds, current L2 and
named human review remain required and inconclusive. The release inventory must
continue to report `release_ready=false`; completion must remain nonzero.
