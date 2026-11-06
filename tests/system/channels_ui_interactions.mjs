import {readFile} from 'node:fs/promises';
export async function uiInteractions(page,context,browser,url,evidence,restart=false){
 const table=page.locator('[data-openintelligentui="Table"]'),line=page.locator('[data-openintelligentui="steward.lineChart"]'),scatter=page.locator('[data-openintelligentui="steward.scatterChart"]'),form=page.locator('[data-openintelligentui="steward.form"]').first(),cards=page.locator('[data-openintelligentui="steward.cards"]'),action=page.locator('[data-openintelligentui="steward.action"]'),widget=page.locator('.intelligent-widget').filter({has:page.locator('iframe[title="Explorer"]')}),frame=page.frameLocator('iframe[title="Explorer"]');
 await form.getByRole('textbox',{name:'Notes',exact:true}).waitFor();await frame.locator('#quantity').waitFor();
 if(restart){
  if(await frame.locator('#quantity').inputValue()!=='7')throw Error('restart lost sandbox input');
  if(await form.getByRole('textbox',{name:'Notes',exact:true}).inputValue()!=='Keep this draft.')throw Error('restart lost submitted form');
  if(!await form.getByRole('button',{name:/Sent/}).isDisabled())throw Error('restart reopened accepted form');
  if(await page.locator('.intelligent-widget summary').count())throw Error('text fallback control returned');
  await page.screenshot({path:evidence+'/restored.png',fullPage:true});return;
 }
 const activate=async button=>{await button.hover();await button.click()};
 const menu=async(scope,name)=>{await activate(scope.getByRole('button',{name:'Options',exact:true}));await page.getByRole('menuitem',{name,exact:true}).click()};
 const downloaded=async(scope,name,ext)=>{const pending=page.waitForEvent('download');await menu(scope,name);const file=await pending;if(!file.suggestedFilename().endsWith(ext))throw Error('incorrect export extension');const path=await file.path();const bytes=await readFile(path);if(ext==='.png'&&!bytes.subarray(0,8).equals(Buffer.from([137,80,78,71,13,10,26,10])))throw Error('invalid PNG');if(ext==='.svg'&&!bytes.toString().includes('<svg'))throw Error('invalid SVG');return bytes};
 await line.locator('.recharts-line-curve').waitFor();await scatter.locator('.recharts-scatter-symbol').first().waitFor();
 await table.getByRole('columnheader',{name:'Value',exact:true}).getByRole('button').click();
 if(await table.locator('tbody tr').first().innerText().then(text=>!text.includes('10')))throw Error('numeric ascending sort failed');
 await table.getByRole('columnheader',{name:'Value',exact:true}).getByRole('button').click();if(!await table.locator('tbody tr').first().innerText().then(text=>text.includes('42')))throw Error('numeric descending sort failed');
 await menu(table,'Filter');await table.getByRole('searchbox',{name:'Filter rows'}).fill('A');if(await table.locator('tbody tr').count()!==1)throw Error('table filter failed');
 await page.waitForFunction(()=>document.querySelector('[data-openintelligentui="steward.lineChart"] svg .recharts-line')!==null);
 const bytes=await downloaded(table,'Download CSV','.csv');if(!bytes.toString().includes('"A","10"')||bytes.toString().includes('"B","42"'))throw Error('CSV ignored current filter');
 await menu(table,'Reset');await line.locator('[data-chart-label="B"]').click();if(await table.locator('tbody tr').count()!==1||!(await table.locator('tbody tr').innerText()).includes('42'))throw Error('chart point selection did not filter the table');await line.locator('[data-chart-label="B"]').focus();await page.keyboard.press('Enter');if(await table.locator('tbody tr').count()!==3)throw Error('keyboard chart selection did not clear the filter');await downloaded(line,'Download SVG','.svg');await downloaded(scatter,'Download PNG','.png');
 await menu(line,'Zoom');const ranges=line.locator('input[type=range]');await ranges.nth(1).fill('1');if(await line.locator('.recharts-line-dot').count()>2)throw Error('chart range did not update');await menu(line,'Reset');
 // The same live frame must survive every Host display mode.
 await frame.locator('#quantity').fill('7');await frame.locator('#value').getByText('14',{exact:true}).waitFor();
 const frameNode=await widget.locator('iframe').elementHandle();
 const exportedSVG=await downloaded(widget,'Download SVG','.svg');if(/<script|onload=/i.test(exportedSVG.toString()))throw Error('graphic export retained executable markup');await downloaded(widget,'Download PNG','.png');const htmlSnapshot=await downloaded(widget,'Download HTML','.html');if(!htmlSnapshot.toString().includes('Content-Security-Policy'))throw Error('HTML snapshot lost its resource boundary');
 for(const[command,mode]of [['Expand','fullscreen'],['Open dialog','modal'],['Keep visible','pip']]){
  await menu(widget,command);await widget.locator('.answer-surface[data-mode="'+mode+'"]').waitFor();
  if(!await frameNode.evaluate(node=>node.isConnected)||await frame.locator('#quantity').inputValue()!=='7')throw Error('display mode remounted the sandbox');
  if(mode==='modal'&&!await widget.locator('.answer-surface').evaluate(node=>node.matches(':modal')))throw Error('dialog did not use the Host top layer');
  await activate(widget.getByRole('button',{name:'Close expanded view',exact:true}));
 }
 await activate(frame.getByRole('button',{name:'Open details',exact:true}));await widget.locator('.answer-surface[data-mode=modal]').waitFor();await page.keyboard.press('Escape');await widget.locator('.answer-surface[data-mode=inline]').waitFor();
 // Input snapshots are captured before the ordinary action request. A lost
 // HTTP acknowledgement retries the identical source, not a second execution.
 await table.getByRole('searchbox',{name:'Filter rows'}).fill('task');
 let lost=false;const sources=[];await page.route('**/api/ui/action',async route=>{if(route.request().method()!=='POST'){await route.continue();return}const value=route.request().postDataJSON();sources.push(value.source_id);if(!lost){lost=true;await route.fetch();await route.abort('failed')}else await route.continue()});
 await activate(action.getByRole('button',{name:'Refresh catalog',exact:true}));
 await action.getByRole('button',{name:'Refresh catalog',exact:true}).waitFor({state:'visible'});
 await page.waitForFunction(()=>!document.querySelector('[data-openintelligentui="steward.action"] button').disabled);
 if(await action.getByRole('alert').count())await activate(action.getByRole('button',{name:'Refresh catalog',exact:true}));
 await page.waitForFunction(()=>!!document.querySelector('[data-openintelligentui="steward.action"] button svg'));
 if(sources.length>1&&sources[0]!==sources[1])throw Error('lost action receipt changed source identity');await page.unroute('**/api/ui/action');
 await table.getByRole('searchbox',{name:'Filter rows'}).fill('memory');await activate(frame.getByRole('button',{name:'Refresh from Ghost',exact:true}));await action.locator('button[data-proposed=true]').waitFor();await activate(action.getByRole('button',{name:'Refresh catalog',exact:true}));await frame.locator('#value').getByText('Refreshed',{exact:true}).waitFor();await menu(table,'Reset');
 // Native required validation focuses the visible selector, not its hidden value.
 await form.getByLabel('Departure',{exact:true}).fill('2026-11-09');await form.getByLabel('Travelers',{exact:true}).fill('3');await form.getByLabel('Confirmed',{exact:true}).check();
 await form.getByRole('button',{name:'Send',exact:true}).click();await form.getByLabel('Travel mode',{exact:true}).and(page.locator('[aria-invalid=true]')).waitFor();
 await form.getByLabel('Travel mode',{exact:true}).and(page.locator(':focus')).waitFor();
 // Host controls remain interactive inside the browser's modal top layer.
 await menu(form,'Open dialog');await form.locator('.answer-surface[data-mode=modal]').waitFor();
 await activate(form.getByRole('button',{name:'Options',exact:true}));await page.getByRole('menuitem',{name:'Keep visible',exact:true}).waitFor();await page.keyboard.press('Escape');
 await form.getByLabel('Travel mode',{exact:true}).click();await page.getByRole('listbox',{name:'Options',exact:true}).getByRole('option',{name:'Train',exact:true}).click();
 await activate(form.getByRole('button',{name:'Close expanded view',exact:true}));
 // Draft values survive a full navigation; selection is submitted only once.
 await form.getByLabel('Departure',{exact:true}).fill('2026-11-09');await form.getByLabel('Travelers',{exact:true}).fill('3');
 await form.getByLabel('Travel mode',{exact:true}).click();await page.getByRole('listbox',{name:'Options',exact:true}).getByRole('option',{name:'Train',exact:true}).click();await form.getByLabel('Meal',{exact:true}).check();await form.getByRole('textbox',{name:'Notes',exact:true}).fill('Keep this draft.');await form.getByLabel('Confirmed',{exact:true}).check();
 // A normal composer acceptance flushes all changed UI state before freezing.
 await page.getByLabel('Message Ghost',{exact:true}).fill('');await form.getByRole('textbox',{name:'Notes',exact:true}).blur();
 await page.waitForResponse(r=>r.url().endsWith('/api/ui/state')&&r.request().method()==='POST'&&r.status()===200);
 await page.reload();await form.getByRole('textbox',{name:'Notes',exact:true}).waitFor();await page.waitForFunction(()=>document.querySelector('[data-openintelligentui="steward.form"] textarea')?.value==='Keep this draft.');
 if(await frame.locator('#quantity').inputValue()!=='7')throw Error('refresh lost sandbox state');
 const inputs=await context.request.get(url+'/api/conversation').then(r=>r.json());const before=inputs.filter(m=>m.role==='user').length;
 await form.getByRole('button',{name:'Send',exact:true}).click();await form.getByRole('button',{name:/Sent/}).waitFor();
 await page.waitForFunction(()=>document.querySelector('[data-openintelligentui="steward.form"] button[type=submit]')?.matches(':disabled'));
 await page.locator('.message.user').filter({hasText:'Keep this draft.'}).waitFor();const after=await context.request.get(url+'/api/conversation').then(r=>r.json());if(after.filter(m=>m.role==='user').length!==before+1)throw Error('form did not become exactly one ordinary input');
 // Server field checks and native no-JS replay use the same accepted identity.
 const formID=await form.getAttribute('data-form-id');
 const plain=await browser.newContext({javaScriptEnabled:false});try{await plain.addCookies(await context.cookies());const p=await plain.newPage();await p.goto(url);const native=p.locator('.native-input-form[data-form-id="'+formID+'"]');if(!await native.locator('button[type=submit]').isDisabled())throw Error('native form failed to restore accepted answer');const feedback=p.locator('.native-input-form').filter({has:p.getByRole('heading',{name:'Feedback',exact:true})});await feedback.getByLabel('Comment',{exact:true}).fill('Native submission');await feedback.getByLabel('Tags',{exact:true}).selectOption(['x','y']);await feedback.getByRole('button',{name:'Send',exact:true}).click();await p.waitForURL('**/?notice=feedback-saved');if(!await p.locator('.native-input-form').filter({has:p.getByRole('heading',{name:'Feedback',exact:true})}).locator('button[type=submit]').isDisabled())throw Error('native multiselect submission did not restore');await p.screenshot({path:evidence+'/native-form.png',fullPage:true})}finally{await plain.close()}
 await cards.getByRole('button',{name:'Next card',exact:true}).click();
 for(const width of [1400,390]){await page.setViewportSize({width,height:980});const controls=await cards.getByRole('button',{name:'Next card',exact:true}).boundingBox(),firstCard=await cards.locator('article').first().boundingBox();if(!controls||!firstCard||controls.y+controls.height>firstCard.y+1)throw Error('carousel controls overlap card content')}
 await page.setViewportSize({width:1400,height:980});await page.screenshot({path:evidence+'/desktop-light.png',fullPage:true});await page.evaluate(()=>document.documentElement.classList.add('dark'));await page.screenshot({path:evidence+'/desktop-dark.png',fullPage:true});
 for(const [name,scope] of [['line-chart',line],['scatter-chart',scatter],['carousel',cards]]){await scope.evaluate(node=>node.scrollIntoView({block:'center'}));await scope.screenshot({path:evidence+'/'+name+'.png'})}
 await page.setViewportSize({width:390,height:844});await widget.evaluate(node=>node.scrollIntoView({block:'center'}));if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('interaction UI overflowed mobile: '+JSON.stringify(await page.evaluate(()=>({width:innerWidth,content:document.documentElement.scrollWidth,overflow:[...document.querySelectorAll('body *')].filter(n=>n.getBoundingClientRect().right>innerWidth+1&&getComputedStyle(n).display!=='none').slice(0,12).map(n=>({tag:n.tagName,classes:n.className,right:n.getBoundingClientRect().right}))}))));await page.screenshot({path:evidence+'/mobile.png'});
 if((await page.locator('body').innerText()).includes('Select a time that works for you')||await page.locator('.intelligent-widget summary').count())throw Error('introductory copy or duplicate fallback appeared');
 await page.setViewportSize({width:1400,height:980});
}
