// Maintained Web UI driver: one real browser, no app-specific mock DOM.
import {chromium} from '../../guest-browser/node_modules/playwright-core/index.mjs';
import {mkdir,writeFile} from 'node:fs/promises';
const [url,evidence,phase='conversation']=process.argv.slice(2);
await mkdir(evidence,{recursive:true});
const options={headless:true};
if(process.env.COLLIE_BROWSER_EXECUTABLE)options.executablePath=process.env.COLLIE_BROWSER_EXECUTABLE;
const browser=await chromium.launch(options);
let page;
try{
 const context=await browser.newContext({viewport:{width:1400,height:980},deviceScaleFactor:1});
 page=await context.newPage();
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 const remote=[];page.on('request',r=>{if(new URL(r.url()).origin!==new URL(url).origin)remote.push(r.url())});
 await page.goto(url);
 await page.getByLabel('Workspace access token').fill(process.env.COLLIE_WEB_TEST_TOKEN);
 await page.getByRole('button',{name:'Open workspace'}).click();
 await page.waitForURL(url+'/');
 await page.locator('#composer-root textarea').waitFor();
 // Prove the maintained frontend bundle is running, not just its HTML fallback.
 await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
 for(const image of await page.getByAltText(/Collie (logo|default agent avatar)/).all()){
  if(!await image.evaluate(img=>img.complete&&img.naturalWidth===1297&&img.naturalHeight===1212))throw new Error('Collie artwork failed to load');
 }
 if(phase==='setup'){
  if(!await page.getByRole('button',{name:'Send message',exact:true}).isDisabled())throw Error('unconfigured composer can send');
  if(await page.locator('.welcome').count())throw Error('setup competes with the normal welcome');
  await page.getByRole('link',{name:'Set up Collie',exact:false}).click();
  await page.getByRole('link',{name:'Connect a model',exact:true}).click();
  await page.getByRole('link',{name:'Local or compatible API',exact:false}).click();
  if(await page.getByRole('checkbox',{name:'Enable image input (verified when tested)',exact:true}).isChecked())throw Error('vision declaration enabled implicitly');
  await page.locator('input[name="endpoint"]').fill('http://127.0.0.1:1/v1');
  await page.locator('input[name="model"]').fill('fixture-model');
  await page.getByRole('button',{name:'Add connection',exact:true}).click();
  if(!new URL(page.url()).pathname.startsWith('/settings/models/'))throw Error('add left model setup');
  await page.getByRole('button',{name:'Test connection',exact:true}).click();
  if(!new URL(page.url()).pathname.startsWith('/settings/models/'))throw Error('test left model setup');
  await page.getByRole('checkbox',{name:'Use this provider for new work'}).check();
  await page.getByRole('button',{name:'Select model',exact:true}).click();
  await page.getByRole('heading',{name:'Ready for new work',exact:true}).waitFor();
  await page.screenshot({path:evidence+'/model-ready.png',fullPage:true});
  // Deterministically type into usable server HTML before the local bundle is
  // released. Progressive enhancement must preserve this draft and its file.
  let release;
  const bundle=new Promise(resolve=>{release=resolve});
  let requested;
  const loading=new Promise(resolve=>{requested=resolve});
  await page.route('**/app.js',async route=>{requested();await bundle;await route.continue()});
  const selecting=page.getByRole('link',{name:'Start a conversation',exact:false}).click();
  await loading;
  await page.getByLabel('Message Collie').fill('Plan my day: finish the release risk note before noon.');
  await page.getByLabel('Attach a file').setInputFiles({name:'web-note.txt',mimeType:'text/plain',buffer:Buffer.from('Web owner attachment fixture.\n')});
  await page.getByLabel('Message Collie').focus();
  await page.getByLabel('Message Collie').press('ArrowLeft');
  const earlySource=await page.locator('input[name="source_id"]').inputValue();
  const selection=await page.getByLabel('Message Collie').evaluate(input=>[input.selectionStart,input.selectionEnd]);
  release();await selecting;
  await page.unroute('**/app.js');
  await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
  await page.evaluate(()=>document.fonts.ready);
  await page.locator('.welcome').evaluate(el=>Promise.all(el.getAnimations().map(a=>a.finished)));
  if(!await page.evaluate(()=>document.fonts.check('500 16px "DM Sans"')&&document.fonts.check('400 40px "Lora"')))throw Error('local brand typography unavailable');
  await page.screenshot({path:evidence+'/welcome.png',fullPage:true});
  if(await page.getByLabel('Message Collie').inputValue()!=='Plan my day: finish the release risk note before noon.')throw new Error('frontend enhancement lost early draft');
  const preserved=await page.getByLabel('Message Collie').evaluate(input=>document.activeElement===input&&JSON.stringify([input.selectionStart,input.selectionEnd]));
  if(preserved!==JSON.stringify(selection)||await page.locator('input[name="source_id"]').inputValue()!==earlySource)throw new Error('frontend enhancement changed draft focus, selection or source identity');
  const file=await page.getByLabel('Attach a file').evaluate(async input=>input.files[0]?{name:input.files[0].name,text:await input.files[0].text()}:null);
  if(file?.name!=='web-note.txt'||file?.text!=='Web owner attachment fixture.\n')throw new Error('frontend enhancement lost early attachment');
  const submitted=page.waitForResponse(r=>r.url()===url+'/message'&&r.request().method()==='POST');
  await page.getByRole('button',{name:'Send message',exact:true}).click();
  if((await submitted).status()!==200)throw new Error('composer submission was not accepted');
  await page.waitForFunction(()=>document.querySelectorAll('.message.assistant').length>0);
  const native=await browser.newContext({javaScriptEnabled:false});
  try{
   await native.addCookies(await context.cookies());const settings=await native.newPage();
   await settings.goto(url+'/settings/notifications');await settings.locator('select[name=target]').selectOption('tui:');
   await settings.getByRole('button',{name:'Save notification channel',exact:true}).click();
   if(new URL(settings.url()).pathname!=='/settings/notifications'||await settings.locator('select[name=target]').inputValue()!=='tui:')throw Error('saving notifications lost its route or selected value');
   await settings.reload();if(await settings.locator('select[name=target]').inputValue()!=='tui:')throw Error('notification preference did not persist');
   await settings.screenshot({path:evidence+'/notifications.png',fullPage:true});
   await settings.locator('select[name=target]').selectOption('auto:');await settings.getByRole('button',{name:'Save notification channel',exact:true}).click();
   await settings.goto(url+'/settings/models');await settings.getByRole('region',{name:'Current model',exact:true}).waitFor();
   await settings.screenshot({path:evidence+'/models.png',fullPage:true});
   await settings.getByRole('link',{name:'Connect a model',exact:true}).click();await settings.screenshot({path:evidence+'/model-sources.png',fullPage:true});
  }finally{await native.close()}
 }

 if(phase==='native-trace'){
  const facts=JSON.parse(process.env.COLLIE_NATIVE_TRACE_FACTS);
  await page.getByRole('link',{name:'Developer',exact:false}).click();
  await page.getByRole('heading',{name:'Traces',exact:true}).waitFor();
  await page.locator('[data-execution-id="'+facts.main+'"] td').first().getByRole('link',{name:'Main',exact:true}).click();
  await page.getByRole('heading',{name:'Trace tree',exact:true}).waitFor();
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
  await page.getByRole('heading',{name:'Goal',exact:true}).waitFor();
  if(!await page.locator('#task-messages').innerText().then(text=>text.includes('Main Task follow-up through Pi.')))throw Error('completed task did not show Main delivery');
  await page.screenshot({path:evidence+'/task-result.png',fullPage:true});

 }
 if(phase==='inspection'){
  const facts=JSON.parse(process.env.COLLIE_INSPECTION_FACTS);
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
  await page.getByLabel('Role',{exact:true}).selectOption('root');
  await page.getByLabel('Status',{exact:true}).selectOption('completed');
  await page.getByRole('button',{name:'Apply filters',exact:true}).click();
  if(await page.locator('[data-execution-id]').count()!==1||await page.locator('[data-execution-id]:not([data-role="root"])').count())throw new Error('execution filters ignored');
  await page.getByText('Created time',{exact:true}).click();
  await page.getByLabel('Created before (UTC)').fill('2000-01-01T00:00');
  await page.getByRole('button',{name:'Apply filters',exact:true}).click();
  await page.getByRole('heading',{name:'No matching executions',exact:true}).waitFor();
  await page.getByRole('link',{name:'Reset',exact:true}).click();
  await page.locator('[data-execution-id="'+root.id+'"] td').first().getByRole('link').click();
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
  await page.locator('.obs-tree .observation-node').filter({hasText:'Generate response'}).last().locator('a').first().click();
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
  await page.getByRole('link',{name:'Manage model',exact:false}).click();
  const capability=process.env.COLLIE_TEST_VISION==='yes'?'Image input verified':'Text only · attachments by reference';
  await page.getByText(capability,{exact:false}).waitFor();
  // The owner may open the native picker while the enhancement bundle loads.
  // Its input must survive that enhancement, including an unfinished selection.
  let release,requested;
  const bundle=new Promise(resolve=>{release=resolve});
  const loading=new Promise(resolve=>{requested=resolve});
  await page.route('**/app.js',async route=>{requested();await bundle;await route.continue()});
  const navigation=page.locator('.app-sidebar').getByRole('link',{name:'Conversation',exact:true}).click();
  await loading;
  await page.getByLabel('Message Collie',{exact:true}).fill('USER_IMAGE_INPUT: inspect this upload.');
  const [picker]=await Promise.all([page.waitForEvent('filechooser'),page.getByLabel('Attach a file').click()]);
  release();await navigation;await page.unroute('**/app.js');
  await page.waitForFunction(()=>document.querySelector('.composer textarea')?.getAttribute('class')?.includes('resize-none'));
  if(!await picker.element().evaluate(input=>input===document.querySelector('.composer input[type=file]')))throw new Error('enhancement replaced the open file picker');
  await picker.setFiles({name:'user-picture.png',mimeType:'image/png',buffer:Buffer.from(process.env.COLLIE_TEST_PICTURE,'base64')});
  await page.getByRole('button',{name:'Send message',exact:true}).click();
  const expected=process.env.COLLIE_TEST_VISION==='yes'?'USER_IMAGE_PIXELS_RECEIVED':'USER_IMAGE_REFERENCE_ONLY';
  await page.locator('.message.assistant').filter({hasText:expected}).waitFor();
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
  const editor=page.getByLabel('Message Collie',{exact:true});
  await editor.fill('Keep this unsent draft exactly.');
  await page.getByLabel('Attach a file').setInputFiles({name:'pending.txt',mimeType:'text/plain',buffer:Buffer.from('Unsent attachment bytes.\n')});
  await editor.focus();await editor.press('ArrowLeft');
  const source=await page.locator('input[name="source_id"]').inputValue();
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
  let attemptedSends=0;
  await page.route('**/message',async route=>{attemptedSends++;await route.abort('failed')});
  await page.getByRole('button',{name:'Send message',exact:true}).click();
  await page.locator('#composer-feedback').filter({hasText:'Send was not confirmed'}).waitFor();
  await checkDraft(false);if(attemptedSends!==1)throw Error('failed send was automatically repeated');
  await page.unroute('**/message');await editor.focus();await editor.evaluate((input,range)=>input.setSelectionRange(...range),selection);
  await echo('live-reply','LIVE_REPLY_WHILE_TYPING');
  await page.locator('.message.assistant').filter({hasText:'LIVE_REPLY_WHILE_TYPING'}).waitFor();
  await checkDraft();
  const top=await page.locator('[data-live-key="'+anchor.key+'"]').evaluate(node=>node.getBoundingClientRect().top);
  if(Math.abs(top-anchor.top)>2)throw new Error('background update moved reading position');
  await page.getByRole('button',{name:'New messages'}).waitFor();
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
  const frozen=await approval.locator('input[name="hash"]').inputValue();
  const progress=await context.newPage();
  try{
   await progress.goto(url+'/tasks?view=all');await progress.locator('[data-live-key^="task:"]').waitFor();
   const tasks=await (await context.request.get(url+'/api/progress')).json();const ref=tasks[0].taskRef;
   const ongoing=progress.locator('[data-live-key="task:'+ref+'"]');
   await ongoing.filter({hasText:'Waiting for your approval'}).waitFor();
   const detail=await context.newPage();
   try{
    await detail.goto(url+'/tasks/'+encodeURIComponent(ref));
    await detail.getByRole('heading',{name:'Goal',exact:true}).waitFor();
    if(await detail.locator('.approval form input[name=hash]').inputValue()!==frozen)throw Error('task detail displayed another approval');
    const communication=await context.newPage();try{
     await communication.goto(await detail.getByRole('link',{name:'Continue this task',exact:false}).getAttribute('href').then(path=>url+path));
     if(await communication.locator('input[name=task_ref]').inputValue()!==ref)throw Error('continuation lost task context');
     await communication.getByLabel('Message Collie',{exact:true}).fill('Keep this task draft.');
     await communication.getByRole('button',{name:'Clear task context',exact:true}).click();
     if(await communication.getByLabel('Message Collie',{exact:true}).inputValue()!=='Keep this task draft.'||await communication.locator('input[name=task_ref]').count())throw Error('clearing task context discarded its draft');
    }finally{await communication.close()}
    await detail.screenshot({path:evidence+'/task-detail.png',fullPage:true});
    await detail.setViewportSize({width:390,height:844});if(await detail.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('mobile task detail overflow');
    await detail.screenshot({path:evidence+'/task-detail-mobile.png',fullPage:true});
   }finally{await detail.close()}
   await page.bringToFront();
   const decision=page.waitForResponse(response=>response.url()===url+'/approval'&&response.request().method()==='POST');
   await approval.getByRole('button',{name:'Decline',exact:true}).click();
   const acknowledged=await decision;
   if(acknowledged.status()!==200||!(await acknowledged.json()).accepted)throw new Error('live approval was not acknowledged');
   if(!acknowledged.request().postData().includes(frozen))throw new Error('approval changed the frozen action hash');
   await page.waitForFunction(()=>!document.querySelector('.approval form'));
   await checkDraft(false);
   await progress.bringToFront();
   await ongoing.filter({hasText:'This round is complete; the task remains open'}).waitFor();
   if((await ongoing.innerText()).includes('must not be written'))throw new Error('raw tool arguments became stage progress');
   await send('live-cancel',plan('builtin__task_manage',{operation:'cancel',taskRef:ref}));
   await progress.bringToFront();
   await progress.locator('[data-live-key="task:'+ref+'"] .state').filter({hasText:'cancelled'}).waitFor();
   if(await progress.locator('form[action="/task/manage"]').count())throw Error('task browsing gained a second control plane');
  }finally{await progress.close();await page.bringToFront()}
  // Another authenticated browser rotates the session. The old page must keep
  // its draft and offer explicit reconnect, without redirecting to login.
  const other=await browser.newContext();
  try{await other.request.post(url+'/session',{form:{token:process.env.COLLIE_WEB_TEST_TOKEN}})}finally{await other.close()}
  await page.getByRole('status').filter({hasText:'Updates paused'}).waitFor();
  await checkDraft(false);
  await context.request.post(url+'/session',{form:{token:process.env.COLLIE_WEB_TEST_TOKEN}});
  const resumed=page.waitForResponse(response=>response.url().includes('/api/updates?')&&[200,304].includes(response.status()));
  await page.getByRole('button',{name:'Retry updates',exact:true}).click();await resumed;
  await checkDraft(false);
  if(navigations!==0)throw new Error('live updates navigated the document');
  await writeFile(evidence+'/live-contract.json',JSON.stringify({draft:true,attachment:true,focus:true,selection:true,scroll:true,conditional:true,reconnect:true,approval:true,progress:true,semanticStages:true,session:true,navigations},null,2));
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
  await mobile.getByLabel('Message Collie',{exact:true}).fill('');
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
   if(!await fallback.getByLabel('Message Collie',{exact:true}).isVisible())throw new Error('no-JavaScript fallback lost composer');
   if(!await fallback.getByText('Processed: Retain this semantic answer.',{exact:true}).isVisible())throw new Error('no-JavaScript fallback lost semantics');
   await fallback.screenshot({path:evidence+'/'+phase+'-no-script.png',fullPage:true});
  }finally{await plain.close()}
 }
 await page.locator('aside').getByRole('link',{name:'Tasks',exact:false}).click();
 if(await page.locator('form[action="/task/manage"]').count())throw Error('task browsing gained a second control plane');
 await page.screenshot({path:evidence+'/'+phase+'-progress.png',fullPage:true});
 if(remote.length)throw new Error('UI contacted an external service: '+remote.join(', '));
 if(errors.length)throw new Error(errors.join(';'));
}catch(error){
 if(page){await page.screenshot({path:evidence+'/'+phase+'-failure.png',fullPage:true});await writeFile(evidence+'/'+phase+'-failure.txt',await page.locator('body').innerText())}
 throw error;
}finally{await browser.close()}
