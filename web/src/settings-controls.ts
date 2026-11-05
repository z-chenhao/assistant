import {t} from './i18n';

// Keep the native select as the form control and no-script fallback. On desktop,
// its popup adds search, checkmarks and keyboard navigation without copying values.
export function enhanceSettingsControls(){
 const page=document.querySelector('.settings-page');
 if(!page)return;
 const search=document.querySelector<HTMLElement>('[data-settings-search]');
 const field=search?.querySelector<HTMLInputElement>('input');
 if(search&&field){
  search.hidden=false;
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
  const sync=()=>{theme.value=read();document.documentElement.classList.toggle('dark',theme.value==='dark'||theme.value==='system'&&media.matches)};
  sync();theme.closest<HTMLElement>('[data-appearance-setting]')!.hidden=false;
  theme.addEventListener('change',()=>{try{if(theme.value==='system')localStorage.removeItem('steward-theme');else localStorage.setItem('steward-theme',theme.value)}catch{};document.documentElement.classList.toggle('dark',theme.value==='dark'||theme.value==='system'&&media.matches)});
  media.addEventListener('change',sync);window.addEventListener('storage',sync);
 }
 const language=page.querySelector<HTMLSelectElement>('#interface-language');
 const save=page.querySelector<HTMLButtonElement>('[data-language-save]');
 if(language&&save){save.hidden=true;language.addEventListener('change',()=>{save.hidden=language.value===Array.from(language.options).find(option=>option.defaultSelected)?.value})}
 let closeCurrent:(()=>void)|undefined;
 page.querySelectorAll<HTMLSelectElement>('select').forEach((select,index)=>{
  const wrapper=document.createElement('span');wrapper.className='settings-select-control';select.before(wrapper);wrapper.append(select);
  const menu=document.createElement('div');menu.className='settings-select-menu';menu.hidden=true;menu.id=`settings-select-${index}`;
  const input=document.createElement('input');input.type='search';input.placeholder=t('Search options');input.setAttribute('aria-label',t('Search options'));input.autocomplete='off';
  const list=document.createElement('div');list.className='settings-select-options';list.role='listbox';list.id=menu.id+'-options';
  const label=select.labels?.[0]?.textContent?.trim()||t('Options');list.setAttribute('aria-label',label);input.setAttribute('aria-controls',list.id);
  const empty=document.createElement('p');empty.className='settings-select-empty';empty.textContent=t('No results.');empty.role='status';empty.hidden=true;
  menu.append(input,list,empty);document.body.append(menu);
  select.setAttribute('aria-haspopup','listbox');select.setAttribute('aria-expanded','false');select.setAttribute('aria-controls',list.id);
  const close=(focus=false)=>{menu.hidden=true;select.setAttribute('aria-expanded','false');if(closeCurrent===dismiss)closeCurrent=undefined;if(focus)select.focus()};
  const dismiss=()=>close();
  const render=()=>{
   list.replaceChildren();
   const query=input.value.trim().toLocaleLowerCase();
   for(const option of select.options){
    if(option.disabled||!option.text.toLocaleLowerCase().includes(query))continue;
    const item=document.createElement('div');item.role='option';item.tabIndex=-1;item.textContent=option.text;item.setAttribute('aria-selected',String(option.selected));
    item.addEventListener('click',()=>{
     const changed=select.value!==option.value;select.value=option.value;close(true);
     if(changed){select.dispatchEvent(new Event('input',{bubbles:true}));select.dispatchEvent(new Event('change',{bubbles:true}))}
    });list.append(item);
   }
   empty.hidden=list.childElementCount>0;
  };
  const open=(keyboard=false)=>{
   if(select.disabled||matchMedia('(max-width: 760px)').matches)return;
   closeCurrent?.();input.value='';render();menu.hidden=false;select.setAttribute('aria-expanded','true');closeCurrent=dismiss;
   const rect=select.getBoundingClientRect(),width=Math.max(240,Math.min(320,rect.width));menu.style.width=`${width}px`;
   menu.style.left=`${Math.max(12,Math.min(innerWidth-width-12,rect.right-width))}px`;
   const height=menu.getBoundingClientRect().height;
   menu.style.top=`${Math.max(12,rect.bottom+height+6>innerHeight&&rect.top>height?rect.top-height-6:Math.min(rect.bottom+6,innerHeight-height-12))}px`;
   if(keyboard&&!select.hasAttribute('data-select-search')&&select.options.length<6)list.querySelector<HTMLElement>('[aria-selected=true]')?.focus();else input.focus();
  };
  select.addEventListener('mousedown',event=>{if(!matchMedia('(max-width: 760px)').matches)event.preventDefault()});
  select.addEventListener('click',event=>{if(matchMedia('(max-width: 760px)').matches)return;event.preventDefault();if(menu.hidden)open();else close(true)});
  select.addEventListener('keydown',event=>{
   if(matchMedia('(max-width: 760px)').matches)return;
   if(['ArrowDown','ArrowUp','Enter',' '].includes(event.key)){event.preventDefault();open(true)}
   if(event.key==='Escape')close(true);
  });
  menu.addEventListener('keydown',event=>{
   const items=Array.from(list.querySelectorAll<HTMLElement>('[role=option]'));
   const active=items.indexOf(document.activeElement as HTMLElement);
   if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close(true)}
   else if(event.key==='Tab'){
    event.preventDefault();close(true);
    const controls=Array.from(document.querySelectorAll<HTMLElement>('a[href],button,input,select,textarea,[tabindex="0"]')).filter(node=>node.getClientRects().length>0&&!node.matches(':disabled')&&!node.closest('[inert]'));
    const next=controls[controls.indexOf(select)+(event.shiftKey?-1:1)];next?.focus();
   }
   else if(event.key==='ArrowDown'||event.key==='ArrowUp'){
    event.preventDefault();const next=active<0?(event.key==='ArrowDown'?0:items.length-1):(active+(event.key==='ArrowDown'?1:-1)+items.length)%items.length;items[next]?.focus();
   }else if(event.key==='Enter'){
    event.preventDefault();(active<0?items[0]:items[active])?.click();
   }else if(event.key===' '&&active>=0){event.preventDefault();items[active]?.click()}
  });
  input.addEventListener('input',render);
  document.addEventListener('pointerdown',event=>{if(!menu.hidden&&!menu.contains(event.target as Node)&&event.target!==select)close()});
  document.addEventListener('focusin',event=>{if(!menu.hidden&&!menu.contains(event.target as Node)&&event.target!==select)close()});
  window.addEventListener('resize',dismiss);window.addEventListener('scroll',event=>{if(!menu.contains(event.target as Node))close()},true);
  new MutationObserver(()=>{if(select.disabled)close()}).observe(select,{attributes:true,attributeFilter:['disabled']});
 });
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
