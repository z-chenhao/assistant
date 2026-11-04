package database

import "testing"

func TestCursorStaysWithinProjectionAndOwner(t *testing.T) {
	q := Query{Collection: "messages", Cursor: Next("messages", "owner", 42, "message-42")}
	c, err := Position(q, "owner")
	if err != nil || c.At != 42 || c.ID != "message-42" {
		t.Fatal("valid continuation failed", err)
	}
	if _, err = Position(q, "other"); err != ErrInvalid {
		t.Fatal("cursor crossed owner")
	}
	q.Collection = "memories"
	if _, err = Position(q, "owner"); err != ErrInvalid {
		t.Fatal("cursor crossed collection")
	}
	for _, q := range []Query{{Collection: "auth_connection"}, {Collection: "executions", Limit: 51}, {Collection: "executions", Limit: -1}, {Collection: "messages", Ref: "memory:record"}, {Collection: "messages", Ref: "conversation:record", Cursor: "a"}, {Collection: "executions", Ref: "run:../host"}} {
		if q.Validate() != ErrInvalid {
			t.Fatal("invalid projection query accepted", q)
		}
	}
}
