import {useEffect,useState,Component,type ReactNode} from 'react';
import {createRoot,type Root} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {z} from 'zod';
import {OpenIntelligentAnswer} from '../components/open-intelligent-ui/catalog';
import {t} from '../i18n';

const component=z.object({name:z.enum(['Table','barChart','pieChart','plan_visualization','scheduleTime','generateSandboxedUi','steward.lineChart','steward.scatterChart','steward.form','steward.cards']),props:z.unknown()});
export const uiPreviewFrame=z.object({revision:z.number().int().nonnegative(),key:z.string().regex(/^[a-f0-9]{32}$/),state:z.enum(['draft','discarded','committed']),reply_key:z.string().regex(/^[a-f0-9]{32}$/).optional(),items:z.array(z.object({key:z.string().regex(/^[a-f0-9]{32}$/),title:z.string().max(200),components:z.array(component).max(16).nullable()})).max(16).optional()});
type Item=NonNullable<z.infer<typeof uiPreviewFrame>['items']>[number];

// This read-only shell has its own scriptless CSP and never inherits the Host
// document's inline-style restrictions. All content comes from the live snapshot.
const visual=z.object({initialHeight:z.number().min(50).max(4000)});
function PreviewSandbox({props,title,session,item,index,revision}:{props:unknown;title:string;session:string;item:string;index:number;revision:number}){
 const[theme,setTheme]=useState(()=>document.documentElement.classList.contains('dark')?'dark':'light');
 useEffect(()=>{const observer=new MutationObserver(()=>setTheme(document.documentElement.classList.contains('dark')?'dark':'light'));observer.observe(document.documentElement,{attributes:true,attributeFilter:['class']});return()=>observer.disconnect()},[]);
 const parsed=visual.safeParse(props);if(!parsed.success)return null;
 const query=new URLSearchParams({key:session,item,component:String(index),revision:String(revision),theme});
 return <div className="intelligent-widget"><iframe title={title} sandbox="" referrerPolicy="no-referrer" style={{height:parsed.data.initialHeight,width:'100%',border:0}} src={'/ui/preview?'+query}/></div>;
}
class PreviewBoundary extends Component<{children:ReactNode},{failed:boolean}>{
 state={failed:false};static getDerivedStateFromError(){return {failed:true}}
 render(){return this.state.failed?null:this.props.children}
}
function PreviewAnswer({item,session,revision}:{item:Item;session:string;revision:number}){
 return <article className="message assistant message-split" aria-busy="true" data-ui-preview={item.key}><div className="message-bubble">
  <p className="field-help" role="status">{t('Generating interactive answer…')}</p>
  {!item.components?.length&&item.title&&<strong>{item.title}</strong>}
  <div inert>{item.components?.map((c,index)=><PreviewBoundary key={index}>{c.name==='generateSandboxedUi'?<PreviewSandbox props={c.props} title={item.title} session={session} item={item.key} index={index} revision={revision}/>:<OpenIntelligentAnswer value={{...c,disabled:true}} fallback=""/>}</PreviewBoundary>)}</div>
 </div></article>;
}

export function createUIPreview(node:HTMLElement){
 let root:Root|null=null,key='',replyKey='';
 const clear=()=>{root?.unmount();root=null;key='';replyKey='';node.hidden=true;node.replaceChildren()};
 const reconcile=(deferred=false)=>{if(replyKey&&(deferred||document.querySelector(`[data-reply-key="${replyKey}"]`)))clear()};
 return {
  clear,reconcile,
  receive(data:unknown){
   const parsed=uiPreviewFrame.safeParse(data);if(!parsed.success)return;
   const frame=parsed.data;
   if(frame.state==='discarded'){if(frame.key===key)clear();return}
   if(frame.state==='committed'){if(frame.key===key){replyKey=frame.reply_key??'';reconcile();return 'committed' as const}return}
   if(key!==frame.key)clear();key=frame.key;replyKey='';
   const items=frame.items??[];node.hidden=!items.length;
   if(!items.length){root?.unmount();root=null;return}
   root??=createRoot(node);
   flushSync(()=>root!.render(<>{items.map(item=><PreviewAnswer key={item.key} item={item} session={frame.key} revision={frame.revision}/>)}</>));
  }
 };
}
