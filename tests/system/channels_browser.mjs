// Maintained Web UI driver: one real browser, no app-specific mock DOM.
import {chromium} from '../../guest-browser/node_modules/playwright-core/index.mjs';
import {mkdir,writeFile} from 'node:fs/promises';
const [url,evidence,phase='conversation']=process.argv.slice(2);
await mkdir(evidence,{recursive:true});
const options={headless:true};
if(process.env.ERII_BROWSER_EXECUTABLE)options.executablePath=process.env.ERII_BROWSER_EXECUTABLE;
const browser=await chromium.launch(options);
let page;
try{
 const context=await browser.newContext({viewport:{width:1400,height:980},deviceScaleFactor:1});
 page=await context.newPage();
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const remote=[];page.on('request',r=>{if(new URL(r.url()).origin!==new URL(url).origin)remote.push(r.url())});
 await page.goto(url);
 await page.getByLabel('Workspace access token').fill(process.env.ERII_WEB_TEST_TOKEN);
 await page.getByRole('button',{name:'Open workspace'}).click();
 await page.waitForURL(url+'/');
 await page.locator('#composer-root textarea').waitFor();
 // Prove the maintained frontend bundle is running, not just its HTML fallback.
 await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
 for(const image of await page.getByAltText(/Erii (character logo|default agent avatar)/).all()){
  if(!await image.evaluate(img=>img.complete&&img.naturalWidth===192))throw new Error('Eri artwork failed to load');
 }
 if(phase==='setup'){
  await page.getByRole('link',{name:'Settings',exact:false}).click();
  await page.locator('input[name="endpoint"]').fill('http://127.0.0.1:1/v1');
  await page.locator('input[name="model"]').first().fill('fixture-model');
  await page.getByRole('button',{name:'Add connection',exact:true}).click();
  await page.getByRole('link',{name:'Settings',exact:false}).click();
  await page.getByRole('button',{name:'Test connection',exact:true}).click();
  await page.getByRole('link',{name:'Settings',exact:false}).click();
  await page.getByRole('checkbox',{name:'Use this provider for new work'}).check();
  // Deterministically type into usable server HTML before the local bundle is
  // released. Progressive enhancement must preserve this draft and its file.
  let release;
  const bundle=new Promise(resolve=>{release=resolve});
  let requested;
  const loading=new Promise(resolve=>{requested=resolve});
  await page.route('**/app.js',async route=>{requested();await bundle;await route.continue()});
  const selecting=page.getByRole('button',{name:'Select model',exact:true}).click();
  await loading;
  await page.getByLabel('Message Erii').fill('Plan my day: finish the release risk note before noon.');
  await page.getByLabel('Attach a file').setInputFiles({name:'web-note.txt',mimeType:'text/plain',buffer:Buffer.from('Web owner attachment fixture.\n')});
  await page.getByLabel('Message Erii').focus();
  await page.getByLabel('Message Erii').press('ArrowLeft');
  const earlySource=await page.locator('input[name="source_id"]').inputValue();
  const selection=await page.getByLabel('Message Erii').evaluate(input=>[input.selectionStart,input.selectionEnd]);
  release();await selecting;
  await page.unroute('**/app.js');
  await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
  if(await page.getByLabel('Message Erii').inputValue()!=='Plan my day: finish the release risk note before noon.')throw new Error('frontend enhancement lost early draft');
  const preserved=await page.getByLabel('Message Erii').evaluate(input=>document.activeElement===input&&JSON.stringify([input.selectionStart,input.selectionEnd]));
  if(preserved!==JSON.stringify(selection)||await page.locator('input[name="source_id"]').inputValue()!==earlySource)throw new Error('frontend enhancement changed draft focus, selection or source identity');
  const file=await page.getByLabel('Attach a file').evaluate(async input=>input.files[0]?{name:input.files[0].name,text:await input.files[0].text()}:null);
  if(file?.name!=='web-note.txt'||file?.text!=='Web owner attachment fixture.\n')throw new Error('frontend enhancement lost early attachment');
  const submitted=page.waitForResponse(r=>r.url()===url+'/message'&&r.request().method()==='POST');
  await page.getByRole('button',{name:'Send message',exact:true}).click();
  if((await submitted).status()!==303)throw new Error('composer submission was not accepted');
  await page.waitForFunction(()=>document.querySelectorAll('.message.assistant').length>0);
 }
 if(phase==='components'){
  await page.locator('[data-renderer="native"] .answer-table').waitFor();
  if(await page.locator('.answer-table tbody tr').count()!==2)throw new Error('native table lost rows');
  if(await page.locator('.answer-table img, .answer-table script').count())throw new Error('table interpreted executable markup');
  if(!await page.locator('.answer-table').innerText().then(t=>t.includes('<img src=x onerror=')))throw new Error('table changed semantic cell text');
  for(const text of ['Processed: Retain this semantic answer.','Processed: Retain this answer when table rendering fails.','Processed: Keep the original text for an empty projection.','Processed: Keep the original text for an unknown version.']){
   if(!await page.getByText(text,{exact:true}).isVisible())throw new Error('missing semantic fallback: '+text);
  }
  if((await page.locator('body').innerText()).includes('Untrusted projection fallback'))throw new Error('projection replaced authoritative semantic fallback');
 }
 if(phase==='approval'){
  await page.locator('[data-renderer="native"] .approval').waitFor();
  const form=page.locator('.approval form').first();
  if(!await form.locator('input[name="approval_id"]').inputValue()||!(/^[a-f0-9]{64}$/.test(await form.locator('input[name="hash"]').inputValue())))throw new Error('native approval lost frozen identity');
 }
 const body=await page.locator('body').innerText();
 if(/run_id|claim_token|provider_exchange_id|run_trace/.test(body))throw new Error('internal trace in product surface');
 if(errors.length)throw new Error(errors.join(';'));
 await page.screenshot({path:evidence+'/'+phase+'.png',fullPage:true});
 await writeFile(evidence+'/'+phase+'.json',JSON.stringify({url:page.url(),body,browser:'chromium',errors},null,2));
 const mobile=await page.context().newPage();
 try{
  mobile.on('pageerror',e=>errors.push(e.message));
  mobile.on('request',r=>{if(new URL(r.url()).origin!==new URL(url).origin)remote.push(r.url())});
  await mobile.setViewportSize({width:390,height:844});
  await mobile.goto(url);
  await mobile.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
  await mobile.getByLabel('Message Erii',{exact:true}).fill('');
  await mobile.evaluate(()=>{document.querySelector('textarea').blur();scrollTo(0,0)});
  await mobile.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  const geometry=await mobile.evaluate(()=>({width:innerWidth,height:innerHeight,content:document.documentElement.scrollWidth,footerBottom:document.querySelector('footer').getBoundingClientRect().bottom}));
  if(geometry.content>geometry.width||geometry.footerBottom>geometry.height)throw new Error('mobile layout exceeds viewport');
  await mobile.screenshot({path:evidence+'/'+phase+'-mobile.png'});
  await writeFile(evidence+'/'+phase+'-mobile.json',JSON.stringify(geometry,null,2));
 }finally{await mobile.close()}
 if(phase==='components'){
  const plain=await browser.newContext({javaScriptEnabled:false,viewport:{width:1400,height:980}});
  try{
   await plain.addCookies(await context.cookies());
   const fallback=await plain.newPage();await fallback.goto(url);
   if(await fallback.locator('.message').count()!==await page.locator('.message').count())throw new Error('no-JavaScript fallback lost messages');
   if(!await fallback.getByLabel('Message Erii',{exact:true}).isVisible())throw new Error('no-JavaScript fallback lost composer');
   if(!await fallback.getByText('Processed: Retain this semantic answer.',{exact:true}).isVisible())throw new Error('no-JavaScript fallback lost semantics');
   await fallback.screenshot({path:evidence+'/'+phase+'-no-script.png',fullPage:true});
  }finally{await plain.close()}
 }
 await page.getByRole('link',{name:'Ongoing work',exact:false}).click();
 if(await page.locator('main').getByRole('button').count())throw new Error('progress became a management surface');
 await page.screenshot({path:evidence+'/'+phase+'-progress.png',fullPage:true});
 if(remote.length)throw new Error('UI contacted an external service: '+remote.join(', '));
 if(errors.length)throw new Error(errors.join(';'));
}catch(error){
 if(page){await page.screenshot({path:evidence+'/'+phase+'-failure.png',fullPage:true});await writeFile(evidence+'/'+phase+'-failure.txt',await page.locator('body').innerText())}
 throw error;
}finally{await browser.close()}
