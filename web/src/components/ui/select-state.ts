export type SelectOption={label:string;disabled:boolean};
export type NavigationKey='ArrowDown'|'ArrowUp'|'Home'|'End';

// Indices refer to native options, including disabled entries. Filtering never
// renumbers submitted values or changes the currently committed selection.
export function matchingOptions(options:readonly SelectOption[],query:string):number[]{
 const search=query.trim().toLocaleLowerCase();
 return options.flatMap((option,index)=>!option.disabled&&option.label.toLocaleLowerCase().includes(search)?[index]:[]);
}

export function moveActive(available:readonly number[],active:number,key:NavigationKey):number{
 if(!available.length)return -1;
 const current=available.indexOf(active);
 if(key==='Home')return available[0];
 if(key==='End')return available[available.length-1];
 if(current<0)return key==='ArrowUp'?available[available.length-1]:available[0];
 return available[(current+(key==='ArrowUp'?-1:1)+available.length)%available.length];
}
