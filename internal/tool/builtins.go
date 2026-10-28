package tool

// BuiltinFamily names a Host transaction/execution boundary. Model-visible
// names and frozen execution policy remain in Spec.
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
	"builtin__task_search":        {TaskBuiltin, "read_only", false},
	"builtin__task_manage":        {TaskBuiltin, "task_control", false},
	"builtin__artifact_search":    {ArtifactBuiltin, "read_only", false},
	"builtin__artifact_manage":    {ArtifactBuiltin, "artifact_write", false},
	"builtin__automation_manage":  {AutomationBuiltin, "task_control", false},
	"builtin__automation_observe": {AutomationBuiltin, "automation_observe", false},
	"builtin__computer_discover":  {ComputerBuiltin, "read_only", true},
	"builtin__computer_bind":      {ComputerBuiltin, "read_only", true},
	"builtin__computer_invoke":    {ComputerBuiltin, "read_only", true},
	"builtin__codemode_execute":   {CodeBuiltin, "read_only", false},
	"builtin__agent_run":          {AgentBuiltin, "spawn_agents", false},
	"builtin__memory_manage":      {MemoryWriteBuiltin, "memory_control", false},
	"builtin__memory_search":      {MemorySearchBuiltin, "read_only", false},
}

func HostFamily(name string) (BuiltinFamily, bool) {
	p, ok := builtinCatalog[name]
	return p.family, ok
}
