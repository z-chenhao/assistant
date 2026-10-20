// Explicit L1 browser peer. Fake Lume/SSH; real Chromium, DOM and production
// Browser adapter. No model-supplied executable, script, CDP endpoint or path.
import {chromium} from '../guest-browser/node_modules/playwright-core/index.mjs';
import {createServer} from 'node:http';
import {readFile,writeFile,mkdir,symlink,copyFile,realpath} from 'node:fs/promises';
import {join,resolve} from 'node:path';
import {fileURLToPath} from 'node:url';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
const exec=promisify(execFile);
const root=await realpath(process.argv[2]);
const repository=resolve(fileURLToPath(new URL('..',import.meta.url)));
const driver=await readFile(join(repository,'internal/computer/macosvm/browser.mjs'),'utf8');
const world=JSON.parse(await readFile(join(root,'world.json'),'utf8'));
const state={orders:[],submissions:0,network:{ethernet:'Not Connected',gateway:false,ordinary:false,proxy:false,apple_tv:false,protocols:1},clips:[]};
await mkdir(join(root,'playwright'));
const module=join(root,'playwright/index.mjs');
for(const file of ['index.mjs','package.json'])await symlink(join(repository,'guest-browser/node_modules/playwright-core',file),join(root,'playwright',file));
await copyFile(join(repository,'internal/computer/macosvm/transfer.py'),join(root,'playwright/erii-transfer.py'));
const browser=await chromium.launchPersistentContext(join(root,'profile'),{headless:true,args:['--remote-debugging-address=127.0.0.1','--remote-debugging-port=0']});
const port=Number((await readFile(join(root,'profile/DevToolsActivePort'),'utf8')).split('\n')[0]);
const persist=()=>writeFile(join(root,'external-state.json'),JSON.stringify(state));
await persist();
const html=body=>'<!doctype html><meta charset="utf-8"><title>Erii M6 sandbox</title><style>body{font:18px system-ui;margin:40px}button,input{font:inherit;margin:12px;padding:8px}pre{white-space:pre-wrap}</style>'+body;
const server=createServer(async(req,res)=>{
 try{
  let body='';for await(const chunk of req){body+=chunk;if(body.length>1<<20)throw new Error('frame too large');}
  if(req.url==='/call'){
   const {method,args={}}=JSON.parse(body);
   const result=JSON.parse((await exec(process.execPath,['--input-type=module','-e',driver,JSON.stringify({module,port,root:join(root,'transfer'),method,args})],{timeout:15000,maxBuffer:12<<20})).stdout);
   // Disconnect exactly after the real submit reached the external state.
   // This models an SSH/Driver acknowledgement lost after an accepted action.
   if(method==='playwright.click'&&args.selector==='#submit'&&!result.error){
    if(state.orders.length!==1)throw new Error('submit did not commit');
    res.destroy();return;
   }
   res.setHeader('Content-Type','application/json');res.end(JSON.stringify(result));return;
  }
  if(req.url==='/book'){
   const form=new URLSearchParams(body);
   const request={restaurant:form.get('restaurant'),date:form.get('date'),time:form.get('time'),party:Number(form.get('party'))};state.submissions++;
   if(request.restaurant!=='R1'||request.date!==world.request.date||request.time!==world.request.time||request.party!==2)throw new Error('wrong frozen booking');
   state.orders.push({order_id:'order_781',...request});await persist();
   // The page performs a synchronous form navigation. The Driver returns
   // after dispatch; /call above drops its transport acknowledgement.
   res.end(html('<h1>Order recorded</h1>'));return;
  }
  if(req.url==='/orders'){res.end(html('<pre id="orders">'+JSON.stringify(state.orders)+'</pre>'));return;}
  if(req.url==='/network/repair'){
   state.network={ethernet:'Connected',gateway:true,ordinary:true,proxy:true,apple_tv:true,protocols:1};await persist();res.end(html('<h1>Network repaired</h1>'));return;
  }
  if(req.url==='/network'){res.end(html('<pre id="network">'+JSON.stringify(state.network)+'</pre><button id="repair" onclick="location.href=\'/network/repair\'">Reconnect Ethernet; one protocol; interrupts gateway temporarily</button>'));return;}
  if(req.url==='/clips'){
   const media=['red.mov','blue.mov','third.mov'].map((name,i)=>`<section><video width="320" height="180" controls preload="metadata" src="/media/${name}" onloadedmetadata="document.getElementById('info-${i}').textContent='${name} '+this.duration+'s '+this.videoWidth+'x'+this.videoHeight"></video><div id="info-${i}">${name} loading</div></section>`).join('');
   res.end(html('<h1>Desktop materials</h1>'+media+'<input id="label" aria-label="Clip label"><button id="save" onclick="location.href=\'/clips/save?label=\'+encodeURIComponent(document.querySelector(\'#label\').value)">Save label</button><pre id="clips">'+JSON.stringify(state.clips)+'</pre>'));return;
  }
  if(['/media/red.mov','/media/blue.mov','/media/third.mov'].includes(req.url)){res.setHeader('Content-Type','video/quicktime');res.end(await readFile(join(root,req.url.slice(1))));return;}
  if(req.url?.startsWith('/clips/save?')){state.clips.push(new URL(req.url,'http://localhost').searchParams.get('label'));await persist();res.writeHead(302,{Location:'/clips'});res.end();return;}
  if(req.url==='/booking'){
   res.end(html('<h1>Final confirmation</h1><pre id="confirmation">'+JSON.stringify({restaurant:world.restaurants[0],request:world.request,total_cny:560})+'</pre><form method="POST" action="/book"><input type="hidden" name="restaurant" value="R1"><input type="hidden" name="date" value="2026-10-17"><input type="hidden" name="time" value="19:00"><input type="hidden" name="party" value="2"><button id="submit">Submit 湖滨店 2026-10-17 19:00 2人 560元 无押金</button></form><div id="status">Not submitted</div>'));return;
  }
  res.writeHead(404);res.end();
 }catch(error){console.error(error.message);res.writeHead(500);res.end(JSON.stringify({error:'PROVIDER_UNAVAILABLE'}));}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
console.log(JSON.stringify({endpoint:`http://127.0.0.1:${server.address().port}/call`,url:`http://127.0.0.1:${server.address().port}`}));
let closing=false;
async function close(){if(closing)return;closing=true;await browser.close();await new Promise(resolve=>server.close(resolve));}
process.on('SIGTERM',()=>close().then(()=>process.exit(0)));
process.on('SIGINT',()=>close().then(()=>process.exit(0)));
