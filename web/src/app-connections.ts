import {t} from './i18n';
import {confirmSettingsAction} from './settings-controls';

interface AppReply {
 accounts_html?:string;
 sidebar_html?:string;
 authorization_url?:string;
 authorization_expires?:string;
 connection_id?:string;
 error?:string;
 code?:string;
 revision?:string;
}
interface PendingLink {url:string;expires:number}

function authorizationURL(raw:string):string|null{
 try{
  const url=new URL(raw);
  return url.protocol==='https:'&&!url.username&&!url.password&&!url.port&&['app.composio.dev','connect.composio.dev'].includes(url.hostname)?url.href:null;
 }catch{return null}
}

// Native forms remain the fallback. OAuth URLs live only in this page's memory;
// no credentials or pending links enter browser storage or navigation queries.
export function enhanceAppConnections(){
 const root=document.querySelector<HTMLElement>('[data-apps]');
 const accounts=root?.querySelector<HTMLElement>('#app-accounts');
 const feedback=root?.querySelector<HTMLElement>('[data-app-feedback]');
 if(!root||!accounts||!feedback)return;
 const dialog=root.querySelector<HTMLDialogElement>('[data-app-dialog]');
 const dialogBody=dialog?.querySelector<HTMLElement>('[data-app-dialog-body]');
 let detailGeneration=0,opener:HTMLElement|null=null;
 const category=root.querySelector<HTMLFormElement>('[data-app-category-form]');
 category?.querySelector('select')?.addEventListener('change',()=>category.requestSubmit());
 const links=new Map<string,PendingLink>();
 const timers=new Map<string,ReturnType<typeof setTimeout>>();
 const deadlines=new Map<string,number>();
 const inFlight=new Set<string>();
 const lifetime=new AbortController();
 let stopped=false,busy=false;
 let generation=0,revision=Number(root.dataset.appRevision??0);
 const say=(message:string,error=false)=>{for(const area of [feedback,...root.querySelectorAll<HTMLElement>('[data-app-detail-feedback]')]){area.textContent=t(message);area.hidden=false;area.classList.toggle('error',error)}};
 const row=(id:string)=>Array.from(accounts.querySelectorAll<HTMLElement>('[data-app-account]')).find(item=>item.dataset.appAccount===id);
 const remember=()=>{
  for(const item of accounts.querySelectorAll<HTMLElement>('[data-app-account]')){
   const anchor=item.querySelector<HTMLAnchorElement>('[data-app-auth-link]');
   if(anchor&&!links.has(item.dataset.appAccount!)){
    const url=authorizationURL(anchor.href),expires=Number(anchor.dataset.appExpires);
    if(url&&expires>Date.now())links.set(item.dataset.appAccount!,{url,expires});
   }
  }
 };
 const synchronize=()=>{
  remember();
  for(const img of document.querySelectorAll<HTMLImageElement>('.app-logo img'))if(img.complete&&img.naturalWidth===0)img.dataset.logoFailed='';
  for(const [id,link] of links){
   const item=row(id);
   if(!item||item.dataset.appState!=='pending'||link.expires<=Date.now()){links.delete(id);continue}
   const area=item.querySelector<HTMLElement>('[data-app-authorization]');
   if(area){
    const anchor=document.createElement('a');anchor.href=link.url;anchor.target='_blank';anchor.rel='noopener noreferrer';anchor.className='button secondary';anchor.textContent=t('Open authorization page');anchor.dataset.appAuthLink='';anchor.dataset.appExpires=String(link.expires);area.replaceChildren(anchor);
   }
  }
  const all=Array.from(accounts.querySelectorAll<HTMLElement>('[data-app-account]'));
  for(const tile of root.querySelectorAll<HTMLElement>('[data-app-tile]')){
   const managed=all.some(item=>item.dataset.appSlug===tile.dataset.appTile);
   for(const action of tile.querySelectorAll<HTMLElement>('[data-app-manage]'))action.hidden=!managed;
   for(const action of tile.querySelectorAll<HTMLElement>('[data-app-quick],[data-app-add]'))action.hidden=managed;
  }
  for(const detail of root.querySelectorAll<HTMLElement>('[data-app-detail]')){
   const matching=all.filter(item=>item.dataset.appSlug===detail.dataset.appDetail);
   const active=matching.find(item=>item.dataset.appState==='active');
   const target=detail.querySelector<HTMLElement>('[data-app-detail-accounts]');
   if(target){
    const focused=document.activeElement;
    const focusedRow=focused?.closest<HTMLElement>('[data-app-account]')?.dataset.appAccount;
    const action=focused instanceof HTMLButtonElement?focused.value:null;
    target.replaceChildren(...matching.map(item=>item.cloneNode(true)));
    if(focusedRow&&action)for(const button of target.querySelectorAll<HTMLButtonElement>('button'))if(button.value===action&&button.closest<HTMLElement>('[data-app-account]')?.dataset.appAccount===focusedRow)button.focus();
   }
   const empty=detail.querySelector<HTMLElement>('[data-app-no-accounts]');if(empty)empty.hidden=matching.length>0;
   const needsLabel=detail.dataset.requireAlias==='true';
   const first=detail.querySelector<HTMLElement>('[data-app-first]');if(first)first.hidden=Boolean(active)||needsLabel;
   const extra=detail.querySelector<HTMLElement>('[data-app-add-account]');if(extra)extra.hidden=(!active&&!needsLabel)||active?.dataset.appPublic==='true';
  }
 };
 const openDetails=async(slug:string,source?:HTMLElement)=>{
  if(!dialog||!dialogBody)return;
  const request=++detailGeneration;
  const response=await fetch('/api/apps/detail?app='+encodeURIComponent(slug),{signal:AbortSignal.any([lifetime.signal,AbortSignal.timeout(15000)]),cache:'no-store'});
  if(!response.ok)throw new Error(t('App details could not be loaded. Refresh and try again.'));
  const html=await response.text();if(stopped||request!==detailGeneration)return;
  dialogBody.innerHTML=html;const title=dialogBody.querySelector('h2');if(title)title.id='app-dialog-title';synchronize();
  if(!feedback.hidden){const area=dialogBody.querySelector<HTMLElement>('[data-app-detail-feedback]');if(area){area.textContent=feedback.textContent;area.hidden=false;area.classList.toggle('error',feedback.classList.contains('error'))}}
  if(!dialog.open){opener=source??document.activeElement as HTMLElement;dialog.showModal()}
 };
 dialog?.addEventListener('close',()=>{detailGeneration++;const target=opener?.getClientRects().length?opener:opener?.closest('[data-app-tile]')?.querySelector<HTMLElement>('[data-app-manage]');target?.focus()});
 root.querySelector('[data-app-close]')?.addEventListener('click',()=>dialog?.close());
 const send=async(body:URLSearchParams):Promise<AppReply>=>{
  const response=await fetch('/apps/configure',{method:'POST',body,headers:{Accept:'application/json'},signal:AbortSignal.any([lifetime.signal,AbortSignal.timeout(45000)]),cache:'no-store'});
  const result:AppReply=await response.json();
  if(!response.ok){const error=new Error(result.error||t('The connection could not be confirmed. Refresh the page before trying again.'));Object.assign(error,{code:result.code});throw error}
  return result;
 };
 const apply=(result:AppReply)=>{
  const incoming=Number(result.revision??revision);if(incoming<revision)return;
  revision=incoming;
  if(result.authorization_url&&result.connection_id){
   const url=authorizationURL(result.authorization_url),expires=Number(result.authorization_expires);
   if(url&&expires>Date.now())links.set(result.connection_id,{url,expires});
  }
  if(result.sidebar_html!==undefined){const sidebar=document.querySelector('#apps-sidebar');if(sidebar)sidebar.innerHTML=result.sidebar_html}
  if(result.accounts_html!==undefined){accounts.innerHTML=result.accounts_html;synchronize()}
 };
 const schedule=(id:string)=>{
  if(stopped||timers.has(id)||inFlight.has(id)||row(id)?.dataset.appState!=='pending')return;
  const deadline=deadlines.get(id)??Date.now()+10*60*1000;deadlines.set(id,deadline);
  if(Date.now()>=deadline){say('Automatic status checks paused. Refresh the status or reconnect the account.');return}
  timers.set(id,setTimeout(async()=>{
   timers.delete(id);inFlight.add(id);
   const acceptedGeneration=generation;
   try{
    const result=await send(new URLSearchParams({action:'check',connection:id}));
    if(stopped||acceptedGeneration!==generation)return;
    apply(result);
    if(row(id)?.dataset.appState==='active')say('Account connected.');
    else if(row(id)?.dataset.appState==='expired')say('Authorization expired. Reconnect the account to continue.',true);
   }catch{
    if(!stopped&&acceptedGeneration===generation)say('Status could not be refreshed. Your accounts have been kept; retrying shortly.',true);
   }finally{inFlight.delete(id);if(!stopped)schedule(id)}
  },5000));
 };
 const watch=()=>{for(const item of accounts.querySelectorAll<HTMLElement>('[data-app-state="pending"]'))schedule(item.dataset.appAccount!)};
 root.addEventListener('submit',event=>{
  const form=event.target;if(!(form instanceof HTMLFormElement)||!form.hasAttribute('data-app-action'))return;
  event.preventDefault();if(busy)return;
  const button=(event as SubmitEvent).submitter;
  const body=new URLSearchParams();for(const [name,value] of new FormData(form))if(typeof value==='string')body.append(name,value);
  if(button instanceof HTMLButtonElement&&button.name)body.set(button.name,button.value);
  const action=body.get('action');
  if(action==='disconnect'&&confirmSettingsAction(form,button instanceof HTMLElement?button:null))return;
  generation++;
  // Reserve the tab synchronously while this click still has user activation.
  const popup=['connect','continue','reconnect'].includes(action??'')?window.open('about:blank','_blank'):null;
  if(popup)popup.opener=null;
  const controls=Array.from(root.querySelectorAll<HTMLButtonElement>('[data-app-action] button'));
  const previous=controls.map(item=>item.disabled);controls.forEach(item=>item.disabled=true);
  busy=true;form.setAttribute('aria-busy','true');
  void(async()=>{
   let opened=false;
   try{
    const result=await send(body);if(stopped)return;
    apply(result);
    if(result.authorization_url){
     const url=authorizationURL(result.authorization_url);if(!url)throw new Error(t('The authorization page could not be opened.'));
     if(popup&&!popup.closed){popup.location.replace(url);opened=true;say('Finish signing in in the authorization tab. This page checks automatically.')}
     else say('The browser blocked the authorization tab. Use Open authorization page to continue.');
    }else say(action==='disconnect'?'Account disconnected.':'App settings updated.');
    watch();
    if(result.authorization_url&&!dialog?.open&&!root.querySelector('.app-detail-page')){
     const slug=body.get('toolkit')||row(result.connection_id??'')?.dataset.appSlug;
     if(slug)await openDetails(slug,button instanceof HTMLElement?button:undefined);
    }
   }catch(error){
    if(stopped)return;
    say(error instanceof Error?error.message:'The connection could not be confirmed. Refresh the page before trying again.',true);
    if(error instanceof Error&&(error as Error&{code?:string}).code==='alias_required'){
     form.dataset.requireAlias='true';
     const slug=body.get('toolkit');if(slug){try{await openDetails(slug)}catch{say('App details could not be loaded. Refresh and try again.',true);return}const extra=dialogBody?.querySelector<HTMLDetailsElement>('[data-app-add-account]');if(extra){extra.closest<HTMLElement>('[data-app-detail]')!.dataset.requireAlias='true';extra.hidden=false;extra.open=true;extra.querySelector<HTMLInputElement>('input[name=alias]')?.focus()}}
    }
   }finally{
    if(!opened&&popup&&!popup.closed)popup.close();
    controls.forEach((item,index)=>item.disabled=previous[index]);busy=false;form.removeAttribute('aria-busy');
    if(!stopped)synchronize();
   }
  })();
 });
 document.addEventListener('click',event=>{
  const source=(event.target as Element).closest<HTMLAnchorElement>('[data-app-open]');
  if(source&&dialog&&!event.ctrlKey&&!event.metaKey&&!event.shiftKey&&event.button===0){event.preventDefault();void openDetails(source.dataset.appOpen!,source).catch(()=>location.assign(source.href));return}
  const anchor=(event.target as Element).closest<HTMLAnchorElement>('[data-app-auth-link]');
  if(!anchor)return;
  if(Number(anchor.dataset.appExpires)<=Date.now()){event.preventDefault();say('This authorization link has expired. Reconnect the account to continue.',true)}
 });
 document.addEventListener('error',event=>{const img=event.target;if(img instanceof HTMLImageElement&&img.closest('.app-logo'))img.dataset.logoFailed=''},true);
 const start=()=>{stopped=false;synchronize();watch()};
 window.addEventListener('pagehide',()=>{stopped=true;lifetime.abort();for(const timer of timers.values())clearTimeout(timer);timers.clear()});
 window.addEventListener('pageshow',event=>{if(event.persisted)location.reload()});
 start();root.dataset.appsReady='';
}
