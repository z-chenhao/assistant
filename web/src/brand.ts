import {t} from './i18n';
import {StewardGhost} from './avatar.js';

export function enhanceBrandMotion(){
 const enhance=()=>{
  for(const slot of document.querySelectorAll<HTMLElement>('.avatar-slot')){
   if(!slot.querySelector('.steward-avatar')){
    slot.removeAttribute('aria-hidden');
    slot.innerHTML=StewardGhost.markup();
   }
  }
  StewardGhost.observe();
 };
 enhance();
 const messages=document.getElementById('conversation-messages');
 if(messages)new MutationObserver(enhance).observe(messages,{childList:true});
 document.addEventListener('click',event=>{
  const target=event.target instanceof Element?event.target:null;
  const avatar=target?.closest<HTMLButtonElement>('.steward-avatar');
  if(avatar)void StewardGhost.react(avatar);
 });
 const pause=document.querySelector<HTMLButtonElement>('[data-avatar-pause]');
 const sync=()=>{if(pause){pause.setAttribute('aria-pressed',String(StewardGhost.isPaused));pause.textContent=StewardGhost.isPaused?t('Resume avatar motion'):t('Pause avatar motion')}};
 sync();pause?.addEventListener('click',()=>{StewardGhost.pause(!StewardGhost.isPaused);sync()});
}
