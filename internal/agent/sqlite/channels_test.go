package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/z-chenhao/collie/internal/agent"
	"github.com/z-chenhao/collie/internal/conversation"
	"github.com/z-chenhao/collie/internal/delivery"
	"github.com/z-chenhao/collie/internal/interaction"
	"github.com/z-chenhao/collie/internal/interaction/feishu"
	"github.com/z-chenhao/collie/internal/tool"
)

// Historical migration fixtures must remove later empty schemas, rather than
// silently preserve them while pretending to start from an older schema version.
// Reconstruct the actual historical schema while retaining every accepted
// business row. Later additive Outbox columns must also be removed, otherwise
// the forward migration test is exercising a mislabeled current schema.
const removeChannelFixtureSchema = removeInspectionFixtureSchema + `DROP INDEX event_outbox_live; ALTER TABLE event_outbox DROP COLUMN dead_at; ALTER TABLE event_outbox DROP COLUMN failure_code; ALTER TABLE event_outbox DROP COLUMN failure_count; DROP TABLE channel_resource; DROP TABLE channel_receipt; DROP TABLE delivery_projection; DROP TABLE channel_preference; DROP TABLE message_route; DROP TABLE channel_callback; DROP TABLE channel_source; DROP TABLE channel_binding; `

func TestChannelConcurrentCallbackAndSourceDedupSharedConversation(t *testing.T) {
	s, r, h, a, _ := loopFixture(t)
	ctx := context.Background()
	web, e := s.BindChannel(ctx, "web", "browser", "local", true, 0)
	if e != nil {
		t.Fatal(e)
	}
	fs, e := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
	if e != nil {
		t.Fatal(e)
	}
	in := interaction.InboundEvent{Channel: fs.Channel, Subject: fs.Subject, Destination: fs.Destination, SourceID: "source", Content: conversation.Text("from Feishu")}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	receipts := make(chan agent.Receipt, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q := in
			q.EventID = fmt.Sprintf("retry-%d", i%4)
			v, e := r.AcceptInbound(ctx, q)
			errs <- e
			receipts <- v
		}(i)
	}
	wg.Wait()
	close(errs)
	close(receipts)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first agent.Receipt
	for v := range receipts {
		if first.MessageID == "" {
			first = v
		} else if !reflect.DeepEqual(first, v) {
			t.Fatal("duplicate created another receipt")
		}
	}
	if scalar(t, s, `SELECT count(*) FROM main_input`) != 1 || scalar(t, s, `SELECT count(*) FROM channel_callback`) != 4 || scalar(t, s, `SELECT count(*) FROM run WHERE kind='main'`) != 1 {
		t.Fatal("duplicate admission")
	}
	bad := in
	bad.EventID = "retry-0"
	bad.SourceID = "other"
	if _, e = r.AcceptInbound(ctx, bad); !errors.Is(e, agent.ErrConflict) {
		t.Fatal("callback drift", e)
	}
	bad = in
	bad.EventID = "different-event"
	bad.Content = conversation.Text("changed")
	if _, e = r.AcceptInbound(ctx, bad); !errors.Is(e, agent.ErrConflict) {
		t.Fatal("source drift", e)
	}
	bad = in
	bad.Subject = "stranger"
	bad.EventID = "unbound"
	if _, e = r.AcceptInbound(ctx, bad); !errors.Is(e, interaction.ErrUnauthorized) {
		t.Fatal("unbound accepted", e)
	}
	// Web and TUI steer the same Main, retain independent ordered messages.
	if _, e = r.AcceptInbound(ctx, interaction.InboundEvent{Channel: web.Channel, Subject: web.Subject, Destination: web.Destination, EventID: "web", SourceID: "web", Content: conversation.Text("from Web")}); e != nil {
		t.Fatal(e)
	}
	mustSubmit(t, a, "terminal", "from TUI")
	mustDrain(t, a)
	messages, e := h.List(ctx, firstConversation(t, s))
	if e != nil {
		t.Fatal(e)
	}
	if len(messages) != 4 || messages[0].Source != "feishu" || messages[1].Source != "web" || messages[2].Source != "cli" {
		t.Fatalf("shared ordering %+v", messages)
	}
	rows := deliveryRows(t, s)
	if len(rows) != 1 || rows[0].Channel != "tui" {
		t.Fatal("latest TUI input inherited older channel", rows)
	}
	for i, m := range messages {
		if m.Sequence != int64(i+1) {
			t.Fatal("unordered", messages)
		}
	}
}
func firstConversation(t *testing.T, s *Store) string {
	t.Helper()
	d, e := s.Diagnostics(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	return d.AgentID
}
func TestChannelRouteFreezeCASRevocationAndDefault(t *testing.T) {
	s, r, _, a, _ := loopFixture(t)
	ctx := context.Background()
	b, e := s.BindChannel(ctx, "feishu", "tenant:app:owner", "chat", true, 0)
	if e != nil {
		t.Fatal(e)
	}
	q := interaction.InboundEvent{Channel: b.Channel, Subject: b.Subject, Destination: b.Destination, EventID: "e", SourceID: "s", Content: conversation.Text("reply to bound destination")}
	if _, e = r.AcceptInbound(ctx, q); e != nil {
		t.Fatal(e)
	}
	mustDrain(t, a)
	rows := deliveryRows(t, s)
	if len(rows) != 1 || rows[0].Channel != "feishu" || rows[0].DestinationRef != "chat" {
		t.Fatal(rows)
	}
	if _, e = s.DeliveryBinding(ctx, rows[0]); e != nil {
		t.Fatal(e)
	}
	drift := rows[0]
	drift.DestinationRef = "other"
	if _, e := s.DeliveryBinding(ctx, drift); e == nil {
		t.Fatal("frozen destination drift allowed")
	}
	wrongApp := feishu.Adapter{Client: feishu.Client{Config: feishu.Config{AppID: "other-app", TenantKey: "tenant"}}, Store: s}
	messages, err := r.history.List(ctx, firstConversation(t, s))
	if err != nil {
		t.Fatal(err)
	}
	var message conversation.Message
	for _, m := range messages {
		if m.MessageID == rows[0].MessageID {
			message = m
		}
	}
	envelope, err := delivery.Compose(rows[0], message)
	if err != nil {
		t.Fatal("account-fence fixture must be an otherwise valid frozen delivery", err)
	}
	if observed := wrongApp.Send(ctx, envelope); observed.Outcome != delivery.Failed {
		t.Fatal("changed Feishu app acquired old delivery", observed)
	}
	if _, e = s.BindChannel(ctx, b.Channel, b.Subject, b.Destination, false, 0); !errors.Is(e, agent.ErrConflict) {
		t.Fatal("binding CAS bypass", e)
	}
	if _, e = s.BindChannel(ctx, b.Channel, b.Subject, b.Destination, false, b.Version); e != nil {
		t.Fatal(e)
	}
	if _, e := s.DeliveryBinding(ctx, rows[0]); e == nil {
		t.Fatal("revoked delivery allowed")
	}
	if _, e = r.AcceptInbound(ctx, q); !errors.Is(e, interaction.ErrUnauthorized) {
		t.Fatal("revoked callback accepted", e)
	}
	if e = s.PreferChannel(ctx, "feishu", b.ID, 1); !errors.Is(e, interaction.ErrUnauthorized) {
		t.Fatal("revoked preference", e)
	}
	if e = s.PreferChannel(ctx, "tui", "", 1); e != nil {
		t.Fatal(e)
	}
	if e = s.PreferChannel(ctx, "auto", "", 1); !errors.Is(e, agent.ErrConflict) {
		t.Fatal("preference CAS bypass", e)
	}
	rows = deliveryRows(t, s)
	if rows[0].Channel != "feishu" || rows[0].DestinationRef != "chat" {
		t.Fatal("frozen destination changed")
	}
}
func TestChannelMigrationV24PreservesDataRollbackAndProjection(t *testing.T) {
	ctx := context.Background()
	db := schemaDB(t)
	if e := migrate(ctx, db, migrations[:24]); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO agent_config VALUES(1,'prior-channel',NULL,'setup_required',7,1,1)`); e != nil {
		t.Fatal(e)
	}
	broken := append(append([]string{}, migrations[:24]...), channelsSQL+`; BAD DDL`)
	if migrate(ctx, db, broken) == nil {
		t.Fatal("broken migration accepted")
	}
	var v, n int
	db.QueryRow(`PRAGMA user_version`).Scan(&v)
	db.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE name='channel_binding'`).Scan(&n)
	if v != 24 || n != 0 {
		t.Fatal("partial migration")
	}
	if e := migrate(ctx, db, migrations); e != nil {
		t.Fatal(e)
	}
	var id string
	db.QueryRow(`SELECT agent_id FROM agent_config`).Scan(&id)
	if id != "prior-channel" {
		t.Fatal("lost prior data")
	}
	s, _, _, a, _ := loopFixture(t)
	mustSubmit(t, a, "projection", "safe <text>")
	mustDrain(t, a)
	row := deliveryRows(t, s)[0]
	raw, e := s.SaveProjection(ctx, row.DeliveryID, "frozen", 1, map[string]string{"text": "safe"})
	if e != nil || !json.Valid(raw) {
		t.Fatal(e)
	}
	again, e := s.SaveProjection(ctx, row.DeliveryID, "frozen", 2, map[string]string{"text": "new"})
	if e != nil || string(again) != string(raw) {
		t.Fatal("projection was recomposed", e)
	}
	if _, e = s.SaveProjection(ctx, row.DeliveryID, "drift", 1, map[string]string{}); e == nil {
		t.Fatal("envelope drift accepted")
	}
}

