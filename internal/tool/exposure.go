package tool

// Exposure controls discovery and declaration. It never grants a capability.
// The empty value retains the direct declarations of historical frozen specs.
type Exposure string

// Non-CodeMode values remain readable only for historical frozen specs and
// fixed builtin surfaces. MCP configuration does not expose these choices.
const (
	Direct    Exposure = "direct"
	CodeMode  Exposure = "codemode"
	Deferred  Exposure = "deferred"
	Hidden    Exposure = "hidden"
	ModelOnly Exposure = "model-only"
)

func (e Exposure) Valid() bool {
	switch e {
	case "", Direct, CodeMode, Deferred, Hidden, ModelOnly:
		return true
	}
	return false
}

func (s Spec) Searchable() bool {
	return !s.Internal && (s.Exposure == CodeMode || s.Exposure == Deferred)
}

func (s Spec) ScriptCallable() bool {
	return s.Exposure != Hidden && s.Exposure != ModelOnly
}

func (s Spec) ModelVisible(loaded map[string]bool) bool {
	return !s.Internal && !s.CodeModeOnly && s.Exposure != Hidden && (!s.Searchable() || loaded[s.Name])
}
