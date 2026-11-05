import {t} from '../../i18n';
import {matchingOptions,moveActive,type NavigationKey} from './select-state';

type SelectParts={
 trigger:HTMLButtonElement|HTMLSelectElement;
 popup:HTMLElement;
 list:HTMLElement;
 search:HTMLInputElement;
 empty:HTMLElement;
 value?:HTMLElement;
 options?:HTMLElement[];
};
type SelectControl={close:(focus?:boolean)=>void;sync:()=>void};
const controls=new WeakMap<HTMLSelectElement,SelectControl>();
let current:SelectControl|undefined;
let nextID=0;

// Native selects remain the sole form value. Supplied markup lets the language
// preference keep its progressive fallback and save feedback without a second picker.
export function enhanceSelect(select:HTMLSelectElement,provided?:SelectParts):SelectControl|undefined{
 if(controls.has(select))return controls.get(select);
 const popup=provided?.popup??document.createElement('div');
 if(typeof popup.showPopover!=='function'||typeof popup.hidePopover!=='function')return;
 const id=provided?.list.id||`control-select-${++nextID}`;
 const trigger=provided?.trigger??select;
 const list=provided?.list??document.createElement('div');
 const search=provided?.search??document.createElement('input');
 const empty=provided?.empty??document.createElement('p');
 let options=provided?.options??[];
 if(!provided){
  list.id=id;list.role='listbox';
  list.setAttribute('aria-label',t('Options'));
  search.type='search';search.placeholder=t('Search options');search.setAttribute('aria-label',t('Search options'));search.autocomplete='off';
  empty.textContent=t('No results.');empty.role='status';
  popup.append(search,list,empty);popup.setAttribute('popover','manual');
 }
 popup.classList.add('control-select-menu');
 list.classList.add('control-select-options');search.classList.add('control-select-search');empty.classList.add('control-select-empty');
 trigger.classList.add('control-select-trigger');
 trigger.setAttribute('aria-haspopup','listbox');trigger.setAttribute('aria-controls',list.id);trigger.setAttribute('aria-expanded','false');
 search.setAttribute('aria-controls',list.id);
 let opened=false,active=-1;
 const locked=()=>select.disabled||trigger.getAttribute('aria-disabled')==='true';
 const available=()=>options.map((option,index)=>({option,index})).filter(({option},i)=>!option.hidden&&!select.options[i]?.disabled);
 const sync=()=>{
  if(provided?.value)provided.value.textContent=select.selectedOptions[0]?.textContent??'';
  options.forEach((option,i)=>option.setAttribute('aria-selected',String(select.options[i]?.selected??false)));
 };
 const close=(focus=false)=>{
  if(opened)popup.hidePopover();opened=false;
  trigger.setAttribute('aria-expanded','false');trigger.removeAttribute('aria-activedescendant');search.removeAttribute('aria-activedescendant');
  if(current===control)current=undefined;
  if(focus)trigger.focus();
 };
 const position=()=>{
  if(!opened)return;
  const bounds=trigger.getBoundingClientRect(),scale=popup.getBoundingClientRect().width/popup.offsetWidth||1;
  const viewport=window.visualViewport,left=viewport?.offsetLeft??0,top=viewport?.offsetTop??0;
  const right=left+(viewport?.width??innerWidth),bottom=top+(viewport?.height??innerHeight);
  const width=Math.min(Math.max(bounds.width,168*scale),right-left-16*scale);
  popup.style.width=width/scale+'px';
  const below=Math.max(0,bottom-bounds.bottom-12*scale),above=Math.max(0,bounds.top-top-12*scale);
  const up=below<Math.min(popup.scrollHeight*scale,140*scale)&&above>below;
  popup.style.maxHeight=Math.max(0,up?above:below)/scale+'px';
  popup.style.left=Math.max(left+8*scale,Math.min(bounds.right-width,right-width-8*scale))/scale+'px';
  popup.style.top=(up?bounds.top-popup.getBoundingClientRect().height-4*scale:bounds.bottom+4*scale)/scale+'px';
 };
 const markActive=(index:number)=>{
  active=index;options.forEach((option,i)=>option.toggleAttribute('data-active',i===active));
  if(active<0||!opened){trigger.removeAttribute('aria-activedescendant');search.removeAttribute('aria-activedescendant');return}
  trigger.setAttribute('aria-activedescendant',options[active].id);search.setAttribute('aria-activedescendant',options[active].id);
  options[active].scrollIntoView({block:'nearest'});
 };
 const filter=()=>{
  const matches=new Set(matchingOptions(options.map((option,i)=>({label:option.textContent??'',disabled:select.options[i]?.disabled??false})),search.value));
  options.forEach((option,i)=>{option.hidden=!matches.has(i)});
  const visible=available();empty.hidden=visible.length>0;markActive(visible[0]?.index??-1);position();
 };
 const choose=()=>{
  if(locked()||active<0||options[active].hidden)return;
  const changed=select.selectedIndex!==active;
  select.selectedIndex=active;close(true);sync();
  if(changed){select.dispatchEvent(new Event('input',{bubbles:true}));select.dispatchEvent(new Event('change',{bubbles:true}))}
 };
 const bindOptions=()=>options.forEach((option,i)=>{
  option.id||=`${id}-option-${i}`;
  option.addEventListener('pointerdown',event=>event.preventDefault());
  option.addEventListener('click',()=>{active=i;choose()});
 });
 const render=()=>{
  if(!provided){
   options=Array.from(select.options,option=>{const item=document.createElement('div');item.role='option';item.textContent=option.text;item.lang=option.lang;return item});
   list.replaceChildren(...options);bindOptions();
  }
  sync();filter();
 };
 const open=()=>{
  if(locked())return;current?.close();
  document.dispatchEvent(new Event('control-select-open'));
  search.value='';render();popup.showPopover();opened=true;current=control;trigger.setAttribute('aria-expanded','true');
  position();markActive(options.findIndex((option,i)=>!option.hidden&&select.options[i]?.selected));
  if(active<0)markActive(available()[0]?.index??-1);
  trigger.focus();
 };
 const move=(key:string)=>{
  markActive(moveActive(available().map(item=>item.index),active,key as NavigationKey));
 };
 const keyboard=(event:KeyboardEvent)=>{
  if(event.target===trigger&&event.key.length===1&&event.key!==' '&&!event.ctrlKey&&!event.metaKey&&!event.altKey){
   if(locked())return;event.preventDefault();if(!opened)open();search.value+=event.key;search.focus();filter();return;
  }
  if(event.key==='Escape'){if(opened){event.preventDefault();event.stopPropagation();close(true)}return}
  if(event.key==='Tab'){
   if(opened&&document.activeElement===search){
    event.preventDefault();close();
    const targets=Array.from(document.querySelectorAll<HTMLElement>('a[href],button,input,select,textarea,[tabindex="0"]')).filter(node=>node.checkVisibility()&&!node.matches(':disabled')&&!node.closest('[inert]'));
    targets[targets.indexOf(trigger)+(event.shiftKey?-1:1)]?.focus();
   }else close();return;
  }
  if(!['ArrowDown','ArrowUp','Home','End','Enter',' '].includes(event.key)||event.key===' '&&event.target===search)return;
  event.preventDefault();if(locked())return;popup.dataset.keyboard='true';
  if(!opened){open();if(event.key==='Home'||event.key==='End')move(event.key);return}
  if(event.key==='Enter'||event.key===' ')choose();else move(event.key);
 };
 const control={close,sync};controls.set(select,control);
 if(provided)bindOptions();
 else{
  const wrapper=document.createElement('span');wrapper.className='control-select';select.before(wrapper);wrapper.append(select);
  popup.hidden=false;document.body.append(popup);
  // Prevent the platform popup only after the enhanced popup is usable.
  select.addEventListener('mousedown',event=>event.preventDefault());
 }
 trigger.addEventListener('click',event=>{event.preventDefault();if(locked())return;delete popup.dataset.keyboard;if(opened)close(true);else open()});
 trigger.addEventListener('keydown',event=>keyboard(event as KeyboardEvent));search.addEventListener('keydown',keyboard);
 search.addEventListener('input',filter);select.addEventListener('change',sync);
 document.addEventListener('pointerdown',event=>{if(event.target instanceof Node&&!trigger.contains(event.target)&&!popup.contains(event.target))close()});
 document.addEventListener('focusin',event=>{if(event.target instanceof Node&&!trigger.contains(event.target)&&!popup.contains(event.target))close()});
 document.addEventListener('preference-help-open',()=>close());
 window.addEventListener('resize',position);window.addEventListener('scroll',position,true);
 window.visualViewport?.addEventListener('resize',position);window.visualViewport?.addEventListener('scroll',position);
 window.addEventListener('pagehide',()=>close());
 select.form?.addEventListener('reset',()=>queueMicrotask(()=>{close();render()}));
 new MutationObserver(()=>{if(locked())close();render()}).observe(select,{attributes:true,attributeFilter:['disabled','selected','label'],childList:true,subtree:true,characterData:true});
 render();return control;
}

export function enhanceSelectControls(){
 document.querySelectorAll<HTMLSelectElement>('select:not([multiple]):not([size]):not([data-language-preference] select)').forEach(select=>enhanceSelect(select));
}
