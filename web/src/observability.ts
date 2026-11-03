// Progressive navigation for the server-rendered trace workbench. The server
// remains the only owner of query/selection state; native links work without JS.
import {revealCurrentTabs} from './shell';
export function enhanceObservability() {
 if(!document.querySelector('#observability-page'))return;
 let pending:AbortController|null=null;
 async function select(url:URL,push:boolean){
  pending?.abort();const controller=new AbortController();pending=controller;
  const current=document.querySelector<HTMLElement>('#observability-workbench');
  const treeTop=current?.querySelector('.obs-tree-pane')?.scrollTop??0;
  const detailTop=current?.querySelector('.obs-detail-pane')?.scrollTop??0;
  const selected=current?.querySelector('.tree-heading[aria-current]')?.closest<HTMLElement>('[data-observation-id]')?.dataset.observationId;
  current?.classList.add('obs-loading');
  try{
   const response=await fetch(url,{credentials:'same-origin',cache:'no-store',signal:controller.signal,headers:{Accept:'text/html'}});
   if(!response.ok)throw new Error('trace read unavailable');
   const parsed=new DOMParser().parseFromString(await response.text(),'text/html');
   const next=parsed.querySelector('#observability-workbench');
   if(!next||!current||controller.signal.aborted)throw new Error('invalid trace view');
   // Capture the latest focus: the user may have tabbed during the request.
   const focus=current.contains(document.activeElement)?document.activeElement:null;
   const focusedHref=focus instanceof HTMLAnchorElement?focus.href:null;
   const focusRegion=focus?.closest('.obs-tree-pane')?'.obs-tree-pane':focus?.closest('.obs-detail-pane')?'.obs-detail-pane':'.obs-trace-header';
   const focusedClasses=focus instanceof HTMLAnchorElement?Array.from(focus.classList):[];
   current.replaceWith(next);
   const tree=next.querySelector('.obs-tree-pane');if(tree)tree.scrollTop=treeTop;
   const nextSelected=next.querySelector('.tree-heading[aria-current]')?.closest<HTMLElement>('[data-observation-id]')?.dataset.observationId;
   const detail=next.querySelector('.obs-detail-pane');if(detail&&selected&&selected===nextSelected)detail.scrollTop=detailTop;
   if(focusedHref){
    const replacement=Array.from(next.querySelectorAll<HTMLAnchorElement>(`${focusRegion} a`)).find(anchor=>anchor.href===focusedHref&&focusedClasses.every(name=>anchor.classList.contains(name)));
    replacement?.focus({preventScroll:true});
   }
   revealCurrentTabs(next);
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
