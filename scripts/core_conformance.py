"""Exact requirement → executed test mapping; absence/skip is a failure."""
import json
import os

from ci_race import cpu_budget, partitions
from fixture_pool import run_cases, run as fixture_run

TESTS = {
    'SC-01': ['TestChildDirectToolRootChildGrandchild', 'TestChildAtomicAdmissionReplayAndBounds'],
    'SC-02': ['TestChildStorageRejectsAncestryAndCapabilityExpansion', 'TestChildDoesNotReceiveAppendedTaskInstructions'],
    'SC-03': ['TestCodeReplayFrozenRequestAndAtomicAdmission', 'TestCodeManifestCannotChangeDuringRecovery'],
    'SC-04': ['TestTaskControlPauseFencesLateWritesAndContinueNewRoot', 'TestTaskControlAppendWaitsOutsideTransactionForPauseBoundary', 'TestTaskControlPauseCancelsSafeToolBeforeNextStep', 'TestSteeringAfterCompactionBeforeGenerationAdmission', 'TestTaskAppendAfterCompactionBeforeGenerationAdmission'],
    'SC-05': ['TestTaskControlCancelTreeAndInvocationOutcomes', 'TestCodeClaimWakeAndOldCallbackFencing', 'TestChildRejectsRealPrivateArtifactAndLateEpochResult'],
    'SC-06': ['TestToolApprovalWrongPrincipalHashDuplicateAndFrozenAdmission', 'TestToolApprovalExpiryDenialAndArgumentValidationNoEffect'],
    'SC-07': ['TestToolReplayPoliciesAndEvidence', 'TestToolTimeoutAndBusinessErrorsDoNotRetry', 'TestToolUnknownReconcilesOnlyFromAuthoritativeEvidence'],
    'SC-08': ['TestCuratorEvidenceDedupEncryptionAndForget', 'TestFreshRememberRetainsIndependentProvenance', 'TestForgetRevokesFrozenPendingGeneration'],
    'SC-09': ['TestArtifactStoreContractScopeIntegrityAndTombstone', 'TestArtifactTamperSymlinkAndTraversal'],
    'SC-10': ['TestSchedulerFairSecondTaskAndDurableTicket', 'TestSchedulerActivityCountsDoNotNotify'],
    'SC-11': ['TestComputerIdentityRecoveryArbitrationAndFencing', 'TestComputerDesktopBarrierAppliesAcrossAppAndRead', 'TestComputerApprovalControlledLeaseResumeAndUnknownNeverDispatchesAgain'],
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
    budget = cpu_budget()
    workers = 4 if budget >= 4 else 2 if budget >= 2 else 1
    def drive(names):
        # Each root owns its stores. Reuse audited partitions and owned process
        # cleanup, with one CPU/package per worker and the original deadline.
        command = ['go','test','-race','-count=1','-json','-p=1','-run','^('+'|'.join(names)+')$', './internal/agent/sqlite', './internal/memory/sqlite']
        return fixture_run(command,cwd=root,env=dict(os.environ,GOMAXPROCS='1'),
                           text=True,capture_output=True,timeout=300)
    results = run_cases(partitions(selected, workers), drive, workers)
    events = [json.loads(line) for result in results for line in result.stdout.splitlines() if line.startswith('{')]
    returncode = next((result.returncode for result in results if result.returncode), 0)
    passed = judge(events,returncode)
    return dict(passed=passed, commands=[result.args for result in results], workers=workers, gomaxprocs=1,
                returncode=returncode,events=events,stderr=''.join(result.stderr for result in results),
                requirements={sc:dict(status='implemented_core',tests=tests) for sc,tests in TESTS.items()},
                boundary_fakes={}, scope='SC-01–12 against implemented stores and executors; external providers remain explicit deterministic Fakes')
