import {useEffect,useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {z} from 'zod';
import {SelectView} from './components/ui/select-view';
import {t} from './i18n';

const models=z.object({choices:z.array(z.object({key:z.string(),label:z.string(),disabled:z.boolean().optional()})),current:z.string(),config_version:z.number().int(),unavailable:z.boolean().optional()});
type Models=z.infer<typeof models>;
function ModelPicker({initial,composer}:{initial:Models;composer:HTMLFormElement}){
 const[data,setData]=useState(initial),[open,setOpen]=useState(false),[loading,setLoading]=useState(false),[saving,setSaving]=useState(false),[sending,setSending]=useState(!!composer.dataset.submitting),[error,setError]=useState('');
 useEffect(()=>{const observer=new MutationObserver(()=>setSending(!!composer.dataset.submitting));observer.observe(composer,{attributes:true,attributeFilter:['data-submitting']});return()=>observer.disconnect()},[composer]);
 async function refresh(){setLoading(true);setError('');try{const response=await fetch('/api/models',{cache:'no-store',signal:AbortSignal.timeout(30000)});if(!response.ok)throw Error();const value=models.parse(await response.json());setData(value);if(value.unavailable)setError('Some account models are unavailable. Try again.');return value}catch{setError('The account model list is unavailable.')}finally{setLoading(false)}}
 async function choose(value:string){
  if(loading||saving||composer.dataset.submitting||!value||value===data.current)return;
  setOpen(false);setSaving(true);setError('');composer.dataset.modelSelecting='true';
  const submit=composer.querySelector<HTMLButtonElement>('button[type=submit]');if(submit)submit.disabled=true;
  try{const response=await fetch('/model/choose',{method:'POST',headers:{Accept:'application/json'},signal:AbortSignal.timeout(30000),body:new URLSearchParams({consent:'yes',choice:value,config_version:String(data.config_version)})});if(!response.ok)throw Error();setData(models.parse(await response.json()))}
  catch{const latest=await refresh();if(latest?.current!==value)setError('Model selection failed. Try again.')}
  finally{delete composer.dataset.modelSelecting;setSaving(false);if(submit)submit.disabled=!!composer.dataset.submitting||document.querySelector<HTMLElement>('[data-can-send]')?.dataset.canSend==='false'}
 }
 const options=data.choices.map(choice=>({value:choice.key,label:choice.label,disabled:loading||!!choice.disabled,lang:''}));
 if(!data.current)options.unshift({value:'',label:t('Choose model'),disabled:true,lang:''});
 return <><SelectView id="composer-model-picker" label={t('Model')} value={data.current} options={options} disabled={saving||sending} busy={saving||loading} open={open} language={false} variant="inline" setOpen={value=>{setOpen(value);if(value)void refresh()}} choose={value=>void choose(value)}/>{error&&<span className="composer-model-error" role="status">{t(error)}</span>}</>;
}
export function enhanceComposerModels(){
 const host=document.querySelector<HTMLElement>('[data-composer-model]'),composer=document.querySelector<HTMLFormElement>('.composer');
 const select=host?.querySelector<HTMLSelectElement>('select');if(!host||!composer||!select)return;
 const initial:Models={choices:[...select.options].filter(option=>option.value).map(option=>({key:option.value,label:option.label})),current:host.dataset.current??'',config_version:Number(host.dataset.configVersion)};
 flushSync(()=>createRoot(host).render(<ModelPicker initial={initial} composer={composer}/>));
 document.querySelector('.model-native-submit')?.remove();
}
