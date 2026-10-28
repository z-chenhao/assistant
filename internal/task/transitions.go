package task

import "errors"

// Status is the Task lifecycle. A failed Run never introduces a failed Task.
type Status string

const (
	Active    Status = "active"
	Paused    Status = "paused"
	Completed Status = "completed"
	Cancelled Status = "cancelled"
)

var ErrDraining = errors.New("task tree is still draining")

type AppendPlan struct {
	FreshRoot bool
	Epoch     int64
}

// PlanAppend consumes an authoritative transaction snapshot. The adapter still
// owns CAS, privacy checks, pins and creating/steering the Root atomically.
func PlanAppend(status Status, epoch int64, liveRoots, liveTree int) (AppendPlan, error) {
	fresh := status != Active || liveRoots == 0
	if fresh && (liveRoots > 0 || liveTree > 0) {
		return AppendPlan{}, ErrDraining
	}
	if fresh {
		epoch++
	}
	return AppendPlan{FreshRoot: fresh, Epoch: epoch}, nil
}
func CanComplete(status Status, pending bool) bool { return status == Active && !pending }
func CanStop(status, target Status) bool {
	return status == target || status == Active || (status == Paused && target == Cancelled)
}