func TestChannelApprovalRacesAndSteeringFrozenHash(t *testing.T) {
	ctx := context.Background()
	calls := effectCall(`{"target":"Alex","text":"Tuesday"}`)
	s, r, _, executor, exchange := admitPolicy(t, []tool.Definition{effectDef(t, "query_before_retry")}, calls)
	if _, e := executor.Execute(ctx, exchange, calls); !errors.Is(e, tool.ErrSuspended) {
		t.Fatal(e)
	}
	views, e := r.Approvals(ctx)
	if e != nil || len(views) != 1 {
		t.Fatal(views, e)
	}
	v := views[0]
	for _, b := range []interaction.Binding{{Channel: "web", Subject: "browser", Destination: "local"}, {Channel: "feishu", Subject: "tenant:app:owner", Destination: "chat"}} {
		if _, e := s.BindChannel(ctx, b.Channel, b.Subject, b.Destination, true, 0); e != nil {
			t.Fatal(e)
		}
	}
	event := func(channel, key, decision string) interaction.InboundEvent {
		subject, dest := "browser", "local"
		if channel == "feishu" {
			subject, dest = "tenant:app:owner", "chat"
		}
		return interaction.InboundEvent{Channel: channel, Subject: subject, Destination: dest, EventID: key, SourceID: key, Content: conversation.Text(v.ID + v.Hash + decision)}
	}
	wrong := event("web", "wrong-hash", "approved")
	if e := r.DecideInbound(ctx, wrong, v.ID, "wrong", "approved"); !errors.Is(e, agent.ErrConflict) {
		t.Fatal("changed approval accepted", e)
	}
	if scalar(t, s, `SELECT count(*) FROM channel_source`) != 0 {
		t.Fatal("failed decision partially committed callback")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			channel := "web"
			if i%2 == 1 {
				channel = "feishu"
			}
			errs <- r.DecideInbound(ctx, event(channel, fmt.Sprint(i), "approved"), v.ID, v.Hash, "approved")
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	if e := r.DecideInbound(ctx, event("feishu", "late-deny", "denied"), v.ID, v.Hash, "denied"); !errors.Is(e, agent.ErrConflict) {
		t.Fatal("conflicting decision accepted", e)
	}
	if _, e := executor.Execute(ctx, exchange, calls); e != nil {
		t.Fatal(e)
	}
	if scalar(t, s, `SELECT attempt_count FROM tool_invocation`) != 1 {
		t.Fatal("duplicate approved effect")
	}
}
