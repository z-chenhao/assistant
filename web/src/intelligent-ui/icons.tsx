import type {SVGProps} from 'react';
export function Icon({name,...props}:SVGProps<SVGSVGElement>&{name:string}){
 const paths:Record<string,string>={more:'M5 12h.01M12 12h.01M19 12h.01',close:'m6 6 12 12M6 18 18 6',expand:'M8 3H3v5M16 3h5v5M3 16v5h5M21 16v5h-5',pip:'M3 4h18v16H3zM12 11h7v7h-7z',modal:'M4 4h16v16H4zM8 8h8',filter:'M4 5h16l-6 7v6l-4 2v-8z',sort:'m8 5-4 4m4-4v14m8 0 4-4m-4 4V5',download:'M12 3v12m-5-5 5 5 5-5M4 16v5h16v-5',reset:'M4 10a8 8 0 1 1 1 8M4 4v6h6',left:'m15 5-7 7 7 7',right:'m9 5 7 7-7 7',check:'m5 12 4 4L19 6',zoom:'M10 4a6 6 0 1 0 0 12 6 6 0 0 0 0-12m5 11 5 5M7 10h6m-3-3v6'};
 paths.down='m6 9 6 6 6-6';
 return <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}><path d={paths[name]??paths.more}/></svg>
}
