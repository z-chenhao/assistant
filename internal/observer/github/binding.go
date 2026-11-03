package github

import (
	"encoding/json"
	"errors"

	"github.com/z-chenhao/assistant/internal/tool"
)

const bindSchema = `{"type":"object","properties":{"task_ref":{"type":"string","minLength":6,"maxLength":256},"task_epoch":{"type":"integer","minimum":1},"version":{"type":"integer","minimum":0}},"required":["task_ref","task_epoch","version"],"additionalProperties":false}`
const unbindSchema = `{"type":"object","properties":{"version":{"type":"integer","minimum":1}},"required":["version"],"additionalProperties":false}`

func ParseBinding(raw string, enabled bool) (BindingRequest, error) {
	var q BindingRequest
	schema := unbindSchema
	if enabled {
		schema = bindSchema
	}
	if !tool.Validate(schema, raw) || json.Unmarshal([]byte(raw), &q) != nil {
		return q, errors.New("invalid GitHub task binding request")
	}
	return q, nil
}

type BindingRequest struct {
	TaskRef   string `json:"task_ref"`
	TaskEpoch int64  `json:"task_epoch"`
	Version   int64  `json:"version"`
}

type TaskBinding struct {
	TaskRef      string `json:"task_ref,omitempty"`
	TaskEpoch    int64  `json:"task_epoch,omitempty"`
	Version      int64  `json:"version"`
	State        string `json:"state"`
	LastSequence int64  `json:"last_observation_sequence"`
}
