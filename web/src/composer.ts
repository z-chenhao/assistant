import {buttonVariants,textareaStyle} from './components/ui/styles';
import {cn} from './lib/utils';

export function enhanceComposer(){
 const root=document.querySelector<HTMLElement>('#composer-root');
 if(!root?.dataset.sourceId)return;
 const form=root.querySelector<HTMLFormElement>('form');
 const text=root.querySelector<HTMLTextAreaElement>('textarea');
 const submit=root.querySelector<HTMLButtonElement>('button[type="submit"]');
 const file=root.querySelector<HTMLInputElement>('input[type=file]');
 const remove=root.querySelector<HTMLButtonElement>('[data-remove-file]');
 const status=root.querySelector<HTMLElement>('.attachment-status');
 if(text)text.className=cn(textareaStyle,text.className);
 if(submit)submit.className=cn(buttonVariants({size:'icon'}),submit.className);
 // Native nodes survive enhancement, including a picker opened while JS loads.
 const showFile=()=>{if(status)status.textContent=file?.files?.[0]?.name??'';if(remove)remove.hidden=!file?.files?.length};
 file?.addEventListener('change',showFile);remove?.addEventListener('click',()=>{if(file)file.value='';showFile()});showFile();
 root.querySelector('[data-clear-task]')?.addEventListener('click',()=>{root.querySelector('.task-context-chip')?.remove();const url=new URL(location.href);url.searchParams.delete('task');history.replaceState(null,'',url)});
 let pending=false;
 form?.addEventListener('submit',event=>{
  event.preventDefault();if(pending||submit?.disabled)return;
  pending=true;form.dataset.submitting='true';if(submit)submit.disabled=true;
  const feedback=root.querySelector<HTMLElement>('#composer-feedback');
  if(feedback)feedback.textContent='Sending…';
  void(async()=>{
   const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),15000);
   try{
    const response=await fetch('/message',{method:'POST',body:new FormData(form),credentials:'same-origin',signal:controller.signal,headers:{Accept:'application/json'}});
    if(!response.ok)throw Error('send not confirmed');
    const result=await response.json();if(result.accepted!==true||typeof result.location!=='string')throw Error('invalid acknowledgement');
    const target=new URL(result.location,location.origin);if(target.origin!==location.origin||target.pathname!=='/')throw Error('invalid destination');
    location.assign(target);
   }catch{if(feedback)feedback.textContent='Send was not confirmed. Your draft and attachment are kept. Check the conversation before sending again.';pending=false;delete form.dataset.submitting;if(submit)submit.disabled=document.querySelector<HTMLElement>('[data-can-send]')?.dataset.canSend==='false'}
   finally{clearTimeout(timeout)}
  })();
 });
}
