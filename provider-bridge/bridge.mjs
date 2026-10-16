// Private protocol v1. Only this module imports Pi; no domain, Tool, or Computer access.
import { normalizeContext } from '@earendil-works/pi-ai/utils/transcript';
import { stream } from '@earendil-works/pi-ai/api/openai-completions';
import { createHash } from 'node:crypto';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';

// The SDK prints malformed provider chunks to console.error; suppress that unsafe sink.
console.error = console.warn = console.log = () => {};
const codes = new Set(['AUTH_REQUIRED', 'MODEL_NOT_FOUND', 'MODEL_INCOMPATIBLE', 'RATE_LIMITED', 'CONTEXT_LENGTH_EXCEEDED', 'PROVIDER_UNAVAILABLE', 'GENERATION_CANCELLED', 'INVALID_PROVIDER_RESPONSE']);
const safe = e => codes.has(e?.message) ? e.message : 'INVALID_PROVIDER_RESPONSE';
const fail = code => { throw new Error(code); };
const hash = endpoint => createHash('sha256').update(JSON.stringify({ endpoint }).replace(/[<>&\u2028\u2029]/g, c=>'\\u'+c.charCodeAt(0).toString(16).padStart(4,'0'))).digest('hex');
const active = new Map();
let initialized = false;

function endpoint(value) {
  const u = new URL(value);
  if (!['http:', 'https:'].includes(u.protocol) || u.username || u.password || u.search || u.hash) fail('MODEL_INCOMPATIBLE');
  return u;
}

// Pi's SDK requires a nonempty constructor key. The transport owns Authorization:
// the SDK-only placeholder is always removed before any network request.
function transport(connection, signal, state) {
  const base = endpoint(connection.endpoint);
  return async (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url ?? input.toString());
    if (u.origin !== base.origin || ![base.pathname.replace(/\/$/, '') + '/chat/completions', base.pathname.replace(/\/$/, '') + '/models'].includes(u.pathname) || u.search || u.hash || u.username || u.password) fail('MODEL_INCOMPATIBLE');
    const headers = new Headers(init.headers);
    headers.delete('authorization');
    if (connection.api_key) headers.set('authorization', `Bearer ${connection.api_key}`);
    for (let attempt = 0; attempt < 3; attempt++) {
      let response;
      try {
        response = await fetch(u, { ...init, headers, signal, redirect: 'manual' });
      } catch (error) {
        if (signal.aborted) fail('GENERATION_CANCELLED');
        if (attempt < 2 && ['ECONNRESET','ECONNREFUSED','EPIPE','ETIMEDOUT','EAI_AGAIN','UND_ERR_CONNECT_TIMEOUT','UND_ERR_SOCKET'].includes(error.cause?.code)) { await delay(50 * 2 ** attempt, undefined, { signal }); continue; }
        state.code = 'PROVIDER_UNAVAILABLE'; fail(state.code);
      }
      if (response.status >= 300 && response.status < 400) {
        await response.body?.cancel(); state.code = 'MODEL_INCOMPATIBLE'; fail(state.code);
      }
      if ([429, 502, 503, 504].includes(response.status) && attempt < 2) {
        await response.body?.cancel(); await delay(50 * 2 ** attempt, undefined, { signal }); continue;
      }
      if (!response.ok) {
        // Inspect only a bounded, allowlisted machine code; never expose the body.
        let machineCode;
        if ([400,422].includes(response.status) && response.body) {
          const reader=response.body.getReader(); let size=0; const chunks=[];
          try { for (;;) {const {value,done}=await reader.read();if(done)break;size+=value.length;if(size>8192)break;chunks.push(value);} if(size<=8192)machineCode=JSON.parse(Buffer.concat(chunks).toString('utf8'))?.error?.code; } catch {} finally {await reader.cancel();}
        } else await response.body?.cancel();
        state.code = ({401:'AUTH_REQUIRED',403:'AUTH_REQUIRED',404:'MODEL_NOT_FOUND',413:'CONTEXT_LENGTH_EXCEEDED',429:'RATE_LIMITED',500:'PROVIDER_UNAVAILABLE',502:'PROVIDER_UNAVAILABLE',503:'PROVIDER_UNAVAILABLE',504:'PROVIDER_UNAVAILABLE'})[response.status] ?? 'MODEL_INCOMPATIBLE';
        if (machineCode === 'context_length_exceeded') state.code='CONTEXT_LENGTH_EXCEEDED';
        fail(state.code);
      }
      // Bound raw SSE before Pi can accumulate a partial JSON/tool argument buffer.
      const reader=response.body.getReader(); let received=0;
      return new Response(new ReadableStream({
        async pull(controller) {
          try {
            const {value,done}=await reader.read();
            if(done){controller.close();return;}
            received+=value.byteLength;
            if(received>4<<20){state.code='INVALID_PROVIDER_RESPONSE';await reader.cancel();controller.error(new Error(state.code));return;}
            controller.enqueue(value);
          } catch {state.code='INVALID_PROVIDER_RESPONSE';controller.error(new Error(state.code));}
        },
        cancel(reason){return reader.cancel(reason);}
      }),{status:response.status,headers:response.headers});
    }
  };
}

