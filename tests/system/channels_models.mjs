import {selectValue} from './channels_controls.mjs';
import {writeFile} from 'node:fs/promises';

export async function modelConnections(page,context,url,evidence){
 const endpoint=process.env.STEWARD_MODEL_CONNECTION_ENDPOINT??'http://127.0.0.1:1/v1';
 const privateKey='SYNTHETIC_WEB_CONNECTION_KEY_NEVER_ECHO';
 const keyed=!!process.env.STEWARD_MODEL_CONNECTION_ENDPOINT;
 const read=async path=>(await context.request.get(url+path)).json();
 await page.goto(url+'/settings/models');
 await page.getByRole('heading',{name:'Models',exact:true}).waitFor();
 if(await page.getByRole('link',{name:'Resources',exact:true}).count()||await page.getByRole('link',{name:'App settings',exact:true}).count())throw Error('Models mixes unrelated settings');
 let authPosts=0;page.on('request',request=>{if(new URL(request.url()).pathname==='/auth/login'&&request.method()==='POST')authPosts++});
 await page.getByRole('link',{name:'Sign in with an account',exact:true}).click();
 await page.getByRole('button',{name:'Sign in with ChatGPT',exact:true}).waitFor();
 if(authPosts||await page.locator('form[action="/auth/login"] input[name=provider]').inputValue()!=='chatgpt')throw Error('account chooser starts implicit authorization');
 await page.screenshot({path:evidence+'/account-provider-choice.png',fullPage:true});
 await page.getByRole('link',{name:'Models',exact:true}).last().click();
 if(await page.getByRole('heading',{name:'Connections',exact:true}).count())throw Error('account identities are mixed with API connections');
 await page.getByRole('heading',{name:'Accounts',exact:true}).waitFor();
 await page.getByRole('heading',{name:'API connections',exact:true}).waitFor();
 const connect=async name=>{
  await page.getByRole('link',{name:'Connect with an API key',exact:true}).click();
  await selectValue(page,page.locator('[name=service]'),'custom');
  await page.getByLabel('Endpoint URL',{exact:true}).fill(endpoint);
  await page.getByLabel('Model ID',{exact:true}).fill('fixture-model');
  await page.locator('[name=name]').fill(name);
  if(keyed)await page.getByLabel('API key',{exact:true}).fill(privateKey);
  await page.getByRole('button',{name:'Connect',exact:true}).click();
  await page.waitForURL(u=>u.pathname==='/settings/models'&&u.searchParams.has('model'));
  const id=new URL(page.url()).searchParams.get('model');
  if((await page.content()).includes(privateKey)||page.url().includes(privateKey))throw Error('credential leaked into rendered metadata');
  if(await page.getByRole('button',{name:/Test connection|Use model/}).count())throw Error('manual model setup remains');
  return id;
 };
 const first=await connect('Primary connection');
 const initial=await read('/api/models');if(!initial.current.startsWith('profile:'+first+':'))throw Error('first connection was not selected automatically');
 const backup=await connect('Backup connection');
 const ready=await read('/api/models');if(ready.choices.length!==2||ready.current!==initial.current)throw Error('connected models are unavailable or default was overwritten');
 // Selecting a model belongs to the composer and preserves a user draft.
 await page.goto(url+'/');await page.getByLabel('Message Ghost',{exact:true}).fill('Keep my draft');
 const picker=page.getByRole('combobox',{name:'Model',exact:true});await picker.click();await page.getByRole('option',{name:'Backup connection',exact:true}).click();await picker.filter({hasText:'Backup connection'}).waitFor();
 if(await page.getByLabel('Message Ghost',{exact:true}).inputValue()!=='Keep my draft')throw Error('model selection lost draft');
 await page.goto(url+'/settings/models/'+encodeURIComponent(first));await page.waitForURL(u=>u.pathname==='/settings/models'&&u.searchParams.get('model')===first);await page.locator('#model-'+first+'>details[open]').waitFor();
 await page.setViewportSize({width:320,height:980});if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('expanded connection details overflow');await page.screenshot({path:evidence+'/models-details-mobile.png',fullPage:true});await page.setViewportSize({width:1400,height:980});
 if(keyed){
  const row=page.locator('#model-'+backup),trigger=row.getByRole('button',{name:/^Manage connection:/});
  const open=async()=>{await trigger.click();await page.getByRole('menuitem',{name:'Disconnect',exact:true}).click()};
  await open();const dialog=page.getByRole('dialog',{name:'Disconnect',exact:true});await dialog.getByRole('button',{name:'Cancel',exact:true}).click();await trigger.and(page.locator(':focus')).waitFor();
  if((await read('/api/models')).choices.length!==2)throw Error('cancel disconnected key');
  await open();await dialog.getByRole('button',{name:'Disconnect',exact:true}).click();await page.waitForURL(u=>u.pathname==='/settings/models'&&!u.search);
  const remaining=await read('/api/models');if(remaining.choices.some(choice=>choice.key.startsWith('profile:'+backup+':')))throw Error('disconnected key remains usable');
  await page.goto(url+'/');await picker.click();await page.getByRole('option',{name:'Primary connection',exact:true}).click();await picker.filter({hasText:'Primary connection'}).waitFor();
 }
 for(const theme of ['light','dark']){
  await page.evaluate(value=>localStorage.setItem('steward-theme',value),theme);
  for(const width of [1400,390,320]){
   await page.setViewportSize({width,height:980});await page.goto(url+'/settings/models');await page.evaluate(()=>document.fonts.ready);
   if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('model connections overflow');
   await page.screenshot({path:`${evidence}/models-${theme}-${width}.png`,fullPage:true});
   await page.getByRole('link',{name:'Connect with an API key',exact:true}).click();await selectValue(page,page.locator('[name=service]'),'custom');
   if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('API form overflow');
   await page.screenshot({path:`${evidence}/api-connection-${theme}-${width}.png`,fullPage:true});
  }
 }
 // Native navigation and a password input remain usable without JavaScript.
 const native=await context.browser().newContext({javaScriptEnabled:false,viewport:{width:390,height:844}});
 try{await native.addCookies(await context.cookies());const plain=await native.newPage();await plain.goto(url+'/settings/models');await plain.getByRole('link',{name:'Connect with an API key',exact:true}).click();await plain.locator('.model-connection-advanced>summary').click();await plain.getByLabel('Endpoint URL',{exact:true}).waitFor();if(await plain.locator('[name=api_key]').getAttribute('type')!=='password')throw Error('native key input is visible text')}finally{await native.close()}
 await page.setViewportSize({width:1400,height:980});await page.goto(url+'/settings/models');
 await writeFile(evidence+'/models-connections.json',JSON.stringify({keyed,first,backup,defaultPreserved:true,composerSelection:true,credentialNotEchoed:true,nativeFallback:true},null,2));
}
