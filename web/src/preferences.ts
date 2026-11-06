import {t} from './i18n';
import {enhanceSelect} from './components/ui/select';

// One native value source, one in-flight write. Unknown results require a GET;
// neither returning from BFCache nor a late response may replay the POST.
export function enhanceLanguagePreference(){
 const form=document.querySelector<HTMLFormElement>('[data-language-preference]');
 if(!form||typeof fetch!=='function'||typeof AbortController!=='function')return;
 const select=form.querySelector<HTMLSelectElement>('select[name=language]');
 const label=form.querySelector<HTMLLabelElement>('#language-label');
 const fallbackSave=form.querySelector<HTMLButtonElement>('.preference-save');
 const feedback=form.querySelector<HTMLElement>('[data-language-feedback]');
 const message=form.querySelector<HTMLElement>('[data-language-message]');
 const retry=form.querySelector<HTMLButtonElement>('[data-language-retry]');
 const refresh=form.querySelector<HTMLAnchorElement>('[data-language-refresh]');
 if(!select||!label||!fallbackSave||!feedback||!message||!retry||!refresh)return;
 let confirmed=select.value,desired=confirmed;
 let state:'idle'|'saving'|'unknown'='idle';
 let generation=0;
 let controller:AbortController|undefined;
 const picker=enhanceSelect(select),trigger=picker?.trigger??select;
 const custom=!!picker;
 const sync=()=>picker?.sync();
 const close=(focus=false)=>picker?.close(focus);
 const report=(text:string,alert=false,action:'retry'|'refresh'|''='')=>{
  feedback.hidden=false;feedback.setAttribute('role',alert?'alert':'status');
  message.textContent=t(text);retry.hidden=action!=='retry';refresh.hidden=action!=='refresh';
 };
 const busy=(locked:boolean)=>{
  // Keep the feedback live region outside the busy subtree, including when an
  // unknown result locks further writes after the network operation has ended.
  picker?.setLocked(locked,state==='saving');
  if(!custom)select.disabled=locked;
 };
 const unknown=()=>{
  state='unknown';busy(true);
  report('Save result not confirmed.',true,'refresh');
 };
 const save=async()=>{
  if(state!=='idle')return;
  desired=select.value;
  if(desired===confirmed)return;
  const attempt=++generation;
  state='saving';close();sync();busy(true);report('Saving…');
  controller=new AbortController();
  const timeout=window.setTimeout(()=>controller?.abort(),15000);
  try{
   const response=await fetch(form.action,{method:'POST',credentials:'same-origin',redirect:'error',headers:{Accept:'application/json'},body:new URLSearchParams({language:desired}),signal:controller.signal});
   if(attempt!==generation)return;
   // These pre-write rejections preserve the previous cookie. Other statuses
   // and malformed responses cannot prove what happened to the preference.
   if(response.status===400||response.status===401||response.status===403){
    select.value=confirmed;sync();state='idle';busy(false);
    if(response.status===400)report('Invalid language. Choose English or Simplified Chinese.',true,'retry');
    else report('Language could not be saved. Reopen settings to check access.',true,'refresh');
    return;
   }
   if(response.status!==200||!response.headers.get('Content-Type')?.includes('application/json')){unknown();return}
   const result:unknown=await response.json();
   if(attempt!==generation)return;
   if(typeof result!=='object'||result===null||!('language' in result)||result.language!==desired){unknown();return}
   // GET reads the cookie back. The server suppresses success if it did not
   // persist; the fragment restores focus only for this explicit operation.
   location.assign('/settings/preferences?notice=language-saved&language='+encodeURIComponent(desired)+'#interface-language-picker');
  }catch{
   if(attempt===generation)unknown();
  }finally{
   window.clearTimeout(timeout);
  }
 };
 window.addEventListener('pagehide',()=>{generation++;controller?.abort();close()});
 window.addEventListener('pageshow',event=>{if(event.persisted){generation++;controller?.abort();close();location.replace('/settings/preferences')}});
 select.addEventListener('change',()=>{if(state==='idle')void save()});
 retry.addEventListener('click',()=>{select.value=desired;sync();void save()});
 form.addEventListener('submit',event=>{event.preventDefault();void save()});
 sync();
 // Commit the enhancement last: initialization failures retain the native form.
 if(custom){select.hidden=true;label.htmlFor=trigger.id}
 fallbackSave.hidden=true;form.dataset.autosaveReady='true';
 if(location.hash==='#interface-language-picker')requestAnimationFrame(()=>{if(custom)trigger.focus();else select.focus()});
}
