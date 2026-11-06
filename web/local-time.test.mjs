import assert from 'node:assert/strict';
import test from 'node:test';
import {buildSync} from 'esbuild';
import {runInNewContext} from 'node:vm';
const source=buildSync({entryPoints:[new URL('src/local-time.ts',import.meta.url).pathname],bundle:true,format:'cjs',write:false}).outputFiles[0].text;
const module={exports:{}};runInNewContext(source,{module,exports:module.exports});
const {formatLocalTime}=module.exports;
test('viewer zone controls the clock and local calendar day, including fractional offsets',()=>{
 const raw='2026-11-05T18:30:00Z';
 const shanghai=formatLocalTime(raw,'message','en','Asia/Shanghai');
 assert.equal(shanghai.text,'02:30:00');assert.equal(shanghai.day,'2026-11-06');
 assert(!/GMT|UTC|Shanghai/.test(shanghai.full));
 const newYork=formatLocalTime(raw,'message','en','America/New_York');
 assert.equal(newYork.text,'13:30:00');assert.equal(newYork.day,'2026-11-05');
 assert.equal(formatLocalTime(raw,'message','zh-CN','Asia/Kathmandu').text,'00:15:00');
});
test('repeated daylight-saving clock times display to seconds without zone labels',()=>{
 const before=formatLocalTime('2026-11-01T05:30:00Z','message','en','America/New_York');
 const after=formatLocalTime('2026-11-01T06:30:00Z','message','en','America/New_York');
 assert.equal(before.text,'01:30:00');assert.equal(after.text,'01:30:00');
 assert.equal(before.full,after.full);assert(!/GMT|UTC|New_York/.test(before.full));
});
test('midnight uses 00:00 and invalid timestamps do not become current times',()=>{
 assert.equal(formatLocalTime('2026-11-06T00:00:00Z','message','en','Etc/GMT').text,'00:00:00');
 assert.equal(formatLocalTime('','message'),null);assert.equal(formatLocalTime('not-a-date','message'),null);
 assert(!formatLocalTime('2026-11-06T00:00:00Z','datetime','en','UTC').full.includes('UTC'));
});
test('recent dates use human labels and older dates use ISO order in the viewer zone',()=>{
 const now=new Date('2026-11-06T16:30:00Z'),zone='Asia/Shanghai';
 assert.equal(formatLocalTime('2026-11-06T16:00:00Z','datetime','en',zone,now).text,'00:00:00');
 assert.equal(formatLocalTime('2026-11-05T16:00:00Z','datetime','en',zone,now).text,'Yesterday 00:00:00');
 assert.equal(formatLocalTime('2026-11-04T16:00:00Z','datetime','en',zone,now).text,'Thursday 00:00:00');
 assert.equal(formatLocalTime('2026-10-20T16:00:00Z','datetime','zh-CN',zone,now).text,'2026-10-21 00:00:00');
 assert.equal(formatLocalTime('2026-11-05T16:00:00Z','date','zh-CN',zone,now).text,'昨天');
 assert.equal(formatLocalTime('2026-10-20T16:00:00Z','datetime','en',zone,now).full,'2026-10-21 00:00:00');
});
