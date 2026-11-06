package sqlite

import (
	"context"
	"database/sql"

	"github.com/z-chenhao/assistant/internal/agent"
	"github.com/z-chenhao/assistant/internal/llm"
)

// ConfigureCostEstimator is startup composition; it does not change spend policy.
func (s *Store) ConfigureCostEstimator(estimator agent.CostEstimator) {
	s.costEstimator = estimator
}

func (s *Store) estimateCost(ctx context.Context, tx *sql.Tx, req llm.GenerationRequest, usage *llm.TokenUsage) error {
	if s.costEstimator == nil || usage == nil {
		return nil
	}
	var configured bool
	if err := tx.QueryRowContext(ctx, `SELECT price_json!='null' FROM spend_snapshot WHERE snapshot_id=?`, req.Snapshot.SnapshotID).Scan(&configured); err != nil {
		return err
	}
	if configured {
		return nil
	}
	quote, ok := s.costEstimator.Quote(req.Snapshot, *usage)
	if !ok {
		return nil
	}
	amount, err := monetaryCost(quote.Price, *usage)
	if err != nil {
		return nil // An unrepresentable estimate must not reject accepted usage.
	}
	price, err := encoded(quote.Price)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO generation_cost_estimate(generation_id,currency,estimated_micros,price_json,source) VALUES(?,?,?,?,?)`, req.GenerationID, quote.Currency, amount, price, quote.Source)
	return err
}
