import {selectValue,selectParts} from './channels_controls.mjs';
import {writeFile} from 'node:fs/promises';

// One scenario module for the maintained channels browser driver; no runner,
// browser lifecycle, fake DOM or alternate product endpoint lives here.
async function navigationViewports(page,evidence){
 const original=page.viewportSize();
 const measurements=[];
 for(const width of [1400,1024,761,760,390,320]){
  await page.setViewportSize({width,height:844});
  await page.evaluate(()=>document.fonts.ready);
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error(`settings overflowed at ${width}px`);
  const mobile=width<=760;
  if(mobile)await page.locator('.sidebar-open').click();
  const geometry=await page.evaluate(()=>{
   const rect=selector=>{const r=document.querySelector(selector).getBoundingClientRect();return {left:r.left,right:r.right,bottom:r.bottom}};
   return {primary:rect('.app-sidebar'),secondary:rect('.section-sidebar'),main:rect('.workspace-main'),settings:rect('.primary-settings'),viewport:innerWidth,height:innerHeight};
  });
  if(geometry.primary.left<0||Math.abs(geometry.primary.right-geometry.secondary.left)>1||geometry.secondary.right>width+1||geometry.height-geometry.settings.bottom>32||geometry.settings.bottom>geometry.height)throw Error(`navigation overlap or misplaced Settings at ${width}px: ${JSON.stringify(geometry)}`);
  if(!mobile&&Math.abs(geometry.secondary.right-geometry.main.left)>1)throw Error(`content overlaps navigation at ${width}px`);
  if(!await page.locator('.app-sidebar [aria-current="page"]').isVisible()||!await page.locator('.section-sidebar [aria-current="page"]').isVisible())throw Error(`navigation lost the selected module/category at ${width}px`);
  // The rail owns module links and its collapse control; personal preferences
  // have a single home in Settings, even if their old selectors are renamed.
  if(await page.locator('.app-sidebar').getByRole('button').count()!==1)throw Error(`primary rail acquired extra preference controls at ${width}px`);
  if(mobile){
   await page.keyboard.press('Escape');
   if(!await page.locator('.sidebar-open').evaluate(node=>node===document.activeElement)||await page.locator('#workspace-main').evaluate(node=>node.inert))throw Error(`closing navigation lost focus or kept content inert at ${width}px`);
  }
  measurements.push({width,...geometry});
 }
 await page.setViewportSize(original);
 await writeFile(evidence+'/navigation-viewports.json',JSON.stringify(measurements,null,2));
}

