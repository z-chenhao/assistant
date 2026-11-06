import test from 'node:test';
import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { readFile, readdir, mkdir, mkdtemp, writeFile, rm, symlink } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { publicAddress, pinnedFetch } from '../src/web-access-network.mjs';
import { exportRepositories } from '../src/web-access-artifacts.mjs';

const vendor = new URL('../src/vendor/web-access/', import.meta.url);
test('pinned Pi source only changes files recorded as host adaptations', async () => {
  const manifest = JSON.parse(await readFile(new URL('upstream.json', vendor), 'utf8'));
  const patches = JSON.parse(await readFile(new URL('adaptations.json', vendor), 'utf8'));
  for (const [name, original] of Object.entries(manifest.sha256)) {
    const hash = createHash('sha256').update(await readFile(new URL(name, vendor))).digest('hex');
    assert.equal(hash, patches[name]?.sha256 ?? original, name);
  }
  for (const name of await readdir(vendor)) if (name.endsWith('.ts')) assert.ok(manifest.sha256[name], name);
});
test('public connections reject reserved and mapped private addresses', () => {
  for (const ip of ['127.0.0.1', '10.1.2.3', '169.254.169.254', '198.18.0.1', '::1', 'fc00::1', '::ffff:127.0.0.1']) assert.equal(publicAddress(ip), false, ip);
  assert.equal(publicAddress('1.1.1.1'), true);
  assert.equal(publicAddress('2606:4700:4700::1111'), true);
  assert.equal(publicAddress('198.18.0.1', ['198.18.0.0/15']), true);
});
test('DNS answers at connection creation are checked and pinned', async () => {
  let dispatched = false;
  const f = pinnedFetch(async () => {}, () => null, {}, async () => { dispatched = true; }, async () => [{ address: '127.0.0.1', family: 4 }]);
  await assert.rejects(f('https://rebinding.example', {}), /WEB_ADDRESS_BLOCKED/);
  assert.equal(dispatched, false);
  let dispatcher;
  const pinned = pinnedFetch(async (_url, options) => {
    assert.deepEqual(await options.lookup(), [{ address: '1.1.1.1', family: 4 }]);
  }, () => null, {}, async (_url, init) => { dispatcher = init.dispatcher; return new Response('ok'); }, async () => [{ address: '1.1.1.1', family: 4 }]);
  assert.equal(await (await pinned('https://public.example', {})).text(), 'ok');
  await dispatcher.close();
});
test('repository export persists bounded snapshots and rejects external paths', async () => {
  const root = await mkdtemp(join(tmpdir(), 'steward-repository-test-'));
  try {
    const repo = join(root, 'github-repos', 'runtime-test', 'repo');
    await mkdir(join(repo, '.git'), { recursive: true });
    await writeFile(join(repo, 'README.md'), 'repository evidence');
    const stored = [{ urls: [{ content: `Repository cloned to: ${repo}\nUse \`read\` and \`bash\` tools at the path above to explore further.` }] }];
    const exported = await exportRepositories(root, { content: [] }, stored);
    assert.equal(exported.attachments.length, 1);
    assert.ok(Buffer.from(exported.attachments[0].data, 'base64').length > 0);
    assert.ok(!exported.stored[0].urls[0].content.includes(repo));
    await symlink(root, join(repo, 'outside'));
    await assert.rejects(exportRepositories(root, {}, [{ urls: [{ content: `Repository cloned to: ${root}` }] }]), /WEB_REPOSITORY_SCOPE/);
  } finally { await rm(root, { recursive: true, force: true }); }
});

test('secret references resolve only in headers and reflected credentials are redacted', async () => {
  const { credentialHeaders, redactCredentials } = await import('../src/web-access-network.mjs');
  process.env.STEWARD_TEST_KEY = 'Bearer private-fixture-credential';
  try {
    assert.equal(credentialHeaders({ authorization: '$STEWARD_TEST_KEY' }).get('authorization'), process.env.STEWARD_TEST_KEY);
    assert.deepEqual(redactCredentials({ [process.env.STEWARD_TEST_KEY]: ['reflected '+process.env.STEWARD_TEST_KEY] }, [process.env.STEWARD_TEST_KEY]), { '[redacted]': ['reflected [redacted]'] });
  } finally { delete process.env.STEWARD_TEST_KEY; }
});
