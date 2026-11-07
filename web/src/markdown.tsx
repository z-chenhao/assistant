import {randomUUID} from './lib/random-id';
import {useEffect,useMemo,useRef} from 'react';
import {Marked,Renderer} from 'marked';
import DOMPurify from 'dompurify';
import {t} from './i18n';

const escape=(text:string)=>text.replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]!));
export function markdownLink(href:string,origin=location.origin):string|null{
 if(/^artifact:[A-Za-z0-9_-]{1,128}$/.test(href))return '/artifact?ref='+encodeURIComponent(href);
 try{const url=new URL(href,origin);return ['http:','https:'].includes(url.protocol)?url.href:null}catch{return null}
}
const renderer=new Renderer();
renderer.html=({text})=>escape(text);
renderer.image=({text})=>escape(text);
renderer.link=function({href,title,tokens}){
 const content=this.parser.parseInline(tokens),target=markdownLink(href);
 return target?`<a href="${escape(target)}"${title?` title="${escape(title)}"`:''} rel="noopener noreferrer">${content}</a>`:content;
};
const markdown=new Marked({renderer,gfm:true,breaks:true});
export function markdownHTML(text:string):string{
 return DOMPurify.sanitize(markdown.parse(text,{async:false}),{ALLOWED_TAGS:['p','br','strong','em','del','s','blockquote','pre','code','ul','ol','li','hr','h1','h2','h3','h4','h5','h6','table','thead','tbody','tr','th','td','a'],ALLOWED_ATTR:['href','title','class','start','colspan','rowspan'],ALLOW_DATA_ATTR:false});
}

declare global{interface Window{stewardDiagram?:(source:string,id:string)=>Promise<string>}}
let diagrams:Promise<void>|undefined;
const attempted=new WeakSet<Element>();
function loadDiagrams(){
 if(!diagrams)diagrams=new Promise<void>((resolve,reject)=>{
  const script=document.createElement('script');script.src='/diagrams.js';script.onload=()=>window.stewardDiagram?resolve():reject(Error('diagram unavailable'));script.onerror=()=>reject(Error('diagram unavailable'));document.head.append(script);
 });
 return diagrams;
}
export function enhanceDiagrams(scope:ParentNode){
 for(const code of scope.querySelectorAll<HTMLElement>('pre>code.language-mermaid')){
  if(attempted.has(code))continue;attempted.add(code);
  const source=code.textContent??'',pre=code.parentElement!;
  if(source.length>20000)continue;
  void(async()=>{
   try{
    await loadDiagrams();if(!code.isConnected)return;
    const svg=await window.stewardDiagram!(source,'diagram-'+randomUUID());
    if(!code.isConnected)return;
    const img=document.createElement('img');img.className='markdown-diagram';img.alt=t('Diagram');
    const href=URL.createObjectURL(new Blob([svg],{type:'image/svg+xml'}));
    img.onload=()=>{URL.revokeObjectURL(href);if(code.isConnected){pre.before(img);pre.hidden=true}};
    img.onerror=()=>URL.revokeObjectURL(href);img.src=href;
   }catch{pre.setAttribute('aria-label',t('Diagram source'))}
  })();
 }
}
export function Markdown({text}:{text:string}){
 const ref=useRef<HTMLDivElement>(null),html=useMemo(()=>markdownHTML(text),[text]);
 useEffect(()=>{if(ref.current)enhanceDiagrams(ref.current)},[html]);
 return <div ref={ref} className="semantic markdown" dangerouslySetInnerHTML={{__html:html}}/>;
}
export function renderDraftMarkdown(node:HTMLElement,text:string){node.classList.add('markdown');node.innerHTML=markdownHTML(text)}
