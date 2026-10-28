package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templates embed.FS

var layout = template.Must(template.New("layout.html").Funcs(template.FuncMap{"authURL": AuthURL, "inspectURL": inspectionURL, "timeInput": timeInput, "runRole": runRole, "taskAction": taskAction, "indent": treeIndent, "executionTree": inspectionTree}).ParseFS(templates, "templates/*.html"))
