// Exercise the visible Base UI control, retaining native interactions for no-JS.
export async function selectParts(control){
 const identity=await control.evaluate(node=>{const select=node.tagName==='SELECT'?node:node.closest('.control-select')?.querySelector('select');if(!select)throw Error('Missing native select');return {id:select.id,name:select.name}});
 // getByLabel starts on the native select before enhancement and then resolves
 // the visible trigger. Keep the form-value locator stable across that change.
 const native=control.page().locator(identity.id?'select[id='+JSON.stringify(identity.id)+']':'select[name='+JSON.stringify(identity.name)+']');
 const wrapper=native.locator('xpath=..');
 return {native,trigger:wrapper.locator('button[role=combobox]')};
}
export async function selectValue(page,control,value){
 const{native,trigger}=await selectParts(control);
 if(!await page.evaluate(()=>document.documentElement.classList.contains('js')&&typeof fetch==='function')){await native.selectOption(value);return}
 await trigger.waitFor();
 const label=await native.locator('option').evaluateAll((options,value)=>options.find(option=>option.value===value)?.label,value);
 if(label===undefined)throw Error('Unknown native option: '+value+' in '+await native.evaluate(node=>node.outerHTML));
 await trigger.click();await page.getByRole('listbox').getByRole('option',{name:label,exact:true}).click();
}
