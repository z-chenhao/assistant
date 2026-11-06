import assert from 'node:assert/strict';
import {selectValue} from './channels_controls.mjs';
import {writeFile} from 'node:fs/promises';

// Same server templates and bundled help used by preferences and live regions.
export async function contextHelp(page,context,browser,url,evidence){
 const bounds=async()=>{
  const tip=page.getByRole('tooltip'),rect=await tip.boundingBox(),viewport=page.viewportSize();
  assert(rect&&rect.x>=0&&rect.y>=0&&rect.x+rect.width<=viewport.width+1&&rect.y+rect.height<=viewport.height+1,'Help left the viewport');return rect;
 };
 const results=[];
 for(const width of [1400,320]){
  await page.setViewportSize({width,height:980});await page.goto(url+'/settings/apps');
  assert.equal(await page.locator('.apps-heading p').count(),0);
  assert.equal(await page.locator('a[href="/settings/services/apps"]').count(),0);
  assert.equal(await page.getByRole('button',{name:'Filter',exact:true}).count(),0);
  await page.locator('[data-app-tile=gmail] .app-summary').click();const dialog=page.locator('[data-app-dialog]');await dialog.waitFor();
  assert.equal(await dialog.getByRole('button',{name:'Connect',exact:true}).isDisabled(),true);
  const help=dialog.getByRole('button',{name:'Help: Connect',exact:true});await help.hover();await page.getByRole('tooltip').waitFor();results.push({page:'app-connection',width,bounds:await bounds()});
  await page.keyboard.press('Escape');assert.equal(await page.getByRole('tooltip').count(),0);assert.equal(await dialog.isVisible(),true);
  await dialog.getByRole('button',{name:'Close',exact:true}).click();
  await page.screenshot({path:`${evidence}/clean-apps-${width}.png`});
 }
 await page.goto(url+'/settings/preferences');
 const preference=page.locator('[data-help-trigger]');await preference.waitFor();
 await page.locator('#interface-theme-picker').focus();await preference.hover();
 const control=await preference.boundingBox(),tip=await bounds();
 for(let y=control.y+control.height/2;y<=tip.y+10;y++){
  await page.mouse.move(control.x+control.width/2,y);assert.equal(await page.getByRole('tooltip').isVisible(),true,'Continuous hover path broke');
 }
 assert.equal(await page.getByRole('tooltip').evaluate(el=>el.matches(':hover')),true);
 await page.mouse.move(319,970);await page.getByRole('tooltip').waitFor({state:'hidden'});
 await preference.focus();await page.getByRole('tooltip').waitFor();await page.keyboard.press('Escape');
 assert.equal(await preference.evaluate(el=>el===document.activeElement),true);
 await page.goto(url+'/settings/notifications');
 await page.getByRole('combobox',{name:'Send future notifications to',exact:true}).waitFor();
 await page.getByRole('button',{name:'Help: Send future notifications to',exact:true}).hover();await bounds();
 await page.screenshot({path:`${evidence}/notifications-help-320.png`});
 await page.goto(url+'/tasks/schedules/new');
 await page.getByRole('switch',{name:'Schedule enabled',exact:true}).waitFor();
 const form=page.locator('.schedule-editor'),saved=await form.evaluate(el=>JSON.stringify([...new FormData(el).entries()]));
 await page.getByRole('button',{name:'Help: Schedule enabled',exact:true}).click();await bounds();await page.keyboard.press('Escape');
 assert.equal(await form.evaluate(el=>JSON.stringify([...new FormData(el).entries()])),saved,'Reading switch help changed the schedule state');
 await page.getByRole('button',{name:'Help: Time zone',exact:true}).focus();await bounds();await page.keyboard.press('Escape');
 assert.equal(await form.evaluate(el=>JSON.stringify([...new FormData(el).entries()])),saved,'Reading guidance changed schedule fields');
 await selectValue(page,page.locator('#schedule-frequency'),'monthly');
 await page.getByRole('button',{name:'Help: Day of month',exact:true}).hover();await bounds();
 await page.screenshot({path:`${evidence}/schedule-help-320.png`});
 await page.goto(url+'/tasks/task:visible');
 const rule=page.getByRole('button',{name:'Help: Execution rule',exact:true});await rule.waitFor();await rule.hover();await bounds();
 await page.screenshot({path:`${evidence}/task-help-320.png`});await page.keyboard.press('Escape');
 // A live replacement gets usable help without rerunning the page initializer.
 await rule.evaluate(el=>{
  const copy=el.closest('[data-context-help]').cloneNode(true),tip=copy.querySelector('[data-help-tooltip]'),trigger=copy.querySelector('[data-help-trigger]');
  tip.id+='-dynamic';tip.hidden=true;trigger.hidden=true;trigger.setAttribute('aria-describedby',tip.id);trigger.setAttribute('aria-label','Dynamic help');copy.querySelector('[data-help-fallback]').hidden=false;
  document.querySelector('.task-page').append(copy);
 });
 const dynamic=page.getByRole('button',{name:'Dynamic help',exact:true});await dynamic.waitFor();await dynamic.hover();await bounds();await page.keyboard.press('Escape');
 const guide=await rule.evaluate(el=>{const tip=el.closest('[data-context-help]').querySelector('[data-help-tooltip]'),text=tip.textContent;tip.textContent=text.repeat(4);return text});
 await page.evaluate(()=>document.documentElement.style.zoom='2');await rule.hover();await bounds();
 await page.getByRole('tooltip').evaluate(async el=>{if(el.scrollHeight<=el.clientHeight)throw new Error('Long help must scroll');el.scrollTop=24;await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));if(el.scrollTop!==24)throw new Error('Help scroll position was reset')});
 await page.keyboard.press('Escape');await page.evaluate(()=>document.documentElement.style.zoom='');await rule.evaluate((el,text)=>{el.closest('[data-context-help]').querySelector('[data-help-tooltip]').textContent=text},guide);
 const touch=await browser.newContext({hasTouch:true,viewport:{width:320,height:740}});
 await touch.addCookies(await context.cookies());const mobile=await touch.newPage();await mobile.goto(url+'/settings/notifications');
 const mobileHelp=mobile.getByRole('button',{name:'Help: Send future notifications to',exact:true});await mobileHelp.tap();await mobile.getByRole('tooltip').waitFor();await mobileHelp.tap();await mobile.getByRole('tooltip').waitFor({state:'hidden'});await touch.close();
 const native=await browser.newContext({javaScriptEnabled:false,viewport:{width:320,height:740}});await native.addCookies(await context.cookies());const fallback=await native.newPage();await fallback.goto(url+'/tasks/schedules/new');
 await fallback.getByRole('button',{name:'Help: Time zone',exact:true}).waitFor({state:'hidden'});
 await fallback.locator('summary[aria-label="Help: Time zone"]').click();
 await fallback.getByText('Use an IANA time zone. One-time times during a clock change must identify one unambiguous instant.',{exact:true}).first().waitFor();await fallback.screenshot({path:`${evidence}/schedule-help-native-320.png`});await native.close();
 await writeFile(evidence+'/help-checks.json',JSON.stringify({viewportBounds:results,hover:true,keyboard:true,touch:true,lateRegions:true,zoom:true,nativeFallback:true,fieldNamesPreserved:true,formUnchanged:true},null,2));
 await page.setViewportSize({width:1400,height:980});
}
