import assert from 'node:assert/strict';
import test from 'node:test';
import {buildSync} from 'esbuild';
import {runInNewContext} from 'node:vm';
function bundle(path,globals={}){
 const source=buildSync({entryPoints:[new URL(path,import.meta.url).pathname],bundle:true,format:'cjs',write:false}).outputFiles[0].text;
 const module={exports:{}};runInNewContext(source,{module,exports:module.exports,...globals});return module.exports;
}
const {parseExactJson}=bundle('src/components/ui/read-only-json.ts');
test('structured JSON keeps exact Unicode, escaped quotes and numeric-looking strings',()=>{
 const value=parseExactJson(' { "content": "完整🦊 <img src=x>", "id": "9007199254740993", "quote": "a\\\"b", "n": 0.125, "empty": null } ');
 assert.equal(value.content,'完整🦊 <img src=x>');assert.equal(value.id,'9007199254740993');assert.equal(value.quote,'a"b');assert.equal(value.n,.125);assert.equal(value.empty,null);
});
for(const source of ['{"n":9007199254740993}','{"n":9007199254740992}','{"n":1.234567890123456789}','{"n":1e100}','{"n":1e400}','{"n":-0}','{"n":1,"n":2}','{"text":"\\u0061"}','{"partial":'])
 test('keep the original JSON when a tree would rewrite evidence: '+source,()=>assert.throws(()=>parseExactJson(source)));

// EventTarget exercises the shared event lifecycle without pretending to lay out
// a browser. Geometry and actual focus remain in the Chromium regression.
class Handle extends EventTarget{
 constructor(){super();this.attributes=new Map();this.dataset={};this.captures=new Set()}
 setAttribute(name,value){this.attributes.set(name,value)}
 getAttribute(name){return this.attributes.get(name)}
 setPointerCapture(id){this.captures.add(id)}
 hasPointerCapture(id){return this.captures.has(id)}
 releasePointerCapture(id){this.captures.delete(id)}
 focus(){}
 remove(){this.removed=true}
}
function resize({reverse=false}={}){
 const {createResizeHandle}=bundle('src/components/ui/resize-handle.ts',{AbortController,document:{createElement:()=>new Handle()}});
 let value=280;const changes=[];
 const handle=createResizeHandle('Resize content',{bounds:()=>({min:200,max:480,value}),at:x=>reverse?1000-x:x,change:(next,commit)=>{value=next;changes.push({value,commit})}});
 const send=(type,properties={})=>{const event=new Event(type,{cancelable:true});Object.assign(event,properties);handle.element.dispatchEvent(event);return event};
 return {...handle,send,changes,get value(){return value}};
}
test('pointer resizing uses relative motion, clamps, and only commits on release',()=>{
 const h=resize();h.send('pointerdown',{button:0,pointerId:7,clientX:123});assert.equal(h.value,280);
 h.send('pointermove',{pointerId:9,clientX:400});assert.equal(h.value,280);
 h.send('pointermove',{pointerId:7,clientX:153});assert.equal(h.value,310);assert.equal(h.changes.at(-1).commit,false);
 h.send('pointermove',{pointerId:7,clientX:900});assert.equal(h.value,480);
 h.send('pointerup',{pointerId:7});assert.equal(h.changes.at(-1).commit,true);assert(!h.element.hasPointerCapture(7));
 assert.equal(h.element.getAttribute('aria-valuenow'),'480');h.dispose();
});
test('cancel and Escape restore the starting width without saving it',()=>{
 for(const cancel of ['pointercancel','Escape']){const h=resize();h.send('pointerdown',{button:0,pointerId:1,clientX:100});h.send('pointermove',{pointerId:1,clientX:160});assert.equal(h.value,340);
  if(cancel==='Escape'){const event=h.send('keydown',{key:'Escape'});assert(event.defaultPrevented)}else h.send(cancel,{pointerId:1});
  assert.equal(h.value,280);assert(h.changes.every(change=>!change.commit));h.dispose();
 }
});
test('keyboard bounds work, modified shortcuts pass through, and disposal removes listeners',()=>{
 const h=resize();h.send('keydown',{key:'ArrowRight',altKey:true});assert.equal(h.value,280);
 h.send('keydown',{key:'ArrowRight'});assert(h.value>280);assert.equal(h.changes.at(-1).commit,true);
 h.send('keydown',{key:'Home'});assert.equal(h.value,200);h.send('keydown',{key:'End'});assert.equal(h.value,480);
 h.dispose();h.send('keydown',{key:'Home'});assert.equal(h.value,480);assert(h.element.removed);
});
test('the right-hand sheet edge expands when dragged left',()=>{
 const h=resize({reverse:true});h.send('pointerdown',{button:0,pointerId:1,clientX:700});h.send('pointermove',{pointerId:1,clientX:660});assert.equal(h.value,320);h.dispose();
});

test('a second pointer cannot steal a drag or commit its width',()=>{
 const h=resize();h.send('pointerdown',{button:0,pointerId:1,clientX:100});
 h.send('pointermove',{pointerId:1,clientX:130});h.send('pointerdown',{button:0,pointerId:2,clientX:500});
 h.send('pointerup',{pointerId:2});assert(h.changes.every(change=>!change.commit));
 h.send('pointermove',{pointerId:1,clientX:140});assert.equal(h.value,320);
 h.send('lostpointercapture',{pointerId:1});assert.equal(h.value,280);assert(h.changes.every(change=>!change.commit));h.dispose();
});
