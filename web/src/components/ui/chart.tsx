// Adapted from shadcn/ui @ 8a3536678a528cc012b9e62484290e79d0be410f.
// MIT; see web/licenses/shadcn-ui.txt. Only the used chart container/tooltip parts.
import type {ComponentProps} from 'react';
import {ResponsiveContainer} from 'recharts';

export function ChartContainer({children}:Pick<ComponentProps<typeof ResponsiveContainer>,'children'>){
 return <div data-slot="chart" className="answer-chart-container"><ResponsiveContainer width="100%" height={240}>{children}</ResponsiveContainer></div>;
}
type Payload={name?:string|number;value?:unknown;color?:string;payload?:{label?:string}};
export function ChartTooltipContent({active,payload,label,name}:{active?:boolean;payload?:readonly Payload[];label?:string|number;name?:string}){
 if(!active||!payload?.length)return null;
 const heading=payload[0]?.payload?.label??label;
 return <div className="answer-chart-tooltip">{heading!==undefined&&<div className="answer-tooltip-label">{heading}</div>}{payload.map((item,index)=>typeof item.value==='number'||typeof item.value==='string'?<div className="answer-tooltip-row" key={index}><span className="answer-tooltip-indicator" style={{backgroundColor:item.color}}/><span>{payload.length===1?name??item.name:item.name}</span><strong>{typeof item.value==='number'?item.value.toLocaleString():item.value}</strong></div>:null)}</div>;
}
