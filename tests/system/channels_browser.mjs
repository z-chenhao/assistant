import {selectValue,selectParts} from './channels_controls.mjs';
// Maintained Web UI driver: one real browser, no app-specific mock DOM.
import {uiInteractions} from './channels_ui_interactions.mjs';
import {capabilitySetup,capabilityDefaults,settingsAccount,pageTransitions} from './channels_capabilities.mjs';
import {sendFeedback} from './channels_sending.mjs';
import {databaseInspection} from './channels_database.mjs';
import {preferences} from './channels_preferences.mjs';
import {chromium} from '../../guest-browser/node_modules/playwright-core/index.mjs';
import {mkdir,writeFile} from 'node:fs/promises';
const [url,evidence,phase='conversation']=process.argv.slice(2);
await mkdir(evidence,{recursive:true});
const options={headless:true};
if(process.env.STEWARD_BROWSER_EXECUTABLE)options.executablePath=process.env.STEWARD_BROWSER_EXECUTABLE;
async function chooseSharedOption(page,control,label,value){
 const{native,trigger}=await selectParts(control),original=await native.inputValue();
 await trigger.click();
 const list=page.getByRole('listbox',{name:'Options',exact:true}),popup=list.locator('xpath=..');
 if(await popup.locator('input[type=search]').count())throw Error('short option list unexpectedly requires search');
 await page.keyboard.press('Escape');await popup.waitFor({state:'hidden'});await trigger.and(page.locator(':focus')).waitFor();
 if(await native.inputValue()!==original)throw Error('shared select cancellation changed the value');
 const values=await native.locator('option').evaluateAll(options=>options.filter(option=>!option.disabled).map(option=>option.value)),target=values.indexOf(value);
 if(target<0)throw Error('missing enabled option: '+value);
 await trigger.press('Enter');await list.getByRole('option').and(page.locator(':focus')).waitFor();await page.keyboard.press('Home');
 for(let index=0;index<target;index++)await page.keyboard.press('ArrowDown');
 await list.getByRole('option',{name:label,exact:true}).and(page.locator('[data-highlighted]')).waitFor();await page.keyboard.press('Enter');
 await popup.waitFor({state:'hidden'});
 if(await native.inputValue()!==value||await popup.isVisible())throw Error('shared select did not confirm into the native form control');
}
const browser=await chromium.launch(options);
let page;
try{
 const context=await browser.newContext({viewport:{width:1400,height:980},deviceScaleFactor:1,reducedMotion:['rich-replies','intelligent-ui','ui-interactions','ui-interactions-restart'].includes(phase)?'reduce':'no-preference'});
 if(phase==='streaming'){
  await context.addInitScript(()=>{
   const NativeEventSource=window.EventSource;
   window.EventSource=class extends NativeEventSource {
    constructor(url,options){
     super(url,options);
     if(url==='/api/conversation/stream')window.dropReplyStream=()=>{
      this.close();
      this.dispatchEvent(new Event('error'));
     };
    }
   };
  });
 }
 page=await context.newPage();
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const remote=[];page.on('request',r=>{if(new URL(r.url()).origin!==new URL(url).origin)remote.push(r.url())});
 await page.goto(url);
 await page.getByLabel('Workspace access token').fill(process.env.STEWARD_WEB_TEST_TOKEN);
 await page.getByRole('button',{name:'Open workspace'}).click();
 await page.waitForURL(url+'/');
 await page.locator('#composer-root textarea').waitFor();
 // Prove the maintained frontend bundle is running, not just its HTML fallback.
 await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
 if(await page.locator('.message').count()&&!new URL(page.url()).hash){
  await page.waitForFunction(()=>{const message=document.querySelector('.message:last-child');return !message||message.getBoundingClientRect().bottom<=innerHeight});
 }
 if(!await page.getByRole('link',{name:'Steward home',exact:true}).isVisible())throw Error('Steward wordmark unavailable');
 if(await page.locator('.chat-identity .agent-name').innerText()!=='Ghost')throw Error('Chat agent identity is not Ghost');
 if(await page.locator('#conversation-messages .avatar-slot,#conversation-messages .steward-avatar').count())throw Error('conversation repeated the header avatar');
 if(await page.getByRole('navigation',{name:'Workspace',exact:true}).getByRole('link').count()!==4)throw Error('workspace navigation must have four entries');
 const avatar=page.locator('.chat-identity .steward-avatar');
 await avatar.waitFor();
 await page.waitForFunction(()=>document.querySelector('.steward-avatar')?.dataset.renderer==='prerendered');
 const canvas=await avatar.locator('canvas').elementHandle();
 const avatarSize=element=>{
  const {width,height}=element.getBoundingClientRect();
  return {width,height};
 };
 const restingSize=await avatar.evaluate(avatarSize);
 const assertAvatarSize=async state=>{
  const size=await avatar.evaluate(avatarSize);
  if(size.width!==restingSize.width||size.height!==restingSize.height){
   throw Error(`Chat avatar resized ${state}: ${restingSize.width}x${restingSize.height} → ${size.width}x${size.height}`);
  }
 };
 await avatar.hover();
 if(await page.evaluate(()=>matchMedia('(prefers-reduced-motion: reduce)').matches)){if(await avatar.evaluate(node=>node.classList.contains('is-waving')))throw Error('reduced motion started a greeting animation')}else await page.waitForFunction(()=>document.querySelector('.steward-avatar')?.classList.contains('is-waving'));
 await assertAvatarSize('on hover');
 if(!await canvas.evaluate(node=>node.isConnected))throw Error('greeting replaced the complete-frame canvas');
 await page.getByRole('link',{name:'Steward home',exact:true}).hover();
 await assertAvatarSize('after hover');
 if(phase==='preferences'){
  await preferences(page,context,browser,url,evidence);
 }else if(phase==='account-signin'||phase==='account-reconnect'){
  await context.route(requestURL=>requestURL.origin!==new URL(url).origin,route=>{
   const requestURL=new URL(route.request().url());
   return requestURL.origin==='https://auth.openai.com'&&requestURL.pathname==='/api/accounts/authorize'
    ?route.fulfill({contentType:'text/plain',body:'Controlled authorization page'})
    :route.abort();
  });
  if(phase==='account-reconnect'){
   for(const theme of ['light','dark']){
    await page.evaluate(value=>localStorage.setItem('steward-theme',value),theme);
    await page.goto(url+'/settings/models/new?provider=chatgpt');
    await page.getByRole('button',{name:'Load models',exact:true}).click();
    await page.locator('[data-model-provider-panel="chatgpt"] .control-select-trigger').waitFor();
    for(const width of [1400,320]){
     await page.setViewportSize({width,height:980});await page.evaluate(()=>document.fonts.ready);
     const layout=await page.evaluate(()=>{
      const panel=document.querySelector('[data-model-provider-panel="chatgpt"]');
      const buttons=[...panel.querySelectorAll('button:not([role=combobox])')].filter(node=>node.checkVisibility()).map(node=>node.getBoundingClientRect().toJSON());
      return {width:innerWidth,scrollWidth:document.documentElement.scrollWidth,accounts:panel.querySelectorAll('.connected-account strong').length,buttons,unwanted:!!document.querySelector('[name=image_input],.settings-footer')||panel.textContent.includes('Selected account:')||panel.textContent.includes('Choose an account model')};
     });
     if(layout.scrollWidth>width||layout.accounts!==1||layout.unwanted||layout.buttons.length!==2||layout.buttons[0].bottom+16>layout.buttons[1].top||layout.buttons.some(box=>box.left<0||box.right>width+1))throw Error('account connection repeats content or stacks actions: '+JSON.stringify(layout));
     await page.screenshot({path:`${evidence}/account-connect-${theme}-${width}.png`,fullPage:true});
    }
   }
   await page.setViewportSize({width:1400,height:980});
   await page.goto(url+'/settings/setup');
   const account=page.getByRole('region',{name:/^ChatGPT/});
   const current=account.locator('.resource-model').filter({hasText:'fixture-account-model'});
   await current.getByText('Current model',{exact:true}).waitFor();
   await account.getByText('connected',{exact:true}).waitFor();
   if(await current.getByRole('button',{name:/Test connection|Use model/}).count())throw Error('current verified model still exposes replaced actions');
   await account.locator('.resource-account > summary').click();
   if(await current.getByRole('button',{name:'Reconnect account',exact:true}).count())throw Error('account actions are repeated inside the model');
   await page.setViewportSize({width:320,height:740});
   if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('Resources overflowed the narrow layout');
   await page.setViewportSize({width:1400,height:980});
   if(await page.locator('#signin-status').innerText())throw Error('another page’s sign-in appeared in Resources');
   await page.getByRole('navigation',{name:'Workspace',exact:true}).getByRole('link',{name:'Settings',exact:true}).click();
   await selectValue(page,page.locator('#interface-theme'),'dark');
   await page.goto(url+'/settings/setup');
   await account.locator('.resource-account > summary').click();
   await page.waitForFunction(()=>document.documentElement.classList.contains('dark'));
   const reconnect=account.getByRole('button',{name:'Reconnect account',exact:true});
   const signOut=account.getByRole('button',{name:'Sign out of provider',exact:true});
   const [left,right]=await Promise.all([reconnect.boundingBox(),signOut.boundingBox()]);
   if(!left||!right||Math.abs(left.y-right.y)>1)throw Error('provider account actions sit on different baselines');
   await page.screenshot({path:evidence+'/model-account-dark.png',fullPage:true,animations:'disabled'});
   const originPath=new URL(page.url()).pathname;
   const popupOpened=context.waitForEvent('page');
   await reconnect.click();
   const popup=await popupOpened;
   await popup.getByText('Controlled authorization page').waitFor();
   await page.getByRole('button',{name:'Cancel sign-in',exact:true}).waitFor();
   if(new URL(page.url()).pathname!==originPath||await reconnect.isVisible()||await signOut.isVisible()){
    throw Error('reconnect returned to the wrong page or displayed competing account actions');
   }
  }else{
   await page.goto(url+'/settings/models/new');
   const signIn=page.getByRole('button',{name:'Sign in with ChatGPT',exact:false});
   await page.evaluate(()=>{window.open=()=>null});
   await signIn.click();
   const fallback=page.getByRole('link',{name:'Continue sign-in in your browser',exact:false});
   await fallback.waitFor();
   if(context.pages().length!==1)throw Error('blocked popup did not preserve native sign-in');
   await page.getByRole('button',{name:'Cancel sign-in',exact:true}).click();
   // Wait for cancellation to settle, then load the complete account controls.
   await page.getByText('Sign-in was not completed. Reconnect when you are ready.').waitFor();
   await page.goto(url+'/settings/models/new');
   const popupOpened=context.waitForEvent('page');
   await signIn.click();
   const popup=await popupOpened;
   await popup.getByText('Controlled authorization page').waitFor();
   if(!await popup.evaluate(()=>window.opener===null))throw Error('authorization popup can access the workspace');
   const pending=page.locator('#signin-status[data-auth-window-opened] [data-signin-pending]');
   await pending.getByRole('status').waitFor();
   const bounds=await pending.boundingBox();
   if(!bounds||bounds.height>55||await fallback.isVisible())throw Error('automatically opened authorization still prompts the user to open it');
   if(!await pending.getByRole('button',{name:'Cancel sign-in',exact:true}).isVisible())throw Error('pending sign-in has no cancellation action');
  }
 }else if(phase==='database'){
  await databaseInspection(page,context,browser,url,evidence);
 }else if(phase==='attention'){
  const composer=page.locator('#composer-root textarea');
  await composer.fill('Keep my conversation draft.');
  await page.getByLabel('Attach a file').setInputFiles({name:'kept.txt',mimeType:'text/plain',buffer:Buffer.from('kept attachment')});
  await composer.focus();await composer.evaluate(node=>node.setSelectionRange(5,9));
  const original=await composer.elementHandle();
  await page.waitForTimeout(1400);
  if(!await original.evaluate(node=>node===document.activeElement&&node.selectionStart===5&&node.selectionEnd===9&&node.value==='Keep my conversation draft.'))throw Error('attention update disturbed conversation input');
  const popup=context.waitForEvent('page');
  await page.locator('.attention-summary[target="_blank"]').click();
  const work=await popup;await work.waitForLoadState();
  if(await work.locator('#tasks-attention .attention-row').count()!==4)throw Error('attention projection omitted question, completed review or failed task');
  if(await work.locator('.attention-heading .status-pill').innerText()!=='4')throw Error('attention count differs from displayed items');
  if((await work.locator('#tasks-attention').innerText()).includes('Artifact reference (content not loaded)'))throw Error('fixture leaked Host attachment framing into question');
  await work.evaluate(()=>{document.activeElement?.blur();scrollTo(0,0)});
  await work.screenshot({path:evidence+'/attention-wide.png',fullPage:true});
  await work.locator('.attention-row').filter({hasText:'Which quarter should the report cover?'}).getByRole('link',{name:'Answer question',exact:true}).click();
  const answer=work.getByLabel('Your answer',{exact:true});await answer.fill('中'.repeat(6000));
  await work.getByRole('button',{name:'Send answer',exact:true}).click();
  if(!(await work.locator('#task-feedback-status').innerText()).includes('too long')||await answer.inputValue()!=='中'.repeat(6000))throw Error('long Chinese answer was not clearly rejected and retained');
  await answer.fill('Use Q3.');await answer.focus();await answer.evaluate(node=>node.setSelectionRange(4,6));
  const native=await answer.elementHandle();await work.waitForTimeout(1400);
  if(!await native.evaluate(node=>node.isConnected&&node===document.activeElement&&node.selectionStart===4&&node.selectionEnd===6&&node.value==='Use Q3.'))throw Error('live detail update replaced answer/focus');
  let posts=0;work.on('request',request=>{if(new URL(request.url()).pathname==='/task/feedback'&&request.method()==='POST')posts++});
  await work.route('**/task/feedback',async route=>{await route.fetch();await route.abort('failed')});
  await work.getByRole('button',{name:'Send answer',exact:true}).click();
  await work.locator('#task-feedback-status textarea[readonly]').waitFor();
  if(await work.locator('#task-feedback-status textarea[readonly]').inputValue()!=='Use Q3.')throw Error('uncertain POST lost answer draft');
  await work.waitForFunction(()=>!document.querySelector('.task-feedback-form'));
  if(posts!==1)throw Error('uncertain response resubmitted automatically');
  await work.screenshot({path:evidence+'/answer-uncertain-kept.png',fullPage:true});
  await work.unroute('**/task/feedback');await work.goto(url+'/tasks');
  await work.locator('.attention-row').filter({hasText:'Which source should this work use?'}).getByRole('link',{name:'Answer question',exact:true}).click();
  await work.getByLabel('Your answer',{exact:true}).fill('Unsaved wording for an outdated question.');
  const externalForm=await work.locator('.task-feedback-form').evaluate(form=>Object.fromEntries(new FormData(form)));
  const externallyAnswered=await context.request.post(url+'/task/feedback',{form:{...externalForm,source_id:'another-page',answer:'External response.'},headers:{Accept:'application/json'}});
  if(!externallyAnswered.ok())throw Error('other page response was not accepted');
  await work.waitForFunction(()=>!document.querySelector('.task-feedback-form'));
  if(await work.locator('#task-feedback-status textarea[readonly]').inputValue()!=='Unsaved wording for an outdated question.'||!(await work.locator('#task-feedback-status').innerText()).includes('has not been submitted'))throw Error('external response silently lost unsubmitted local draft');
  await work.screenshot({path:evidence+'/external-change-draft-kept.png',fullPage:true});
  await work.goto(url+'/tasks');
  await work.getByRole('link',{name:'Review result',exact:true}).click();
  const reviewURL=work.url();
  await work.setViewportSize({width:390,height:844});
  // Set the language using the real preference form; cookie names stay private.
  await work.request.post(url+'/settings/language',{form:{language:'zh-CN'}});
  await work.reload();await work.getByLabel('你的审阅意见',{exact:true}).waitFor();
  await work.evaluate(()=>document.fonts.ready);
  if(await work.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('narrow review overflows horizontally');
  await work.evaluate(()=>{document.activeElement?.blur();scrollTo(0,0)});
  await work.screenshot({path:evidence+'/review-zh-narrow.png',fullPage:true});
  const stale=await context.newPage();await stale.route('**/api/updates?**',route=>route.abort('internetdisconnected'));await stale.goto(reviewURL);
  await stale.getByLabel('你的审阅意见',{exact:true}).fill('A different stale review.');
  await work.getByLabel('你的审阅意见',{exact:true}).fill('Clarify the introduction.');
  await work.getByLabel('你的审阅意见',{exact:true}).press('Tab');
  if(!await work.getByRole('button',{name:'发送审阅意见',exact:true}).evaluate(node=>node===document.activeElement))throw Error('review keyboard order lost submit');
  await work.getByRole('button',{name:'发送审阅意见',exact:true}).press('Enter');
  await work.waitForFunction(()=>!document.querySelector('.task-feedback-form'));
  await stale.getByRole('button',{name:'发送审阅意见',exact:true}).click();
  await stale.locator('#task-feedback-status textarea[readonly]').waitFor();
  if(await stale.locator('#task-feedback-status textarea[readonly]').inputValue()!=='A different stale review.')throw Error('stale review lost draft');
  await stale.screenshot({path:evidence+'/review-stale-kept.png',fullPage:true});
  await stale.unroute('**/api/updates?**');await stale.evaluate(()=>document.dispatchEvent(new Event('visibilitychange')));
  await stale.waitForFunction(()=>!document.querySelector('.task-feedback-form'));
  if(!await original.evaluate(node=>node.isConnected&&node.value==='Keep my conversation draft.')||await page.locator('.composer input[type=file]').evaluate(node=>node.files?.[0]?.name)!=='kept.txt')throw Error('attention navigation lost original conversation draft/file');
  await writeFile(evidence+'/attention-browser.json',JSON.stringify({questionPosts:posts,keptDraft:true,keptAttachment:true,staleReviewRejected:true,keyboard:true,language:'zh-CN',width:390},null,2));
  await stale.close();await work.close();
 }else if(phase==='send-feedback'){await sendFeedback(page,context,url,evidence)}else if(phase==='page-transitions'){await pageTransitions(page,context,url,evidence)}else if(phase==='capability-defaults'){await capabilityDefaults(page,context,url,evidence)}else if(phase==='settings-account'){await settingsAccount(page,context,url,evidence)}else if(phase==='capability-setup'){await capabilitySetup(page,context,url,evidence)}else{
 if(phase==='setup'){
  await page.evaluate(()=>document.fonts.ready);
  await page.setViewportSize({width:320,height:740});
  await page.getByRole('button',{name:'Open navigation',exact:true}).click();
  for(const link of await page.getByRole('navigation',{name:'Workspace',exact:true}).getByRole('link').all()){
   const bounds=await link.boundingBox();
   if(!bounds||bounds.x<0||bounds.x+bounds.width>320)throw Error('compact navigation is outside the viewport');
  }
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('compact workspace overflows horizontally');
  await page.screenshot({path:evidence+'/navigation-compact.png',fullPage:true});
  await page.locator('.brand-row').getByRole('button',{name:'Close navigation',exact:true}).click();
  await page.setViewportSize({width:1400,height:980});
  const workspaceNav=page.getByRole('navigation',{name:'Workspace',exact:true});
  await workspaceNav.getByRole('link',{name:'Settings',exact:true}).click();
  await page.getByRole('link',{name:'Preferences',exact:true}).click();
  const preferenceRow=page.locator('form[action="/settings/language"]');
  const help=preferenceRow.getByRole('button',{name:'About interface language'});
  const tooltip=preferenceRow.locator('[role=tooltip]');
  if(await page.getByRole('heading',{name:'Preferences',exact:true}).count()!==1||await page.locator('h1:not(.sr-only)').count()!==1||await tooltip.isVisible())throw Error('Preferences loses its semantic title, duplicates it visually or shows help by default');
  await help.hover();
  if(!await tooltip.isVisible())throw Error('language help is unavailable on hover');
  await page.locator('.section-navigation').hover();
  await help.focus();
  if(!await tooltip.isVisible())throw Error('language help is unavailable from the keyboard');
  await page.keyboard.press('Escape');
  if(await tooltip.isVisible()||!await help.evaluate(node=>node===document.activeElement))throw Error('language help does not dismiss with Escape while preserving focus');
  const row=await preferenceRow.boundingBox();
  const control=await preferenceRow.locator('[data-language-trigger]').boundingBox();
  if(!row||!control||row.height>72||control.height>40||await preferenceRow.locator('.preference-save').isVisible())throw Error('language preference does not fit a compact autosave row');
  for(const language of ['zh-CN','en']){
   await page.locator('[data-language-trigger]').click();
   await page.locator(`[data-language-option="${language}"]`).click();
   await page.locator(`html[lang="${language}"]`).waitFor();
  }
  await workspaceNav.getByRole('link',{name:'Chat',exact:true}).click();
  await page.waitForURL(url+'/');
  if(await page.locator('html').getAttribute('lang')!=='en')throw Error('saved English preference did not carry into Chat');
  if(!await page.getByRole('button',{name:'Send message',exact:true}).isDisabled())throw Error('unconfigured composer can send');
  if(await page.locator('.welcome').count())throw Error('setup competes with the normal welcome');
  if(await page.locator('#conversation-attention a').count())throw Error('missing configuration became a work attention item');
  const setupCallout=page.locator('.setup-callout');
  const composer=page.locator('#composer-root .composer');
  await setupCallout.getByRole('heading',{name:"Let's get Steward ready."}).waitFor();
  const setupBounds=await setupCallout.boundingBox();
  const composerBounds=await composer.boundingBox();
  if(!setupBounds||setupBounds.height>100||!composerBounds||composerBounds.height>130)throw Error('empty Chat setup or composer takes too much vertical space');
  if((await page.locator('#composer-root').innerText()).match(/Connect a ready model|Your ongoing conversation/))throw Error('Chat repeats setup or generic approval copy below the composer');
  const setupLink=page.getByRole('link',{name:'Set up Steward',exact:false});
  if(await setupLink.count()!==1)throw Error('Chat has competing setup actions');
  await setupLink.click();
  await page.waitForURL(url+'/settings/setup');
  const settingsNav=page.getByRole('navigation',{name:'Settings sections',exact:true});
  const resourcesTab=settingsNav.getByRole('link',{name:'Resources',exact:true});
  if(await resourcesTab.getAttribute('aria-current')!=='page'||await settingsNav.getByRole('link',{name:'Models',exact:true}).count())throw Error('models are not nested under Resources');
  const models=page.getByRole('region',{name:'Models',exact:true});
  await models.getByText('No model selected',{exact:true}).waitFor();
  await models.getByRole('link',{name:'Connect a model',exact:true}).click();
  if(await resourcesTab.getAttribute('aria-current')!=='page')throw Error('model connection left the Resources section');
  await page.getByRole('button',{name:'Sign in with ChatGPT',exact:false}).waitFor();
  const connectURL=page.url();
  await selectValue(page,page.getByLabel('Connection method'),'compatible');
  await page.getByLabel('Endpoint URL').waitFor();
  if(page.url()!==connectURL||await page.getByRole('button',{name:'Sign in with ChatGPT',exact:false}).count())throw Error('provider selection did not switch fields in place');
  const plain=await browser.newContext({javaScriptEnabled:false,viewport:{width:320,height:740}});
  try{
   await plain.addCookies(await context.cookies());
   const native=await plain.newPage();
   await native.goto(connectURL);
   await native.getByLabel('Connection method',{exact:true}).selectOption('compatible');
   await native.getByRole('button',{name:'Show settings',exact:true}).click();
   await native.getByLabel('Endpoint URL').waitFor();
   if(await native.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('model connection overflows the narrow layout');
  }finally{await plain.close()}
  if(await page.locator('[name=image_input]').count())throw Error('image capability still requires a user switch');
  await page.locator('input[name="endpoint"]').fill('http://127.0.0.1:1/v1');
  await page.locator('input[name="model"]').fill('fixture-model');
  await page.getByRole('button',{name:'Add connection',exact:true}).click();
  const onResources=(notice)=>page.waitForURL(location=>location.pathname==='/settings/setup'&&location.searchParams.get('notice')===notice);
  await onResources('active');
  const firstModel=page.locator('.resource-model').filter({has:page.locator('summary').getByText('fixture-model',{exact:true})});
  await firstModel.getByText('Current model',{exact:true}).waitFor();
  if(await firstModel.getByRole('button',{name:/Test connection|Use model/}).count())throw Error('first connected model was not selected automatically');
  if(await page.locator('.model-consent,.settings-footer').count())throw Error('settings retained technical consent or notices footer');
  if(await firstModel.getByText(/^(Context \/ maximum output|Image handling)$/).count())throw Error('Resources shows read-only capability fields');
  const firstID=new URL(page.url()).searchParams.get('model');
  await firstModel.locator('summary').click();
  await page.screenshot({path:evidence+'/model-detail-wide.png',fullPage:true});
  await page.setViewportSize({width:320,height:740});
  const mobileHeader=await page.locator('.workspace-topbar').evaluate(topbar=>{
   const crumb=topbar.querySelector('span').getBoundingClientRect();
   const status=topbar.querySelector('.settings-exit').getBoundingClientRect();
   return {overlap:crumb.right>status.left,overflow:document.documentElement.scrollWidth>innerWidth};
  });
  if(mobileHeader.overlap||mobileHeader.overflow)throw Error('expanded model overlapped or overflowed the narrow layout');
  await page.screenshot({path:evidence+'/model-detail-mobile.png',fullPage:true});
  await page.setViewportSize({width:1400,height:980});
  await firstModel.locator('summary').click();
  await firstModel.getByText('Current model',{exact:true}).waitFor();
  await firstModel.getByText('Connection verified',{exact:true}).waitFor();
  if(await firstModel.getByRole('button',{name:/Test connection|Use model/}).count())throw Error('current verified model still exposes replaced actions');

  await models.getByRole('link',{name:'Connect a model',exact:true}).click();
  await selectValue(page,page.getByLabel('Connection method'),'compatible');
  await page.locator('input[name="endpoint"]').fill('http://127.0.0.1:1/v1');
  await page.locator('input[name="name"]').fill('Backup model');
  await page.locator('input[name="model"]').fill('fixture-model');
  await page.getByRole('button',{name:'Add connection',exact:true}).click();
  await onResources('added');
  const backupModel=page.locator('.resource-model').filter({has:page.getByText('Backup model',{exact:true})});
  await firstModel.getByText('Current model',{exact:true}).waitFor();
  await page.goto(url+'/settings/setup');
  if(await page.locator('.resource-model > details[open]').count())throw Error('Resources shows expanded model configurations by default');
  await firstModel.locator('summary').click();
  await backupModel.locator('summary').click();
  if(await firstModel.locator('details[open]').count())throw Error('multiple model configurations remain open');
  await backupModel.getByRole('button',{name:'Test connection',exact:true}).click();
  await onResources('tested');
  await backupModel.getByText('Connection verified',{exact:true}).waitFor();
  if(await backupModel.getByRole('button',{name:'Test connection',exact:true}).count())throw Error('successful test did not replace the test action');
  await backupModel.locator('summary').click();
  await backupModel.getByRole('button',{name:'Use model',exact:true}).click();
  await onResources('active');
  await backupModel.getByText('Current model',{exact:true}).waitFor();
  if(await firstModel.getByText('Current model',{exact:true}).count())throw Error('switching left both models current');
  if(await resourcesTab.getAttribute('aria-current')!=='page')throw Error('model selection left the Resources section');
  await page.goto(url+'/settings/models/'+encodeURIComponent(firstID));
  await page.waitForURL(location=>location.pathname==='/settings/setup'&&location.searchParams.get('model')===firstID);
  await firstModel.locator('details[open]').waitFor();
  await firstModel.getByRole('button',{name:'Use model',exact:true}).waitFor();
  await page.screenshot({path:evidence+'/model-ready.png',fullPage:true});
  // Deterministically type into usable server HTML before the local bundle is
  // released. Progressive enhancement must preserve this draft and its file.
  let release;
  const bundle=new Promise(resolve=>{release=resolve});
  let requested;
  const loading=new Promise(resolve=>{requested=resolve});
  await page.route('**/app.js',async route=>{requested();await bundle;await route.continue()});
  const selecting=page.locator('.app-sidebar').getByRole('link',{name:'Chat',exact:true}).click();
  await loading;
  await page.getByLabel('Message Ghost').fill('Plan my day: finish the release risk note before noon.');
  await page.getByLabel('Attach a file').setInputFiles({name:'web-note.txt',mimeType:'text/plain',buffer:Buffer.from('Web owner attachment fixture.\n')});
  await page.getByLabel('Message Ghost').focus();
  await page.getByLabel('Message Ghost').press('ArrowLeft');
  const earlySource=await page.locator('input[name="source_id"]').inputValue();
  const selection=await page.getByLabel('Message Ghost').evaluate(input=>[input.selectionStart,input.selectionEnd]);
  release();await selecting;
  await page.unroute('**/app.js');
  await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
  await page.evaluate(()=>document.fonts.ready);
  await page.locator('.welcome').evaluate(el=>Promise.all(el.getAnimations().map(a=>a.finished)));
  if(!await page.evaluate(()=>document.fonts.check('500 16px "Inter"')&&document.fonts.check('400 40px "Source Serif 4"')))throw Error('local brand typography unavailable');
  await page.screenshot({path:evidence+'/welcome.png',fullPage:true});
  if(await page.getByLabel('Message Ghost').inputValue()!=='Plan my day: finish the release risk note before noon.')throw new Error('frontend enhancement lost early draft');
  const preserved=await page.getByLabel('Message Ghost').evaluate(input=>document.activeElement===input&&JSON.stringify([input.selectionStart,input.selectionEnd]));
  if(preserved!==JSON.stringify(selection)||await page.locator('input[name="source_id"]').inputValue()!==earlySource)throw new Error('frontend enhancement changed draft focus, selection or source identity');
  const file=await page.getByLabel('Attach a file').evaluate(async input=>input.files[0]?{name:input.files[0].name,text:await input.files[0].text()}:null);
  if(file?.name!=='web-note.txt'||file?.text!=='Web owner attachment fixture.\n')throw new Error('frontend enhancement lost early attachment');
  const editor=page.getByLabel('Message Ghost');
  await editor.evaluate(input=>{
   window.keyboardSubmissions=0;
   input.form.addEventListener('submit',()=>window.keyboardSubmissions++);
  });
  await editor.evaluate(input=>input.setSelectionRange(input.value.length,input.value.length));await editor.press('Shift+Enter');
  if(await editor.inputValue()!=='Plan my day: finish the release risk note before noon.\n')throw Error('Shift+Enter did not insert a newline');
  await editor.evaluate(input=>{
   input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',isComposing:true,bubbles:true,cancelable:true}));
   input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',keyCode:229,bubbles:true,cancelable:true}));
   input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',repeat:true,bubbles:true,cancelable:true}));
   const send=input.form.querySelector('button[type=submit]');send.disabled=true;
   input.dispatchEvent(new KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));send.disabled=false;
  });
  if(await page.evaluate(()=>window.keyboardSubmissions)!==0)throw Error('newline, composition, repeated or disabled Enter submitted a message');
  await page.getByRole('button',{name:'Send message',exact:true}).evaluate(button=>{
   const wasDark=document.documentElement.classList.contains('dark');
   try{
    for(const dark of [false,true]){
     document.documentElement.classList.toggle('dark',dark);
     if(getComputedStyle(button).backgroundColor!=='rgb(184, 79, 47)')throw Error('send button is not the shared dark orange in both themes');
    }
   }finally{document.documentElement.classList.toggle('dark',wasDark)}
  });
  await editor.fill('Plan my day: finish the release risk note before noon.');
  const submitted=page.waitForResponse(r=>r.url()===url+'/message'&&r.request().method()==='POST');
  await editor.press('Enter');
  if((await submitted).status()!==200)throw new Error('composer submission was not accepted');
  if(await page.evaluate(()=>window.keyboardSubmissions)!==1)throw Error('Enter did not submit exactly once');
  await page.waitForFunction(()=>document.querySelectorAll('.message.assistant').length>0);
  const native=await browser.newContext({javaScriptEnabled:false});
  try{
   await native.addCookies(await context.cookies());const settings=await native.newPage();
   await settings.goto(url+'/settings/notifications');await selectValue(settings,settings.locator('select[name=target]'),'tui:');
   await settings.getByRole('button',{name:'Save notification channel',exact:true}).click();
   if(new URL(settings.url()).pathname!=='/settings/notifications'||await settings.locator('select[name=target]').inputValue()!=='tui:')throw Error('saving notifications lost its route or selected value');
   await settings.reload();if(await settings.locator('select[name=target]').inputValue()!=='tui:')throw Error('notification preference did not persist');
   await settings.screenshot({path:evidence+'/notifications.png',fullPage:true});
   await selectValue(settings,settings.locator('select[name=target]'),'auto:');await settings.getByRole('button',{name:'Save notification channel',exact:true}).click();
   await settings.goto(url+'/settings/models');await settings.getByText('Current model',{exact:true}).waitFor();
   await settings.screenshot({path:evidence+'/models.png',fullPage:true});
   await settings.getByRole('link',{name:'Connect a model',exact:true}).click();await settings.screenshot({path:evidence+'/model-sources.png',fullPage:true});
  }finally{await native.close()}
 }

 if(phase==='native-trace'){
  const facts=JSON.parse(process.env.STEWARD_NATIVE_TRACE_FACTS);
  await page.getByRole('link',{name:'Developer',exact:false}).click();
  await page.getByRole('heading',{name:'Traces',exact:true}).waitFor();
  await page.setViewportSize({width:720,height:980});
  await page.locator('.obs-filters button[role=combobox]').first().waitFor();
  for(const control of await page.locator('.obs-filters button[role=combobox]').all()){
   if(!await control.evaluate(node=>parseFloat(getComputedStyle(node).fontSize)>=16&&node.getBoundingClientRect().height>=44))throw Error('trace filters missed the compact input and touch rules');
  }
  await chooseSharedOption(page,page.getByLabel('Role',{exact:true}),'Root Worker','root');
  await selectValue(page,page.getByLabel('Role',{exact:true}),'');
  await page.setViewportSize({width:1400,height:980});
  await page.locator('[data-execution-id="'+facts.main+'"] td').first().getByRole('link',{name:'Main',exact:true}).click();
  await page.getByRole('heading',{name:'Trace tree',exact:true}).waitFor();
  for(const status of await page.locator('.obs-tree .obs-node-status').all()){
   if((await status.innerText()).replace('●','').trim()!==await status.getAttribute('data-status'))throw Error('trace tree does not expose each status as readable text');
  }
  const gen=page.locator('.obs-tree [data-observation-id="generation:'+facts.generation+'"] a').first();
  await gen.click();
  await page.locator('.obs-selected-name').filter({hasText:'Generate response'}).waitFor();
  await page.getByRole('link',{name:'Context',exact:true}).click();
  if(!await page.locator('.frozen-content').innerText().then(text=>text.length>40))throw Error('system prompt missing from unified context');
  await page.screenshot({path:evidence+'/trace-prompt.png',fullPage:true});
  await page.getByRole('link',{name:'Overview',exact:true}).click();
  await page.screenshot({path:evidence+'/trace-overview.png',fullPage:true});
  await page.locator('#observability-workbench').screenshot({path:evidence+'/trace-workbench.png'});
  const treeScroll=await page.locator('.obs-tree-pane').evaluate(node=>{node.scrollTop=60;return node.scrollTop});
  await page.getByRole('link',{name:'Requests',exact:true}).click();
  await page.getByRole('heading',{name:'Final provider request',exact:true}).waitFor();
  if(await page.locator('.obs-tree-pane').evaluate(node=>node.scrollTop)!==treeScroll)throw new Error('node selection lost tree position');
  if(!await page.locator('.wire-body').innerText().then(text=>text.includes('"messages"')&&text.includes('"attempt"')))throw new Error('final request view missing');
  await page.screenshot({path:evidence+'/trace-wire.png',fullPage:true});
  await page.goBack();await page.locator('.obs-detail-tabs a[aria-current="page"]').filter({hasText:'Overview'}).waitFor();
  await page.goto(url+'/developer/observability?run='+encodeURIComponent(facts.root));
  await page.getByText('Execution facts & data coverage',{exact:true}).click();
  await page.getByText('Task root · no parent Worker. A Main-created Task is a causal link, not a parent/child Run relationship.',{exact:true}).waitFor();
  await page.locator('.obs-tree .observation-node').filter({hasText:'Generate response'}).last().locator('a').first().click();
  await page.locator('.obs-selected-name').filter({hasText:'Generate response'}).waitFor();
  await page.screenshot({path:evidence+'/trace-worker.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('mobile trace overflowed viewport');
  await page.screenshot({path:evidence+'/trace-mobile.png',fullPage:true});
  await page.setViewportSize({width:1400,height:980});
  const plain=await browser.newContext({javaScriptEnabled:false});
  try{await plain.addCookies(await context.cookies());const fallback=await plain.newPage();await fallback.goto(page.url());await fallback.getByRole('heading',{name:'Trace tree',exact:true}).waitFor();if(!await fallback.locator('.obs-tree .observation-node').count())throw new Error('trace requires JavaScript')}finally{await plain.close()}
  await page.locator('.app-sidebar').getByRole('link',{name:'Tasks',exact:true}).click();
  await page.getByRole('link',{name:'All tasks',exact:true}).click();
  await page.locator('.task-row').first().click();
  await page.getByRole('heading',{name:'Prepare a safe independent report. Return evidence; perform no external actions.',exact:true}).waitFor();
  if(await page.getByRole('heading',{name:'Planned objective',exact:true}).count())throw Error('task detail repeated its title as the objective');
  if(!await page.locator('#task-messages').innerText().then(text=>text.includes('Main Task follow-up through Pi.')))throw Error('completed task did not show Main delivery');
  await page.screenshot({path:evidence+'/task-result.png',fullPage:true});

 }
 if(phase==='inspection'){
  const facts=JSON.parse(process.env.STEWARD_INSPECTION_FACTS);
  const main=facts.main,root=facts.runs.find(r=>r.role==='root'),child=facts.runs.find(r=>r.role==='child'&&r.parent!==root.id);
  await page.getByRole('link',{name:'Developer',exact:false}).click();
  await page.getByRole('heading',{name:'Traces',exact:true}).waitFor();
  const row=page.locator('[data-execution-id="'+main+'"]');
  await row.getByRole('link',{name:'Main',exact:true}).click();
  await page.getByText('Execution facts & data coverage',{exact:true}).click();
  await page.locator('.task-timeline .inspect-task').click();
  await page.locator('.obs-task-epochs .object-row').first().click();
  await page.locator('.obs-tree .run-node').first().waitFor();
  if(await page.locator('.run-node[data-role="root"]').count()!==1||await page.locator('.run-node[data-role="child"]').count()!==2)throw new Error('inspector lost Root/Child tree');
  if(await page.locator('.run-node[data-role="root"]').getAttribute('data-parent-run')!=='')throw new Error('Main was made the Root parent');
  for(const expected of facts.runs.filter(r=>r.role!=='main')){
   const node=page.locator('.run-node[data-run-id="'+expected.id+'"]');
   if(await node.getAttribute('data-parent-run')!==expected.parent||await node.getAttribute('data-root-run')!==expected.root)throw new Error('rendered ancestry differs from stored relationships');
  }
  if(await page.locator('.obs-task-link img').count()||!await page.locator('.obs-task-link').innerText().then(text=>text.includes('<img src=x')))throw new Error('Task title was interpreted as HTML');
  await page.locator('.run-node[data-run-id="'+child.id+'"] a.tree-heading').click();
  await page.getByText('Saved metadata & timing',{exact:true}).click();
  if(!await page.locator('.parent-run').innerText().then(text=>text.includes(child.parent)))throw new Error('selected Child lost parent navigation');
  await page.locator('.parent-run').click();
  await page.getByText('Execution facts & data coverage',{exact:true}).click();
  await page.locator('.creator-run').click();
  if(await page.locator('.run-node[data-role="root"],.run-node[data-role="child"]').count())throw new Error('Main page presented Task Roots as its Child Runs');
  await page.getByRole('link',{name:'Traces',exact:true}).first().click();
  await selectValue(page,page.getByLabel('Role',{exact:true}),'root');
  await selectValue(page,page.getByLabel('Status',{exact:true}),'completed');
  await page.getByRole('button',{name:'Apply filters',exact:true}).click();
  if(await page.locator('[data-execution-id]').count()!==1||await page.locator('[data-execution-id]:not([data-role="root"])').count())throw new Error('execution filters ignored');
  await page.getByText('Created time',{exact:true}).click();
  await page.getByLabel('Created before (UTC)').fill('2000-01-01T00:00');
  await page.getByRole('button',{name:'Apply filters',exact:true}).click();
  await page.getByRole('heading',{name:'No matching executions',exact:true}).waitFor();
  await page.getByRole('link',{name:'Reset',exact:true}).click();
  await page.locator('[data-execution-id="'+root.id+'"] td').first().getByRole('link').click();
  for(const width of [851,900]){
   await page.setViewportSize({width,height:900});
   const workbench=await page.locator('.obs-workbench').evaluate(node=>{
    const tree=node.querySelector('.obs-tree-pane').getBoundingClientRect(),detail=node.querySelector('.obs-detail-pane').getBoundingClientRect(),bounds=node.getBoundingClientRect();
    return {stacked:detail.top>=tree.bottom-1,contained:detail.right<=bounds.right+1};
   });
   if(!workbench.stacked||!workbench.contained)throw Error('intermediate trace details are clipped instead of stacked');
  }
  await page.setViewportSize({width:1400,height:980});
  await page.getByText('Execution facts & data coverage',{exact:true}).click();
  await page.getByText('Task root · no parent Worker. A Main-created Task is a causal link, not a parent/child Run relationship.',{exact:true}).waitFor();
  await page.screenshot({path:evidence+'/inspection-tree.png',fullPage:true});
  const compact=await context.newPage();
  try{
   await compact.setViewportSize({width:390,height:844});await compact.goto(page.url());
   if(await compact.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('mobile inspection overflowed viewport');
   await compact.screenshot({path:evidence+'/inspection-tree-mobile.png',fullPage:true});
  }finally{await compact.close()}
  const plain=await browser.newContext({javaScriptEnabled:false});
  try{await plain.addCookies(await context.cookies());const fallback=await plain.newPage();await fallback.goto(page.url());if(await fallback.locator('.run-node').count()!==3)throw new Error('inspector requires JavaScript')}finally{await plain.close()}
  const generationLink=page.locator('.obs-tree .observation-node').filter({hasText:'Generate response'}).last().locator('a').first();
  const generationHref=await generationLink.getAttribute('href');
  await generationLink.focus();
  const selectedGeneration=page.waitForResponse(response=>response.url()===new URL(generationHref,url).href);
  await generationLink.press('Enter');await selectedGeneration;
  await page.waitForFunction(()=>!document.querySelector('#observability-workbench')?.classList.contains('obs-loading'));
  if(!await page.evaluate(href=>document.activeElement?.getAttribute('href')===href&&!!document.activeElement?.closest('.obs-tree'),generationHref))throw Error('trace selection lost keyboard focus in its tree row');
  await page.getByRole('link',{name:'Compare',exact:true}).click();
  await page.getByRole('heading',{name:'Adjacent input diff',exact:true}).waitFor();
  if(!await page.locator('.context-diff [data-change="added"]').count())throw Error('tool continuation additions missing from diff');
  await page.getByRole('link',{name:'Context',exact:true}).click();
  await page.getByRole('heading',{name:'Full frozen context',exact:true}).waitFor();
  const system=page.locator('.context-item-link').filter({hasText:'system'}).first();await system.click();
  const meta=await page.locator('.context-reader').evaluate(node=>({...node.dataset}));
  const frozen=await (await context.request.get(url+'/api/developer/context?'+new URLSearchParams({run:meta.contextRun,step:meta.contextStep,section:meta.contextSection,item:meta.contextItem,offset:meta.contextOffset}))).json();
  if(!frozen.content||await page.locator('.frozen-content').textContent()!==frozen.content)throw new Error('system prompt differs from frozen Host content');
  await page.getByRole('link',{name:'Complete stored item (JSON)',exact:true}).click();
  if(!(await page.locator('.frozen-content').innerText()).includes('"role":"system"'))throw new Error('stored message JSON unavailable');
  await page.getByRole('link',{name:'tools',exact:true}).click();
  if(!await page.locator('.frozen-content').innerText().then(text=>text.includes('input_schema')))throw new Error('Tool schema unavailable');
  await page.getByRole('link',{name:'response',exact:true}).click();
  await page.getByRole('link',{name:'Complete stored item (JSON)',exact:true}).click();
  if(!await page.locator('.frozen-content').innerText().then(text=>text.includes('tool_calls')))throw new Error('model response unavailable');
  await page.getByRole('link',{name:'provenance',exact:true}).click();
  if(!await page.locator('.frozen-content').innerText().then(text=>text.includes('stage')))throw new Error('saved compaction provenance unavailable');
  const long=facts.long_context;
  await page.goto(url+'/developer/observability?run='+encodeURIComponent(long.run));
  await page.goto(url+'/developer/observability/'+encodeURIComponent(long.run)+'?tab=context&ctx=1&step='+long.step);
  await page.getByRole('link',{name:'Step '+long.step,exact:true}).click();
  await page.locator('.context-item-link').filter({hasText:'['+long.item+']'}).click();
  let full='';
  for(let i=0;i<20;i++){
   full+=await page.locator('.frozen-content').innerText();
   const more=page.getByRole('link',{name:'Continue full content',exact:false});
   if(!await more.count())break;
   await more.click();
  }
  if(!full.includes('LONG_CONTEXT_END')||!full.includes('完整上下文🦊'))throw new Error('long Unicode context silently clipped');
  if(await page.locator('.context-content img').count())throw new Error('context interpreted as HTML');
  await page.screenshot({path:evidence+'/context-content.png',fullPage:true});
  await page.setViewportSize({width:390,height:844});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw new Error('mobile context overflowed viewport');
  await page.screenshot({path:evidence+'/context-mobile.png',fullPage:true});
  await page.setViewportSize({width:1400,height:980});
  const nojs=await browser.newContext({javaScriptEnabled:false});
  try{await nojs.addCookies(await context.cookies());const fallback=await nojs.newPage();await fallback.goto(page.url());if(!await fallback.locator('.frozen-content').innerText().then(text=>text.includes('LONG_CONTEXT_END')))throw new Error('context requires JavaScript')}finally{await nojs.close()}
 }
 if(phase==='image'){
  await page.locator('.app-sidebar').getByRole('link',{name:'Settings',exact:true}).click();
  await page.getByRole('navigation',{name:'Settings sections'}).getByRole('link',{name:'Resources',exact:true}).click();
  await page.locator('.resource-model').filter({has:page.getByText('Current model',{exact:true})}).getByText('Connection verified',{exact:true}).waitFor();
  // The owner may open the native picker while the enhancement bundle loads.
  // Its input must survive that enhancement, including an unfinished selection.
  let release,requested;
  const bundle=new Promise(resolve=>{release=resolve});
  const loading=new Promise(resolve=>{requested=resolve});
  await page.route('**/app.js',async route=>{requested();await bundle;await route.continue()});
  const navigation=page.locator('.app-sidebar').getByRole('link',{name:'Chat',exact:true}).click();
  await loading;
  await page.getByLabel('Message Ghost',{exact:true}).fill('USER_IMAGE_INPUT: inspect this upload.');
  const [picker]=await Promise.all([page.waitForEvent('filechooser'),page.getByLabel('Attach a file').click()]);
  release();await navigation;await page.unroute('**/app.js');
  await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
  if(!await picker.element().evaluate(input=>input===document.querySelector('.composer input[type=file]')))throw new Error('enhancement replaced the open file picker');
  await picker.setFiles({name:'user-picture.png',mimeType:'image/png',buffer:Buffer.from(process.env.STEWARD_TEST_PICTURE,'base64')});
  await page.getByRole('button',{name:'Send message',exact:true}).click();
  const expected=process.env.STEWARD_TEST_VISION==='yes'?'USER_IMAGE_PIXELS_RECEIVED':'USER_IMAGE_REFERENCE_ONLY';
  await page.locator('.message.assistant').filter({hasText:expected}).waitFor();
 }
 if(phase==='streaming'){
  page.setDefaultTimeout(8000);
  const editor=page.getByLabel('Message Ghost',{exact:true});
  const send=page.getByRole('button',{name:'Send message',exact:true});
  const draft=page.locator('#conversation-draft');
  const committed=page.locator('#conversation-messages .message.assistant');
  const first='First chunk <img src=x onerror=unsafe()>',second='Second chunk 中文.';
  const release=name=>writeFile(process.env.STEWARD_STREAM_GATES+'/'+name,'');
  const source=page.locator('.composer input[name=source_id]');
  const before=await source.inputValue();
  const editorNode=await editor.elementHandle();
  let navigations=0;page.on('framenavigated',frame=>{if(frame===page.mainFrame())navigations++});
  await editor.fill('STREAM_SUCCESS');await send.click();
  await draft.filter({hasText:first}).waitFor();
  if(await committed.count()||await draft.locator('img').count()||before===await source.inputValue())throw Error('draft was committed, interpreted as HTML, or did not rotate the accepted request identity');
  const durable=await (await context.request.get(url+'/api/conversation')).json();
  if(durable.some(message=>message.role==='assistant'))throw Error('preview was persisted before the provider completed');
  await editor.fill('Keep typing while Ghost writes.');
  await editor.focus();await editor.evaluate(node=>node.setSelectionRange(5,11));
  await page.getByLabel('Attach a file').setInputFiles({name:'stream-kept.txt',mimeType:'text/plain',buffer:Buffer.from('keep')});
  const late=await context.newPage();
  try{
   await late.goto(url);
   await late.locator('#conversation-draft').filter({hasText:first}).waitFor({timeout:8000});
  }finally{await late.close();await page.bringToFront()}
  await page.evaluate(()=>window.dropReplyStream());
  await draft.waitFor({state:'hidden'});
  await release('second');
  const draftText=draft.locator('[data-draft-text]');
  await draftText.filter({hasText:second}).waitFor();
  if((await draftText.innerText()).trimEnd()!==first+'\n'+second)throw Error('reconnect lost, repeated or exposed streamed text');
  await page.screenshot({path:evidence+'/streaming-draft.png'});
  let resumeProjection;
  const projectionGate=new Promise(resolve=>{resumeProjection=resolve});
  await page.route('**/api/updates?**',async route=>{await projectionGate;await route.continue().catch(()=>{})});
  try{
   await release('finish');
   await draftText.filter({hasText:'Final tail.'}).waitFor();
   if((await draftText.innerText()).trimEnd()!==first+'\n'+second+'\nFinal tail.'||await committed.count())throw Error('commit lost or repeated text before the final projection');
  }finally{resumeProjection()}
  await page.unroute('**/api/updates?**');
  await committed.filter({hasText:'Final tail.'}).waitFor();
  await draft.waitFor({state:'hidden'});
  if(await committed.count()!==1||!await editorNode.evaluate(node=>node===document.activeElement&&node.value==='Keep typing while Ghost writes.'&&node.selectionStart===5&&node.selectionEnd===11))throw Error('commit duplicated reply or disturbed the draft editor');
  if(await page.getByLabel('Attach a file').evaluate(node=>node.files[0]?.name)!=='stream-kept.txt')throw Error('streaming lost the attachment');
  await page.getByLabel('Attach a file').setInputFiles([]);
  await editor.fill('STREAM_FAILURE');await send.click();
  await draft.filter({hasText:'Interrupted draft only.'}).waitFor();
  await release('fail');await draft.waitFor({state:'hidden'});
  await committed.filter({hasText:'The request could not be completed. Please retry explicitly.'}).waitFor();
  const count=await page.locator('#conversation-messages [data-reply-key]').count();
  if(count!==1||await committed.filter({hasText:'Interrupted draft only.'}).count()||navigations!==0)throw Error('failed stream changed committed replies or navigated: '+JSON.stringify({count,navigations}));
  await writeFile(evidence+'/stream-contract.json',JSON.stringify({previewBeforeCompletion:true,lateSubscriber:true,reconnected:true,commitTail:true,committedOnce:true,failedDiscarded:true,navigations}));
 }
 if(phase==='live'){
  page.setDefaultTimeout(10000);
  let navigations=0;page.on('framenavigated',frame=>{if(frame===page.mainFrame())navigations++});
  const send=async(source,text)=>{
   const response=await context.request.post(url+'/api/message',{data:{source_id:source,content:[{type:'text',text}]}});
   if(!response.ok())throw new Error('live input not accepted: '+response.status());
  };
  const plan=(name,args)=>'Capabilities: '+JSON.stringify([{name,arguments:args}]);
  const echo=async(key,text)=>send(key,plan('mcp__fixture__echo',{text}));
  await echo('live-initial','Opening note\n'+'Context line\n'.repeat(55)+'INITIAL_DONE');
  await page.locator('.message.assistant').filter({hasText:'INITIAL_DONE'}).waitFor();
  const editor=page.getByLabel('Message Ghost',{exact:true});
  await editor.fill('Keep this unsent draft exactly.');
  await page.getByLabel('Attach a file').setInputFiles({name:'pending.txt',mimeType:'text/plain',buffer:Buffer.from('Unsent attachment bytes.\n')});
  await editor.focus();await editor.press('ArrowLeft');
  let source=await page.locator('input[name="source_id"]').inputValue();
  const selection=await editor.evaluate(input=>[input.selectionStart,input.selectionEnd]);
  await page.evaluate(()=>{window.liveEditor=document.querySelector('textarea');window.liveFile=document.querySelector('input[type=file]');scrollTo(0,180)});
  const anchor=await page.locator('[data-live-key^="message:"]').first().evaluate(node=>({key:node.dataset.liveKey,top:node.getBoundingClientRect().top}));
  const checkDraft=async(focused=true)=>{
   if(await editor.inputValue()!=='Keep this unsent draft exactly.'||await page.locator('input[name="source_id"]').inputValue()!==source)throw new Error('live update lost draft or request identity');
   const kept=await editor.evaluate(input=>({node:input===window.liveEditor,file:document.querySelector('input[type=file]')===window.liveFile,focused:document.activeElement===input,selection:[input.selectionStart,input.selectionEnd]}));
   if(!kept.node||!kept.file||focused&&!kept.focused||JSON.stringify(kept.selection)!==JSON.stringify(selection))throw new Error('live update replaced input nodes, focus or selection');
   const file=await page.getByLabel('Attach a file').evaluate(async input=>({name:input.files[0]?.name,text:await input.files[0]?.text()}));
   if(file.name!=='pending.txt'||file.text!=='Unsent attachment bytes.\n')throw new Error('live update lost attachment');
  };
  // Real read-only panels preserve the existing composer and its file. Switching
  // panes must close the previous viewer without replacing the conversation.
  await page.getByRole('button',{name:'Worker progress',exact:true}).click();
  await page.locator('#worker-panel[open]').waitFor();await checkDraft(false);
  await page.getByRole('button',{name:'Open Computer',exact:true}).click();
  await page.locator('#computer-panel[open]').waitFor();
  if(await page.locator('#worker-panel[open]').count())throw Error('two workspace panes opened');
  await page.getByRole('button',{name:'Expand computer view',exact:true}).click();
  if(!await page.locator('#computer-panel').evaluate(panel=>panel.matches(':modal')))throw Error('expanded Computer did not protect focus');
  await checkDraft(false);
  await page.getByRole('button',{name:'Reduce computer view',exact:true}).click();
  if(await page.locator('#computer-panel').evaluate(panel=>panel.matches(':modal')))throw Error('reduced desktop pane kept conversation inert');
  await checkDraft(false);
  await page.getByRole('button',{name:'Close computer view',exact:true}).click();
  if(await page.locator('.workspace-panel[open]').count())throw Error('workspace pane stayed open');
  if(!await page.getByRole('button',{name:'Open Computer',exact:true}).evaluate(button=>document.activeElement===button))throw Error('pane did not restore trigger focus');
  await editor.focus();await checkDraft();
  const afterPanels=await page.locator('[data-live-key="'+anchor.key+'"]').evaluate(node=>({top:node.getBoundingClientRect().top,scroll:scrollY}));
  if(Math.abs(afterPanels.top-anchor.top)>2)throw Error('pane roundtrip moved anchor: '+JSON.stringify({before:anchor,after:afterPanels}));
  let attemptedSends=0;
  let releaseSend;
  const heldSend=new Promise(resolve=>{releaseSend=resolve});
  await page.route('**/message',async route=>{attemptedSends++;await heldSend;await route.abort('failed')});
  const sending=page.waitForRequest(r=>r.url()===url+'/message'&&r.method()==='POST');
  await editor.press('Enter');await sending;
  await editor.press('Enter');await editor.press('Enter');
  if(attemptedSends!==1||await editor.inputValue()!==''||!(await page.locator('#conversation-outgoing').innerText()).includes('Keep this unsent draft exactly.'))throw Error('pending Enter repeated submission or changed the draft');
  releaseSend();
  await page.locator('#conversation-outgoing [data-send-state=uncertain]').waitFor();
  await editor.fill('Keep this unsent draft exactly.');
  await page.getByLabel('Attach a file').setInputFiles({name:'pending.txt',mimeType:'text/plain',buffer:Buffer.from('Unsent attachment bytes.\n')});
  source=await page.locator('input[name="source_id"]').inputValue();
  await editor.evaluate((input,range)=>input.setSelectionRange(...range),selection);
  await page.locator('[data-live-key="'+anchor.key+'"]').evaluate((node,top)=>scrollBy(0,node.getBoundingClientRect().top-top),anchor.top);
  await checkDraft(false);if(attemptedSends!==1)throw Error('failed send was automatically repeated');
  await page.unroute('**/message');await editor.focus();await editor.evaluate((input,range)=>input.setSelectionRange(...range),selection);
  await echo('live-reply','LIVE_REPLY_WHILE_TYPING');
  await page.locator('.message.assistant').filter({hasText:'LIVE_REPLY_WHILE_TYPING'}).waitFor();
  await checkDraft();
  const top=await page.locator('[data-live-key="'+anchor.key+'"]').evaluate(node=>node.getBoundingClientRect().top);
  if(Math.abs(top-anchor.top)>2)throw new Error('background update moved reading position: '+JSON.stringify({before:anchor.top,after:top}));
  await page.getByRole('button',{name:'Latest messages'}).waitFor();
  await page.waitForResponse(response=>response.url().includes('/api/updates?')&&response.status()===304);
  await checkDraft();
  // A real network failure retains the DOM and draft, then reads current state.
  let interrupted=false;
  await page.route('**/api/updates?**',async route=>{if(!interrupted){interrupted=true;await route.abort()}else await route.continue()});
  await page.getByRole('status').filter({hasText:'Connection lost'}).waitFor();
  await checkDraft();
  await echo('live-reconnect','LIVE_REPLY_AFTER_RECONNECT');
  await page.locator('.message.assistant').filter({hasText:'LIVE_REPLY_AFTER_RECONNECT'}).waitFor();
  await page.unroute('**/api/updates?**');
  await checkDraft();
  // The approval and progress come from a real Task/Worker and frozen native
  // write invocation. Rejecting it must not navigate away from this draft.
  const task='Compare release options, identify the tradeoffs, and prepare a concise recommendation for the owner.\nCapabilities: '+JSON.stringify([
   {name:'builtin__write',arguments:{path:'unapproved.txt',content:'must not be written'},progress:'Compared the available options; preparing the draft.'},
   {name:'mcp__fixture__echo',arguments:{text:'Ready to return the reviewed options.'},progress:'Draft save declined; preparing the reviewed options.'},
  ]);
  await send('live-task',plan('builtin__task_manage',{operation:'create',task,completionPolicy:'ongoing'}));
  const approval=page.locator('.approval form').first();await approval.waitFor();
  await page.locator('#workspace-status').filter({hasText:'Working'}).waitFor();
  await checkDraft();
  const pending=page.locator('[data-pending-jump]');await pending.waitFor();
  const stack=await page.locator('.chat-topbar .topbar-actions').evaluate(node=>Array.from(node.querySelectorAll('[data-panel-open],[data-pending-jump]')).map(item=>({top:item.getBoundingClientRect().top,bottom:item.getBoundingClientRect().bottom,left:item.getBoundingClientRect().left})));
  if(stack.length!==3||stack[1].top<stack[0].bottom||stack[2].top<stack[1].bottom||stack.some(item=>Math.abs(item.left-stack[0].left)>1))throw Error('approval entry is not below Computer and Worker');
  await pending.click();await checkDraft(false);
  if(!await page.locator('.approval').first().evaluate(node=>document.activeElement===node))throw Error('pending entry did not focus its approval');
  await editor.focus();
  const frozen=await approval.locator('input[name="hash"]').inputValue();
  const progress=await context.newPage();
  try{
   await progress.goto(url+'/tasks?view=all');
   const tasks=await (await context.request.get(url+'/api/progress')).json();
   const matches=tasks.filter(item=>item.title.startsWith('Compare release options,'));
   if(matches.length!==1)throw Error('expected the exact ongoing approval Task');
   const ref=matches[0].taskRef;
   const ongoing=progress.locator('[data-live-key="task:'+ref+'"]');
   await ongoing.filter({hasText:'Waiting for your approval'}).waitFor();
   const detail=await context.newPage();
   try{
    await detail.goto(url+'/tasks/'+encodeURIComponent(ref));
    await detail.getByRole('heading',{name:'Planned objective',exact:true}).waitFor();
    if(await detail.locator('.approval form input[name=hash]').inputValue()!==frozen)throw Error('task detail displayed another approval');
    const communication=await context.newPage();try{
     await communication.goto(await detail.getByRole('link',{name:'Continue this task',exact:false}).getAttribute('href').then(path=>url+path));
     if(await communication.locator('input[name=task_ref]').inputValue()!==ref)throw Error('continuation lost task context');
     await communication.getByLabel('Message Ghost',{exact:true}).fill('Keep this task draft.');
     await communication.getByRole('button',{name:'Clear task context',exact:true}).click();
     if(await communication.getByLabel('Message Ghost',{exact:true}).inputValue()!=='Keep this task draft.'||await communication.locator('input[name=task_ref]').count())throw Error('clearing task context discarded its draft');
    }finally{await communication.close()}
    await detail.screenshot({path:evidence+'/task-detail.png',fullPage:true});
    for(const width of [820,900]){
     await detail.setViewportSize({width,height:900});
     const layout=await detail.locator('.task-detail-grid').evaluate(node=>{
      const primary=node.children[0].getBoundingClientRect(),side=node.children[1].getBoundingClientRect();
      return {stacked:side.top>=primary.bottom-1,values:[...node.querySelectorAll('.facts dd')].every(value=>value.getBoundingClientRect().width>=100)};
     });
     if(!layout.stacked||!layout.values)throw Error('intermediate task content or progress values are compressed');
    }
    await detail.setViewportSize({width:390,height:844});if(await detail.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('mobile task detail overflow');
    await detail.screenshot({path:evidence+'/task-detail-mobile.png',fullPage:true});
   }finally{await detail.close()}
   await page.bringToFront();
   const decision=page.waitForResponse(response=>response.url()===url+'/approval'&&response.request().method()==='POST');
   await approval.getByRole('button',{name:'Decline',exact:true}).click();
   const acknowledged=await decision;
   if(acknowledged.status()!==200||!(await acknowledged.json()).accepted)throw new Error('live approval was not acknowledged');
   if(!acknowledged.request().postData().includes(frozen))throw new Error('approval changed the frozen action hash');
   await page.waitForFunction(()=>!document.querySelector('.approval form')&&!document.querySelector('[data-pending-jump]'));
   await checkDraft(false);
   await progress.bringToFront();
   await ongoing.filter({hasText:'This round is complete; the task remains open'}).waitFor();
   // The planned objective now displays the original request, including this fixture's
   // capability script. Only stage/progress text must exclude raw tool arguments.
   const progressText=(await ongoing.locator('p:not(.task-objective-preview):not(.task-list-rule)').allTextContents()).join('\n');
   if(progressText.includes('must not be written'))throw new Error('raw tool arguments became stage progress');
   await send('live-cancel',plan('builtin__task_manage',{operation:'cancel',taskRef:ref}));
   await progress.bringToFront();
   await progress.locator('[data-live-key="task:'+ref+'"] .state').filter({hasText:'cancelled'}).waitFor();
   if(await progress.locator('form[action="/task/manage"]').count())throw Error('task browsing gained a second control plane');
  }finally{await progress.close();await page.bringToFront()}
  // Another authenticated browser rotates the session. The old page must keep
  // its draft and offer explicit reconnect, without redirecting to login.
  const other=await browser.newContext();
  try{await other.request.post(url+'/session',{form:{token:process.env.STEWARD_WEB_TEST_TOKEN}})}finally{await other.close()}
  await page.getByRole('status').filter({hasText:'Updates paused'}).waitFor();
  await checkDraft(false);
  await context.request.post(url+'/session',{form:{token:process.env.STEWARD_WEB_TEST_TOKEN}});
  const resumed=page.waitForResponse(response=>response.url().includes('/api/updates?')&&[200,304].includes(response.status()));
  await page.getByRole('button',{name:'Retry updates',exact:true}).click();await resumed;
  await checkDraft(false);
  if(navigations!==0)throw new Error('live updates navigated the document');
  await writeFile(evidence+'/live-contract.json',JSON.stringify({draft:true,attachment:true,focus:true,selection:true,scroll:true,conditional:true,reconnect:true,approval:true,progress:true,semanticStages:true,session:true,navigations},null,2));
 }
 if(phase==='schedule'){
  let schedulePosts=0;page.on('request',request=>{if(request.method()==='POST'&&new URL(request.url()).pathname==='/schedule/configure')schedulePosts++});
  const initialTasks=await (await context.request.get(url+'/api/progress')).json();
  if(!Array.isArray(initialTasks))throw Error('Task progress did not return a list');
  const taskCount=initialTasks.length;
  await page.getByRole('link',{name:'Tasks',exact:true}).click();
  await page.getByRole('link',{name:'Create schedule',exact:true}).click();
  const editor=page.locator('form.schedule-editor');
  await page.getByLabel('Instructions',{exact:true}).fill('Review recurring subscriptions and VPS renewals; do not pay or renew automatically.');
  await page.setViewportSize({width:720,height:980});
  const frequency=page.getByLabel('Frequency',{exact:true});
  await chooseSharedOption(page,frequency,'Weekly','weekly');
  const weekday=page.locator('select[name=weekday]'),weekdayTrigger=(await selectParts(weekday)).trigger;await weekdayTrigger.waitFor({state:'visible'});
  await frequency.click();await weekdayTrigger.focus();await weekdayTrigger.press('Enter');
  if(await page.getByRole('listbox').count()!==1)throw Error('Task selectors left competing popups open');
  await page.keyboard.press('Escape');
  await chooseSharedOption(page,frequency,'Weekdays','weekdays');
  if(await weekdayTrigger.isVisible()||schedulePosts!==0)throw Error('choosing a frequency submitted or failed to hide dependent fields');
  await page.setViewportSize({width:1400,height:980});
  await page.getByLabel('Local time',{exact:true}).fill('07:40');
  await page.getByLabel('Time zone',{exact:true}).fill('Asia/Shanghai');
  const submitted=await editor.evaluate(form=>Object.fromEntries(new FormData(form)));
  await page.getByRole('button',{name:'Save schedule',exact:true}).click();
  await page.getByRole('heading',{name:'Edit schedule',exact:true}).waitFor();
  if(await (await selectParts(page.getByLabel('Frequency',{exact:true}))).native.inputValue()!=='weekdays'||await page.getByLabel('Local time',{exact:true}).inputValue()!=='07:40'||await page.getByLabel('Time zone',{exact:true}).inputValue()!=='Asia/Shanghai')throw Error('schedule did not persist friendly fields');
  await page.screenshot({path:evidence+'/schedule-editor-wide.png',fullPage:true});
  const editURL=page.url();
  const replay=await context.request.post(url+'/schedule/configure',{form:submitted,maxRedirects:0});
  if(replay.status()!==303)throw Error('identical schedule retry did not return its durable receipt');
  const drift=await context.request.post(url+'/schedule/configure',{form:{...submitted,instruction:'Changed payload under the same request identity'},maxRedirects:0});
  if(drift.status()!==409)throw Error('schedule request-key drift was accepted');
  const enabled=page.getByRole('switch',{name:'Schedule enabled',exact:true});
  const beforeToggle=schedulePosts;await enabled.focus();await page.keyboard.press('Space');
  if(await enabled.isChecked()||schedulePosts!==beforeToggle)throw Error('Task switch changed its manual-save semantics');
  if(await enabled.evaluate(node=>node.getBoundingClientRect().height)<44)throw Error('Task switch lost its touch target');
  await page.getByRole('button',{name:'Save schedule',exact:true}).click();
  await page.getByRole('heading',{name:'Edit schedule',exact:true}).waitFor();
  await page.reload();
  if(await page.getByLabel('Schedule enabled',{exact:true}).isChecked())throw Error('schedule disable was not persisted');
  await page.getByLabel('Schedule enabled',{exact:true}).check();
  await page.getByRole('button',{name:'Save schedule',exact:true}).click();
  await page.getByRole('heading',{name:'Edit schedule',exact:true}).waitFor();
  const stale=await context.newPage();
  try{
   await stale.goto(editURL);
   await page.getByLabel('Instructions',{exact:true}).fill('Updated future instruction from the first editor.');
   await page.getByRole('button',{name:'Save schedule',exact:true}).click();
   await page.getByRole('heading',{name:'Edit schedule',exact:true}).waitFor();
   await stale.getByLabel('Instructions',{exact:true}).fill('Preserve this stale draft.');
   await stale.getByRole('button',{name:'Save schedule',exact:true}).click();
   await stale.getByRole('alert').filter({hasText:'This schedule changed'}).waitFor();
   if(await stale.getByLabel('Instructions',{exact:true}).inputValue()!=='Preserve this stale draft.')throw Error('schedule conflict lost the owner draft');
  }finally{await stale.close()}
  if((await (await context.request.get(url+'/api/progress')).json()).length!==taskCount)throw Error('saving a future schedule dispatched a Task');
  await page.goto(url+'/tasks');
  await page.getByRole('heading',{name:'Scheduled work',exact:true}).waitFor();
  await page.getByRole('link',{name:'Create schedule',exact:true}).click();
  await page.getByLabel('Instructions',{exact:true}).fill('One-time future review.');
  await selectValue(page,page.getByLabel('Frequency',{exact:true}),'once');
  await page.getByLabel('Date',{exact:true}).fill('2027-03-14');
  await page.getByLabel('Local time',{exact:true}).fill('02:30');
  await page.getByLabel('Time zone',{exact:true}).fill('America/New_York');
  await page.getByRole('button',{name:'Save schedule',exact:true}).click();
  await page.getByRole('alert').filter({hasText:'unambiguous'}).waitFor();
  if(await page.getByLabel('Local time',{exact:true}).inputValue()!=='02:30')throw Error('invalid local schedule time lost draft');
  await page.setViewportSize({width:320,height:740});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('schedule editor overflowed compact viewport');
  await page.screenshot({path:evidence+'/schedule-mobile.png',fullPage:true});
  await page.evaluate(()=>document.documentElement.classList.add('dark'));
  await page.screenshot({path:evidence+'/schedule-mobile-dark.png',fullPage:true});
  if(await page.getByRole('button',{name:'Save schedule',exact:true}).isDisabled())throw Error('invalid schedule response disabled explicit retry');
  await page.getByLabel('Local time',{exact:true}).fill('03:30');
  await page.getByRole('button',{name:'Save schedule',exact:true}).click();
  await page.getByRole('heading',{name:'Edit schedule',exact:true}).waitFor();
  if(await page.getByLabel('Local time',{exact:true}).inputValue()!=='03:30')throw Error('corrected schedule could not be saved');
  const touch=await browser.newContext({hasTouch:true,viewport:{width:320,height:740}});
  try{
   await touch.addCookies(await context.cookies());const p=await touch.newPage();await p.goto(editURL);
   const toggle=p.getByRole('switch',{name:'Schedule enabled',exact:true}),was=await toggle.isChecked();
   await p.getByLabel('Frequency',{exact:true}).tap();await p.getByRole('listbox').getByRole('option',{name:'Once',exact:true}).tap();
   await p.getByLabel('Date',{exact:true}).waitFor({state:'visible'});
   await toggle.tap();if(await toggle.isChecked()===was)throw Error('touch did not toggle the shared switch');
   await p.reload();if(await toggle.isChecked()!==was||await (await selectParts(p.getByLabel('Frequency',{exact:true}))).native.inputValue()!=='weekdays')throw Error('touch bypassed explicit Task saving');
  }finally{await touch.close()}
  const native=await browser.newContext({javaScriptEnabled:false,viewport:{width:320,height:740}});
  try{
   await native.addCookies(await context.cookies());const p=await native.newPage();await p.goto(editURL);
   await selectValue(p,p.getByLabel('Frequency',{exact:true}),'weekly');await p.locator('select[name=weekday]').waitFor({state:'visible'});
   await p.getByRole('switch',{name:'Schedule enabled',exact:true}).uncheck();
   await Promise.all([p.waitForNavigation({waitUntil:'load'}),p.getByRole('button',{name:'Save schedule',exact:true}).click()]);
   await p.reload();
   if(await (await selectParts(p.getByLabel('Frequency',{exact:true}))).native.inputValue()!=='weekly'||await p.getByRole('switch',{name:'Schedule enabled',exact:true}).isChecked())throw Error('native Task controls did not persist');
   await p.screenshot({path:evidence+'/schedule-native.png',fullPage:true});
  }finally{await native.close()}
  if((await (await context.request.get(url+'/api/progress')).json()).length!==taskCount)throw Error('control checks dispatched a Task');
  await writeFile(evidence+'/schedule-browser.json',JSON.stringify({keyboard:true,search:true,emptyResults:true,dependentFields:true,singlePopup:true,switchManualSave:true,persistence:true,staleDraft:true,invalidRetry:true,touch:true,nativeFallback:true,noPrematureDispatch:true,widths:[1400,720,320]},null,2));
  await page.setViewportSize({width:1400,height:980});
  await page.goto(url);
 }

 if(phase==='ui-interactions'||phase==='ui-interactions-restart')await uiInteractions(page,context,browser,url,evidence,phase==='ui-interactions-restart');
 if(phase==='intelligent-ui'){
  const activate=async button=>{await button.hover();await button.click()};
  await page.getByRole('columnheader',{name:'Option',exact:true}).waitFor();
  if(await page.locator('.message .intelligent-widget').count()!==2)throw Error('custom UI widgets missing');
  const widget=page.frameLocator('iframe[title="Calculator"]');
  await widget.locator('#security').getByText('blocked:2',{exact:true}).waitFor();
  const direct=await context.newPage();
  try{const response=await direct.goto(url+await page.locator('iframe[title="Calculator"]').getAttribute('src'));if(!response.headers()['content-type']?.startsWith('text/plain')||await direct.locator('script').count())throw Error('Standalone Widget URL executed generated code')}finally{await direct.close();await page.bringToFront()}
  await widget.locator('#n').fill('25');await widget.getByRole('button',{name:'Compute',exact:true}).click();
  await widget.locator('#answer').getByText('50',{exact:true}).waitFor();
  await widget.locator('#amplitude').fill('35');if(await widget.locator('#wave').getAttribute('r')!=='35')throw Error('upstream sandbox controls did not update SVG');
  const activity=JSON.parse(await widget.locator('#open-generative-ui-activity').textContent());if(activity.type!=='open-generative-ui'||activity.content.htmlComplete!==true||activity.content.jsExpressions[0]!=='initialize()')throw Error('upstream completed activity shape changed');
  await page.evaluate(()=>document.documentElement.classList.add('dark'));
  await widget.locator('html[data-widget-theme="dark"]').waitFor();
  const calculator=page.locator('.intelligent-widget').filter({has:page.locator('iframe[title="Calculator"]')});
  await activate(calculator.getByRole('button',{name:'Options',exact:true}));await page.getByRole('menuitem',{name:'Expand',exact:true}).click();
  await page.locator('.widget-expanded').waitFor();await page.keyboard.press('Escape');await page.locator('.widget-expanded').waitFor({state:'detached'});
  await page.getByText('This interactive view could not be displayed.').waitFor();
  if(await page.locator('.intelligent-widget summary').count())throw Error('interactive answers retained a Text version control');
  if(await calculator.locator('.widget-fallback').count())throw Error('successful visual duplicated its fallback');
  if(!await widget.locator('body').evaluate(()=>document.fonts.check('14px Inter')))throw Error('sandbox did not load the shared UI font');
  const sandboxGap=await widget.locator('.actions button').evaluateAll(nodes=>{const [a,b]=nodes.map(node=>node.getBoundingClientRect());return b.left-a.right});if(sandboxGap<7)throw Error('sandbox actions touched or overlapped');
  const before=await page.locator('.message.user').count();
  await activate(widget.getByRole('button',{name:'Discuss this result'}));
  await calculator.getByRole('textbox',{name:'Message to Ghost'}).waitFor();
  if(await page.locator('.message.user').count()!==before)throw Error('widget auto-sent a message');
  if(await calculator.getByRole('textbox',{name:'Message to Ghost'}).inputValue()!=='Discuss the computed result: 50')throw Error('selected state missing from follow-up');
  await calculator.getByRole('textbox',{name:'Message to Ghost'}).fill('Draft written by user.');
  await activate(widget.getByRole('button',{name:'Discuss this result'}));
  if(await calculator.getByRole('textbox',{name:'Message to Ghost'}).inputValue()!=='Draft written by user.')throw Error('Widget replaced the Host-owned draft');
  await calculator.getByRole('textbox',{name:'Message to Ghost'}).fill('Discuss the computed result: 50');
  const assertActionGap=async()=>{const boxes=await calculator.locator('.widget-followup-actions button').evaluateAll(nodes=>nodes.map(node=>node.getBoundingClientRect().toJSON()));if(boxes.length!==2)throw Error('missing action group');const [a,b]=boxes;if(Math.abs(a.top-b.top)<2?b.left-a.right<7:b.top-a.bottom<7)throw Error('widget actions touched or overlapped')};
  await assertActionGap();await page.setViewportSize({width:390,height:844});await assertActionGap();if(await widget.getByRole('button',{name:'Compute',exact:true}).evaluate(node=>node.getBoundingClientRect().height)<44)throw Error('mobile sandbox action is too small');await page.setViewportSize({width:1400,height:980});
  let lost=false;const sources=[];
  await page.route('**/api/message',async route=>{
   const input=route.request().postDataJSON();
   if(input.source_id?.startsWith('ui-widget:')){sources.push(input.source_id);if(!lost){lost=true;await route.fetch();await route.abort('failed');return}}
   await route.continue();
  });
  await calculator.getByRole('button',{name:'Send to Ghost',exact:true}).scrollIntoViewIfNeeded();
  await page.screenshot({path:evidence+'/before-widget-send.png'});
  await activate(calculator.getByRole('button',{name:'Send to Ghost',exact:true}));
  await calculator.getByRole('alert').waitFor();
  await activate(widget.getByRole('button',{name:'Discuss this result'}));
  await activate(calculator.getByRole('button',{name:'Send to Ghost',exact:true}));
  await calculator.getByRole('textbox',{name:'Message to Ghost'}).waitFor({state:'detached'});
  if(sources.length!==2||sources[0]!==sources[1])throw Error('Widget retry changed its source identity');
  await page.unroute('**/api/message');
  await page.getByText('Discuss the computed result: 50',{exact:true}).waitFor();
  await page.reload();await page.frameLocator('iframe[title="Calculator"]').locator('#answer').getByText('50',{exact:true}).waitFor();if(await page.frameLocator('iframe[title="Calculator"]').locator('#n').inputValue()!=='25')throw Error('calculator control state was not restored');
  await page.setViewportSize({width:390,height:844});
  await calculator.evaluate(node=>node.scrollIntoView({block:'center'}));
  await page.screenshot({path:evidence+'/intelligent-ui-widget-mobile.png'});
  if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+2))throw Error('widget overflowed mobile viewport');
  await page.setViewportSize({width:1400,height:980});
  await calculator.evaluate(node=>node.scrollIntoView({block:'center'}));
  await page.screenshot({path:evidence+'/intelligent-ui-widget-desktop.png'});
  const plain=await browser.newContext({javaScriptEnabled:false});try{await plain.addCookies(await context.cookies());const text=await plain.newPage();await text.goto(url);if(!await text.getByText('The calculator doubles its input. No network or Host access.',{exact:true}).count())throw Error('no-script text alternative missing')}finally{await plain.close()}
 }
 if(phase==='rich-replies'){
  const grouped=page.locator('.message.assistant.message-split').first(),bubbles=grouped.locator(':scope > .message-bubble');
  if(await bubbles.count()!==2||await bubbles.nth(0).locator('.answer-table').count()!==1||await bubbles.nth(1).locator('.answer-table').count())throw Error('Intelligent UI must occupy the first bubble before the final reply bubble');
  const groupBox=await grouped.boundingBox(),uiBox=await bubbles.first().boundingBox();
  if(uiBox.width<groupBox.width*.85)throw Error('Intelligent UI collapsed to its minimum content width');
  await page.locator('[data-renderer="native"] .answer-table').waitFor();
  if(await page.locator('.answer-table tbody tr').count()!==2||await page.locator('.answer-table img,.answer-table script').count())throw Error('generated table lost data or interpreted markup');
  if(!(await page.locator('.answer-table').innerText()).includes('<img src=x onerror='))throw Error('generated table changed literal cells');
  for(const name of ['Table','barChart','pieChart','plan_visualization','scheduleTime'])await page.locator('[data-openintelligentui="'+name+'"]').first().waitFor();
  await page.locator('[data-openintelligentui=barChart] .recharts-bar-rectangle').first().waitFor();await page.locator('[data-openintelligentui=pieChart] .recharts-sector').first().waitFor();
  if(await page.locator('[data-openintelligentui=barChart] .recharts-bar-rectangle').count()!==2||!await page.locator('[data-openintelligentui=pieChart] .recharts-sector').count())throw Error('upstream native chart data did not render');
  const titleStyle=await page.locator('.native-chart .native-answer-title').first().evaluate(node=>({size:parseFloat(getComputedStyle(node).fontSize),font:getComputedStyle(node).fontFamily}));if(titleStyle.size>14||!titleStyle.font.includes('Inter'))throw Error('native answer typography diverged from Steward');
  if((await page.locator('.native-time-picker').allTextContents()).some(text=>/[📅🗓🔄]/u.test(text)))throw Error('native answer retained decorative emoji');
  if(await page.locator('.intelligent-widget summary').count())throw Error('native composition retained Text version');
  const chartBox=await page.locator('[data-openintelligentui=barChart] svg.recharts-surface').boundingBox();if(chartBox.width<100||chartBox.height<100)throw Error('native chart SVG was clipped to zero size');
  for(const theme of ['dark','light']){await page.evaluate(theme=>document.documentElement.classList.toggle('dark',theme==='dark'),theme);await page.screenshot({path:evidence+'/native-components-'+theme+'.png',fullPage:true})}
  await page.setViewportSize({width:390,height:844});if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('native component overflowed mobile');await page.screenshot({path:evidence+'/native-components-mobile.png',fullPage:true});await page.setViewportSize({width:1400,height:980});
  const scenario=page.frameLocator('iframe[title="Trip comparison"]');await scenario.locator('#level').fill('42');
  const pickers=page.locator('[data-openintelligentui=scheduleTime]');
  const firstID=await pickers.nth(0).getAttribute('data-form-id'),secondID=await pickers.nth(1).getAttribute('data-form-id');
  const first=page.locator('[data-openintelligentui=scheduleTime][data-form-id="'+firstID+'"]');
  await page.getByLabel('Message Ghost',{exact:true}).fill('Keep the ordinary conversation draft.');
  let calls=0;await page.route('**/ui/form',async route=>{calls++;if(calls>1)await route.fetch();await route.abort('failed')});
  await first.getByRole('button',{name:/2026-12-01/}).click();await first.getByRole('alert').filter({hasText:'not confirmed'}).waitFor();
  await first.getByRole('button',{name:/2026-12-01/}).click();await page.locator('[data-openintelligentui=scheduleTime][data-form-id="'+firstID+'"][data-response="0"]').waitFor();await page.unroute('**/ui/form');
  if(calls!==2||await page.getByLabel('Message Ghost',{exact:true}).inputValue()!=='Keep the ordinary conversation draft.')throw Error('time response duplicated or replaced the draft');
  if(await scenario.locator('#level').inputValue()!=='42')throw Error('native response refresh destroyed the adjacent interactive state');
  await page.reload();await page.locator('[data-openintelligentui=scheduleTime][data-form-id="'+firstID+'"][data-response="0"]').waitFor();await first.getByText('Time Selected',{exact:true}).waitFor();
  const native=await browser.newContext({javaScriptEnabled:false});try{await native.addCookies(await context.cookies());const plain=await native.newPage();await plain.goto(url);const form=plain.locator('.native-meeting-fallback[data-form-id="'+secondID+'"]');await form.getByRole('button',{name:'None of these work',exact:true}).click();await plain.waitForURL('**/?notice=feedback-saved');if(await plain.locator('.native-meeting-fallback[data-form-id="'+secondID+'"]').getAttribute('data-submitted')!=='true')throw Error('native selection was not restored');await plain.screenshot({path:evidence+'/rich-replies-no-script.png',fullPage:true})}finally{await native.close()}
  await page.reload();await page.locator('[data-openintelligentui=scheduleTime][data-form-id="'+secondID+'"][data-response="decline"]').waitFor();

 }
 if(phase==='components'){
  await page.locator('.markdown strong').getByText('Markdown rendered',{exact:true}).waitFor();
  const download=page.getByRole('link',{name:'Download game',exact:true});
  const downloaded=await context.request.get(new URL(await download.getAttribute('href'),url).href);
  if(!downloaded.ok()||await downloaded.text()!=='<h1>Downloaded artifact</h1>'||!downloaded.headers()['content-disposition']?.includes('attachment'))throw Error('Markdown artifact did not download through the authenticated artifact route');
  if(await page.locator('.markdown a[href^="javascript:"],.markdown script,.markdown img:not(.markdown-diagram)').count())throw Error('Markdown allowed executable markup');
  await page.locator('.markdown-diagram').waitFor();
  if(!await page.locator('.markdown-diagram').evaluate(img=>img.complete&&img.naturalWidth>0))throw Error('Mermaid image did not render');
  for(const theme of ['dark','light']){
   await page.evaluate(theme=>document.documentElement.classList.toggle('dark',theme==='dark'),theme);
   await page.waitForFunction(theme=>document.querySelector('link[rel=icon]').getAttribute('href')==='/brand/ghost-'+(theme==='dark'?'ivory':'charcoal')+'.png',theme);
   const actions=page.locator('.chat-topbar .topbar-actions');const before=await actions.boundingBox();
   await page.evaluate(()=>scrollTo(0,document.documentElement.scrollHeight));await page.mouse.wheel(0,1000);
   const after=await actions.boundingBox();if(Math.abs(after.y-before.y)>1)throw Error('Computer and Worker moved with the document');
   await page.screenshot({path:evidence+'/markdown-'+theme+'.png',fullPage:true});
  }
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
  await page.locator('.approval').waitFor();
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
  await mobile.getByLabel('Message Ghost',{exact:true}).fill('');
  await mobile.evaluate(()=>{document.querySelector('textarea').blur();scrollTo(0,0)});
  await mobile.evaluate(()=>new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve))));
  const geometry=await mobile.evaluate(()=>({width:innerWidth,height:innerHeight,content:document.documentElement.scrollWidth,footerBottom:document.querySelector('footer').getBoundingClientRect().bottom}));
  if(geometry.content>geometry.width||geometry.footerBottom>geometry.height)throw new Error('mobile layout exceeds viewport');
  await mobile.screenshot({path:evidence+'/'+phase+'-mobile.png'});
  await writeFile(evidence+'/'+phase+'-mobile.json',JSON.stringify(geometry,null,2));
  await mobile.getByRole('button',{name:'Open navigation',exact:true}).focus();
  await mobile.getByRole('button',{name:'Open navigation',exact:true}).press('Enter');
  if(!await mobile.locator('#workspace-navigation').evaluate(node=>node.contains(document.activeElement)))throw Error('opening mobile navigation lost keyboard focus');
  await mobile.locator('.brand-row').getByRole('button',{name:'Close navigation',exact:true}).press('Enter');
  if(!await mobile.getByRole('button',{name:'Open navigation',exact:true}).evaluate(node=>node===document.activeElement))throw Error('closing mobile navigation lost keyboard focus');
  await mobile.setViewportSize({width:320,height:740});await mobile.goto(url+'/settings/capabilities');
  await mobile.evaluate(()=>document.fonts.ready);
  await mobile.getByRole('button',{name:'Open navigation',exact:true}).click();
  const selected=mobile.locator('.settings-navigation [aria-current="page"]');
  if(!await selected.isVisible()||!await selected.getAttribute('href').then(value=>value==='/settings/capabilities'))throw Error('mobile settings navigation lost its current section');
  await mobile.keyboard.press('Escape');
  await mobile.setViewportSize({width:1001,height:900});await mobile.goto(url);
  await mobile.getByRole('button',{name:'Open Computer',exact:true}).click();
  if(!await mobile.locator('#computer-panel').evaluate(panel=>panel.matches(':modal')))throw Error('Computer compresses Chat at intermediate widths');
  await mobile.getByRole('button',{name:'Close computer view',exact:true}).click();
 }finally{await mobile.close()}
 if(phase==='components'){
  const plain=await browser.newContext({javaScriptEnabled:false,viewport:{width:1400,height:980}});
  try{
   await plain.addCookies(await context.cookies());
   const fallback=await plain.newPage();await fallback.goto(url);
   if(await fallback.locator('.message').count()!==await page.locator('.message').count())throw new Error('no-JavaScript fallback lost messages');
   if(!await fallback.getByLabel('Message Ghost',{exact:true}).isVisible())throw new Error('no-JavaScript fallback lost composer');
   if(!await fallback.getByText('Processed: Retain this semantic answer.',{exact:true}).isVisible())throw new Error('no-JavaScript fallback lost semantics');
   await fallback.screenshot({path:evidence+'/'+phase+'-no-script.png',fullPage:true});
  }finally{await plain.close()}
 }
 await page.locator('aside').getByRole('link',{name:'Tasks',exact:false}).click();
 if(await page.locator('form[action="/task/manage"]').count())throw Error('task browsing gained a second control plane');
 await page.screenshot({path:evidence+'/'+phase+'-progress.png',fullPage:true});
 }
 if(remote.length)throw new Error('UI contacted an external service: '+remote.join(', '));
 if(errors.length)throw new Error(errors.join(';'));
}catch(error){
 if(phase==='account-signin'||phase==='account-reconnect'){const frame=String(error?.stack??'').split('\n').find(line=>line.includes('channels_browser.mjs:')||line.includes('channels_controls.mjs:'));throw Error('Browser did not complete the controlled account flow'+(frame?' ('+frame.trim()+')':''))}
 if(page){await page.screenshot({path:evidence+'/'+phase+'-failure.png',fullPage:true});await writeFile(evidence+'/'+phase+'-failure.txt',await page.locator('body').innerText())}
 throw error;
}finally{await browser.close()}
