"""Exact requirement → executed test mapping; absence/skip is a failure."""
import json
import subprocess
from core_boundaries import exercise

TESTS = {
    'SC-01': ['TestChildDirectToolRootChildGrandchild', 'TestChildAtomicAdmissionReplayAndBounds'],
    'SC-02': ['TestChildStorageRejectsAncestryAndCapabilityExpansion', 'TestChildDoesNotReceiveAppendedTaskInstructions'],
    'SC-03': ['TestCodeReplayFrozenRequestAndAtomicAdmission', 'TestCodeManifestCannotChangeDuringRecovery'],
    'SC-04': ['TestTaskControlPauseFencesLateWritesAndContinueNewRoot', 'TestTaskControlAppendWaitsOutsideTransactionForPauseBoundary'],
    'SC-05': ['TestTaskControlCancelTreeAndInvocationOutcomes', 'TestCodeClaimWakeAndOldCallbackFencing', 'TestChildRejectsRealPrivateArtifactAndLateEpochResult'],
    'SC-06': ['TestToolApprovalWrongPrincipalHashDuplicateAndFrozenAdmission', 'TestToolApprovalExpiryDenialAndArgumentValidationNoEffect'],
    'SC-07': ['TestToolReplayPoliciesAndEvidence', 'TestToolTimeoutAndBusinessErrorsDoNotRetry', 'TestToolUnknownReconcilesOnlyFromAuthoritativeEvidence'],
    'SC-09': ['TestArtifactStoreContractScopeIntegrityAndTombstone', 'TestArtifactTamperSymlinkAndTraversal'],
    'SC-10': ['TestSchedulerFairSecondTaskAndDurableTicket', 'TestSchedulerActivityCountsDoNotNotify'],
    'SC-12': ['TestChildFrozenModelAndCapabilitiesAfterProfileSwitch', 'TestDeliveryClaimFencesLateAckAndQueriesAcceptedSend'],
}


def judge(events, returncode):
    expected = {test for tests in TESTS.values() for test in tests}
    outcomes = {test: [] for test in expected}
    for event in events:
        if event.get('Test') in outcomes and event.get('Action') in ('pass','fail','skip'):
            outcomes[event['Test']].append(event['Action'])
    return returncode == 0 and all(outcomes[name] == ['pass'] for name in expected)


def run(root, cpu_profile=None):
    selected = sorted({test for tests in TESTS.values() for test in tests})
    command = ['go','test','-race','-count=1','-json','-run','^('+'|'.join(selected)+')$', './internal/agent/sqlite']
    if cpu_profile is not None:
        command[5:5] = ['-cpuprofile', str(cpu_profile), '-o', str(cpu_profile.parent / 'conformance.test')]
    result = subprocess.run(command,cwd=root,text=True,capture_output=True,timeout=300)
    events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
    fakes = exercise()
    passed = judge(events,result.returncode) and all(all(s['assertions'].values()) for s in fakes.values())
    return dict(passed=passed, command=command, returncode=result.returncode,events=events,stderr=result.stderr,
                requirements={sc:dict(status='implemented_core',tests=tests) for sc,tests in TESTS.items()},
                boundary_fakes=fakes, scope='L0 core contracts plus explicit M6-only Fake contracts; no full M6 conformance claim')


def main():
    import argparse
    import hashlib
    import os
    from pathlib import Path
    import resource
    import sys
    import time
    from verify_suite import source_state, runner_hardware
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--evidence', type=Path, required=True)
    parser.add_argument('--cpu-profile', type=Path)
    args = parser.parse_args()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    if args.cpu_profile is not None:
        args.cpu_profile = args.cpu_profile.resolve()
        args.cpu_profile.parent.mkdir(parents=True, exist_ok=True)
    before = source_state()
    started = time.monotonic()
    result = run(Path(__file__).resolve().parents[1], args.cpu_profile)
    usage = resource.getrusage(resource.RUSAGE_CHILDREN)
    result.update(version=1, **before, source_stable=before == source_state(),
                  hardware=runner_hardware(), wall_seconds=time.monotonic()-started,
                  child_cpu_seconds=usage.ru_utime+usage.ru_stime,
                  peak_reaped_child_rss_bytes=usage.ru_maxrss*(1 if sys.platform=='darwin' else 1024),
                  rss_scope='maximum reaped-child RSS; not a concurrent process-tree sum',
                  gc=os.environ.get('GOGC'), memory_limit=os.environ.get('GOMEMLIMIT'),
                  image_os=os.environ.get('ImageOS'), image_version=os.environ.get('ImageVersion'))
    if args.cpu_profile is not None:
        result['test_binary_sha256'] = hashlib.sha256((args.cpu_profile.parent/'conformance.test').read_bytes()).hexdigest()
    result['passed'] = result['passed'] and result['source_stable'] and not result['source_dirty']
    args.evidence.write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps({k:v for k,v in result.items() if k not in ('events','boundary_fakes','stderr','requirements')}))
    return 0 if result['passed'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
