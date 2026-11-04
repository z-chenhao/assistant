import {t} from './i18n';

// The fallback is a native disclosure. Enhance only after the whole help is ready.
function enhancePreferenceHelp(){
 for(const help of document.querySelectorAll<HTMLElement>('[data-preference-help]')){
  const trigger=help.querySelector<HTMLButtonElement>('[data-help-trigger]');
  const tooltip=help.querySelector<HTMLElement>('[data-help-tooltip]');
  const fallback=help.querySelector<HTMLDetailsElement>('[data-help-fallback]');
  if(!trigger||!tooltip||!fallback)continue;
  let hovered=false,pinned=false,dismissed=false;
  const render=()=>{tooltip.hidden=dismissed||!(hovered||pinned||document.activeElement===trigger)};
  help.addEventListener('pointerenter',event=>{if(event.pointerType==='touch')return;hovered=true;dismissed=false;render()});
  help.addEventListener('pointerleave',()=>{hovered=false;render()});
  trigger.addEventListener('focus',()=>{dismissed=false;render()});
  trigger.addEventListener('blur',()=>{pinned=false;render()});
  // Focus can reveal the tooltip before a touch/keyboard click. Toggle the
  // explicit pin, not visibility, so that first activation never closes it.
  trigger.addEventListener('click',()=>{pinned=!pinned;dismissed=!pinned;render()});
  document.addEventListener('pointerdown',event=>{if(event.target instanceof Node&&!help.contains(event.target)){pinned=false;dismissed=true;render()}});
  document.addEventListener('keydown',event=>{if(event.key==='Escape'&&!tooltip.hidden){pinned=false;dismissed=true;render()}});
  window.addEventListener('pageshow',event=>{if(event.persisted){pinned=false;dismissed=true;render()}});
  fallback.hidden=true;trigger.hidden=false;
 }
}

// Settings use native navigation; a submission has one explicit attempt.
export function enhanceSettings(){
 enhancePreferenceHelp();
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
  for(const button of buttons){button.disabled=true;if(!form.hasAttribute('data-preserve-submit-label'))button.textContent=form.getAttribute('action')?.endsWith('/model/test')?t('Verifying…'):t('Working…')}
  reset.set(form,()=>{action?.remove();delete form.dataset.submitting;form.removeAttribute('aria-busy');buttons.forEach((button,i)=>{button.disabled=disabled[i];button.textContent=labels[i]})});
 })}
 window.addEventListener('pageshow',event=>{if(event.persisted){for(const restore of reset.values())restore();reset.clear()}});
}
