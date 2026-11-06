// One shared, top-layer timestamp disclosure supports hover, keyboard and touch.
// Without Popover support, the native title and accessible label remain usable.
export function enhanceTimeDetails(){
 if(!('showPopover' in HTMLElement.prototype))return;
 const popup=document.createElement('div');popup.className='control-time-detail';popup.popover='manual';popup.setAttribute('role','tooltip');document.body.append(popup);
 let active:HTMLTimeElement|null=null,pinned=false;
 const close=()=>{popup.hidePopover();active=null;pinned=false};
 const position=()=>{
  if(!active?.isConnected){close();return}
  const box=active.getBoundingClientRect(),zoom=parseFloat(getComputedStyle(document.documentElement).zoom)||1;
  const size=popup.getBoundingClientRect();
  popup.style.left=Math.max(12,Math.min(box.left,innerWidth-size.width-12))/zoom+'px';
  popup.style.top=(box.bottom+size.height+8<innerHeight?box.bottom+6:Math.max(12,box.top-size.height-6))/zoom+'px';
 };
 const show=(time:HTMLTimeElement)=>{
  if(time!==active)pinned=false;
  active=time;popup.textContent=time.getAttribute('aria-label');time.removeAttribute('title');
  popup.showPopover();position();
 };
 const target=(event:Event)=>event.target instanceof Element?event.target.closest<HTMLTimeElement>('time[data-local-time="message"]'):null;
 document.addEventListener('pointerover',event=>{const time=target(event);if(time&&event.pointerType!=='touch')show(time)});
 document.addEventListener('pointerout',event=>{if(target(event)&&!pinned&&document.activeElement!==active&&!(event.relatedTarget instanceof Node&&popup.contains(event.relatedTarget)))close()});
 popup.addEventListener('pointerleave',()=>{if(!pinned&&document.activeElement!==active)close()});
 document.addEventListener('focusin',event=>{const time=target(event);if(time)show(time)});
 document.addEventListener('focusout',event=>{if(target(event))close()});
 document.addEventListener('click',event=>{
  const time=target(event);
  if(time){if(active===time&&pinned)close();else{show(time);pinned=true}}
  else if(!(event.target instanceof Node&&popup.contains(event.target)))close();
 });
 document.addEventListener('keydown',event=>{if(event.key==='Escape'&&active){close();event.preventDefault();event.stopPropagation()}},true);
 window.addEventListener('resize',()=>{if(active)position()});
 window.addEventListener('scroll',()=>{if(active)close()},true);
 new MutationObserver(()=>{if(active&&!active.isConnected)close()}).observe(document.body,{subtree:true,childList:true});
}
