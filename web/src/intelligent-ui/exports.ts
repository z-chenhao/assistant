import DOMPurify from 'dompurify';
// All exports are local. Generated markup is never inserted into the Host DOM.
export const safeName=(value:string)=>value.replace(/[\\/:*?"<>|\x00-\x1f]/g,'-').slice(0,100)||'answer';
export function download(bytes:BlobPart,type:string,name:string){const url=URL.createObjectURL(new Blob([bytes],{type}));const link=document.createElement('a');link.href=url;link.download=name;link.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
export function csv(rows:unknown[][]){return '\ufeff'+rows.map(row=>row.map(cell=>{let value=String(cell??'');if(/^[\s]*[=+@\-\t\r]/.test(value)&&!(typeof cell==='number'||/^[-+]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][-+]?\d+)?$/.test(value)))value="'"+value;return '"'+value.replaceAll('"','""')+'"'}).join(',')).join('\r\n')}
export function serializeSVG(svg:SVGSVGElement){
 const copy=svg.cloneNode(true) as SVGSVGElement;const original=[svg,...svg.querySelectorAll('*')],clones=[copy,...copy.querySelectorAll('*')];
 const properties=['fill','stroke','stroke-width','font-family','font-size','font-weight','opacity','color','text-anchor','dominant-baseline','background-color'];
 original.forEach((node,i)=>{const style=getComputedStyle(node);for(const key of properties)clones[i].setAttribute(key,style.getPropertyValue(key))});
 const b=svg.getBoundingClientRect();copy.setAttribute('xmlns','http://www.w3.org/2000/svg');copy.setAttribute('width',String(b.width));copy.setAttribute('height',String(b.height));return safeSVG(new XMLSerializer().serializeToString(copy));
}
const imageURIs=new RegExp('^(?:#|data:image/(?:png|jpeg|webp|gif);base64,)');
const cssURL=new RegExp(String.raw`url\(\s*(['"]?)(.*?)\1\s*\)`,'gi');
const fragment=new RegExp(String.raw`^#[-\w:.]+$`);
export function safeSVG(source:string){
 const clean=DOMPurify.sanitize(source,{USE_PROFILES:{svg:true,svgFilters:true},FORBID_TAGS:['script','foreignObject','style','animate','set','animateMotion','animateTransform'],FORBID_ATTR:['style'],ALLOWED_URI_REGEXP:imageURIs});
 const doc=new DOMParser().parseFromString(String(clean),'image/svg+xml'),root=doc.documentElement;
 if(root.localName!=='svg'||root.namespaceURI!=='http://www.w3.org/2000/svg')throw Error('Invalid SVG.');
 for(const node of [root,...root.querySelectorAll('*')])for(const attr of [...node.attributes]){
  if(['fill','stroke','filter','clip-path','mask','cursor'].includes(attr.name)&&attr.value.includes('\\')){node.removeAttribute(attr.name);continue}
  if(attr.value.toLowerCase().includes('url('))node.setAttribute(attr.name,attr.value.replace(cssURL,(_match,_quote,value)=>fragment.test(value)?'url('+value+')':'none'));
 }
 return new XMLSerializer().serializeToString(root);
}
export async function svgPNG(source:string,name:string){source=safeSVG(source);const image=new Image();const url=URL.createObjectURL(new Blob([source],{type:'image/svg+xml'}));try{await new Promise<void>((resolve,reject)=>{image.onload=()=>resolve();image.onerror=()=>reject(Error('Image export failed.'));image.src=url});const canvas=document.createElement('canvas');canvas.width=Math.max(1,image.naturalWidth*2);canvas.height=Math.max(1,image.naturalHeight*2);const drawing=canvas.getContext('2d');if(!drawing)throw Error('Image export unavailable.');drawing.drawImage(image,0,0,canvas.width,canvas.height);const blob=await new Promise<Blob>((resolve,reject)=>canvas.toBlob(b=>b?resolve(b):reject(Error('Image export failed.')),'image/png'));download(blob,'image/png',name)}finally{URL.revokeObjectURL(url)}}
