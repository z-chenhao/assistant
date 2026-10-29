package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templates embed.FS

var layout = template.Must(template.New("layout.html").Funcs(template.FuncMap{
	"authURL":         AuthURL,
	"inspectURL":      inspectionURL,
	"timeInput":       timeInput,
	"runRole":         runRole,
	"taskAction":      taskAction,
	"indent":          treeIndent,
	"executionTree":   inspectionTree,
	"contextSections": func() []string { return []string{"messages", "tools", "response", "provenance", "recall"} },
	"contextURL":      contextURL,
	"contextLink":     contextLink,
	"diffLink":        diffLink,
	"partEnd":         func(v *contextView) int { return v.Query.Offset + len(v.Frozen.Content) },
}).ParseFS(templates, "templates/*.html"))
