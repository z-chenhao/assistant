// Adapted from OpenIntelligentUI @ 8391a94; MIT, web/licenses/OpenIntelligentUI.txt.
import {z} from 'zod';
import {ChartExplorer} from '../../../intelligent-ui/charts';
export const BarChartProps=z.object({title:z.string(),description:z.string(),data:z.array(z.object({label:z.string(),value:z.number()}))});
export function BarChart(props:z.infer<typeof BarChartProps>&{reference?:string}){return <ChartExplorer kind="bar" {...props}/>}
