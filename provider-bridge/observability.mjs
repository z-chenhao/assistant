import {createHash,randomUUID} from 'node:crypto';

// A diagnostic projection of the final SDK body. No headers, connection,
// credentials, provider response bodies or image Base64 cross this boundary.
export function wireObserver(request,send){
 if(!request.observe)return ()=>{};
 const secrets=[request.connection?.api_key,request.connection?.access_token].filter(Boolean);
 const redact=value=>{
  if(typeof value==='string'){
   if(value.startsWith('data:')){
    const data=value.slice(value.indexOf(',')+1);
    const match=Object.entries(request.images??{}).find(([,image])=>image.data===data);
    return {media_ref:match?.[0]??null,sha256:createHash('sha256').update(Buffer.from(data,'base64')).digest('hex'),
     ...(match?{original_width:match[1].original_width,original_height:match[1].original_height,preview_width:match[1].preview_width,preview_height:match[1].preview_height}:{}),redacted:'image pixels'};
   }
   for(const secret of secrets)value=value.replaceAll(secret,'[credential redacted]');
   return value;
  }
  if(Array.isArray(value))return value.map(redact);
  if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).map(([key,v])=>[key,/^(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|cookie|set-cookie)$/i.test(key)?'[credential redacted]':redact(v)]));
  return value;
 };
 return (phase,attempt,body,status=0,error='')=>{
  // Diagnostics are bounded and never alter transport success/failure.
  try{
   const event={generation_id:request.generation_id,attempt,phase,at:Date.now()*1000,format:'erii-wire-v1/pi-ai-1.3.0',http_status:status,error_code:error};
   if(phase==='started'){
    const clean=redact(JSON.parse(body));
    const encoded=JSON.stringify(clean);
    event.body_hash=createHash('sha256').update(encoded).digest('hex');
    if(Buffer.byteLength(encoded)<=256*1024)event.body=clean;
    else event.omitted='sanitized body exceeds 256 KiB';
   }
   send({jsonrpc:'2.0',method:'generation.observation',params:event});
  }catch{}
 };
}

export const dispatchID=()=>randomUUID();
