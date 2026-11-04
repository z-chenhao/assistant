package app

import (
	"context"

	"github.com/z-chenhao/assistant/internal/database"
)

func (a *App) Database(ctx context.Context, q database.Query) (database.Page, error) {
	return a.databaseReader.Browse(ctx, q)
}
