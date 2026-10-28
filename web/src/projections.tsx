import {Component,type ReactNode} from 'react';
import {flushSync} from 'react-dom';
import {createRoot,type Root} from 'react-dom/client';
import {z} from 'zod';
import {Button} from './components/ui/button';
import {Card, CardContent} from './components/ui/card';
import {DataTable, TableProps} from './components/data-table';

// This renderer consumes the existing Host-owned UISpec v1. No model code,
// HTML interpreter, visualization router, provider or credential path is added.
const text=z.string().max(128*1024);
const component=z.discriminatedUnion('type',[
  z.object({type:z.literal('markdown'),text:text.default('')}),
  z.object({type:z.literal('artifact'),text:text.default(''),artifact_ref:z.string().regex(/^artifact:[^/\\\s\0]*$/)}),
  z.object({type:z.literal('progress'),text:text.default(''),status:z.string().optional()}),
  z.object({type:z.literal('table'),columns:TableProps.shape.columns,rows:TableProps.shape.rows}),
  z.object({type:z.literal('confirmation'),text:text.default(''),approval_id:z.string().min(1),frozen_arguments_hash:z.string().length(64)}),
  z.object({type:z.literal('form'),fields:z.array(z.object({name:z.string().min(1),label:z.string().min(1),value:z.string()})).min(1).max(16)}),
]);
const projection=z.object({schema_version:z.literal(1),fallback:z.string(),components:z.array(component).max(128)});
type UIComponent=z.infer<typeof component>;

class RenderBoundary extends Component<{children:ReactNode;fallback:string},{failed:boolean}> {
  state={failed:false};
  static getDerivedStateFromError(){return {failed:true}}
  render(){return this.state.failed?<p className="semantic">{this.props.fallback}</p>:this.props.children}
}

function NativeComponent({value:c}:{value:UIComponent}) {
  switch(c.type){
    case 'markdown':return <div className="semantic">{c.text}</div>;
    case 'artifact':return <Card className="artifact-card"><CardContent><a className="artifact" href={'/artifact?ref='+encodeURIComponent(c.artifact_ref)}>↓ {c.text} attachment</a></CardContent></Card>;
    case 'progress':return <p className="progress">{c.status} · {c.text}</p>;
    case 'table':return <DataTable props={{title:'Comparison',columns:c.columns,rows:c.rows,source:'From this conversation'}}/>;
    case 'confirmation':return <Card className="approval"><CardContent><p className="semantic">{c.text}</p><form action="/approval" method="post"><input type="hidden" name="approval_id" value={c.approval_id}/><input type="hidden" name="hash" value={c.frozen_arguments_hash}/><Button name="decision" value="approved">Approve</Button><Button variant="secondary" name="decision" value="denied">Decline</Button></form></CardContent></Card>;
    case 'form':return <dl>{c.fields.map(f=><div key={f.name}><dt>{f.label}</dt><dd>{f.value}</dd></div>)}</dl>;
  }
}

const roots=new Map<HTMLElement,Root>();
export function enhanceProjections(scope:ParentNode=document){

for(const node of scope.querySelectorAll<HTMLElement>('[data-ui-spec]')){
  if(roots.has(node))continue;
  try{
    const result=projection.safeParse(JSON.parse(node.dataset.uiSpec||''));
    // Retain the server's escaped fallback for unknown or unsupported specs.
    if(!result.success)continue;
    const spec=result.data;
    const fallback=node.dataset.uiFallback;
    if(fallback===undefined)continue;
    if(spec.components.some(c=>c.type==='table'&&c.rows.some(row=>row.length!==c.columns.length)))continue;
    const root=createRoot(node);roots.set(node,root);
    flushSync(()=>root.render(<RenderBoundary fallback={fallback}>{spec.components.length?spec.components.map((c,i)=><RenderBoundary key={i} fallback={fallback}><NativeComponent value={c}/></RenderBoundary>):<p className="semantic">{fallback}</p>}</RenderBoundary>));
    node.dataset.renderer='native';
  }catch{/* Malformed projection retains the existing semantic HTML. */}
}
}
export function releaseProjections(scope:HTMLElement){
 for(const [node,root] of roots){if(scope===node||scope.contains(node)){root.unmount();roots.delete(node)}}
}
