import assert from 'node:assert/strict';
import test from 'node:test';
import {buildSync} from 'esbuild';
import {runInNewContext} from 'node:vm';

const code=buildSync({entryPoints:[new URL('src/components/open-intelligent-ui/export-utils.ts',import.meta.url).pathname],bundle:true,format:'cjs',write:false}).outputFiles[0].text;
const module={exports:{}};runInNewContext(code,{module,exports:module.exports});
test('native chart exports preserve text labels without creating executable markup',()=>{
 const label='</script><img src=x onerror=alert(1)>';
 for(const kind of ['bar','pie']){
  const html=module.exports.chartToStandaloneHtml(kind,{title:'<script>literal title</script>',description:'Source: user data',data:[{label,value:42}]});
  assert(!html.includes(label));assert(html.includes('&lt;script&gt;literal title&lt;/script&gt;'));
  const labels=JSON.parse(html.match(/labels: (\[[^\n]+\])/)[1]);assert.equal(labels[0],label);
 }
});
