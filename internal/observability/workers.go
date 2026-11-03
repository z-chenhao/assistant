package observability

// WorkerActivity is a bounded, read-only workspace projection. Execution state
// stays owned by the runtime; prompts and tool payloads are not part of it.
type WorkerActivity struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	TaskRef   string `json:"task_ref,omitempty"`
	CreatedAt string `json:"created_at"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
}
type WorkerActivityList struct {
	Items []WorkerActivity `json:"items"`
	More  bool             `json:"more"`
}
