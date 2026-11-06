import {t} from './i18n';

// Save one exact candidate at a time. The Host still validates, probes and CASes
// the configuration; changing controls while a request is pending queues a save.
export function enhanceCapabilityForms(){
 const forms=Array.from(document.querySelectorAll<HTMLFormElement>('[data-capability-form]'));
 const baseline=new Map<HTMLFormElement,string>();
 const values=(form:HTMLFormElement)=>{
  const data=new URLSearchParams();
  for(const [key,value] of new FormData(form))if(typeof value==='string')data.append(key,value);
  return data;
 };
 const fields=(form:HTMLFormElement)=>{const data=values(form);data.delete('revision');data.delete('configuration');return data.toString()};
 for(const form of forms){
  const enabled=form.querySelector<HTMLInputElement>('input[name="enabled"]');
  const mode=form.querySelector<HTMLSelectElement>('[data-capability-mode]');
  const status=form.querySelector<HTMLElement>('[data-capability-draft]');
  const yaml=form.querySelector<HTMLTextAreaElement>('textarea[name="configuration"]');
  const file=form.querySelector<HTMLInputElement>('[data-capability-import]');
  const sync=()=>{
   if(!enabled&&!mode)return;
   const active=mode?mode.value==='custom':enabled!.checked;
   const group=form.querySelector<HTMLElement>('[data-capability-fields]');if(group)group.hidden=!active;
   form.querySelectorAll<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement|HTMLButtonElement>('[data-capability-fields] input:not([type="hidden"]),[data-capability-fields] select,[data-capability-fields] textarea,[data-capability-fields] button').forEach(control=>{control.disabled=!active});
  };
  let pending=false,queued=false,timer:ReturnType<typeof setTimeout>|undefined;
  const feedback=(message:string,error=false)=>{
   if(!status)return;
   status.replaceChildren(document.createTextNode(t(message)));status.setAttribute('role',error?'alert':'status');
   if(error){const retry=document.createElement('button');retry.type='button';retry.className='secondary setting-action';retry.textContent=t('Retry');retry.addEventListener('click',()=>void save());const refresh=document.createElement('a');refresh.href=location.pathname;refresh.textContent=t('Refresh to check');status.append(' ',retry,' ',refresh)}
  };
  const save=async()=>{
   clearTimeout(timer);sync();
   if(pending){queued=true;return}
   if(!form.checkValidity())return;
   const data=values(form),sent=data.toString(),sentFields=fields(form),sentYAML=data.get('configuration')??'';
   if(sent===baseline.get(form)){feedback('Saved.');return}
   data.set('action','apply');pending=true;queued=false;form.setAttribute('aria-busy','true');feedback('Saving…');
   const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),30000);
   let saved=false;
   try{
    const response=await fetch(form.action,{method:'POST',body:data,credentials:'same-origin',signal:controller.signal});
    const parsed=new DOMParser().parseFromString(await response.text(),'text/html');
    if(!response.ok){const message=parsed.querySelector('.feedback.error')?.textContent?.trim();throw Error(message||t('Could not save. Your changes are kept.'))}
    const next=new URL(response.url),revision=parsed.querySelector<HTMLInputElement>('[data-capability-form] [name="revision"]')?.value;
    if(next.origin!==location.origin||next.pathname!==location.pathname||!revision)throw Error(t('Could not confirm the save. Refresh to check.'));
    const canonical=parsed.querySelector<HTMLInputElement>('.capability-guided [name="configuration"]')?.value??parsed.querySelector<HTMLTextAreaElement>('.capability-editor [name="configuration"]')?.value??sentYAML;
    for(const other of forms){
     other.querySelector<HTMLInputElement>('[name="revision"]')!.value=revision;
     const stored=other.querySelector<HTMLInputElement>('input[name="configuration"]');if(stored)stored.value=canonical;
     const editor=other.querySelector<HTMLTextAreaElement>('textarea[name="configuration"]');if(editor&&(!editor.dataset.edited||editor===yaml&&editor.value===sentYAML)){editor.value=canonical;delete editor.dataset.edited}
    }
    const current=document.querySelector('.capabilities-page>.page-heading .status-pill'),updated=parsed.querySelector('.capabilities-page>.page-heading .status-pill');
    if(current&&updated)current.replaceWith(updated);
    saved=true;
    if(fields(form)!==sentFields||yaml&&yaml.value!==canonical)queued=true;
    else{for(const other of forms)baseline.set(other,values(other).toString());feedback('Saved.')}
   }catch(error){feedback(error instanceof Error?error.message:'Could not save. Your changes are kept.',true)}
   finally{clearTimeout(timeout);pending=false;form.removeAttribute('aria-busy');if(saved&&queued)void save()}
  };
  const changed=(event:Event)=>{
   sync();document.querySelectorAll<HTMLElement>('[data-capability-feedback]').forEach(node=>{node.hidden=true});
   if(yaml&&event.target===yaml)yaml.dataset.edited='true';
   if(event.target===mode&&mode?.value==='custom'){feedback('Complete the settings below. Changes save automatically.');return}
   clearTimeout(timer);timer=setTimeout(()=>void save(),event.type==='input'?700:0);
  };
  sync();baseline.set(form,values(form).toString());
  form.addEventListener('change',event=>{if(event.target!==file)changed(event)});
  form.addEventListener('input',event=>{if(event.target!==file&&!(event.target instanceof HTMLSelectElement))changed(event)});
  form.addEventListener('submit',event=>{const submitter=(event as SubmitEvent).submitter;if(submitter instanceof HTMLButtonElement&&submitter.name==='action'&&submitter.value!=='apply')return;event.preventDefault();void save()});
  file?.addEventListener('change',()=>{void(async()=>{
   const selected=file.files?.[0];if(!selected||!yaml)return;
   if(selected.size>65536){feedback('Choose a YAML file smaller than 64 KiB.',true);return}
   try{yaml.value=await selected.text();yaml.dataset.edited='true';feedback('Checking configuration…');await save()}catch{feedback('Could not read the YAML file. Choose it again.',true)}
  })()});
  form.querySelector<HTMLButtonElement>('[data-capability-example]')?.addEventListener('click',event=>{
   const button=event.currentTarget as HTMLButtonElement;
   const href=URL.createObjectURL(new Blob([button.dataset.capabilityExample??''],{type:'application/yaml'}));
   const link=document.createElement('a');link.href=href;link.download=(form.querySelector<HTMLInputElement>('[name="capability"]')?.value??'capability')+'.yaml';link.click();setTimeout(()=>URL.revokeObjectURL(href),1000);
  });
  form.querySelectorAll('[data-client-pending]').forEach(node=>node.removeAttribute('data-client-pending'));
  form.dataset.autosaveReady='true';
 }
}
