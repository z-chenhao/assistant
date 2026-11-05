import {t} from './i18n';
import {confirmSettingsAction,enhanceSettingsControls} from './settings-controls';
import {enhanceLanguagePreference} from './preferences';

// The fallback is a native disclosure. Enhance only after the whole help is ready.
function enhancePreferenceHelp(){
 for(const help of document.querySelectorAll<HTMLElement>('[data-preference-help]')){
  const trigger=help.querySelector<HTMLButtonElement>('[data-help-trigger]');
  const tooltip=help.querySelector<HTMLElement>('[data-help-tooltip]');
  const fallback=help.querySelector<HTMLDetailsElement>('[data-help-fallback]');
  if(!trigger||!tooltip||!fallback)continue;
  let hovered=false,pinned=false,dismissed=false;
  const render=()=>{tooltip.hidden=dismissed||!(hovered||pinned||document.activeElement===trigger);if(!tooltip.hidden)document.dispatchEvent(new Event('preference-help-open'))};
  document.addEventListener('control-select-open',()=>{pinned=false;dismissed=true;render()});
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

const signInOpenedHash='#auth-window-opened';

async function openSignIn(popup:Window, form:HTMLFormElement){
 let returnPath=location.pathname+location.search;
 let opened=false;
 try{
  const signal=AbortSignal.timeout(8000);
  const body=new URLSearchParams();
  for(const [name,value] of new FormData(form))if(typeof value==='string')body.append(name,value);
  const response=await fetch(form.action,{method:'POST',body,signal});
  if(!response.ok)return;
  const target=new URL(response.url);
  if(target.origin===location.origin&&(target.pathname==='/settings/setup'||target.pathname==='/settings/models/new'))returnPath=target.pathname+target.search;
  while(!popup.closed&&!signal.aborted){
   const response=await fetch('/api/auth',{signal,cache:'no-store'});
   if(!response.ok)return;
   const state=await response.json();
   if(state.status!=='pending')return;
   const event=state.events?.find((event:{kind:string,url?:string})=>event.kind==='open_url'&&event.url);
   if(event){
    const url=new URL(event.url);
    if(url.protocol!=='https:'||url.username||url.password)return;
    popup.location.replace(url.href);
    opened=true;
    return;
   }
   await new Promise(resolve=>setTimeout(resolve,150));
  }
 }catch{
  // The local sign-in page remains the fallback for a blocked or failed launch.
 }finally{
  if(!opened)popup.close();
  location.assign(opened?returnPath+signInOpenedHash:returnPath);
 }
}

// Settings use native navigation; a submission has one explicit attempt.
export function enhanceSettings(){
 enhancePreferenceHelp();
 try{enhanceLanguagePreference()}finally{document.querySelector('[data-language-preference]')?.setAttribute('data-language-ready','')}
 enhanceSettingsControls();
 if(location.hash===signInOpenedHash){
  const signIn=document.querySelector<HTMLElement>('#signin-status');
  if(signIn?.querySelector('[data-signin-pending]'))signIn.dataset.authWindowOpened='';
  history.replaceState(history.state,'',location.pathname+location.search);
 }
 const provider=document.querySelector<HTMLFormElement>('[data-model-provider]');
 const select=provider?.querySelector<HTMLSelectElement>('select');
 if(provider&&select){
  const panels=document.querySelectorAll<HTMLElement>('[data-model-provider-panel]');
  const showSelected=()=>{for(const panel of panels)panel.hidden=panel.dataset.modelProviderPanel!==select.value};
  provider.addEventListener('submit',event=>event.preventDefault());
  select.addEventListener('change',showSelected);
  showSelected();
 }
 const forms=document.querySelectorAll<HTMLFormElement>('.settings-page form');
 const reset=new Map<HTMLFormElement,()=>void>();
 for(const form of forms){form.addEventListener('submit',event=>{
  if(event.defaultPrevented||form.dataset.autosaveReady)return;
  if(form.dataset.submitting){event.preventDefault();return}
  if(confirmSettingsAction(form,(event as SubmitEvent).submitter)){event.preventDefault();return}
  if(form.hasAttribute('data-open-signin')){
   const popup=window.open('about:blank','_blank');
   if(popup){
    popup.opener=null;
    event.preventDefault();
    void openSignIn(popup,form);
   }
  }
  const buttons=Array.from(form.querySelectorAll<HTMLButtonElement>('button[type=submit],button:not([type])'));
  const labels=buttons.map(button=>button.textContent??'');
  const disabled=buttons.map(button=>button.disabled);
  const submitter=(event as SubmitEvent).submitter;
  let action:HTMLInputElement|undefined;
  // Disabled controls are omitted from native form submission. Preserve the
  // clicked action before applying the double-submit guard.
  if(submitter instanceof HTMLButtonElement&&submitter.name){action=document.createElement('input');action.type='hidden';action.name=submitter.name;action.value=submitter.value;form.append(action)}
  form.dataset.submitting='true';form.setAttribute('aria-busy','true');
  const verifying=form.hasAttribute('data-model-verify')||form.getAttribute('action')?.endsWith('/model/test')&&!submitter?.hasAttribute('data-model-restore');
  for(const button of buttons){button.disabled=true;if(!form.hasAttribute('data-preserve-submit-label'))button.textContent=verifying?t('Verifying…'):t('Working…')}
  reset.set(form,()=>{action?.remove();delete form.dataset.submitting;form.removeAttribute('aria-busy');buttons.forEach((button,i)=>{button.disabled=disabled[i];button.textContent=labels[i]})});
 })}
 window.addEventListener('pageshow',event=>{if(event.persisted){for(const restore of reset.values())restore();reset.clear()}});
}
