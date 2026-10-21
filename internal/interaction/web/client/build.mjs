import {build} from 'esbuild';
import {execFileSync} from 'node:child_process';
import {readFileSync,writeFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
const root=fileURLToPath(new URL('.',import.meta.url));
const notices=['ad-agent','OpenIntelligentUI'].map(name=>readFileSync(root+'licenses/'+name+'.txt','utf8')).join('\n');
const js=await build({absWorkingDir:root,entryPoints:['src/main.tsx'],bundle:true,minify:true,write:false,
  format:'iife',target:['es2022'],outfile:'../assets/client.js',
  jsx:'automatic',define:{'process.env.NODE_ENV':'"production"'},
  banner:{js:'/*! Erii UI; bundled third-party licenses:\n'+notices+'\n*/'},legalComments:'inline'});
const css=execFileSync(root+'node_modules/.bin/tailwindcss',['-i','src/style.css','--minify'],{cwd:root,stdio:['ignore','pipe','inherit']});
const dependencies={react:'LICENSE','react-dom':'LICENSE',scheduler:'LICENSE',zod:'LICENSE',clsx:'license','class-variance-authority':'LICENSE','tailwind-merge':'LICENSE.md',tailwindcss:'LICENSE'};
const licenses=[notices,readFileSync(root+'licenses/eri.txt','utf8'),...Object.entries(dependencies).map(([name,file])=>name+'\n'+readFileSync(root+'node_modules/'+name+'/'+file,'utf8'))].join('\n\n');
const outputs={'client.js':js.outputFiles[0].contents,'style.css':css,'licenses.txt':licenses,'brand/NOTICE.txt':readFileSync(root+'licenses/eri-artwork.md')};
for(const [name,bytes] of Object.entries(outputs)){
  const path=root+'../assets/'+name;
  if(process.argv.includes('--check')){
    if(!readFileSync(path).equals(Buffer.from(bytes)))throw new Error('Stale embedded UI asset: '+name+'; run npm run build --prefix internal/interaction/web/client');
  }else writeFileSync(path,bytes);
}
