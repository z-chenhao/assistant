// Run before CSS so navigation and client-only slots have their final layout.
const html=document.documentElement;
html.classList.add('js');
// Restore native navigation/forms if the deferred bundle cannot be downloaded.
window.addEventListener('error',event=>{
 if(event.target instanceof HTMLScriptElement&&event.target.getAttribute('src')==='/app.js')html.classList.remove('js','sidebar-collapsed');
},true);
let saved:string|null=null,collapsed=false;
try{saved=localStorage.getItem('steward-theme');collapsed=localStorage.getItem('steward-sidebar')==='collapsed'}catch{}
html.classList.toggle('dark',saved==='dark'||!saved&&matchMedia('(prefers-color-scheme: dark)').matches);
html.classList.toggle('sidebar-collapsed',collapsed&&matchMedia('(min-width: 761px)').matches);
