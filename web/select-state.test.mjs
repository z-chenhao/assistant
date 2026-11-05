import assert from 'node:assert/strict';
import test from 'node:test';
import {matchingOptions,moveActive} from './src/components/ui/select-state.ts';

const options=[{label:'Disabled match',disabled:true},{label:'English',disabled:false},{label:'简体中文',disabled:false},{label:'English (disabled)',disabled:true}];
test('search ignores case and surrounding whitespace while keeping native indices',()=>{
 assert.deepEqual(matchingOptions(options,' ENGLISH '),[1]);
 assert.deepEqual(matchingOptions(options,'中文'),[2]);
 assert.deepEqual(matchingOptions(options,''),[1,2]);
});
test('no results and fully disabled lists have no active option',()=>{
 assert.deepEqual(matchingOptions(options,'unavailable'),[]);
 assert.deepEqual(matchingOptions([{label:'English',disabled:true}],''),[]);
 for(const key of ['ArrowDown','ArrowUp','Home','End'])assert.equal(moveActive([],1,key),-1);
});
test('keyboard navigation wraps and skips filtered or disabled native options',()=>{
 const available=[1,4,7];
 assert.equal(moveActive(available,7,'ArrowDown'),1);
 assert.equal(moveActive(available,1,'ArrowUp'),7);
 assert.equal(moveActive(available,4,'Home'),1);
 assert.equal(moveActive(available,4,'End'),7);
});
test('a removed active option restarts from the correct edge after filtering',()=>{
 assert.equal(moveActive([2,5,9],4,'ArrowDown'),2);
 assert.equal(moveActive([2,5,9],4,'ArrowUp'),9);
 assert.equal(moveActive([5],-1,'ArrowDown'),5);
 assert.equal(moveActive([5],5,'ArrowUp'),5);
});
test('filtering and navigation leave supplied option data unchanged',()=>{
 const copy=structuredClone(options);
 const available=matchingOptions(options,'中文');
 assert.equal(moveActive(Object.freeze(available),1,'End'),2);
 assert.deepEqual(options,copy);
});
