import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdtemp, mkdir, realpath, readFile, writeFile, copyFile, rm, symlink} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join, resolve} from 'node:path';
import {createServer} from 'node:http';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {setTimeout as delay} from 'node:timers/promises';
import {fileURLToPath} from 'node:url';
import {chromium} from 'playwright-core';
import {waitForDevToolsPort} from '../scripts/browser_readiness.mjs';
const exec=promisify(execFile);
const here=fileURLToPath(new URL('.',import.meta.url));
const source=await readFile(new URL('../internal/computer/macosvm/browser.mjs',import.meta.url),'utf8');
const packageRoot=resolve(here,'node_modules/playwright-core');

test('real Chromium: inventory, locator, input, navigation expiry, download and scoped chooser', {timeout:60000}, async()=>{
 const root=await realpath(await mkdtemp(join(tmpdir(),'erii-browser-')));
 const module=join(root,'playwright/index.mjs');
 const transfer=join(root,'playwright/erii-transfer.py');
 const server=createServer((req,res)=>{
  if(req.url==='/download'){res.writeHead(200,{'Content-Disposition':'attachment; filename=fixture.txt','Content-Type':'text/plain'});res.end('fixture download'+'x'.repeat(300000));return;}
  res.end('<title>Fixture</title><input id="field" aria-label="Fixture text"><button id="button" onclick="document.querySelector(\'#status\').textContent=\'clicked:\'+event.detail" oncontextmenu="document.querySelector(\'#status\').textContent=\'context\';return false">Click</button><div id="status">ready</div><a id="download" href="/download">Download</a><button id="upload" onclick="document.querySelector(\'#file\').click()">Upload</button><input type="file" id="file" onchange="document.querySelector(\'#status\').textContent=this.files[0].name">');
 });
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 const endpoint=`http://127.0.0.1:${server.address().port}`;
 let browserContext;
 try{
  // Each invocation owns its transfer helper; concurrent test commands must
  // not install/remove a shared file beneath node_modules.
  await mkdir(join(root,'playwright'));
  for(const file of ['index.mjs','package.json'])await symlink(join(packageRoot,file),join(root,'playwright',file));
  await copyFile(new URL('../internal/computer/macosvm/transfer.py',import.meta.url),transfer);
  browserContext=await chromium.launchPersistentContext(join(root,'profile'),{headless:true,args:['--remote-debugging-address=127.0.0.1','--remote-debugging-port=0']});
  const port=await waitForDevToolsPort(join(root,'profile'));
  const call=async(method,args={},onLog)=>{
    const child=exec(process.execPath,['--input-type=module','-e',source,JSON.stringify({module,port,root:join(root,'EriiTransfer'),method,args})],{timeout:15000,maxBuffer:12<<20,env:{...process.env,...(onLog?{DEBUG:'pw:api'}:{})}});
    if(onLog)child.child.stderr.on('data',onLog);
    return JSON.parse((await child).stdout);
  };
  const inventory=await call('inventory');assert.equal(inventory.browsers[0].id,'chromium');
  const tabId=inventory.tabs[0].id;
  assert.equal((await call('goto',{tabId,url:endpoint})).effect,'dispatched');
  const capture=await call('observe',{tabId,screenshot:true});assert.ok(capture.screenshot_png_b64);assert.ok(capture.tree_markdown.includes('Fixture text'));assert.ok(capture.elements.every(e=>e.element_token));
  const navigation=capture.navigation;
  assert.equal((await call('playwright.count',{tabId,selector:'#field',expectedNavigation:navigation})).value,1);
  assert.equal((await call('playwright.count',{tabId,selector:'html',expectedNavigation:navigation})).value,1);
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
  const waitingToken=changed.elements.find(e=>e.label==='Click').element_token;
  const dynamicPeer=await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
  await dynamicPeer.contexts()[0].pages()[0].evaluate(()=>{
    const button=document.querySelector('#button');button.disabled=true;
    setTimeout(()=>{const replacement=button.cloneNode(true);replacement.disabled=false;button.replaceWith(replacement);},1000);
  });
  await dynamicPeer.close();
  assert.equal((await call('click',{tabId,element_token:waitingToken})).error,'STALE_ELEMENT_REFERENCE','core ElementRef must not retarget a replacement while waiting for actionability');

  // Synchronize at actual selector resolution, then navigate while the input
  // waits for actionability. Matching enabled controls in B must stay inert.
  const page=browserContext.pages()[0];
  for (const [method,target,missing] of [['playwright.click','#button',false],['playwright.fill','#field',false],['playwright.click','#button',true]]) {
    await page.goto(endpoint);
    const bound=(await call('observe',{tabId})).navigation;
    await page.evaluate(({target,missing})=>{
      const element=document.querySelector(target);
      if(missing) element.remove(); else if(target==='#button') element.disabled=true; else element.readOnly=true;
    },{target,missing});
    let waiting;
    const boundary=new Promise(resolve=>{waiting=resolve;});
    const pending=call(method,{tabId,selector:target,value:'must not reach B',expectedNavigation:bound,timeoutMs:5000},log=>{
      if(missing ? log.includes('waiting for locator') && log.includes(target) : /element is not enabled|element is not editable/.test(log))waiting();
    });
    await Promise.race([boundary,pending.then(value=>assert.fail('action did not reach wait: '+JSON.stringify(value)))]);
    await page.goto(endpoint+'/replacement');
    const outcome=await pending;
    assert.equal(outcome.error,'STALE_SURFACE_HANDLE',JSON.stringify(outcome));
    assert.equal(await page.locator('#status').textContent(),'ready','input crossed the bound document during waiting');
    assert.equal(await page.locator('#field').inputValue(),'','fill crossed the bound document during waiting');
  }

  // Within one document, CSS retains dynamic replacement semantics.
  await page.goto(endpoint);
  await page.evaluate(()=>{
    const old=document.querySelector('#button');old.disabled=true;
    setTimeout(()=>{const replacement=old.cloneNode(true);replacement.disabled=false;old.replaceWith(replacement);},500);
  });
  assert.equal((await call('playwright.click',{tabId,selector:'#button'})).effect,'dispatched');
  assert.equal(await page.locator('#status').textContent(),'clicked:1');

  await call('reload',{tabId});
  assert.equal((await call('playwright.count',{tabId,selector:'#field',expectedNavigation:navigation})).error,'STALE_SURFACE_HANDLE');
  assert.equal((await call('playwright.count',{tabId,selector:'#field >> internal:control=enter-frame'})).error,'INVALID_ARGUMENT');
  await symlink('/etc/passwd',join(root,'EriiTransfer/downloads/escape'));
  assert.equal((await call('playwright.upload',{tabId,selector:'#upload',path:'downloads/escape'})).error,'PERMISSION_DENIED');
 }finally{
  if(browserContext)await browserContext.close();
  await new Promise(resolve=>server.close(resolve));
  await rm(root,{recursive:true,force:true,maxRetries:3,retryDelay:100});
 }
});


