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


def run(root):
    selected = sorted({test for tests in TESTS.values() for test in tests})
    command = ['go','test','-race','-count=1','-json','-run','^('+'|'.join(selected)+')$', './internal/agent/sqlite']
    result = subprocess.run(command,cwd=root,text=True,capture_output=True,timeout=300)
    events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
    fakes = exercise()
    passed = judge(events,result.returncode) and all(all(s['assertions'].values()) for s in fakes.values())
    return dict(passed=passed, command=command, returncode=result.returncode,events=events,stderr=result.stderr,
                requirements={sc:dict(status='implemented_core',tests=tests) for sc,tests in TESTS.items()},
                boundary_fakes=fakes, scope='L0 core contracts plus explicit M6-only Fake contracts; no full M6 conformance claim')
