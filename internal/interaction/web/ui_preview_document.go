package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	htmlnode "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The shell is a regular sandboxed response: srcdoc would inherit the Host's
// style-src 'self' and block the shared inline theme. GET reads only memory.
func (s *Server) previewWidget(w http.ResponseWriter, r *http.Request) {
	p := &s.uiPreviews
	p.mu.Lock()
	var props json.RawMessage
	title := ""
	if p.frame.State == "draft" && p.frame.Key == r.URL.Query().Get("key") {
		index, err := strconv.Atoi(r.URL.Query().Get("component"))
		for _, item := range p.frame.Items {
			if item.Key == r.URL.Query().Get("item") && err == nil && index >= 0 && index < len(item.Components) && item.Components[index].Name == "generateSandboxedUi" {
				props = item.Components[index].Props
				title = item.Title
				break
			}
		}
	}
	p.mu.Unlock()
	var visual struct {
		HTML string `json:"html"`
		CSS  string `json:"css"`
	}
	if props == nil || json.Unmarshal(props, &visual) != nil {
		fail(w, 404)
		return
	}
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'; script-src 'none'; style-src 'unsafe-inline'; font-src data:; img-src 'none'; connect-src 'none'; frame-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), clipboard-read=(), clipboard-write=()")
	w.Header().Set("Origin-Agent-Cluster", "?1")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Header.Get("Sec-Fetch-Dest") == "document" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprint(w, title)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	theme := "light"
	if r.URL.Query().Get("theme") == "dark" {
		theme = "dark"
	}
	styles, _ := assets.ReadFile("assets/widget.css")
	css := strings.ReplaceAll(visual.CSS, "<", `\3c `)
	_, _ = fmt.Fprint(w, `<!doctype html><html class="ogui-theme" data-widget-theme="`+theme+`"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>`+html.EscapeString(title)+`</title><style>`+string(styles)+"\n"+css+"\n"+`html,body{height:auto!important;min-height:0!important;margin:0;overflow-wrap:anywhere}*{box-sizing:border-box}svg,canvas{max-width:100%}</style></head><body inert>`+previewMarkup(visual.HTML)+`</body></html>`)
}

func previewMarkup(source string) string {
	nodes, err := htmlnode.ParseFragment(strings.NewReader(source), &htmlnode.Node{Type: htmlnode.ElementNode, Data: "body", DataAtom: atom.Body})
	if err != nil {
		return ""
	}
	allowed := " div span p h1 h2 h3 h4 h5 h6 strong em b i small code pre br hr ul ol li table caption colgroup col thead tbody tfoot tr th td dl dt dd blockquote form label input button select option optgroup textarea output progress meter fieldset legend section article header footer main figure figcaption canvas svg g path rect circle ellipse line polyline polygon text tspan defs marker clippath mask lineargradient radialgradient stop use symbol title desc pattern filter feblend fecolormatrix fecomponenttransfer fecomposite feconvolvematrix fediffuselighting fedisplacementmap fedistantlight fedropshadow feflood fefunca fefuncb fefuncg fefuncr fegaussianblur feimage femerge femergenode femorphology feoffset fepointlight fespecularlighting fespotlight fetile feturbulence "
	root := &htmlnode.Node{Type: htmlnode.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, node := range nodes {
		root.AppendChild(node)
	}
	var clean func(*htmlnode.Node)
	clean = func(parent *htmlnode.Node) {
		for node := parent.FirstChild; node != nil; {
			next := node.NextSibling
			if node.Type == htmlnode.ElementNode {
				tag := strings.ToLower(node.Data)
				if tag == "form" {
					node.Data = "div"
					node.DataAtom = atom.Div
				}
				if !strings.Contains(allowed, " "+tag+" ") {
					parent.RemoveChild(node)
					node = next
					continue
				}
				attrs := node.Attr[:0]
				for _, a := range node.Attr {
					key := strings.ToLower(a.Key)
					if key == "href" && strings.HasPrefix(a.Val, "#") {
						attrs = append(attrs, a)
						continue
					}
					if strings.HasPrefix(key, "on") {
						continue
					}
					switch key {
					case "href", "src", "srcset", "srcdoc", "action", "formaction", "autofocus", "contenteditable", "tabindex", "form", "background", "poster":
						continue
					}
					attrs = append(attrs, a)
				}
				node.Attr = attrs
				switch tag {
				case "input", "button", "select", "textarea":
					node.Attr = append(node.Attr, htmlnode.Attribute{Key: "disabled"})
				}
				clean(node)
			} else if node.Type != htmlnode.TextNode {
				parent.RemoveChild(node)
			}
			node = next
		}
	}
	clean(root)
	var out bytes.Buffer
	for node := root.FirstChild; node != nil; node = node.NextSibling {
		_ = htmlnode.Render(&out, node)
	}
	return out.String()
}