// Real navigation, native form writes and delayed bundle delivery. Check
// relationships before/after enhancement rather than platform-specific pixels.
export async function pageTransitions(page,context,url,evidence){
 const measurements={resources:[],collapse:[],loading:[],fonts:[],focus:[]};
 await page.addInitScript(()=>{
  window.navigationFontFrames=[];
  if(!['/settings/preferences','/settings/notifications'].includes(location.pathname))return;
  const sample=()=>{
   const brand=document.querySelector('.brand');
   if(brand)window.navigationFontFrames.push({brand:brand.getBoundingClientRect().width,ready:document.fonts.check('14px Inter')&&document.fonts.check('14px StewardWordmark')});
   if(performance.now()<300)requestAnimationFrame(sample);
  };
  requestAnimationFrame(sample);
 });
 const settle=async()=>{await page.evaluate(()=>document.fonts.ready);await page.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))))};
 for(const language of ['en','zh-CN']){
  await context.request.post(url+'/settings/language',{form:{language}});
  for(const theme of ['light','dark']){
   await page.evaluate(value=>localStorage.setItem('steward-theme',value),theme);
   for(const width of [1400,1024,820,800,780,761,760,390,320]){
    await page.setViewportSize({width,height:844});await page.goto(url+'/settings/setup');await settle();
    const geometry=await page.evaluate(()=>({width:innerWidth,scrollWidth:document.documentElement.scrollWidth,
     title:document.querySelector('.settings-page h1').getBoundingClientRect().toJSON(),
     bounds:[...document.querySelectorAll('.resource-model .resource-name,.model-connection,.model-use,.model-consent')].map(node=>node.getBoundingClientRect().toJSON()),
     nameLines:[...document.querySelectorAll('.resource-model .resource-name')].map(node=>{const range=document.createRange();range.selectNodeContents(node);return range.getClientRects().length})}));
    if(geometry.scrollWidth>width||geometry.title.width<2||geometry.bounds.some(r=>r.left<0||r.right>width+1)||geometry.nameLines.some(lines=>lines>1))throw Error('Resources title, model name or controls overflowed: '+JSON.stringify({language,theme,...geometry}));
    measurements.resources.push({language,theme,...geometry});
    if(width===761||width===390)await page.screenshot({path:`${evidence}/resources-${language}-${theme}-${width}.png`,fullPage:true});
   }
  }
 }
 await context.request.post(url+'/settings/language',{form:{language:'en'}});
 await page.setViewportSize({width:1400,height:980});await page.goto(url+'/settings/preferences');
 await settle();
 // Real native clicks must reuse the critical fonts from the previous page.
 for(const path of ['/settings/notifications','/settings/preferences','/settings/notifications']){
  await page.locator(`.settings-navigation a[href="${path}"]`).click();await page.waitForURL(url+path);await settle();
  const fonts=await page.evaluate(()=>({frames:window.navigationFontFrames,resources:performance.getEntriesByType('resource').filter(entry=>/\/(InterVariable\.woff2|Michroma-Regular\.ttf)\?/.test(entry.name)).map(entry=>({name:new URL(entry.name).pathname,bytes:entry.transferSize}))}));
  if(!fonts.frames.length||fonts.frames.some(frame=>!frame.ready)||fonts.frames.some(frame=>Math.abs(frame.brand-fonts.frames[0].brand)>1)||fonts.resources.length!==2||fonts.resources.some(resource=>resource.bytes!==0))throw Error('Settings navigation flashed or downloaded critical fonts again: '+JSON.stringify(fonts));
  measurements.fonts.push({path,...fonts});
 }
 const heading=await page.locator('.settings-page h1').boundingBox();
 for(const path of ['/settings/notifications','/settings/setup','/settings/apps','/settings/capabilities']){
  await page.goto(url+path);const next=await page.locator('.settings-page h1').boundingBox();
  // Apps needs a wider catalogue; it keeps the shared title height/top while
  // its horizontal position follows its own content padding.
  const expectedX=path==='/settings/apps'?await page.locator('.apps-page').evaluate(node=>node.getBoundingClientRect().x+parseFloat(getComputedStyle(node).paddingLeft)):heading.x;
  if(!next||Math.abs(next.x-expectedX)>1||Math.abs(next.y-heading.y)>1||Math.abs(next.height-heading.height)>1)throw Error('Settings title left its content bounds or shared vertical alignment: '+path);
 }
 for(const language of ['en','zh-CN']){
  await context.request.post(url+'/settings/language',{form:{language}});
  for(const width of [1400,390,320]){
   await page.setViewportSize({width,height:844});await page.goto(url+'/settings/apps');await settle();
   if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error(`Apps overflowed at ${width}px`);
   if(await page.locator('.settings-navigation a[aria-current="page"]').count()!==1||!await page.locator('.settings-navigation a[href="/settings/apps"][aria-current="page"]').count())throw Error('Apps selection disagrees with Settings navigation');
   if(!await page.getByText('Gmail',{exact:true}).count()||await page.locator('button[value="connect"]').count())throw Error('unconfigured directory lost suggestions or enabled connection actions');
   await page.screenshot({path:`${evidence}/apps-${language}-${width}.png`});
   await page.locator('#app-search').fill('Notion');await page.locator('#app-search').press('Enter');
   if(!await page.getByText('Notion',{exact:true}).count()||await page.getByText('Gmail',{exact:true}).count())throw Error('Apps search did not filter the directory');
  }
 }
 await context.request.post(url+'/settings/language',{form:{language:'en'}});
 await page.setViewportSize({width:1400,height:980});await page.goto(url+'/settings/capabilities');
 // Collapse once, then cross every module and reload. The shared preference
 // must be respected even when the destination has two navigation rails.
 await page.locator('.brand-row [data-sidebar-toggle]').click();
 for(const path of ['/settings/preferences','/settings/setup','/developer/observability','/developer/database','/tasks','/']){
  await page.goto(url+path);await page.reload();
  const state=await page.evaluate(()=>({path:location.pathname,main:document.querySelector('main').getBoundingClientRect().x,sidebar:getComputedStyle(document.querySelector('.app-sidebar')).display}));
  if(state.main!==64||state.sidebar==='none'||!await page.locator('.brand-row [data-sidebar-toggle]').isVisible())throw Error('collapsed navigation reset: '+JSON.stringify(state));
  measurements.collapse.push(state);
 }
 for(const [width,path,collapsed,theme] of [[390,'/settings/preferences',false,'light'],[320,'/settings/preferences',false,'dark'],[1400,'/settings/preferences',false,'light'],[1400,'/',true,'dark'],[390,'/tasks',false,'dark']]){
  await page.setViewportSize({width,height:980});
  await page.evaluate(({collapsed,theme})=>{localStorage.setItem('steward-sidebar',collapsed?'collapsed':'open');localStorage.setItem('steward-theme',theme)},{collapsed,theme});
  let release;const held=new Promise(resolve=>{release=resolve});
  await page.route('**/app.js',async route=>{await held;await route.continue()});
  try{
   await page.goto(url+path,{waitUntil:'commit'});await page.locator('main').waitFor();
   await page.waitForFunction(()=>document.querySelector('link[rel=stylesheet]')?.sheet);await settle();
   if(await page.locator('[data-sidebar-toggle],[data-sidebar-close]').evaluateAll(buttons=>buttons.some(button=>!button.disabled)))throw Error('navigation became actionable before its event handlers loaded');
   const geometry=()=>{
    const selectors=['.workspace-main','.page-heading','.settings-group','.settings-footer','[data-appearance-setting]','[data-avatar-setting]',...Array.from({length:5},(_,i)=>`.settings-navigation a:nth-child(${i+1})`)];
    return {dark:document.documentElement.classList.contains('dark'),boxes:Object.fromEntries(selectors.map(selector=>{const node=document.querySelector(selector);return [selector,node?.getBoundingClientRect().toJSON()??null]}))};
   };
   const before=await page.evaluate(geometry);
   if(before.dark!==(theme==='dark')||before.boxes['.workspace-main'].y!==0||collapsed&&before.boxes['.workspace-main'].x!==64)throw Error('first paint used the wrong theme/navigation');
   await page.screenshot({path:`${evidence}/loading-${width}-${path.split('/').filter(Boolean).join('-')}-before.png`});
   release();await page.waitForLoadState('load');await settle();const after=await page.evaluate(geometry);
   for(const [selector,rect] of Object.entries(before.boxes))if(rect){
    const next=after.boxes[selector];
    if(!next||['x','y','width','height'].some(key=>Math.abs(rect[key]-next[key])>1))throw Error('bundle initialization moved '+selector+': '+JSON.stringify({width,path,before:rect,after:next}));
   }
   if(await page.locator('[data-sidebar-toggle],[data-sidebar-close]').evaluateAll(buttons=>buttons.some(button=>button.disabled)))throw Error('initialized navigation remained disabled');
   const toggle=page.locator(width<=760?'.sidebar-open':'.brand-row [data-sidebar-toggle]');
   await toggle.click();
   await page.waitForFunction(({mobile,collapsed})=>mobile?document.documentElement.classList.contains('mobile-nav'):document.documentElement.classList.contains('sidebar-collapsed')!==collapsed,{mobile:width<=760,collapsed});
   await page.locator('.brand-row [data-sidebar-toggle]').click();
   await page.waitForFunction(({mobile,collapsed})=>mobile?!document.documentElement.classList.contains('mobile-nav'):document.documentElement.classList.contains('sidebar-collapsed')===collapsed,{mobile:width<=760,collapsed});
   measurements.loading.push({width,path,theme,collapsed,before,after});
   await page.screenshot({path:`${evidence}/loading-${width}-${path.split('/').filter(Boolean).join('-')}-after.png`});
  }finally{release();await page.unroute('**/app.js')}
 }
 // A failed bundle download must reveal the no-script form/navigation fallback.
 await page.route('**/app.js',route=>route.abort());await page.goto(url+'/settings/preferences');await page.unroute('**/app.js');
 if(!await page.locator('.app-sidebar').isVisible()||!await page.locator('.preference-save').isVisible()||!await page.locator('#interface-language').isVisible())throw Error('bundle failure left native navigation or form inaccessible');
 await page.setViewportSize({width:1400,height:980});await page.goto(url+'/settings/notifications');
 const target=page.locator('#notification-target');await selectValue(page,target,'tui:');
 await Promise.all([page.waitForNavigation({waitUntil:'load'}),page.getByRole('button',{name:'Save notification channel',exact:true}).click()]);
 await page.locator('.page-feedback.success').waitFor();await selectValue(page,target,'auto:');
 if(!await page.locator('.page-feedback.success').isHidden()||!await page.locator('[data-notification-draft]').isVisible())throw Error('editing retained stale saved feedback');
 await page.reload();if(await target.inputValue()!=='tui:'||await page.locator('[data-notification-draft]').isVisible())throw Error('notification draft was saved without submission or persisted after reload');
 await selectValue(page,target,'auto:');await Promise.all([page.waitForNavigation({waitUntil:'load'}),page.getByRole('button',{name:'Save notification channel',exact:true}).click()]);
 await page.reload();if(await target.inputValue()!=='auto:')throw Error('explicit notification save was lost');
 await page.screenshot({path:evidence+'/notifications-saved.png'});
 for(const theme of ['light','dark']){
  await page.evaluate(value=>localStorage.setItem('steward-theme',value),theme);
  await page.goto(url+'/');await settle();
  for(const width of [1400,390]){
   await page.setViewportSize({width,height:980});
   const text=page.locator('.composer textarea');await text.click();
   for(const input of ['pointer','keyboard']){
    if(input==='keyboard'){await page.keyboard.press('Tab');await page.keyboard.press('Shift+Tab')}
    const focus=await text.evaluate(node=>{
     const composer=node.closest('.composer'),style=getComputedStyle(composer);
     return {active:node===document.activeElement,outline:parseFloat(style.outlineWidth),border:style.borderColor,background:style.backgroundColor,shadow:style.boxShadow};
    });
    if(!focus.active||focus.outline!==0||focus.border===focus.background||focus.shadow==='none')throw Error('composer focus added an outer frame or lost its indicator: '+JSON.stringify(focus));
    measurements.focus.push({theme,width,input,...focus});
   }
   await page.locator('.composer').screenshot({path:`${evidence}/composer-focus-${theme}-${width}.png`});
  }
 }
 await writeFile(evidence+'/page-transitions.json',JSON.stringify({...measurements,notificationDraft:true,bundleFailureFallback:true},null,2));
}

