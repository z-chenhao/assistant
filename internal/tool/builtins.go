package tool

// BuiltinFamily names a Host transaction/execution boundary. It is not frozen
// model metadata: wire names, policies and capability hashes remain unchanged.
type BuiltinFamily string

const (
	TaskBuiltin         BuiltinFamily = "task"
	ArtifactBuiltin     BuiltinFamily = "artifact"
	AutomationBuiltin   BuiltinFamily = "automation"
	ComputerBuiltin     BuiltinFamily = "computer"
	CodeBuiltin         BuiltinFamily = "code"
	AgentBuiltin        BuiltinFamily = "agent"
	MemoryWriteBuiltin  BuiltinFamily = "memory_write"
	MemorySearchBuiltin BuiltinFamily = "memory_search"
)

type builtinPolicy struct {
	family     BuiltinFamily
	permission string
	internal   bool
}

// This is the single trusted name/policy catalog used for validation and Host
// dispatch. A provider cannot register an arbitrary Host name or permission.
var builtinCatalog = map[string]builtinPolicy{
	"builtin__task__search":        {TaskBuiltin, "read_only", false},
	"builtin__task__manage":        {TaskBuiltin, "task_control", false},
	"builtin__artifact__search":    {ArtifactBuiltin, "read_only", false},
	"builtin__artifact__manage":    {ArtifactBuiltin, "artifact_write", false},
	"builtin__automation__manage":  {AutomationBuiltin, "task_control", false},
	"builtin__automation__observe": {AutomationBuiltin, "automation_observe", false},
	"builtin__computer__discover":  {ComputerBuiltin, "read_only", true},
	"builtin__computer__bind":      {ComputerBuiltin, "read_only", true},
	"builtin__computer__invoke":    {ComputerBuiltin, "read_only", true},
	"builtin__codemode__execute":   {CodeBuiltin, "read_only", false},
	"builtin__agent__run":          {AgentBuiltin, "spawn_agents", false},
	"builtin__memory__manage":      {MemoryWriteBuiltin, "memory_control", false},
	"builtin__memory__search":      {MemorySearchBuiltin, "read_only", false},
}

func HostFamily(name string) (BuiltinFamily, bool) {
	p, ok := builtinCatalog[name]
	return p.family, ok
}
