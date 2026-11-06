import {selectValue,selectParts} from './channels_controls.mjs';
// Maintained channels scenario against the real Web server. Routes below inject
// transport failures only; successful saves exercise the actual cookie handler.
import {writeFile} from 'node:fs/promises';

export async function preferences(page,context,browser,url,evidence){
 const trigger=p=>p.locator('[data-language-trigger]');
 const popup=p=>p.locator('[data-language-popup]');
 const option=(p,value)=>p.locator(`[data-language-option="${value}"]`);
 const help=p=>p.locator('[data-help-trigger]');
 const check=async(condition,message)=>{if(!await condition)throw Error(message)};
 const hoverPaths=[];
 const hoverHelp=async(p,state)=>{
  // Keep focus elsewhere: focus/pinning must not conceal a broken hover path.
  await p.locator('#interface-theme-picker').focus();
  await p.mouse.move(p.viewportSize().width-2,p.viewportSize().height-2);
  await help(p).hover();const tip=p.getByRole('tooltip');await tip.waitFor();
  const control=await help(p).boundingBox(),tooltip=await tip.boundingBox();
  if(tooltip.x<0||tooltip.x+tooltip.width>p.viewportSize().width+1)throw Error('help outside viewport');
  const trace=[];
  for(let y=control.y+control.height/2;y<=tooltip.y+12;y+=1){
   await p.mouse.move(control.x+control.width/2,y);
   const visible=await tip.isVisible();trace.push({y,visible});
   if(!visible){
    await writeFile(evidence+'/preferences-hover-failure.json',JSON.stringify({state,control,tooltip,trace},null,2));
    throw Error('help disappeared along continuous pointer path: '+JSON.stringify({state,y,control,tooltip}));
   }
  }
  await check(p.locator('[data-preference-help]').evaluate(n=>n.matches(':hover')),'tooltip did not retain hover');
  await p.screenshot({path:evidence+'/preferences-hover-'+state+'.png',fullPage:true});
  hoverPaths.push({state,viewport:p.viewportSize(),control,tooltip,gap:tooltip.y-control.y-control.height,trace});
  await p.mouse.move(p.viewportSize().width-2,p.viewportSize().height-2);
  await tip.waitFor({state:'hidden'});
 };
 const ready=async p=>{await trigger(p).waitFor();await check(trigger(p).getAttribute('aria-disabled').then(v=>v!=='true'),'selector remained busy')};
 const choose=async(p,value)=>{
  const prior=await p.locator('#interface-language').inputValue();
  await trigger(p).click();await option(p,value).click();
  if(prior!==value){
   await p.waitForURL(u=>u.searchParams.get('language')===value&&u.hash==='#interface-language-picker');
   await p.waitForLoadState();await ready(p);
   await check(p.locator('html').getAttribute('lang').then(v=>v===value),'save did not change page language');
  }
 };
 const layout=async p=>{
  await p.evaluate(()=>document.fonts.ready);
  const result=await p.evaluate(()=>{
   const rect=node=>{const r=node.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,center:r.y+r.height/2}};
   const row=document.querySelector('.preference-row'),button=row.querySelector('[data-language-trigger]');
   const bounds=[row.querySelector('label'),row.querySelector('[data-help-trigger]'),button].map(rect);
   const canvas=document.createElement('canvas'),ctx=canvas.getContext('2d'),style=getComputedStyle(button);
   ctx.font=`${style.fontSize} ${style.fontFamily}`;
   return {viewport:innerWidth,scrollWidth:document.documentElement.scrollWidth,bounds,
    valueFits:ctx.measureText(button.querySelector('span').textContent).width+parseFloat(style.paddingLeft)+parseFloat(style.paddingRight)+24<=button.clientWidth,
    pageTitles:[...document.querySelectorAll('.settings-page h1')].filter(n=>!n.classList.contains('sr-only')).length,
    visibleHeadings:[...document.querySelectorAll('.settings-page h1,.settings-page h2')].filter(n=>!n.classList.contains('sr-only')).length,
    unwantedCopy:/选择后自动保存并刷新页面|Saves automatically and reloads this page/.test(document.querySelector('.settings-page').textContent)};
  });
  if(result.scrollWidth>result.viewport||result.pageTitles!==1||!result.valueFits||result.unwantedCopy)throw Error('layout/copy: '+JSON.stringify(result));
  if(Math.max(...result.bounds.map(r=>r.center))-Math.min(...result.bounds.map(r=>r.center))>1)throw Error('default preference row wrapped');
  for(let i=1;i<result.bounds.length;i++)if(result.bounds[i].x<result.bounds[i-1].right-1)throw Error('controls overlap');
  await check(p.locator('.preference-save').isHidden(),'Save visible in enhanced UI');
  return result;
 };
 const popupBounds=async p=>{
  await popup(p).waitFor({state:'visible'});
  const box=await popup(p).boundingBox(),control=await trigger(p).boundingBox();
  const viewport=p.viewportSize();
  if(!box||box.x<0||box.x+box.width>viewport.width+1||box.y<0||box.y+box.height>viewport.height+1||Math.abs(box.x+box.width-control.x-control.width)>1)throw Error('popup outside viewport or not right aligned: '+JSON.stringify({box,control}));
  return box;
 };
 await page.goto(url+'/settings/preferences');await ready(page);
 const matrix=[];
 for(const language of ['en','zh-CN']){
  if(await page.locator('#interface-language').inputValue()!==language)await choose(page,language);
  await page.goto(url+'/settings/preferences');await ready(page);
  for(const theme of ['light','dark'])for(const width of [1400,390,320]){
   await page.setViewportSize({width,height:width===1400?980:844});
   await page.evaluate(dark=>document.documentElement.classList.toggle('dark',dark),theme==='dark');
   const geometry=await layout(page);
   if(width<=390&&geometry.bounds.slice(1).some(r=>r.height<44||r.width<44))throw Error('small touch target');
   await page.screenshot({path:`${evidence}/preferences-${language}-${theme}-${width}.png`,fullPage:true});
   const height=await page.locator('.preference-group').evaluate(n=>n.getBoundingClientRect().height);
   await trigger(page).click();const box=await popupBounds(page);
   const rows=await popup(page).getByRole('option').evaluateAll(options=>options.map(option=>option.getBoundingClientRect().height));
   if(await popup(page).locator('input').count()||rows.length!==2||box.height>rows.reduce((height,row)=>height+row,0)+12)throw Error('two-language Select is not compact or gained a search field');
   if(await page.locator('.preference-group').evaluate(n=>n.getBoundingClientRect().height)!==height)throw Error('popup shifted row');
   if(await popup(page).locator('[aria-selected=true]').getAttribute('data-language-option')!==language)throw Error('selected option lost');
   await page.screenshot({path:`${evidence}/preferences-${language}-${theme}-${width}-open.png`,fullPage:true});
   await page.keyboard.press('Escape');await popup(page).waitFor({state:'hidden'});
   matrix.push({language,theme,width,...geometry,popup:box});
   await hoverHelp(page,`${language}-${theme}-${width}`);
  }
 }
 await choose(page,'en');await page.goto(url+'/settings/preferences');await ready(page);
 await page.setViewportSize({width:390,height:844});
 let posts=0;page.on('request',r=>{if(r.method()==='POST'&&new URL(r.url()).pathname==='/settings/language')posts++});
 await help(page).hover();await page.getByRole('tooltip').waitFor();
 await page.getByRole('tooltip').hover();await check(page.getByRole('tooltip').isVisible(),'help not hoverable');
 await page.keyboard.press('Escape');await check(page.getByRole('tooltip').isHidden(),'help Escape failed');
 await trigger(page).focus();await page.keyboard.press('Enter');
 await check(page.getByRole('tooltip').isHidden(),'list and help both open');
 await page.waitForFunction(()=>document.activeElement?.getAttribute('role')==='option');
 await page.keyboard.press('End');await option(page,'zh-CN').and(page.locator('[data-highlighted]')).waitFor();
 await page.keyboard.press('Home');await option(page,'en').and(page.locator('[data-highlighted]')).waitFor();
 await page.keyboard.press('ArrowDown');await page.keyboard.press('Escape');
 await trigger(page).click();await page.waitForFunction(()=>document.activeElement?.getAttribute('role')==='option');await page.keyboard.press('End');await page.keyboard.press('Tab');
 await popup(page).waitFor({state:'hidden'});
 await trigger(page).click();await option(page,'en').click();
 await trigger(page).click();await page.mouse.click(380,700);await popup(page).waitFor({state:'hidden'});
 if(posts!==0)throw Error('browsing/cancelling/current selection posted');
 await trigger(page).focus();await page.keyboard.press('Space');await page.waitForFunction(()=>document.activeElement?.getAttribute('role')==='option');await page.keyboard.press('End');await page.keyboard.press('Enter');
 await page.waitForURL(u=>u.searchParams.get('language')==='zh-CN');await page.waitForLoadState();await ready(page);
 await check(trigger(page).evaluate(n=>n===document.activeElement),'save did not restore focus');
 if(posts!==1)throw Error('keyboard confirmation did not save exactly once');
 await choose(page,'en');await page.goto(url+'/settings/preferences');await ready(page);

 // Hold the real POST to observe the lock and attempt duplicate submissions.
 let release;const held=new Promise(resolve=>{release=resolve});
 await page.route('**/settings/language',async route=>{await held;await route.continue()});
 const before=posts;const pendingPost=page.waitForRequest(r=>r.url()===url+'/settings/language'&&r.method()==='POST');
 await trigger(page).click();await option(page,'zh-CN').click();await pendingPost;
 await check(trigger(page).getAttribute('aria-disabled').then(v=>v==='true'),'missing synchronous lock');
 await page.locator('[data-language-preference]').evaluate(f=>{f.requestSubmit();f.requestSubmit()});
 await trigger(page).click({force:true});
 if(posts!==before+1)throw Error('busy selection created duplicate requests');
 await page.screenshot({path:evidence+'/preferences-saving.png',fullPage:true});
 release();await page.waitForURL(u=>u.searchParams.get('language')==='zh-CN');await page.waitForLoadState();await ready(page);await page.unroute('**/settings/language');
 await choose(page,'en');

 // A definite rejection retains the old cookie and permits an explicit retry.
 await page.route('**/settings/language',route=>route.fulfill({status:400,contentType:'application/json',body:'{"error":"invalid_language"}'}));
 await trigger(page).click();await option(page,'zh-CN').click();
 await page.locator('[data-language-feedback][role=alert]').waitFor();
 await check(page.locator('#interface-language').inputValue().then(v=>v==='en'),'rejection did not restore confirmed value');
 await hoverHelp(page,'rejected-feedback');
 await page.screenshot({path:evidence+'/preferences-rejected.png',fullPage:true});
 await page.unroute('**/settings/language');await page.locator('[data-language-retry]').click();
 await page.waitForURL(u=>u.searchParams.get('language')==='zh-CN');await page.waitForLoadState();await ready(page);
 await choose(page,'en');

 // The server applied the preference, but a damaged response cannot confirm it.
 await page.route('**/settings/language',async route=>{const response=await route.fetch();await route.fulfill({response,body:'not-json'})});
 const unknownBefore=posts;await trigger(page).click();await option(page,'zh-CN').click();
 await page.locator('[data-language-refresh]').waitFor();
 await check(trigger(page).getAttribute('aria-disabled').then(v=>v==='true'),'unknown result still accepts writes');
 await page.screenshot({path:evidence+'/preferences-unconfirmed.png',fullPage:true});
 await page.unroute('**/settings/language');await page.locator('[data-language-refresh]').click();await ready(page);
 await check(page.locator('html').getAttribute('lang').then(v=>v==='zh-CN'),'refresh did not read applied cookie');
 if(posts!==unknownBefore+1)throw Error('unknown result replayed POST');
 await choose(page,'en');

 for(const failure of ['network','forbidden','server-error','mismatch','timeout']){
  let finish;
  if(failure==='network')await page.route('**/settings/language',route=>route.abort('failed'));
  else if(failure==='forbidden')await page.route('**/settings/language',route=>route.fulfill({status:403,body:'Forbidden'}));
  else if(failure==='server-error')await page.route('**/settings/language',route=>route.fulfill({status:503,body:'Unavailable'}));
  else if(failure==='mismatch')await page.route('**/settings/language',route=>route.fulfill({status:200,contentType:'application/json',body:'{"language":"en"}'}));
  else {const pending=new Promise(r=>{finish=r});await page.route('**/settings/language',async route=>{await pending;await route.abort().catch(()=>{})})}
  const count=posts;await trigger(page).click();await option(page,'zh-CN').click();
  await page.locator('[data-language-refresh]').waitFor({timeout:20000});
  if(failure==='forbidden')await check(trigger(page).getAttribute('aria-disabled').then(v=>v==='false'),'rejection did not unlock');
  else await check(trigger(page).getAttribute('aria-disabled').then(v=>v==='true'),'unconfirmed result did not lock');
  if(finish)finish();await page.unroute('**/settings/language');
  await page.locator('[data-language-refresh]').click();await ready(page);
  if(posts!==count+1)throw Error(failure+' replayed request');
 }
 // Success acknowledgement with no Set-Cookie must fail the GET readback.
 await page.route('**/settings/language',route=>route.fulfill({status:200,contentType:'application/json',body:'{"language":"zh-CN"}'}));
 await trigger(page).click();await option(page,'zh-CN').click();
 await page.waitForURL(u=>u.searchParams.get('language')==='zh-CN');await page.waitForLoadState();await ready(page);
 await check(page.locator('[data-language-feedback]').innerText().then(v=>v.includes('cookie settings')),'readback falsely confirmed blocked cookie');
 await page.unroute('**/settings/language');
 await page.setViewportSize({width:320,height:844});
 await page.locator('.preference-group').evaluate(n=>n.style.width='210px');
 await check(page.locator('.preference-controls').evaluate(n=>n.getBoundingClientRect().top>document.querySelector('.preference-name').getBoundingClientRect().bottom),'test did not produce a wrapped row');
 await hoverHelp(page,'wrapped-feedback');
 await page.locator('.preference-group').evaluate(n=>n.style.width='');
 await page.goto(url+'/settings/models');await page.goBack();await ready(page);
 const countBeforeReturn=posts;
 await page.evaluate(()=>window.dispatchEvent(new PageTransitionEvent('pageshow',{persisted:true})));
 await page.waitForURL(url+'/settings/preferences');await ready(page);
 if(posts!==countBeforeReturn)throw Error('BFcache replayed POST');

 // Touch uses the same option confirmation and mutually exclusive help.
 const touch=await browser.newContext({hasTouch:true,viewport:{width:320,height:740}});
 try{
  await touch.addCookies(await context.cookies());const mobile=await touch.newPage();await mobile.goto(url+'/settings/preferences');await ready(mobile);
  await help(mobile).tap();await mobile.getByRole('tooltip').waitFor();await trigger(mobile).tap();
  await check(mobile.getByRole('tooltip').isHidden(),'touch popup left help open');
  await option(mobile,'en').tap();await popup(mobile).waitFor({state:'hidden'});
  await choose(mobile,'zh-CN');
 }finally{await touch.close()}

 // No JS and failed initialization retain the native form.
 for(const mode of ['no-script','initialization-failed']){
  const fallbackContext=await browser.newContext({javaScriptEnabled:mode!=='no-script',viewport:{width:320,height:740}});
  try{
   await fallbackContext.addCookies(await context.cookies());
   if(mode==='initialization-failed')await fallbackContext.addInitScript(()=>{window.fetch=undefined});
   const fallback=await fallbackContext.newPage();await fallback.goto(url+'/settings/preferences');
   await fallback.locator('#interface-language').waitFor();
   await fallback.locator('#interface-language').selectOption('zh-CN');
   await fallback.locator('.preference-save').click();
   await fallback.waitForURL(u=>u.searchParams.get('notice')==='language-saved');await fallback.waitForLoadState();
   await check(fallback.locator('html').getAttribute('lang').then(v=>v==='zh-CN'),mode+' save did not persist');
   await fallback.screenshot({path:evidence+'/preferences-'+mode+'.png',fullPage:true});
  }finally{await fallbackContext.close()}
 }
 await page.setViewportSize({width:640,height:980});await page.evaluate(()=>document.documentElement.style.zoom='2');
 await trigger(page).click();await popupBounds(page);await page.screenshot({path:evidence+'/preferences-css-zoom-200.png',fullPage:true});await page.keyboard.press('Escape');
 await page.evaluate(()=>document.documentElement.style.zoom='');
 await page.setViewportSize({width:390,height:600});
 await page.evaluate(()=>{const group=document.querySelector('.preference-group');group.style.marginTop='600px';document.body.style.minHeight='2200px';scrollTo(0,group.getBoundingClientRect().top+scrollY-innerHeight+90)});
 await trigger(page).click();const flipped=await popupBounds(page),control=await trigger(page).boundingBox();
 if(flipped.y+flipped.height>control.y+1)throw Error('popup did not flip above near viewport bottom');
 await page.setViewportSize({width:320,height:600});await page.evaluate(()=>new Promise(r=>requestAnimationFrame(()=>requestAnimationFrame(r))));await popupBounds(page);await page.keyboard.press('Escape');
 for(const section of ['setup','notifications','capabilities']){
  await page.goto(url+'/settings/'+section);
  await check(page.locator('.settings-navigation [aria-current=page]').getAttribute('href').then(v=>v==='/settings/'+section),'neighbor selection lost');
  await check(page.locator('[data-language-preference]').count().then(v=>v===0),'preference leaked to neighbor');
  if(section==='notifications'){
   const save=async()=>{
    const button=page.getByRole('button',{name:'Save notification channel',exact:true});
    const action=await button.evaluate(node=>node.form.action);
    await Promise.all([page.waitForResponse(response=>response.url()===action&&response.request().method()==='POST'),button.click()]);
    await page.waitForURL(u=>u.pathname==='/settings/notifications'&&u.searchParams.get('notice')==='saved');
    await page.waitForLoadState();
   };
   await selectValue(page,page.locator('select[name=target]'),'tui:');await save();await page.reload();
   await check(page.locator('select[name=target]').inputValue().then(v=>v==='tui:'),'neighbor submit guard regressed');
   await selectValue(page,page.locator('select[name=target]'),'auto:');await save();
  }
 }
 await writeFile(evidence+'/preferences-browser.json',JSON.stringify({matrix,hoverPaths,keyboard:true,touch:true,autosave:true,singleFlight:true,rejectedRetry:true,unknownAppliedReadback:true,network:true,timeout:true,authRejection:true,mismatchedAck:true,blockedCookieReadback:true,bfcache:true,fallbacks:true,flipped:true,cssZoom:2,neighborSections:true},null,2));
}
