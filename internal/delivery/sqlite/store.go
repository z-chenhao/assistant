// Package sqlite persists DeliveryRequest and its distinct delivery_outbox in
// state.db. The application's state owner installs the forward migration.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/z-chenhao/collie/internal/delivery"
	"github.com/z-chenhao/collie/internal/event"
	eventsqlite "github.com/z-chenhao/collie/internal/event/sqlite"
)

type Store struct{ DB *sql.DB }

func changed(ctx context.Context, tx *sql.Tx, id, status string) error {
	e, err := event.New("/collie/delivery", "ai.collie.delivery."+status+".v1", id, struct {
		ID     string `json:"delivery_id"`
		Status string `json:"status"`
	}{id, status})
	if err != nil {
		return delivery.ErrStorage
	}
	return eventsqlite.Enqueue(ctx, tx, e)
}

const columns = `d.delivery_id,d.message_id,d.conversation_id,d.channel,d.destination_ref,d.intent,d.status,d.idempotency_key,d.attempt_count,coalesce(d.last_error,''),d.created_at,d.updated_at,d.version`

type scanner interface{ Scan(...any) error }

func read(row scanner) (r delivery.Request, err error) {
	err = row.Scan(&r.DeliveryID, &r.MessageID, &r.ConversationID, &r.Channel, &r.DestinationRef, &r.Intent, &r.Status, &r.IdempotencyKey, &r.AttemptCount, &r.LastError, &r.CreatedAt, &r.UpdatedAt, &r.Version)
	return
}

// Enqueue participates in the producer transaction, after Conversation Append.
// Identity drift fails closed; duplicate event delivery is a read-only success.
func Enqueue(ctx context.Context, tx *sql.Tx, r delivery.Request) error {
	if r.Validate() != nil {
		return delivery.ErrConflict
	}
	old, err := read(tx.QueryRowContext(ctx, `SELECT `+columns+` FROM delivery_request d WHERE delivery_id=? OR message_id=? OR idempotency_key=?`, r.DeliveryID, r.MessageID, r.IdempotencyKey))
	if err == nil {
		if old.DeliveryID != r.DeliveryID || old.MessageID != r.MessageID || old.ConversationID != r.ConversationID || old.Channel != r.Channel || old.DestinationRef != r.DestinationRef || old.Intent != r.Intent || old.IdempotencyKey != r.IdempotencyKey || old.CreatedAt != r.CreatedAt {
			return delivery.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return delivery.ErrStorage
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO delivery_request(delivery_id,message_id,conversation_id,channel,destination_ref,intent,status,idempotency_key,created_at,updated_at,version) VALUES(?,?,?,?,?,?,'pending',?,?,?,1)`, r.DeliveryID, r.MessageID, r.ConversationID, r.Channel, r.DestinationRef, r.Intent, r.IdempotencyKey, r.CreatedAt, r.CreatedAt)
	if err != nil {
		return delivery.ErrStorage
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO delivery_outbox(delivery_id,next_wake_at) VALUES(?,?)`, r.DeliveryID, r.CreatedAt); err != nil {
		return delivery.ErrStorage
	}
	return changed(ctx, tx, r.DeliveryID, "pending")
}

func (s Store) Claim(ctx context.Context, channel, destination string, excluded []string, now time.Time, lease time.Duration) (delivery.Claim, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return delivery.Claim{}, delivery.ErrStorage
	}
	defer tx.Rollback()
	query := `SELECT ` + columns + ` FROM delivery_request d JOIN delivery_outbox o USING(delivery_id) WHERE d.channel=? AND d.destination_ref=? AND o.next_wake_at<=? AND (o.claim_token IS NULL OR o.claim_expires_at<=?)`
	args := []any{channel, destination, now.UnixMicro(), now.UnixMicro()}
	for _, id := range excluded {
		query += ` AND d.delivery_id!=?`
		args = append(args, id)
	}
	query += ` ORDER BY d.created_at,d.delivery_id LIMIT 1`
	r, err := read(tx.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return delivery.Claim{}, delivery.ErrEmpty
	}
	if err != nil {
		return delivery.Claim{}, delivery.ErrStorage
	}
	c := delivery.Claim{Request: r, Token: uuid.Must(uuid.NewV7()).String(), Recover: r.Status == "sending" || r.Status == "unknown"}
	status := "sending"
	increment := 1
	if r.Status == "sending" {
		// Recovery reattaches to the admitted attempt. A receipt query is
		// not a new send attempt and cannot exhaust the retry allowance.
		increment = 0
	}
	if r.Status == "unknown" {
		status = "unknown"
		increment = 0
	}
	updated, err := tx.ExecContext(ctx, `UPDATE delivery_request SET status=?,attempt_count=attempt_count+?,updated_at=?,version=version+1 WHERE delivery_id=? AND version=?`, status, increment, now.UnixMicro(), r.DeliveryID, r.Version)
	if err != nil {
		return c, delivery.ErrStorage
	}
	if n, _ := updated.RowsAffected(); n != 1 {
		return c, delivery.ErrConflict
	}
	if _, err = tx.ExecContext(ctx, `UPDATE delivery_outbox SET claim_token=?,claim_expires_at=? WHERE delivery_id=?`, c.Token, now.Add(lease).UnixMicro(), r.DeliveryID); err != nil {
		return c, delivery.ErrStorage
	}
	if err = changed(ctx, tx, r.DeliveryID, status); err != nil {
		return c, delivery.ErrStorage
	}
	if tx.Commit() != nil {
		return c, delivery.ErrStorage
	}
	c.Request.AttemptCount += increment
	c.Request.Version++
	return c, nil
}

