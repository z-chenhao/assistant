import assert from 'node:assert/strict';
import test from 'node:test';
import {buildSync} from 'esbuild';
import {runInNewContext} from 'node:vm';
const source=buildSync({entryPoints:[new URL('src/lib/random-id.ts',import.meta.url).pathname],bundle:true,format:'cjs',write:false}).outputFiles[0].text;
function load(crypto){const module={exports:{}};runInNewContext(source,{module,exports:module.exports,crypto});return module.exports.randomUUID}
test('secure contexts use the native UUID with its Crypto receiver',()=>{
 const crypto={randomUUID(){assert.equal(this,crypto);return 'native-id'},getRandomValues(){throw Error('unexpected fallback')}};
 assert.equal(load(crypto)(),'native-id');
});
test('HTTP fallback retains UUIDv4 version, variant and remaining entropy',()=>{
 const crypto={getRandomValues(bytes){assert.equal(this,crypto);assert.equal(bytes.length,16);return bytes.fill(255)}};
 assert.equal(load(crypto)(),'ffffffff-ffff-4fff-bfff-ffffffffffff');
 let call=0;const next=load({getRandomValues(bytes){return bytes.fill(call++)}});
 assert.equal(next(),'00000000-0000-4000-8000-000000000000');
 assert.equal(next(),'01010101-0101-4101-8101-010101010101');
});
test('unavailable cryptographic entropy is never replaced with a weak random source',()=>{
 const next=load({getRandomValues(){throw Error('entropy unavailable')}});
 assert.throws(next,/entropy unavailable/);
});
