import {useLayoutEffect,useState,type CSSProperties} from 'react';

// Floating UI positions in viewport pixels. Cancel root CSS zoom around its
// positioner and restore it on the popup, preserving enlarged text and targets.
export function usePopupZoom():CSSProperties{
 const[zoom,setZoom]=useState(1);
 useLayoutEffect(()=>{const sync=()=>{const value=parseFloat(getComputedStyle(document.documentElement).zoom);setZoom(Number.isFinite(value)&&value>0?value:1)};sync();const observer=new MutationObserver(sync);observer.observe(document.documentElement,{attributes:true,attributeFilter:['style']});return()=>observer.disconnect()},[]);
 return {zoom:1/zoom,'--control-popup-zoom':zoom} as CSSProperties;
}
