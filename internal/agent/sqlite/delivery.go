package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/z-chenhao/erii/internal/agent"
	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery"
	deliverysqlite "github.com/z-chenhao/erii/internal/delivery/sqlite"
	deliveryterminal "github.com/z-chenhao/erii/internal/delivery/terminal"
)

func (s *Store) TerminalInbox() *deliveryterminal.Adapter { return deliveryterminal.New(s.db) }

func (s *Store) Deliveries() deliverysqlite.Store { return deliverysqlite.Store{DB: s.db} }

// Only trusted lifecycle/Security callers create these four semantic notices.
// Workers never enqueue their own completion or ordinary progress.
func (r *Runs) notification(ctx context.Context, tx *sql.Tx, runID, id, intent, source, text string) error {
	var conversationID string
	if err := tx.QueryRowContext(ctx, `SELECT conversation_id FROM run WHERE run_id=?`, runID).Scan(&conversationID); err != nil {
		return err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM notification_intent WHERE message_id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	m := conversation.Message{MessageID: id, ConversationID: conversationID, Role: "assistant", Content: conversation.Text(text), Source: source, SourceEventID: id, CreatedAt: time.Now().UTC().Truncate(time.Microsecond)}
	raw, err := encoded(m)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO conversation_intent(message_id,message_json) VALUES(?,?)`, id, raw); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO notification_intent VALUES(?,?)`, id, intent)
	return err
}

func (r *Runs) enqueueNotifications(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT i.message_json,n.intent FROM notification_intent n JOIN conversation_intent i USING(message_id) WHERE i.flushed=1 AND NOT EXISTS(SELECT 1 FROM delivery_request d WHERE d.message_id=n.message_id) ORDER BY i.sequence`)
	if err != nil {
		return err
	}
	type notice struct {
		m      conversation.Message
		intent string
	}
	items := []notice{}
	for rows.Next() {
		var raw string
		var n notice
		if rows.Scan(&raw, &n.intent) != nil || json.Unmarshal([]byte(raw), &n.m) != nil {
			rows.Close()
			return agent.ErrStorage
		}
		items = append(items, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, n := range items {
		id := "delivery:" + n.m.MessageID
		if err = deliverysqlite.Enqueue(ctx, tx, delivery.Request{DeliveryID: id, MessageID: n.m.MessageID, ConversationID: n.m.ConversationID, Channel: "tui", DestinationRef: n.m.ConversationID, Intent: n.intent, IdempotencyKey: id, CreatedAt: n.m.CreatedAt.UnixMicro()}); err != nil {
			return err
		}
	}
	return nil
}
