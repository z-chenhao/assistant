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
 const motion=document.querySelector<HTMLInputElement>('[data-avatar-motion]');
 if(motion){
  const reduced=matchMedia('(prefers-reduced-motion: reduce)');
  const setting=motion.closest<HTMLElement>('[data-avatar-setting]');
  const note=setting?.querySelector<HTMLElement>('[data-reduced-motion-note]');
  const sync=()=>{motion.checked=!StewardGhost.isPaused&&!reduced.matches;motion.disabled=reduced.matches;if(note)note.hidden=!reduced.matches};
  if(setting)setting.removeAttribute('data-client-pending');
  sync();motion.addEventListener('change',()=>{StewardGhost.pause(!motion.checked);sync()});
  reduced.addEventListener('change',sync);
 }
}
