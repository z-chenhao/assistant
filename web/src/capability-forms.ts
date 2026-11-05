import {t} from './i18n';

// This is feedback only. The server owns exact-candidate verification and will
// reject an edited, expired or stale configuration even without JavaScript.
export function enhanceCapabilityForms(){
 document.querySelectorAll<HTMLFormElement>('[data-capability-form]').forEach(form=>{
  const enabled=form.querySelector<HTMLInputElement>('input[name="enabled"]');
  const mode=form.querySelector<HTMLSelectElement>('[data-capability-mode]');
  const syncEnabled=()=>{
   if(!enabled&&!mode)return;
   const active=mode?mode.value==='custom':enabled!.checked;
   const check=form.querySelector<HTMLButtonElement>('[data-capability-check]');if(check)check.hidden=!active;
   if(mode){const fields=form.querySelector<HTMLElement>('[data-capability-fields]');if(fields)fields.hidden=!active}
   form.querySelectorAll<HTMLInputElement|HTMLSelectElement|HTMLTextAreaElement|HTMLButtonElement>('[data-capability-fields] input:not([type="hidden"]),[data-capability-fields] select,[data-capability-fields] textarea,[data-capability-fields] button').forEach(control=>{control.disabled=!active});
  };
  syncEnabled();
  const snapshot=()=>JSON.stringify(Array.from(new FormData(form).entries()));
  const initial=snapshot();
  const changed=()=>{
   form.querySelector<HTMLButtonElement>('button[value="save"]')?.setAttribute('disabled','');
   const status=form.querySelector<HTMLElement>('[data-capability-draft]');
   if(status)status.textContent=snapshot()===initial?'':t('Unsaved changes.');
  };
  form.addEventListener('input',changed);form.addEventListener('change',()=>{syncEnabled();changed()});
 });
}
