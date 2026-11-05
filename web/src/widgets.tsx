import {useEffect,useRef,useState} from 'react';
import {Button} from './components/ui/button';

// OpenIntelligentUI's isolated custom-answer pattern, adapted to Steward's
// durable Host artifacts and Code Mode tool calls. No model/provider runs here.
export function IntelligentWidget({title,reference,fallback}:{title:string;reference:string;fallback:string}){
 const frame=useRef<HTMLIFrameElement>(null);
 const [failed,setFailed]=useState(false),[ready,setReady]=useState(false),[expanded,setExpanded]=useState(false);
 const [draft,setDraft]=useState(''),[busy,setBusy]=useState(false),[error,setError]=useState('');
 const source=useRef<string|null>(null);
 const sending=useRef(false);
 const draftValue=useRef('');
 function updateDraft(value:string){draftValue.current=value;setDraft(value)}
 useEffect(()=>{
  const theme=()=>frame.current?.contentWindow?.postMessage({channel:'steward-widget-v1',type:'theme',theme:document.documentElement.classList.contains('dark')?'dark':'light'},'*');
  const listener=(event:MessageEvent)=>{
   if(event.source!==frame.current?.contentWindow||event.origin!=='null'||event.data?.channel!=='steward-widget-v1')return;
   const value=event.data;
   if(value.type==='ready'){setReady(true);theme()}
   if(value.type==='error')setFailed(true);
   if(value.type==='resize'&&typeof value.height==='number'&&Number.isFinite(value.height)&&frame.current)frame.current.style.height=Math.max(160,Math.min(1000,Math.ceil(value.height)))+'px';
   if(value.type==='prompt'&&!sending.current&&!draftValue.current&&typeof value.text==='string'&&value.text.trim()&&value.text.length<=4096){updateDraft(value.text);source.current=null;setError('')}
  };
  window.addEventListener('message',listener);
  const observer=new MutationObserver(theme);observer.observe(document.documentElement,{attributes:true,attributeFilter:['class']});
  const timeout=window.setTimeout(()=>{if(frame.current?.dataset.ready!=='true')setFailed(true)},10000);
  return()=>{window.removeEventListener('message',listener);observer.disconnect();window.clearTimeout(timeout)};
 },[reference]);
 async function send(){
  if(sending.current||!draft.trim())return;sending.current=true;setBusy(true);setError('');source.current??=crypto.randomUUID();
  try{
   const response=await fetch('/api/message',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({source_id:'ui-widget:'+source.current,content:[{type:'text',text:draft}]})});
   if(!response.ok)throw Error('Your message could not be sent. Retry to use the same receipt.');
   updateDraft('');source.current=null;
  }catch(e){setError(e instanceof Error?e.message:'Message unavailable')}finally{sending.current=false;setBusy(false)}
 }
 return <section className={'intelligent-widget'+(expanded?' widget-expanded':'')} data-widget-ref={reference}>
  <header><strong>{title}</strong><Button type="button" onClick={()=>setExpanded(!expanded)}>{expanded?'Close expanded view':'Expand'}</Button></header>
  {!failed&&<iframe ref={frame} title={title} src={'/ui/widget?ref='+encodeURIComponent(reference)} sandbox="allow-scripts" referrerPolicy="no-referrer" data-ready={String(ready)} onError={()=>setFailed(true)}/>}
  {!ready&&!failed&&<p role="status">Preparing interactive answer…</p>}
  {failed&&<p role="status">The interactive view is unavailable. The text version is preserved.</p>}
  <details><summary>Text version</summary><p className="semantic">{fallback}</p></details>
  {draft&&<div className="widget-followup"><label>Message to Ghost<textarea disabled={busy} value={draft} maxLength={4096} onChange={e=>{updateDraft(e.target.value);source.current=null}}/></label><Button type="button" disabled={busy} onClick={()=>void send()}>{busy?'Sending…':'Send to Ghost'}</Button><Button type="button" disabled={busy} onClick={()=>{updateDraft('');source.current=null}}>Dismiss</Button><p>Sending a message does not approve external actions.</p></div>}
  {error&&<p role="alert">{error}</p>}
 </section>
}
