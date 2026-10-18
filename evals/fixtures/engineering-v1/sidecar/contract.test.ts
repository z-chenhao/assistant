import { readFileSync } from 'node:fs';
import assert from 'node:assert/strict';
import test from 'node:test';
test('minimal Child request/result, no model or Run IDs', () => {
 const source=readFileSync(new URL('./contract.ts',import.meta.url),'utf8');
 assert.match(source,/task: string/);assert.match(source,/readonly text: string/);
 for(const forbidden of ['model:', 'runId:', 'call_key:', 'timeout:']) assert.ok(!source.includes(forbidden));
});
