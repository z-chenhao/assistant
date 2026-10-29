// One scenario module for the maintained channels browser driver; no runner,
// browser lifecycle, fake DOM or alternate product endpoint lives here.
export async function capabilitySetup(page,context,url,evidence){
 const facts=JSON.parse(process.env.COLLIE_CAPABILITY_SETUP_FACTS);
 const open=async id=>{await page.goto(url+'/settings/capabilities/'+id);await page.locator('#capability-configuration').waitFor()};
 const test=async text=>{
  await page.locator('#capability-configuration').fill(typeof text==='string'?text:JSON.stringify(text,null,2));
  await Promise.all([page.waitForURL(url+'/settings/capabilities/*'),page.getByRole('button',{name:'Test configuration',exact:true}).click()]);
  if(!new URL(page.url()).pathname.startsWith('/settings/capabilities/'))throw Error('capability test lost canonical route');
 };
 const save=async()=>{await page.getByRole('button',{name:'Save verified configuration',exact:true}).click();if(await page.getByText('Unsaved candidate.',{exact:false}).count())throw Error('candidate was not persisted')};
 await page.goto(url+'/settings/models');await page.getByRole('link',{name:'Capabilities',exact:true}).click();
 if(await page.locator('.object-row').count()!==6)throw Error('missing configured capability kinds');
 await page.screenshot({path:evidence+'/capabilities-before.png',fullPage:true});
 await open('memory');
 if(!await page.getByRole('button',{name:'Save verified configuration',exact:true}).isDisabled())throw Error('untested candidate can be saved');
 await test('encryption_key_ref: env:COLLIE_MISSING_SETUP_KEY\n');
 await page.getByRole('status').filter({hasText:'Memory key unavailable'}).waitFor();
 if(!await page.getByRole('button',{name:'Save verified configuration',exact:true}).isDisabled())throw Error('failed candidate can be saved');
 await test('encryption_key_ref: env:COLLIE_SETUP_MEMORY_KEY\n');
 await page.getByText('The referenced 32-byte encryption key is available.',{exact:false}).waitFor();
 await page.reload();if(!await page.locator('#capability-configuration').inputValue().then(v=>v.includes('COLLIE_SETUP_MEMORY_KEY')))throw Error('PRG lost tested draft');
 await save();await page.getByRole('status').filter({hasText:'Restart Collie to load'}).waitFor();
 await page.screenshot({path:evidence+'/memory-saved.png',fullPage:true});
 await open('embedding');
 await test({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:2});
 await page.getByRole('status').filter({hasText:'Embedding connection failed'}).waitFor();
 await test({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:5});
 await page.getByText('Embedding endpoint returned a valid 5-dimensional vector',{exact:false}).waitFor();await save();
 await open('skills');await test({root:facts.skills+'/missing',scope:'agent'});await page.getByText('Skill catalog unavailable.',{exact:false}).waitFor();
 await test({root:facts.skills,scope:'agent'});await page.getByText('actual Skill catalog',{exact:false}).waitFor();await save();
 await open('mcp');
 const provider={name:'owner_catalog',command:facts.binary,args:['-test-mcp-server',facts.provider],directory:facts.provider,tools:{not_advertised:'read_only'}};
 await test([provider]);await page.getByText('MCP discovery failed.',{exact:false}).waitFor();
 provider.tools={read:'read_only',send:'external_effect'};await test([provider]);await page.getByText('2 allowed tools',{exact:false}).waitFor();await save();
 await open('execution');await test({root:facts.workspace,toolchain_roots:[]});await page.getByText('fixed diagnostic command completed',{exact:false}).waitFor();await save();
 // Restore the implicit workspace so Computer can be the sole saved backend.
 await test('');await page.getByText('fixed diagnostic command completed',{exact:false}).waitFor();await save();
 await open('computer');await test(facts.computer);await page.getByText('Lume version, sealed base and credential availability verified.',{exact:false}).waitFor();
 await page.screenshot({path:evidence+'/computer-tested.png',fullPage:true});await save();
 await page.goto(url+'/settings/capabilities');
 if(await page.getByText('Restart required to load saved changes',{exact:true}).count()!==6)throw Error('saved configuration changes are not distinguished from running state');
 await page.screenshot({path:evidence+'/capabilities-configured.png',fullPage:true});
 await page.setViewportSize({width:320,height:740});
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth))throw Error('capabilities overflowed a compact viewport');
 await page.screenshot({path:evidence+'/capabilities-mobile.png',fullPage:true});
 const plain=await page.context().browser().newContext({javaScriptEnabled:false,viewport:{width:390,height:844}});
 try{await plain.addCookies(await context.cookies());const p=await plain.newPage();await p.goto(url+'/settings/capabilities/embedding');await p.locator('#capability-configuration').fill('provider: openai_compatible\nendpoint: '+facts.embedding+'\nmodel: semantic-fixture-v1\ndimensions: 5\n');await p.getByRole('button',{name:'Test configuration',exact:true}).click();await p.getByText('valid 5-dimensional vector',{exact:false}).waitFor();await p.screenshot({path:evidence+'/capability-no-script.png',fullPage:true})}finally{await plain.close()}
}
