package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templates embed.FS

var layout = template.Must(template.New("layout.html").Funcs(template.FuncMap{
	"traceURL":        traceURL,
	"prettyBody":      prettyBody,
	"observedAt":      observedAt,
	"duration":        duration,
	"elapsed":         elapsed,
	"obsStatuses":     func() []string { return []string{"queued", "running", "waiting", "completed", "failed", "cancelled"} },
	"obsRole":         observationRole,
	"parentRun":       observationParent,
	"obsIcon":         observationIcon,
	"obsTabs":         func() []string { return []string{"overview", "context", "diff", "wire", "metadata"} },
	"money":           money,
	"obsJSON":         viewJSON,
	"authURL":         AuthURL,
	"timeInput":       timeInput,
	"runRole":         runRole,
	"taskAction":      taskAction,
	"indent":          treeIndent,
	"contextSections": func() []string { return []string{"messages", "tools", "response", "provenance", "recall"} },
	"contextURL":      contextURL,
	"contextLink":     contextLink,
	"diffLink":        diffLink,
	"partEnd":         func(v *contextView) int { return v.Query.Offset + len(v.Frozen.Content) },
}).ParseFS(templates, "templates/*.html"))