// Renewal never changes semantic state/version. Token, version and live expiry
// are checked atomically, so an expired owner cannot revive itself or overtake
// a recovery owner even when its old transport returns late.
func (s Store) Renew(ctx context.Context, c delivery.Claim, lease time.Duration) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return delivery.ErrStorage
	}
	defer tx.Rollback()
	// Capture time after acquiring the write transaction, not before waiting
	// on another writer. An expired claim must never be revived by lock delay.
	now := time.Now()
	updated, err := tx.ExecContext(ctx, `UPDATE delivery_outbox SET claim_expires_at=max(claim_expires_at,?) WHERE delivery_id=? AND claim_token=? AND claim_expires_at>? AND EXISTS(SELECT 1 FROM delivery_request d WHERE d.delivery_id=delivery_outbox.delivery_id AND d.version=?)`, now.Add(lease).UnixMicro(), c.Request.DeliveryID, c.Token, now.UnixMicro(), c.Request.Version)
	if err != nil {
		return delivery.ErrStorage
	}
	if n, _ := updated.RowsAffected(); n != 1 {
		return delivery.ErrClaimLost
	}
	if tx.Commit() != nil {
		return delivery.ErrStorage
	}
	return nil
}

func (s Store) Settle(ctx context.Context, c delivery.Claim, o delivery.Observation, now, next time.Time) error {
	switch o.Outcome {
	case delivery.Delivered, delivery.Retry, delivery.Failed, delivery.Unknown:
	default:
		o = delivery.Observation{Outcome: delivery.Unknown}
	}
	code := ""
	switch o.Code {
	case "unavailable", "rejected", "retry_exhausted", "unconfirmed", "credentials_unavailable", "credentials_rejected":
		code = o.Code
	}
	if o.Outcome == delivery.Unknown {
		code = "unconfirmed"
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return delivery.ErrStorage
	}
	defer tx.Rollback()
	var valid int
	if tx.QueryRowContext(ctx, `SELECT count(*) FROM delivery_outbox WHERE delivery_id=? AND claim_token=? AND claim_expires_at>?`, c.Request.DeliveryID, c.Token, time.Now().UnixMicro()).Scan(&valid) != nil {
		return delivery.ErrStorage
	}
	if valid != 1 {
		return delivery.ErrClaimLost
	}
	updated, err := tx.ExecContext(ctx, `UPDATE delivery_request SET status=?,last_error=nullif(?,''),updated_at=?,version=version+1 WHERE delivery_id=? AND version=?`, o.Outcome, code, now.UnixMicro(), c.Request.DeliveryID, c.Request.Version)
	if err != nil {
		return delivery.ErrStorage
	}
	if n, _ := updated.RowsAffected(); n != 1 {
		return delivery.ErrClaimLost
	}
	if o.Outcome == delivery.Delivered || o.Outcome == delivery.Failed {
		_, err = tx.ExecContext(ctx, `DELETE FROM delivery_outbox WHERE delivery_id=?`, c.Request.DeliveryID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE delivery_outbox SET claim_token=NULL,claim_expires_at=0,next_wake_at=? WHERE delivery_id=?`, next.UnixMicro(), c.Request.DeliveryID)
	}
	if err != nil {
		return delivery.ErrStorage
	}
	if err = changed(ctx, tx, c.Request.DeliveryID, string(o.Outcome)); err != nil {
		return delivery.ErrStorage
	}
	if tx.Commit() != nil {
		return delivery.ErrStorage
	}
	return nil
}

func (s Store) List(ctx context.Context, conversation string) ([]delivery.Request, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+columns+` FROM delivery_request d WHERE conversation_id=? ORDER BY created_at,delivery_id`, conversation)
	if err != nil {
		return nil, delivery.ErrStorage
	}
	defer rows.Close()
	result := []delivery.Request{}
	for rows.Next() {
		r, err := read(rows)
		if err != nil {
			return nil, delivery.ErrStorage
		}
		result = append(result, r)
	}
	if rows.Err() != nil {
		return nil, delivery.ErrStorage
	}
	return result, nil
}
