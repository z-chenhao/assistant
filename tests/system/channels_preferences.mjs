// A scenario for the maintained channels driver, against the real Web server.
import {writeFile} from 'node:fs/promises';

export async function preferences(page,context,browser,url,evidence){
 const help=()=>page.locator('[data-help-trigger]');
 const tooltip=()=>page.getByRole('tooltip');
 const save=()=>page.locator('.preference-save');
 const check=async(condition,message)=>{if(!await condition)throw Error(message)};
 const layout=async()=>{
  await page.evaluate(()=>document.fonts.ready);
  const result=await page.evaluate(()=>{
   const row=document.querySelector('.preference-row'),select=row.querySelector('select');
   const nodes=[row.querySelector('label'),row.querySelector('[data-help-trigger]'),select,row.querySelector('[type=submit]')];
   const bounds=nodes.map(node=>{const r=node.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,center:r.y+r.height/2}});
   const canvas=document.createElement('canvas'),ctx=canvas.getContext('2d'),style=getComputedStyle(select);
   ctx.font=`${style.fontSize} ${style.fontFamily}`;
   return {viewport:innerWidth,scrollWidth:document.documentElement.scrollWidth,bounds,
    optionFits:ctx.measureText(select.selectedOptions[0].text).width+parseFloat(style.paddingLeft)+parseFloat(style.paddingRight)+20<=select.clientWidth,
    visibleHeadings:[...document.querySelectorAll('.settings-page h1,.settings-page h2')].filter(node=>!node.classList.contains('sr-only')).length};
  });
  if(result.scrollWidth>result.viewport||result.visibleHeadings||!result.optionFits)throw Error('preference layout/option overflow: '+JSON.stringify(result));
  const centers=result.bounds.map(r=>r.center);
  if(Math.max(...centers)-Math.min(...centers)>1)throw Error('preference controls are not on one row');
  for(let i=1;i<result.bounds.length;i++)if(result.bounds[i].x<result.bounds[i-1].x+result.bounds[i-1].width-1)throw Error('preference controls overlap');
  if(result.bounds.at(-1).x+result.bounds.at(-1).width>result.viewport)throw Error('Save is outside viewport');
  return result;
 };
 await page.goto(url+'/settings/preferences');
 await help().waitFor();
 await check(tooltip().isHidden(),'help starts open');
 const matrix=[];
 for(const language of ['en','zh-CN']){
  await page.locator('#interface-language').selectOption(language);
  await save().click();await page.waitForURL(url+'/settings/preferences?notice=language-saved');
  await page.goto(url+'/settings/preferences');
  await page.reload();
  await check(page.locator('html').getAttribute('lang').then(value=>value===language),'language was not persisted');
  for(const theme of ['light','dark'])for(const width of [1400,390,320]){
   await page.setViewportSize({width,height:width===1400?980:844});
   await page.evaluate(dark=>document.documentElement.classList.toggle('dark',dark),theme==='dark');
   const geometry=await layout();
   // The other option must fit before saving too, especially Chinese in English UI.
   await page.locator('#interface-language').selectOption(language==='en'?'zh-CN':'en');await layout();
   await check(page.locator('html').getAttribute('lang').then(value=>value===language),'selection saved without an explicit submit');
   await page.locator('#interface-language').selectOption(language);
   if(width<=390&&geometry.bounds.slice(1).some(r=>r.height<44||r.width<44))throw Error('mobile preference target is too small');
   await page.screenshot({path:`${evidence}/preferences-${language}-${theme}-${width}.png`,fullPage:true});
   matrix.push({language,theme,width,...geometry});
  }
 }
 // Return to English through the actual form before testing its interactions.
 await page.locator('#interface-language').selectOption('en');await save().click();
 await page.waitForURL(url+'/settings/preferences?notice=language-saved');
 await page.setViewportSize({width:390,height:844});
 await help().hover();await tooltip().waitFor();
 await tooltip().hover();await check(tooltip().isVisible(),'moving into help dismissed it');
 const popup=await tooltip().boundingBox();
 if(popup.x<0||popup.x+popup.width>390)throw Error('help is outside viewport');
 await page.screenshot({path:evidence+'/preferences-help.png',fullPage:true});
 await page.mouse.move(380,700);await check(tooltip().isHidden(),'pointer-only help stayed open after leaving');
 await page.locator('.section-tabs a').last().focus();await page.keyboard.press('Tab');
 await check(help().evaluate(node=>node===document.activeElement),'help is not in keyboard order');
 await tooltip().waitFor();await page.keyboard.press('Escape');
 await check(tooltip().isHidden(),'Escape did not dismiss help');
 await check(help().evaluate(node=>node===document.activeElement),'Escape moved focus');
 await page.keyboard.press('Tab');await check(page.locator('#interface-language').evaluate(node=>node===document.activeElement),'select is not next in keyboard order');
 await page.keyboard.press('Shift+Tab');await tooltip().waitFor();
 await page.keyboard.press('Enter');await check(tooltip().isVisible(),'first activation closed focused help');
 await page.keyboard.press('Enter');await check(tooltip().isHidden(),'second activation did not close help');
 await page.keyboard.press('Space');await tooltip().waitFor();
 await page.mouse.click(380,700);await check(tooltip().isHidden(),'outside click did not close help');

 const touch=await browser.newContext({hasTouch:true,viewport:{width:320,height:740}});
 try{
  await touch.addCookies(await context.cookies());const mobile=await touch.newPage();
  await mobile.goto(url+'/settings/preferences');
  const trigger=mobile.locator('[data-help-trigger]'),tip=mobile.getByRole('tooltip');
  let posts=0;mobile.on('request',request=>{if(request.method()==='POST')posts++});
  await trigger.tap();await tip.waitFor();await trigger.tap();await check(tip.isHidden(),'second touch did not close help');
  await trigger.tap();await tip.waitFor();await mobile.touchscreen.tap(310,650);await check(tip.isHidden(),'outside touch did not close help');
  await check(Promise.resolve(posts===0),'help submitted a form');
 }finally{await touch.close()}

 // Capture the synchronous busy state before the real native POST navigates.
 // A second requestSubmit in the same turn must be rejected by the form guard.
 let posts=0;
 const countPost=request=>{if(request.url()===url+'/settings/language'&&request.method()==='POST')posts++};
 page.on('request',countPost);
 await page.locator('#interface-language').selectOption('zh-CN');
 const response=page.waitForResponse(response=>response.url()===url+'/settings/language'&&response.request().method()==='POST');
 const busy=await page.locator('.preference-row').evaluate(form=>{
  const button=form.querySelector('[type=submit]'),before=button.getBoundingClientRect().width;
  form.requestSubmit(button);form.requestSubmit(button);
  return {disabled:button.disabled,label:button.textContent,busy:form.getAttribute('aria-busy'),sameWidth:before===button.getBoundingClientRect().width};
 });
 if(!busy.disabled||busy.label!=='Save'||busy.busy!=='true'||!busy.sameWidth)throw Error('invalid compact busy state: '+JSON.stringify(busy));
 if((await response).status()!==303)throw Error('language save did not redirect');
 await page.locator('html[lang="zh-CN"]').waitFor();
 await page.waitForLoadState();
 if(posts!==1)throw Error('native preference submitted more than once');
 page.off('request',countPost);
 await check(page.locator('html').getAttribute('lang').then(value=>value==='zh-CN'),'save did not switch interface');
 await page.getByRole('status').filter({hasText:'语言已保存。'}).waitFor();
 await page.goto(url+'/settings/models');await page.goBack();
 await check(save().isEnabled(),'returning to preferences left Save disabled');
 await page.locator('#interface-language').selectOption('en');await save().click();
 await page.waitForURL(url+'/settings/preferences?notice=language-saved');
 // Exercise the existing BFcache restoration handler deterministically as well.
 await page.locator('.preference-row').evaluate(form=>{
  form.addEventListener('submit',event=>event.preventDefault(),{once:true});form.requestSubmit();
 });
 await check(save().isDisabled(),'native submit guard did not run');
 await page.evaluate(()=>window.dispatchEvent(new PageTransitionEvent('pageshow',{persisted:true})));
 await check(save().isEnabled(),'persisted pageshow did not restore Save');

 await page.setViewportSize({width:640,height:980});
 await page.evaluate(()=>document.documentElement.style.zoom='2');
 await layout();await page.screenshot({path:evidence+'/preferences-css-zoom-200.png',fullPage:true});
 await page.evaluate(()=>document.documentElement.style.zoom='');

 const native=await browser.newContext({javaScriptEnabled:false,viewport:{width:320,height:740}});
 try{
  await native.addCookies(await context.cookies());const fallback=await native.newPage();
  await fallback.goto(url+'/settings/preferences');
  const details=fallback.locator('[data-help-fallback]');
  await details.locator('summary').focus();await fallback.keyboard.press('Enter');
  await check(details.evaluate(node=>node.open),'native help did not open with Enter');
  await fallback.screenshot({path:evidence+'/preferences-no-script.png',fullPage:true});
  await details.locator('summary').press('Space');await check(details.evaluate(node=>!node.open),'native help did not close with Space');
  await fallback.locator('#interface-language').selectOption('zh-CN');await fallback.locator('.preference-save').click();
  await fallback.waitForURL(url+'/settings/preferences?notice=language-saved');
  await check(fallback.locator('html').getAttribute('lang').then(value=>value==='zh-CN'),'no-JS save did not switch language');
  await fallback.reload();await check(fallback.locator('#interface-language').inputValue().then(value=>value==='zh-CN'),'no-JS language was not retained');
 }finally{await native.close()}

 for(const section of ['models','notifications','capabilities']){
  await page.goto(url+'/settings/'+section);
  await check(page.locator('.section-tabs [aria-current=page]').getAttribute('href').then(value=>value==='/settings/'+section),'neighbor tab lost selection');
  await check(page.locator('[data-preference-help]').count().then(count=>count===0),'preference help leaked to another section');
  if(section==='notifications'){
   await page.locator('select[name=target]').selectOption('tui:');await page.getByRole('button',{name:'Save notification channel',exact:true}).click();
   await page.reload();await check(page.locator('select[name=target]').inputValue().then(value=>value==='tui:'),'neighbor form did not save');
   await page.locator('select[name=target]').selectOption('auto:');await page.getByRole('button',{name:'Save notification channel',exact:true}).click();
  }
 }
 await writeFile(evidence+'/preferences-browser.json',JSON.stringify({matrix,keyboard:true,hover:true,touch:true,nativePosts:posts,bfcacheHandler:'persisted pageshow event',noJavaScript:true,cssZoom:2,neighborSections:true},null,2));
}