const zeroUsage = () => ({ input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input:0, output:0, cacheRead:0, cacheWrite:0, total:0 } });
function context(r, model) {
  const messages = [];
  for (const m of r.messages) {
    if (m.role === 'user') messages.push({ role:'user', content:m.text, timestamp:0 });
    else if (m.role === 'assistant') messages.push({ role:'assistant', content:[...(m.text ? [{type:'text',text:m.text}] : []), ...(m.tool_calls ?? []).map(t => ({type:'toolCall',id:t.id,name:t.name,arguments:JSON.parse(t.arguments)}))], api:model.api, provider:model.provider, model:model.id, usage:zeroUsage(), stopReason:m.tool_calls?.length ? 'toolUse':'stop', timestamp:0 });
    else if (m.role === 'tool') {
      const call = r.messages.flatMap(m => m.tool_calls ?? []).find(t => t.id === m.tool_call_id);
      if (!call) fail('INVALID_PROVIDER_RESPONSE');
      messages.push({ role:'toolResult', toolCallId:m.tool_call_id, toolName:call.name, content:[{type:'text',text:m.text}], isError:false, timestamp:0 });
    } else fail('INVALID_PROVIDER_RESPONSE');
  }
  return { messages, tools:(r.tools ?? []).map(t => ({name:t.name,description:'',parameters:JSON.parse(t.input_schema)})) };
}

async function generate(r, job, send) {
  const started = performance.now();
  let sequence = 0;
  const event = (kind, rest = {}) => { if (!job.cancelled) { send({jsonrpc:'2.0',method:'generation.event',params:{generation_id:r.generation_id,sequence:sequence+1,kind,...rest}}); sequence++; } };
  const s = r.model_snapshot;
  const state = {};
  try {
    if (r.session_id !== r.run_id || !r.run_id || !s.snapshot_id || s.provider_kind !== 'openai_compatible' || s.wire_api !== 'openai_completions' || s.endpoint_hash !== hash(r.connection.endpoint) || s.max_output_tokens < 1 || s.max_output_tokens > 65536 || s.context_window > 1048576) fail('MODEL_INCOMPATIBLE');
    if (s.max_output_tokens > s.context_window || s.context_window < 1 || s.reasoning_effort || Object.keys(s.generation_defaults ?? {}).length || ['developer_role','reasoning_effort','parallel_tool_calls','stream_usage'].some(k=>s.compatibility?.[k]) || s.compatibility?.tool_result_format !== 'openai') fail('MODEL_INCOMPATIBLE');
    const model = {id:s.model_id,name:s.model_id,api:'openai-completions',provider:'erii-local',baseUrl:r.connection.endpoint,reasoning:false,input:['text'],contextWindow:s.context_window,maxTokens:s.max_output_tokens,cost:{input:0,output:0,cacheRead:0,cacheWrite:0},compat:{supportsStore:false,supportsDeveloperRole:false,supportsReasoningEffort:false,supportsUsageInStreaming:false,supportsStrictMode:false,maxTokensField:'max_tokens',sendSessionAffinityHeaders:true,sessionAffinityFormat:'openai',supportsLongCacheRetention:false}};
    // Optional compatibility and sampling stay conservative until individually probed.
    const events = stream(model, normalizeContext(context(r, model)), { apiKey:r.connection.api_key || 'sdk-constructor-only', fetch:transport(r.connection,job.controller.signal,state), signal:job.controller.signal, sessionId:r.session_id, maxTokens:s.max_output_tokens, maxRetries:0, timeoutMs:30000, onProviderStreamEvent:e=>{if(e.usage) {if(!Number.isSafeInteger(e.usage.prompt_tokens) || !Number.isSafeInteger(e.usage.completion_tokens) || e.usage.prompt_tokens<0 || e.usage.completion_tokens<0) fail('INVALID_PROVIDER_RESPONSE');state.usageSeen=true;}} });
    let terminal = false;
    let count = 0, bytes = 0, tools = 0;
    for await (const e of events) {
      if (++count > 4096) fail('INVALID_PROVIDER_RESPONSE');
      if (e.type === 'text_delta') {
        bytes += Buffer.byteLength(e.delta);
        if (bytes > 1 << 20) fail('INVALID_PROVIDER_RESPONSE');
        event('text_delta',{text:e.delta});
      } else if (e.type === 'toolcall_end') {
        const argumentsJSON=JSON.stringify(e.toolCall.arguments);
        if(++tools>32 || Buffer.byteLength(argumentsJSON)>65536) fail('INVALID_PROVIDER_RESPONSE');
        event('tool_call',{tool_call:{id:e.toolCall.id,name:e.toolCall.name,arguments:argumentsJSON}});
      }
      else if (e.type === 'done') {
        terminal = true;
        const u = e.message.usage;
        event('completed',{...(state.usageSeen ? {usage:{input_tokens:u.input,output_tokens:u.output,cache_read_tokens:u.cacheRead,cache_write_tokens:u.cacheWrite}} : {}),latency_ms:Math.ceil(performance.now()-started)});
      } else if (e.type === 'error') {
        terminal = true;
        event('error',{error_code:job.controller.signal.aborted ? 'GENERATION_CANCELLED' : state.code ?? 'INVALID_PROVIDER_RESPONSE'});
      }
    }
    if (!terminal) fail('INVALID_PROVIDER_RESPONSE');
  } catch (e) {
    event('error',{error_code:state.code ?? safe(e)});
  } finally {
    job.controller.abort();
    active.delete(r.generation_id);
    r.connection = undefined;
  }
}

