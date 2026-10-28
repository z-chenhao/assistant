import {build} from 'esbuild';
import {execFileSync} from 'node:child_process';
import {mkdtempSync,readFileSync,rmSync,writeFileSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('.',import.meta.url));
const notices=['ad-agent','OpenIntelligentUI'].map(name=>readFileSync(root+'licenses/'+name+'.txt','utf8')).join('\n');
const js=await build({absWorkingDir:root,entryPoints:['src/main.tsx'],bundle:true,minify:true,write:false,
  format:'iife',target:['es2022'],outfile:'../internal/interaction/web/assets/client.js',
  jsx:'automatic',define:{'process.env.NODE_ENV':'"production"'},
  banner:{js:'/*! Erii UI; bundled third-party licenses:\n'+notices+'\n*/'},legalComments:'inline'});
// Use Tailwind's file output: a synchronous child can otherwise lose buffered stdout.
const cssTemp=mkdtempSync(join(tmpdir(),'erii-web-css-'));
let css;
try{
  const output=join(cssTemp,'style.css');
  execFileSync(root+'node_modules/.bin/tailwindcss',['-i','src/style.css','--minify','--output',output],{cwd:root,stdio:['ignore','ignore','inherit']});
  css=readFileSync(output);
  if(!css.length)throw new Error('Tailwind produced an empty stylesheet');
}finally{rmSync(cssTemp,{recursive:true,force:true});}
const dependencies={react:'LICENSE','react-dom':'LICENSE',scheduler:'LICENSE',zod:'LICENSE',clsx:'license','class-variance-authority':'LICENSE','tailwind-merge':'LICENSE.md',tailwindcss:'LICENSE'};
const licenses=[notices,readFileSync(root+'licenses/eri.txt','utf8'),...Object.entries(dependencies).map(([name,file])=>name+'\n'+readFileSync(root+'node_modules/'+name+'/'+file,'utf8'))].join('\n\n');
const outputs={'client.js':js.outputFiles[0].contents,'style.css':css,'licenses.txt':licenses,'brand/NOTICE.txt':readFileSync(root+'licenses/eri-artwork.md')};
for(const [name,bytes] of Object.entries(outputs)){
  const path=root+'../internal/interaction/web/assets/'+name;
  if(process.argv.includes('--check')){
    if(!readFileSync(path).equals(Buffer.from(bytes)))throw new Error('Stale embedded UI asset: '+name+'; run npm run build --prefix web');
  }else writeFileSync(path,bytes);
}
