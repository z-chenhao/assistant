// Deterministic transport for the Go final-acceptance regression. The real
// Bridge and Pi still parse and deliver Responses through the stdio boundary.
import { fileURLToPath } from 'node:url';

globalThis.fetch=async input=>{
  const url=new URL(typeof input==='string'?input:input.url??input.toString());
  if(url.href==='https://api.openai.com/v1/models')return Response.json({models:[{slug:'fixture-model',visibility:'list'}]});
  if(url.href!=='https://api.openai.com/v1/responses')throw Error('unexpected fixture route');
  const item={type:'message',id:'msg_fixture',role:'assistant',content:[{type:'output_text',text:'late result',annotations:[]}]};
  const events=[
    ['response.created',{response:{id:'resp_fixture',status:'in_progress'}}],
    ['response.output_item.added',{output_index:0,item:{...item,content:[]}}],
    ['response.output_text.delta',{output_index:0,item_id:item.id,content_index:0,delta:'late result'}],
    ['response.output_item.done',{output_index:0,item}],
    ['response.completed',{response:{id:'resp_fixture',status:'completed',usage:{input_tokens:1,output_tokens:1,total_tokens:2}}}]
  ];
  return new Response(events.map(([type,rest])=>'event: '+type+'\ndata: '+JSON.stringify({type,...rest})+'\n\n').join(''),{headers:{'content-type':'text/event-stream'}});
};
process.argv[1]=fileURLToPath(new URL('../bridge.mjs',import.meta.url));
await import('../bridge.mjs');
