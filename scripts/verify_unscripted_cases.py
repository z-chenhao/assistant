#!/usr/bin/env python3
"""Offline production-CLI evidence for the unscripted entry; never semantic scores."""
import argparse
from contextlib import contextmanager
from http.server import ThreadingHTTPServer
import json
from pathlib import Path
import tempfile
import threading
import time

from fixture_pool import Scope
from release_receipts import PACKET_FIELDS, Receipts, hashed, require, trial
from unscripted_cases import ROOT, ProductionHarness, file_receipt, rows
from verify_local_provider import Endpoint


class DeadlineEndpoint(Endpoint):
    def do_POST(self):
        if self.server.mode == 'stall':
            self.server.stalled += 1
            time.sleep(5)
        super().do_POST()


@contextmanager
def endpoint():
    # Reuse the maintained HTTP peer; no new provider or response plans.
    server = ThreadingHTTPServer(('127.0.0.1', 0), DeadlineEndpoint)
    server.stalled = 0
    server.requests, server.mode = [], 'success'
    thread = threading.Thread(target=server.serve_forever)
    thread.start()
    try:
        yield server
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=10)
        require(not thread.is_alive(), 'loopback listener leaked')


def verify(binary, evidence):
    report = dict(kind='loopback_mechanism', semantic_scores=None, checks=[], raw_evidence=[])
    raw = Path(tempfile.mkdtemp(prefix=evidence.stem+'-raw-', dir=evidence.parent))
    scope = Scope()
    try:
        with tempfile.TemporaryDirectory(prefix='erii-unscripted-') as tmp, endpoint() as server:
            base = Path(tmp)
            def prepared(name, ready=True):
                root = base/name
                root.mkdir()
                config = root/'config.json'
                config.write_text(json.dumps(dict(version=1, llm=dict(provider_bridge_command='node',
                    provider_bridge_script=str(ROOT/'provider-bridge/bridge.mjs'), provider_bridge_version='pi-ai-1.3.0-v1'))))
                h = ProductionHarness(binary, root, config, 30)
                try:
                    status = h.command('status')
                    if ready:
                        p = h.command('model', 'add', f'http://127.0.0.1:{server.server_port}/v1', 'fixture-model')
                        h.command('model', 'test', p['profile_id'])
                        status = h.command('status')
                        p = h.command('model', 'list')[0]
                        h.command('model', 'select', p['profile_id'], str(p['version']), str(status['config_version']))
                finally:
                    h.cleanup()
                return root, config

            def command(root, config, action, name, *extra, code=0):
                out = raw/name
                # Environment sanitation is identical to the production driver.
                h = ProductionHarness(binary, root, config, 30)
                try:
                    p = scope.run(['python3', str(ROOT/'scripts/unscripted_cases.py'), action,
                        '--binary', str(binary), '--root', str(root), '--config', str(config),
                        '--output', str(out), *extra], cwd=ROOT, env=h.env, capture_output=True, text=True, timeout=40)
                finally:
                    h.cleanup()
                require(p.returncode == code, f'{name}: expected exit {code}, observed {p.returncode}: {p.stderr}')
                return out

            def plan(root, config, name, seconds='20'):
                out = command(root, config, 'freeze', name, '--case', 'GC-09',
                              '--execution-kind', 'loopback_mechanism', '--seconds', seconds)
                p = out/'plan.json'
                return p, hashed(p.read_bytes())

            root, config = prepared('no-profile', ready=False)
            before = len(server.requests)
            out = command(root, config, 'freeze', 'setup-required', code=2)
            require(json.loads((out/'plan.json').read_text())['status'] == 'setup_required', 'implicit profile')
            require(len(server.requests) == before and not rows(root, 'run'), 'setup invoked model')
            report['checks'].append('setup_required_without_model_requests')

            root, config = prepared('success')
            p, sha = plan(root, config, 'success-plan')
            start = len(server.requests)
            out = command(root, config, 'run', 'success', '--plan', str(p), '--plan-sha256', sha)
            observation = json.loads((out/'observation.json').read_text())
            state = json.loads((out/'native.json').read_text())
            packet = json.loads((out/'judge-input.json').read_text())
            require(observation['status'] == 'observed_fragment' and not observation['eligible_for_grading']
                    and observation['final_score'] is None, 'loopback earned semantic score')
            require(all(observation['assertions'].values()) and state['tools'] and state['provider_usage'], 'native pipeline absent')
            require(set(packet) == PACKET_FIELDS, 'Judge leakage')
            require('required_text' not in json.dumps(packet) and 'forbidden_text' not in json.dumps(packet), 'Gold leaked')
            requests = [r['body'] for r in server.requests[start:] if r.get('body')]
            require(requests and all('mcp__fixture__echo' not in json.dumps(r) for r in requests), 'scripted bridge used')
            require(all('-test-bridge' not in c['args'] for c in observation['commands']), 'test bridge used')
            require(len(state['generations']) == len({g['generation_id'] for g in state['generations']}), 'duplicate generation')
            # The existing strict consumer must refuse this fragment even with
            # a hash-valid file; it cannot be promoted by a Judge score.
            ref = dict(path='observation.json', sha256=hashed((out/'observation.json').read_bytes()))
            try:
                trial(Receipts(out, {}), ref, {}, {}, {}, {})
            except ValueError:
                pass
            else:
                raise RuntimeError('strict release consumer accepted fragment')
            report['checks'].append('production_cli_native_tool_usage_and_restricted_judge_packet')
            report['checks'].append('strict_release_consumer_rejects_loopback_fragment')

            root, config = prepared('drift')
            p, sha = plan(root, config, 'drift-plan')
            config.write_text(config.read_text()+'\n')
            out = command(root, config, 'run', 'config-drift', '--plan', str(p), '--plan-sha256', sha, code=1)
            require(not rows(root, 'run') and 'drift' in json.loads((out/'observation.json').read_text())['error'], 'drift reached model')
            report['checks'].append('configuration_drift_rejected_before_ingress')

            root, config = prepared('plan-drift')
            p, sha = plan(root, config, 'bytes-plan')
            p.write_text(p.read_text()+'\n')
            command(root, config, 'run', 'plan-drift', '--plan', str(p), '--plan-sha256', sha, code=1)
            require(not rows(root, 'run'), 'changed plan accepted')
            report['checks'].append('plan_bytes_drift_rejected_before_ingress')

            root, config = prepared('profile-drift')
            p, sha = plan(root, config, 'profile-plan')
            h = ProductionHarness(binary, root, config, 30)
            try:
                candidate = h.command('model', 'add', f'http://127.0.0.1:{server.server_port}/v1', 'other-model')
                h.command('model', 'test', candidate['profile_id'])
                candidate = next(p for p in h.command('model', 'list') if p['profile_id'] == candidate['profile_id'])
                status = h.command('status')
                h.command('model', 'select', candidate['profile_id'], str(candidate['version']), str(status['config_version']))
            finally:
                h.cleanup()
            out = command(root, config, 'run', 'profile-drift', '--plan', str(p), '--plan-sha256', sha, code=1)
            require(not rows(root, 'run'), 'changed active model accepted')
            report['checks'].append('active_model_drift_rejected_before_ingress')

            root, config = prepared('unsupported')
            out = command(root, config, 'freeze', 'unsupported', '--case', 'GC-11', code=2)
            require(json.loads((out/'plan.json').read_text())['status'] == 'ineligible' and not rows(root, 'run'), 'missing adapter passed')
            report['checks'].append('missing_adapter_explicitly_ineligible')

            for mode, seconds in (('auth', '20'), ('partial', '20'), ('stall', '2')):
                root, config = prepared(mode)
                p, sha = plan(root, config, mode+'-plan', seconds)
                server.mode = mode
                out = command(root, config, 'run', mode, '--plan', str(p), '--plan-sha256', sha, code=1)
                value = json.loads((out/'observation.json').read_text())
                require(value['status'] == 'failed' and value['cleanup']['owned_processes_joined'], 'failure/cleanup lost')
                require(value['final_score'] is None and not value['eligible_for_grading'], 'failed trial graded')
                if mode == 'stall':
                    require(server.stalled > 0, 'timeout did not reach the HTTP boundary')
                server.mode = 'success'
                report['checks'].append(mode+'_failure_propagates_with_cleanup')
            require(all(p.exists() for p in base.iterdir()), 'operator state removed')
        report['cleanup'] = dict(operator_fixture_roots_removed=True, loopback_joined=True, children_joined=True)
        # Keep complete observations locally, with byte refs for independent readback.
        for path in sorted(raw.rglob('*.json')):
            report['raw_evidence'].append(dict(path=str(path.relative_to(evidence.parent)), sha256=hashed(path.read_bytes())))
        evidence.write_text(json.dumps(report, ensure_ascii=False, indent=2)+'\n')
        return report
    finally:
        scope.cancel()
        scope.finish()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--evidence', type=Path, required=True)
    args = parser.parse_args()
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    args.evidence.unlink(missing_ok=True)
    report = verify(args.binary.resolve(), args.evidence.resolve())
    print(json.dumps(dict(checks=report['checks'], semantic_scores=None)))


if __name__ == '__main__':
    main()
