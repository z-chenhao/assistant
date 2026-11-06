"use client";
import {useState} from 'react';
import {t} from '../i18n';
import {z} from 'zod';
import {AnswerSurface} from '../intelligent-ui/surface';
import {useUIState} from '../intelligent-ui/state';
import {useInteractionGroup} from '../intelligent-ui/group';
import {csv,download,safeName} from '../intelligent-ui/exports';
import {Icon} from '../intelligent-ui/icons';
import {Input} from './ui/answer-controls';
export const TableProps=z.object({title:z.string().max(300),columns:z.array(z.string().max(200)).min(1).max(12),rows:z.array(z.array(z.string().max(2000)).max(12)).max(200),source:z.string().max(1000).describe('Data provenance: user-provided, calculated, sourced, or explicitly illustrative.')});
export function DataTable({props,reference=''}:{props:z.infer<typeof TableProps>;reference?:string}){
 const parsed=TableProps.safeParse(props);const group=useInteractionGroup();const[state,update,saved]=useUIState(reference,{sort:-1,direction:1},s=>({title:props.title,sort:s.sort>=0?props.columns[s.sort]:null,direction:s.direction}));const [filterOpen,setFilterOpen]=useState(false);
 if(!parsed.success||parsed.data.rows.some(row=>row.length!==parsed.data.columns.length))return <p role="alert">{t('This table has invalid rows or columns. Ask the assistant to regenerate it.')}</p>;
 const{title,columns,rows,source}=parsed.data;
 const numeric=columns.map((_,index)=>rows.some(row=>row[index].trim()!=='')&&rows.every(row=>row[index].trim()===''||Number.isFinite(Number(row[index]))));
 const filtered=rows.filter(row=>(!group.query||row.some(cell=>cell.toLocaleLowerCase().includes(group.query.toLocaleLowerCase())))&&(!group.selected||row.includes(group.selected)));
 const visible=state.sort<0?filtered:[...filtered].sort((a,b)=>state.direction*a[state.sort].localeCompare(b[state.sort],undefined,{numeric:true,sensitivity:'base'}));
 return <AnswerSurface title={title} actions={[{label:'Filter',icon:<Icon name="filter"/>,run:()=>setFilterOpen(!filterOpen)},{label:'Reset',icon:<Icon name="reset"/>,run:()=>{group.setQuery('');if(group.selected)group.select(group.selected);update({sort:-1,direction:1})}},{label:'Download CSV',icon:<Icon name="download"/>,run:()=>download(csv([columns,...visible]),'text/csv;charset=utf-8',safeName(title)+'.csv')}]}>
  {(filterOpen||group.query)&&<div className="answer-filter"><Input type="search" aria-label={t('Filter rows')} placeholder={t('Filter')} value={group.query} onChange={event=>group.setQuery(event.target.value)}/></div>}
  <section className="answer-table" aria-label={title}><div className="answer-table-scroll" role="region" aria-label={`${title} ${t('table')}`} tabIndex={0}><table><caption>{title}</caption><thead><tr>{columns.map((column,index)=><th key={index} scope="col" data-numeric={numeric[index]} aria-sort={state.sort===index?(state.direction===1?'ascending':'descending'):'none'}><button type="button" className="answer-column-sort" onClick={()=>update({sort:index,direction:state.sort===index?-state.direction:1})}>{column}{state.sort===index&&<Icon name="sort"/>}</button></th>)}</tr></thead><tbody>{visible.map((row,index)=><tr key={index} aria-selected={group.selected?row.includes(group.selected):undefined}>{row.map((cell,column)=><td key={column} data-numeric={numeric[column]}>{column===0?<button type="button" className="answer-cell-select" aria-pressed={group.selected===cell} onClick={()=>group.select(cell)}>{cell}</button>:cell}</td>)}</tr>)}</tbody></table>{!visible.length&&<p>{t('No rows to display.')}</p>}</div><p className="answer-table-source">{source}</p></section>
  {saved.error&&<p className="field-help" role="alert">{t(saved.error)}</p>}
 </AnswerSurface>
}
