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
 // Use the actual dropdown, search/filter, keyboard selection and native submit.
 await page.locator('#interface-language').click();
 const popup=page.locator('.settings-select-menu:visible');
 await popup.getByRole('searchbox').fill('简体');
 if(await popup.getByRole('option').count()!==1)throw Error('language search did not filter');
 await page.screenshot({path:evidence+'/language-dropdown.png',fullPage:true});
 await popup.getByRole('searchbox').press('ArrowDown');await page.keyboard.press('Enter');
 if(await page.locator('#interface-language').inputValue()!=='zh-CN')throw Error('keyboard selection lost language value');
 await Promise.all([page.waitForNavigation({waitUntil:'domcontentloaded'}),page.getByRole('button',{name:'Save language',exact:true}).click()]);
 await page.locator('[data-settings-search]:not([hidden])').waitFor();
 await page.goto(url+'/settings/preferences');await page.locator('[data-settings-search]:not([hidden])').waitFor();
 await page.screenshot({path:evidence+'/preferences.png',fullPage:true});
 await page.locator('#interface-language').press('Enter');await popup.getByRole('searchbox').fill('zz-no-result');
 await popup.getByText('无结果。',{exact:true}).waitFor();await page.keyboard.press('Escape');
 if(!await page.locator('#interface-language').evaluate(node=>node===document.activeElement)||await popup.count())throw Error('Escape lost dropdown focus');
 await page.locator('#interface-language').press('Enter');await popup.getByRole('searchbox').press('Tab');
 if(!await page.locator('#interface-theme').evaluate(node=>node===document.activeElement)||await popup.count())throw Error('Tab did not continue to the next setting');
 await page.locator('#interface-language').click();await page.getByRole('heading',{level:1}).click();
 if(await popup.count())throw Error('outside click kept dropdown open');
 const navSearch=page.getByRole('searchbox',{name:'搜索设置',exact:true});await navSearch.fill('通知');
 if(await page.locator('.settings-navigation a:visible').count()!==1)throw Error('settings navigation filter did not match');
 await navSearch.fill('zz-no-result');await page.getByText('未找到设置。',{exact:true}).waitFor();await navSearch.press('Escape');
 if(await page.locator('.settings-navigation a:visible').count()!==4)throw Error('settings filter did not reset');
 await navigationViewports(page,evidence);
 await page.locator('#interface-theme').selectOption('dark');
 if(!await page.locator('html').evaluate(node=>node.classList.contains('dark')))throw Error('appearance did not apply');
 await page.reload();await page.locator('#interface-theme').waitFor();
 if(await page.locator('#interface-theme').inputValue()!=='dark')throw Error('appearance did not persist');
 await page.screenshot({path:evidence+'/preferences-dark.png',fullPage:true});
 await page.locator('#interface-theme').selectOption('light');
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
 await page.locator('#capability-mode').click();await popup.getByRole('option',{name:'自定义',exact:true}).click();
 if(!await page.locator('[data-capability-fields]').isVisible())throw Error('custom dropdown did not reveal inputs');
 await page.locator('#capability-mode').selectOption('automatic');
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
  await p.locator('#interface-language').selectOption('en');await p.getByRole('button',{name:'保存语言',exact:true}).click();
  if(await p.locator('html').getAttribute('lang')!=='en')throw Error('native language fallback did not save');
  await p.screenshot({path:evidence+'/settings-no-script.png',fullPage:true});
 }finally{await plain.close()}
}

