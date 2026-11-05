package web

import (
	"context"
	"fmt"
	"html"
	"net/http"

	"github.com/z-chenhao/assistant/internal/delivery/ui"
)

type widgetReader interface {
	WebWidget(context.Context, string) (ui.Widget, error)
}

func (s *Server) widget(w http.ResponseWriter, r *http.Request) {
	reader, ok := s.Core.(widgetReader)
	if !ok {
		fail(w, 503)
		return
	}
	doc, err := reader.WebWidget(r.Context(), r.URL.Query().Get("ref"))
	if err != nil || doc.HTML == "" {
		fail(w, 404)
		return
	}
	// CSP sandbox applies to every HTML response. Direct browser navigation
	// receives only text, keeping generated code inside the Host-owned frame.
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts; default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'; frame-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'")
	w.Header().Set("Origin-Agent-Cluster", "?1")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), clipboard-read=(), clipboard-write=()")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Header.Get("Sec-Fetch-Dest") == "document" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, doc.Fallback)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The bridge only proposes bounded text and resize events; the parent
	// requires its own visible Send button before accepting any user input.
	// Bootstrap precedes all generated CSS/HTML, which may contain closing tags.
	_, _ = fmt.Fprint(w, `<!doctype html><html data-widget-theme="light"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`+html.EscapeString(doc.Title)+`</title><script>
(()=>{for(const name of ['RTCPeerConnection','webkitRTCPeerConnection','Worker','SharedWorker']){try{Object.defineProperty(window,name,{value:undefined,writable:false,configurable:false})}catch{}}const send=(type,value)=>parent.postMessage({channel:'steward-widget-v1',type,...value},'*');Object.defineProperty(window,'steward',{value:Object.freeze({sendPrompt:({text})=>{if(typeof text==='string'&&text.trim()&&text.length<=4096)send('prompt',{text})}}),writable:false,configurable:false});addEventListener('message',event=>{if(event.source!==parent||event.data?.channel!=='steward-widget-v1')return;if(event.data.type==='theme')document.documentElement.dataset.widgetTheme=event.data.theme==='dark'?'dark':'light'});addEventListener('error',event=>{send('error',{});event.preventDefault()});addEventListener('unhandledrejection',event=>{send('error',{});event.preventDefault()});addEventListener('DOMContentLoaded',()=>{send('ready',{});new ResizeObserver(()=>send('resize',{height:document.documentElement.scrollHeight})).observe(document.body)});})();
</script><style>:root{color-scheme:light;--background:#fff;--foreground:#252521;--muted:#f4f3ee;--border:#dddcd4;--brand:#7050b5} :root[data-widget-theme=dark]{color-scheme:dark;--background:#252521;--foreground:#efeee8;--muted:#34342e;--border:#55554b;--brand:#b39bd9}body{margin:0;padding:16px;background:var(--background);color:var(--foreground);font:15px/1.5 system-ui,sans-serif}*{box-sizing:border-box}button,input,select{font:inherit}button{cursor:pointer}svg,img,canvas{max-width:100%}`+doc.CSS+`</style></head><body>`+doc.HTML+`<script>`+doc.JavaScript+`</script></body></html>`)
}
