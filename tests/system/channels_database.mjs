import {writeFile} from 'node:fs/promises';

export async function databaseInspection(page,context,browser,url,evidence){
 await page.locator('aside').getByRole('link',{name:'Developer',exact:true}).click();
 await page.getByRole('navigation',{name:'Developer sections'}).getByRole('link',{name:'Database',exact:true}).click();
 await page.locator('.database-page').waitFor();
 const filterOffset=await page.locator('.database-filter').evaluate(form=>{
  const field=form.querySelector('input[name=ref]').getBoundingClientRect();
  const action=form.querySelector('button').getBoundingClientRect();
  return Math.abs(field.bottom-action.bottom);
 });
 if(filterOffset>1)throw Error('record reference and filter action sit on different baselines');
 const collections=page.getByRole('navigation',{name:'Record collections'});
 if(await collections.getByRole('link').count()!==4)throw Error('metadata collections missing');
 await collections.getByRole('link',{name:'Messages',exact:true}).click();
 const firstRefs=await page.locator('[data-record-ref]').evaluateAll(rows=>rows.map(row=>row.dataset.recordRef));
 if(firstRefs.length!==20)throw Error('default metadata page is not bounded');
 await page.getByRole('link',{name:'Older records →',exact:true}).click();
 const secondRefs=await page.locator('[data-record-ref]').evaluateAll(rows=>rows.map(row=>row.dataset.recordRef));
 if(!secondRefs.length||secondRefs.some(ref=>firstRefs.includes(ref)))throw Error('browser metadata continuation lost its position');
 const selected=firstRefs[0];
 await page.getByLabel('Exact record reference',{exact:true}).fill(selected);
 await page.getByRole('button',{name:'Find record',exact:true}).click();
 if(await page.locator('[data-record-ref]').count()!==1||await page.locator('[data-record-ref]').getAttribute('data-record-ref')!==selected)throw Error('exact metadata filter ignored reference');
 await page.locator('.database-links').getByRole('link',{name:/^Execution/}).first().click();
 if(!(await page.locator('[data-record-ref]').getAttribute('data-record-ref')).startsWith('run:'))throw Error('message did not link to its actual execution');
 await page.getByRole('link',{name:'Inspect execution →',exact:true}).click();
 await page.locator('#observability-workbench').waitFor();
 await page.getByRole('navigation',{name:'Developer sections'}).getByRole('link',{name:'Database',exact:true}).click();
 await page.getByRole('navigation',{name:'Record collections'}).getByRole('link',{name:'Memory',exact:true}).click();
 await page.locator('.database-record-ref').first().click();
 if(!(await page.locator('.database-fields').innerText()).includes('encrypted'))throw Error('sealed metadata did not retain its storage form');
 await page.locator('.database-links').getByRole('link',{name:/^Source (message|execution)/}).first().click();
 if(await page.locator('[data-record-ref]').count()!==1)throw Error('Memory source link did not open one authorized record');
 await page.getByRole('navigation',{name:'Record collections'}).getByRole('link',{name:'Artifacts',exact:true}).click();
 await page.locator('.database-record-ref').first().click();
 if(await page.locator('.database-page img').count()||!(await page.locator('.database-fields').innerText()).includes('<img src=x'))throw Error('artifact metadata was executed or lost');
 const text=await page.locator('.database-page').innerText();
 if(/PRIVATE_DATABASE_|sealed:v1:|storage_key|credential_ref/.test(text))throw Error('metadata page disclosed a protected value');
 await page.screenshot({path:evidence+'/database-wide.png',fullPage:true});
 await page.setViewportSize({width:390,height:844});
 await page.goto(url+'/developer/database?collection=memories');
 await page.evaluate(()=>document.fonts.ready);
 const geometry=await page.evaluate(()=>({width:innerWidth,content:document.documentElement.scrollWidth,table:document.querySelector('.obs-table-scroll').clientWidth}));
 if(geometry.content>geometry.width)throw Error('metadata table expanded the mobile page');
 await page.screenshot({path:evidence+'/database-mobile.png',fullPage:true});
 await page.goto(url+'/settings/preferences');
 await page.locator('#interface-language').selectOption('zh-CN');
 await page.getByRole('button',{name:'Save language',exact:true}).click();
 await page.goto(url+'/developer/database?collection=memories');
 if(await page.getByRole('heading',{name:'数据库',exact:true}).count()!==1)throw Error('Database page did not localize');
 const plain=await browser.newContext({javaScriptEnabled:false,viewport:{width:1200,height:850}});
 try{
  await plain.addCookies(await context.cookies());
  const native=await plain.newPage();
  await native.goto(url+'/developer/database?collection=messages');
  await native.getByLabel('确切记录引用',{exact:true}).fill(selected);
  await native.getByRole('button',{name:'查找记录',exact:true}).click();
  if(await native.locator('[data-record-ref]').count()!==1)throw Error('no-script metadata search failed');
  await native.locator('.database-links a').first().click();
  if(!native.url().includes('/developer/database?'))throw Error('no-script relationship navigation failed');
 }finally{await plain.close()}
 await writeFile(evidence+'/database-contract.json',JSON.stringify({collections:4,firstPage:firstRefs.length,secondPage:secondRefs.length,exactReference:true,causalLinks:true,encryptedWithoutKey:true,localized:true,noJavaScript:true,mobile:geometry},null,2));
}
