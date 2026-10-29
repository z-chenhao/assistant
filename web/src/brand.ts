// Gentle pointer response for the welcome mascot and primary action. Delegation
// also covers a welcome region inserted after model setup by live updates.
export function enhanceBrandMotion(){
 const reduced=matchMedia('(prefers-reduced-motion: reduce)');
 const fine=matchMedia('(hover: hover) and (pointer: fine)');
 let active:HTMLElement|null=null;
 const reset=()=>{active?.style.removeProperty('--pointer-x');active?.style.removeProperty('--pointer-y');active=null};
 document.addEventListener('pointermove',event=>{
  if(reduced.matches||!fine.matches){reset();return}
  const target=event.target instanceof Element?event.target.closest<HTMLElement>('.welcome-collie, .send-button'):null;
  if(!target||target.matches(':disabled')){reset();return}
  if(active!==target){reset();active=target}
  const bounds=target.getBoundingClientRect();
  const unit=(v:number,size:number)=>Math.max(-1,Math.min(1,v/Math.max(size,1)*2-1));
  target.style.setProperty('--pointer-x',String(unit(event.clientX-bounds.left,bounds.width)));
  target.style.setProperty('--pointer-y',String(unit(event.clientY-bounds.top,bounds.height)));
 });
 document.addEventListener('pointerout',event=>{if(active&&!(event.relatedTarget instanceof Node&&active.contains(event.relatedTarget)))reset()});
 window.addEventListener('blur',reset);
 reduced.addEventListener('change',reset);fine.addEventListener('change',reset);
}
