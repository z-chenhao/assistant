import http from 'node:http';
import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {createInterface} from 'node:readline';
import {fileURLToPath} from 'node:url';

// Go owns parsing and inference. This explicit fixture only substitutes owned
// loopback HTTP for the pinned HTTPS origins; production has no fetch override.
export async function nativeFixture(t, fetcher) {
 const server=http.createServer(async(req,res)=>{
  try {
   const chunks=[];for await(const c of req)chunks.push(c);
   const origin=req.url.startsWith('/v1/')?'https://api.openai.com':'https://auth.openai.com';
   const headers={...req.headers};delete headers.host;delete headers.connection;
   const response=await fetcher(origin+req.url,{method:req.method,headers,...(chunks.length?{body:Buffer.concat(chunks)}:{})});
   res.writeHead(response.status,Object.fromEntries(response.headers));
   if(response.body){for await(const chunk of response.body)res.write(chunk);}res.end();
  } catch {res.writeHead(502);res.end();}
 });
 server.listen(0,'127.0.0.1');await once(server,'listening');
 const child=spawn(fileURLToPath(new URL('../../bin/steward',import.meta.url)),['-test-native-model-bridge',`http://127.0.0.1:${server.address().port}`],{stdio:['pipe','pipe','pipe']});
 let id=0,stderr='';const pending=new Map();const sinks=new Set();child.stderr.on('data',v=>stderr+=v);
 const lines=createInterface({input:child.stdout});
 lines.on('line',line=>{const frame=JSON.parse(line);if(frame.id){pending.get(frame.id)?.(frame);pending.delete(frame.id);}else{for(const sink of sinks)sink(frame);}});
 const handle=async(frame,sink)=>{
  sinks.add(sink);const current=++id;const done=new Promise(resolve=>pending.set(current,resolve));child.stdin.write(JSON.stringify({...frame,id:current})+'\n');const result=await done;sink(result);return result;
 };
 const rpc=async(method,params={})=>{const result=await handle({jsonrpc:'2.0',method,params},()=>{});if(result.error)throw Error(result.error.message);return result.result;};
 t.after(async()=>{child.stdin.end();if(child.exitCode===null){child.kill();await once(child,'exit');}lines.close();server.closeAllConnections();await new Promise(resolve=>server.close(resolve));if(stderr)throw Error(stderr);});
 await rpc('bridge.initialize',{protocol_version:1});return {handle,rpc};
}
