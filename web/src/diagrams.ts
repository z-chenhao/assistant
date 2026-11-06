import mermaid from 'mermaid';
window.stewardDiagram=async(source,id)=>{
 const dark=document.documentElement.classList.contains('dark');
 mermaid.initialize({startOnLoad:false,securityLevel:'strict',suppressErrorRendering:true,maxTextSize:20000,maxEdges:200,secure:['securityLevel','startOnLoad','maxTextSize','maxEdges','htmlLabels','flowchart'],theme:dark?'dark':'neutral',fontFamily:'Inter, PingFang SC, sans-serif',flowchart:{htmlLabels:false}});
 const {svg}=await mermaid.render(id,source);
 return svg;
};
