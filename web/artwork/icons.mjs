// Export the exact first production pose; no separate logo drawing or source copy.
import assert from 'node:assert/strict';
import {createHash} from 'node:crypto';
import {readFile,writeFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {chromium} from '../../guest-browser/node_modules/playwright-core/index.mjs';

const root=new URL('../../',import.meta.url);
const manifest=JSON.parse(await readFile(new URL('manifest.json',import.meta.url),'utf8'));
const check=process.argv.includes('--check');
for(const asset of [...manifest.sources,...manifest.atlases]){
  const bytes=await readFile(new URL(asset.file,root));
  assert.equal(createHash('sha256').update(bytes).digest('hex'),asset.sha256,`Artwork source drift: ${asset.file}`);
}
const browser=await chromium.launch({headless:true});
try{
  const page=await browser.newPage();
  for(const icon of manifest.icons){
    const atlas=await readFile(new URL(icon.atlas,root));
    const existing=check?await readFile(new URL(icon.file,root)):null;
    const result=await page.evaluate(async({atlas,existing,size})=>{
      const decode=async(data,type)=>{const image=new Image();image.src=`data:${type};base64,${data}`;await image.decode();return image;};
      const image=await decode(atlas,'image/webp');
      const canvas=document.createElement('canvas');canvas.width=canvas.height=size;
      const ctx=canvas.getContext('2d');ctx.drawImage(image,0,0,size,size,0,0,size,size);
      const expected=ctx.getImageData(0,0,size,size).data;
      const png=canvas.toDataURL('image/png').split(',')[1];
      if(existing){
        const stored=await decode(existing,'image/png');
        if(stored.width!==size||stored.height!==size)return {matches:false};
        ctx.clearRect(0,0,size,size);ctx.drawImage(stored,0,0);
        const actual=ctx.getImageData(0,0,size,size).data;
        return {matches:expected.every((v,i)=>v===actual[i])};
      }
      return {png};
    },{atlas:atlas.toString('base64'),existing:existing?.toString('base64'),size:icon.size});
    if(check)assert.ok(result.matches,`Stale brand icon: ${icon.file}; run node web/artwork/icons.mjs`);
    else await writeFile(new URL(icon.file,root),Buffer.from(result.png,'base64'));
    console.log(`${check?'Verified':'Exported'} ${fileURLToPath(new URL(icon.file,root))}`);
  }
}finally{await browser.close();}
