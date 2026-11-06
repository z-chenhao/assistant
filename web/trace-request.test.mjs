import assert from 'node:assert/strict';
import test from 'node:test';
import {buildSync} from 'esbuild';
import {runInNewContext} from 'node:vm';
const source=buildSync({entryPoints:[new URL('src/trace-request.ts',import.meta.url).pathname],bundle:true,format:'cjs',write:false}).outputFiles[0].text;
const module={exports:{}};runInNewContext(source,{module,exports:module.exports,document:{documentElement:{lang:'en'}}});
const {parseReadableRequest}=module.exports;
test('request reading decodes multiline strings and Unicode without changing raw evidence',()=>{
 const source='{"instructions":"hello\\n\\u003cscript\\u003e","messages":[{"role":"user","content":"你好"}]}';
 const value=parseReadableRequest(source);
 assert.equal(value.instructions,'hello\n<script>');assert.equal(value.messages[0].content,'你好');
 assert(source.includes('\\u003c'));
});
test('ambiguous, partial or lossy request JSON retains its original view',()=>{
 for(const source of ['{"a":1,"a":2}','{"a":1,"\\u0061":2}','{"n":9007199254740993}','{"unfinished":'])assert.throws(()=>parseReadableRequest(source));
});
