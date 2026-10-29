// Settings use native navigation; a submission has one explicit attempt.
export function enhanceSettings(){
 const forms=document.querySelectorAll<HTMLFormElement>('.settings-page form');
 const reset=new Map<HTMLFormElement,()=>void>();
 for(const form of forms){form.addEventListener('submit',event=>{
  if(form.dataset.submitting){event.preventDefault();return}
  const buttons=Array.from(form.querySelectorAll<HTMLButtonElement>('button[type=submit],button:not([type])'));
  const labels=buttons.map(button=>button.textContent??'');
  const disabled=buttons.map(button=>button.disabled);
  const submitter=(event as SubmitEvent).submitter;
  let action:HTMLInputElement|undefined;
  // Disabled controls are omitted from native form submission. Preserve the
  // clicked action before applying the double-submit guard.
  if(submitter instanceof HTMLButtonElement&&submitter.name){action=document.createElement('input');action.type='hidden';action.name=submitter.name;action.value=submitter.value;form.append(action)}
  form.dataset.submitting='true';form.setAttribute('aria-busy','true');
  for(const button of buttons){button.disabled=true;button.textContent=form.getAttribute('action')?.endsWith('/model/test')?'Verifying…':'Working…'}
  reset.set(form,()=>{action?.remove();delete form.dataset.submitting;form.removeAttribute('aria-busy');buttons.forEach((button,i)=>{button.disabled=disabled[i];button.textContent=labels[i]})});
 })}
 window.addEventListener('pageshow',event=>{if(event.persisted){for(const restore of reset.values())restore();reset.clear()}});
}
