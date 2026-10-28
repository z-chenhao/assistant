import {z} from 'zod';
import {enhanceProjections,releaseProjections} from './projections';

const snapshot=z.object({view:z.enum(['/','/progress','/settings']),before:z.number().int().nonnegative(),latest:z.number().int().nonnegative(),regions:z.record(z.string(),z.string())});
const regions={
 '/':['workspace-status','conversation-messages','conversation-approvals'],
 '/progress':['workspace-status','ongoing-work'],
 '/settings':['workspace-status','signin-status'],
} as const;
type View=keyof typeof regions;

// Cache Host markup, not React's enhanced DOM. Stable keyed islands retain
// their component state, focus and nodes when unrelated content changes.
function regionPatcher(ids:readonly string[]){
 const originals=new WeakMap<Element,string>();
 const markup=new Map<string,string>();
 for(const id of ids){
  const region=document.getElementById(id);
  if(region){markup.set(id,region.innerHTML);for(const child of region.children)originals.set(child,child.outerHTML)}
 }
 return (id:string,html:string)=>{
  const region=document.getElementById(id);
  if(!region||markup.get(id)===html)return;
  const parsed=document.createElement('template');parsed.innerHTML=html;
  if(id==='workspace-status'||id==='signin-status'){
   releaseProjections(region);region.replaceChildren(parsed.content);
  }else{
   const previous=Array.from(region.children);
   const keyed=new Map(previous.map((child,i)=>[(child as HTMLElement).dataset.liveKey??`position:${i}`,child]));
   const retained=new Set<Element>();
   Array.from(parsed.content.children).forEach((incoming,i)=>{
    const key=(incoming as HTMLElement).dataset.liveKey??`position:${i}`;
    const old=keyed.get(key),raw=incoming.outerHTML;
    const child=old&&originals.get(old)===raw?old:incoming;
    retained.add(child);originals.set(child,raw);
    if(region.children[i]!==child)region.insertBefore(child,region.children[i]??null);
   });
   for(const child of previous){if(!retained.has(child)){releaseProjections(child as HTMLElement);child.remove()}}
  }
  markup.set(id,html);enhanceProjections(region);
 };
}

function viewportAnchor(){
 const focused=document.activeElement;
 if(focused instanceof HTMLElement&&focused!==document.body&&!focused.closest('#composer-root'))return {node:focused,top:focused.getBoundingClientRect().top};
 const node=Array.from(document.querySelectorAll<HTMLElement>('[data-live-key]')).find(node=>{
  const box=node.getBoundingClientRect();return box.bottom>80&&box.top<innerHeight-200;
 });
 return node?{node,top:node.getBoundingClientRect().top}:null;
}
function atBottom(){return scrollY+innerHeight>=document.documentElement.scrollHeight-100}
function scrollToLatest(){window.scrollTo({top:document.documentElement.scrollHeight,behavior:'instant'})}

