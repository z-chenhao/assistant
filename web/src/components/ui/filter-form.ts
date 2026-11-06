// Read-only GET filters share one enhancement; native submission remains the
// fallback. Explicit saves elsewhere do not opt in to this behavior.
export function enhanceFilterForms(){
 const forms=document.querySelectorAll<HTMLFormElement>('form[data-auto-filter]');
 for(const form of forms){
  if(form.method.toLowerCase()!=='get')continue;
  const rail=form.querySelector<HTMLDetailsElement>('.obs-filter-rail'),key='steward.filters.open:'+location.pathname;
  try{const saved=sessionStorage.getItem(key);sessionStorage.removeItem(key);if(rail&&saved!==null)rail.open=saved==='true'}catch{}
  for(const button of form.querySelectorAll<HTMLButtonElement>('button[data-filter-submit]'))button.hidden=true;
  form.addEventListener('change',event=>{
   const target=event.target;
   if(!(target instanceof HTMLInputElement||target instanceof HTMLSelectElement))return;
   if(target.disabled||!target.name||target.name==='search'||!form.reportValidity())return;
   try{if(rail)sessionStorage.setItem(key,String(rail.open))}catch{}
   form.requestSubmit();
  });
 }
}