export async function capabilityDefaults(page,context,url,evidence){
 await page.goto(url+'/');
 await page.evaluate(()=>document.fonts.ready);
 const primary=page.getByRole('navigation',{name:'Workspace',exact:true});
 const primaryLinks=await primary.getByRole('link').evaluateAll(links=>links.map(link=>({href:link.getAttribute('href'),top:link.getBoundingClientRect().top,bottom:link.getBoundingClientRect().bottom})));
 if(primaryLinks.map(link=>link.href).join(',')!=='/,/tasks,/developer/observability,/settings/preferences'||await page.evaluate(()=>innerHeight)-primaryLinks[3].bottom>32)throw Error('primary navigation lost module order or bottom Settings entry');
 await primary.getByRole('link',{name:'Settings',exact:true}).click();
 const stableLinks=await primary.getByRole('link').evaluateAll(links=>links.map(link=>link.getBoundingClientRect().top));
 if(stableLinks.some((top,i)=>Math.abs(top-primaryLinks[i].top)>1))throw Error('opening Settings moved the primary navigation');
 const rails=await page.evaluate(()=>{const primary=document.querySelector('.app-sidebar').getBoundingClientRect(),secondary=document.querySelector('.section-sidebar').getBoundingClientRect(),main=document.querySelector('.workspace-main').getBoundingClientRect();return {primary:primary.right,secondaryLeft:secondary.left,secondaryRight:secondary.right,mainLeft:main.left}});
 if(rails.primary!==rails.secondaryLeft||rails.secondaryRight!==rails.mainLeft)throw Error('secondary navigation replaced or overlaps the primary rail/content');
 await primary.getByRole('link',{name:'Developer',exact:true}).click();
 await page.getByRole('navigation',{name:'Developer sections'}).getByRole('link',{name:'Database',exact:true}).click();
 await page.screenshot({path:evidence+'/developer-navigation.png',fullPage:true});
 await primary.getByRole('link',{name:'Tasks',exact:true}).click();
 if(await page.locator('.section-sidebar').count())throw Error('Tasks acquired empty secondary categories');
 await primary.getByRole('link',{name:'Settings',exact:true}).click();
 // Explicit collapse can be reversed without leaving a settings page.
 await page.locator('.brand-row').getByRole('button',{name:'Close navigation',exact:true}).click();
 await page.getByRole('button',{name:'Open navigation',exact:true}).click();
 if(!await page.locator('.section-sidebar').isVisible())throw Error('collapsed settings navigation could not be reopened');
 // Low-frequency presentation preferences live in Preferences, not the rail.
 if(await page.locator('.app-sidebar').innerText().then(text=>text.includes('Your personal AI steward.'))||await page.locator('[data-theme-toggle],[data-avatar-pause]').count())throw Error('sidebar retained duplicate presentation controls');
 const motion=page.getByRole('switch',{name:'Avatar motion',exact:true});await motion.uncheck();
 await primary.getByRole('link',{name:'Chat',exact:true}).click();
 const avatar=page.locator('.steward-avatar').first();await avatar.waitFor();
 await page.waitForFunction(()=>document.querySelector('.steward-avatar')?.dataset.renderer==='prerendered');
 if(await avatar.getAttribute('data-frame')!=='0'||await avatar.locator('.ghost-art').evaluate(node=>getComputedStyle(node).animationPlayState)!=='paused')throw Error('saved avatar preference did not stop motion in Chat');
 await primary.getByRole('link',{name:'Settings',exact:true}).click();
 if(await motion.isChecked())throw Error('avatar preference did not persist across pages');
 await page.reload();if(await motion.isChecked())throw Error('avatar preference did not persist on reload');
 await motion.check();
 // System reduced motion takes precedence over the local animation setting.
 const quiet=await page.context().browser().newContext({reducedMotion:'reduce'});
 try{
  await quiet.addCookies(await context.cookies());const p=await quiet.newPage();await p.goto(url+'/settings/preferences');
  const control=p.getByRole('switch',{name:'Avatar motion',exact:true});await control.waitFor();
  if(!await control.isDisabled()||await control.isChecked())throw Error('reduced motion preference was ignored');
  await p.getByText('Follows your system’s reduced motion preference.',{exact:true}).waitFor();
  await p.emulateMedia({reducedMotion:'no-preference'});
  await p.waitForFunction(()=>{const input=document.querySelector('[data-avatar-motion]');return !input.disabled&&input.checked});
  await control.uncheck();
  await p.emulateMedia({reducedMotion:'reduce'});
  await p.waitForFunction(()=>document.querySelector('[data-avatar-motion]').disabled);
  await p.emulateMedia({reducedMotion:'no-preference'});
  await p.waitForFunction(()=>{const input=document.querySelector('[data-avatar-motion]');return !input.disabled&&!input.checked});
  if(await p.evaluate(()=>localStorage.getItem('steward-avatar-motion'))!=='paused')throw Error('system motion changes overwrote the saved user preference');
 }finally{await quiet.close()}
 // A two-option language list uses native Select without a search field.
 const language=page.locator('[data-language-trigger]');
 const languagePopup=page.locator('[data-language-popup]');
 await language.click();await languagePopup.waitFor();
 if(await languagePopup.locator('input').count()||await languagePopup.getByRole('option').count()!==2)throw Error('short language list gained search or lost an option');
 await page.screenshot({path:evidence+'/language-dropdown.png',fullPage:true});
 await page.keyboard.press('End');await page.keyboard.press('Enter');
 await page.waitForURL(u=>u.searchParams.get('language')==='zh-CN');await page.waitForLoadState();
 await page.goto(url+'/settings/preferences');await page.locator('[data-settings-search]:not([hidden])').waitFor();
 await page.screenshot({path:evidence+'/preferences.png',fullPage:true});
 await language.press('Enter');await page.waitForFunction(()=>document.activeElement?.getAttribute('role')==='option');await page.keyboard.press('ArrowUp');await page.keyboard.press('Escape');await languagePopup.waitFor({state:'hidden'});await language.and(page.locator(':focus')).waitFor();
 if(!await language.evaluate(node=>node===document.activeElement)||await languagePopup.isVisible())throw Error('Escape lost dropdown focus');
 await language.press('Enter');await page.waitForFunction(()=>document.activeElement?.getAttribute('role')==='option');await page.keyboard.press('Tab');await languagePopup.waitFor({state:'hidden'});
 if(!await page.locator('#interface-theme-picker').evaluate(node=>node===document.activeElement)||await languagePopup.isVisible())throw Error('Tab did not continue to the next setting');
 await language.click();await languagePopup.waitFor();
 await page.waitForFunction(()=>!document.querySelector('[data-language-popup]')?.hasAttribute('data-starting-style'));
 const heading=await page.getByRole('heading',{level:1}).boundingBox();await page.mouse.click(heading.x+heading.width/2,heading.y+heading.height/2);await languagePopup.waitFor({state:'hidden'});
 if(await languagePopup.isVisible())throw Error('outside click kept dropdown open');
 const popup=page.locator('.control-select-menu:visible');
 const navSearch=page.getByRole('searchbox',{name:'搜索设置',exact:true});await navSearch.fill('通知');
 if(await page.locator('.settings-navigation a:visible').count()!==1)throw Error('settings navigation filter did not match');
 await navSearch.fill('zz-no-result');await page.getByText('未找到设置。',{exact:true}).waitFor();await navSearch.press('Escape');
 if(await page.locator('.settings-navigation a:visible').count()!==5)throw Error('settings filter did not reset');
 await navigationViewports(page,evidence);
 await selectValue(page,page.locator('#interface-theme'),'dark');
 if(!await page.locator('html').evaluate(node=>node.classList.contains('dark')))throw Error('appearance did not apply');
 await page.reload();await page.locator('#interface-theme-picker').waitFor();
 if(await page.locator('#interface-theme').inputValue()!=='dark')throw Error('appearance did not persist');
 await page.screenshot({path:evidence+'/preferences-dark.png',fullPage:true});
 await selectValue(page,page.locator('#interface-theme'),'light');
 await page.goto(url+'/settings/notifications');await page.screenshot({path:evidence+'/notifications.png',fullPage:true});
 await page.goto(url+'/settings/models');await page.screenshot({path:evidence+'/models.png',fullPage:true});
 await page.goto(url+'/settings/models/new?provider=compatible');await page.screenshot({path:evidence+'/model-inputs.png',fullPage:true});
 await page.goto(url+'/settings/capabilities');
 if(await page.locator('.object-row').count()!==5)throw Error('duplicate Memory search entry');
 const memory=page.locator('.object-row[href="/settings/capabilities/memory"]');
 await memory.getByText('已启用（默认）',{exact:true}).waitFor();
 await page.screenshot({path:evidence+'/capabilities-default.png',fullPage:true});
 await memory.click();
 await page.getByText('已启用（默认）',{exact:true}).waitFor();
 if(!await page.locator('section.panel.form-panel').isVisible()||await page.locator('[data-capability-fields]').isVisible())throw Error('default mode exposed custom configuration');
 await selectValue(page,page.locator('#capability-mode'),'custom');
 if(!await page.locator('[data-capability-fields]').isVisible())throw Error('custom dropdown did not reveal inputs');
 await selectValue(page,page.locator('#capability-mode'),'automatic');
 await page.goto(url+'/settings/capabilities/memory');await page.locator('[data-capability-fields][hidden]').waitFor({state:'hidden'});
 await page.screenshot({path:evidence+'/memory-default.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('Memory settings overflowed on mobile');
 await page.screenshot({path:evidence+'/memory-mobile.png',fullPage:true});
 await page.getByRole('button',{name:'打开导航',exact:true}).click();
 const bothRails=await page.evaluate(()=>{const first=document.querySelector('.app-sidebar').getBoundingClientRect(),second=document.querySelector('.section-sidebar').getBoundingClientRect();return first.left>=0&&first.right<=second.left&&second.right<=innerWidth});
 if(!bothRails||!await page.locator('.app-sidebar [aria-current="page"]').isVisible()||!await page.locator('.settings-navigation [aria-current="page"]').isVisible())throw Error('mobile navigation did not preserve both levels');
 const last=page.locator('.settings-navigation a').last();await last.focus();await page.keyboard.press('Tab');
 if(!await page.locator('.app-sidebar .brand').evaluate(node=>node===document.activeElement))throw Error('two-level drawer did not wrap keyboard focus');
 await page.screenshot({path:evidence+'/settings-navigation-mobile.png',fullPage:true});
 await page.keyboard.press('Escape');
 if(!await page.getByRole('button',{name:'打开导航',exact:true}).evaluate(node=>node===document.activeElement))throw Error('mobile settings navigation lost focus');
 // All settings remain navigable and submit real forms without the bundle.
 const plain=await page.context().browser().newContext({javaScriptEnabled:false,viewport:{width:390,height:844}});
 try{
  await plain.addCookies(await context.cookies());const p=await plain.newPage();await p.goto(url+'/settings/preferences');
  await p.locator('#interface-language').selectOption('en');await p.getByRole('button',{name:'保存',exact:true}).click();
  if(await p.locator('html').getAttribute('lang')!=='en')throw Error('native language fallback did not save');
  await p.screenshot({path:evidence+'/settings-no-script.png',fullPage:true});
 }finally{await plain.close()}
}

export async function capabilitySetup(page,context,url,evidence){
 const facts=JSON.parse(process.env.STEWARD_CAPABILITY_SETUP_FACTS);
 const guided=()=>page.locator('form.capability-guided');
 const status=form=>form.locator('[data-capability-draft]');
 const response=()=>page.waitForResponse(r=>new URL(r.url()).pathname==='/capability/configure'&&r.request().method()==='POST');
 const open=async id=>{await page.goto(url+'/settings/capabilities/'+id);const disclosure=page.locator('.capability-advanced');await disclosure.locator('summary').first().click()};
 const apply=async(value,error)=>{
  const text=typeof value==='string'?value:JSON.stringify(value,null,2);
  const form=page.locator('.capability-editor');
  await Promise.all([response(),form.locator('[data-capability-import]').setInputFiles({name:'settings.yaml',mimeType:'application/yaml',buffer:Buffer.from(text)})]);
  await status(form).filter({hasText:error??'Saved.'}).waitFor();
 };
 await page.goto(url+'/settings/capabilities');
 if(await page.locator('.object-row').count()!==5||await page.locator('.settings-footer').count())throw Error('capability navigation or footer regressed');
 await page.goto(url+'/settings/capabilities/skills');
 if(await guided().getByRole('button',{name:'Save',exact:true}).count())throw Error('enhanced capability form still requires Save');
 for(const mode of ['automatic','off']){
  await Promise.all([response(),selectValue(page,guided().getByLabel('Configuration mode',{exact:true}),mode)]);
  await status(guided()).getByText('Saved.',{exact:true}).waitFor();await page.reload();
  if(await (await selectParts(guided().getByLabel('Configuration mode',{exact:true}))).native.inputValue()!==mode)throw Error('automatic mode/opt-out did not persist');
 }
 await page.goto(url+'/settings/capabilities/memory');
 if(await page.locator('.capability-advanced').evaluate(node=>node.open))throw Error('Memory did not lead with the compact form');
 const revision=await guided().locator('[name=revision]').inputValue();
 await selectValue(page,guided().getByLabel('Configuration mode',{exact:true}),'custom');
 await Promise.all([response(),guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_MISSING_SETUP_KEY')]);
 await status(guided()).filter({hasText:'Memory key unavailable'}).waitFor();
 if(await guided().locator('[name=revision]').inputValue()!==revision)throw Error('failed automatic save changed the file revision');
 await page.screenshot({path:evidence+'/memory-guided-failed.png',fullPage:true});
 await Promise.all([response(),guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_SETUP_MEMORY_KEY')]);
 await status(guided()).filter({hasText:'Saved.'}).waitFor();
 if(await guided().locator('[name=revision]').inputValue()===revision)throw Error('automatic save did not commit');
 await page.screenshot({path:evidence+'/memory-guided-saved.png',fullPage:true});
 await open('memory');
 if(await page.locator('#capability-configuration').isVisible())throw Error('YAML textarea dominates the import flow');
 const before=await page.locator('.capability-editor [name=revision]').inputValue();
 await apply('encryption_key_ref: env:STEWARD_MISSING_SETUP_KEY\n','Memory key unavailable');
 if(await page.locator('.capability-editor [name=revision]').inputValue()!==before)throw Error('invalid imported YAML persisted');
 await apply('encryption_key_ref: env:STEWARD_SETUP_MEMORY_KEY # imported\n');
 await page.screenshot({path:evidence+'/memory-yaml-import.png',fullPage:true});
 const download=page.waitForEvent('download');await page.getByRole('button',{name:'Download example',exact:true}).click();if((await download).suggestedFilename()!=='memory.yaml')throw Error('example download lost its file type');
 await open('embedding');await apply({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:2},'Embedding connection failed');
 await apply({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:5});
 await open('skills');await apply({root:facts.skills+'/missing',scope:'agent'},'Skill catalog unavailable');await apply({root:facts.skills,scope:'agent'});
 await open('mcp');
 const provider={name:'owner_catalog',command:facts.binary,args:['-test-mcp-server',facts.provider],directory:facts.provider,tools:{not_advertised:'read_only'}};
 await apply([provider],'MCP discovery failed');provider.tools={read:'read_only',send:'external_effect'};await apply([provider]);
 await page.screenshot({path:evidence+'/mcp-imported.png',fullPage:true});
 // Native forms and native file import still validate and save without JavaScript.
 const plain=await page.context().browser().newContext({javaScriptEnabled:false,viewport:{width:390,height:844}});
 try{
  await plain.addCookies(await context.cookies());const p=await plain.newPage();await p.goto(url+'/settings/capabilities/embedding');
  await p.locator('.capability-guided').getByRole('button',{name:'Save',exact:true}).click();
  const advanced=p.locator('.capability-advanced');await advanced.locator('summary').first().click();
  await advanced.locator('[data-capability-import]').setInputFiles({name:'embedding.yml',mimeType:'application/yaml',buffer:Buffer.from(JSON.stringify({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:5}))});
  await advanced.getByRole('button',{name:'Apply configuration',exact:true}).click();
  if(await p.locator('.feedback.error').count())throw Error('native YAML import failed');
  await p.screenshot({path:evidence+'/capability-no-script.png',fullPage:true});
 }finally{await plain.close()}
 await open('execution');await apply({root:facts.workspace,toolchain_roots:[]});await apply('');
 await open('computer');await apply(facts.computer);
 await page.goto(url+'/settings/capabilities');
 if(await page.getByText('Restart required',{exact:true}).count()!==5)throw Error('saved state was confused with active runtime');
 await page.screenshot({path:evidence+'/capabilities-configured.png',fullPage:true});
 await page.setViewportSize({width:320,height:740});if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('capabilities overflow compact viewport');
 await page.screenshot({path:evidence+'/capabilities-mobile.png',fullPage:true});
}

export async function settingsAccount(page,context,url,evidence){
 await page.goto(url+'/settings/setup');
 await page.locator('details.resource-account').first().locator(':scope > summary').click();
 let posts=0;page.on('request',request=>{if(new URL(request.url()).pathname==='/auth/logout'&&request.method()==='POST')posts++});
 const logout=page.getByRole('button',{name:'Sign out of provider',exact:true});
 await logout.click();
 const dialog=page.getByRole('dialog',{name:'Sign out of provider',exact:true});await dialog.waitFor();
 if(posts!==0||!await dialog.getByRole('button',{name:'Cancel',exact:true}).evaluate(node=>node===document.activeElement))throw Error('sign-out confirmation submitted early or lost safe focus');
 const centered=await dialog.evaluate(node=>{const rect=node.getBoundingClientRect();return Math.abs(rect.x+rect.width/2-innerWidth/2)<2&&Math.abs(rect.y+rect.height/2-innerHeight/2)<2});
 if(!centered)throw Error('confirmation dialog is outside the center of the viewport');
 await page.screenshot({path:evidence+'/signout-confirmation.png',fullPage:true});
 await dialog.getByRole('button',{name:'Cancel',exact:true}).click();
 if(posts!==0||!await logout.evaluate(node=>node===document.activeElement))throw Error('cancel changed account or lost focus');
 await logout.click();await page.keyboard.press('Escape');
 if(posts!==0||await dialog.count())throw Error('Escape changed account');
 await logout.click();
 await Promise.all([page.waitForNavigation({waitUntil:'domcontentloaded'}),dialog.getByRole('button',{name:'Sign out of provider',exact:true}).click()]);
 if(posts!==1)throw Error('confirmed sign-out was submitted more than once');
}
