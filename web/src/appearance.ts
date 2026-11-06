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
// Favicons live outside the document's CSS cascade; use the atlas-derived
// light character explicitly whenever the workspace switches to dark mode.
const favicon=document.querySelector<HTMLLinkElement>('link[rel="icon"]');
const syncFavicon=()=>{if(favicon)favicon.href=html.classList.contains('dark')?'/brand/ghost-ivory.png':'/brand/ghost-charcoal.png'};
syncFavicon();
new MutationObserver(syncFavicon).observe(html,{attributes:true,attributeFilter:['class']});
html.classList.toggle('sidebar-collapsed',collapsed&&matchMedia('(min-width: 761px)').matches);

// Remember only the browser zone; no account or scheduler settings are changed.
try{document.cookie=`steward-timezone=${encodeURIComponent(Intl.DateTimeFormat().resolvedOptions().timeZone)}; Path=/; Max-Age=31536000; SameSite=Strict`}catch{}
