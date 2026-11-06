package web

import (
	"embed"
	"html/template"
)

//go:embed templates/*.html
var templates embed.FS

func newLayout(language string) *template.Template {
	return template.Must(template.New("layout.html").Funcs(template.FuncMap{
		"tr":                  func(text string) string { return translate(language, text) },
		"fontURL":             fontURL,
		"help":                help,
		"memorySourcesOffset": memorySourcesOffset, "memoryOffset": memoryOffset, "memoryURL": memoryURL, "memorySourceLabel": memorySourceLabel, "memoryOriginLabel": memoryOriginLabel, "memoryStatus": memoryStatus,
		"capabilityURL": capabilityURL,
		"traceTime":     traceTime,
		"traceColumns": func() []string {
			return []string{"Created time", "Type", "Input", "Output", "Wall time", "Model", "Tokens", "Accepted cost", "Status", "Trace ID"}
		},
		"traceURL": traceURL, "traceListURL": traceListURL, "traceClearURL": traceClearURL, "tracePresetURL": tracePresetURL,
		"canSend":       canSend,
		"bindingTarget": bindingTarget, "humanTime": func(raw string) string { return localeTime(language, raw) }, "observedTime": func(raw string) string { return localeTime(language, raw) }, "toolLabel": toolLabel,
		"taskURL": taskURL, "taskListURL": taskListURL, "taskViews": taskViews, "historyLabel": historyLabel,
		"scheduleTime":      func(raw, zone string) string { return localScheduleTime(language, raw, zone) },
		"scheduleFrequency": scheduleFrequency,
		"conversationURL":   conversationURL, "tabLabel": tabLabel,
		"prettyBody":          prettyBody,
		"observedDateTime":    observedDateTime,
		"observedAt":          observedAt,
		"providerTimingLabel": providerTimingLabel,
		"duration":            duration,
		"elapsed":             elapsed,
		"obsStatuses":         func() []string { return []string{"queued", "running", "waiting", "completed", "failed", "cancelled"} },
		"obsKind":             observationKind,
		"obsRole":             observationRole,
		"parentRun":           observationParent,
		"obsIcon":             observationIcon,

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
		"partEnd":         func(v *contextView) int { return v.Query.Offset + len(v.Frozen.Content) },
	}).ParseFS(templates, "templates/*.html"))

}

var layout = newLayout("en")
var chineseLayout = newLayout("zh-CN")
