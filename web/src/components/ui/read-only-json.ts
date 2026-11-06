import {t} from '../../i18n';
const enhanced=new WeakSet<HTMLElement>();

// Only build a tree when JavaScript can round-trip every recorded token.
// Otherwise the original JSON remains the faithful display, including duplicate
// keys, numeric precision, escaped values and incomplete chunks.
export function parseExactJson(source:string):unknown {
 let unsafe=false;
 const value:unknown=JSON.parse(source,(_key,value:unknown)=>{if(typeof value==='number'&&(!Number.isFinite(value)||(Number.isInteger(value)&&!Number.isSafeInteger(value))))unsafe=true;return value});
 const tokens=source.match(/"(?:\\.|[^"\\])*"|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?|\b(?:true|false|null)\b|[{}\[\],:]/g)??[];
 if(unsafe||tokens.join('')!==JSON.stringify(value))throw new Error('JSON needs exact source view');
 return value;
}

// Render DOM text only. The original pre is retained verbatim as the JSON view;
// incomplete chunks and unsafe numeric values always keep that exact fallback.
export function enhanceReadOnlyJson(scope:ParentNode){
 for(const pre of scope.querySelectorAll<HTMLElement>('pre[data-readonly-json]')){
  if(enhanced.has(pre))continue;enhanced.add(pre);
  const source=pre.textContent??'',tree=document.createElement('div');tree.className='control-json-tree';tree.dataset.jsonTree='';
  const tokens:Node[]=[];let offset=0;
  for(const match of source.matchAll(/"(?:\\.|[^"\\])*"|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?|\b(?:true|false|null)\b/g)){
   const at=match.index!;tokens.push(document.createTextNode(source.slice(offset,at)));
   const span=document.createElement('span'),literal=match[0];span.className='control-json-'+(literal.startsWith('"')?(/^\s*:/.test(source.slice(at+literal.length))?'key':'string'):literal==='null'?'null':literal==='true'||literal==='false'?'boolean':'number');span.textContent=literal;tokens.push(span);offset=at+literal.length;
  }
  tokens.push(document.createTextNode(source.slice(offset)));pre.replaceChildren(...tokens);
  let budget=2000;
  try{
   const value=parseExactJson(source);
   const token=(text:string,kind:string)=>{const span=document.createElement('span');span.className='control-json-'+kind;span.textContent=text;return span};
   const render=(value:unknown,key:string|null,depth:number):HTMLElement=>{
    if(--budget<0)throw new Error('JSON tree limit');
    const collection=value!==null&&typeof value==='object';
    const row=document.createElement(collection?'details':'div');row.className=collection?'control-json-branch':'control-json-leaf';
    const line=collection?document.createElement('summary'):row;
    if(key!==null)line.append(token(JSON.stringify(key),'key'),document.createTextNode(': '));
    if(collection){
     const entries=Object.entries(value as Record<string,unknown>),array=Array.isArray(value);
     line.append(token((array?'[':'{')+' '+entries.length+' '+t('items')+' '+(array?']':'}'),'container'));row.append(line);
     (row as HTMLDetailsElement).open=depth<2;
     const children=document.createElement('div');children.className='control-json-children';
     for(const [name,child] of entries)children.append(render(child,name,depth+1));row.append(children);
    }else line.append(token(JSON.stringify(value),value===null?'null':typeof value));
    return row;
   };
   tree.append(render(value,null,0));pre.before(tree);pre.dataset.jsonReady='';
  }catch{tree.remove()}
 }
}
export function setReadOnlyJsonMode(scope:ParentNode,mode:'formatted'|'json'){
 for(const pre of scope.querySelectorAll<HTMLElement>('pre[data-json-ready]'))pre.hidden=mode==='formatted';
 for(const tree of scope.querySelectorAll<HTMLElement>('[data-json-tree]'))tree.hidden=mode==='json';
 for(const panel of scope.querySelectorAll<HTMLElement>('[data-json-only]'))panel.hidden=mode!=='json';
 for(const panel of scope.querySelectorAll<HTMLElement>('[data-formatted-only]'))panel.hidden=mode!=='formatted';
}
