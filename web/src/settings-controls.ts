import {t} from './i18n';

// Business behavior consumes native form values; shared Base UI controls own UI.
export function enhanceSettingsControls(){
 const page=document.querySelector('.settings-page');
 if(!page)return;
 const notification=page.querySelector<HTMLFormElement>('[data-notification-preference]');
 if(notification){
  const initial=JSON.stringify(Array.from(new FormData(notification).entries()));
  const feedback=document.querySelector<HTMLElement>('.page-feedback.success');
  const draft=page.querySelector<HTMLElement>('[data-notification-draft]');
  const sync=()=>{
   const dirty=JSON.stringify(Array.from(new FormData(notification).entries()))!==initial;
   if(feedback)feedback.hidden=dirty;
   if(draft)draft.hidden=!dirty;
  };
  notification.addEventListener('input',sync);notification.addEventListener('change',sync);window.addEventListener('pageshow',sync);
 }
 const search=document.querySelector<HTMLElement>('[data-settings-search]');
 const field=search?.querySelector<HTMLInputElement>('input');
 if(search&&field){
  search.removeAttribute('data-client-pending');
  const links=Array.from(document.querySelectorAll<HTMLAnchorElement>('.settings-navigation a'));
  const filter=()=>{
   const query=field.value.trim().toLocaleLowerCase();
   for(const link of links)link.hidden=!(link.textContent??'').toLocaleLowerCase().includes(query);
   const empty=document.querySelector<HTMLElement>('.settings-no-results');if(empty)empty.hidden=links.some(link=>!link.hidden);
  };
  field.addEventListener('input',filter);
  field.addEventListener('keydown',event=>{if(event.key==='Escape'){field.value='';filter()}if(event.key==='Enter'){event.preventDefault();links.find(link=>!link.hidden)?.click()}});
 }
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
 const dialog=document.createElement('dialog');dialog.className='settings-confirm';
 const heading=document.createElement('h2');heading.id='settings-confirm-title';heading.textContent=title;dialog.setAttribute('aria-labelledby',heading.id);
 const copy=document.createElement('p');copy.id='settings-confirm-description';copy.textContent=form.dataset.confirmDescription??'';dialog.setAttribute('aria-describedby',copy.id);
 const actions=document.createElement('div');actions.className='actions';
 const cancel=document.createElement('button');cancel.type='button';cancel.className='secondary';cancel.textContent=t('Cancel');cancel.autofocus=true;
 const confirm=document.createElement('button');confirm.type='button';confirm.className='danger-text';confirm.textContent=title;
 actions.append(cancel,confirm);dialog.append(heading,copy,actions);document.body.append(dialog);
 cancel.addEventListener('click',()=>dialog.close());
 confirm.addEventListener('click',()=>{dialog.close();form.dataset.confirmed='true';form.requestSubmit(submitter instanceof HTMLButtonElement?submitter:undefined)});
 dialog.addEventListener('close',()=>{dialog.remove();submitter?.focus()});
 dialog.addEventListener('click',event=>{if(event.target!==dialog)return;const rect=dialog.getBoundingClientRect();if(event.clientX<rect.left||event.clientX>rect.right||event.clientY<rect.top||event.clientY>rect.bottom)dialog.close()});
 dialog.showModal();return true;
}