export async function handle(f, send) {
  const reply = result => send({jsonrpc:'2.0',id:f.id,result});
  try {
    if (f.jsonrpc !== '2.0' || !Number.isSafeInteger(f.id) || f.id < 1) fail('INVALID_PROVIDER_RESPONSE');
    const p = f.params ?? {};
    if (f.method === 'bridge.initialize') {
      if (p.protocol_version !== 1) fail('MODEL_INCOMPATIBLE');
      initialized = true; return reply({protocol_version:1});
    }
    if (!initialized) fail('MODEL_INCOMPATIBLE');
    switch (f.method) {
      case 'bridge.shutdown':
        for (const j of active.values()) { j.cancelled=true; j.controller.abort(); }
        await Promise.all([...active.values()].map(j=>j.done));
        return reply({});
      case 'model.probe':
        if (p.provider_kind !== 'openai_compatible' || p.wire_api !== 'openai_completions') fail('MODEL_INCOMPATIBLE');
        endpoint(p.endpoint); return reply({});
      case 'models.list': case 'models.refresh': {
        const controller = new AbortController();
        const timer = setTimeout(()=>controller.abort(),3000);
        try {
          const c = p.connection;
          const res = await transport(c,controller.signal,{})(c.endpoint.replace(/\/$/,'')+'/models');
          // Bound catalog memory before parsing untrusted JSON.
          const reader = res.body.getReader(); let size=0; const chunks=[];
          try { for (;;) { const {value,done}=await reader.read(); if(done) break; size+=value.length; if(size>1<<20) fail('INVALID_PROVIDER_RESPONSE'); chunks.push(value); } }
          finally { await reader.cancel(); }
          const json = JSON.parse(Buffer.concat(chunks).toString('utf8'));
          if (!Array.isArray(json.data) || json.data.length > 4096) fail('INVALID_PROVIDER_RESPONSE');
          return reply(json.data.map(m=> { if(typeof m.id!=='string' || !m.id || m.id.length>512) fail('INVALID_PROVIDER_RESPONSE'); return {model_id:m.id,context_window:0,max_output_tokens:0}; }));
        } finally { clearTimeout(timer); }
      }
      case 'generation.start': case 'generation.continue': {
        if (!p.generation_id || active.has(p.generation_id)) fail('INVALID_PROVIDER_RESPONSE');
        const job={controller:new AbortController(),session:p.session_id,cancelled:false};
        active.set(p.generation_id,job);
        reply({});
        job.done=generate(p,job,send);
        return;
      }
      case 'generation.cancel': {
        const j=active.get(p.generation_id);
        if(j) { j.cancelled=true; j.controller.abort(); await j.done; }
        return reply({});
      }
      case 'session.dispose': {
        const jobs=[...active.values()].filter(j=>j.session===p.session_id);
        for(const j of jobs) { j.cancelled=true; j.controller.abort(); }
        await Promise.all(jobs.map(j=>j.done));
        return reply({});
      }
      default: fail('MODEL_INCOMPATIBLE');
    }
  } catch(e) { send({jsonrpc:'2.0',id:f.id,error:{code:-32000,message:safe(e)}}); }
}

// Bounded framing, concurrent dispatch for cancellation, stdout exclusively RPC.
if (process.argv[1] === fileURLToPath(import.meta.url)) {
  let buffer=Buffer.alloc(0);
  const send=f=>{const data=JSON.stringify(f);if(Buffer.byteLength(data)>=1<<20)fail('INVALID_PROVIDER_RESPONSE');process.stdout.write(data+'\n');};
  process.stdin.on('data',chunk=> {
    buffer=Buffer.concat([buffer,chunk]);
    for(;;) {
      const i=buffer.indexOf(10);
      if(i<0) { if(buffer.length>=1<<20) process.exit(1); break; }
      if(i>=1<<20) process.exit(1);
      const line=buffer.subarray(0,i); buffer=buffer.subarray(i+1);
      try { void handle(JSON.parse(line),send); } catch { process.exit(1); }
    }
  });
  process.stdin.on('end',()=>process.exit(0));
  process.on('uncaughtException',()=>process.exit(1));
  process.on('unhandledRejection',()=>process.exit(1));
}
