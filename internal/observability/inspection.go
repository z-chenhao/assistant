package observability

import (
	"errors"
	"time"
)

var ErrInvalid = errors.New("invalid inspection query")
var ErrNotFound = errors.New("execution not found")

// Execution exposes relationship metadata only. Prompts, tool payloads,
// provider/account configuration and ownership tokens stay out of this model.
type Execution struct {
	ID         string `json:"run_id"`
	Role       string `json:"role"`
	Status     string `json:"status"`
	WaitReason string `json:"wait_reason,omitempty"`
	Parent     string `json:"parent_run_id,omitempty"`
	Root       string `json:"root_run_id,omitempty"`
	Depth      int    `json:"depth"`
	Epoch      int64  `json:"task_epoch,omitempty"`
	TaskRef    string `json:"task_ref,omitempty"`
	CreatedAt  string `json:"created_at"`
	StartedAt  string `json:"started_at,omitempty"`
	EndedAt    string `json:"ended_at,omitempty"`
}

type ExecutionQuery struct {
	Role, Status, Cursor string
	Since, Until         time.Time
	Limit                int
}

func (q ExecutionQuery) Validate() error {
	if q.Limit < 0 || q.Limit > 100 || len(q.Cursor) > 512 {
		return ErrInvalid
	}
	switch q.Role {
	case "", "main", "root", "child":
	default:
		return ErrInvalid
	}
	switch q.Status {
	case "", "queued", "running", "waiting", "completed", "failed", "cancelled":
	default:
		return ErrInvalid
	}
	if !q.Since.IsZero() && !q.Until.IsZero() && !q.Since.Before(q.Until) {
		return ErrInvalid
	}
	return nil
}

type Executions struct {
	Items []Execution `json:"items"`
	Next  string      `json:"next,omitempty"`
}

type TaskNode struct {
	Ref            string `json:"task_ref"`
	Title          string `json:"title"`
	TitleTruncated bool   `json:"title_truncated"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
}

// TaskLink comes from the original committed request receipt. Replayed Tool
// receipts cannot fabricate additional Task creation relationships.
type TaskLink struct {
	TaskNode
	Operation  string `json:"operation"`
	Invocation string `json:"invocation_id"`
	AcceptedAt string `json:"accepted_at"`
}

type RelationshipQuery struct {
	RunID, TaskRef, Cursor string
	Limit                  int
}

func (q RelationshipQuery) Validate() error {
	if (q.RunID == "") == (q.TaskRef == "") || len(q.RunID) > 128 || len(q.TaskRef) > 256 || len(q.Cursor) > 512 || q.Limit < 0 || q.Limit > 100 {
		return ErrInvalid
	}
	return nil
}

type Relationships struct {
	Selected   *Execution  `json:"selected,omitempty"`
	Task       *TaskNode   `json:"task,omitempty"`
	Creator    *Execution  `json:"creator,omitempty"`
	Executions []Execution `json:"executions"`
	Tasks      []TaskLink  `json:"tasks"`
	Next       string      `json:"next,omitempty"`
}
