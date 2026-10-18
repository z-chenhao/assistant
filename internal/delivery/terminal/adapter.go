// Package terminal owns the local terminal's durable inbox. A terminal session
// renders this retained projection; reconnecting may redisplay history. Raw
// stdout has no acknowledgement protocol and is not an exactly-once transport.
package terminal

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/event"
	eventsqlite "github.com/z-chenhao/erii/internal/event/sqlite"
	"github.com/z-chenhao/erii/internal/llm"
)

type Adapter struct{ db *sql.DB }

func (*Adapter) Idempotent() bool { return true }

func New(db *sql.DB) *Adapter { return &Adapter{db: db} }

// freeze removes delivery bookkeeping, which can change between retries.
func freeze(e delivery.Envelope) delivery.Envelope {
	e.Request.Status = ""
	e.Request.AttemptCount = 0
	e.Request.LastError = ""
	e.Request.UpdatedAt = 0
	e.Request.Version = 0
	return e
}
func (a *Adapter) Query(ctx context.Context, e delivery.Envelope) delivery.Observation {
	var raw string
	err := a.db.QueryRowContext(ctx, `SELECT envelope_json FROM terminal_message WHERE idempotency_key=? OR delivery_id=? OR message_id=?`, e.Request.IdempotencyKey, e.Request.DeliveryID, e.Request.MessageID).Scan(&raw)
	if err == sql.ErrNoRows {
		return delivery.Observation{Outcome: delivery.Retry}
	}
	if err != nil {
		return delivery.Observation{Outcome: delivery.Unknown}
	}
	expected, _ := llm.CanonicalJSON(freeze(e))
	if raw != string(expected) {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	return delivery.Observation{Outcome: delivery.Delivered}
}
func (a *Adapter) Send(ctx context.Context, e delivery.Envelope) delivery.Observation {
	if _, err := delivery.Compose(e.Request, e.Message); err != nil || e.Request.Channel != "tui" || e.Request.DestinationRef != e.Message.ConversationID {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	if observed := a.Query(ctx, e); observed.Outcome != delivery.Retry {
		return observed
	}
	raw, err := llm.CanonicalJSON(freeze(e))
	if err != nil {
		return delivery.Observation{Outcome: delivery.Failed, Code: "rejected"}
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return delivery.Observation{Outcome: delivery.Retry, Code: "unavailable"}
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO terminal_message VALUES(?,?,?,?,?)`, e.Request.IdempotencyKey, e.Request.DeliveryID, e.Request.MessageID, e.Request.DestinationRef, string(raw))
	// A concurrent sender or commit error requires authoritative readback,
	// never an assumption that the write failed before taking effect.
	if err != nil {
		tx.Rollback()
		return a.Query(ctx, e)
	}
	accepted, err := event.New("/erii/delivery", "ai.erii.delivery.accepted.v1", e.Request.DeliveryID, struct {
		ID string `json:"delivery_id"`
	}{e.Request.DeliveryID})
	if err == nil {
		err = eventsqlite.Enqueue(ctx, tx, accepted)
	}
	if err != nil {
		tx.Rollback()
		return delivery.Observation{Outcome: delivery.Retry, Code: "unavailable"}
	}
	if tx.Commit() != nil {
		return a.Query(ctx, e)
	}
	return delivery.Observation{Outcome: delivery.Delivered}
}
func (a *Adapter) Messages(ctx context.Context, destination string) ([]delivery.Envelope, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT envelope_json FROM terminal_message WHERE destination_ref=? ORDER BY rowid`, destination)
	if err != nil {
		return nil, delivery.ErrStorage
	}
	defer rows.Close()
	result := []delivery.Envelope{}
	for rows.Next() {
		var raw string
		var e delivery.Envelope
		if rows.Scan(&raw) != nil || json.Unmarshal([]byte(raw), &e) != nil {
			return nil, delivery.ErrStorage
		}
		result = append(result, e)
	}
	if rows.Err() != nil {
		return nil, delivery.ErrStorage
	}
	return result, nil
}

// History includes retained pre-v12 sent messages without projection rows.
// Unsent semantic intents stay hidden until this destination accepts them.
// Input remains visible immediately.
func (a *Adapter) History(ctx context.Context, destination string, messages []conversation.Message, requests []delivery.Request) ([]conversation.Message, error) {
	inbox, err := a.Messages(ctx, destination)
	if err != nil {
		return nil, err
	}
	accepted := map[string]bool{}
	for _, e := range inbox {
		accepted[e.Message.MessageID] = true
	}
	for _, r := range requests {
		if r.Channel == "tui" && r.DestinationRef == destination && r.Status == "sent" {
			accepted[r.MessageID] = true
		}
	}
	result := []conversation.Message{}
	for _, m := range messages {
		if m.Role == "user" || accepted[m.MessageID] {
			result = append(result, m)
		}
	}
	return result, nil
}
