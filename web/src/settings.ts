// Settings use native navigation; a submission has one explicit attempt.
export function enhanceSettings(){
 const forms=document.querySelectorAll<HTMLFormElement>('.settings-page form');
 const reset=new Map<HTMLFormElement,()=>void>();
 for(const form of forms){form.addEventListener('submit',event=>{
  if(form.dataset.submitting){event.preventDefault();return}
  const buttons=Array.from(form.querySelectorAll<HTMLButtonElement>('button[type=submit],button:not([type])'));
  const labels=buttons.map(button=>button.textContent??'');
  form.dataset.submitting='true';form.setAttribute('aria-busy','true');
  for(const button of buttons){button.disabled=true;button.textContent=form.action.endsWith('/model/test')?'Verifying…':'Working…'}
  reset.set(form,()=>{delete form.dataset.submitting;form.removeAttribute('aria-busy');buttons.forEach((button,i)=>{button.disabled=false;button.textContent=labels[i]})});
 })}
 window.addEventListener('pageshow',event=>{if(event.persisted){for(const restore of reset.values())restore();reset.clear()}});
}
