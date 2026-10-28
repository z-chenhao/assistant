package agent

// OperationError separates infrastructure boundaries without exposing provider
// bodies, SQL text or private context. Use errors.Is/As to inspect the cause.
type OperationError struct {
	Operation string
	Category  error
	Cause     error
}

func (e *OperationError) Error() string        { return e.Category.Error() }
func (e *OperationError) Unwrap() error        { return e.Cause }
func (e *OperationError) Is(target error) bool { return target == e.Category }
