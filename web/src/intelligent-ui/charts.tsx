import {t} from '../i18n';
import {useState} from 'react';
import {BarChart,Bar,PieChart,Pie,LineChart,Line,ScatterChart,Scatter,XAxis,YAxis,Tooltip,CartesianGrid,Cell} from 'recharts';
import {CHART_COLORS} from '../components/open-intelligent-ui/charts/config';
import {ChartContainer,ChartTooltipContent} from '../components/ui/chart';
import {AnswerSurface} from './surface';
import {useInteractionGroup} from './group';
import {useUIState} from './state';
import {csv,download,safeName} from './exports';
import {Icon} from './icons';
export type ChartData={label:string;value?:number;x?:number;y?:number};
export function ChartExplorer({kind,title,description,data,reference=''}:{kind:'bar'|'pie'|'line'|'scatter';title:string;description:string;data:ChartData[];reference?:string}){
 const group=useInteractionGroup();const[zoom,setZoom]=useState(false);const[state,update,saved]=useUIState(reference,{start:0,end:Math.max(0,data.length-1)},s=>({title,range:[s.start,s.end]}));
 const filtered=data.filter(d=>!group.query||d.label.toLocaleLowerCase().includes(group.query.toLocaleLowerCase()));const shown=zoom?filtered.slice(state.start,state.end+1):filtered;
 const reduced=matchMedia('(prefers-reduced-motion: reduce)').matches;const choose=(point:unknown)=>{const p=point as {label?:string;payload?:{label?:string}};const label=p?.label??p?.payload?.label;if(label)group.select(label)};
 const common={data:shown,margin:{top:8,right:12,bottom:0,left:0}};const axes=<><CartesianGrid vertical={false} stroke="var(--border)"/><XAxis dataKey={kind==='scatter'?'x':'label'} type={kind==='scatter'?'number':'category'} axisLine={false} tickLine={false} tickMargin={10} tick={{fontSize:12,fill:'var(--muted-foreground)'}}/><YAxis dataKey={kind==='scatter'?'y':undefined} type="number" axisLine={false} tickLine={false} tickMargin={8} width={36} tick={{fontSize:12,fill:'var(--muted-foreground)'}}/><Tooltip cursor={{fill:'var(--hover)',fillOpacity:.6,stroke:'var(--border)'}} content={<ChartTooltipContent name={title}/>}/></>;
 const dot=(point:unknown)=>{const p=point as {cx?:number;cy?:number;payload?:ChartData};if(p.cx===undefined||p.cy===undefined||!p.payload)return <g/>;const label=p.payload.label;return <g className="recharts-line-dot" data-chart-label={label} role="button" tabIndex={0} aria-label={label+': '+p.payload.value} onClick={()=>group.select(label)} onKeyDown={event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();group.select(label)}}}><circle cx={p.cx} cy={p.cy} r={12} fill="transparent"/><circle cx={p.cx} cy={p.cy} r={3} fill={group.selected===label?CHART_COLORS[0]:'var(--muted)'} stroke={CHART_COLORS[0]} strokeWidth={1.5}/></g>};
 const colors=shown.map((d,index)=><Cell key={index} fill={CHART_COLORS[kind==='bar'||kind==='scatter'?0:index%CHART_COLORS.length]} fillOpacity={!group.selected||group.selected===d.label?1:.3}/>);
 const exportRows=kind==='scatter'?[['Label','X','Y'],...shown.map(d=>[d.label,d.x,d.y])]:[['Label','Value'],...shown.map(d=>[d.label,d.value])];
 return <AnswerSurface title={title} graphic actions={[{label:'Zoom',icon:<Icon name="zoom"/>,disabled:kind==='pie',run:()=>setZoom(!zoom)},{label:'Reset',icon:<Icon name="reset"/>,run:()=>{setZoom(false);update({start:0,end:Math.max(0,data.length-1)});group.setQuery('');if(group.selected)group.select(group.selected)}},{label:'Download CSV',run:()=>download(csv(exportRows),'text/csv;charset=utf-8',safeName(title)+'.csv')}] }>
  <div className="native-chart"><div className="native-answer-heading"><h3 className="native-answer-title">{title}</h3>{description&&<p className="native-answer-description">{description}</p>}</div>
  {!shown.length?<p className="native-answer-empty">{t('No data available')}</p>:<ChartContainer>
   {kind==='bar'?<BarChart {...common}>{axes}<Bar dataKey="value" maxBarSize={48} radius={[5,5,0,0]} isAnimationActive={!reduced} onClick={choose}>{colors}</Bar></BarChart>
    :kind==='pie'?<PieChart><Pie data={shown} dataKey="value" nameKey="label" innerRadius={56} outerRadius={88} paddingAngle={3} cornerRadius={4} stroke="none" isAnimationActive={!reduced} onClick={choose}>{colors}</Pie><Tooltip content={<ChartTooltipContent name={title}/>}/></PieChart>
    :kind==='line'?<LineChart {...common}>{axes}<Line type="monotone" dataKey="value" stroke={CHART_COLORS[0]} strokeWidth={2} dot={dot} activeDot={{r:5}} isAnimationActive={!reduced}/></LineChart>
    :<ScatterChart margin={common.margin}>{axes}<Scatter name={title} data={shown} fill={CHART_COLORS[0]} isAnimationActive={!reduced} onClick={choose}>{colors}</Scatter></ScatterChart>}
  </ChartContainer>}
  {zoom&&filtered.length>1&&<div className="answer-chart-range"><label>{t('From')}<input type="range" min="0" max={Math.max(0,state.end)} value={state.start} onChange={event=>update({start:Number(event.target.value)})}/></label><label>{t('To')}<input type="range" min={state.start} max={filtered.length-1} value={state.end} onChange={event=>update({end:Number(event.target.value)})}/></label></div>}
  {kind==='pie'&&<div className="native-chart-legend">{shown.map((d,i)=><button type="button" key={i} className="native-chart-legend-item" aria-pressed={group.selected===d.label} onClick={()=>group.select(d.label)}><span className="native-chart-legend-color" style={{backgroundColor:CHART_COLORS[i%CHART_COLORS.length]}}/>{d.label}</button>)}</div>}
  </div>{saved.error&&<p className="field-help" role="alert">{t(saved.error)}</p>}
 </AnswerSurface>
}
