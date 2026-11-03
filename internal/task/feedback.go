package task

import "errors"

// Feedback is an owner-facing question or review. Its answer is input to Main,
// never a Task transition or an external-action approval.
type Feedback struct {
	Ref             string   `json:"feedback_ref"`
	TaskRef         string   `json:"task_ref"`
	Kind            string   `json:"kind"`
	Prompt          string   `json:"prompt"`
	SourceMessageID string   `json:"source_message_id"`
	ArtifactRefs    []string `json:"artifact_refs"`
	Status          string   `json:"status"`
	CreatedAt       string   `json:"created_at"`
	ExpiresAt       string   `json:"expires_at"`
}

var ErrFeedbackStale = errors.New("feedback is no longer current; inspect the task before continuing")

func ValidFeedbackAnswer(answer string) bool { return validText(answer) }

// FeedbackPage contains only actionable questions/reviews, with a stable opaque
// reference cursor. Historical responses remain in the audit store.
type FeedbackPage struct {
	Items []Feedback `json:"items"`
	Next  string     `json:"next,omitempty"`
}
