// Progressive navigation for the server-rendered trace workbench. The server
// remains the only owner of query/selection state; native links work without JS.
export function enhanceObservability() {
 if(!document.querySelector('#observability-page'))return;
 let pending:AbortController|null=null;
 async function select(url:URL,push:boolean){
  pending?.abort();const controller=new AbortController();pending=controller;
  const current=document.querySelector<HTMLElement>('#observability-workbench');
  const positions=['.obs-tree-pane','.obs-waterfall-pane'].map(selector=>[selector,current?.querySelector(selector)?.scrollTop??0] as const);
  current?.classList.add('obs-loading');
  try{
   const response=await fetch(url,{credentials:'same-origin',cache:'no-store',signal:controller.signal,headers:{Accept:'text/html'}});
   if(!response.ok)throw new Error('trace read unavailable');
   const parsed=new DOMParser().parseFromString(await response.text(),'text/html');
   const next=parsed.querySelector('#observability-workbench');
   if(!next||!current||controller.signal.aborted)throw new Error('invalid trace view');
   current.replaceWith(next);
   for(const [selector,top] of positions){const pane=next.querySelector(selector);if(pane)pane.scrollTop=top;}
   if(push)history.pushState(null,'',url);
  }catch{if(!controller.signal.aborted)location.assign(url);}
  finally{if(pending===controller)pending=null;}
 }
 document.addEventListener('click',event=>{
  if(!(event instanceof MouseEvent)||event.button!==0||event.ctrlKey||event.metaKey||event.shiftKey||event.altKey)return;
  const anchor=(event.target as Element|null)?.closest<HTMLAnchorElement>('a.obs-select');
  if(!anchor||anchor.target||anchor.download)return;
  const url=new URL(anchor.href);
  if(url.origin!==location.origin||!url.pathname.startsWith('/developer/observability/'))return;
  event.preventDefault();void select(url,true);
 });
 window.addEventListener('popstate',()=>{if(location.pathname.startsWith('/developer/observability/'))void select(new URL(location.href),false);});
 window.addEventListener('pagehide',()=>pending?.abort());
}
