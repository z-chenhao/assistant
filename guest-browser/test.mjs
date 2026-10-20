import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp, realpath, readFile, copyFile, rm, symlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {createServer} from 'node:http';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {setTimeout as delay} from 'node:timers/promises';
import {fileURLToPath} from 'node:url';
import {chromium} from 'playwright-core';
const exec=promisify(execFile);
const here=fileURLToPath(new URL('.',import.meta.url));
const source=await readFile(new URL('../internal/computer/macosvm/browser.mjs',import.meta.url),'utf8');
const module=resolve(here,'node_modules/playwright-core/index.mjs');
const transfer=resolve(here,'node_modules/playwright-core/erii-transfer.py');

test('real Chromium: inventory, locator, input, navigation expiry, download and scoped chooser', {timeout:60000}, async()=>{
 const root=await realpath(await mkdtemp(join(tmpdir(),'erii-browser-')));
 const server=createServer((req,res)=>{
  if(req.url==='/download'){res.writeHead(200,{'Content-Disposition':'attachment; filename=fixture.txt','Content-Type':'text/plain'});res.end('fixture download'+'x'.repeat(300000));return;}
  res.end('<title>Fixture</title><input id="field" aria-label="Fixture text"><button id="button" onclick="document.querySelector(\'#status\').textContent=\'clicked:\'+event.detail" oncontextmenu="document.querySelector(\'#status\').textContent=\'context\';return false">Click</button><div id="status">ready</div><a id="download" href="/download">Download</a><button id="upload" onclick="document.querySelector(\'#file\').click()">Upload</button><input type="file" id="file" onchange="document.querySelector(\'#status\').textContent=this.files[0].name">');
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const endpoint=`http://127.0.0.1:${server.address().port}`;
 let browserContext;
 try{
  await copyFile(new URL('../internal/computer/macosvm/transfer.py',import.meta.url),transfer);
  browserContext=await chromium.launchPersistentContext(join(root,'profile'),{headless:true,args:['--remote-debugging-address=127.0.0.1','--remote-debugging-port=0']});
  let port;for(let n=0;n<200;n++){try{port=Number((await readFile(join(root,'profile/DevToolsActivePort'),'utf8')).split('\n')[0]);break;}catch{await delay(25);}}
  assert.ok(port,'doctor: Chromium CDP not ready');
  const call=async(method,args={})=>JSON.parse((await exec(process.execPath,['--input-type=module','-e',source,JSON.stringify({module,port,root:join(root,'EriiTransfer'),method,args})],{timeout:15000,maxBuffer:12<<20})).stdout);
  const inventory=await call('inventory');assert.equal(inventory.browsers[0].id,'chromium');
  const tabId=inventory.tabs[0].id;
  assert.equal((await call('goto',{tabId,url:endpoint})).effect,'dispatched');
  const capture=await call('observe',{tabId,screenshot:true});assert.ok(capture.screenshot_png_b64);assert.ok(capture.tree_markdown.includes('Fixture text'));assert.ok(capture.elements.every(e=>e.element_token));
  const navigation=capture.navigation;
  assert.equal((await call('playwright.count',{tabId,selector:'#field',expectedNavigation:navigation})).value,1);
  assert.equal((await call('playwright.fill',{tabId,selector:'#field',value:'verified',expectedNavigation:navigation})).effect,'dispatched');
  assert.equal((await call('playwright.click',{tabId,selector:'#button'})).effect,'dispatched');
  assert.equal((await call('playwright.textContent',{tabId,selector:'#status'})).value,'clicked:1');
  const buttonView=await call('observe',{tabId,screenshot:true});
  const buttonToken=buttonView.elements.find(e=>e.label==='Click').element_token;
  await call('click',{tabId,element_token:buttonToken,options:{count:2}});
  assert.equal((await call('playwright.textContent',{tabId,selector:'#status'})).value,'clicked:2');
  const geometryPeer=await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
  const box=await geometryPeer.contexts()[0].pages()[0].locator('#button').boundingBox();
  await geometryPeer.close();
  await call('click',{tabId,x:box.x+box.width/2,y:box.y+box.height/2,geometry:buttonView.geometry,options:{button:'right'}});
  assert.equal((await call('playwright.textContent',{tabId,selector:'#status'})).value,'context');
  assert.equal((await call('playwright.download',{tabId,selector:'#download',path:'downloads/fixture'})).effect,'dispatched');
  assert.equal(await readFile(join(root,'EriiTransfer/downloads/fixture'),'utf8'),'fixture download'+'x'.repeat(300000));
  assert.equal((await call('playwright.upload',{tabId,selector:'#upload',path:'downloads/fixture'})).effect,'dispatched');
  assert.equal((await call('playwright.textContent',{tabId,selector:'#status'})).value,'fixture');
  const observed=await call('observe',{tabId});
  const token=observed.elements.find(e=>e.label==='Fixture text').element_token;
  const peer=await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
  await peer.contexts()[0].pages()[0].evaluate(()=>{const old=document.querySelector('#field');old.replaceWith(old.cloneNode(true));});
  await peer.close();
  assert.equal((await call('setValue',{tabId,element_token:token,value:'stale'})).error,'STALE_ELEMENT_REFERENCE');
  const changed=await call('observe',{tabId,screenshot:true});assert.ok(changed.tree_markdown);
  await call('reload',{tabId});
  assert.equal((await call('playwright.count',{tabId,selector:'#field',expectedNavigation:navigation})).error,'STALE_SURFACE_HANDLE');
  assert.equal((await call('playwright.count',{tabId,selector:'#field >> internal:control=enter-frame'})).error,'INVALID_ARGUMENT');
  await symlink('/etc/passwd',join(root,'EriiTransfer/downloads/escape'));
  assert.equal((await call('playwright.upload',{tabId,selector:'#upload',path:'downloads/escape'})).error,'PERMISSION_DENIED');
 }finally{
  if(browserContext)await browserContext.close();
  await new Promise(resolve=>server.close(resolve));
  await rm(transfer,{force:true});await rm(root,{recursive:true,force:true,maxRetries:3,retryDelay:100});
 }
});
