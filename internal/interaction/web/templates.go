package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templates embed.FS

var layout = template.Must(template.New("layout.html").Funcs(template.FuncMap{
	"capabilityURL": capabilityURL,
	"traceURL":      traceURL, "traceListURL": traceListURL,
	"otherProfiles": otherProfiles,
	"modelURL":      modelURL, "activeProfile": activeProfile, "canSend": canSend, "providerName": providerName,
	"bindingTarget": bindingTarget, "humanTime": humanTime, "observedTime": humanTime, "toolLabel": toolLabel,
	"taskURL": taskURL, "taskListURL": taskListURL, "taskViews": taskViews, "historyLabel": historyLabel,
	"conversationURL": conversationURL, "tabLabel": tabLabel,
	"prettyBody":  prettyBody,
	"observedAt":  observedAt,
	"duration":    duration,
	"elapsed":     elapsed,
	"obsStatuses": func() []string { return []string{"queued", "running", "waiting", "completed", "failed", "cancelled"} },
	"obsRole":     observationRole,
	"parentRun":   observationParent,

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
