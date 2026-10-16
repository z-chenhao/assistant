import { test } from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { request } from 'node:http';
import { AccountAuth,accountCatalog,API } from '../account.mjs';
import { startFixture } from './account-fixture.mjs';

async function fixture(t,options={}){
  const peer=await startFixture();const auth=new AccountAuth({fetch:peer.fetch,...options});
  t.after(async()=>{await auth.cancel();await peer.close();});
  const start=()=>auth.start({device_id:randomUUID()});
  const complete=async r=>{const callback=await peer.authorize(r.events[0].url);assert.equal((await fetch(callback)).status,200);for(let i=0;i<200;i++){const r=await auth.poll();if(r.status==='connected')return r.credential;await new Promise(r=>setTimeout(r,5));}throw new Error('login timeout');};
  return {peer,auth,start,complete};
}
test('current OAuth PKCE, random loopback port, one-use state and dynamic catalog',async t=>{
  const f=await fixture(t);const r=await f.start();const u=new URL(r.events[0].url),callback=new URL(u.searchParams.get('redirect_uri'));
  assert.equal(callback.hostname,'127.0.0.1');assert.notEqual(callback.port,'1455');assert.equal(u.searchParams.get('code_challenge_method'),'S256');
  const invalid=new URL(callback);invalid.search='state=wrong&code=wrong';assert.equal((await fetch(invalid)).status,400);assert.equal(f.peer.stats.exchanges,0);
  const valid=await f.peer.authorize(u);
  const badHost=await new Promise((resolve,reject)=>{const req=request(valid,{headers:{host:'evil.test'}},res=>{res.resume();resolve(res.statusCode);});req.on('error',reject);req.end();});assert.equal(badHost,400);
  assert.equal((await fetch(valid)).status,200);assert.equal((await fetch(valid)).status,400);
  let result;for(let i=0;i<200;i++){result=await f.auth.poll();if(result.status==='connected')break;await new Promise(r=>setTimeout(r,5));}
  assert.equal(result.status,'connected');await assert.rejects(f.auth.poll(),/AUTH_REQUIRED/);assert.equal(f.peer.stats.exchanges,1);
  const c=result.credential;const connection={endpoint:API,access_token:c.access_token};
  assert.deepEqual((await accountCatalog(connection,f.peer.fetch)).map(m=>m.model_id),['fixture-account-model']);
  await f.peer.control({models:['new-account-model']});assert.deepEqual((await accountCatalog(connection,f.peer.fetch)).map(m=>m.model_id),['new-account-model']);
  await assert.rejects(accountCatalog({...connection,endpoint:'https://evil.test'},f.peer.fetch),/AUTH_REQUIRED/);
  const rotated=await f.auth.refresh(c);assert.notEqual(rotated.refresh_token,c.refresh_token);assert.equal(rotated.subject_hash,c.subject_hash);
  await assert.rejects(f.auth.refresh(c),/AUTH_REQUIRED/);assert.equal(await f.auth.revoke(rotated),true);
});
for(const [name,claims] of Object.entries({issuer:{iss:'https://evil.test'},audience:{aud:'other-client'},nonce:{nonce:'wrong'},expired:{exp:1},future:{iat:9999999999},subject:{sub:''}}))test('reject ID token '+name,async t=>{
  const f=await fixture(t);await f.peer.control({claims});await assert.rejects(f.complete(await f.start()),/AUTH_REQUIRED/);
});
test('expired state, callback binding, and cancelled listener',async t=>{
  let now=Date.now();const f=await fixture(t,{now:()=>now});const r=await f.start();const callback=await f.peer.authorize(r.events[0].url);
  const wrong=new URL(callback);wrong.pathname='/wrong';assert.equal((await fetch(wrong)).status,400);
  now+=300001;assert.equal((await fetch(callback)).status,400);await assert.rejects(f.auth.poll(),/AUTH_REQUIRED/);assert.equal(f.peer.stats.exchanges,0);
  await assert.rejects(fetch(callback));
});
test('returning account reuses issued registration and rejects identity substitution',async t=>{
  const f=await fixture(t);const c=await f.complete(await f.start());
  const r=await f.auth.start({device_id:randomUUID(),registration:{client_id:c.client_id,subject_hash:c.subject_hash},credential:c});
  const u=new URL(r.events[0].url);assert.equal(u.searchParams.get('client_id'),c.client_id);assert.equal(u.searchParams.has('agent_name_hint'),false);assert.equal(u.searchParams.get('id_token_hint'),c.id_token);
  await f.peer.control({claims:{sub:'substituted'}});await assert.rejects(f.complete(r),/AUTH_REQUIRED/);
});

for(const mode of ['signature','no-scope','platform-key'])test('reject grant '+mode,async t=>{
  const f=await fixture(t);await f.peer.control({mode});await assert.rejects(f.complete(await f.start()),/AUTH_REQUIRED/);
});
test('returning callback cannot replace issued client; errors consume state',async t=>{
  const f=await fixture(t);const c=await f.complete(await f.start());const registration={client_id:c.client_id,subject_hash:c.subject_hash};
  const r=await f.auth.start({device_id:randomUUID(),registration});const u=new URL(await f.peer.authorize(r.events[0].url));u.searchParams.set('client_id','oaiapp_other');
  assert.equal((await fetch(u)).status,400);await assert.rejects(f.auth.poll(),/AUTH_REQUIRED/);
  const next=await f.start();const error=new URL(new URL(next.events[0].url).searchParams.get('redirect_uri'));error.search=new URLSearchParams({state:new URL(next.events[0].url).searchParams.get('state'),error:'denied'}).toString();
  assert.equal((await fetch(error)).status,400);assert.equal((await fetch(error)).status,400);await assert.rejects(f.auth.poll(),/AUTH_REQUIRED/);
});

test('auth redirects are rejected without forwarding token or raw error',async t=>{
  let calls=0;
  const auth=new AccountAuth({fetch:async()=>{calls++;return new Response('unsafe fixture material',{status:307,headers:{location:'https://evil.test'}});}});
  await assert.rejects(auth.refresh({client_id:'client',refresh_token:'fixture',subject_hash:'hash'}),e=>e.message==='AUTH_REQUIRED');assert.equal(calls,1);
});

test('malformed catalog is a protocol error rather than authorization loss',async()=>{
  const connection={endpoint:API,access_token:'owned-fixture'};
  for(const body of ['not JSON',JSON.stringify({models:[{slug:5,visibility:'list'}]}),JSON.stringify({models:{}})]){
    await assert.rejects(accountCatalog(connection,async()=>new Response(body)),e=>e.message==='INVALID_PROVIDER_RESPONSE');
  }
});
