import {t} from './i18n';

// Reveal the selected section inside its own strip without scrolling the page.
export function revealCurrentTabs(root:ParentNode=document){
 root.querySelectorAll<HTMLElement>('.section-tabs,.obs-detail-tabs,.round-links').forEach(tabs=>{
  const active=tabs.querySelector<HTMLElement>('[aria-current]');
  if(!active)return;
  const bounds=tabs.getBoundingClientRect(),selected=active.getBoundingClientRect();
  if(selected.left<bounds.left)tabs.scrollLeft+=selected.left-bounds.left;
  else if(selected.right>bounds.right)tabs.scrollLeft+=selected.right-bounds.right;
 });
}
// Local presentation preferences. Host-owned task fragments remain the source of truth.
export function enhanceShell(){
 const html=document.documentElement;
 const footer=document.querySelector<HTMLElement>('.conversation-footer');
 if(footer&&'ResizeObserver' in window)new ResizeObserver(()=>{
  html.style.setProperty('--composer-height',`${Math.ceil(footer.getBoundingClientRect().height)}px`);
 }).observe(footer);
 document.addEventListener('invalid',event=>{
  const input=event.target;
  if(!(input instanceof HTMLInputElement||input instanceof HTMLTextAreaElement||input instanceof HTMLSelectElement))return;
  if(html.lang==='zh-CN')input.setCustomValidity(t(input.validity.valueMissing?'Please fill out this field.':input.validity.typeMismatch?'Enter a valid value.':'Please check this field.'));
 },true);
 document.addEventListener('input',event=>{const input=event.target;if(input instanceof HTMLInputElement||input instanceof HTMLTextAreaElement||input instanceof HTMLSelectElement)input.setCustomValidity('')});

 html.classList.add('js');
 const mobile=matchMedia('(max-width: 760px)');
 let collapsed=false;
 try{collapsed=localStorage.getItem('steward-sidebar')==='collapsed'}catch{}
 html.classList.toggle('sidebar-collapsed',collapsed);
 const toggles=document.querySelectorAll<HTMLButtonElement>('[data-sidebar-toggle]');
 const sidebar=document.getElementById('workspace-navigation');
 const update=()=>{
  const open=mobile.matches?html.classList.contains('mobile-nav'):!html.classList.contains('sidebar-collapsed');
  toggles.forEach(button=>button.setAttribute('aria-expanded',String(open)));
  if(sidebar)sidebar.inert=!open;
  const main=document.getElementById('workspace-main');if(main)main.inert=mobile.matches&&open;
 };
 const close=()=>{html.classList.remove('mobile-nav');update()};
 toggles.forEach(button=>button.addEventListener('click',()=>{
  if(mobile.matches){
   html.classList.toggle('mobile-nav');update();
   if(html.classList.contains('mobile-nav'))sidebar?.querySelector<HTMLAnchorElement>('a')?.focus();else document.querySelector<HTMLButtonElement>('.sidebar-open')?.focus();
  }else{
   html.classList.toggle('sidebar-collapsed');update();
   try{localStorage.setItem('steward-sidebar',html.classList.contains('sidebar-collapsed')?'collapsed':'open')}catch{}
   document.querySelector<HTMLButtonElement>(html.classList.contains('sidebar-collapsed')?'.sidebar-open':'.brand-row .sidebar-toggle')?.focus();
  }
 }));
 document.querySelector('[data-sidebar-close]')?.addEventListener('click',()=>{close();document.querySelector<HTMLButtonElement>('.sidebar-open')?.focus()});
 document.addEventListener('keydown',event=>{
  if(!mobile.matches||!html.classList.contains('mobile-nav'))return;
  if(event.key==='Escape'){close();document.querySelector<HTMLButtonElement>('.sidebar-open')?.focus()}
  if(event.key==='Tab'&&sidebar){
   const nodes=Array.from(sidebar.querySelectorAll<HTMLElement>('a,button'));const first=nodes[0],last=nodes.at(-1);
   if(event.shiftKey&&document.activeElement===first){event.preventDefault();last?.focus()}
   else if(!event.shiftKey&&document.activeElement===last){event.preventDefault();first?.focus()}
  }
 });
 mobile.addEventListener('change',close);update();
 revealCurrentTabs();
 void document.fonts.ready.then(()=>revealCurrentTabs());
 window.addEventListener('resize',()=>revealCurrentTabs());
 const themes=document.querySelectorAll<HTMLButtonElement>('[data-theme-toggle]');
 const syncTheme=()=>themes.forEach(button=>{button.setAttribute('aria-label',html.classList.contains('dark')?t('Switch to light appearance'):t('Switch to dark appearance'));button.setAttribute('aria-pressed',String(html.classList.contains('dark')))});
 syncTheme();themes.forEach(button=>button.addEventListener('click',()=>{html.classList.toggle('dark');try{localStorage.setItem('steward-theme',html.classList.contains('dark')?'dark':'light')}catch{};syncTheme()}));

}
