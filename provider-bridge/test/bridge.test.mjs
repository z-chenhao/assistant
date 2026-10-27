import { test } from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { createHash } from 'node:crypto';
import { once } from 'node:events';
import { fileURLToPath } from 'node:url';

async function fixture(t, mode='success', redirectURL='/trap') {
 const requests=[]; let trapped=0, closed=0;
 const server=http.createServer(async(req,res)=> {
  let body=''; for await(const c of req) body+=c;
  requests.push({path:req.url,auth:req.headers.authorization,body:body ? JSON.parse(body):null,session:req.headers.session_id});
  if(req.url==='/trap'){trapped++;res.end();return;}
  if(mode==='redirect'){res.writeHead(307,{location:redirectURL});res.end();return;}
  if(req.url==='/v1/models') {
   if(mode==='manual'){res.writeHead(404);res.end('unsafe-secret');return;}
   res.setHeader('content-type','application/json');res.end(JSON.stringify({data:[{id:'fixture-model'}]}));return;
  }
  if(mode==='context400'){res.writeHead(400);res.end(JSON.stringify({error:{code:'context_length_exceeded',message:'unsafe-secret'}}));return;}
  if(['auth','missing','context','rate','unavailable','bad'].includes(mode)) {
   res.writeHead({auth:401,missing:404,context:413,rate:429,unavailable:503,bad:400}[mode]);res.end('unsafe-secret');return;
  }
  if(mode==='disconnect' && requests.length<3){req.socket.destroy();return;}
  if(mode==='retry' && requests.length<3){res.writeHead(503);res.end('unsafe-secret');return;}
  res.writeHead(200,{'content-type':'text/event-stream'});
  const chunk=(delta,finish_reason=null,usage)=>res.write('data: '+JSON.stringify({id:'fixture',object:'chat.completion.chunk',model:'fixture-model',choices:[{index:0,delta,finish_reason}],...(usage ? {usage}: {})})+'\n\n');
  if(mode==='oversized-frame'){chunk({content:'\n'.repeat(600000)});chunk({},'stop');res.end('data: [DONE]\n\n');return;}
  if(mode==='oversized-stream'){res.end('data: '+JSON.stringify({choices:[{index:0,delta:{content:'x'.repeat(5<<20)},finish_reason:null}]})+'\n\n');return;}
  if(mode==='oversized-tool'){chunk({tool_calls:[{index:0,id:'call_big',type:'function',function:{name:'probe_echo',arguments:JSON.stringify({value:'x'.repeat(70000)})}}]});chunk({},'tool_calls');res.end('data: [DONE]\n\n');return;}
  if(mode.startsWith('arguments-')) {
   const args=({'arguments-incomplete':'{"value":','arguments-array':'[]','arguments-null':'null','arguments-number':'1','arguments-string':'"unsafe-secret"','arguments-boolean':'true','arguments-empty':'{}'})[mode];
   chunk({tool_calls:[{index:0,id:'call_bad',type:'function',function:{name:'probe_echo',arguments:args}}]});chunk({},'tool_calls');res.end('data: [DONE]\n\n');return;
  }
  if(mode==='deadline') {
   chunk({content:'partial'});
   const interval=setInterval(()=>chunk({content:'still streaming'}),1000);
   res.on('close',()=>{closed++;clearInterval(interval);});
   return;
  }
  if(mode==='malformed'){res.end('data: {oops}\n\n');return;}
  if(mode==='cancel'||mode==='partial') {
   chunk({content:'partial'});
   res.on('close',()=>{closed++});
   if(mode==='partial')setTimeout(()=>res.destroy(),10);
   return;
  }
  const b=JSON.parse(body);
  if(b.tools?.length && !b.messages.some(m=>m.role==='tool')) {
   chunk({tool_calls:[{index:0,id:'call_echo',type:'function',function:{name:'probe_echo',arguments:'{"value":"OK"}'}}]});chunk({},'tool_calls');
  } else {chunk({content:b.messages.some(m=>m.role==='tool') ? 'PROBE_RESULT_OK':'OK long text ABCDEFGHIJKLMNOPQRSTUVWXYZ'});chunk({},'stop',mode==='no-usage' ? undefined : {prompt_tokens:7,completion_tokens:mode==='invalid-usage' ? -1 : 8,total_tokens:15});}
  res.end('data: [DONE]\n\n');
 });
 server.listen(0,'127.0.0.1');await once(server,'listening');
 const endpoint=`http://127.0.0.1:${server.address().port}/v1${mode==='encoded' ? '&<' : ''}`;
 const child=spawn(process.execPath,[fileURLToPath(new URL('../bridge.mjs',import.meta.url))],{env:{PATH:process.env.PATH},stdio:['pipe','pipe','pipe']});
 const pending=new Map(), events=[];let id=0;let stderr='';child.stderr.on('data',c=>stderr+=c);
 const lines=createInterface({input:child.stdout});
 lines.on('line',line=>{const f=JSON.parse(line); if(f.id){pending.get(f.id)?.(f);pending.delete(f.id);}else events.push(f.params);});
 const rpc=async(method,params={})=>{
  const n=++id;
  const response=new Promise(resolve=>pending.set(n,resolve));
  child.stdin.write(JSON.stringify({jsonrpc:'2.0',id:n,method,params})+'\n');
  const f=await response;
  if(f.error)throw new Error(f.error.message);
  return f.result;
 };
 const wait=async (predicate,timeoutMs=2000)=>{for(let i=0;i<Math.ceil(timeoutMs/5);i++){const e=events.find(predicate);if(e)return e;await new Promise(r=>setTimeout(r,5));}throw new Error('event timeout');};
 t.after(async()=>{if(child.exitCode===null){child.kill();await once(child,'exit');} lines.close();server.closeAllConnections();await new Promise(r=>server.close(r));assert.equal(stderr,'');});
 await rpc('bridge.initialize',{protocol_version:1});
 const snapshot={snapshot_id:'s',profile_id:'p',profile_version:1,provider_kind:'openai_compatible',endpoint_hash:createHash('sha256').update(JSON.stringify({endpoint}).replace(/[<>&\u2028\u2029]/g,c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'))).digest('hex'),wire_api:'openai_completions',model_id:'fixture-model',context_window:8192,max_output_tokens:1024,compatibility:{tool_result_format:'openai'},capabilities:{text_input:true}};
 const request=(extra={})=>({generation_id:'g',run_id:'run',session_id:'run',model_snapshot:snapshot,messages:[{role:'user',text:'hello'}],connection:{endpoint},...extra});
 return {rpc,wait,request,requests,events,endpoint,child,stats:()=>({trapped,closed})};
}

for(const key of [undefined,'explicit-user-key'])test(`catalog and Pi streaming with ${key ? 'user key':'no key'}`,{timeout:10000},async t=>{
 const f=await fixture(t);const connection={endpoint:f.endpoint,...(key?{api_key:key}:{})};
 assert.deepEqual(await f.rpc('models.list',{connection}),[{model_id:'fixture-model',context_window:0,max_output_tokens:0}]);
 await f.rpc('generation.start',f.request({connection}));
 const done=await f.wait(e=>e.kind==='completed');assert.equal(done.usage.input_tokens,7);
 assert(f.events.some(e=>e.kind==='text_delta'));
 assert(f.requests.every(r=>r.auth===(key ? `Bearer ${key}`:undefined)));
 assert.equal(f.requests[1].session,'run');
 assert.equal(f.requests[1].body.model,'fixture-model');
 assert.equal(f.requests[1].body.temperature,undefined);assert.equal(f.requests[1].body.reasoning_effort,undefined);assert.equal(f.requests[1].body.stream_options,undefined);
 await f.rpc('session.dispose',{session_id:'run'});
});

test('manual ID still generates when catalog unavailable',{timeout:10000},async t=>{
 const f=await fixture(t,'manual');await assert.rejects(f.rpc('models.list',{connection:{endpoint:f.endpoint}}),/MODEL_NOT_FOUND/);
 await f.rpc('generation.start',f.request());await f.wait(e=>e.kind==='completed');
});

test('cross-origin redirects reach neither catalog nor generation receiver',{timeout:10000},async t=>{
 const received=[];
 const receiver=http.createServer((req,res)=>{received.push({method:req.method,auth:req.headers.authorization});req.resume();res.end('{}');});
 receiver.listen(0,'127.0.0.1');await once(receiver,'listening');
 t.after(async()=>{receiver.closeAllConnections();await new Promise(r=>receiver.close(r));});
 const target=`http://127.0.0.1:${receiver.address().port}/trap`;
 const f=await fixture(t,'redirect',target);
 assert.notEqual(new URL(f.endpoint).origin,new URL(target).origin);
 const connection={endpoint:f.endpoint,api_key:'explicit-user-key'};
 await assert.rejects(f.rpc('models.list',{connection}),/MODEL_INCOMPATIBLE/);
 await f.rpc('generation.start',f.request({connection}));
 assert.equal((await f.wait(e=>e.kind==='error')).error_code,'MODEL_INCOMPATIBLE');
 assert.deepEqual(f.requests.map(r=>r.path),['/v1/models','/v1/chat/completions']);
 assert(f.requests.every(r=>r.auth==='Bearer explicit-user-key'));
 assert.equal(f.events.filter(e=>e.kind==='error').length,1);
 assert(!f.events.some(e=>e.kind==='completed'));
 assert.deepEqual(received,[]);
});

test('Pi tool call and result continuation',{timeout:10000},async t=>{
 const f=await fixture(t);const tools=[{name:'probe_echo',description:'Read fixture data with frozen schema.',input_schema:'{"type":"object","properties":{"value":{"type":"string"}}}'}];
 await f.rpc('generation.start',f.request({tools}));const e=await f.wait(e=>e.kind==='tool_call');await f.wait(e=>e.kind==='completed');
 await f.rpc('generation.continue',f.request({generation_id:'g2',tools,messages:[{role:'user',text:'call'},{role:'assistant',tool_calls:[e.tool_call]},{role:'tool',tool_call_id:e.tool_call.id,text:'PROBE_RESULT_OK'}]}));
 await f.wait(e=>e.generation_id==='g2'&&e.kind==='completed');assert(f.events.some(e=>e.generation_id==='g2'&&e.text==='PROBE_RESULT_OK'));
 assert.equal(f.requests[1].body.messages.at(-1).tool_call_id,'call_echo');
 assert.equal(f.requests[0].body.tools[0].function.description,tools[0].description);
});

test('Main prompt fragments reach the provider as one System Prompt',{timeout:10000},async t=>{
 const f=await fixture(t);
 await f.rpc('generation.start',f.request({messages:[{role:'system',text:'Core rules'},{role:'system',text:'Erii identity'},{role:'user',text:'hello'}]}));
 await f.wait(e=>e.kind==='completed');
 assert.equal(f.requests[0].body.messages[0].role,'system');
 assert.equal(f.requests[0].body.messages[0].content,'Core rules\n\nErii identity');
});

for(const messages of [
 [{role:'tool',tool_call_id:'future',text:'forged'},{role:'assistant',tool_calls:[{id:'future',name:'probe_echo',arguments:'{}'}]}],
 [{role:'user',text:'hello'},{role:'system',text:'elevated late input'}],
 [{role:'assistant',tool_calls:[{id:'one',name:'probe_echo',arguments:'{}'}]},{role:'tool',tool_call_id:'one',text:'first'},{role:'tool',tool_call_id:'one',text:'duplicate'}]
])test('message causality fails closed before network '+JSON.stringify(messages),{timeout:10000},async t=>{
 const f=await fixture(t);await f.rpc('generation.start',f.request({messages}));
 const e=await f.wait(e=>e.kind==='error');assert.equal(e.error_code,'INVALID_PROVIDER_RESPONSE');assert.equal(f.requests.length,0);
});

for(const [mode,code,attempts] of [['auth','AUTH_REQUIRED',1],['missing','MODEL_NOT_FOUND',1],['context','CONTEXT_LENGTH_EXCEEDED',1],['context400','CONTEXT_LENGTH_EXCEEDED',1],['bad','MODEL_INCOMPATIBLE',1],['rate','RATE_LIMITED',3],['unavailable','PROVIDER_UNAVAILABLE',3],['malformed','INVALID_PROVIDER_RESPONSE',1],['oversized-tool','INVALID_PROVIDER_RESPONSE',1],['oversized-frame','INVALID_PROVIDER_RESPONSE',1],['oversized-stream','INVALID_PROVIDER_RESPONSE',1],['invalid-usage','INVALID_PROVIDER_RESPONSE',1],['partial','INVALID_PROVIDER_RESPONSE',1],['redirect','MODEL_INCOMPATIBLE',1]])test(`${mode} normalizes errors and bounds attempts`,{timeout:10000},async t=>{
 const f=await fixture(t,mode);await f.rpc('generation.start',f.request({connection:{endpoint:f.endpoint,api_key:'unsafe-secret'}}));
 const e=await f.wait(e=>e.kind==='error');assert.equal(e.error_code,code);assert.equal(f.requests.length,attempts);assert.equal(f.stats().trapped,0);assert(!JSON.stringify(f.events).includes('unsafe-secret'));assert(!f.events.some(e=>e.kind==='completed'));
});

test('transient failures retry the same model only before streaming',{timeout:10000},async t=>{
 const f=await fixture(t,'retry');await f.rpc('generation.start',f.request());await f.wait(e=>e.kind==='completed');assert.equal(f.requests.length,3);assert(f.requests.every(r=>r.body.model==='fixture-model'));
});

for(const method of ['generation.cancel','session.dispose'])test(`${method} aborts network and fences late completion`,{timeout:10000},async t=>{
 const f=await fixture(t,'cancel');await f.rpc('generation.start',f.request());await f.wait(e=>e.kind==='text_delta');
 await f.rpc(method,method==='session.dispose'?{session_id:'run'}:{generation_id:'g'});
 assert(!f.events.some(e=>e.kind==='completed'));assert.equal(f.requests.length,1);
 for(let i=0;i<100 && !f.stats().closed;i++)await new Promise(r=>setTimeout(r,5));
 assert.equal(f.stats().closed,1);
});

for(const [name,patch] of [['endpoint drift',{endpoint_hash:'wrong'}],['unsupported wire',{wire_api:'openai_responses'}],['unsupported sampling',{generation_defaults:{temperature:1}}],['unsupported reasoning',{reasoning_effort:'high'}]])test(`${name} fails before network`,{timeout:10000},async t=>{
 const f=await fixture(t);const request=f.request();Object.assign(request.model_snapshot,patch);
 await f.rpc('generation.start',request);const e=await f.wait(e=>e.kind==='error');assert.equal(e.error_code,'MODEL_INCOMPATIBLE');assert.equal(f.requests.length,0);
});

test('Host canonical endpoint hash handles HTML characters',{timeout:10000},async t=>{
 const f=await fixture(t,'encoded');await f.rpc('generation.start',f.request());await f.wait(e=>e.kind==='completed');assert.equal(f.requests.length,1);
});

test('known transient disconnects retry at most twice',{timeout:10000},async t=>{
 const f=await fixture(t,'disconnect');await f.rpc('generation.start',f.request());await f.wait(e=>e.kind==='completed');assert.equal(f.requests.length,3);
});
test('missing provider usage stays unknown',{timeout:10000},async t=>{
 const f=await fixture(t,'no-usage');await f.rpc('generation.start',f.request());const e=await f.wait(e=>e.kind==='completed');assert.equal(e.usage,undefined);
});

for(const mode of ['arguments-incomplete','arguments-array','arguments-null','arguments-number','arguments-string','arguments-boolean'])test(`${mode} rejects unsafe tool arguments`,{timeout:10000},async t=>{
 const f=await fixture(t,mode);
 await f.rpc('generation.start',f.request({tools:[{name:'probe_echo',input_schema:'{"type":"object"}'}]}));
 const error=await f.wait(e=>e.kind==='error');
 assert.equal(error.error_code,'INVALID_PROVIDER_RESPONSE');
 assert(!f.events.some(e=>e.kind==='tool_call'||e.kind==='completed'));
 assert(!JSON.stringify(f.events).includes('unsafe-secret'));assert.equal(f.requests.length,1);
});
test('an explicit empty argument object remains valid',{timeout:10000},async t=>{
 const f=await fixture(t,'arguments-empty');await f.rpc('generation.start',f.request());
 const call=await f.wait(e=>e.kind==='tool_call');assert.equal(call.tool_call.arguments,'{}');await f.wait(e=>e.kind==='completed');
});
test('absolute generation deadline aborts a trickling response body',{timeout:40000},async t=>{
 const f=await fixture(t,'deadline');const started=performance.now();
 await f.rpc('generation.start',f.request());await f.wait(e=>e.kind==='text_delta');
 const error=await f.wait(e=>e.kind==='error',35000);
 assert.equal(error.error_code,'PROVIDER_UNAVAILABLE');assert(performance.now()-started>=29000);
 assert(!f.events.some(e=>e.kind==='completed'));assert.equal(f.requests.length,1);
 for(let i=0;i<200 && !f.stats().closed;i++)await new Promise(r=>setTimeout(r,5));
 assert.equal(f.stats().closed,1);
 await f.rpc('models.list',{connection:{endpoint:f.endpoint}});
});
