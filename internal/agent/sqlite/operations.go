package sqlite

import (
	"context"

	"github.com/z-chenhao/collie/internal/operations"
	operationssqlite "github.com/z-chenhao/collie/internal/operations/sqlite"
)

func (s *Store) Operations(ctx context.Context) (operations.Report, error) {
	return (operationssqlite.Store{DB: s.db, Limits: s.schedulerLimits()}).Report(ctx)
}
func (s *Store) OperationalTrace(ctx context.Context, id string) (operations.RunTrace, error) {
	return (operationssqlite.Store{DB: s.db}).Trace(ctx, id)
}
