// Maintained Web UI driver: one real browser, no app-specific mock DOM.
import {chromium} from '../guest-browser/node_modules/playwright-core/index.mjs';
import {mkdir,writeFile} from 'node:fs/promises';
const [url,evidence,phase='conversation']=process.argv.slice(2);
await mkdir(evidence,{recursive:true});
const options={headless:true};
if(process.env.ERII_BROWSER_EXECUTABLE)options.executablePath=process.env.ERII_BROWSER_EXECUTABLE;
const browser=await chromium.launch(options);
let page;
try{
 page=await browser.newPage({viewport:{width:1400,height:980},deviceScaleFactor:1});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.goto(url);
 await page.getByLabel('Workspace access token').fill(process.env.ERII_WEB_TEST_TOKEN);
 await page.getByRole('button',{name:'Open workspace'}).click();
 await page.waitForURL(url+'/');
 if(phase==='setup'){
  await page.getByRole('link',{name:'Settings',exact:false}).click();
  await page.locator('input[name="endpoint"]').fill('http://127.0.0.1:1/v1');
  await page.locator('input[name="model"]').first().fill('fixture-model');
  await page.getByRole('button',{name:'Add connection',exact:true}).click();
  await page.getByRole('link',{name:'Settings',exact:false}).click();
  await page.getByRole('button',{name:'Test connection',exact:true}).click();
  await page.getByRole('link',{name:'Settings',exact:false}).click();
  await page.getByRole('checkbox',{name:'Use this provider for new work'}).check();
  await page.getByRole('button',{name:'Select model',exact:true}).click();
  await page.getByLabel('Message Erii').fill('Plan my day: finish the release risk note before noon.');
  await page.getByLabel('Attach a file').setInputFiles({name:'web-note.txt',mimeType:'text/plain',buffer:Buffer.from('Web owner attachment fixture.\n')});
  await page.getByRole('button',{name:'Send message',exact:true}).click();
  await page.waitForFunction(()=>document.querySelectorAll('.message.assistant').length>0);
 }
 const body=await page.locator('body').innerText();
 if(/run_id|claim_token|provider_exchange_id|run_trace/.test(body))throw new Error('internal trace in product surface');
 if(errors.length)throw new Error(errors.join(';'));
 await page.screenshot({path:evidence+'/'+phase+'.png',fullPage:true});
 await writeFile(evidence+'/'+phase+'.json',JSON.stringify({url:page.url(),body,browser:'chromium',errors},null,2));
 await page.getByRole('link',{name:'Ongoing work',exact:false}).click();
 if(await page.locator('main').getByRole('button').count())throw new Error('progress became a management surface');
 await page.screenshot({path:evidence+'/'+phase+'-progress.png',fullPage:true});
}catch(error){
 if(page){await page.screenshot({path:evidence+'/'+phase+'-failure.png',fullPage:true});await writeFile(evidence+'/'+phase+'-failure.txt',await page.locator('body').innerText())}
 throw error;
}finally{await browser.close()}
