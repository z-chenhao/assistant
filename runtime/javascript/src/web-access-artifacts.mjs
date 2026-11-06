import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { realpath, lstat } from 'node:fs/promises';
import { join } from 'node:path';
const execute = promisify(execFile);

// Pi exposes clone paths to its shell tools. Steward exports complete bounded
// snapshots instead, so a result cannot point at a deleted invocation directory.
export async function exportRepositories(root, result, stored, signal) {
  const realRoot = await realpath(root);
  const paths = new Set();
  for (const item of stored) for (const page of item.urls ?? []) {
    const match = /^Repository cloned to: (.+)$/m.exec(page.content ?? '');
    if (match) paths.add(match[1]);
  }
  const replacements = new Map();
  const attachments = [];
  for (const path of paths) {
    const real = await realpath(path);
    if (!real.startsWith(join(realRoot, 'github-repos') + '/') || (await lstat(path)).isSymbolicLink()) throw new Error('WEB_REPOSITORY_SCOPE');
    const name = `github-repository-${attachments.length + 1}.tar.gz`;
    const { stdout } = await execute('/usr/bin/tar', ['-czf', '-', '--exclude=.git', '-C', real, '.'], { encoding: 'buffer', maxBuffer: 16 * 1024 * 1024, signal });
    attachments.push({ name, mediaType: 'application/gzip', data: stdout.toString('base64') });
    replacements.set(path, `[attached snapshot: ${name}]`);
  }
  function rewrite(value) {
    if (typeof value === 'string') {
      for (const [path, label] of replacements) value = value.replaceAll(path, label);
      return value.replaceAll('Use `read` and `bash` tools at the path above to explore further.', 'Use the attached repository snapshot in an authorized coding workspace to explore further.');
    }
    if (Array.isArray(value)) return value.map(rewrite);
    if (value && typeof value === 'object') return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, rewrite(item)]));
    return value;
  }
  return { result: rewrite(result), stored: rewrite(stored), attachments };
}
