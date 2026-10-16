// Current OpenAI public-client subscription protocol. No credential files or
// legacy Codex routes. Injectable I/O is private to deterministic fixtures.
import { createServer } from 'node:http';
import { randomBytes, createHash, createPublicKey, verify } from 'node:crypto';

export const AUTH = 'https://auth.openai.com';
export const API = 'https://api.openai.com/v1';
const scope = 'openid profile email offline_access resource.invoke chatgpt.tokens.use.direct';
const fail = () => { throw new Error('AUTH_REQUIRED'); };
const digest = s => createHash('sha256').update(s).digest('hex');
const text = s => typeof s === 'string' && s.length > 0 && s.length <= 32768;

export async function boundedJSON(response, limit = 1 << 20) {
  if (!response.ok || !response.body) { await response.body?.cancel(); fail(); }
  const reader = response.body.getReader(); const chunks = []; let length = 0;
  try {
    for (;;) {
      const {value, done} = await reader.read(); if (done) break;
      length += value.length; if (length > limit) fail(); chunks.push(value);
    }
    return JSON.parse(Buffer.concat(chunks).toString('utf8'));
  } finally { await reader.cancel(); }
}

export class AccountAuth {
  constructor({fetch: fetcher = globalThis.fetch, now = Date.now, ttl = 300000} = {}) {
    this.fetch = fetcher; this.now = now; this.ttl = ttl; this.pending = undefined;
  }
  async request(url, init = {}) {
    const u = new URL(url);
    if (u.origin !== AUTH || u.username || u.password || u.hash) fail();
    return this.fetch(u, {...init, redirect:'manual', signal:AbortSignal.timeout(10000)});
  }
  async identity(token, client, nonce) {
    if (!text(token)) fail();
    const parts = token.split('.'); if (parts.length !== 3 || parts.some(p=>!p || !/^[\w-]+$/.test(p))) fail();
    const header = JSON.parse(Buffer.from(parts[0], 'base64url'));
    if (!['RS256','ES256'].includes(header.alg) || !text(header.kid) || header.crit) fail();
    const jwks = await boundedJSON(await this.request(AUTH+'/.well-known/jwks.json'));
    if (!Array.isArray(jwks.keys) || jwks.keys.length > 100) fail();
    const keys = jwks.keys.filter(k=>k.kid===header.kid && (!k.use || k.use==='sig') && (!k.alg || k.alg===header.alg) && (header.alg==='RS256' ? k.kty==='RSA' : k.kty==='EC' && k.crv==='P-256'));
    if (keys.length !== 1) fail();
    const key = createPublicKey({key:keys[0],format:'jwk'});
    if (!verify('sha256', Buffer.from(parts[0]+'.'+parts[1]), {key, dsaEncoding:'ieee-p1363'}, Buffer.from(parts[2],'base64url'))) fail();
    const claims = JSON.parse(Buffer.from(parts[1], 'base64url'));
    const now = this.now()/1000;
    if (claims.iss !== AUTH || !(claims.aud === client || Array.isArray(claims.aud) && claims.aud.includes(client)) ||
        claims.azp !== undefined && claims.azp !== client || Array.isArray(claims.aud) && claims.aud.length > 1 && claims.azp !== client ||
        !text(claims.sub) || !Number.isFinite(claims.exp) || claims.exp <= now || !Number.isFinite(claims.iat) || claims.iat > now+5 ||
        claims.nbf !== undefined && (!Number.isFinite(claims.nbf) || claims.nbf > now+5) || nonce !== undefined && claims.nonce !== nonce) fail();
    return digest(claims.iss+'\n'+claims.sub);
  }
  async credential(token, client, nonce, prior) {
    if (!text(token.access_token) || token.access_token.startsWith('sk-') || !text(token.refresh_token) || !Number.isFinite(token.expires_in) || token.expires_in <= 0 || token.expires_in > 31536000 || token.token_type?.toLowerCase() !== 'bearer') fail();
    const scopes = typeof token.scope === 'string' ? token.scope.split(/\s+/) : prior?.scopes;
    if (!Array.isArray(scopes) || !scopes.includes('chatgpt.tokens.use.direct')) fail();
    let subject = prior?.subject_hash;
    if (token.id_token) subject = await this.identity(token.id_token, client, nonce);
    else if (nonce !== undefined || !subject) fail();
    if (prior?.subject_hash && subject !== prior.subject_hash) fail();
    return {client_id:client,subject_hash:subject,id_token:token.id_token ?? prior?.id_token,
      access_token:token.access_token,refresh_token:token.refresh_token,scopes,
      expires_at:new Date(this.now()+token.expires_in*1000).toISOString()};
  }
  async token(body) {
    return boundedJSON(await this.request(AUTH+'/api/accounts/oauth/token', {
      method:'POST',headers:{'content-type':'application/x-www-form-urlencoded',accept:'application/json'},body,
    }), 65536);
  }
  async start({device_id, registration, credential}) {
    await this.cancel();
    if (typeof device_id !== 'string' || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(device_id)) fail();
    const verifier=randomBytes(32).toString('base64url');
    const challenge=createHash('sha256').update(verifier).digest('base64url');
    const state = randomBytes(32).toString('base64url'), nonce = randomBytes(32).toString('base64url');
    const p = {state,nonce,verifier,registration,credential,status:'pending',expires:this.now()+this.ttl,used:false};
    const server = createServer((req,res)=> {
      // No access log; every response is constant, including errors.
      const send = status => { res.writeHead(status,{'content-type':'text/plain','cache-control':'no-store','referrer-policy':'no-referrer'});res.end(status===200?'Erii: authorization received. You can close this window.':'Erii: invalid or expired authorization.'); };
      let u; try { u = new URL(req.url, p.redirect); } catch {send(400);return;}
      if (req.method!=='GET' || req.headers.host!==new URL(p.redirect).host || u.origin!==new URL(p.redirect).origin || u.pathname!=='/auth/callback' || u.hash || p.used || this.now()>=p.expires ||
          u.searchParams.getAll('state').length!==1 || u.searchParams.get('state')!==state) {send(400);return;}
      p.used = true; // Consume before token exchange, including an OAuth error.
      if (u.searchParams.has('error') || u.searchParams.getAll('code').length!==1 || !text(u.searchParams.get('code'))) {p.status='failed';send(400);return;}
      const issued = u.searchParams.get('client_id') ?? registration?.client_id;
      if (!text(issued) || issued==='dynamic_agent_client' || registration && issued!==registration.client_id || u.searchParams.getAll('client_id').length>1) {p.status='failed';send(400);return;}
      send(200);
      p.work = this.token(new URLSearchParams({grant_type:'authorization_code',client_id:issued,code:u.searchParams.get('code'),code_verifier:verifier,redirect_uri:p.redirect,resource:API}))
        .then(t=>this.credential(t,issued,nonce,registration))
        .then(c=>{if(this.pending===p && this.now()<p.expires){p.result=c;p.status='connected';}})
        .catch(()=>{p.status='failed';});
    });
    await new Promise((resolve,reject)=>{server.once('error',reject);server.listen(0,'127.0.0.1',resolve);});
    p.server=server;p.redirect=`http://127.0.0.1:${server.address().port}/auth/callback`;
    p.timer=setTimeout(()=>{p.status='failed';server.close();server.closeAllConnections();},this.ttl);
    this.pending=p;
    const url = new URL(AUTH+'/api/accounts/authorize');
    url.search = new URLSearchParams({client_id:registration?.client_id ?? 'dynamic_agent_client',
      ...(registration?{}:{agent_name_hint:'Erii'}),ext_agent_host_id:'urn:uuid:'+device_id,
      response_type:'code',redirect_uri:p.redirect,resource:API,scope,state,nonce,code_challenge:challenge,code_challenge_method:'S256',
      ...(credential?.id_token?{id_token_hint:credential.id_token}:{})}).toString();
    return {status:'pending',events:[{kind:'open_url',url:url.toString()}]};
  }
  async poll() {
    const p=this.pending;
    if (!p || p.status==='failed' || this.now()>=p.expires) {await this.cancel();fail();}
    if (p.status!=='connected') {const progress=p.used && !p.progress;p.progress ||= p.used;return {status:'pending',events:progress?[{kind:'progress'}]:[]};}
    const credential=p.result;await this.cancel();return {status:'connected',events:[],credential};
  }
  async cancel() {
    const p=this.pending;this.pending=undefined;
    if(p){clearTimeout(p.timer);p.server.close();p.server.closeAllConnections();await p.work;p.result=undefined;p.credential=undefined;}
  }
  async refresh(credential) {
    if (!credential || !text(credential.client_id) || !text(credential.refresh_token) || !credential.subject_hash) fail();
    const token=await this.token(new URLSearchParams({grant_type:'refresh_token',client_id:credential.client_id,refresh_token:credential.refresh_token,resource:API}));
    return this.credential(token,credential.client_id,undefined,credential);
  }
  async revoke(credential) {
    if (!credential) return false;
    try {
      const config=await boundedJSON(await this.request(AUTH+'/.well-known/openid-configuration'));
      if(config.issuer!==AUTH || !text(config.revocation_endpoint)) return false;
      const r=await this.request(config.revocation_endpoint,{method:'POST',headers:{'content-type':'application/x-www-form-urlencoded'},
        body:new URLSearchParams({token:credential.refresh_token,token_type_hint:'refresh_token',client_id:credential.client_id})});
      await r.body?.cancel();return r.status===200;
    } catch {return false;}
  }
}

