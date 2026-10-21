import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp,writeFile,mkdir,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {setTimeout as delay} from 'node:timers/promises';
import {waitForDevToolsPort} from './readiness.mjs';

for (const initial of [null,'','321','0\n','invalid\n','65536\n']) {
 test(`CDP readiness waits for a valid published port after ${JSON.stringify(initial)}`,async()=>{
  const root=await mkdtemp(join(tmpdir(),'erii-cdp-'));
  let publication;
  try {
   if (initial!==null) await writeFile(join(root,'DevToolsActivePort'),initial);
   const ready=waitForDevToolsPort(root);
   publication=delay(75).then(()=>writeFile(join(root,'DevToolsActivePort'),'32123\n/devtools/browser/fixture\n'));
   assert.equal(await ready,32123);
  } finally {
   await publication;
   await rm(root,{recursive:true,force:true});
  }
 });
}

test('CDP readiness fails by its deadline and propagates filesystem errors',async()=>{
 const root=await mkdtemp(join(tmpdir(),'erii-cdp-'));
 try {
  await assert.rejects(waitForDevToolsPort(root,50),/doctor: Chromium CDP port not ready/);
  await mkdir(join(root,'DevToolsActivePort'));
  await assert.rejects(waitForDevToolsPort(root),{code:'EISDIR'});
 } finally {await rm(root,{recursive:true,force:true});}
});