export async function capabilitySetup(page,context,url,evidence){
 const facts=JSON.parse(process.env.STEWARD_CAPABILITY_SETUP_FACTS);
 const advanced=async target=>{
  const disclosure=target.locator('details.capability-advanced');
  if(!await disclosure.evaluate(node=>node.open))await disclosure.locator('summary').first().click();
  await target.locator('#capability-configuration').waitFor({state:'visible'});
 };
 const open=async id=>{await page.goto(url+'/settings/capabilities/'+id);await advanced(page)};
 const guided=()=>page.locator('form.capability-guided');
 const guidedTest=async()=>{
  await guided().getByRole('button',{name:'Check connection',exact:true}).click();
  await page.waitForURL(url+'/settings/capabilities/*');
  if(new URL(page.url()).searchParams.has('editor'))throw Error('guided test switched to Advanced YAML');
 };
 const sourcePath=()=>page.locator('details.technical-details').filter({has:page.getByText('Connection details',{exact:true})}).last().locator('code').textContent();
 const test=async text=>{
  await advanced(page);
  await page.locator('#capability-configuration').fill(typeof text==='string'?text:JSON.stringify(text,null,2));
  await Promise.all([page.waitForURL(url+'/settings/capabilities/*'),page.getByRole('button',{name:'Test configuration',exact:true}).click()]);
  if(!new URL(page.url()).pathname.startsWith('/settings/capabilities/')||new URL(page.url()).searchParams.get('editor')!=='advanced'||!await page.locator('details.capability-advanced').evaluate(node=>node.open))throw Error('advanced test lost its canonical editor route');
 };
 const save=async()=>{await page.getByRole('button',{name:'Save verified configuration',exact:true}).click();if(await page.getByText('Unsaved candidate.',{exact:false}).count())throw Error('candidate was not persisted')};
 await page.goto(url+'/settings/models');await page.getByRole('link',{name:'Capabilities',exact:true}).click();
 if(await page.locator('.object-row').count()!==5||await page.locator('.object-row[href="/settings/capabilities/embedding"]').count())throw Error('Memory search must not be a separate capability');
 await page.screenshot({path:evidence+'/capabilities-before.png',fullPage:true});
 // Default mode and an explicit opt-out survive the same public form/PRG path.
 await page.goto(url+'/settings/capabilities/skills');
 await guided().getByLabel('Configuration mode',{exact:true}).selectOption('automatic');
 await Promise.all([page.waitForNavigation({waitUntil:'domcontentloaded'}),guided().getByRole('button',{name:'Save',exact:true}).click()]);
 await page.reload();
 if(await guided().getByLabel('Configuration mode',{exact:true}).inputValue()!=='automatic')throw Error('automatic mode did not persist');
 await page.screenshot({path:evidence+'/skills-automatic.png',fullPage:true});
 await guided().getByLabel('Configuration mode',{exact:true}).selectOption('off');
 await Promise.all([page.waitForNavigation({waitUntil:'domcontentloaded'}),guided().getByRole('button',{name:'Save',exact:true}).click()]);
 await page.reload();
 if(await guided().getByLabel('Configuration mode',{exact:true}).inputValue()!=='off')throw Error('explicit opt-out did not persist');
 // Start with the ordinary form: no YAML or provider protocol knowledge is
 // needed to enable Memory, repair a failed probe, retest edits and save.
 await page.goto(url+'/settings/capabilities/memory');
 if(!await guided().isVisible()||await page.locator('details.capability-advanced').evaluate(node=>node.open))throw Error('Memory did not lead with its guided form');
 const initialRevision=await guided().locator('input[name="revision"]').inputValue(),initialPath=await sourcePath();
 await guided().getByLabel('Configuration mode',{exact:true}).selectOption('custom');
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_MISSING_SETUP_KEY');
 await guided().getByRole('button',{name:'Save',exact:true}).click();
 await page.getByRole('status').filter({hasText:'Memory key unavailable'}).waitFor();
 if(await guided().getByLabel('Encryption key reference',{exact:true}).inputValue()!=='env:STEWARD_MISSING_SETUP_KEY'||await guided().locator('input[name="revision"]').inputValue()!==initialRevision||await sourcePath()!==initialPath)throw Error('failed save lost the exact draft or source revision');
 await page.screenshot({path:evidence+'/memory-guided-failed.png',fullPage:true});
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_SETUP_MEMORY_KEY');
 await guidedTest();await page.getByRole('status').filter({hasText:'Connection available.'}).waitFor();
 await page.screenshot({path:evidence+'/memory-guided-tested.png',fullPage:true});
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_MISSING_SETUP_KEY');
 await guided().getByRole('button',{name:'Save',exact:true}).click();
 await page.getByRole('status').filter({hasText:'Memory key unavailable'}).waitFor();
 if(await guided().locator('input[name="revision"]').inputValue()!==initialRevision)throw Error('save reused a check for different fields');
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_SETUP_MEMORY_KEY');
 await guided().getByRole('button',{name:'Save',exact:true}).click();
 await page.getByRole('status').filter({hasText:'Changes saved. Restart Steward to apply.'}).waitFor();
 if(new URL(page.url()).pathname!=='/settings/capabilities/memory'||await guided().getByLabel('Encryption key reference',{exact:true}).inputValue()!=='env:STEWARD_SETUP_MEMORY_KEY'||await guided().locator('input[name="revision"]').inputValue()===initialRevision||await sourcePath()!==initialPath)throw Error('save lost fields, source path or configuration revision');
 await page.screenshot({path:evidence+'/memory-guided-saved.png',fullPage:true});
 // Keep the original Advanced YAML repair/probe/save coverage as well.
 await open('memory');
 await test('encryption_key_ref: env:STEWARD_MISSING_SETUP_KEY\n');
 await page.getByRole('status').filter({hasText:'Memory key unavailable'}).waitFor();
 if(!await page.getByRole('button',{name:'Save verified configuration',exact:true}).isDisabled())throw Error('failed candidate can be saved');
 await test('encryption_key_ref: env:STEWARD_SETUP_MEMORY_KEY\n');
 await page.getByRole('status').filter({hasText:'Connection available.'}).waitFor();
 await page.reload();if(!await page.locator('#capability-configuration').inputValue().then(v=>v.includes('STEWARD_SETUP_MEMORY_KEY')))throw Error('PRG lost tested draft');
 await save();await page.getByRole('status').filter({hasText:'Changes saved. Restart Steward to apply.'}).waitFor();
 await page.screenshot({path:evidence+'/memory-saved.png',fullPage:true});
 await open('embedding');
 await test({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:2});
 await page.getByRole('status').filter({hasText:'Embedding connection failed'}).waitFor();
 await test({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:5});
 await page.getByText('Connection available.',{exact:false}).waitFor();await save();
 await open('skills');await test({root:facts.skills+'/missing',scope:'agent'});await page.getByText('Skill catalog unavailable.',{exact:false}).waitFor();
 await test({root:facts.skills,scope:'agent'});await page.getByText('Connection available.',{exact:false}).waitFor();await save();
 // Repeated provider/tool controls use native form actions. Preserve the
 // supplied fixture paths and exact arguments through add/remove and Test/Save.
 await page.goto(url+'/settings/capabilities/mcp');
 await guided().getByRole('switch',{name:'Enable this capability',exact:true}).check();
 await guided().getByLabel('Provider name',{exact:true}).fill('owner_catalog');
 await guided().getByLabel('Executable path',{exact:true}).fill(facts.binary);
 await guided().getByLabel('Private state directory',{exact:true}).fill(facts.provider);
 await guided().getByLabel('Command arguments (optional)',{exact:false}).fill('-test-mcp-server\n'+facts.provider);
 await guided().getByLabel('Tool name',{exact:true}).fill('send');
 await guided().getByLabel('Permission',{exact:false}).selectOption('external_effect');
 const mcpRevision=await guided().locator('input[name="revision"]').inputValue();
 await guided().getByRole('button',{name:'Add allowed tool',exact:true}).click();
 await guided().getByLabel('Tool name',{exact:true}).nth(1).fill('read');
 await guided().getByRole('button',{name:'Add tool provider',exact:true}).click();
 if(await guided().locator('.capability-provider').count()!==2)throw Error('native provider add did not retain both rows');
 await guided().locator('.capability-provider').nth(1).getByRole('button',{name:'Remove provider',exact:true}).click();
 if(await guided().locator('.capability-provider').count()!==1||await guided().getByLabel('Provider name',{exact:true}).inputValue()!=='owner_catalog'||await guided().getByLabel('Executable path',{exact:true}).inputValue()!==facts.binary||await guided().getByLabel('Private state directory',{exact:true}).inputValue()!==facts.provider||await guided().getByLabel('Command arguments (optional)',{exact:false}).inputValue()!=='-test-mcp-server\n'+facts.provider||await guided().locator('input[name="revision"]').inputValue()!==mcpRevision)throw Error('native MCP row actions lost provider fields or revision');
 await guidedTest();await page.getByText('Connection available.',{exact:false}).waitFor();
 if(await guided().getByLabel('Tool name',{exact:true}).count()!==2||await guided().getByLabel('Tool name',{exact:true}).nth(0).inputValue()!=='send'||await guided().getByLabel('Tool name',{exact:true}).nth(1).inputValue()!=='read'||await guided().getByLabel('Permission',{exact:false}).nth(0).inputValue()!=='external_effect')throw Error('guided MCP test changed its allowlist or permissions');
 await guided().getByRole('button',{name:'Save',exact:true}).click();
 if(await page.getByText('Unsaved candidate.',{exact:false}).count()||await guided().getByLabel('Command arguments (optional)',{exact:false}).inputValue()!=='-test-mcp-server\n'+facts.provider)throw Error('guided MCP save failed exact-candidate verification');
 await page.screenshot({path:evidence+'/mcp-guided-saved.png',fullPage:true});
 await open('mcp');
 const provider={name:'owner_catalog',command:facts.binary,args:['-test-mcp-server',facts.provider],directory:facts.provider,tools:{not_advertised:'read_only'}};
 await test([provider]);await page.getByText('MCP discovery failed.',{exact:false}).waitFor();
 provider.tools={read:'read_only',send:'external_effect'};await test([provider]);await page.getByText('Connection available.',{exact:false}).waitFor();await save();
 // Verify the native fallback before platform execution diagnostics, so an
 // unavailable kernel workspace does not hide form accessibility evidence.
 const plain=await page.context().browser().newContext({javaScriptEnabled:false,viewport:{width:390,height:844}});
 try{
  await plain.addCookies(await context.cookies());const p=await plain.newPage();
  await p.goto(url+'/settings/capabilities/embedding');
  const form=p.locator('form.capability-guided');
  if(!await form.isVisible()||await p.locator('details.capability-advanced').evaluate(node=>node.open)||await form.getByLabel('Endpoint URL',{exact:true}).inputValue()!==facts.embedding||await form.getByLabel('Vector dimensions',{exact:true}).inputValue()!=='5')throw Error('no-JavaScript guided form lost saved fields');
  await form.getByRole('button',{name:'Check connection',exact:true}).click();
  await p.getByText('Connection available.',{exact:false}).waitFor();
  await form.getByRole('button',{name:'Save',exact:true}).click();
  if(await p.getByText('Unsaved candidate.',{exact:false}).count())throw Error('no-JavaScript guided settings were not saved');
  await p.screenshot({path:evidence+'/capability-guided-no-script.png',fullPage:true});
  await advanced(p);
  await p.locator('#capability-configuration').fill('provider: openai_compatible\nendpoint: '+facts.embedding+'\nmodel: semantic-fixture-v1\ndimensions: 5\n');
  await p.getByRole('button',{name:'Test configuration',exact:true}).click();
  await p.getByText('Connection available.',{exact:false}).waitFor();
  if(new URL(p.url()).searchParams.get('editor')!=='advanced'||!await p.locator('details.capability-advanced').evaluate(node=>node.open))throw Error('no-JavaScript advanced PRG lost its editor');
  await p.screenshot({path:evidence+'/capability-no-script.png',fullPage:true});
 }finally{await plain.close()}
 await open('execution');await test({root:facts.workspace,toolchain_roots:[]});await page.getByText('Connection available.',{exact:false}).waitFor();await save();
 // Restore the implicit workspace so Computer can be the sole saved backend.
 await test('');await page.getByText('Connection available.',{exact:false}).waitFor();await save();
 await open('computer');await test(facts.computer);await page.getByText('Connection available.',{exact:false}).waitFor();
 await page.screenshot({path:evidence+'/computer-tested.png',fullPage:true});await save();
 await page.goto(url+'/settings/capabilities');
 if(await page.getByText('Restart required',{exact:true}).count()!==5)throw Error('saved configuration changes are not distinguished from running state');
 await page.screenshot({path:evidence+'/capabilities-configured.png',fullPage:true});
 await page.setViewportSize({width:320,height:740});
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('capabilities overflowed a compact viewport');
 await page.screenshot({path:evidence+'/capabilities-mobile.png',fullPage:true});

}

export async function settingsAccount(page,context,url,evidence){
 await page.goto(url+'/settings/models');
 await page.getByRole('link',{name:'Manage model →',exact:true}).click();
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