export async function accountCatalog(connection, fetcher=globalThis.fetch, signal) {
  if (connection.endpoint!==API || !text(connection.access_token) || connection.api_key) fail();
  const deadline=AbortSignal.timeout(10000);
  const response=await fetcher(API+'/models',{headers:{authorization:'Bearer '+connection.access_token},redirect:'manual',signal:signal?AbortSignal.any([signal,deadline]):deadline});
  if(!response.ok){await response.body?.cancel();throw new Error(({401:'AUTH_REQUIRED',403:'AUTH_REQUIRED',429:'RATE_LIMITED'})[response.status] ?? 'PROVIDER_UNAVAILABLE');}
  let data;
  try{data=await boundedJSON(response);}catch{throw new Error('INVALID_PROVIDER_RESPONSE');}
  const invalid=()=>{throw new Error('INVALID_PROVIDER_RESPONSE');};
  if(!Array.isArray(data.models) || data.models.length>4096) invalid();
  const seen=new Set();
  return data.models.filter(m=>m.visibility==='list').map(m=>{
    if(!text(m.slug) || m.slug.length>512 || seen.has(m.slug)) invalid();seen.add(m.slug);
    return {model_id:m.slug,context_window:Math.min(1048576,Number.isSafeInteger(m.context_window)&&m.context_window>0?m.context_window:8192),max_output_tokens:Math.min(65536,Number.isSafeInteger(m.max_output_tokens)&&m.max_output_tokens>0?m.max_output_tokens:1024)};
  });
}
