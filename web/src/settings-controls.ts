import {t} from './i18n';

// Business behavior consumes native form values; shared Base UI controls own UI.
export function enhanceSettingsControls(){
 const page=document.querySelector('.settings-page');
 if(!page)return;
 const theme=document.querySelector<HTMLSelectElement>('#interface-theme');
 if(theme){
  const media=matchMedia('(prefers-color-scheme: dark)');
  const read=()=>{try{return localStorage.getItem('steward-theme')??'system'}catch{return 'system'}};
  const sync=()=>{theme.value=read();document.documentElement.classList.toggle('dark',theme.value==='dark'||theme.value==='system'&&media.matches);theme.dispatchEvent(new Event('control-value-sync'))};
  sync();theme.closest<HTMLElement>('[data-appearance-setting]')!.removeAttribute('data-client-pending');
  theme.addEventListener('change',()=>{try{if(theme.value==='system')localStorage.removeItem('steward-theme');else localStorage.setItem('steward-theme',theme.value)}catch{};document.documentElement.classList.toggle('dark',theme.value==='dark'||theme.value==='system'&&media.matches)});
  media.addEventListener('change',sync);window.addEventListener('storage',sync);
 }

}

export function confirmSettingsAction(form:HTMLFormElement,submitter:HTMLElement|null){
 const title=form.dataset.settingsConfirm;
 if(!title||form.dataset.confirmed==='true'){delete form.dataset.confirmed;return false}
 const focusReturn=document.getElementById(form.dataset.controlMenuOpener??'')??submitter;
 const dialog=document.createElement('dialog');dialog.className='settings-confirm';
 const heading=document.createElement('h2');heading.id='settings-confirm-title';heading.textContent=title;dialog.setAttribute('aria-labelledby',heading.id);
 const copy=document.createElement('p');copy.id='settings-confirm-description';copy.textContent=form.dataset.confirmDescription??'';dialog.setAttribute('aria-describedby',copy.id);
 const actions=document.createElement('div');actions.className='actions';
 const cancel=document.createElement('button');cancel.type='button';cancel.className='secondary';cancel.textContent=t('Cancel');cancel.autofocus=true;
 const confirm=document.createElement('button');confirm.type='button';confirm.className='danger-text';confirm.textContent=title;
 actions.append(cancel,confirm);dialog.append(heading,copy,actions);document.body.append(dialog);
 cancel.addEventListener('click',()=>dialog.close());
 confirm.addEventListener('click',()=>{dialog.close();form.dataset.confirmed='true';form.requestSubmit(submitter instanceof HTMLButtonElement?submitter:undefined)});
 dialog.addEventListener('close',()=>{dialog.remove();focusReturn?.focus()});
 dialog.addEventListener('click',event=>{if(event.target!==dialog)return;const rect=dialog.getBoundingClientRect();if(event.clientX<rect.left||event.clientX>rect.right||event.clientY<rect.top||event.clientY>rect.bottom)dialog.close()});
 dialog.showModal();return true;
}
