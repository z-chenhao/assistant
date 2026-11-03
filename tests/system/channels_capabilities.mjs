// One scenario module for the maintained channels browser driver; no runner,
// browser lifecycle, fake DOM or alternate product endpoint lives here.
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
  await guided().getByRole('button',{name:'Test settings',exact:true}).click();
  await page.waitForURL(url+'/settings/capabilities/*');
  if(new URL(page.url()).searchParams.has('editor'))throw Error('guided test switched to Advanced YAML');
 };
 const sourcePath=()=>page.locator('details.technical-details').filter({has:page.getByText('Configuration source and lifecycle',{exact:true})}).locator('code').textContent();
 const test=async text=>{
  await advanced(page);
  await page.locator('#capability-configuration').fill(typeof text==='string'?text:JSON.stringify(text,null,2));
  await Promise.all([page.waitForURL(url+'/settings/capabilities/*'),page.getByRole('button',{name:'Test configuration',exact:true}).click()]);
  if(!new URL(page.url()).pathname.startsWith('/settings/capabilities/')||new URL(page.url()).searchParams.get('editor')!=='advanced'||!await page.locator('details.capability-advanced').evaluate(node=>node.open))throw Error('advanced test lost its canonical editor route');
 };
 const save=async()=>{await page.getByRole('button',{name:'Save verified configuration',exact:true}).click();if(await page.getByText('Unsaved candidate.',{exact:false}).count())throw Error('candidate was not persisted')};
 await page.goto(url+'/settings/models');await page.getByRole('link',{name:'Capabilities',exact:true}).click();
 if(await page.locator('.object-row').count()!==6)throw Error('missing configured capability kinds');
 await page.screenshot({path:evidence+'/capabilities-before.png',fullPage:true});
 // Start with the ordinary form: no YAML or provider protocol knowledge is
 // needed to enable Memory, repair a failed probe, retest edits and save.
 await page.goto(url+'/settings/capabilities/memory');
 if(!await guided().isVisible()||await page.locator('details.capability-advanced').evaluate(node=>node.open))throw Error('Memory did not lead with its guided form');
 const initialRevision=await guided().locator('input[name="revision"]').inputValue(),initialPath=await sourcePath();
 if(!await guided().getByRole('button',{name:'Save tested settings',exact:true}).isDisabled())throw Error('untested guided settings can be saved');
 await guided().getByRole('checkbox',{name:'Enable this capability',exact:true}).check();
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_MISSING_SETUP_KEY');
 await guidedTest();
 await page.getByRole('status').filter({hasText:'Memory key unavailable'}).waitFor();
 if(!await guided().getByRole('button',{name:'Save tested settings',exact:true}).isDisabled()||await guided().getByLabel('Encryption key reference',{exact:true}).inputValue()!=='env:STEWARD_MISSING_SETUP_KEY'||await guided().locator('input[name="revision"]').inputValue()!==initialRevision||await sourcePath()!==initialPath)throw Error('failed guided probe lost its fields, revision or source path');
 await page.screenshot({path:evidence+'/memory-guided-failed.png',fullPage:true});
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_SETUP_MEMORY_KEY');
 await guidedTest();await page.getByText('The referenced 32-byte encryption key is available.',{exact:false}).waitFor();
 if(await guided().getByRole('button',{name:'Save tested settings',exact:true}).isDisabled())throw Error('successful guided probe did not permit explicit save');
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_MISSING_SETUP_KEY');
 if(!await guided().getByRole('button',{name:'Save tested settings',exact:true}).isDisabled())throw Error('editing guided settings reused a verification proof');
 await guided().getByRole('status').filter({hasText:'Settings changed. Test again before saving.'}).waitFor();
 await guided().getByLabel('Encryption key reference',{exact:true}).fill('env:STEWARD_SETUP_MEMORY_KEY');
 if(!await guided().getByRole('button',{name:'Save tested settings',exact:true}).isDisabled())throw Error('restoring a field silently reenabled Save without retesting');
 await guidedTest();await page.getByText('The referenced 32-byte encryption key is available.',{exact:false}).waitFor();
 await page.screenshot({path:evidence+'/memory-guided-tested.png',fullPage:true});
 await guided().getByRole('button',{name:'Save tested settings',exact:true}).click();
 await page.getByRole('status').filter({hasText:'Restart Steward to load'}).waitFor();
 if(new URL(page.url()).pathname!=='/settings/capabilities/memory'||await guided().getByLabel('Encryption key reference',{exact:true}).inputValue()!=='env:STEWARD_SETUP_MEMORY_KEY'||await guided().locator('input[name="revision"]').inputValue()===initialRevision||await sourcePath()!==initialPath)throw Error('guided save lost fields, source path or configuration revision');
 await page.screenshot({path:evidence+'/memory-guided-saved.png',fullPage:true});
 // Keep the original Advanced YAML repair/probe/save coverage as well.
 await open('memory');
 await test('encryption_key_ref: env:STEWARD_MISSING_SETUP_KEY\n');
 await page.getByRole('status').filter({hasText:'Memory key unavailable'}).waitFor();
 if(!await page.getByRole('button',{name:'Save verified configuration',exact:true}).isDisabled())throw Error('failed candidate can be saved');
 await test('encryption_key_ref: env:STEWARD_SETUP_MEMORY_KEY\n');
 await page.getByText('The referenced 32-byte encryption key is available.',{exact:false}).waitFor();
 await page.reload();if(!await page.locator('#capability-configuration').inputValue().then(v=>v.includes('STEWARD_SETUP_MEMORY_KEY')))throw Error('PRG lost tested draft');
 await save();await page.getByRole('status').filter({hasText:'Restart Steward to load'}).waitFor();
 await page.screenshot({path:evidence+'/memory-saved.png',fullPage:true});
 await open('embedding');
 await test({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:2});
 await page.getByRole('status').filter({hasText:'Embedding connection failed'}).waitFor();
 await test({provider:'openai_compatible',endpoint:facts.embedding,model:'semantic-fixture-v1',dimensions:5});
 await page.getByText('Embedding endpoint returned a valid 5-dimensional vector',{exact:false}).waitFor();await save();
 await open('skills');await test({root:facts.skills+'/missing',scope:'agent'});await page.getByText('Skill catalog unavailable.',{exact:false}).waitFor();
 await test({root:facts.skills,scope:'agent'});await page.getByText('actual Skill catalog',{exact:false}).waitFor();await save();
 // Repeated provider/tool controls use native form actions. Preserve the
 // supplied fixture paths and exact arguments through add/remove and Test/Save.
 await page.goto(url+'/settings/capabilities/mcp');
 await guided().getByRole('checkbox',{name:'Enable this capability',exact:true}).check();
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
 await guidedTest();await page.getByText('2 allowed tools',{exact:false}).waitFor();
 if(await guided().getByLabel('Tool name',{exact:true}).count()!==2||await guided().getByLabel('Tool name',{exact:true}).nth(0).inputValue()!=='send'||await guided().getByLabel('Tool name',{exact:true}).nth(1).inputValue()!=='read'||await guided().getByLabel('Permission',{exact:false}).nth(0).inputValue()!=='external_effect')throw Error('guided MCP test changed its allowlist or permissions');
 await guided().getByRole('button',{name:'Save tested settings',exact:true}).click();
 if(await page.getByText('Unsaved candidate.',{exact:false}).count()||await guided().getByLabel('Command arguments (optional)',{exact:false}).inputValue()!=='-test-mcp-server\n'+facts.provider)throw Error('guided MCP save failed exact-candidate verification');
 await page.screenshot({path:evidence+'/mcp-guided-saved.png',fullPage:true});
 await open('mcp');
 const provider={name:'owner_catalog',command:facts.binary,args:['-test-mcp-server',facts.provider],directory:facts.provider,tools:{not_advertised:'read_only'}};
 await test([provider]);await page.getByText('MCP discovery failed.',{exact:false}).waitFor();
 provider.tools={read:'read_only',send:'external_effect'};await test([provider]);await page.getByText('2 allowed tools',{exact:false}).waitFor();await save();
 // Verify the native fallback before platform execution diagnostics, so an
 // unavailable kernel workspace does not hide form accessibility evidence.
 const plain=await page.context().browser().newContext({javaScriptEnabled:false,viewport:{width:390,height:844}});
 try{
  await plain.addCookies(await context.cookies());const p=await plain.newPage();
  await p.goto(url+'/settings/capabilities/embedding');
  const form=p.locator('form.capability-guided');
  if(!await form.isVisible()||await p.locator('details.capability-advanced').evaluate(node=>node.open)||await form.getByLabel('Endpoint URL',{exact:true}).inputValue()!==facts.embedding||await form.getByLabel('Vector dimensions',{exact:true}).inputValue()!=='5')throw Error('no-JavaScript guided form lost saved fields');
  await form.getByRole('button',{name:'Test settings',exact:true}).click();
  await p.getByText('valid 5-dimensional vector',{exact:false}).waitFor();
  await form.getByRole('button',{name:'Save tested settings',exact:true}).click();
  if(await p.getByText('Unsaved candidate.',{exact:false}).count())throw Error('no-JavaScript guided settings were not saved');
  await p.screenshot({path:evidence+'/capability-guided-no-script.png',fullPage:true});
  await advanced(p);
  await p.locator('#capability-configuration').fill('provider: openai_compatible\nendpoint: '+facts.embedding+'\nmodel: semantic-fixture-v1\ndimensions: 5\n');
  await p.getByRole('button',{name:'Test configuration',exact:true}).click();
  await p.getByText('valid 5-dimensional vector',{exact:false}).waitFor();
  if(new URL(p.url()).searchParams.get('editor')!=='advanced'||!await p.locator('details.capability-advanced').evaluate(node=>node.open))throw Error('no-JavaScript advanced PRG lost its editor');
  await p.screenshot({path:evidence+'/capability-no-script.png',fullPage:true});
 }finally{await plain.close()}
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

}
