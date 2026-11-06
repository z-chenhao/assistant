import {t} from './i18n';
import {enhanceSelect} from './components/ui/select';

export function enhanceNotificationPreference(){
 const form=document.querySelector<HTMLFormElement>('[data-notification-preference]');
 if(!form||typeof fetch!=='function'||typeof AbortController!=='function')return;
 const select=form.querySelector<HTMLSelectElement>('select[name=target]');
 const version=form.querySelector<HTMLInputElement>('input[name=version]');
 const fallback=form.querySelector<HTMLElement>('[data-notification-fallback]');
 const feedback=document.querySelector<HTMLElement>('[data-notification-feedback]');
 const message=feedback?.querySelector<HTMLElement>('[data-notification-message]');
 const refresh=feedback?.querySelector<HTMLElement>('[data-notification-refresh]');
 if(!select||!version||!fallback||!feedback||!message||!refresh)return;
 const picker=enhanceSelect(select);
 let confirmed=select.value,state:'idle'|'saving'|'unknown'='idle',generation=0;
 let controller:AbortController|undefined;
 const lock=(value:boolean)=>{picker?.setLocked(value,state==='saving');if(!picker)select.disabled=value};
 const report=(text:string,error=false)=>{feedback.hidden=false;feedback.setAttribute('role',error?'alert':'status');message.textContent=t(text);refresh.hidden=!error};
 const unknown=()=>{state='unknown';lock(true);report('Save result not confirmed.',true)};
 const save=async()=>{
  if(state!=='idle'||select.value===confirmed)return;
  const desired=select.value,revision=Number(version.value),attempt=++generation;
  state='saving';picker?.close();lock(true);report('Saving…');
  controller=new AbortController();const timeout=window.setTimeout(()=>controller?.abort(),15000);
  try{
   const response=await fetch(form.action,{method:'POST',credentials:'same-origin',redirect:'error',headers:{Accept:'application/json'},body:new URLSearchParams({target:desired,version:version.value}),signal:controller.signal});
   if(attempt!==generation)return;
   if([400,401,403,409,422].includes(response.status)){
    // No blind retry: a competing tab may have advanced the revision.
    state='unknown';lock(true);
    report(response.status===409?'Settings changed elsewhere. Refresh to check.':'Notification preference could not be saved.',true);return;
   }
   if(response.status!==200||!response.headers.get('Content-Type')?.includes('application/json')){unknown();return}
   const result=await response.json();if(attempt!==generation)return;
   if(result?.target!==desired||result?.version!==revision+1){unknown();return}
   confirmed=desired;version.value=String(result.version);state='idle';lock(false);feedback.hidden=true;
   document.querySelector<HTMLElement>('.page-feedback.success')?.setAttribute('hidden','');
  }catch{if(attempt===generation)unknown()}finally{window.clearTimeout(timeout)}
 };
 select.addEventListener('change',()=>void save());
 form.addEventListener('submit',event=>{event.preventDefault();void save()});
 window.addEventListener('pagehide',()=>{generation++;controller?.abort();picker?.close()});
 window.addEventListener('pageshow',event=>{if(event.persisted)location.replace('/settings/notifications')});
 fallback.hidden=true;form.dataset.autosaveReady='true';
}
