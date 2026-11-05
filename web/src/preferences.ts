import {t} from './i18n';

// One native value source, one in-flight write. Unknown results require a GET;
// neither returning from BFCache nor a late response may replay the POST.
export function enhanceLanguagePreference(){
 const form=document.querySelector<HTMLFormElement>('[data-language-preference]');
 if(!form||typeof fetch!=='function'||typeof AbortController!=='function')return;
 const select=form.querySelector<HTMLSelectElement>('select[name=language]');
 const trigger=form.querySelector<HTMLButtonElement>('[data-language-trigger]');
 const value=form.querySelector<HTMLElement>('#language-value');
 const popup=form.querySelector<HTMLElement>('[data-language-popup]');
 const label=form.querySelector<HTMLLabelElement>('#language-label');
 const fallbackSave=form.querySelector<HTMLButtonElement>('.preference-save');
 const feedback=form.querySelector<HTMLElement>('[data-language-feedback]');
 const message=form.querySelector<HTMLElement>('[data-language-message]');
 const retry=form.querySelector<HTMLButtonElement>('[data-language-retry]');
 const refresh=form.querySelector<HTMLAnchorElement>('[data-language-refresh]');
 const options=Array.from(form.querySelectorAll<HTMLElement>('[data-language-option]'));
 if(!select||!trigger||!value||!popup||!label||!fallbackSave||!feedback||!message||!retry||!refresh||options.length!==select.options.length)return;
 const search=form.querySelector<HTMLInputElement>('[data-language-search]');
 const empty=form.querySelector<HTMLElement>('[data-language-empty]');
 const custom=typeof popup.showPopover==='function'&&typeof popup.hidePopover==='function';
 let confirmed=select.value,desired=confirmed,active=0,opened=false;
 let state:'idle'|'saving'|'unknown'='idle';
 let generation=0;
 let controller:AbortController|undefined;
 const sync=()=>{
  value.textContent=select.selectedOptions[0]?.textContent??'';
  for(const option of options)option.setAttribute('aria-selected',String(option.dataset.languageOption===select.value));
 };
 const close=(focus=false)=>{
  if(opened)popup.hidePopover();
  opened=false;trigger.setAttribute('aria-expanded','false');trigger.removeAttribute('aria-activedescendant');
  if(focus)trigger.focus();
 };
 const markActive=(index:number)=>{
  active=(index+options.length)%options.length;
  options.forEach((option,i)=>option.toggleAttribute('data-active',i===active));
  trigger.setAttribute('aria-activedescendant',options[active].id);
  options[active].scrollIntoView({block:'nearest'});
 };
 const position=()=>{
  if(!opened)return;
  const bounds=trigger.getBoundingClientRect();
  const scale=popup.getBoundingClientRect().width/popup.offsetWidth||1;
  const viewport=window.visualViewport;
  const left=viewport?.offsetLeft??0,top=viewport?.offsetTop??0;
  const right=left+(viewport?.width??innerWidth),bottom=top+(viewport?.height??innerHeight);
  const width=Math.min(Math.max(bounds.width,168*scale),right-left-16*scale);
  popup.style.width=width/scale+'px';
  const below=Math.max(0,bottom-bounds.bottom-12*scale),above=Math.max(0,bounds.top-top-12*scale);
  const up=below<Math.min(popup.scrollHeight*scale,140*scale)&&above>below;
  popup.style.maxHeight=Math.max(0,up?above:below)/scale+'px';
  popup.style.left=Math.max(left+8*scale,Math.min(bounds.right-width,right-width-8*scale))/scale+'px';
  popup.style.top=(up?bounds.top-popup.getBoundingClientRect().height-4*scale:bounds.bottom+4*scale)/scale+'px';
 };
 const open=()=>{
  if(!custom||state!=='idle')return;
  document.dispatchEvent(new Event('preference-picker-open'));
  if(search)search.value='';for(const option of options)option.hidden=false;if(empty)empty.hidden=true;
  popup.showPopover();opened=true;trigger.setAttribute('aria-expanded','true');
  position();markActive(Math.max(0,options.findIndex(option=>option.dataset.languageOption===select.value)));
 };
 const report=(text:string,alert=false,action:'retry'|'refresh'|''='')=>{
  feedback.hidden=false;feedback.setAttribute('role',alert?'alert':'status');
  message.textContent=t(text);retry.hidden=action!=='retry';refresh.hidden=action!=='refresh';
 };
 const busy=(locked:boolean)=>{
  // Keep the feedback live region outside the busy subtree, including when an
  // unknown result locks further writes after the network operation has ended.
  trigger.setAttribute('aria-busy',String(state==='saving'));
  trigger.setAttribute('aria-disabled',String(locked));
  if(!custom)select.disabled=locked;
 };
 const unknown=()=>{
  state='unknown';busy(true);
  report('Save result not confirmed.',true,'refresh');
 };
 const save=async()=>{
  if(state!=='idle')return;
  desired=select.value;
  if(desired===confirmed)return;
  const attempt=++generation;
  state='saving';close();sync();busy(true);report('Saving…');
  controller=new AbortController();
  const timeout=window.setTimeout(()=>controller?.abort(),15000);
  try{
   const response=await fetch(form.action,{method:'POST',credentials:'same-origin',redirect:'error',headers:{Accept:'application/json'},body:new URLSearchParams({language:desired}),signal:controller.signal});
   if(attempt!==generation)return;
   // These pre-write rejections preserve the previous cookie. Other statuses
   // and malformed responses cannot prove what happened to the preference.
   if(response.status===400||response.status===401||response.status===403){
    select.value=confirmed;sync();state='idle';busy(false);
    if(response.status===400)report('Invalid language. Choose English or Simplified Chinese.',true,'retry');
    else report('Language could not be saved. Reopen settings to check access.',true,'refresh');
    return;
   }
   if(response.status!==200||!response.headers.get('Content-Type')?.includes('application/json')){unknown();return}
   const result:unknown=await response.json();
   if(attempt!==generation)return;
   if(typeof result!=='object'||result===null||!('language' in result)||result.language!==desired){unknown();return}
   // GET reads the cookie back. The server suppresses success if it did not
   // persist; the fragment restores focus only for this explicit operation.
   location.assign('/settings/preferences?notice=language-saved&language='+encodeURIComponent(desired)+'#interface-language-picker');
  }catch{
   if(attempt===generation)unknown();
  }finally{
   window.clearTimeout(timeout);
  }
 };
 const visible=()=>options.map((option,index)=>({option,index})).filter(({option})=>!option.hidden);
 const move=(key:string)=>{
  const available=visible();if(!available.length)return;
  const current=available.findIndex(({index})=>index===active);
  const next=key==='Home'?0:key==='End'?available.length-1:(current+(key==='ArrowUp'?-1:1)+available.length)%available.length;
  markActive(available[next].index);
 };
 const choose=()=>{
  if(state!=='idle')return;
  if(options[active].hidden)return;
  select.value=options[active].dataset.languageOption??confirmed;
  close(true);sync();void save();
 };
 trigger.addEventListener('click',()=>{if(state==='idle'){delete popup.dataset.keyboard;if(opened)close();else open()}});
 trigger.addEventListener('keydown',event=>{
  const {key}=event;
  if(key==='Escape'){event.preventDefault();close(true);return}
  if(key==='Tab'){close();return}
  if(!['ArrowDown','ArrowUp','Home','End','Enter',' '].includes(key))return;
  event.preventDefault();
  if(state!=='idle')return;
  popup.dataset.keyboard='true';
  if(!opened){open();if(key==='Home')markActive(0);else if(key==='End')markActive(options.length-1);return}
  if(key==='Enter'||key===' '){choose();return}
  move(key);
 });
 search?.addEventListener('input',()=>{
  const query=search.value.trim().toLocaleLowerCase();
  options.forEach(option=>{option.hidden=!(option.textContent??'').toLocaleLowerCase().includes(query)});
  const available=visible();if(empty)empty.hidden=available.length>0;
  if(available.length)markActive(available[0].index);else{trigger.removeAttribute('aria-activedescendant');options.forEach(option=>delete option.dataset.active)}
  position();
 });
 search?.addEventListener('keydown',event=>{
  if(event.key==='Escape'){event.preventDefault();event.stopPropagation();close(true)}
  else if(event.key==='Tab')close();
  else if(event.key==='Enter'){event.preventDefault();if(visible().length)choose()}
  else if(['ArrowDown','ArrowUp','Home','End'].includes(event.key)){event.preventDefault();popup.dataset.keyboard='true';move(event.key)}
 });
 options.forEach((option,i)=>{
  option.addEventListener('pointerdown',event=>event.preventDefault());
  option.addEventListener('click',()=>{active=i;choose()});
 });
 document.addEventListener('pointerdown',event=>{if(event.target instanceof Node&&!trigger.contains(event.target)&&!popup.contains(event.target))close()});
 document.addEventListener('focusin',event=>{if(event.target instanceof Node&&!trigger.contains(event.target)&&!popup.contains(event.target))close()});
 document.addEventListener('preference-help-open',()=>close());
 window.addEventListener('resize',position);
 window.addEventListener('scroll',position,true);
 window.visualViewport?.addEventListener('resize',position);
 window.visualViewport?.addEventListener('scroll',position);
 window.addEventListener('pagehide',()=>{generation++;controller?.abort();close()});
 window.addEventListener('pageshow',event=>{if(event.persisted){generation++;controller?.abort();close();location.replace('/settings/preferences')}});
 select.addEventListener('change',()=>{if(state==='idle')void save()});
 retry.addEventListener('click',()=>{select.value=desired;sync();void save()});
 form.addEventListener('submit',event=>{event.preventDefault();void save()});
 sync();
 // Commit the enhancement last: initialization failures retain the native form.
 if(custom){popup.hidden=false;select.hidden=true;trigger.hidden=false;label.htmlFor=trigger.id}else popup.hidden=true;
 fallbackSave.hidden=true;form.dataset.autosaveReady='true';
 if(location.hash==='#interface-language-picker')requestAnimationFrame(()=>{if(custom)trigger.focus();else select.focus()});
}
