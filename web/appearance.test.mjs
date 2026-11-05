import assert from 'node:assert/strict';
import test from 'node:test';
import {buildSync} from 'esbuild';
import {runInNewContext} from 'node:vm';
const script=buildSync({entryPoints:[new URL('src/appearance.ts',import.meta.url).pathname],bundle:true,format:'iife',write:false}).outputFiles[0].text;
function boot({theme,sidebar,mobile=false,dark=false,storageBlocked=false}={}){
 const classes=new Set();let resourceError;
 class Script{getAttribute(){return '/app.js'}}
 const classList={add(...names){names.forEach(n=>classes.add(n))},remove(...names){names.forEach(n=>classes.delete(n))},toggle(name,on){on?classes.add(name):classes.delete(name)}};
 runInNewContext(script,{document:{documentElement:{classList}},window:{addEventListener(type,callback){if(type==='error')resourceError=callback}},HTMLScriptElement:Script,
  localStorage:{getItem(key){if(storageBlocked)throw Error('storage unavailable');return key==='steward-theme'?theme:sidebar}},
  matchMedia:query=>({matches:query.includes('color-scheme')?dark:!mobile})});
 return {classes,failBundle(){resourceError({target:new Script()})}};
}
test('explicit theme overrides the OS before page markup is parsed',()=>{
 assert(boot({theme:'dark',dark:false}).classes.has('dark'));
 assert(!boot({theme:'light',dark:true}).classes.has('dark'));
 assert(boot({dark:true}).classes.has('dark'));
});
test('desktop collapsed preference applies without needing a page or module',()=>{
 assert(boot({sidebar:'collapsed'}).classes.has('sidebar-collapsed'));
 assert(!boot({sidebar:'open'}).classes.has('sidebar-collapsed'));
 assert(!boot({sidebar:'collapsed',mobile:true}).classes.has('sidebar-collapsed'));
});
test('blocked storage still marks scripting available without failing boot',()=>{
 assert.deepEqual([...boot({storageBlocked:true}).classes],['js']);
 assert(boot({storageBlocked:true,dark:true}).classes.has('dark'));
});
test('bundle download failure restores usable native navigation and forms',()=>{
 const state=boot({theme:'dark',sidebar:'collapsed'});state.failBundle();
 assert.deepEqual([...state.classes],['dark']);
});
