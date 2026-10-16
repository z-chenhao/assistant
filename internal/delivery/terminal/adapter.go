// Package terminal owns the local terminal's durable inbox. A terminal session
// renders this retained projection; reconnecting may redisplay history. Raw
// stdout has no acknowledgement protocol and is not an exactly-once transport.
package terminal

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"

	"github.com/z-chenhao/erii/internal/conversation"
	"github.com/z-chenhao/erii/internal/delivery"
	"github.com/z-chenhao/erii/internal/llm"
	_ "modernc.org/sqlite"
)

type Adapter struct{ db *sql.DB }

func (*Adapter) Idempotent() bool { return true }

func Open(ctx context.Context, root string) (*Adapter, error) {
	if !filepath.IsAbs(root) {
		return nil, delivery.ErrStorage
	}
	path := filepath.Join(root, "terminal.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, delivery.ErrStorage
	}
	if f.Close() != nil {
		return nil, delivery.ErrStorage
	}
	u := url.URL{Scheme: "file", Path: path}
	q := u.Query()
	for _, p := range []string{"busy_timeout(5000)", "journal_mode(WAL)"} {
		q.Add("_pragma", p)
	}
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, delivery.ErrStorage
	}
	db.SetMaxOpenConns(1)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, delivery.ErrStorage
	}
	defer tx.Rollback()
	var version int
	if tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version) != nil || version < 0 || version > 1 {
		tx.Rollback()
		db.Close()
		return nil, delivery.ErrStorage
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `CREATE TABLE terminal_message (
 idempotency_key TEXT PRIMARY KEY, delivery_id TEXT NOT NULL UNIQUE, message_id TEXT NOT NULL UNIQUE,
 destination_ref TEXT NOT NULL, envelope_json TEXT NOT NULL CHECK(json_valid(envelope_json))
) STRICT;
CREATE TRIGGER terminal_immutable BEFORE UPDATE ON terminal_message BEGIN SELECT RAISE(ABORT,'immutable terminal receipt'); END;
CREATE TRIGGER terminal_retained BEFORE DELETE ON terminal_message BEGIN SELECT RAISE(ABORT,'terminal history retained'); END;
CREATE TRIGGER terminal_no_replace BEFORE INSERT ON terminal_message WHEN EXISTS(SELECT 1 FROM terminal_message WHERE idempotency_key=NEW.idempotency_key OR delivery_id=NEW.delivery_id OR message_id=NEW.message_id) BEGIN SELECT RAISE(ABORT,'terminal identity exists'); END;
PRAGMA user_version=1;`)
	}
	if err != nil {
		tx.Rollback()
		db.Close()
		return nil, delivery.ErrStorage
	}
	if tx.Commit() != nil {
		db.Close()
		return nil, delivery.ErrStorage
	}
	return &Adapter{db: db}, nil
}

// freeze removes delivery bookkeeping, which can change between retries.
func freeze(e delivery.Envelope) delivery.Envelope {
	e.Request.Status = ""
	e.Request.AttemptCount = 0
	e.Request.LastError = ""
	e.Request.UpdatedAt = 0
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
	_, err = a.db.ExecContext(ctx, `INSERT INTO terminal_message VALUES(?,?,?,?,?)`, e.Request.IdempotencyKey, e.Request.DeliveryID, e.Request.MessageID, e.Request.DestinationRef, string(raw))
	// A concurrent sender or commit error requires authoritative readback,
	// never an assumption that the write failed before taking effect.
	if err != nil {
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
func (a *Adapter) Close() error { return a.db.Close() }

// History is the terminal projection, including retained pre-v12 sent messages
// whose original receipt predates terminal.db. Unsent semantic intents stay
// hidden until this destination accepts them. Input remains visible immediately.
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
	sort.SliceStable(result, func(i, j int) bool { return result[i].Sequence < result[j].Sequence })
	return result, nil
}
