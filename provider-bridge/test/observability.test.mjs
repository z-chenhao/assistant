import {test} from 'node:test';
import assert from 'node:assert/strict';
import {wireObserver} from '../observability.mjs';

test('wire observation is opt-in, sanitized, bounded and cannot fail transport',()=>{
 const events=[];wireObserver({},e=>events.push(e))('started','attempt','{}');assert.equal(events.length,0);
 const image=Buffer.from('synthetic preview bytes').toString('base64');
 const request={observe:true,generation_id:'g',connection:{api_key:'PRIVATE_KEY',access_token:'PRIVATE_ACCESS'},images:{'artifact:picture':{data:image,original_width:600,original_height:400,preview_width:60,preview_height:40}}};
 const record=wireObserver(request,e=>events.push(e));
 record('started','one',JSON.stringify({messages:[{role:'user',content:[{type:'text',text:'PRIVATE_KEY PRIVATE_ACCESS'},{type:'image_url',image_url:{url:'data:image/jpeg;base64,'+image}}]}],authorization:'PRIVATE_KEY',api_key:'PRIVATE_KEY'}));
 const raw=JSON.stringify(events);assert(!raw.includes('PRIVATE_KEY'));assert(!raw.includes('PRIVATE_ACCESS'));assert(!raw.includes(image));
 const payload=events[0].params;assert.equal(payload.body.messages[0].content[1].image_url.url.media_ref,'artifact:picture');assert.equal(payload.body_hash.length,64);
 record('started','huge',JSON.stringify({messages:[{content:'x'.repeat(300000)}]}));assert.equal(events[1].params.body,undefined);assert.match(events[1].params.omitted,/exceeds/);
 record('headers','one',null,200);assert.equal(events[2].params.http_status,200);
 assert.doesNotThrow(()=>wireObserver(request,()=>{throw Error('sink unavailable')})('started','one','{}'));
});