test('debugger port readiness: delayed and partial publication', {timeout:2000}, async()=>{
 const root=await mkdtemp(join(tmpdir(),'erii-browser-ready-'));
 const published=(async()=>{
  await delay(25);await writeFile(join(root,'DevToolsActivePort'),'0\n');
  await delay(25);await writeFile(join(root,'DevToolsActivePort'),'');
  await delay(25);await writeFile(join(root,'DevToolsActivePort'),'43123\n/devtools/browser/fixture\n');
 })();
 try{assert.equal(await waitForDevToolsPort(root,1000),43123)}
 finally{await published;await rm(root,{recursive:true,force:true})}
});

test('debugger port readiness: deadline and non-transient read errors fail closed', {timeout:2000}, async()=>{
 const root=await mkdtemp(join(tmpdir(),'erii-browser-not-ready-'));
 try{
  await writeFile(join(root,'DevToolsActivePort'),'0\n');
  await assert.rejects(waitForDevToolsPort(root,30),/Chromium debugger endpoint not ready/);
  await rm(join(root,'DevToolsActivePort'));
  await mkdir(join(root,'DevToolsActivePort'));
  await assert.rejects(waitForDevToolsPort(root,1000),{code:'EISDIR'});
 }finally{await rm(root,{recursive:true,force:true})}
});
