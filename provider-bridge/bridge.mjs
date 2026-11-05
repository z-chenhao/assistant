// Compatibility entry point for existing deployments. The protocol and model
// implementation run in Steward's Go binary; no JavaScript model SDK is loaded.
import {spawn} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const child = spawn(fileURLToPath(new URL('../bin/steward', import.meta.url)), ['-model-bridge'], {stdio: ['pipe', 'pipe', 'inherit']});
process.stdin.pipe(child.stdin);
child.stdout.pipe(process.stdout);
child.stdin.on('error', () => {});
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => { child.stdin.end(); child.kill(signal); });
child.once('error', () => { process.exitCode = 1; });
child.once('exit', (code, signal) => { process.exitCode = signal ? 1 : (code ?? 1); });