export function startLiveUpdates(){
 const main=document.querySelector<HTMLElement>('main[data-live-view]');
 if(!main||!Object.hasOwn(regions,main.dataset.liveView??''))return null;
 const view=main.dataset.liveView as View;
 const patch=regionPatcher(regions[view]);
 const feedback=document.getElementById('live-update-status');
 const jump=document.querySelector<HTMLButtonElement>('#new-messages');
 let before=Number(main.dataset.liveBefore??0),latest=Number(main.dataset.liveLatest??0);
 let etag='',timer=0,generation=0,delay=1000,stopped=false,follow=false;
 let request:AbortController|null=null;
 const say=(text:string)=>{if(feedback&&feedback.textContent!==text)feedback.textContent=text};
 const accessLost=()=>{
  stopped=true;say('Updates paused. Your draft is kept. ');
  if(!feedback)return;
  const signin=document.createElement('a');signin.href='/';signin.target='_blank';signin.rel='noreferrer';signin.textContent='Sign in';
  const retry=document.createElement('button');retry.type='button';retry.textContent='Retry updates';
  retry.addEventListener('click',()=>{stopped=false;delay=1000;void refresh()});
  feedback.append(signin,document.createTextNode(' · '),retry);
 };
 const showJump=()=>{if(jump)jump.hidden=false};
 const pause=()=>{generation++;clearTimeout(timer);request?.abort();request=null};
 const finishFollow=()=>{scrollToLatest();if(jump)jump.hidden=true;follow=false};

 async function refresh(){
  if(stopped||document.visibilityState!=='visible')return;
  clearTimeout(timer);request?.abort();
  const current=++generation,controller=new AbortController();request=controller;
  const timeout=setTimeout(()=>controller.abort(),10000);
  try{
   const query=new URLSearchParams({view});if(before)query.set('before',String(before));
   const response=await fetch('/api/updates?'+query,{credentials:'same-origin',cache:'no-store',signal:controller.signal,headers:etag?{'If-None-Match':etag}:{}});
   if(current!==generation)return;
   if(response.status===401||response.status===403){accessLost();return}
   if(response.status===304){if(follow)finishFollow();delay=1000;say('');return}
   if(!response.ok)throw new Error('updates unavailable');
   const value=snapshot.parse(await response.json());
   if(current!==generation)return;
   if(value.view!==view||value.before!==before||regions[view].some(id=>typeof value.regions[id]!=='string'))throw new Error('invalid view');
   const bottom=atBottom(),anchor=viewportAnchor();
   let deferMessages=before!==0;
   if(view==='/'&&!bottom&&!follow&&anchor?.node.dataset.liveKey?.startsWith('message:')){
    const parsed=document.createElement('template');parsed.innerHTML=value.regions['conversation-messages'];
    // If the bounded latest page has moved past the reading position, retain
    // that history until the user explicitly jumps to the new messages.
    if(!Array.from(parsed.content.querySelectorAll<HTMLElement>('[data-live-key]')).some(node=>node.dataset.liveKey===anchor.node.dataset.liveKey))deferMessages=true;
   }
   for(const id of regions[view]){if(id!=='conversation-messages'||!deferMessages||follow)patch(id,value.regions[id])}
   if(view==='/'&&(follow||bottom&&before===0))finishFollow();
   else{
    if(anchor?.node.isConnected)window.scrollBy(0,anchor.node.getBoundingClientRect().top-anchor.top);
    if(view==='/'&&(value.latest>latest||before!==0))showJump();
   }
   latest=value.latest;etag=response.headers.get('ETag')??'';delay=1000;say('');
  }catch{
   if(current===generation&&!stopped&&document.visibilityState==='visible'){
    say('Connection lost. Reconnecting… Your draft is kept.');delay=Math.min(delay*2,30000);
   }
  }finally{
   clearTimeout(timeout);
   if(current===generation){request=null;if(!stopped&&document.visibilityState==='visible')timer=window.setTimeout(refresh,delay)}
  }
 }
 jump?.addEventListener('click',()=>{
  if(stopped)return;
  before=0;etag='';follow=true;
  history.replaceState(null,'','/');void refresh();
 });
 // Keep conversation drafts when approving a newly arrived action. Each POST
 // carries the displayed frozen hash and is attempted exactly once; a lost
 // acknowledgement triggers a state read, never an automatic resubmission.
 document.addEventListener('submit',event=>{
  const form=event.target;
  if(!(form instanceof HTMLFormElement)||form.getAttribute('action')!=='/approval')return;
  event.preventDefault();
  if(stopped)return;
  if(form.dataset.submitting)return;
  const submitter=(event as SubmitEvent).submitter;
  if(!(submitter instanceof HTMLButtonElement))return;
  const data=new URLSearchParams();
  for(const [key,value] of new FormData(form)){if(typeof value==='string')data.append(key,value)}
  data.set(submitter.name,submitter.value);
  const buttons=Array.from(form.querySelectorAll('button'));
  form.dataset.submitting='true';for(const button of buttons)button.disabled=true;
  void (async()=>{
   const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),10000);
   try{
    const response=await fetch('/approval',{method:'POST',body:data,credentials:'same-origin',headers:{Accept:'application/json'},signal:controller.signal});
    if(!response.ok||!(await response.json()).accepted)throw new Error('approval not confirmed');
    say('');
   }catch{say('Approval could not be confirmed. Checking its current state. Your draft is kept.')}
   finally{
    clearTimeout(timeout);delete form.dataset.submitting;for(const button of buttons)button.disabled=false;
    void refresh();
   }
  })();
 });
 document.addEventListener('visibilitychange',()=>{if(document.visibilityState==='visible')void refresh();else pause()});
 window.addEventListener('pagehide',pause);
 window.addEventListener('pageshow',event=>{if(event.persisted)void refresh()});
 if(before)showJump();
 return {start:()=>{void refresh()}};
}
