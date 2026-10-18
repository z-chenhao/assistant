// Package computer owns the Computer execution scope. Surface leases bind to
// this trusted identity; scripts cannot select it in their arguments.
package computer

import "context"

type executionKey struct{}

func WithExecution(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionKey{}, id)
}
func Execution(ctx context.Context) string { id, _ := ctx.Value(executionKey{}).(string); return id }
