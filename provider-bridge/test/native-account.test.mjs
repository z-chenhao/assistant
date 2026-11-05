import {test} from 'node:test';
import assert from 'node:assert/strict';
import {randomUUID} from 'node:crypto';
import {startFixture} from './account-fixture.mjs';
import {nativeFixture} from './native-fixture.mjs';

async function account(t){
 const peer=await startFixture();t.after(()=>peer.close());const {rpc}=await nativeFixture(t,peer.fetch);
 const start=()=>rpc('auth.start',{device_id:randomUUID()});
 const complete=async login=>{const callback=await peer.authorize(login.events[0].url);assert.equal((await fetch(callback)).status,200);for(let i=0;i<200;i++){const next=await rpc('auth.continue');if(next.status==='connected')return next.credential;await new Promise(resolve=>setTimeout(resolve,5));}throw Error('native login timeout');};
 return {peer,rpc,start,complete};
}
test('native Go PKCE, JWT verification, credential rotation, catalog and revocation',async t=>{
 const f=await account(t);const login=await f.start();const u=new URL(login.events[0].url);assert.equal(u.searchParams.get('code_challenge_method'),'S256');
 const wrong=new URL(u.searchParams.get('redirect_uri'));wrong.search='state=wrong&code=wrong';assert.equal((await fetch(wrong)).status,400);assert.equal(f.peer.stats.exchanges,0);
 const c=await f.complete(login);const connection={endpoint:'https://api.openai.com/v1',access_token:c.access_token};
 assert.deepEqual((await f.rpc('models.list',{connection})).map(m=>m.model_id),['fixture-account-model']);
 const refreshed=await f.rpc('auth.refresh',{credential:c});assert.notEqual(refreshed.refresh_token,c.refresh_token);assert.equal(refreshed.subject_hash,c.subject_hash);
 await assert.rejects(f.rpc('auth.refresh',{credential:c}),/AUTH_REQUIRED/);
 const returning=await f.rpc('auth.start',{device_id:randomUUID(),registration:{client_id:refreshed.client_id,subject_hash:refreshed.subject_hash},credential:refreshed});assert.equal(new URL(returning.events[0].url).searchParams.get('client_id'),c.client_id);
 const next=await f.complete(returning);assert.equal(next.subject_hash,c.subject_hash);
 assert.equal((await f.rpc('auth.logout',{connection_id:'c',credential:next})).revoked,true);
});
for(const [name,claims] of Object.entries({issuer:{iss:'https://evil.test'},audience:{aud:'other'},nonce:{nonce:'wrong'},expired:{exp:1},future:{iat:9999999999},subject:{sub:''}}))test('native Go rejects ID token '+name,async t=>{const f=await account(t);await f.peer.control({claims});await assert.rejects(f.complete(await f.start()),/AUTH_REQUIRED/);});
for(const mode of ['signature','no-scope','platform-key'])test('native Go rejects grant '+mode,async t=>{const f=await account(t);await f.peer.control({mode});await assert.rejects(f.complete(await f.start()),/AUTH_REQUIRED/);});
test('native Go cancel closes callback; dynamic registration stays bound to identity',async t=>{
 const f=await account(t);const login=await f.start();const callback=await f.peer.authorize(login.events[0].url);await f.rpc('auth.cancel');await assert.rejects(fetch(callback));assert.equal(f.peer.stats.exchanges,0);
 const c=await f.complete(await f.start());const next=await f.rpc('auth.start',{device_id:randomUUID(),registration:{client_id:c.client_id,subject_hash:c.subject_hash}});await f.peer.control({claims:{sub:'substitution'}});await assert.rejects(f.complete(next),/AUTH_REQUIRED/);
});
